// Package ir defines the compiled schema intermediate representation.
// The IR is immutable once constructed. All layout is deterministic given
// the same source. No consumer may mutate an IR after compilation.
package ir

import "github.com/asaidimu/go-anansi/v6/core/document"

// ── Field descriptor bit masks ────────────────────────────────────────────────

const (
	FDMaskType         uint32 = 0x0000001E // bits 4–1
	FDMaskRequired     uint32 = 0x00000020 // bit  5
	FDMaskUnique       uint32 = 0x00000040 // bit  6
	FDMaskDeprecated   uint32 = 0x00000080 // bit  7
	FDMaskFieldIndex   uint32 = 0x00007F00 // bits 14–8
	FDMaskOwnerSchema  uint32 = 0x007F8000 // bits 22–15
	FDMaskTargetSchema uint32 = 0x7F800000 // bits 30–23
	FDMaskTerminal     uint32 = 0x80000000 // bit  31
)

// ── FieldTypeEnum ─────────────────────────────────────────────────────────────

// FieldTypeEnum is the ordinal encoding of a field's type in bits 4–1 of a
// FieldDescriptor. Values 0–14 are defined; 15 is reserved.
type FieldTypeEnum uint8

const (
	TypeUnknown   FieldTypeEnum = iota // 0
	TypeString                         // 1
	TypeNumber                         // 2
	TypeInteger                        // 3
	TypeBoolean                        // 4
	TypeBytes                          // 5
	TypeArray                          // 6  schema-bearing
	TypeSet                            // 7  schema-bearing
	TypeEnum                           // 8  schema-bearing
	TypeObject                         // 9  schema-bearing
	TypeRecord                         // 10 schema-bearing
	TypeUnion                          // 11 schema-bearing (Variants)
	TypeComposite                      // 12 schema-bearing (Variants)
	TypeGeometry                       // 13
	TypeDecimal                        // 14 stored as TypeInt (scaled integer)
)

// IsSchemaBearing reports whether a FieldTypeEnum requires a target_schema.
func (t FieldTypeEnum) IsSchemaBearing() bool {
	return t >= TypeArray && t <= TypeComposite
}

// ── Index enums ───────────────────────────────────────────────────────────────

type IndexType uint8

const (
	IndexTypeNormal   IndexType = iota
	IndexTypeUnique
	IndexTypePrimary
	IndexTypeSpatial
	IndexTypeFulltext
)

type IndexOrder uint8

const (
	IndexOrderAsc  IndexOrder = iota
	IndexOrderDesc
)

// ── Logical and comparison operators ─────────────────────────────────────────

type LogicalOperator uint8

const (
	LogicalAnd  LogicalOperator = iota
	LogicalOr
	LogicalNot
	LogicalNor
	LogicalXor
	LogicalNand
	LogicalXnor
)

type ComparisonOperator uint8

const (
	ComparisonEq       ComparisonOperator = iota
	ComparisonNeq
	ComparisonLt
	ComparisonLte
	ComparisonGt
	ComparisonGte
	ComparisonIn
	ComparisonNin
	ComparisonContains
	ComparisonNcontains
	ComparisonExists
	ComparisonNexists
)

// ── CompiledSchema ────────────────────────────────────────────────────────────

// CompiledSchema is the top-level IR artifact. One per compiled source document.
// It is immutable once returned from Compile.
type CompiledSchema struct {
	// Descriptors is the global flat array of FieldDescriptor values covering
	// all fields across all schemas. Fields are laid out in schema-index order;
	// within each schema, fields are ordered by UUID lexicographic sort.
	Descriptors []uint32

	// SchemaOffsets gives the descriptor range for each schema.
	// SchemaOffsets[N] packs start (bits 15–0) and end (bits 31–16) into Descriptors.
	// len(SchemaOffsets) == number of schemas in the document.
	SchemaOffsets []uint32

	// Variants maps a descriptor value to the ordered list of schema indices
	// for that field's union or composite variants. Only populated for TypeUnion
	// and TypeComposite fields.
	Variants map[uint32][]uint8

	// Store holds enum value sets and field defaults. Nil if the document
	// contains no enum fields and no fields with defaults.
	Store *document.Document

	// ResolvedConstraints is the fully resolved forest of constraints
	// defined at the root. All field paths are absolute from the root.
	ResolvedConstraints *ResolvedConstraintTree

	// ResolvedIndexes maps a packed (schemaIndex<<8 | indexOrdinal) key to
	// the storage-engine-ready resolved index form.
	ResolvedIndexes map[uint16]ResolvedIndex

	// Meta maps schema index to schema metadata. Required for correct traversal
	// of type schemas (union, composite, array, record, set) which carry no
	// fields of their own and must be resolved via Meta.Type / Meta.Variants /
	// Meta.TargetSchema. Also used by code generators, diff tools, and error
	// reporters.
	Meta map[uint8]*SchemaMetadata

	// AddressSpace is the compiled address table for field path resolution.
	// Required by Address(). Built during Compile alongside all other fields.
	AddressSpace *CompiledAddressSpace

	// PathCache maps a resolved DocumentKey back to the dot-separated path string
	// that produced it. Populated lazily by DocumentKey() on each successful
	// resolution. Used by Serialize to reconstruct constraint field paths from
	// DocumentKeys without a separate reverse-lookup pass.
	PathCache map[document.DocumentKey]string
}

