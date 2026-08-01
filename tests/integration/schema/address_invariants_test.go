package schema_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddressInvariants pins the address-space contract:
//
//   - single-step (root-level) terminal fields get unique addresses in [0, 2^14)
//   - multi-step paths get addresses in [2^14, 2^27)
//   - the two regions never overlap
func TestAddressInvariants(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	singleSeen := make(map[uint32]string)
	var multiSeen []uint32

	for i, meta := range cs.FieldsMeta {
		fd := cs.Descriptors[i]
		if fd.SchemaIdx() != 0 {
			continue
		}

		// Single-step path for every root field.
		step := definition.NewResolvedStep(0, fd.FieldIdx())
		addr := definition.Address(cs, definition.ResolvedPath{step})

		if fd.Terminal() {
			assert.Less(t, addr, uint32(definition.SingleStepRegion), "single-step address of %q must stay in [0, 2^14)", meta.Name)
			// Address 0 is the sentinel shared with "not addressable" (and is a
			// legitimate slot for the first field), so it is excluded from the
			// uniqueness set — mirroring Address()'s own contract.
			if addr == 0 {
				continue
			}
			if prev, dup := singleSeen[addr]; dup {
				t.Fatalf("address collision: %q and %q both map to %d", prev, meta.Name, addr)
			}
			singleSeen[addr] = meta.Name
		} else {
			// Non-terminal root fields are structural and not addressable alone.
			assert.Equal(t, uint32(0), addr, "non-terminal root field %q must not receive an address", meta.Name)
		}

		// Multi-step paths: descend into every child field of object/array
		// containers that carry a child schema slot.
		childIdx := fd.ChildSchemaIdx()
		if fd.Terminal() || childIdx == definition.FdNoChild {
			continue
		}
		for j, childMeta := range cs.FieldsMeta {
			cFd := cs.Descriptors[j]
			if cFd.SchemaIdx() != childIdx {
				continue
			}
			path := definition.ResolvedPath{
				definition.NewResolvedStep(0, fd.FieldIdx()),
				definition.NewResolvedStep(childIdx, cFd.FieldIdx()),
			}
			maddr := definition.Address(cs, path)
			assert.GreaterOrEqual(t, maddr, uint32(definition.MultiStepBase), "multi-step address of %q.%q must live in [2^14, 2^27)", meta.Name, childMeta.Name)
			assert.Less(t, maddr, uint32(1<<definition.AddrBits), "multi-step address of %q.%q exceeds address space", meta.Name, childMeta.Name)
			multiSeen = append(multiSeen, maddr)
		}
	}

	// Every single-step address must be disjoint from every multi-step address
	// by construction (different regions).
	for addr := range singleSeen {
		assert.Less(t, addr, uint32(definition.SingleStepRegion))
	}
	for _, m := range multiSeen {
		assert.GreaterOrEqual(t, m, uint32(definition.SingleStepRegion))
	}

	// Multi-step addresses must themselves be unique.
	multiSet := make(map[uint32]bool, len(multiSeen))
	for _, m := range multiSeen {
		require.False(t, multiSet[m], "duplicate multi-step address %d", m)
		multiSet[m] = true
	}
	assert.NotEmpty(t, multiSeen)
}

// TestAddress_EmptyPath verifies the degenerate path yields address 0.
func TestAddress_EmptyPath(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)
	assert.Equal(t, uint32(0), definition.Address(cs, nil))
}

// TestAddress_TerminalityAgreement ensures the descriptor's Terminal() flag and
// the address allocator agree about what is addressable. Address 0 is both the
// first single-step slot and the "not addressable" sentinel, so terminal fields
// are checked against the region bound rather than for non-zero-ness.
func TestAddress_TerminalityAgreement(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	for _, fd := range cs.Descriptors {
		if fd.SchemaIdx() != 0 {
			continue
		}
		addr := definition.Address(cs, definition.ResolvedPath{definition.NewResolvedStep(0, fd.FieldIdx())})
		if fd.Terminal() {
			assert.Less(t, addr, uint32(definition.SingleStepRegion))
		} else {
			assert.Zero(t, addr)
		}
	}
}
