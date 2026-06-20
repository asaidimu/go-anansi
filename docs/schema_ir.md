# Schema IR Specification

The IR is the compiled form of a human-authored JSON schema definition. It sits
between the source schema and all downstream consumers: a structural validator,
a binary serializer, a code generator, LLM structured output constraints, and a
Wails boundary encoder/decoder.

The IR is immutable once constructed. All layout is deterministic given the same
source. No consumer may mutate an IR after compilation.

---

## Design Principles

**Fully resolved.** All UUID references are resolved at compile time. No
consumer ever follows a reference to find out what a type is.

**Flat over nested.** Data is organized in flat arrays indexed by position.
Pointer chasing is minimized. The hot path is purely bitwise operations over
contiguous memory.

**Hot/cold separation.** Structural data touched by the validator and serializer
on every operation lives in contiguous, GC-scannable-minimal structures. Metadata
needed only by code generation, diffing, and error reporting lives behind a
pointer and is never loaded by structural consumers.

**Self-describing.** The descriptor array is self-describing. Schema membership,
field position, type, and graph properties are all encoded in each descriptor.
No auxiliary index is required to interpret a descriptor in isolation.

**Hard limits are invariants, not guidelines.** 128 schemas per document, 127
fields per schema. The compiler rejects sources that exceed these. They are a
direct consequence of the bit layout and cannot be changed without a breaking
IR format change.

---

## FieldDescriptor

A `FieldDescriptor` is a packed `uint32` encoding the complete structural
metadata of a single field. Every field in every schema — root or nested — is
represented by exactly one `FieldDescriptor`. All properties are extracted via
bitwise mask and shift. No parsing, no allocation, no pointer indirection.

### Bit Layout

```
 31  30        23  22        15  14        8   7   6   5   4     1  0
  C   T T T T T T T  O O O O O O O O  F F F F F F F   d   u   r  T T T T  _
  │   └─────────┘    └──────────────┘ └───────────┘
  │   target_schema  owner_schema     field_index
  │
  └── terminal

  d = deprecated    u = unique    r = required
  T T T T = type (bits 4–1)       _ = reserved (bit 0, must be zero)
```

| Bits  | Width | Name            | Description |
|-------|-------|-----------------|-------------|
| 31    | 1     | `terminal`      | 1 = following this field from its current position in a traversal will not produce a cycle. Always 1 for scalar (non-schema-bearing) fields. Set by the compiler's cycle-detection pass. |
| 30–23 | 8     | `target_schema` | Index of the schema this field's type resolves to. Zero for non-schema-bearing types. |
| 22–15 | 8     | `owner_schema`  | Index of the schema this field belongs to. Root schema = 0. |
| 14–8  | 7     | `field_index`   | Zero-based position of this field within its owner schema, by UUID lexicographic order. |
| 7     | 1     | `deprecated`    | 1 = field is marked deprecated in the source. |
| 6     | 1     | `unique`        | 1 = field values must be unique across all records. |
| 5     | 1     | `required`      | 1 = field must be present in every record. |
| 4–1   | 4     | `type`          | `FieldTypeEnum` ordinal (0–13). |
| 0     | 1     | _(reserved)_    | Must be zero. Reserved for future IR format use. |

### Field Descriptions

#### `terminal` (bit 31)

Set by the compiler's cycle-detection pass after all references are resolved.
This is a field-level property, not a schema-level property. It answers the
question: if a traversal follows this specific field from its current position
in the schema graph, will it terminate? `1` means yes — no cycle is reachable
via this field. `0` means a cycle exists on this path.

A field in schema A pointing to schema B may be terminal while a different
field also pointing to schema B is not, depending on the shape of the graph
above each field. The bit is path-sensitive and belongs on the descriptor, not
on the schema.

For non-schema-bearing (scalar) fields there is no target to follow. The
compiler always sets `terminal = 1` for scalar fields. Consumers may read the
bit unconditionally without first checking the field type.

Cycle detection is a compile-time responsibility. No runtime cycle detection
is ever needed.

#### `target_schema` (bits 30–23)

