package ir

import (
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/document"
)

// =============================================================================
// FIELD DESCRIPTOR
// =============================================================================
//
// FieldDescriptor is a packed uint32 describing a single field's structural
// properties within its owning SchemaSlot.
//
//	bits 31-28  DataType   (4 bits)  — wire type of the field value
//	bits 27-26  Kind       (2 bits)  — traversal class (Simple/Object/Array/Complex)
//	bits 25-19  schemaIdx  (7 bits)  — index of the owning SchemaSlot in CompiledSchema.schemas
//	bits 18-12  fieldIdx   (7 bits)  — position of this field within its owning SchemaSlot.Fields
//	bits 11     required   (1 bit)
//	bits 10     hasDefault (1 bit)
//	bits  9     deprecated (1 bit)
//	bits  8     unique     (1 bit)
//	bits  7     terminal   (1 bit)   — back-edge of a recursive cycle; DAG traversal
//	                                   stops here. Terminal fields have no traversable
//	                                   sub-paths. All FieldKindSimple fields are implicitly
//	                                   terminal. Non-simple back-edge fields must be
//	                                   marked terminal explicitly at link time.
//	bits  6-0   targetIdx  (7 bits)  — meaning depends on Kind:
//	                                     FieldKindSimple  — unused
//	                                     FieldKindObject  — schemaIdx of the child SchemaSlot
//	                                     FieldKindArray   — schemaIdx of the element SchemaSlot
//	                                     FieldKindComplex — index into owning SchemaSlot.Complex
type FieldDescriptor uint32

// FieldKind drives traversal logic independently of DataType.
//
//	FieldKindSimple  — leaf value; no child schema; targetIdx unused
//	FieldKindObject  — one child schema (object, record); targetIdx = child schemaIdx
//	FieldKindArray   — one element schema (array, set); targetIdx = element schemaIdx
//	FieldKindComplex — multiple schemas (union, composite); targetIdx = index into SchemaSlot.Complex
type FieldKind uint8

const (
	FieldKindSimple  FieldKind = iota // 0
	FieldKindObject                   // 1
	FieldKindArray                    // 2
	FieldKindComplex                  // 3
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
	fdTargetIdxMask = uint32(0x7F) // bits 6-0
)

func (f FieldDescriptor) DataType() document.DataType { return document.DataType((f & FieldDescriptor(fdTypeMask)) >> 28) }
func (f FieldDescriptor) Kind() FieldKind             { return FieldKind((f & FieldDescriptor(fdKindMask)) >> 26) }
func (f FieldDescriptor) SchemaIdx() uint8            { return uint8((f & FieldDescriptor(fdSchemaIdxMask)) >> 19) }
func (f FieldDescriptor) FieldIdx() uint8             { return uint8((f & FieldDescriptor(fdFieldIdxMask)) >> 12) }
func (f FieldDescriptor) Required() bool              { return f&FieldDescriptor(fdRequired) != 0 }
func (f FieldDescriptor) HasDefault() bool            { return f&FieldDescriptor(fdHasDefault) != 0 }
func (f FieldDescriptor) Deprecated() bool            { return f&FieldDescriptor(fdDeprecated) != 0 }
func (f FieldDescriptor) Unique() bool                { return f&FieldDescriptor(fdUnique) != 0 }

// Terminal reports whether DAG traversal stops at this field.
// All FieldKindSimple fields are implicitly terminal. Non-simple fields that form
// the back-edge of a recursive schema cycle must be explicitly marked terminal
// at link time. Terminal fields have no traversable sub-paths.
func (f FieldDescriptor) Terminal() bool { return f&FieldDescriptor(fdTerminal) != 0 }

