package definition

import (
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/document"
)

// =============================================================================
// DATAPOINT ADDRESS SCHEME
// =============================================================================
//
// Every field reachable via any valid path in the schema graph is assigned a
// unique 27-bit integer address. This address is the foundation of the
// DataPoint addressing scheme used by DataContainer for storage and retrieval.
//
// THE ADDRESS SPACE (27 bits = 134,217,728 slots)
//
//   [0, 2^14)          — length-1 paths  (single field, no traversal)
//   [2^14, 2^27)       — length>1 paths  (field reached through a parent)
//
// The length-1 region is populated by GlobalIdx: fields[] is sorted by UUID
// at link time, so a field's position in that slice is its address. Maximum
// population is 128 schemas × 128 fields = 2^14 slots.
//
// The length>1 region uses recursive block subdivision, described below.
//
// RECURSIVE BLOCK SUBDIVISION
//
// The multi-step region (size ≈ 2^27) is treated as a single root block. Each
// non-terminal field at the root level owns an equal sub-block carved from
// that root block. Each sub-block is recursively subdivided in the same way
// for the next level, and so on.
//
// Within each block at any level, the layout is:
//
//   [ terminal slots | non-terminal sub-blocks ... ]
//
//   T = number of terminal (KindSimple or Terminal-flagged) fields in the
//       schema slot at this level. Each gets one slot, ordinal by fields[]
//       position.
//
//   N = number of non-terminal fields. They share the remainder of the block
//       equally, with each sub-block size rounded down to the nearest power
//       of two for alignment.
//
//       subBlockSize = floorPow2((blockSize - T) / N)
//
// The address of a path is computed by walking this subdivision level by
// level, skipping the slots/sub-blocks of all siblings that precede the
// chosen field at each level, then landing at the terminal slot or the base
// of the next sub-block.
//
// PROPERTIES
//
//   Deterministic   — fields[] is sorted at link time; sibling order is fixed.
//                     Given the same CompiledSchema, Address() always returns
//                     the same integer for the same path.
//
//   Collision-free  — terminal slots are individually assigned; non-terminal
//                     sub-blocks are non-overlapping by construction (packed
//                     sequentially after the terminal region with power-of-two
//                     alignment).
//
//   No pre-built table — CompiledSchema carries no path map. Address() is a
//                     pure function over the immutable fields[]/schemas[]/refs[]
//                     slices. The result may be cached by the caller.
//
//   Lazy / pay-per-use — cost is O(depth × fields-per-schema) on first call.
//                     Callers cache results; the hot path never recomputes.
//
//   Terminates      — Terminal() fields have subBlockSize 1 and no children;
//                     the loop is bounded by len(path). Recursive schemas must
//                     mark their back-edge fields Terminal() at link time,
//                     which is enforced by the link-phase cycle check.
//
// LINK-TIME VALIDATION
//
// The link phase must verify for every schema reachable at every depth that:
//
//   floorPow2((inheritedBlockSize - T) / N) >= 1
//
// If this condition fails, the address space is exhausted at that depth for
// that schema. The remedy is either to increase addrBits, flatten the schema,
// or mark some intermediate non-terminal fields Terminal() (treating their
// subtree as opaque to the address scheme).
//
// WORKED EXAMPLE (from the meta-schema, schema.json)
//
// The worst non-recursive path is:
//   Schema → schemas(NestedSchema) → fields(Field) → schema(union/FieldSchemaRef)
//          → constraints(Constraint/composite) → ConstraintUnion(union)
//          → ConstraintGroup → operator
//
// Block sizes at each level (actual T and N counts, not worst-case):
//   root      blockSize ≈ 2^27   T=4  N=4  → subBlockSize = 2^24
//   level 1   blockSize = 2^24   T=8  N=4  → subBlockSize = 2^21
//   level 2   blockSize = 2^21   T=6  N=1  → subBlockSize = 2^20
//   level 3   blockSize = 2^20   T=0  N=2  → subBlockSize = 2^18
//   level 4   blockSize = 2^18   T=1  N=2  → subBlockSize = 2^16
//   level 5   blockSize = 2^16   T=0  N=2  → subBlockSize = 2^14
//   level 6   blockSize = 2^14   T=0  N=2  → subBlockSize = 2^12
//   level 7   blockSize = 2^12   T=1  N=1  → subBlockSize = 2^11
//
// Still 2,048 slots at depth 8, with both recursive back-edges
// (ConstraintGroup.rules and IndexConditionGroup.conditions) already
// terminated. The 2^27 budget is not remotely threatened by this schema.
//
// RECURSIVE SCHEMAS
//
// Two cycles exist in schema.json:
//   ConstraintGroup.rules    → ConstraintUnion → ConstraintGroup  (back-edge: rules)
//   IndexConditionGroup.conditions → IndexConditionUnion → IndexConditionGroup
//                                                          (back-edge: conditions)
//
// Both back-edge fields must be marked Terminal() at link time. Their address
// is a single terminal slot within their parent block. Sub-paths through them
// are not individually addressable — their contents are validated and stored
// as an opaque unit by the DataContainer.