// ── FieldDescriptor helpers ───────────────────────────────────────────────────

// DescriptorToDataPoint extracts the storage key for a field descriptor.
// The id is formed from owner_schema (bits 22–15) and field_index (bits 14–8)
// — a 15-bit composite at bits 22–8, extracted by (fd >> 8) & 0x7FFF. This
// uniquely identifies a field across all schemas because owner_schema occupies
// the upper 8 bits of the 15-bit id and field_index occupies the lower 7 bits.
func DescriptorToDataPoint(fd uint32) document.DataPoint {
	typ := fieldTypeToDataType(FieldTypeEnum((fd & FDMaskType) >> 1))
	id := int32((fd >> 8) & 0x7FFF)
	p, _ := document.NewDataPoint(typ, id)
	return p
}

// fieldTypeToDataType maps a FieldTypeEnum to the document.DataType used for
// storage in Document. Used by Store population and DescriptorToDataPoint.
func fieldTypeToDataType(t FieldTypeEnum) document.DataType {
	switch t {
	case TypeString:
		return document.TypeString
	case TypeNumber:
		return document.TypeFloat
	case TypeInteger:
		return document.TypeInt
	case TypeDecimal:
		return document.TypeInt // stored as scaled integer
	case TypeBoolean:
		return document.TypeBool
	case TypeBytes:
		return document.TypeBytes
	case TypeGeometry:
		return document.TypeGeometry
	case TypeRecord:
		return document.TypeRecord
	case TypeArray, TypeSet:
		return document.TypeArrayObject
	case TypeObject:
		return document.TypeRecord
	default:
		return document.TypeUnknown
	}
}

// enumElemTypeToArrayDataType maps a scalar FieldTypeEnum to the DataType used
// for storing its enum value set as a typed array in Store.
func enumElemTypeToArrayDataType(t FieldTypeEnum) document.DataType {
	switch t {
	case TypeInteger, TypeDecimal:
		return document.TypeArrayInt
	case TypeNumber:
		return document.TypeArrayFloat
	case TypeString:
		return document.TypeArrayString
	case TypeBoolean:
		return document.TypeArrayBool
	default:
		return document.TypeArrayUnknown
	}
}

// ExtractType extracts the FieldTypeEnum from a descriptor.
func ExtractType(fd uint32) FieldTypeEnum {
	return FieldTypeEnum((fd & FDMaskType) >> 1)
}

// ExtractFieldIndex extracts the 7-bit field_index from a descriptor.
func ExtractFieldIndex(fd uint32) uint8 {
	return uint8((fd & FDMaskFieldIndex) >> 8)
}

// ExtractOwnerSchema extracts the 8-bit owner_schema index from a descriptor.
func ExtractOwnerSchema(fd uint32) uint8 {
	return uint8((fd & FDMaskOwnerSchema) >> 15)
}

// ExtractTargetSchema extracts the 8-bit target_schema index from a descriptor.
// Returns 0 for union and composite fields — those fields do not use the
// target_schema bits; their targets are stored in CompiledSchema.Variants.
func ExtractTargetSchema(fd uint32) uint8 {
	return uint8((fd & FDMaskTargetSchema) >> 23)
}

// IsTerminal reports whether the terminal bit is set on a descriptor.
func IsTerminal(fd uint32) bool {
	return fd&FDMaskTerminal != 0
}

// IsRequired reports whether the required bit is set on a descriptor.
func IsRequired(fd uint32) bool {
	return fd&FDMaskRequired != 0
}