Index into the compiled schema table for the schema this field's type resolves
to. Valid only for schema-bearing types. Must be zero for scalar types. The root
schema is index 0; nested schemas are assigned indices 1–127 in compilation
order.

#### `owner_schema` (bits 22–15)

Index of the schema this field belongs to. Root schema is always 0. Allows a
consumer to reconstruct schema membership from a flat list of descriptors
without a nested data structure.

#### `field_index` (bits 14–8)

Zero-based ordinal position of this field within its owner schema, assigned by
UUID lexicographic sort order at compile time. Sort order is stable across
compilations given the same source, ensuring binary layout and wire format
agreements remain consistent.

#### `deprecated` (bit 7)

Set when the source field carries `deprecated: true`. The field remains
structurally valid. Code generators may suppress output or emit warnings.

#### `unique` (bit 6)

Set when the source field carries `unique: true`. Canonical location for
field-level uniqueness in the IR. Index-level uniqueness is tracked separately
in the index descriptor and does not affect this bit.

#### `required` (bit 5)

Set when the source field carries `required: true`. A record lacking a required
field is structurally invalid.

#### `type` (bits 4–1)

`FieldTypeEnum` ordinal. 4 bits accommodate values 0–15; the current enum
occupies 0–13, leaving two ordinals available for future types.

#### Reserved (bit 0)

Must be zero. The compiler must emit zero for this bit. Consumers must not
interpret it. Reserved for future IR format use.

### Masks and Extraction

```go
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

fieldType    := FieldTypeEnum((fd & FDMaskType)         >> 1)
isRequired   := (fd & FDMaskRequired)   != 0
isUnique     := (fd & FDMaskUnique)     != 0
isDeprecated := (fd & FDMaskDeprecated) != 0
fieldIndex   := uint8((fd & FDMaskFieldIndex)           >> 8)
ownerSchema  := uint8((fd & FDMaskOwnerSchema)          >> 15)
targetSchema := uint8((fd & FDMaskTargetSchema)         >> 23)
isTerminal   := (fd & FDMaskTerminal)   != 0
```

### FieldTypeEnum

| Ordinal | Name        | Schema-bearing | Notes |
|---------|-------------|----------------|-------|
| 0       | `unknown`   | no  | Untyped. No structural validation. |
| 1       | `string`    | no  | UTF-8 string. |
| 2       | `number`    | no  | IEEE 754 double-precision float. |
| 3       | `integer`   | no  | 64-bit signed integer. |
| 4       | `boolean`   | no  | Boolean true/false. |
| 5       | `bytes`     | no  | Raw byte sequence. |
| 6       | `array`     | yes | Ordered collection. Element type given by `target_schema`. |
| 7       | `set`       | yes | Unordered unique collection. Element type given by `target_schema`. |
| 8       | `enum`      | yes | Scalar constrained to a fixed value set. Value set given by `CompiledSchema.Store`. |
| 9       | `object`    | yes | Structured object with named fields. Shape given by `target_schema`. |
| 10      | `record`    | yes | Key-value map. Value type given by `target_schema`. |
| 11      | `union`     | yes | One of several named schemas. Variants given by `CompiledSchema.Variants`. |
| 12      | `composite` | yes | Merge of several named schemas. Constituents given by `CompiledSchema.Variants`. |
| 13      | `geometry`  | no  | Geospatial value. Format is implementation-defined. |

### Compiler Invariants

1. **`target_schema` is zero for non-schema-bearing types.** If the type ordinal
   is not in {6,7,8,9,10,11,12}, bits 30–23 must be zero.
2. **`target_schema` is non-zero for schema-bearing types.** Bits 30–23 must
   encode a valid schema index in 0–127.
3. **`field_index` is unique within `owner_schema`.** No two descriptors with
   the same `owner_schema` may share a `field_index`.
4. **`terminal` is compiler-derived.** Set during graph analysis after all
   references are resolved. Never present in the source. Always 1 for scalar
   fields.
5. **`Variants` is populated if and only if type is `union` or `composite`.**
   All other fields must have no entry in `CompiledSchema.Variants`.
6. **`Store` enum entries are populated if and only if type is `enum`.** The
   DataPoint key is derived via `DescriptorToDataPoint`. The stored array type
   must match the enum's element type per the DataType mapping table.