// =============================================================================
// CONSTANTS
// =============================================================================

const (
	// addrBits is the width of the DataPoint address space.
	addrBits = 27

	// maxGlobalIdx is the upper bound of the length-1 region.
	// 128 schemas × 128 fields = 2^14.
	maxGlobalIdx = uint32(1 << 14)

	// multiStepBase is where the length>1 region begins.
	multiStepBase = maxGlobalIdx

	// multiStepSize is the total number of slots available for length>1 paths.
	multiStepSize = uint32(1<<addrBits) - multiStepBase

	// maxPathDepth is the maximum number of steps in a resolved path.
	// Enforced at link time. Must match the pathCacheKey buffer capacity
	// (128 bytes / 2 bytes per step = 64 steps).
	maxPathDepth = 64
)

// =============================================================================
// FIELD DESCRIPTOR
// =============================================================================

// FieldDescriptor is a packed uint32 describing a single field's structural
// properties. The fields[] slice in CompiledSchema is sorted by UUID at link
// time — a field's position in that slice is its globalIdx, which is the
// foundation of the DataPoint addressing scheme.
//
//	bits 31-28  DataType   (4 bits)
//	bits 27-26  Kind       (2 bits)
//	bits 25-19  schemaIdx  (7 bits) — owning SchemaSlot index
//	bits 18-12  fieldIdx   (7 bits) — position within owning schema's fields
//	bits 11     required   (1 bit)
//	bits 10     hasDefault (1 bit)
//	bits  9     deprecated (1 bit)
//	bits  8     unique     (1 bit)
//	bits  7     terminal   (1 bit)  — does not participate in any cycle;
//	                                  back-edge fields in recursive schemas
//	                                  must be marked terminal at link time.
//	                                  Terminal fields own exactly one address
//	                                  slot and have no addressable sub-paths.
//	bits  6-0   spare      (7 bits)
type FieldDescriptor uint32

// FieldKind drives traversal logic independently of DataType.
//
//	simple  — leaf value, no child schema
//	object  — one child schema (object, record)
//	array   — one element schema (array, set, geometry treated as terminal)
//	complex — multiple schemas (union, composite)
type FieldKind uint8

const (
	KindSimple  FieldKind = iota
	KindObject
	KindComplex
)

const (
	fdTypeMask      = uint32(0xF) << 28
	fdKindMask      = uint32(0x3) << 26
	fdSchemaIdxMask = uint32(0x7F) << 19
	fdFieldIdxMask  = uint32(0x7F) << 12
	fdRequired      = uint32(1) << 11
	fdHasDefault    = uint32(1) << 10
	fdDeprecated    = uint32(1) << 9
	fdUnique        = uint32(1) << 8
	fdTerminal      = uint32(1) << 7

	// fdIdentityMask covers only the bits that uniquely identify a field:
	// schemaIdx and fieldIdx. Used for field comparison in Address() to avoid
	// false mismatches caused by differing flag bits (required, deprecated, etc.)
	// between a descriptor retrieved from fields[] and one constructed from a
	// ResolvedStep.
	fdIdentityMask = fdSchemaIdxMask | fdFieldIdxMask
)

