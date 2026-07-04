package definition

import (
	"sync"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/document"
)

// =============================================================================
// FIELD KIND
// =============================================================================

type FieldKind uint8

const (
	KindSimple      FieldKind = iota
	KindObject
	KindArrayField
	KindComplex
)

// =============================================================================
// DATAPOINT ADDRESS SCHEME — TWO LAYERS
// =============================================================================
//
// This package defines two distinct addressing concepts:
//
//   1. INTERNAL DataPoint — a compact unique identifier derived from the
//      FieldDescriptor bit layout. It encodes the field's DataType (4 bits)
//      plus a 27-bit discriminator formed by the structural bits of the
//      descriptor (SchemaIdx, FieldIdx, ChildSchemaIdx, Kind, Required,
//      HasDefault). Because every (schema, field) pair gets a unique
//      FieldDescriptor, this DataPoint is collision-free across all fields
//      in the compiled schema. It is used exclusively for CompiledSchema
//      side tables: Defaults, Enums, Variants.
//
//      Formula:  DataPoint = (descriptor & 0xFFFFFFE0) | (DataType << 1)
//
//      The DocumentKey (64-bit) embeds BOTH the internal DataPoint (for
//      type-directed storage lookup) and the full FieldDescriptor (for
//      rule evaluation), making each document value self-describing.
//
//   2. USER-DATA DataPoint — a 27-bit address computed by Address() from a
//      ResolvedPath using recursive block subdivision. This address depends
//      on the full path (not just the final field), so the same field at
//      two different nesting levels receives different addresses. It is
//      used for flat storage of user data in the DataContainer.
//
//      Layout:  [0, 2^14)        — single-step paths (root-level fields)
//               [2^14, 2^27)     — multi-step paths, subdivided recursively
//
//      The two address spaces are orthogonal:
//        - The INTERNAL DataPoint answers: "which field definition is this?"
//        - The USER-DATA DataPoint answers: "at which path was this value stored?"
//
// =============================================================================
// FIELD DESCRIPTOR
// =============================================================================
//
// FieldDescriptor is a packed uint32 that identifies a field in the flat
// CompiledSchema field table. Every unique (schema instance, field) pair
// gets its own descriptor.
//
// Layout:
//
//	bits 31-28: DataType (4 bits)
//	bits 27-22: SchemaIdx (6 bits) — index into CompiledSchema.Schemas (max 63)
//	bits 21-15: FieldIdx (7 bits) — position within the parent schema's fields (max 127)
//	bits 14-9:  ChildSchemaIdx (6 bits) — 0x3F if no child
//	bits 8-7:   Kind (2 bits)
//	bit  6:     Required
//	bit  5:     HasDefault
//	bit  4:     Deprecated
//	bit  3:     Unique
//	bit  2:     Terminal
//	bit  1:     Nullable
//	bit  0:     Recursive
type FieldDescriptor uint32

const (
	fdTypeMask         = uint32(0xF) << 28
	fdSchemaIdxMask    = uint32(0x3F) << 22
	fdFieldIdxMask     = uint32(0x7F) << 15
	fdChildSchemaIdxMask = uint32(0x3F) << 9
	fdKindMask         = uint32(0x3) << 7
	fdRequired         = uint32(1) << 6
	fdHasDefault       = uint32(1) << 5
	fdDeprecated       = uint32(1) << 4
	fdUnique           = uint32(1) << 3
	fdTerminal         = uint32(1) << 2
	fdNullable         = uint32(1) << 1
	fdRecursive        = uint32(1) << 0
	FdNoChild   uint8  = 0x3F // terminal/no-child sentinel
)

func MakeFieldDescriptor(dt document.DataType, kind FieldKind, schemaIdx, fieldIdx uint8, required, hasDefault, deprecated, unique, terminal, nullable, recursive bool, childSchemaIdx uint8) FieldDescriptor {
	var fd uint32
	fd |= uint32(dt) << 28
	fd |= uint32(schemaIdx&0x3F) << 22
	fd |= uint32(fieldIdx&0x7F) << 15
	fd |= uint32(childSchemaIdx&0x3F) << 9
	fd |= uint32(kind) << 7
	if required {
		fd |= fdRequired
	}
	if hasDefault {
		fd |= fdHasDefault
	}
	if deprecated {
		fd |= fdDeprecated
	}
	if unique {
		fd |= fdUnique
	}
	if terminal {
		fd |= fdTerminal
	}
	if nullable {
		fd |= fdNullable
	}
	if recursive {
		fd |= fdRecursive
	}
	return FieldDescriptor(fd)
}

func (f FieldDescriptor) DataType() document.DataType {
	return document.DataType((uint32(f) & fdTypeMask) >> 28)
}

func (f FieldDescriptor) SchemaIdx() uint8 {
	return uint8((uint32(f) & fdSchemaIdxMask) >> 22)
}

func (f FieldDescriptor) FieldIdx() uint8 {
	return uint8((uint32(f) & fdFieldIdxMask) >> 15)
}

// ChildSchemaIdx returns the child schema slot index for non-terminal fields.
// Returns 0x3F if the field has no child schema (terminal or scalar).
func (f FieldDescriptor) ChildSchemaIdx() uint8 {
	return uint8((uint32(f) & fdChildSchemaIdxMask) >> 9)
}

func (f FieldDescriptor) Kind() FieldKind {
	return FieldKind((uint32(f) & fdKindMask) >> 7)
}

func (f FieldDescriptor) Required() bool {
	return uint32(f)&fdRequired != 0
}

func (f FieldDescriptor) HasDefault() bool {
	return uint32(f)&fdHasDefault != 0
}

func (f FieldDescriptor) Deprecated() bool {
	return uint32(f)&fdDeprecated != 0
}