// IsUnique reports whether the unique bit is set on a descriptor.
func IsUnique(fd uint32) bool {
	return fd&FDMaskUnique != 0
}

// IsDeprecated reports whether the deprecated bit is set on a descriptor.
func IsDeprecated(fd uint32) bool {
	return fd&FDMaskDeprecated != 0
}

// IsSchemaBearing reports whether a descriptor's type requires a target_schema.
func IsSchemaBearing(fd uint32) bool {
	return ExtractType(fd).IsSchemaBearing()
}

// ── SchemaMetadata ────────────────────────────────────────────────────────────

// FieldMeta holds the UUID and name for a single field, keyed by descriptor value.
type FieldMeta struct {
	UUID        string
	Name        string
	Description string
}

// SchemaMetadata holds schema metadata. It is required for correct traversal of
// type schemas (union, composite, array, record, set) and is also consumed by
// code generators, diff tools, and error reporters.
type SchemaMetadata struct {
	UUID          string
	Name          string
	Version       string
	Description   string
	Concrete      bool
	Type          FieldTypeEnum             // for type schemas (enum, union, composite, array, record, set)
	Values        []any                     // for enum types only
	TargetSchema  uint8                     // for single-target type schemas (array, record, set)
	Variants      []uint8                   // for multi-target type schemas (union, composite)
	Fields        map[uint32]FieldMeta      // descriptor value → UUID + name
	Indexes       map[string]IndexDescriptor
	IndexOrdinals map[string]uint8           // index UUID → ordinal within this schema
	Metadata      map[string]any
}

// ── Index types ───────────────────────────────────────────────────────────────

// IndexDescriptor is the cold representation of a single index definition.
type IndexDescriptor struct {
	Name        string
	Description string
	Type        IndexType
	Order       IndexOrder
	Unique      bool
	Fields      []string       // dot-separated path strings — resolved to DataPoints at compile time
	Condition   IndexCondition // nil if no partial condition
}

// IndexCondition is implemented by IndexConditionLeaf and IndexConditionGroup.
type IndexCondition interface {
	indexCondition()
}

// IndexConditionLeaf is a single field comparison for a partial index.
type IndexConditionLeaf struct {
	Field    string
	Operator ComparisonOperator
	Value    any
}

// IndexConditionGroup combines child conditions with a logical operator.
type IndexConditionGroup struct {
	Operator   LogicalOperator
	Conditions []IndexCondition
}

func (IndexConditionLeaf)  indexCondition() {}
func (IndexConditionGroup) indexCondition() {}

// ResolvedIndex is the storage-engine-ready form of a single index definition.
type ResolvedIndex struct {
	Type      IndexType
	Order     IndexOrder
	Unique    bool
	Fields    []document.DocumentKey
	Condition ResolvedCondition
}

// ResolvedCondition is implemented by ResolvedConditionLeaf and ResolvedConditionGroup.
type ResolvedCondition interface {
	resolvedCondition()
}

// ResolvedConditionLeaf is the resolved form of an IndexConditionLeaf.
type ResolvedConditionLeaf struct {
	Field    document.DocumentKey
	Operator ComparisonOperator
	Value    any
}

// ResolvedConditionGroup is the resolved form of an IndexConditionGroup.
type ResolvedConditionGroup struct {
	Operator   LogicalOperator
	Conditions []ResolvedCondition
}

func (ResolvedConditionLeaf)  resolvedCondition() {}
func (ResolvedConditionGroup) resolvedCondition() {}

// ── Constraint types ──────────────────────────────────────────────────────────

// ConstraintTree is the cold constraint forest for a single schema.
type ConstraintTree struct {
	Roots    []ConstraintNode
	Index    map[string]ConstraintNode // node UUID → cold node
	Ordinals map[string]uint16         // node UUID → compiler-assigned ordinal
}

// ConstraintNode is implemented by Constraint and ConstraintGroup.
type ConstraintNode interface {
	constraintNode()
}

// Constraint is a single leaf constraint with a named predicate.
type Constraint struct {
	UUID        string
	Name        string
	Description string
	Predicate   string   // key into PredicateMap — resolved at compile time
	Fields      []string // dot-separated paths — resolved to DataPoints at compile time
	Parameters  any
}

// ConstraintGroup is a logical combination of constraint nodes.
type ConstraintGroup struct {
	UUID        string
	Name        string
	Description string
	Operator    LogicalOperator
	Constraints []ConstraintNode
}