// TargetIdx returns the targetIdx bits (6-0), whose meaning depends on Kind:
//
//	FieldKindObject  — schemaIdx of the child SchemaSlot
//	FieldKindArray   — schemaIdx of the element SchemaSlot
//	FieldKindComplex — index into the owning SchemaSlot.Complex slice
//	FieldKindSimple  — undefined; callers must not read targetIdx for simple fields
func (f FieldDescriptor) TargetIdx() uint8 { return uint8(f & FieldDescriptor(fdTargetIdxMask)) }

// IsLeaf reports whether DAG traversal stops at this field: either it is
// FieldKindSimple (no child schema by definition) or it has been explicitly marked
// Terminal (back-edge of a recursive cycle).
func (f FieldDescriptor) IsLeaf() bool {
	return f.Kind() == FieldKindSimple || f.Terminal()
}

// =============================================================================
// COMPLEX FIELD — UNION AND COMPOSITE
// =============================================================================

// ComplexKind distinguishes the two structural kinds of a complex field.
//
//	ComplexUnion     — exactly one variant is active at runtime (discriminated union)
//	ComplexComposite — all constituents are merged; all fields present simultaneously
type ComplexKind uint8

const (
	ComplexUnion     ComplexKind = iota // pick-one: one variant active at runtime
	ComplexComposite                    // merge-all: all constituent fields always present
)

// CompiledComplex describes the structure of a FieldKindComplex field.
//
// For ComplexUnion, Variants holds the schemaIdx of each possible variant.
// Exactly one variant is active per document value; the active variant is
// determined at runtime by the validator/encoder inspecting the document.
//
// For ComplexComposite, Variants holds the schemaIdx of each constituent.
// All constituents are merged — their fields are all simultaneously present.
// Paths through a composite field are valid at link time because every
// constituent's fields are always present regardless of runtime state.
//
// Variants is a sub-slice of CompiledSchema.variants — the shared backing
// array allocated exactly once at link time. No independent heap allocation.
type CompiledComplex struct {
	Kind     ComplexKind
	Variants []uint8 // sub-slice of CompiledSchema.variants
}

// =============================================================================
// SCHEMA SLOT
// =============================================================================

// SchemaSlot describes one named schema within the compiled IR.
//
// Fields is a sub-slice of CompiledSchema.fields — the shared backing array
// of all FieldDescriptors across all slots, allocated exactly once at link
// time. Descriptors within a slot are ordered by UUID at link time for
// deterministic traversal.
//
// Complex is a sub-slice of CompiledSchema.complex — the shared backing array
// of all CompiledComplex entries across all slots, allocated exactly once at
// link time. A FieldKindComplex FieldDescriptor's TargetIdx() is an index into
// this slot's window of that array.
//
// SchemaSlot is immutable after link time.
type SchemaSlot struct {
	Idx     uint8             // schemaIdx of this slot within CompiledSchema.schemas
	Fields  []FieldDescriptor // sub-slice of CompiledSchema.fields
	Complex []CompiledComplex // sub-slice of CompiledSchema.complex
}

// =============================================================================
// RESOLVED PATH
// =============================================================================

// ResolvedPath is a sequence of FieldDescriptors representing a fully linked
// field path through the schema graph, produced at link time from a string path.
//
// Each descriptor in the path is the field being traversed at that depth.
// The first descriptor is always a field in the root SchemaSlot (Idx == 0).
// Intermediate descriptors are fields in child schemas followed by the
// preceding descriptor's TargetIdx().
//
// Traversal rules:
//
//   - FieldKindObject/FieldKindArray intermediate steps: follow TargetIdx() as schemaIdx
//     into CompiledSchema.schemas to reach the next SchemaSlot.
//
//   - FieldKindComplex intermediate steps through a composite: the composite's
//     constituents (via SchemaSlot.Complex[fd.TargetIdx()].Variants) are
//     searched in order for a slot that contains the next step's field.
//     This is valid at link time because all constituent fields are always present.
//
//   - FieldKindComplex intermediate steps through a union: not permitted at link time.
//     Which variant is active is a runtime property. Union fields may only
//     appear as the terminal step of a path.
//
//   - Terminal() fields must be the final step; no sub-paths exist beyond them.
//
// ResolvedPath replaces the former []ResolvedStep design. Each step is a full
// FieldDescriptor rather than a (schemaIdx, fieldIdx) pair — the descriptor
// already carries all information needed for traversal and validation.
type ResolvedPath []FieldDescriptor

