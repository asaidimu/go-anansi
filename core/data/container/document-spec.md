# DataContainer Specification

This document describes the implemented `core/data/container` package and its integration
with the schema compiler in `core/schema/definition`. The type mapping is congruent
with the `FieldType` enum defined in `core/schema/meta/schema.json`.

`DataContainer` is the replacement for map-based document storage. It is a self-describing,
type-indexed, poolable, sparse data container addressed by 64-bit `DataContainerKey`s.

## Design Philosophy

1. **Schema Opacity** — `DataContainer` has no knowledge of schema internals. The schema is
   an opaque concern of the schema compiler, which produces the keys and values DataContainer
   stores.
2. **Efficient Memory** — Pay only for slots actually used. Zero allocations on reuse
   after pool warmup.
3. **Fast Access** — O(1) for all field operations after warmup. Nested fields are
   resolved by the schema compiler at definition time, not at access time.
4. **Explicit State** — Unambiguous three-way distinction: not set, null, and has value.
5. **Poolable by Design** — `Clear()` resets without deallocating. A `Pool` returns
   `*DataContainer`s after recursively reclaiming their nested children.
6. **Self-Describing Keys** — A single `DataContainerKey` embeds both the `DataPoint`
   (type-directed storage lookup) and the `FieldDescriptor` (rule evaluation), so a
   value carries everything needed for validation without a secondary lookup.

---

## Schema Type Mapping

This section defines how schema-level `FieldType`s (as defined by the `FieldType`
enum in `core/schema/meta/schema.json`) map to DataContainer `DataType`s. The mapping is
the responsibility of the **schema compiler** (`core/schema/definition/link.go`) —
`DataContainer` itself is schema-agnostic and only ever sees `DataType`s, `DataContainerKey`s,
and typed values.

The `FieldType` enum is the authoritative list of schema types:

```
unknown, string, number, integer, decimal, boolean,
array, enum, object, record, union, composite, geometry, bytes
```

### Primitive types

These map directly with no schema-layer involvement.

| Schema type | DataContainer `DataType` | Go type |
|---|---|---|
| `unknown` | `TypeUnknown` | `any` |
| `string` | `TypeString` | `string` |
| `number` | `TypeFloat` | `float64` |
| `integer` | `TypeInt` | `int64` |
| `decimal` | `TypeString` | canonical decimal string |
| `boolean` | `TypeBool` | `bool` |
| `bytes` | `TypeBytes` | `[]byte` |
| `geometry` | `TypeGeometry` | `[][]float64` |

**`decimal`** is stored as its **canonical decimal string** (`core/types/decimal`).
The schema validator accepts string values for `decimal` fields
(`decimal.IsValid`), and encoding/decoding to and from the string form is the schema
layer's responsibility. This preserves arbitrary precision — `decimal` is *never*
widened to `float64`.

### `enum`

Enums map to `TypeInt` when the enum's values are numeric, and `TypeString`
otherwise. The selected value is stored directly (an ordinal index for numeric
enums, the string label for string enums). Value↔label translation is a schema-layer
concern; DataContainer stores only the typed value.

### `object`

An `object` field is stored as `TypeRecord`, holding a **nested `*DataContainer`** compiled
from the referenced object schema. Objects are *not* flattened into leaf DataPoints at
the DataContainer layer; nesting is preserved so the codec and validator can recurse.

### `record`

A `record` (`Record<string, T>`) is treated like a **container** at the DataContainer
layer — identical to `array`. String-key semantics are a schema-layer concern; DataContainer
stores an ordered collection of typed values.

### `array`

Array element type determines which `TypeArray*` slot is used.

| Element type | DataContainer `DataType` |
|---|---|
| `integer` | `TypeArrayInt` |
| `number` | `TypeArrayFloat` |
| `decimal` | `TypeArrayString` (canonical decimal strings) |
| `string` | `TypeArrayString` |
| `boolean` | `TypeArrayBool` |
| `bytes` | `TypeArrayBytes` |
| `geometry` | `TypeArrayGeometry` |
| `object` (named element schema) | `TypeArrayObject` |
| `unknown` / bare container / open element | `TypeArrayUnknown` |