func (f FieldDescriptor) DataType() document.DataType { return document.DataType((f & FieldDescriptor(fdTypeMask)) >> 28) }
func (f FieldDescriptor) Kind() FieldKind    { return FieldKind((f & FieldDescriptor(fdKindMask)) >> 26) }
func (f FieldDescriptor) SchemaIdx() uint8   { return uint8((f & FieldDescriptor(fdSchemaIdxMask)) >> 19) }
func (f FieldDescriptor) FieldIdx() uint8    { return uint8((f & FieldDescriptor(fdFieldIdxMask)) >> 12) }
func (f FieldDescriptor) Required() bool     { return f&FieldDescriptor(fdRequired) != 0 }
func (f FieldDescriptor) HasDefault() bool   { return f&FieldDescriptor(fdHasDefault) != 0 }
func (f FieldDescriptor) Deprecated() bool   { return f&FieldDescriptor(fdDeprecated) != 0 }
func (f FieldDescriptor) Unique() bool       { return f&FieldDescriptor(fdUnique) != 0 }

// Terminal reports whether this field is a leaf in the address scheme.
// Terminal fields own exactly one address slot and have no addressable
// sub-paths. All KindSimple fields are implicitly terminal. Non-simple fields
// that form the back-edge of a recursive schema cycle must be explicitly
// marked terminal at link time.
func (f FieldDescriptor) Terminal() bool { return f&FieldDescriptor(fdTerminal) != 0 }

// isLeaf reports whether this field is a leaf for the purposes of block
// subdivision: either it is KindSimple (no child schema by definition) or
// it has been explicitly marked Terminal() (back-edge of a recursive cycle).
func (f FieldDescriptor) isLeaf() bool {
	return f.Kind() == KindSimple || f.Terminal()
}

// =============================================================================
// SCHEMA SLOT
// =============================================================================

// SchemaSlot describes one nested schema's layout within the compiled arrays.
// 4 bytes. 128 slots = 512 bytes — fits in L1.
//
// fieldStart/fieldCount locate this schema's descriptors in fields[].
// refStart/refCount locate child schema indices in refs[].
//
//	KindObject/KindArray : exactly one ref (child schema index)
//	KindComplex          : refCount refs, one per union/composite variant
type SchemaSlot struct {
	fieldStart uint8
	fieldCount uint8
	refStart   uint8
	refCount   uint8
}

// =============================================================================
// RESOLVED PATH
// =============================================================================

// ResolvedStep is a (schemaIdx, fieldIdx) pair packed into a uint16,
// produced at link time from a string path segment.
//
//	bits 15-8  schemaIdx  — index of the schema slot that owns this field
//	bits  7-0  fieldIdx   — position of the field within that schema slot's fields
//
// String paths in constraints and indexes are fully resolved during the link
// phase. After linking, no string path operations occur on the hot path.
//
// Each step carries the schemaIdx of the schema that owns the field, not the
// child schema that the field points to. When Address() steps into a child
// schema, it uses c.Slot(path[depth].SchemaIdx()) — the schema index embedded
// in the next step — which the link phase must have resolved to the correct
// child schema index for the traversal to be valid.
type ResolvedStep uint16

func NewResolvedStep(schemaIdx, fieldIdx uint8) ResolvedStep {
	return ResolvedStep(uint16(schemaIdx)<<8 | uint16(fieldIdx))
}

func (r ResolvedStep) SchemaIdx() uint8 { return uint8(r >> 8) }
func (r ResolvedStep) FieldIdx() uint8  { return uint8(r & 0xFF) }

