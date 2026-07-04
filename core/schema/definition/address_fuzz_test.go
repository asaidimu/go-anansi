package definition_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/document"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

// FuzzAddressInvariants constructs synthetic CompiledSchemas that stress-test
// the Address() block-subdivision algorithm: deep chains, wide flat schemas,
// mixed terminal/non-terminal fields, and recursive back-edges.
func FuzzAddressInvariants(f *testing.F) {
	for _, seed := range addressSeeds() {
		f.Add(
			seed.numSchemas,
			seed.schemaFields,
			seed.terminals,
			seed.recursive,
		)
	}

	f.Fuzz(func(t *testing.T, numSchemas uint8, schemaFields uint8, terminals uint8, recursive int8) {
		// Clamp to safe bounds.
		n := int(numSchemas) % 63
		if n == 0 {
			n = 1
		}
		nFields := int(schemaFields) % 8
		if nFields == 0 {
			nFields = 1
		}

		cs := buildChain(n, nFields, terminals, recursive)
		if cs == nil {
			return
		}

		paths := enumeratePaths(cs)
		if len(paths) == 0 {
			return
		}
		if len(paths) > 10000 {
			return // too many paths to verify
		}

		seen := make(map[uint32]string)
		zeroCnt := 0
		for _, p := range paths {
			addr := definition.Address(cs, p)
			key := p.PathKey()

			if addr == 0 {
				zeroCnt++
				continue
			}

			if addr >= 1<<27 {
				t.Errorf("address overflow: path %s -> %08x (>= 2^27)", key, addr)
				return
			}

			if len(p) == 1 && addr >= 1<<14 {
				t.Errorf("single-step address out of range: path %s -> %d (>= 2^14)", key, addr)
				return
			}

			if len(p) > 1 && addr < 1<<14 {
				t.Errorf("multi-step address in single-step range: path %s -> %d (< 2^14)", key, addr)
				return
			}

			if prev, dup := seen[addr]; dup {
				t.Errorf("address collision %08x: paths %q and %q (len %d and %d)", addr, prev, key, len(prev)/2, len(key)/2)
				t.Logf("  path1 steps:")
				for i := 0; i < len(prev)/2; i++ {
					t.Logf("    (%d, %d)", prev[i*2], prev[i*2+1])
				}
				t.Logf("  path2 steps:")
				for i := 0; i < len(key)/2; i++ {
					t.Logf("    (%d, %d)", key[i*2], key[i*2+1])
				}
				return
			}
			seen[addr] = key
		}
		_ = zeroCnt
	})
}

// ---------------------------------------------------------------------------
// Seed corpus
// ---------------------------------------------------------------------------

type addressSeed struct {
	numSchemas   uint8
	schemaFields uint8
	terminals    uint8
	recursive    int8
}

func addressSeeds() []addressSeed {
	return []addressSeed{
		// Deep chain, single terminal per schema.
		{numSchemas: 30, schemaFields: 1, terminals: 0b0001, recursive: -1},
		// Deep chain, single non-terminal per schema (all non-terminal, terminals=0).
		{numSchemas: 30, schemaFields: 1, terminals: 0b0000, recursive: -1},
		// Wide flat schema, all terminal.
		{numSchemas: 1, schemaFields: 8, terminals: 0b1111, recursive: -1},
		// Mixed: half terminal, half non-terminal at each level.
		{numSchemas: 10, schemaFields: 4, terminals: 0b0101, recursive: -1},
		// Deep chain with recursive back-edge at varying depths.
		{numSchemas: 10, schemaFields: 2, terminals: 0b0001, recursive: 0},
		{numSchemas: 10, schemaFields: 2, terminals: 0b0010, recursive: 5},
	}
}

// ---------------------------------------------------------------------------
// Synthetic CompiledSchema builder
// ---------------------------------------------------------------------------

// buildChain constructs a linear chain of n schemas.
// Schema i points to schema i+1 via its first non-terminal field.
// terminals is a bitmask; if bit j is set, field j is KindSimple+Terminal.
// recursiveDepth: if >= 0, the field at that position in the last schema
// points back to schema 0 (recursive back-edge).
func buildChain(n, nFields int, terminals uint8, recursiveDepth int8) *definition.CompiledSchema {
	descriptors := make([]definition.FieldDescriptor, 0, n*nFields)
	slots := make([]definition.SchemaSlot, n)
	meta := make([]definition.SchemaMeta, n)

	// Assign schema slots sequentially.
	for i := 0; i < n; i++ {
		slots[i].FieldStart = uint16(i * nFields)
		slots[i].FieldCount = uint16(nFields)
	}

	// Build descriptors.
	for i := 0; i < n; i++ {
		for j := 0; j < nFields; j++ {
			terminal := (terminals>>j)&1 != 0
			childSchemaIdx := uint8(definition.FdNoChild)
			kind := definition.KindSimple

			// Last field in the chain: if we hit the end or this is a recursive anchor.
			if i == n-1 && j == nFields-1 && recursiveDepth >= 0 {
				// Recursive back-edge: terminal object pointing back to root.
				childSchemaIdx = definition.FdNoChild
				kind = definition.KindObject
				terminal = true
				fd := definition.MakeFieldDescriptor(
					document.TypeUnknown, kind, uint8(i), uint8(j),
					false, false, false, false, terminal, false, true,
					childSchemaIdx,
				)
				descriptors = append(descriptors, fd)
				continue
			}

			if !terminal {
				childSchemaIdx = uint8(i + 1)
				if i == n-1 {
					// Last schema: no further children; make this field terminal.
					terminal = true
					childSchemaIdx = definition.FdNoChild
				} else {
					kind = definition.KindObject
				}
			}

			fd := definition.MakeFieldDescriptor(
				document.TypeUnknown, kind, uint8(i), uint8(j),
				false, false, false, false, terminal, false, false,
				childSchemaIdx,
			)
			descriptors = append(descriptors, fd)
		}
	}

	// Compute footprints bottom-up.
	for i := n - 1; i >= 0; i-- {
		var fp uint32
		for j := 0; j < nFields; j++ {
			fd := descriptors[i*nFields+j]
			if fd.Terminal() {
				fp++
			} else if fd.ChildSchemaIdx() != definition.FdNoChild {
				fp += slots[fd.ChildSchemaIdx()].Footprint
			}
		}
		slots[i].Footprint = fp
	}

	return &definition.CompiledSchema{
		Descriptors: descriptors,
		Schemas:     slots,
		SchemasMeta: meta,
	}
}

// enumeratePaths returns every valid ResolvedPath in the CompiledSchema,
// stopping early once the cap is reached (to avoid exponential blowup).
const pathCap = 5000

func enumeratePaths(cs *definition.CompiledSchema) []definition.ResolvedPath {
	result := make([]definition.ResolvedPath, 0, 256)
	var walk func(schemaIdx uint8, prefix definition.ResolvedPath)
	walk = func(schemaIdx uint8, prefix definition.ResolvedPath) {
		if len(result) >= pathCap {
			return
		}
		if int(schemaIdx) >= len(cs.Schemas) {
			return
		}
		slot := cs.Schemas[schemaIdx]
		for j := uint8(0); j < uint8(slot.FieldCount); j++ {
			if len(result) >= pathCap {
				return
			}
			step := definition.NewResolvedStep(schemaIdx, j)
			path := append(append(definition.ResolvedPath(nil), prefix...), step)
			result = append(result, path)

			fd := cs.Descriptors[int(slot.FieldStart)+int(j)]
			if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
				walk(fd.ChildSchemaIdx(), path)
			}
		}
	}
	walk(0, nil)
	return result
}
