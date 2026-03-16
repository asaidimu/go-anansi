package ir

// address_space.go defines CompiledAddressSpace, the compile-time address
// table that backs Address() resolution. It is produced alongside
// CompiledSchema and stored on it. Consumers that call Address() receive
// both pointers and pass them together.
//
// All fields are fixed-size arrays bounded by the IR hard limits (128 schemas,
// 127 fields per schema). No heap allocation occurs during resolution.

const (
	// addressSpaceBits is the width of the ordinal space.
	addressSpaceBits = 27

	// addressSpaceMax is 2^27 − 1, the highest valid ordinal.
	// Ordinal 0 is the sentinel for invalid/unresolvable paths.
	addressSpaceMax = uint32((1 << addressSpaceBits) - 1)
)

// CompiledAddressSpace holds all tables required to resolve any dot-separated
// field path to a unique 27-bit ordinal in O(path length) time with zero
// allocation.
//
// The address space is partitioned into two disjoint regions:
//
//   - Front region (1 .. FrontSize): ordinals for every node in the acyclic
//     projection of the schema graph, assigned by pre-order DFS.
//
//   - Back region (FrontSize+1 .. 2^27−1): one block per unique cyclic target
//     schema, allocated from the top of the space downward.
//
// Address() uses BlockBases[target] != 0 as the sole test for whether a
// schema-bearing field crosses into a cyclic target. All BlockBases entries
// for non-cyclic schemas are 0 and must remain 0.
type CompiledAddressSpace struct {
	// FieldOrdinals[schemaIdx][fieldIdx] is the ordinal of that field in the
	// acyclic pre-order DFS. The same value is reused inside blocks — the
	// block-relative position of a field is identical to its front ordinal.
	FieldOrdinals [128][127]uint32

	// FieldNames[schemaIdx][fieldName] → fieldIdx.
	// Populated from SchemaMetadata.Fields at compile time.
	// Required for name-based segment resolution in Address().
	FieldNames [128]map[string]uint8

	// BackEdgeOrdinal[schemaIdx][fieldIdx] is the position of this back-edge
	// field among all back-edge fields targeting the same schema, in field UUID
	// lex order. Zero for non-back-edge fields (also zero for the first
	// back-edge field, so callers must check IsSchemaBearing+BlockBases instead
	// of this value alone to detect back-edges).
	BackEdgeOrdinal [128][127]uint8

	// BlockBases[schemaIdx] is the base address of the back block for this
	// cyclic target schema. Zero means the schema is not a cyclic target.
	BlockBases [128]uint32

	// BlockSize[schemaIdx] = AcyclicSubtreeSize[schemaIdx] × BackEdgeCount.
	// Zero for non-cyclic-target schemas.
	BlockSize [128]uint32

	// AcyclicSubtreeSize[schemaIdx] is the number of path nodes in the acyclic
	// projection of this schema, including the root node of that subtree.
	AcyclicSubtreeSize [128]uint32

	// FrontSize is the total number of ordinals assigned in the front region.
	// Valid front ordinals are 1..FrontSize.
	FrontSize uint32
}