7. **Every schema has at least one field.** The compiler rejects schemas with
   no fields. `start < end` for all entries in `SchemaOffsets`.
8. **`ResolvedConstraints` is complete at compile time.** Every schema that
   has a `ConstraintTree` in `SchemaMetadata` has a corresponding
   `ResolvedConstraintTree` in `CompiledSchema.ResolvedConstraints`. Every
   node UUID in `ConstraintTree.Index` has a corresponding ordinal in
   `ConstraintTree.Ordinals` and a corresponding entry in
   `ResolvedConstraintTree.Index`. An unknown predicate name in any leaf is
   a compile error.
9. **`ResolvedIndexes` is complete at compile time.** Every index UUID present
   in any `SchemaMetadata.Indexes` map has a corresponding ordinal in
   `SchemaMetadata.IndexOrdinals` and a corresponding entry in
   `CompiledSchema.ResolvedIndexes`. All field paths and condition field
   references must resolve successfully via `Address()` or compilation fails.
10. **Bit 0 is always zero.** The compiler must not set the reserved bit
    regardless of source content.

### Hard Limits

| Limit                        | Value | Source                                                                  |
|------------------------------|-------|-------------------------------------------------------------------------|
| Maximum fields per schema    | 127   | `field_index` is 7 bits (0–126); value 127 is reserved                 |
| Maximum schemas per document | 128   | `owner_schema` / `target_schema` are 8 bits; root = 0, nested = 1–127  |
| Maximum type ordinals        | 15    | `type` is 4 bits; current enum uses 0–13                               |

---

## CompiledSchema

The top-level IR artifact. One per compiled source document.

```go
type CompiledSchema struct {
    Descriptors         []uint32
    SchemaOffsets       []uint32
    Variants            map[uint32][]uint8
    Store               *DataContainer
    ResolvedConstraints map[uint8]*ResolvedConstraintTree
    ResolvedIndexes     map[uint16]ResolvedIndex
    Meta                map[uint8]*SchemaMetadata
}
```

### `Descriptors []uint32`

Global flat array of `FieldDescriptor` values covering all fields across all
schemas — root and nested. Fields are laid out in schema-index order: all fields
of schema 0 first, then schema 1, and so on. Within each schema, fields are
ordered by UUID lexicographic sort, which determines `field_index`.

### `SchemaOffsets []uint32`

Slice giving the descriptor range for each schema. Position in the slice is the
schema index — `SchemaOffsets[N]` describes schema N. The slice length is
therefore equal to the total number of schemas in the document; no separate
schema count is required.

Each entry packs a start and end offset into `Descriptors`:

```go
// bits 15–0:  start index (inclusive) into Descriptors
// bits 31–16: end index (exclusive) into Descriptors
start  := uint16(SchemaOffsets[N])
end    := uint16(SchemaOffsets[N] >> 16)
fields := Descriptors[start:end]
```

Start and end are self-contained per entry. No sentinel, no dependency on
adjacent entries. The compiler guarantees every schema has at least one field,
so `start < end` for all valid entries. Contains no pointers; not scanned by
the GC.

### `Variants map[uint32][]uint8`

Sparse map from descriptor value to the ordered list of schema indices that
field resolves to. Only populated for `union` and `composite` fields. All other
fields have no entry. Keyed by descriptor value, which is stable and unique per
field by construction — `owner_schema` and `field_index` bits guarantee no two
fields share a descriptor value.

`Variants` is structural metadata — it describes what a union or composite field
*is*, not what it *holds*. Validators and serializers consult it to determine
the shape of a field before processing any values. It lives on `CompiledSchema`
directly rather than in `Store`.

### `Store *DataContainer`

Typed value store for enum value sets and field defaults, keyed by `DataPoint`
derived from each field's descriptor. Nil if the compiled document contains no
enum fields and no fields with defaults.

A descriptor is converted to a `DataPoint` using only its identity bits:

```go
// DescriptorToDataPoint extracts the storage key for a field descriptor.
// Only owner_schema (bits 22–15) and field_index (bits 14–8) contribute to
// the ID — 15 bits total, well within the 27-bit DataPoint ID space.
// The DataType is taken from the descriptor's type bits (bits 4–1).
func DescriptorToDataPoint(fd uint32) DataPoint {
    typ := DataType((fd & FDMaskType) >> 1)
    id  := int32((fd >> 8) & 0x7FFF)
    p, _ := NewDataPoint(typ, id)
    return p
}
```

`terminal`, `target_schema`, `deprecated`, `unique`, and `required` are not
part of field identity and are not encoded in the DataPoint key.

**Enum value sets** are stored as typed arrays, one entry per enum field.
The `DataType` in the DataPoint matches the enum's element type:

| Enum element type | DataType stored   |
|-------------------|-------------------|
| `integer`         | `TypeArrayInt`    |
| `number`          | `TypeArrayFloat`  |
| `string`          | `TypeArrayString` |
| `boolean`         | `TypeArrayBool`   |

Enum value sets are hot data — consulted on every validation pass for
enum-typed fields. The value set is fully resolved at compile time; no source
document is consulted at validation time.

**Field defaults** are stored as scalar values keyed by the field's DataPoint,
using the `DataType` corresponding to the field's base type. Only populated for
fields that carry a `default` in the source.

### `Meta map[uint8]*SchemaMetadata`

Sparse map from schema index to cold schema metadata. Only populated for
schemas that exist in the source. Loaded only by consumers that require it —
code generators, diff tools, error reporters. Never touched by the validator or
binary serializer. GC scanning cost is proportional to the number of schemas
actually present, not the hard limit.

### `ResolvedConstraints map[uint8]*ResolvedConstraintTree`

Sparse map from schema index to the fully resolved constraint forest for that
schema. Nil entry means the schema has no constraints. Populated at compile
time. The compiler requires a `PredicateMap` as input; an unknown predicate
name in any leaf is a compile error.

At validation time the validator evaluates each root in
`ResolvedConstraintTree.Roots` independently. Structural validation is a hard
gate — constraint evaluation only begins if the document passes structural
validation completely.

### `ResolvedIndexes map[uint16]ResolvedIndex`

Sparse map from a packed schema+ordinal key to the fully resolved,
storage-engine-ready index form. Populated at compile time for every index
across all schemas in the document. Field path strings and condition field
references are resolved to `DataPoint` values via `Address()`. The storage
engine consults this map directly — no string hashing, no path resolution at
runtime.

The key is a `uint16` packing the schema index into the high byte and the
index's ordinal within that schema (assigned by the compiler in source order)
into the low byte:

```go
key := uint16(schemaIndex)<<8 | uint16(indexOrdinal)
```

The cold `IndexDescriptor` in `SchemaMetadata.Indexes` carries the source UUID.
The compiler populates `SchemaMetadata.IndexOrdinals` mapping each UUID to its
assigned ordinal, enabling cross-reference from UUID to hot form in two numeric
lookups.

---

## SchemaMetadata

Cold metadata for a single schema. Loaded only by consumers that require it.
Never touched by the validator or binary serializer.

```go
// FieldMeta holds the UUID and name for a single field, keyed by descriptor value.
type FieldMeta struct {
    UUID string
    Name string
}

type SchemaMetadata struct {
    UUID          string
    Name          string
    Version       string
    Description   string
    Fields        map[uint32]FieldMeta   // descriptor value → UUID + name
    Indexes       map[string]IndexDescriptor
    IndexOrdinals map[string]uint8       // index UUID → ordinal within this schema
    Constraints   *ConstraintTree
    Metadata      map[string]any
}
```

`Fields` is keyed by descriptor value — consistent with `Variants` on
`CompiledSchema` and the DataPoint keys used in `Store`. A single lookup
resolves a descriptor to both the field's UUID and its human-readable name.

`Indexes` is keyed by index UUID from the source.

`IndexOrdinals` maps each index UUID to its compiler-assigned ordinal within
this schema. Combined with the schema index, this produces the `uint16` key
for `CompiledSchema.ResolvedIndexes`:

```go
key := uint16(schemaIndex)<<8 | uint16(meta.IndexOrdinals[uuid])
```