### `union` and `composite`

Both map to `TypeRecord` — the field value is a nested `*DataContainer`. The compiler records
the variant / part schema slots for each field so the validator and codec can interpret
the nested container. There is no `TypeUnknown` fallback for unions: the discriminator is
carried by the variant schemas, not by the storage type.

### Full mapping table

| Schema type | Condition | DataContainer `DataType` | `FieldKind` |
|---|---|---|---|
| `unknown` | — | `TypeUnknown` | Simple |
| `string` | — | `TypeString` | Simple |
| `number` | — | `TypeFloat` | Simple |
| `integer` | — | `TypeInt` | Simple |
| `decimal` | — | `TypeString` | Simple |
| `boolean` | — | `TypeBool` | Simple |
| `bytes` | — | `TypeBytes` | Simple |
| `geometry` | — | `TypeGeometry` | Simple |
| `enum` | numeric values | `TypeInt` | Simple |
| `enum` | string values | `TypeString` | Simple |
| `object` | — | `TypeRecord` | Object (non-terminal) |
| `recursive` | — | `TypeRecord` | Object (terminal) |
| `array` / `record` | named element schema | `TypeArrayObject` | ArrayField (non-terminal) |
| `array` / `record` | inline `integer` element | `TypeArrayInt` | ArrayField |
| `array` / `record` | inline `number` element | `TypeArrayFloat` | ArrayField |
| `array` / `record` | inline `decimal` element | `TypeArrayString` | ArrayField |
| `array` / `record` | inline `string` element | `TypeArrayString` | ArrayField |
| `array` / `record` | inline `boolean` element | `TypeArrayBool` | ArrayField |
| `array` / `record` | inline `bytes` element | `TypeArrayBytes` | ArrayField |
| `array` / `record` | inline `geometry` element | `TypeArrayGeometry` | ArrayField |
| `array` / `record` | bare / open element | `TypeArrayUnknown` | ArrayField |
| `union` | — | `TypeRecord` | Complex (non-terminal) |
| `composite` | — | `TypeRecord` | Complex (non-terminal) |

Inline element types come from the `InlineTypeKind` enum
(`string, number, integer, decimal, boolean, bytes, unknown, record`) — a subset of
`FieldType`, since inline descriptors cannot reference named schemas.

---

## Type System

### DataType

```go
type DataType uint8

const (
	TypeUnknown       DataType = iota // any
	TypeInt                           // int64
	TypeFloat                         // float64
	TypeString                        // string
	TypeBool                          // bool
	TypeBytes                         // []byte
	TypeGeometry                      // [][]float64
	TypeRecord                        // *DataContainer
	TypeArrayUnknown                  // []any
	TypeArrayInt                      // []int64
	TypeArrayFloat                    // []float64
	TypeArrayString                   // []string
	TypeArrayBool                     // []bool
	TypeArrayBytes                    // [][]byte
	TypeArrayObject                   // []*DataContainer
	TypeArrayGeometry                 // [][][]float64
)
```

The iota values map directly to slot indices in `DataContainer.data [16]unsafe.Pointer`.
There are exactly 16 types and exactly 16 slots — this is intentional and must be preserved.

**Slot backing types:**

| Constant | Go type | Notes |
|---|---|---|
| `TypeUnknown` | `any` | Escape hatch for untyped values |
| `TypeInt` | `int64` | Covers all integer widths; numeric enum ordinals |
| `TypeFloat` | `float64` | Covers all float widths |
| `TypeString` | `string` | UTF-8; canonical decimal strings |
| `TypeBool` | `bool` | |
| `TypeBytes` | `[]byte` | Binary blobs, hashes, UUIDs, encoded payloads |
| `TypeGeometry` | `[][]float64` | Array of coordinate rings |
| `TypeRecord` | `*DataContainer` | Nested typed sub-document (object/union/composite/recursive) |
| `TypeArrayUnknown` | `[]any` | Also covers open / bare container elements |
| `TypeArrayInt` | `[]int64` | |
| `TypeArrayFloat` | `[]float64` | |
| `TypeArrayString` | `[]string` | |
| `TypeArrayBool` | `[]bool` | |
| `TypeArrayBytes` | `[][]byte` | |
| `TypeArrayObject` | `[]*DataContainer` | Ordered array of nested documents |
| `TypeArrayGeometry` | `[][][]float64` | |