// ResolvedPath is a sequence of ResolvedSteps representing a fully linked
// field path. Stored in CompiledConstraint and CompiledIndex in place of
// string paths after the link phase.
//
// The first step always refers to a field in schema slot 0 (the root schema).
// Intermediate steps refer to fields in child schemas as followed by the
// preceding step's Kind. The final step is the addressed field.
//
// Paths through Terminal() fields must not have length > 1 beyond that field:
// Terminal fields have no addressable sub-paths.
//
// KindComplex fields must only appear as the final step of a path. The active
// variant of a union or composite is a runtime property, not a static path
// property; intermediate complex fields are not resolvable at link time.
//
// Path length must not exceed maxPathDepth (64 steps), enforced at link time.
type ResolvedPath []ResolvedStep

// =============================================================================
// COMPILED SCHEMA — STRUCTURAL CORE
// =============================================================================

// CompiledSchema is the hot-path IR for a single schema version.
//
// fields[] is sorted by UUID at link time. A field's position in this slice
// is its globalIdx — its address for length-1 paths.
//
// No maps, no strings, no heap pointers beyond slice headers and values.
//
// Memory budget for a maximally populated schema:
//
//	fields:  16384 × 4 bytes =  64 KB  (128 schemas × 128 fields)
//	schemas:   128 × 4 bytes = 512 bytes
//	refs:      256 × 1 byte  = 256 bytes
//	values:    *DataContainer — one allocation, immutable after link
type CompiledSchema struct {
	fields  []FieldDescriptor
	schemas []SchemaSlot
	refs    []uint8        // child schema indices; uint8 sufficient for 7-bit schemaIdx
	values  *document.DataContainer // enum members and field defaults, keyed by DataPoint
	// immutable after link time
}