// =============================================================================
// COMPILED SCHEMA — STRUCTURAL CORE
// =============================================================================

// CompiledSchema is the hot-path IR for a single schema version.
//
// Three contiguous backing arrays are allocated exactly once at link time,
// sized to the precise counts determined during the compilation pass:
//
//	fields   — all FieldDescriptors across all slots, contiguous in memory.
//	           Each SchemaSlot.Fields is a sub-slice of this array.
//
//	complex  — all CompiledComplex entries across all slots, contiguous in memory.
//	           Each SchemaSlot.Complex is a sub-slice of this array.
//
//	variants — all variant schemaIdx values across all CompiledComplex entries,
//	           contiguous in memory. Each CompiledComplex.Variants is a sub-slice
//	           of this array.
//
// Because CompiledSchema is immutable after link time, these arrays are never
// grown or reallocated. The allocator touches each array exactly once. DAG
// traversal reads contiguous memory at every level — no pointer chasing between
// independently allocated slot or complex structs.
//
// schemas holds the SlotSchema headers. Each slot's Fields and Complex slices
// are sub-slices of fields and complex respectively — the slice headers (pointer,
// len, cap) are the only per-slot overhead beyond the backing arrays themselves.
//
// values holds enum members and field defaults, keyed by (schemaIdx, fieldIdx).
//
// CompiledSchema is immutable after link time and safe for concurrent use.
type CompiledSchema struct {
	schemas  []SchemaSlot
	fields   []FieldDescriptor       // backing array for all SchemaSlot.Fields sub-slices
	complex  []CompiledComplex       // backing array for all SchemaSlot.Complex sub-slices
	variants []uint8                 // backing array for all CompiledComplex.Variants sub-slices
	values   *document.DataContainer // enum members and field defaults
}

// Slot returns the SchemaSlot for the given schemaIdx.
// O(1) direct index — no map lookup.
func (c *CompiledSchema) Slot(schemaIdx uint8) *SchemaSlot {
	return &c.schemas[schemaIdx]
}

// Root returns the root SchemaSlot (slot 0).
// DAG traversal always begins here.
func (c *CompiledSchema) Root() *SchemaSlot {
	return &c.schemas[0]
}

// Child returns the SchemaSlot that a FieldKindObject or FieldKindArray field points to.
// Callers must only invoke this for fields where Kind() is FieldKindObject or FieldKindArray.
func (c *CompiledSchema) Child(fd FieldDescriptor) *SchemaSlot {
	return &c.schemas[fd.TargetIdx()]
}

// ComplexOf returns the CompiledComplex for a FieldKindComplex field.
// Callers must only invoke this for fields where Kind() is FieldKindComplex.
// The returned pointer is into the shared complex backing array.
func (c *CompiledSchema) ComplexOf(slot *SchemaSlot, fd FieldDescriptor) *CompiledComplex {
	return &slot.Complex[fd.TargetIdx()]
}

// Variant returns the SchemaSlot for a specific variant or constituent of a
// CompiledComplex, identified by its position in the Variants slice.
func (c *CompiledSchema) Variant(cx *CompiledComplex, i int) *SchemaSlot {
	return &c.schemas[cx.Variants[i]]
}

// =============================================================================
// SCHEMA BUILDER — LINK-TIME CONSTRUCTION ONLY
// =============================================================================