---

## Field Identification

### DataPoint

`DataPoint` is the 32-bit type-directed identifier embedded in every key.

```go
type DataPoint int32
```

**Bit layout: Null(1) : Type(4) : ID(27) = 32 bits**

```
┌──────────┬────────────┬──────────────────────────────────┐
│ Null(1b) │ Type(4b)   │          ID(27b)                 │
└──────────┴────────────┴──────────────────────────────────┘
   0          1–4              5–31
```

**Components:**

| Component | Bits | Range | Purpose |
|---|---|---|---|
| **Null** | 1 | 0–1 | Null flag. 1 = explicitly null. |
| **Type** | 4 | 0–15 | Maps directly to `DataType` iota. Tells DataContainer which typed slice to use. |
| **ID** | 27 | 0–134,217,727 | Unique field identifier. Schema-derived; DataContainer never interprets it. |

**Constants:**

```go
const (
	nullBits = 1
	typeBits = 4
	dataBits = nullBits + typeBits // 5

	typeMask       DataPoint = 0xF       // 4 bits
	identifierMask int32     = 0x7FFFFFF // 27 bits
)
```

**Construction and accessors:**

```go
func NewDataPoint(typ DataType, id ...int32) (DataPoint, error)
func (p DataPoint) Type() DataType
func (p DataPoint) ID() int32
func (p DataPoint) WithID(id int32) (DataPoint, error)
func (p DataPoint) IsNull() bool
```

### DataContainerKey

`DataContainerKey` is the 64-bit key used for all DataContainer storage. It embeds the
`DataPoint` (low 32 bits) for type-directed storage lookup alongside the full
`FieldDescriptor` (high 32 bits) for rule evaluation.

```go
type DataContainerKey int64

// bits 63–32: field descriptor (uint32) — type, owner_schema, field_index, flags
// bits 31–0:  DataPoint (int32)          — null flag, DataType, 27-bit ordinal
```

```go
func NewDataContainerKey(dp DataPoint, descriptor uint32) DataContainerKey
func (k DataContainerKey) DataPoint() DataPoint
func (k DataContainerKey) Descriptor() uint32
func (k DataContainerKey) Type() DataType
func (k DataContainerKey) IsNull() bool
```

`DataContainer.positions` is keyed by the **full 64-bit `DataContainerKey`**. Two keys with the
same `DataPoint` but different descriptors are distinct entries — this is what lets the
same field be distinguished across schema versions or call-site constraint overrides.

### FieldDescriptor (schema compiler)

`core/schema/definition` packs field metadata into a 32-bit `FieldDescriptor`:

```
bits 31-28: DataType (4 bits)
bits 27-22: SchemaIdx (6 bits)
bits 21-15: FieldIdx (7 bits)
bits 14-9:  ChildSchemaIdx (6 bits) — 0x3F if no child
bits 8-7:   Kind (2 bits)           — Simple / Object / ArrayField / Complex
bit  6:     Required
bit  5:     HasDefault
bit  4:     Deprecated
bit  3:     Unique
bit  2:     Terminal
bit  1:     Nullable
bit  0:     Recursive
```

The compiler derives a `DataPoint` from a descriptor as:

```
DataPoint = (descriptor & 0xFFFFFFE0) | ((descriptor >> 28) & 0xF) << 1
```

so the 27-bit ID encodes the descriptor's structural fields. The `AddressCache`
(`compiled.go`) memoises `path → DataPoint` so that after warmup a nested path
resolves in a single map lookup.

---

## Hole Management