func (Constraint)      constraintNode() {}
func (ConstraintGroup) constraintNode() {}

// ResolvedConstraintTree is the hot constraint forest for a single schema.
type ResolvedConstraintTree struct {
	Roots []ResolvedConstraintNode
	Index map[uint16]ResolvedConstraintNode // ordinal → resolved node
}

// Predicate is a validation function resolved from a PredicateMap at compile time.
type Predicate func(data *document.Document, fields []document.DocumentKey, args any) bool

// PredicateMap is the registry of named predicate functions supplied to the compiler.
type PredicateMap map[string]Predicate

// ResolvedConstraintNode is implemented by ResolvedConstraint and ResolvedConstraintGroup.
type ResolvedConstraintNode interface {
	resolvedConstraintNode()
}

// ResolvedConstraint is the validator-ready form of a single leaf constraint.
// Identity fields are retained from the cold Constraint so that Serialize can
// reconstruct the source document without loss.
type ResolvedConstraint struct {
	UUID          string
	Name          string
	Description   string
	PredicateName string // original string key into PredicateMap
	Predicate     Predicate
	Fields        []document.DocumentKey
	Parameters    any
}

// ResolvedConstraintGroup is the validator-ready form of a constraint group.
// Identity fields are retained from the cold ConstraintGroup for Serialize.
type ResolvedConstraintGroup struct {
	UUID        string
	Name        string
	Description string
	Operator    LogicalOperator
	Constraints []ResolvedConstraintNode
}

func (ResolvedConstraint)      resolvedConstraintNode() {}
func (ResolvedConstraintGroup) resolvedConstraintNode() {}

// ── Traversal ─────────────────────────────────────────────────────────────────

// visitedSet tracks which schema indices have been enqueued during graph walks.
// Two uint64s cover the full 128-schema index space with bitwise ops only.
// Zero value is ready to use.
type visitedSet struct {
	lo uint64 // schemas 0–63
	hi uint64 // schemas 64–127
}

func (v *visitedSet) mark(idx uint8) {
	if idx < 64 {
		v.lo |= 1 << idx
	} else {
		v.hi |= 1 << (idx - 64)
	}
}

func (v *visitedSet) seen(idx uint8) bool {
	if idx < 64 {
		return v.lo&(1<<idx) != 0
	}
	return v.hi&(1<<(idx-64)) != 0
}

// enqueueTargets appends the schema target(s) of a schema-bearing field to the
// walk queue, skipping any already visited. For union/composite fields the
// targets come from cs.Variants; for all other schema-bearing types the target
// comes from the descriptor's target_schema bits.
//
// This is the single authoritative place that handles the union/composite split,
// preventing the target_schema=0 false-enqueue that would otherwise occur for
// those types.
func enqueueTargets(cs *CompiledSchema, fd uint32, typ FieldTypeEnum, visited *visitedSet, queue []uint8, tail int) int {
	if typ == TypeUnion || typ == TypeComposite {
		for _, v := range cs.Variants[fd] {
			if !visited.seen(v) {
				visited.mark(v)
				queue[tail] = v
				tail = (tail + 1) % len(queue)
			}
		}
	} else {
		target := ExtractTargetSchema(fd)
		if !visited.seen(target) {
			visited.mark(target)
			queue[tail] = target
			tail = (tail + 1) % len(queue)
		}
	}
	return tail
}