func (f FieldDescriptor) Unique() bool {
	return uint32(f)&fdUnique != 0
}

func (f FieldDescriptor) Terminal() bool {
	return uint32(f)&fdTerminal != 0
}

func (f FieldDescriptor) Nullable() bool {
	return uint32(f)&fdNullable != 0
}

func (f FieldDescriptor) Recursive() bool {
	return uint32(f)&fdRecursive != 0
}

// DataPoint returns the 32-bit DataPoint for this field descriptor.
func (f FieldDescriptor) DataPoint() uint32 {
	return (uint32(f) & 0xFFFFFFE0) | ((uint32(f) >> 28) & 0xF) << 1
}

// FieldDescriptorFromDataPoint recovers a FieldDescriptor from a DataPoint.
func FieldDescriptorFromDataPoint(dp uint32) FieldDescriptor {
	return FieldDescriptor((dp & 0xFFFFFFE0) | ((dp>>1)&0xF)<<28)
}

// =============================================================================
// SCHEMA SLOT
// =============================================================================

type SchemaSlot struct {
	FieldStart uint16 // index into CompiledSchema.Descriptors
	FieldCount uint16
	Footprint  uint32 // total address slots needed by this schema's subtree
}

// =============================================================================
// METADATA TYPES (cold path)
// =============================================================================

type FieldMeta struct {
	ID          string   // stable UUIDv7 — never changes across renames
	Name        string
	Path        string
	Parts       []string
	Description string
	Default     LiteralValue
}

type SchemaMeta struct {
	Name        string
	Description string
}

// =============================================================================
// ADDRESS SPACE BOUNDARIES
// =============================================================================

const (
	AddrBits          = 27
	SingleStepRegion  = 1 << 14   // [0, 2^14) — single-step paths
	MultiStepBase     = SingleStepRegion  // 2^14
	MultiStepSize     = (1 << AddrBits) - MultiStepBase  // 2^27 - 2^14
)

// =============================================================================
// COMPILED SCHEMA
// =============================================================================

type CompiledSchema struct {
	Descriptors []FieldDescriptor
	FieldsMeta  []FieldMeta
	Schemas     []SchemaSlot
	SchemasMeta []SchemaMeta
	Defaults    *document.Document
	Enums       *document.Document // keyed by DataPoint; value is []string (string enum), []int64 (int enum), or []any (complex enum)
	Variants    map[uint32][]uint8 // keyed by DataPoint; variant schema slot indices for union/composite fields
	Constraints []ResolvedConstraint
	Indexes     map[IndexId]Index

	// FieldTypes preserves the original FieldType for each descriptor.
	// Indexed by absolute descriptor index (parallel to Descriptors/FieldsMeta).
	FieldTypes []FieldType

	// SchemaConstraints holds per-slot raw constraints from the source
	// NestedSchema definitions. Indexed by schema slot index.
	SchemaConstraints []SchemaConstraint

	// FieldRefConstraints holds per-field call-site constraint overrides
	// for object/recursive fields. Keyed by the field's DataPoint.
	FieldRefConstraints map[uint32]SchemaConstraint
}



// =============================================================================
// RESOLVED PATH
// =============================================================================

type ResolvedStep uint16

func NewResolvedStep(schemaIdx, fieldIdx uint8) ResolvedStep {
	return ResolvedStep(uint16(schemaIdx)<<8 | uint16(fieldIdx))
}

func (r ResolvedStep) SchemaIdx() uint8 { return uint8(r >> 8) }
func (r ResolvedStep) FieldIdx() uint8  { return uint8(r & 0xFF) }

type ResolvedPath []ResolvedStep

func (p ResolvedPath) PathKey() string {
	if len(p) == 0 {
		return ""
	}
	var buf [128]byte
	for i, step := range p {
		buf[i*2] = step.SchemaIdx()
		buf[i*2+1] = step.FieldIdx()
	}
	return string(buf[:len(p)*2])
}

// =============================================================================
// COMPILED CONSTRAINT AND INDEX
// =============================================================================

type CompiledConstraint struct {
	Predicate  string
	Fields     []ResolvedPath
	Parameters any
}

type CompiledIndex struct {
	Type      IndexType
	Unique    bool
	Fields    []ResolvedPath
	Condition *CompiledIndexCondition
}

type CompiledIndexCondition struct {
	Field    ResolvedStep
	Operator common.ComparisonOperator
}

// =============================================================================
// ADDRESS CACHE
// =============================================================================

type AddressCache struct {
	mu    sync.RWMutex
	cache map[string]uint32
}

func NewAddressCache() *AddressCache {
	return &AddressCache{cache: make(map[string]uint32)}
}

func (ac *AddressCache) DataPoint(cs *CompiledSchema, path ResolvedPath) uint32 {
	key := path.PathKey()
	ac.mu.RLock()
	if dp, ok := ac.cache[key]; ok {
		ac.mu.RUnlock()
		return dp
	}
	ac.mu.RUnlock()
	dp := resolveDataPoint(cs, path)
	ac.mu.Lock()
	ac.cache[key] = dp
	ac.mu.Unlock()
	return dp
}

func resolveDataPoint(cs *CompiledSchema, path ResolvedPath) uint32 {
	if len(path) == 0 {
		return 0
	}
	step := path[len(path)-1]
	schemaIdx := step.SchemaIdx()
	fieldIdx := step.FieldIdx()
	if int(schemaIdx) >= len(cs.Schemas) {
		return 0
	}
	slot := cs.Schemas[schemaIdx]
	absFieldIdx := int(slot.FieldStart) + int(fieldIdx)
	if absFieldIdx >= len(cs.Descriptors) {
		return 0
	}
	return cs.Descriptors[absFieldIdx].DataPoint()
}