Holes reuse `DataPoint` as their encoding. A hole stores the `DataType` of the freed
slot and the slice index now available for reuse. In `DataContainer`, holes are wrapped in a
`DataContainerKey` carrying the same descriptor as the freed field.

```go
// Creating a hole when a position is freed (SetNull / Unset):
hole, _ := NewDataPoint(key.Type(), sliceIndex)
d.holes = append(d.holes, NewDataContainerKey(hole, key.Descriptor()))

// Claiming a hole of a given type (LIFO scan, swap-and-pop removal):
func (d *DataContainer) claimHole(typ DataType) int32 {
	for i := len(d.holes) - 1; i >= 0; i-- {
		if d.holes[i].Type() == typ {
			idx := d.holes[i].DataPoint().ID()
			d.holes[i] = d.holes[len(d.holes)-1]
			d.holes = d.holes[:len(d.holes)-1]
			return idx
		}
	}
	return -1
}
```

**Properties:**
- LIFO scan (backwards) — recently freed slots are reused first, improving cache locality.
- O(h) scan where h is the number of holes; h is small in practice.
- Swap-and-pop removal is O(1) and avoids shifting.
- Slice indices are bounded by `identifierMask`; `freePosition` panics if encoding a hole
  ever fails, guarding against regressions.

---

## DataContainer

`DataContainer` is the storage engine. It holds up to 16 typed slices (one per `DataType`),
accessed via `unsafe.Pointer` to avoid interface boxing.

```go
type DataContainer struct {
	data      [16]unsafe.Pointer // index = DataType iota value; lazily initialised
	positions map[int64]int32    // int64(DataContainerKey) -> slice index (-1 = null)
	holes     []DataContainerKey      // freed slice positions available for reuse
}
```

### Storage Layout

Each slot in `data` corresponds to a `DataType` by its iota index. The pointer stored is
a pointer **to the slice header** (`*[]T`), not to the backing array. This is critical
for append safety: when `append` reallocates the backing array, it updates the slice
header in place, so `data[i]` remains valid after any reallocation.

Slots are **lazily initialised** on first write (default capacity 8). An untouched slot
holds a nil pointer — no allocation occurs for unused types.

**Memory cost of `data`:** `16 × 8 bytes = 128 bytes` on 64-bit systems, always,
regardless of how many slots are populated.

### Append safety

```go
// Correct — appends through the stored pointer to the header.
// If the backing array grows, the header is updated in place.
ptr := (*[]int64)(d.slot(TypeInt))
*ptr = append(*ptr, value)

// Wrong — takes a copy of the header value.
// After append with growth, the copy is stale.
ints := *(*[]int64)(d.slot(TypeInt))
ints = append(ints, value) // d.data[TypeInt] is now stale if reallocation occurred
```

Always append through the pointer, never through a copied header.

### GC safety

`unsafe.Pointer` is traced by the Go GC — as long as `DataContainer` is reachable, all slice
headers and their backing arrays are reachable. The risk arises only if a pointer is
ever widened to `uintptr`. Never convert `data[i]` to `uintptr` except as an atomic
expression within a single `unsafe.Pointer` operation.

### Creation

```go
func NewDataContainer() *DataContainer {
	return &DataContainer{
		positions: make(map[int64]int32),
	}
}
```

Typed slices are lazily initialised on first use. Only `positions` is allocated at
construction time.

---

## Field States

A field can be in exactly one of three states:

| State | `positions` entry | `IsSet()` | `IsNull()` | `HasValue()` | Get return |
|---|---|---|---|---|---|
| **Not Set** | absent | false | false | false | zero, false, nil |
| **Null** | `-1` | true | true | false | zero, true, nil |
| **Has Value** | `≥ 0` | true | false | true | value, true, nil |

**State transitions:**

```
Not Set ──Set(value)── -> Has Value
  │             │
  └──SetNull()── -> Null ←───┘
           │
         Unset()
           │
         Not Set
```