// TerminalWalk visits every field reachable via terminal edges from schemaIdx.
// Non-terminal schema-bearing fields are passed to visit but not recursed into.
//
// Cycle safety: the terminal bit records that no cycle was detected on the
// specific DFS path used during Pass 5, but the terminal subgraph is not
// globally guaranteed to be acyclic across all entry points. TerminalWalk
// therefore maintains its own visitedSet and will visit each schema at most
// once per call, preventing infinite recursion.
//
// No heap allocation.
func TerminalWalk(cs *CompiledSchema, schemaIdx uint8, visit func(fd uint32)) {
	// BFS over schemas reachable via terminal edges.
	// Queue size 256 with power-of-two modulo: supports up to 255 concurrent
	// entries (one slot always empty to distinguish full from empty), which
	// exceeds the 128-schema hard limit with margin.
	var (
		visited    visitedSet
		queue      [256]uint8
		head, tail int
	)
	queue[tail] = schemaIdx
	tail++
	visited.mark(schemaIdx)

	for head != tail {
		idx := queue[head]
		head = (head + 1) & 255

		start := uint16(cs.SchemaOffsets[idx])
		end := uint16(cs.SchemaOffsets[idx] >> 16)

		for _, fd := range cs.Descriptors[start:end] {
			// Only visit leaf (non-schema-bearing) fields. Schema-bearing fields
			// are structural nodes; their leaf descendants are visited when their
			// target schemas are dequeued.
			if !IsSchemaBearing(fd) {
				visit(fd)
			}

			// Only follow terminal edges. Non-terminal means a cycle was
			// detected on this field's path during compilation — do not recurse.
			if fd&FDMaskTerminal == 0 || !IsSchemaBearing(fd) {
				continue
			}

			typ := ExtractType(fd)
			if typ == TypeUnion || typ == TypeComposite {
				for _, v := range cs.Variants[fd] {
					if !visited.seen(v) {
						visited.mark(v)
						queue[tail] = v
						tail = (tail + 1) & 255
					}
				}
			} else {
				target := ExtractTargetSchema(fd)
				if !visited.seen(target) {
					visited.mark(target)
					queue[tail] = target
					tail = (tail + 1) & 255
				}
			}
		}

		// Type schema passthrough: a type schema has no fields of its own; its
		// fields live in the schema(s) it points to. Follow via Meta.
		if start == end {
			if m := cs.Meta[idx]; m != nil && m.Type.IsSchemaBearing() {
				if m.Type == TypeUnion || m.Type == TypeComposite {
					for _, v := range m.Variants {
						if !visited.seen(v) {
							visited.mark(v)
							queue[tail] = v
							tail = (tail + 1) & 255
						}
					}
				} else {
					target := m.TargetSchema
					if !visited.seen(target) {
						visited.mark(target)
						queue[tail] = target
						tail = (tail + 1) & 255
					}
				}
			}
		}
	}
}

// FullWalk visits every field in every schema reachable from schemaIdx,
// visiting each schema exactly once. Cycle-safe. No allocation.
func FullWalk(cs *CompiledSchema, schemaIdx uint8, visit func(fd uint32)) {
	// Queue size 256 with power-of-two modulo. The visitedSet prevents any
	// schema from being enqueued more than once, so the maximum concurrent
	// queue depth is 128 (the schema hard limit). 256 slots ensures the
	// full=empty ambiguity of a power-of-two ring buffer cannot occur at
	// the maximum occupancy.
	var (
		visited    visitedSet
		queue      [256]uint8
		head, tail int
	)
	queue[tail] = schemaIdx
	tail++
	visited.mark(schemaIdx)

	for head != tail {
		idx := queue[head]
		head = (head + 1) & 255

		start := uint16(cs.SchemaOffsets[idx])
		end := uint16(cs.SchemaOffsets[idx] >> 16)

		for _, fd := range cs.Descriptors[start:end] {
			visit(fd)
			if !IsSchemaBearing(fd) {
				continue
			}

			typ := ExtractType(fd)
			// FIX: union/composite fields store targets exclusively in
			// cs.Variants — their target_schema bits are always zero.
			// Previously, ExtractTargetSchema was called unconditionally,
			// causing schema 0 (root) to be re-enqueued for every union/
			// composite field encountered before the root was marked visited.
			if typ == TypeUnion || typ == TypeComposite {
				for _, v := range cs.Variants[fd] {
					if !visited.seen(v) {
						visited.mark(v)
						queue[tail] = v
						tail = (tail + 1) & 255
					}
				}
			} else {
				target := ExtractTargetSchema(fd)
				if !visited.seen(target) {
					visited.mark(target)
					queue[tail] = target
					tail = (tail + 1) & 255
				}
			}
		}

		// Type schema passthrough: a type schema has no fields of its own; its
		// fields live in the schema(s) it points to. Follow via Meta.
		if start == end {
			if m := cs.Meta[idx]; m != nil && m.Type.IsSchemaBearing() {
				if m.Type == TypeUnion || m.Type == TypeComposite {
					for _, v := range m.Variants {
						if !visited.seen(v) {
							visited.mark(v)
							queue[tail] = v
							tail = (tail + 1) & 255
						}
					}
				} else if !visited.seen(m.TargetSchema) {
					visited.mark(m.TargetSchema)
					queue[tail] = m.TargetSchema
					tail = (tail + 1) & 255
				}
			}
		}
	}
}