// GlobalIdx returns the position of a field in the sorted fields[] slice,
// which is its address for length-1 paths.
//
// Returns (index, true) if found, (0, false) if the field does not exist in
// this schema. Callers must check the boolean — a missing field silently
// returning 0 would alias the first field's address.
//
// O(log N) binary search on a sorted []uint32 — branchless, cache-friendly.
func (c *CompiledSchema) GlobalIdx(schemaIdx, fieldIdx uint8) (uint16, bool) {
	target := FieldDescriptor(uint32(schemaIdx)<<19 | uint32(fieldIdx)<<12)
	mask := FieldDescriptor(fdIdentityMask)
	lo, hi := 0, len(c.fields)
	for lo < hi {
		mid := (lo + hi) >> 1
		if c.fields[mid]&mask < target&mask {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(c.fields) && c.fields[lo]&mask == target&mask {
		return uint16(lo), true
	}
	return 0, false
}

// Slot returns the SchemaSlot for the given schemaIdx.
func (c *CompiledSchema) Slot(schemaIdx uint8) *SchemaSlot {
	return &c.schemas[schemaIdx]
}

// Fields returns the FieldDescriptors for a given schema slot.
func (c *CompiledSchema) Fields(slot *SchemaSlot) []FieldDescriptor {
	return c.fields[slot.fieldStart : slot.fieldStart+slot.fieldCount]
}

// Refs returns the child schema indices for a given schema slot.
func (c *CompiledSchema) Refs(slot *SchemaSlot) []uint8 {
	return c.refs[slot.refStart : slot.refStart+slot.refCount]
}

// =============================================================================
// ADDRESS COMPUTATION
// =============================================================================

// Address computes the 27-bit DataPoint address for a resolved path.
//
// For length-1 paths the address is simply GlobalIdx — the field's position
// in the sorted fields[] slice.
//
// For length>1 paths the address is computed by recursive block subdivision
// of the multi-step region [multiStepBase, 2^27). See the package-level
// comment block for a full description of the scheme.
//
// Address is a pure function: same CompiledSchema + same path → same result,
// always. It carries no state and is safe to call concurrently. Callers that
// need O(1) repeated access should cache the result; see AddressCache.
//
// Preconditions (enforced at link time, not re-checked here for performance):
//   - path has length >= 1 and <= maxPathDepth
//   - every step in path refers to a valid (schemaIdx, fieldIdx) pair
//   - no step beyond a Terminal() field exists in the path
//   - KindComplex fields only appear as the final step (unions/composites
//     are not valid intermediate steps — which variant is active is a
//     runtime property, not a static path property)
//   - each step's schemaIdx is the child schema index resolved by the
//     link phase for the preceding step's field traversal
func Address(c *CompiledSchema, path ResolvedPath) uint32 {
	if len(path) == 1 {
		idx, _ := c.GlobalIdx(path[0].SchemaIdx(), path[0].FieldIdx())
		return uint32(idx)
	}

	blockBase := multiStepBase
	blockSize := multiStepSize

	// identityMask isolates schemaIdx and fieldIdx bits for field comparison.
	// Fields are compared by identity (schemaIdx + fieldIdx) rather than full
	// descriptor equality to avoid false mismatches caused by differing flag
	// bits (required, deprecated, etc.) between the descriptor stored in
	// fields[] and the one reconstructed from path[depth].
	const identityMask = FieldDescriptor(fdIdentityMask)

	for depth := 1; depth < len(path); depth++ {
		// Use the schemaIdx carried by the current step. Each step's schemaIdx
		// is the child schema index resolved by the link phase for the preceding
		// field traversal — not the owning schema of the parent field.
		parentSlot := c.Slot(path[depth-1].SchemaIdx())
		fields := c.Fields(parentSlot)

		// Build the identity descriptor for the chosen field from the path step,
		// using only schemaIdx and fieldIdx bits. This is compared against
		// descriptors in fields[] using identityMask to avoid flag-bit collisions.
		chosen := FieldDescriptor(uint32(path[depth].SchemaIdx())<<19 | uint32(path[depth].FieldIdx())<<12)

		// Count terminal (T) and non-terminal (N) fields in this schema slot.
		// This determines how the current block is partitioned.
		T, N := uint32(0), uint32(0)
		for _, fd := range fields {
			if fd.isLeaf() {
				T++
			} else {
				N++
			}
		}

		// Determine whether the chosen field is a leaf in this slot.
		var chosenIsLeaf bool
		for _, fd := range fields {
			if fd&identityMask == chosen&identityMask {
				chosenIsLeaf = fd.isLeaf()
				break
			}
		}

		if chosenIsLeaf {
			// Terminal field: its address is blockBase + its ordinal among
			// the terminal fields of this slot (packed before the sub-blocks).
			// Compare by identity mask only — flag bits must not affect ordering.
			termOrdinal := uint32(0)
			for _, fd := range fields {
				if fd&identityMask == chosen&identityMask {
					break
				}
				if fd.isLeaf() {
					termOrdinal++
				}
			}
			return blockBase + termOrdinal
		}

		// Non-terminal field: step into its sub-block.
		//
		// Sub-block size is the remainder of the block after terminal slots,
		// divided equally among N non-terminals, rounded down to a power of
		// two for alignment.
		subBlockSize := floorPow2((blockSize - T) / N)

		// Ordinal of the chosen field among the non-terminals in this slot.
		// Non-terminal sub-blocks are laid out contiguously after the T
		// terminal slots. Compare by identity mask only.
		ntOrdinal := uint32(0)
		for _, fd := range fields {
			if fd&identityMask == chosen&identityMask {
				break
			}
			if !fd.isLeaf() {
				ntOrdinal++
			}
		}

		// Advance into the chosen non-terminal's sub-block.
		blockBase = blockBase + T + ntOrdinal*subBlockSize
		blockSize = subBlockSize
	}

	// If the path ends on a non-terminal (addressing the container itself
	// rather than a leaf within it), the address is the base of its block.
	return blockBase
}

// floorPow2 returns the largest power of two <= n.
// Returns 1 for n == 0 to avoid zero-size sub-blocks; the link-time
// ValidateAddressSpace check will catch genuine exhaustion before it
// can produce a zero here.
func floorPow2(n uint32) uint32 {
	if n == 0 {
		return 1
	}
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return (n >> 1) + 1
}

// =============================================================================
// ADDRESS CACHE
// =============================================================================

// AddressCache is a caller-owned lazy cache of Address() results.
//
// Address() is pure and O(depth × fields-per-schema) on every call.
// For repeated access to the same paths — which is the common case once a
// workload has warmed up — callers should hold an AddressCache and call
// Lookup instead of Address directly.
//
// AddressCache is safe for concurrent use. It grows only as new paths are
// seen and never shrinks. In practice it converges quickly to the hot set
// of paths the workload actually touches.
type AddressCache struct {
	mu    sync.RWMutex
	cache map[string]uint32
}

func NewAddressCache() *AddressCache {
	return &AddressCache{cache: make(map[string]uint32)}
}

// Lookup returns the address for path, computing and caching it on first call.
func (ac *AddressCache) Lookup(c *CompiledSchema, path ResolvedPath) uint32 {
	key := pathCacheKey(path)

	ac.mu.RLock()
	if addr, ok := ac.cache[key]; ok {
		ac.mu.RUnlock()
		return addr
	}
	ac.mu.RUnlock()

	addr := Address(c, path)

	ac.mu.Lock()
	ac.cache[key] = addr
	ac.mu.Unlock()

	return addr
}

// pathCacheKey encodes a ResolvedPath as a string key without heap allocation
// for paths up to maxPathDepth steps (2 bytes per step, packed into a
// [maxPathDepth*2]byte). Path length is bounded to maxPathDepth at link time;
// paths exceeding this limit must be rejected before reaching this function.
func pathCacheKey(path ResolvedPath) string {
	var buf [maxPathDepth * 2]byte
	for i, step := range path {
		buf[i*2] = step.SchemaIdx()
		buf[i*2+1] = step.FieldIdx()
	}
	return string(buf[:len(path)*2])
}

// =============================================================================
// LINK-TIME ADDRESS SPACE VALIDATION
// =============================================================================

// ValidateAddressSpace walks every schema reachable from the root slot and
// verifies that no block is subdivided to zero size. Must be called once
// during the link phase, after fields[] is sorted and Terminal() bits are set.
//
// Returns an error describing the first path that would exhaust the address
// space, or nil if the schema is within budget.
func (c *CompiledSchema) ValidateAddressSpace() error {
	return c.validateBlock(0, multiStepSize, nil)
}

// validateBlock recursively checks that block subdivision remains viable at
// each level of the schema graph.
//
// blockSize is the number of address slots available at this level.
// pathSoFar accumulates the schemaIdx sequence for error reporting.
//
// Note: blockBase is intentionally absent from this signature. The validation
// only requires knowing that sub-blocks remain non-zero in size; the absolute
// base address is not needed for the viability check.
func (c *CompiledSchema) validateBlock(schemaIdx uint8, blockSize uint32, pathSoFar []uint8) error {
	slot := c.Slot(schemaIdx)
	fields := c.Fields(slot)

	T, N := uint32(0), uint32(0)
	for _, fd := range fields {
		if fd.isLeaf() {
			T++
		} else {
			N++
		}
	}

	if N == 0 {
		return nil // all terminals, no sub-blocks needed
	}

	if blockSize <= T {
		return &AddressExhaustedError{
			Path:      pathSoFar,
			BlockSize: blockSize,
			T:         T,
			N:         N,
		}
	}

	subBlockSize := floorPow2((blockSize - T) / N)
	if subBlockSize == 0 {
		return &AddressExhaustedError{
			Path:      pathSoFar,
			BlockSize: blockSize,
			T:         T,
			N:         N,
		}
	}

	for _, fd := range fields {
		if fd.isLeaf() {
			continue
		}
		childSchemaIdx := fd.SchemaIdx()
		// Append to a copy of pathSoFar to avoid aliasing across sibling
		// iterations — append may reuse the underlying array if cap allows.
		childPath := make([]uint8, len(pathSoFar)+1)
		copy(childPath, pathSoFar)
		childPath[len(pathSoFar)] = childSchemaIdx
		if err := c.validateBlock(childSchemaIdx, subBlockSize, childPath); err != nil {
			return err
		}
	}

	return nil
}

// AddressExhaustedError is returned by ValidateAddressSpace when a schema
// path exhausts the block subdivision budget.
type AddressExhaustedError struct {
	// Path is the schemaIdx sequence leading to the exhausted block.
	// Use CompiledMeta.Schemas to resolve these indices to human-readable names.
	Path      []uint8
	BlockSize uint32
	T         uint32 // terminal field count at this level
	N         uint32 // non-terminal field count at this level
}

func (e *AddressExhaustedError) Error() string {
	return "address space exhausted: block subdivision collapsed to zero at schema path"
}

// =============================================================================
// COMPILED CONSTRAINT AND INDEX — VALIDATION AND STORAGE LAYERS
// =============================================================================

// CompiledConstraint is the linked form of a constraint rule.
// Fields are resolved paths — no string path operations at runtime.
// Predicate is the only string — resolved to a function pointer at
// validator construction time, never on the hot path.
type CompiledConstraint struct {
	Predicate  string
	Fields     []ResolvedPath
	Parameters any
}

// CompiledIndexCondition is the linked form of a partial index condition.
// Field is resolved at link time. Value is retrieved from
// CompiledSchema.values at evaluation time using the field's DataPoint.
type CompiledIndexCondition struct {
	Field    ResolvedStep
	Operator common.ComparisonOperator
}

// CompiledIndex is the linked form of an index definition.
type CompiledIndex struct {
	Type      IndexType
	Unique    bool
	Fields    []ResolvedPath
	Condition *CompiledIndexCondition
}

// =============================================================================
// METADATA LAYER — COLD PATH ONLY
// =============================================================================

// FieldMeta is cold metadata for a single field.
// Indexed by globalIdx from fields[]. Never touched on the hot path.
// Name is used only during the link phase for path resolution — after
// linking it is cold.
type FieldMeta struct {
	ID          [16]byte // UUID-v7 as raw bytes
	Name        string
	Description string
}

// SchemaMeta is cold metadata for a nested schema.
// Indexed by schemaIdx from schemas[].
type SchemaMeta struct {
	ID          [16]byte
	Name        string
	Description string
	Version     string
	Concrete    bool
}

// CompiledMeta is the cold metadata layer, parallel to CompiledSchema.
// Loaded lazily — a schema is fully operational without it.
// Never referenced during validation, serialization, or deserialization.
type CompiledMeta struct {
	Fields  []FieldMeta  // indexed by globalIdx
	Schemas []SchemaMeta // indexed by schemaIdx
}

// =============================================================================
// COMPILED ENTRY AND REGISTRY
// =============================================================================

// CompiledEntry is a fully linked schema version combining all layers.
// Core is always loaded. Meta is loaded lazily on first cold-path access.
type CompiledEntry struct {
	Core        *CompiledSchema
	Constraints []CompiledConstraint
	Indexes     []CompiledIndex
	Meta        *CompiledMeta // nil until first cold-path access
}

// Registry holds all compiled schema versions.
// The map is touched only for initial UUID resolution — never on the
// field traversal hot path. All hot-path consumers hold a *CompiledEntry
// directly after the first lookup.
type Registry struct {
	mu      sync.RWMutex
	schemas map[[16]byte]*CompiledEntry
}

func NewRegistry() *Registry {
	return &Registry{
		schemas: make(map[[16]byte]*CompiledEntry),
	}
}

func (r *Registry) Get(id [16]byte) (*CompiledEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.schemas[id]
	return e, ok
}

func (r *Registry) Register(id [16]byte, entry *CompiledEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schemas[id] = entry
}