**Null semantics:** When a field transitions to null its current slice position is
**immediately freed into holes**. The positions entry becomes `-1` and holds no index.
The freed slot is available for reuse by any other field of the same type.

---

## Core Operations

### Set / Append / Get

All 16 types follow the identical pattern (shown for `int64`):

```go
func (d *DataContainer) SetInt(key DataContainerKey, value int64) error {
	if key.Type() != TypeInt {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		// Live position — update in place, no allocation.
		(*(*[]int64)(d.slot(TypeInt)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeInt); idx >= 0 {
		// Reuse a freed position.
		(*(*[]int64)(d.slot(TypeInt)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendInt(key, value) // no hole — append.
}

func (d *DataContainer) AppendInt(key DataContainerKey, value int64) error {
	if key.Type() != TypeInt {
		return ErrTypeMismatch
	}
	ptr := (*[]int64)(d.slot(TypeInt))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *DataContainer) GetInt(key DataContainerKey) (int64, bool, error) {
	if key.Type() != TypeInt {
		return 0, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return 0, false, nil // not set
	}
	if idx < 0 {
		return 0, true, nil // null
	}
	return (*(*[]int64)(d.slot(TypeInt)))[idx], true, nil
}
```

**Return convention:** `(value, isSet, error)`
- `false, false, nil` -> not set
- `zero, true, nil` -> explicitly null
- `value, true, nil` -> has value
- `_, _, err` -> type mismatch

**Complexity:**
- Update existing value: O(1)
- Insert, hole available: O(h), h typically < 10
- Insert, no hole: O(1) amortised

### SetNull / Unset

```go
func (d *DataContainer) SetNull(key DataContainerKey) {
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		d.freePosition(key, idx) // free the current slice position immediately
	}
	d.positions[k] = -1
}

func (d *DataContainer) Unset(key DataContainerKey) {
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		d.freePosition(key, idx)
	}
	delete(d.positions, k)
}
```

Setting a field null (or unsetting a valued field) frees its slice position into holes.
A null field holds no slice index; an unset field becomes `IsSet=false`.

### State checks

```go
func (d *DataContainer) IsSet(key DataContainerKey) bool     // present in positions
func (d *DataContainer) IsNull(key DataContainerKey) bool    // present with idx < 0
func (d *DataContainer) HasValue(key DataContainerKey) bool  // present with idx >= 0
func (d *DataContainer) Length() int                    // len(positions)
```

All O(1).

### Clear (for pooling)

`Clear` resets slice **lengths** to zero while preserving **capacity**, clears
`positions`, and empties `holes`. Backing arrays survive intact. Combined with pooling,
a warmed `DataContainer` performs zero allocations on reuse.

```go
func (d *DataContainer) Clear() {
	clear(d.positions)
	d.holes = d.holes[:0]
	for i, ptr := range d.data {
		if ptr == nil {
			continue
		}
		switch DataType(i) {
		case TypeInt:
			*(*[]int64)(ptr) = (*(*[]int64)(ptr))[:0]
		// ... one case per DataType ...
		}
	}
}
```

The mutation is done through the stored pointer (`*s = (*s)[:0]`), never through a
local copy — a local copy would leave `data[i]` pointing at the old untruncated header.

**Complexity:** O(16) constant sweep + O(m) for `clear(positions)` where m = number of
set fields.

### Walk (serialisation / deserialisation)

`Walk` exposes `positions` and the `slot` accessor directly to the caller. This enables
zero-copy serialisation and in-place deserialisation without boxing values through `any`.

```go
func (d *DataContainer) Walk(
	walker func(
		positions map[int64]int32,
		slot func(t DataType, initialSize ...int) unsafe.Pointer,
	) (any, error),
) (any, error) {
	return walker(d.positions, d.slot)
}
```

**Warning:** `Walk` grants direct mutable access to `positions` and the slice headers.
The caller is responsible for maintaining invariants: all positive indices in
`positions` must be valid indices into their respective typed slice, and holes must
reflect any positions freed outside of the normal `Unset`/`SetNull` path.