`Constraints` is nil if the schema has no constraints.

---

## Indexes

Indexes have two representations in the IR: a cold form for introspection and
error reporting, and a hot resolved form used exclusively by the storage engine.

### Cold form — `SchemaMetadata.Indexes`

Carried through the IR for introspection and error reporting. Never touched by
the storage engine at runtime.

```go
// IndexDescriptor is the cold representation of a single index definition.
type IndexDescriptor struct {
    Name        string
    Description string
    Type        IndexType
    Order       IndexOrder
    Unique      bool
    Fields      []string       // dot-separated path strings — resolved to DataPoints at compile time
    Condition   IndexCondition // optional partial-index condition — field refs resolved at compile time
}
```

`IndexType` and `IndexOrder` are `uint8` enums corresponding to `IndexTypeEnum`
and `IndexOrderEnum` in the source.

`IndexCondition` is a recursive condition tree. Leaf nodes carry a single field
comparison; group nodes combine child conditions with a logical operator. The
sealed interface ensures exhaustive handling at every use site.

```go
// IndexCondition is implemented by IndexConditionLeaf and IndexConditionGroup.
type IndexCondition interface {
    indexCondition()
}

type IndexConditionLeaf struct {
    Field    string
    Operator common.ComparisonOperator
    Value    any
}

type IndexConditionGroup struct {
    Operator   common.LogicalOperator
    Conditions []IndexCondition
}

func (IndexConditionLeaf)  indexCondition() {}
func (IndexConditionGroup) indexCondition() {}
```

`common.ComparisonOperator` and `common.LogicalOperator` are defined in
`package common` as `byte` enums with full JSON marshaling.

### Hot form — `CompiledSchema.ResolvedIndexes`

Fully resolved at compile time. Field path strings in `Fields` and field
references in `Condition` leaf nodes are resolved to `DataPoint` values via
`Address()`. The storage engine operates entirely on `DataPoint` keys into
`DataContainer` — no string resolution at runtime.

```go
// ResolvedIndex is the storage-engine-ready form of a single index definition.
type ResolvedIndex struct {
    Type      IndexType
    Order     IndexOrder
    Unique    bool
    Fields    []DataPoint        // resolved via Address(), same order as IndexDescriptor.Fields
    Condition ResolvedCondition  // nil if the index has no partial condition
}

// ResolvedCondition is implemented by ResolvedConditionLeaf and ResolvedConditionGroup.
type ResolvedCondition interface {
    resolvedCondition()
}

type ResolvedConditionLeaf struct {
    Field    DataPoint
    Operator common.ComparisonOperator
    Value    any
}

type ResolvedConditionGroup struct {
    Operator   common.LogicalOperator
    Conditions []ResolvedCondition
}

func (ResolvedConditionLeaf)  resolvedCondition() {}
func (ResolvedConditionGroup) resolvedCondition() {}
```

`ResolvedIndexes` on `CompiledSchema` is keyed by index UUID, consistent with
`SchemaMetadata.Indexes`. The storage engine reporting an index violation can
use the UUID to retrieve the cold `IndexDescriptor` for its name, description,
and original field paths without any re-resolution.

---

## Constraints

Constraints have two representations in the IR: a cold form for introspection
and error reporting, and a hot resolved form used exclusively by the validator.

Each schema's constraints are organized as a forest — a slice of independent
root nodes, each the entry point to a constraint tree. Roots are evaluated
independently; a failure in one tree does not affect evaluation of another.
Within each tree, evaluation short-circuits according to the logical operator:
AND stops at first failure, OR stops at first success.

Structural validation is a hard gate. Constraint evaluation only begins if the
document passes structural validation — required fields present, types correct,
enum values valid — completely. A constraint referencing a missing field will
never run.

### Cold form — `SchemaMetadata.Constraints`

Carried through the IR for introspection and error reporting. Never touched by
the validator.