// SchemaBuilder accumulates counts during a first compilation pass, allocates
// the backing arrays at their exact sizes, then populates them in a second pass.
// The zero value is ready to use. Call Build() once to produce an immutable
// CompiledSchema. SchemaBuilder must not be used after Build() is called.
//
// Usage:
//
//	var b SchemaBuilder
//	// first pass: b.AddField(), b.AddComplex(), b.AddVariant() to count
//	b.Alloc()
//	// second pass: b.SetSlot() to assign sub-slices to each SchemaSlot
//	schema := b.Build(nSchemas, values)
type SchemaBuilder struct {
	fields   []FieldDescriptor
	complex  []CompiledComplex
	variants []uint8
}

// Alloc allocates the three backing arrays at their exact sizes based on counts
// accumulated during the first compilation pass. Must be called exactly once,
// after all Add* calls and before any Set* calls.
func (b *SchemaBuilder) Alloc(nFields, nComplex, nVariants int) {
	b.fields = make([]FieldDescriptor, 0, nFields)
	b.complex = make([]CompiledComplex, 0, nComplex)
	b.variants = make([]uint8, 0, nVariants)
}

// AppendFields appends a slot's field descriptors into the backing array and
// returns the sub-slice to assign to SchemaSlot.Fields.
func (b *SchemaBuilder) AppendFields(fds []FieldDescriptor) []FieldDescriptor {
	start := len(b.fields)
	b.fields = append(b.fields, fds...)
	return b.fields[start:]
}

// AppendComplex appends a slot's CompiledComplex entries into the backing array
// and returns the sub-slice to assign to SchemaSlot.Complex.
// Each CompiledComplex must have its Variants already set via AppendVariants.
func (b *SchemaBuilder) AppendComplex(entries []CompiledComplex) []CompiledComplex {
	start := len(b.complex)
	b.complex = append(b.complex, entries...)
	return b.complex[start:]
}

// AppendVariants appends variant schemaIdx values into the backing array and
// returns the sub-slice to assign to CompiledComplex.Variants.
func (b *SchemaBuilder) AppendVariants(vs []uint8) []uint8 {
	start := len(b.variants)
	b.variants = append(b.variants, vs...)
	return b.variants[start:]
}

// Build produces the immutable CompiledSchema from the accumulated backing
// arrays and the provided slot headers. nSchemas is the total number of slots;
// schemas must have been populated with sub-slices from the Append* methods.
// Build must be called exactly once; the builder must not be used afterward.
func (b *SchemaBuilder) Build(schemas []SchemaSlot, values *document.DataContainer) *CompiledSchema {
	return &CompiledSchema{
		schemas:  schemas,
		fields:   b.fields,
		complex:  b.complex,
		variants: b.variants,
		values:   values,
	}
}

// =============================================================================
// COMPILED CONSTRAINT AND INDEX — VALIDATION AND STORAGE LAYERS
// =============================================================================

// PredicateName represents the name of a supported predicate.
type PredicateName string

// CompiledConstraint is the linked, recursive form of a constraint.
//
// A leaf node (IsGroup == false) holds a single predicate rule. Predicate
// is a function name resolved to a callable at validator construction time.
// Fields are fully resolved ResolvedPaths — no string operations on the hot path.
//
// A group node (IsGroup == true) combines child constraints with a logical
// operator. Children are evaluated according to Op at validation time.
type CompiledConstraint struct {
	IsGroup bool

	// Leaf fields — meaningful when IsGroup == false.
	Predicate  string
	Fields     []ResolvedPath
	Parameters any

	// Group fields — meaningful when IsGroup == true.
	Op       common.LogicalOperator
	Children []CompiledConstraint
}

// CompiledIndexCondition is the linked, recursive form of a partial index
// condition.
//
// A leaf node (IsGroup == false) holds a single field comparison.
// Field is the FieldDescriptor of the field being compared — it carries
// schemaIdx, fieldIdx, Kind, and TargetIdx, sufficient to locate the field
// value in any document without further lookup.
//
// A group node (IsGroup == true) combines child conditions with a logical
// operator.
type CompiledIndexCondition struct {
	IsGroup bool

	// Leaf fields — meaningful when IsGroup == false.
	Field    FieldDescriptor
	Operator common.ComparisonOperator
	Value    LiteralValue

	// Group fields — meaningful when IsGroup == true.
	Op       common.LogicalOperator
	Children []CompiledIndexCondition
}