**Complexity:** O(1) — Walk itself is a direct delegation. Cost is entirely determined
by what the walker does.

---

## Collection

`Collection` is an ordered, pool-aware bag of `*DataContainer`s. It makes no assumptions
about schema identity — documents from different schemas can coexist.

### Ownership model

- A collection returned by `NewCollection` **owns** its documents and returns them to
  the pool on `Release`.
- A collection returned by `Filter` is a **view** — it holds pointers to the same
  documents as the source but does not own them. `Release` on a view only resets the
  view's own slice.
- A collection returned by `FilterCopy`/`Project` **owns** its documents (fresh
  copies from the pool) and releases them on `Release`.

```go
type Collection struct {
	docs  []*DataContainer
	pool  *Pool
	owner bool
}
```

### Operations

```go
func NewCollection(pool *Pool) *Collection
func (c *Collection) Append(doc *DataContainer) error
func (c *Collection) Len() int
func (c *Collection) At(i int) *DataContainer
func (c *Collection) Each(f func(i int, doc *DataContainer) bool)
func (c *Collection) Filter(keep func(*DataContainer) bool) *Collection
func (c *Collection) FilterCopy(keep func(*DataContainer) bool) (*Collection, error)
func (c *Collection) Project(keys []DataContainerKey) (*Collection, error)
func (c *Collection) Reduce(initial any, f func(acc any, doc *DataContainer) any) any
func (c *Collection) Release()
```

- `Filter` — view; use when the source outlives the filtered result.
- `FilterCopy` — owning copies; use when the source will be released before the result
  is consumed, or an independent copy is needed for mutation.
- `Project` — owning copies containing only the requested keys.
- `Release` — returns all owned documents to the pool (via `Pool.Put`, which recurses
  into nested children) and resets the collection. No-op on documents for views.

`FilterCopy` and `Project` deep-copy each document: `TypeRecord` and `TypeArrayObject`
children are copied recursively (allocated from the pool), so copies share no child
pointers with the source. Releasing both the copy and the source is therefore safe —
each collection returns its own documents and children to the pool exactly once.

---

## Pool

`Pool` is a `sync.Pool` for `*DataContainer`s.

```go
type Pool struct {
	pool sync.Pool
}
```

```go
func NewPool() *Pool
func (p *Pool) Get() *DataContainer
func (p *Pool) Put(doc *DataContainer)
func (p *Pool) Acquire(f func(*DataContainer) error) error
func (p *Pool) Walk(walker func(*DataContainer, map[int64]int32, func(DataType, ...int) unsafe.Pointer) error) (*DataContainer, error)
```

`Put` recurses into `TypeRecord` and `TypeArrayObject` slots before clearing, returning
any child documents back to the pool first. This prevents child documents from leaking
when a parent is returned. The caller must not hold references to a document (or its
children) after calling `Put` — they are cleared and reused.

`Acquire` is the recommended pattern for request handlers: get, run, return regardless
of error.

---

## Error Types

```go
var (
	ErrTypeMismatch  = fmt.Errorf("type mismatch")
	ErrBucketFull    = fmt.Errorf("container full")
	ErrIDOutOfBounds = fmt.Errorf("id out of bounds")
)
```

---

## Performance Characteristics

| Operation | Complexity | Notes |
|---|---|---|
| `Get` | O(1) | One map lookup + one slice index |
| `Set` (update) | O(1) | Map lookup + slice write |
| `Set` (insert, no holes) | O(1) amortised | Map insert + slice append |
| `Set` (insert, holes) | O(h) | h typically < 10 |
| `SetNull` | O(1) | Map update + hole append |
| `Unset` | O(1) | Map delete + hole append |
| `IsSet` / `IsNull` / `HasValue` | O(1) | Single map lookup |
| `Walk` | O(1) | Delegation only; walker cost is caller's |
| `Clear` | O(16 + m) | 16-slot sweep + map reset; m = set fields |
| path → `DataContainerKey` (cached) | O(1) | `AddressCache` lock-free read |

---

## Memory Layout

