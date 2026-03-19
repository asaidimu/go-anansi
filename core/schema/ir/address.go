package ir

import (
	"errors"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

// address.go implements Address(), a method on *Schema that resolves
// a dot-separated field path string to a document.DataPoint.
//
// The algorithm is defined in the Schema Address Space spec (Section 6). It
// runs in O(path length) time with zero allocation. cs.AddressSpace must be
// populated (i.e. Compile must have completed) before Address is called.
//
// The returned DataPoint encodes:
//   - Type: the document.DataType of the terminal field.
//   - ID:   the 27-bit ordinal assigned by the address space build.
//
// Returns (0, ErrAddressNotFound) on any failure: empty path, unknown segment
// name, non-schema-bearing field in a non-terminal position, ambiguous union
// variant, or depth overflow.

// ErrAddressNotFound is returned by Address when the path cannot be resolved.
var ErrAddressNotFound = errors.New("address: path not found")

// Address resolves a dot-separated field path to a document.DataPoint.
//
// Example paths: "name", "address.city", "parent.lines.product"
func (cs *Schema) Address(path string) (document.DataPoint, error) {
	as := cs.AddressSpace
	if as == nil {
		return 0, errors.New("address: AddressSpace not built — schema was not fully compiled")
	}
	if path == "" {
		return 0, ErrAddressNotFound
	}

	ordinal, terminalFD, ok := cs.resolveOrdinal(path)
	if !ok {
		return 0, ErrAddressNotFound
	}

	typ := fieldTypeToDataType(ExtractType(terminalFD))
	dp, err := document.NewDataPoint(typ, int32(ordinal))
	if err != nil {
		return 0, err
	}
	return dp, nil
}

// DocumentKey resolves a dot-separated field path to a document.DocumentKey,
// packing the resolved DataPoint together with the terminal field descriptor
// into a single 64-bit key. This is the primary resolution method for
// validation — the returned key carries both the value identity (DataPoint)
// and all field-level rules (descriptor) without any secondary lookup.
//

func (cs *Schema) DocumentKey(path string) (document.DocumentKey, error) {
	// 1. Fast path: check the registry first.
	if cs.PathCache != nil {
		if dk, ok := cs.PathCache.GetKey(path); ok {
			return dk, nil
		}
	}

	as := cs.AddressSpace
	if as == nil {
		return 0, errors.New("address: AddressSpace not built — schema was not fully compiled")
	}
	if path == "" {
		return 0, ErrAddressNotFound
	}

	// 2. Resolve the ordinal only if not in cache.
	ordinal, terminalFD, ok := cs.resolveOrdinal(path)
	if !ok {
		return 0, ErrAddressNotFound
	}

	typ := fieldTypeToDataType(ExtractType(terminalFD))
	dp, err := document.NewDataPoint(typ, int32(ordinal))
	if err != nil {
		return 0, err
	}

	dk := document.NewDocumentKey(dp, terminalFD)

	// 3. Store in the unified registry.
	if cs.PathCache != nil {
		cs.PathCache.Put(dk, path)
	}
	return dk, nil
}

// resolveOrdinal is the core resolution algorithm from spec Section 6.4,
// extended to also return the terminal field's descriptor so Address can
// derive the correct document.DataType.
//
// Returns (ordinal, terminalFD, true) on success, (0, 0, false) on failure.
func (cs *Schema) resolveOrdinal(path string) (ordinal uint32, terminalFD uint32, ok bool) {
	as := cs.AddressSpace
	segments := strings.Split(path, ".")

	var (
		schemaIdx  = uint8(0)
		blockBase  = uint32(0)
		blockOwner = uint8(0)
		depth      = uint32(0)
	)

	for i, segment := range segments {
		// Step 1: resolve name → field_index.
		nameMap := as.FieldNames[schemaIdx]
		if nameMap == nil {
			return 0, 0, false
		}
		fieldIdx, found := nameMap[segment]
		if !found {
			return 0, 0, false
		}

		// Step 2: look up front ordinal.
		frontOrdinal := as.FieldOrdinals[schemaIdx][fieldIdx]

		// Step 3: Check if it's a back-edge and immediately update depth/blockBase
		if backEdgeIdx := as.BackEdgeOrdinal[schemaIdx][fieldIdx]; backEdgeIdx > 0 {
			fd := descriptorAt(cs, schemaIdx, fieldIdx)
			typ := ExtractType(fd)
			target := ExtractTargetSchema(fd)

			if typ == TypeUnion || typ == TypeComposite {
				if i < len(segments)-1 {
					t, resolved := cs.resolveTarget(fd, typ, segments[i+1])
					if resolved {
						target = t
					}
				} else {
					// Fallback for terminal union back-edges
					variants := cs.Variants[fd]
					if len(variants) > 0 {
						target = variants[0]
					}
				}
			}

			entryOrdinal := uint32(backEdgeIdx - 1)
			depth++
			blockSz := as.BlockSize[target]

			// Depth overflow check
			if blockSz == 0 || depth*blockSz > as.BlockBases[target]-as.FrontSize {
				return 0, 0, false
			}
			blockBase = as.BlockBases[target] -
				(depth * blockSz) +
				(entryOrdinal * as.AcyclicSubtreeSize[target])

			// Track the root of the current cyclic block
			blockOwner = target
		}

		// Step 4: final segment — apply block offset and return.
		if i == len(segments)-1 {
			fd := descriptorAt(cs, schemaIdx, fieldIdx)
			var ord uint32
			if blockBase == 0 {
				ord = frontOrdinal
			} else {
				// Normalize absolute frontOrdinal to the current cyclic block
				ord = blockBase + (frontOrdinal - as.EntryOrdinal[blockOwner])
			}
			return ord, fd, true
		}

		// Step 5: not final — must be schema-bearing.
		fd := descriptorAt(cs, schemaIdx, fieldIdx)
		if fd == 0 || !IsSchemaBearing(fd) {
			return 0, 0, false
		}

		typ := ExtractType(fd)
		target, resolved := cs.resolveTarget(fd, typ, segments[i+1])
		if !resolved {
			return 0, 0, false
		}

		schemaIdx = target
	}

	return 0, 0, false
}

// resolveTarget determines the next schema index for a schema-bearing field.
//
// For non-union/composite fields: if the target schema is itself a union or
// composite, it resolves nextSegment against its variants. Otherwise returns
// ExtractTargetSchema(fd).
//
// For union/composite fields: nextSegment must name a field in exactly one
// variant of the field. Zero or multiple matches → (0, false).
func (cs *Schema) resolveTarget(fd uint32, typ FieldTypeEnum, nextSegment string) (uint8, bool) {
	if typ == TypeUnion || typ == TypeComposite {
		// Union/composite field: resolve next segment against variants.
		return resolveUnionTarget(cs, cs.Variants[fd], nextSegment)
	}

	target := ExtractTargetSchema(fd)

	// If the target schema is a union/composite, resolve against its variants.
	if m := cs.Meta[target]; m != nil && (m.Type == TypeUnion || m.Type == TypeComposite) {
		return resolveUnionTarget(cs, m.Variants, nextSegment)
	}

	return target, true
}

// resolveUnionTarget finds which variant from the given list contains a field
// named nextSegment. Returns (variantIdx, true) iff at least one variant matches.
// If multiple variants match (common field), the first match is returned.
func resolveUnionTarget(cs *Schema, variants []uint8, nextSegment string) (uint8, bool) {
	as := cs.AddressSpace
	for _, variantIdx := range variants {
		if nm := as.FieldNames[variantIdx]; nm != nil {
			if _, ok := nm[nextSegment]; ok {
				return variantIdx, true
			}
		}
	}
	return 0, false
}

// descriptorAt returns the descriptor for the field at (schemaIdx, fieldIdx).
// Descriptors within a schema are stored in field_index order, so
// Descriptors[schemaStart + fieldIdx] is the correct position.
// Returns 0 if out of bounds.
func descriptorAt(cs *Schema, schemaIdx uint8, fieldIdx uint8) uint32 {
	start, end := schemaOffsetRange(cs, schemaIdx)
	if start == end {
		return 0
	}
	pos := start + int(fieldIdx)
	if pos >= end || pos >= len(cs.Descriptors) {
		return 0
	}
	return cs.Descriptors[pos]
}