```go
// ConstraintTree is the cold constraint forest for a single schema.
// Roots holds the independent top-level constraint trees.
// Index maps every node UUID in the forest to its ConstraintNode, enabling
// O(1) lookup for error reporting at any depth.
// Ordinals maps every node UUID to its compiler-assigned uint16 ordinal,
// enabling cross-reference into ResolvedConstraintTree.Index without string
// hashing on the hot path.
type ConstraintTree struct {
    Roots    []ConstraintNode
    Index    map[string]ConstraintNode
    Ordinals map[string]uint16
}

// ConstraintNode is implemented by Constraint and ConstraintGroup.
type ConstraintNode interface {
    constraintNode()
}

type Constraint struct {
    Name        string   // human-readable label
    Description string
    Predicate   string   // key into PredicateMap — resolved at compile time
    Fields      []string // dot-separated paths — resolved to DataPoints at compile time
    Parameters  any      // passed through verbatim to the predicate function
}

// ConstraintGroup is a logical combination of constraints. Every node in the
// tree — leaf or group — carries a UUID and appears in ConstraintTree.Index.
type ConstraintGroup struct {
    Name        string
    Description string
    Operator    common.LogicalOperator
    Constraints []ConstraintNode
}

func (Constraint)      constraintNode() {}
func (ConstraintGroup) constraintNode() {}
```

### Hot form — `CompiledSchema.ResolvedConstraints`

Fully resolved at compile time. The compiler takes a `PredicateMap` as input
alongside the source schema. For every leaf constraint it resolves field paths
to `DataPoint` values via `Address()` and looks up the predicate function by
name. For every group it preserves the logical structure with children resolved
in place. An unknown predicate name is a compile error — not a runtime error.

```go
// ResolvedConstraintTree is the hot constraint forest for a single schema.
// Roots holds the resolved independent top-level trees.
// Index maps every node's compiler-assigned uint16 ordinal to its
// ResolvedConstraintNode. On failure, the validator looks up the ordinal
// from ConstraintTree.Ordinals to retrieve the cold node for error reporting.
type ResolvedConstraintTree struct {
    Roots []ResolvedConstraintNode
    Index map[uint16]ResolvedConstraintNode
}

// Predicate is a validation function resolved from a PredicateMap at compile time.
// data is the container being validated.
// fields are the pre-resolved DataPoints for the constraint's field paths, in
// source order.
// args is the constraint's Parameters value, passed through verbatim.
type Predicate func(data *DataContainer, fields []DataPoint, args any) bool

// PredicateMap is the registry of named predicate functions supplied to the compiler.
type PredicateMap map[string]Predicate

// ResolvedConstraintNode is implemented by ResolvedConstraint and ResolvedConstraintGroup.
type ResolvedConstraintNode interface {
    resolvedConstraintNode()
}

// ResolvedConstraint is the validator-ready form of a single leaf constraint.
type ResolvedConstraint struct {
    Predicate  Predicate   // function pointer, looked up from PredicateMap at compile time
    Fields     []DataPoint // resolved via Address(), same order as Constraint.Fields
    Parameters any         // passed through verbatim from Constraint.Parameters
}

// ResolvedConstraintGroup is the validator-ready form of a constraint group.
// Children may be leaves or nested groups.
type ResolvedConstraintGroup struct {
    Operator    common.LogicalOperator
    Constraints []ResolvedConstraintNode
}

func (ResolvedConstraint)      resolvedConstraintNode() {}
func (ResolvedConstraintGroup) resolvedConstraintNode() {}
```

On failure at any node the validator has the failing `ResolvedConstraintNode`
directly. To surface a human-readable error it looks up the node's ordinal in
`ConstraintTree.Ordinals` — one string lookup into cold data — then retrieves
the cold `ConstraintNode` from `ConstraintTree.Index` at that UUID for its
name, description, and original field paths. The hot evaluation path never
touches strings.

---

## Traversal

The IR exposes two canonical walk primitives. Both are zero-allocation. Neither
requires the caller to manage cycle detection or visited state.

**`TerminalWalk`** recurses only into fields where `terminal = 1`. Non-terminal
fields are visited as leaves — the `visit` function is called but the traversal
does not descend into their `target_schema`. This is the correct walk for the
validator and serializer: they process whatever fields are present in a
`DataContainer` and do not need to follow cyclic schema references.