```go
// DataContainer with 3 fields: name (string), age (int), email (string, null)
nameKey,  _ := keys.Name()     // DataContainerKey: TypeString descriptor + DataPoint
ageKey,   _ := keys.Age()      // DataContainerKey: TypeInt descriptor + DataPoint
emailKey, _ := keys.Email()    // DataContainerKey: TypeString descriptor + DataPoint

// After: doc.SetString(nameKey, "Alice"), doc.SetInt(ageKey, 30), doc.SetNull(emailKey)

doc.positions = {
  int64(nameKey):  0, // name  -> strings[0]
  int64(ageKey):   0, // age  -> ints[0]
  int64(emailKey): -1, // email -> null, no slice position held
}

// *(*[]string)(doc.data[TypeString]) = ["Alice"]
// *(*[]int64)(doc.data[TypeInt])     = [30]
// doc.holes = [] // email was set directly to null, no prior value to free
```

---

## Hole Reuse Example

```go
k1, _ := container.NewDataContainerKey(container.NewDataPoint(container.TypeString, 1), 0)
k2, _ := container.NewDataContainerKey(container.NewDataPoint(container.TypeString, 2), 0)
k3, _ := container.NewDataContainerKey(container.NewDataPoint(container.TypeString, 3), 0)
k4, _ := container.NewDataContainerKey(container.NewDataPoint(container.TypeString, 4), 0)

doc.SetString(k1, "A") // strings[0]
doc.SetString(k2, "B") // strings[1]
doc.SetString(k3, "C") // strings[2]

doc.Unset(k2)
// holes = [DataContainerKey{TypeString, idx=1}]  ← index 1 is free
// strings = ["A", "B", "C"]    <- backing array unchanged; B still in memory but unreachable

doc.SetString(k4, "D") // claims hole -> strings[1]
// holes = []
// strings = ["A", "D", "C"]
```

---

## Design Rationale

### Why unsafe.Pointer instead of typed slice fields?

The schema is not known at compile time. A fixed struct with 16 named typed fields
would require a parallel lookup mechanism to map runtime field identities to struct
fields — negating the compile-time safety benefit while adding structural rigidity.
`DataContainer.data [16]unsafe.Pointer` provides dense typed storage with runtime
flexibility. The type information lives in the `DataContainerKey`/`DataPoint`, not in the
struct layout.

### Why 64-bit DataContainerKey instead of a 32-bit DataPoint key?

The high 32 bits carry the `FieldDescriptor` (type, owner schema, field index, flags).
This makes every stored value self-describing for validation without a secondary
lookup, and lets the same field coexist across schema versions or call-site constraint
overrides. The storage key is the full `DataContainerKey`.

### Why a flat -1 sentinel for null instead of encoding the null index?

`SetNull` immediately frees the position into holes, so there is no index to recover.
A flat sentinel is simpler and the hole mechanism handles reuse correctly.

### Why holes use DataPoint encoding instead of a separate Hole type?

`DataPoint` already encodes a type and a 27-bit integer — exactly what a hole needs
(the type of the freed slot and the index within the typed slice). Reusing it
eliminates a type and lets the hole scan/claim code use the same accessors as
everything else.

### Why the positions map key is the full int64(DataContainerKey)?

Type and descriptor bits are included in the key. Two fields with the same ID but
different types or descriptors produce different keys. This is correct: they live in
different typed slices (or reference different field definitions) and are logically
independent. Full key = no ambiguity.

### Why integer keys for positions instead of string keys?

Integer hashing in Go is O(1) with near-zero cost. String hashing is O(len(string))
and involves pointer chasing. At high throughput across millions of documents the
difference compounds. Integer keys are one of the primary reasons this design
outperforms `map[string]any`.

### Why pool documents?

A single `DataContainer` at steady state allocates nothing on reuse: `Clear()` resets
lengths to zero, `clear(positions)` retains bucket allocation, and the backing arrays
behind each typed slice are preserved. Every request that reuses a pooled document
skips all allocation and GC pressure entirely.

### Why `decimal` is stored as a string?