// CompiledIndex is the linked form of an index definition.
// Fields are fully resolved ResolvedPaths. Order is a typed value rather than
// a raw string so consumers can act on it without parsing.
type CompiledIndex struct {
	Type      IndexType
	Order     IndexOrder
	Unique    bool
	Fields    []ResolvedPath
	Condition *CompiledIndexCondition
}

// IndexOrder is the typed sort direction for an index.
type IndexOrder uint8

const (
	IndexOrderAsc  IndexOrder = iota
	IndexOrderDesc
)

// =============================================================================
// METADATA LAYER — COLD PATH ONLY
// =============================================================================

// FieldMeta is cold metadata for a single field.
// Indexed by (schemaIdx, fieldIdx) — parallel to FieldDescriptor's own identity bits.
// Never touched on the hot path.
type FieldMeta struct {
	ID          [16]byte // UUID-v7 as raw bytes (map key from the JSON definition)
	Name        string
	Description string
}

// SchemaMeta is cold metadata for a SchemaSlot.
// Indexed by schemaIdx — parallel to SchemaSlot.Idx.
type SchemaMeta struct {
	ID          [16]byte
	Name        string
	Description string
	Version     string
	Concrete    bool
	Metadata    map[string]any // arbitrary key-value pairs from the schema definition
}

// ConstraintMeta is cold metadata for a compiled constraint.
// Indexed by position in CompiledEntry.Constraints.
type ConstraintMeta struct {
	ID          [16]byte
	Name        string
	Description string
}

// IndexMeta is cold metadata for a compiled index.
// Indexed by position in CompiledEntry.Indexes.
type IndexMeta struct {
	ID          [16]byte
	Name        string
	Description string
}

// CompiledMeta is the cold metadata layer, parallel to CompiledSchema.
// Loaded lazily — a schema is fully operational without it.
// Never referenced during validation, serialization, or deserialization.
//
// Fields is a two-dimensional slice indexed as Fields[schemaIdx][fieldIdx],
// matching the (schemaIdx, fieldIdx) identity carried by FieldDescriptor.
type CompiledMeta struct {
	Fields      [][]FieldMeta    // Fields[schemaIdx][fieldIdx]
	Schemas     []SchemaMeta     // indexed by schemaIdx, parallel to CompiledSchema.schemas
	Constraints []ConstraintMeta // indexed parallel to CompiledEntry.Constraints
	Indexes     []IndexMeta      // indexed parallel to CompiledEntry.Indexes
}

// =============================================================================
// COMPILED ENTRY AND REGISTRY
// =============================================================================

// CompiledEntry is a fully linked schema version combining all layers.
// Core is always loaded. Meta is loaded lazily on first cold-path access.
//
// Constraints and Indexes are root-level. NestedConstraints and NestedIndexes
// carry constraints and indexes defined directly on nested SchemaSlots,
// keyed by schemaIdx. Slot 0 (root) is never present in the nested maps —
// root-level constraints live in Constraints.
type CompiledEntry struct {
	Core              *CompiledSchema
	Constraints       []CompiledConstraint
	Indexes           []CompiledIndex
	NestedConstraints map[uint8][]CompiledConstraint // keyed by schemaIdx; nil if none
	NestedIndexes     map[uint8][]CompiledIndex      // keyed by schemaIdx; nil if none
	Meta              *CompiledMeta                  // nil until first cold-path access
}

// =============================================================================
// REGISTRY
// =============================================================================

// Registry holds all compiled schema versions, keyed by UUID-v7.
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