**`FullWalk`** visits every reachable schema exactly once regardless of cycles.
Correct for code generators and diff tools that need to emit or compare every
schema definition reachable from a root. Implemented iteratively with a
fixed-capacity queue and a bitmask visited set — both sized to the hard schema
limit of 128 and stack-allocated with no heap allocation.

```go
// visitedSet tracks which schema indices have been enqueued during FullWalk.
// Two uint64s cover the full 128-schema index space with bitwise ops only.
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

// isSchemaBearing reports whether the descriptor's type requires a target_schema.
func isSchemaBearing(fd uint32) bool {
    typ := (fd & FDMaskType) >> 1
    return typ >= 6 && typ <= 12 // TypeArray through TypeComposite
}

// TerminalWalk visits every field reachable via terminal edges from schemaIdx.
// Non-terminal fields are passed to visit but not recursed into.
// Stack depth is bounded by the longest acyclic path in the schema graph.
// No allocation.
func TerminalWalk(cs *CompiledSchema, schemaIdx uint8, visit func(fd uint32)) {
	start := uint16(cs.SchemaOffsets[schemaIdx])
	end   := uint16(cs.SchemaOffsets[schemaIdx] >> 16)
    
	for _, fd := range cs.Descriptors[start:end] {
		visit(fd)
        
		// Recurse only if the field is both terminal and schema-bearing.
		if (fd&FDMaskTerminal) != 0 && isSchemaBearing(fd) {
			typ := (fd & FDMaskType) >> 1
            
			// Union and Composite fields resolve their targets via the Variants map.
			if typ == 11 || typ == 12 { // TypeUnion, TypeComposite
				for _, variantIdx := range cs.Variants[fd] {
					TerminalWalk(cs, variantIdx, visit)
				}
			} else {
				// All other schema-bearing fields use the embedded target_schema bits.
				target := uint8((fd & FDMaskTargetSchema) >> 23)
				TerminalWalk(cs, target, visit)
			}
		}
	}
}

// FullWalk visits every field in every schema reachable from schemaIdx,
// visiting each schema exactly once. Cycle-safe. No allocation.
func FullWalk(cs *CompiledSchema, schemaIdx uint8, visit func(fd uint32)) {
    var (
        visited    visitedSet
        queue      [128]uint8
        head, tail int
    )
    queue[tail] = schemaIdx
    tail++
    visited.mark(schemaIdx)

    for head != tail {
        idx  := queue[head]
        head  = (head + 1) & 127

        start := uint16(cs.SchemaOffsets[idx])
        end   := uint16(cs.SchemaOffsets[idx] >> 16)
        for _, fd := range cs.Descriptors[start:end] {
            visit(fd)
            if isSchemaBearing(fd) {
                target := uint8((fd & FDMaskTargetSchema) >> 23)
                if !visited.seen(target) {
                    visited.mark(target)
                    queue[tail] = target
                    tail = (tail + 1) & 127
                }
                // union and composite fields carry their concrete variant schemas
                // in Variants, not in target_schema. Enqueue each variant schema
                // that has not yet been visited.
                typ := (fd & FDMaskType) >> 1
                if typ == 11 || typ == 12 { // TypeUnion, TypeComposite
                    for _, v := range cs.Variants[fd] {
                        if !visited.seen(v) {
                            visited.mark(v)
                            queue[tail] = v
                            tail = (tail + 1) & 127
                        }
                    }
                }
            }
        }
    }
}
```

### Guarantees

- `TerminalWalk` terminates by construction — `terminal = 1` is only set on
  fields that lead to acyclic subgraphs, as established by the compiler's
  cycle-detection pass.
- `FullWalk` terminates by construction — the visited set prevents any schema
  from being enqueued more than once, and the schema count is bounded at 128.
  `union` and `composite` fields have their variant schemas enqueued via
  `Variants` in addition to `target_schema`, ensuring no reachable schema is
  missed.
- Neither walk allocates. `visitedSet` is two `uint64`s. The `queue` in
  `FullWalk` is a `[128]uint8` on the stack. Both are sized exactly to the
  hard schema limit.
- The `visit` callback receives raw `uint32` descriptor values. All field
  properties are extracted via the standard masks. The callback is responsible
  for any type dispatch it requires.