`decimal` values carry arbitrary precision and scale. Widening to `float64`
(`TypeFloat`) loses that precision. Storing the canonical decimal string
(`core/types/decimal`) preserves exactness end to end, and the schema validator
already accepts string values for `decimal` fields.

---

## Testing Considerations

### Unit Tests

1. **State transitions** — all combinations of Set -> SetNull -> Unset -> Set
2. **Hole reuse** — verify LIFO order, type matching, swap-and-pop correctness
3. **Type safety** — mismatched Get/Set operations return `ErrTypeMismatch`
4. **Append safety** — set many fields of the same type to force multiple
   reallocations; verify all previously set fields remain accessible with correct values
5. **Clear correctness** — verify all slice lengths are zero, positions is empty,
   holes is empty; verify backing array capacities are preserved
6. **Pooling** — acquire, populate, release, re-acquire; verify state is fully reset
7. **Decimal mapping** — decimal fields round-trip as canonical strings through
   Set/Get and through the schema compiler (`TypeString` / `TypeArrayString`)
8. **Concurrent access** — `Pool` and `AddressCache` are safe for concurrent use;
   `DataContainer` is not

### Property Tests (invariants that must always hold)

1. **Hole invariant** — `len(holes) + count(positions with idx ≥ 0) ≤ len(typed slice)`
   for each type
2. **Position invariant** — all `idx ≥ 0` in positions are valid indices into their
   typed slice (`idx < len(slice)`)
3. **Type invariant** — `DataContainerKey(k).Type()` matches the typed slice the index
   refers to
4. **Null invariant** — no position in `positions` with `idx = -1` has a corresponding
   entry in `holes`

---

## Frequently Asked Questions

### Q: Why not just use map[string]any?

**A:** Three compounding problems. First, every string key allocation and every `any`
box is a heap object the GC must track — under high throughput this produces continuous
GC pressure proportional to request rate. Second, nested access chains map lookups and
type assertions, each of which can fail silently or panic. Third, `map[string]any`
cannot be pooled in a way that recovers its key allocation cost. This design eliminates
all three problems.

### Q: How does this handle nested structures?

**A:** Nested fields (object/union/composite) are stored as nested `*DataContainer`s in
`TypeRecord` slots. The schema compiler resolves a path to a `DataContainerKey` at
definition time (cached in `AddressCache`), so access after warmup is a single map
lookup regardless of nesting depth.

### Q: What about arrays and typed lists?

**A:** Arrays are first-class types. `TypeArrayInt` holds `[]int64`, `TypeArrayString`
holds `[]string`, `TypeArrayBytes` holds `[][]byte`, `TypeArrayObject` holds
`[]*DataContainer`, `TypeArrayGeometry` holds `[][][]float64`, and so on. Arrays are stored
as typed Go slices and accessed without decoding. Bare or open element types map to
`TypeArrayUnknown`.

### Q: Can I mix different schema versions?

**A:** No. A schema compiler version is bound to a `CompiledSchema`; documents acquired
under one version must only be used with keys derived from that same version. Keys from
a different version may have the same bit pattern but different semantics, leading to
silent type confusion or wrong-slot access.

### Q: How do I serialise a document?

**A:** Use `Walk`, which exposes `positions` and the `slot` accessor. Iterate
`positions`, extract the `DataContainerKey`, use `.Type()` to determine which slice to read
from, and use the index to read the value. `TypeRecord` values recurse as nested
documents.

### Q: What is the maximum number of fields?

**A:** The 27-bit ID field supports up to 134,217,727 distinct identifiers per `DataType`,
and each typed slice is bounded by `identifierMask` (134,217,727) entries per container.

### Q: Is DataContainer thread-safe?

**A:** No. `DataContainer` is not thread-safe — use one document per goroutine. `Pool`,
`Collection` (as a read-only bag after build), and `AddressCache` are safe for
concurrent use. The intended pattern is: share the pool/compiled schema across all
goroutines; each goroutine acquires its own `DataContainer` for the duration of a request.
