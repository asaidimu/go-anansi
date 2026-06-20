# Document Specification

## Design Philosophy

1. **Schema Opacity** — Document has no knowledge of schema internals. The schema is an opaque `interface{}`.
2. **Efficient Memory** — Pay only for slots actually used. Zero allocations on reuse after pool warmup.
3. **Fast Access** — O(1) for all field operations after warmup. Arbitrarily nested fields are O(1) because nesting is flattened into the 27-bit ID space at schema definition time, not at access time.
4. **Explicit State** — Unambiguous three-way distinction: not set, null, and has value.
5. **Poolable by Design** — `Clear()` resets without deallocating. Pool is schema-version-bound, guaranteeing structural compatibility across reuse.
6. **Deterministic Addressing** — DataPoints are derived deterministically from a schema. The same field path always produces the same DataPoint, enabling a path -> DataPoint cache that reaches a fixed point after warmup.

---

## Schema Type Mapping

This section defines how schema-level types (as defined by the meta-schema's `FieldTypeEnum`) map to Document `DataType`s. This mapping is the responsibility of the **schema compiler** — Document itself is schema-agnostic and only ever sees `DataPoint`s and typed values.

### Guiding principle

`record` and `object` are distinguished by one thing: whether the keys are known at schema definition time. If you can enumerate the keys, use `object` — it gets flattened and costs nothing at runtime. If the keys are arbitrary strings determined by data, use `record`.

---

### Primitive types

These map directly with no schema-layer involvement.

| Schema type | Document `DataType` | Go type |
|---|---|---|
| `unknown` | `TypeUnknown` | `any` |
| `string` | `TypeString` | `string` |
| `number` | `TypeFloat` | `float64` |
| `integer` | `TypeInt` | `int64` |
| `boolean` | `TypeBool` | `bool` |
| `bytes` | `TypeBytes` | `[]byte` |
| `geometry` | `TypeGeometry` | `[][]float64` |

---

### Structural types — always flattened

**`object`** — Never stored as a container. An object field `address` with sub-fields `city` and `postcode` compiles to two independent DataPoints: `address.city` (TypeString) and `address.postcode` (TypeString). The object wrapper disappears entirely. Document sees only the leaf fields.

**`composite`** — A merged field set from multiple schemas. At compile time all constituent fields are enumerated and each is assigned its own DataPoint. The composite type does not exist at Document runtime.

**`set`** — Identical to `array` at the Document layer. Set uniqueness is a schema-layer constraint enforced before values reach Document.

---

### `enum`

Enums are stored as `TypeInt`. The selected value is encoded as its **ordinal index** into the schema's `values` array (0-based). This gives fixed-size storage, cheap comparison, and 1-byte varint encoding for any enum with fewer than 64 values — which covers most practical cases. The mapping between ordinal and string label is a schema-layer concern; Document stores only the integer.

```
// Schema definition:
status: { type: enum, values: ["draft", "published", "archived"] }

// At runtime: status = "published"
 -> DataPoint{TypeInt, id=N}
 -> stored value: int64(1)  // ordinal index of "published"
```

The schema layer is responsible for ordinal -> string and string -> ordinal translation before values reach Document and after they leave it.

---

### `union`

A union's mapping depends on whether its variants are **structurally compatible** — meaning they all resolve to the same Document `DataType` after applying the mapping rules above.

**Compatible variants** — collapse to the shared concrete type. Document stores the value directly with no boxing.

```
integer | number  -> both TypeFloat (number widens integer) -> TypeFloat
object A | object B -> both flattened             -> individual DataPoints
           (schema layer uses a discriminator field to distinguish)
```

**Incompatible variants** — any union where variants map to different DataTypes. The field is stored as `TypeUnknown`. The schema layer is responsible for encoding and decoding the value, including any discriminator needed to identify the variant.

```
string | object   -> TypeString vs flattened object  -> TypeUnknown
integer | array   -> TypeInt vs TypeArray*       -> TypeUnknown
string | integer  -> TypeString vs TypeInt       -> TypeUnknown
```

The compatibility test is applied after fully resolving each variant through all other mapping rules (including enum -> TypeInt, object -> flatten, etc.).

---

### `record`

A `record` is `Record<string, T>` — a map with arbitrary string keys whose values conform to schema `T`. The keys are always runtime data by definition. If the keys were known at schema definition time, the type would be `object`, not `record`.

**`record` with no schema (`T = any`)** -> `TypeUnknown` storing `map[string]any`. Keys and values are both opaque to Document.

**`record` with schema `T`** -> `TypeRecord` storing `map[string]*DataContainer`. Each value is a typed DataContainer compiled from schema `T`; the string keys remain runtime data. Document stores the map as a boxed value in the `TypeRecord` slot.

```
// record with no schema
metadata: record
 -> DataPoint{TypeUnknown, id=N}
 -> value: map[string]any{...}

// record with schema
fields: record<Field>
 -> DataPoint{TypeRecord, id=N}
 -> value: map[string]*DataContainer{
   "name": DataContainer{...Field fields...},
   "age": DataContainer{...Field fields...},
 }
```

`TypeRecord` exists precisely for this case. It is the only use of that slot — `object` types are always flattened and never produce a `TypeRecord` value.

---

### `array`

Array element type determines which `TypeArray*` slot is used.

| Element type | Document `DataType` |
|---|---|
| `integer` | `TypeArrayInt` |
| `number` | `TypeArrayFloat` |
| `string` | `TypeArrayString` |
| `boolean` | `TypeArrayBool` |
| `bytes` | `TypeArrayBytes` |
| `enum` | `TypeArrayInt` (ordinals) |
| `geometry` | `TypeArrayGeometry` |
| `object` (known schema) | `TypeArrayObject` |
| `union` (compatible variants) | `TypeArray*` of the resolved type |
| `union` (incompatible variants) | `TypeArrayUnknown` |
| `unknown` | `TypeArrayUnknown` |
| array of arrays | `TypeArrayUnknown` |

---

### `TypeRecord` usage

`TypeRecord` (`map[string]*DataContainer`) holds schema-typed records with arbitrary runtime string keys.

Lookup within a `TypeRecord` value is O(1) Go map lookup on the string key, performed by the schema layer after retrieving the boxed map from Document. Document itself only stores and retrieves the map as an opaque `any` — it does not index into it.

---

### `TypeArrayObject` usage

`TypeArrayObject` (`[]*DataContainer`) holds ordered arrays of typed objects. Each element is a `*DataContainer` compiled from a known element schema. Element order is significant and preserved. Used for `array` fields whose element type is an `object` schema.

---

### Summary table

| Schema type | Condition | Document `DataType` |
|---|---|---|
| `unknown` | — | `TypeUnknown` |
| `string` | — | `TypeString` |
| `number` | — | `TypeFloat` |
| `integer` | — | `TypeInt` |
| `boolean` | — | `TypeBool` |
| `bytes` | — | `TypeBytes` |
| `geometry` | — | `TypeGeometry` |
| `enum` | — | `TypeInt` (ordinal index) |
| `union` | all variants same DataType | that concrete DataType |
| `union` | variants map to different DataTypes | `TypeUnknown` |
| `object` | — | Flattened -> individual DataPoints, no container |
| `composite` | — | Flattened -> individual DataPoints, no container |
| `set` | — | Same as `array` |
| `record` | no schema | `TypeUnknown` (`map[string]any`) |
| `record` | with schema `T` | `TypeRecord` (`map[string]*DataContainer`) |
| `array` | primitive element | `TypeArrayInt/Float/String/Bool/Bytes` |
| `array` | enum element | `TypeArrayInt` (ordinals) |
| `array` | geometry element | `TypeArrayGeometry` |
| `array` | object element (known schema) | `TypeArrayObject` |
| `array` | incompatible/open element | `TypeArrayUnknown` |
| array of arrays | — | `TypeArrayUnknown` |

---

## Type System

### DataType

```go
type DataType uint8

const (
  TypeUnknown    DataType = iota // any
  TypeInt              // int64
  TypeFloat             // float64
  TypeString             // string
  TypeBool              // bool
  TypeBytes             // []byte
  TypeGeometry            // [][]float64 (GeoJSON-style coordinate rings)
  TypeRecord             // map[string]*DataContainer
  TypeArrayUnknown          // []any
  TypeArrayInt            // []int64
  TypeArrayFloat           // []float64
  TypeArrayString          // []string
  TypeArrayBool           // []bool
  TypeArrayBytes           // [][]byte
  TypeArrayObject          // []*DataContainer
  TypeArrayGeometry         // [][][]float64
)
```

**Type Coverage:**

| Constant | Go type | Notes |
|---|---|---|
| `TypeUnknown` | `any` | Escape hatch for untyped values |
| `TypeInt` | `int64` | Covers all integer widths |
| `TypeFloat` | `float64` | Covers all float widths |
| `TypeString` | `string` | UTF-8 |
| `TypeBool` | `bool` | |
| `TypeBytes` | `[]byte` | Binary blobs, hashes, UUIDs, encoded payloads |
| `TypeGeometry` | `[][]float64` | Array of coordinate rings |
| `TypeRecord` | `map[string]*DataContainer` | Schema-typed record with arbitrary string keys |
| `TypeArrayUnknown` | `[]any` | Also covers array-of-arrays |
| `TypeArrayInt` | `[]int64` | |
| `TypeArrayFloat` | `[]float64` | |
| `TypeArrayString` | `[]string` | |
| `TypeArrayBool` | `[]bool` | |
| `TypeArrayBytes` | `[][]byte` | |
| `TypeArrayObject` | `[]*DataContainer` | |
| `TypeArrayGeometry` | `[][][]float64` | |

The iota values map directly to slot indices in `DataContainer.data [16]unsafe.Pointer`. There are exactly 16 types and exactly 16 slots — this is intentional and must be preserved.

---

## Field Identification

### DataPoint

```go
type DataPoint int32
```

**Bit Layout: Null(1) : Type(4) : ID(27) = 32 bits**

```
┌──────────┬────────────┬──────────────────────────────────┐
│ Null(1b) │ Type(4b) │      ID(27b)        │
└──────────┴────────────┴──────────────────────────────────┘
   0     1–4          5–31
```

**Components:**

| Component | Bits | Range | Purpose |
|---|---|---|---|
| **Null** | 1 | 0–1 | Null flag. 1 = explicitly null. Carried in the DataPoint itself so null intent survives transport. |
| **Type** | 4 | 0–15 | Maps directly to `DataType` iota. Tells Document which typed slice to use — no side-channel lookup needed. |
| **ID** | 27 | 0–134,217,727 | Unique field identifier. Schema-derived and arbitrary — Document never interprets this value. |

**Constants:**

```go
const (
  nullBits = 1
  typeBits = 4
  dataBits = nullBits + typeBits // 5 — combined shift for ID placement

  typeMask    DataPoint = 0xF    // 4 bits
  identifierMask int32   = 0x7FFFFFF // 27 bits
)
```

**Construction:**

```go
func NewDataPoint(typ DataType, id ...int32) (DataPoint, error) {
  if len(id) == 0 {
    return DataPoint(typ) << nullBits, nil
  }
  if id[0] < 0 || id[0] > identifierMask {
    return 0, ErrIDOutOfBounds
  }
  return (DataPoint(id[0]) << dataBits) | (DataPoint(typ) << nullBits), nil
}
```

**Accessor methods:**

```go
func (p DataPoint) Type() DataType {
  return DataType((p >> nullBits) & typeMask)
}

func (p DataPoint) ID() int32 {
  return int32(p>>dataBits) & identifierMask
}

func (p DataPoint) IsNull() bool {
  return p&1 == 1
}

// WithID returns a new DataPoint preserving type and null bits but with a new ID.
func (p DataPoint) WithID(id int32) (DataPoint, error) {
  if id < 0 || id > identifierMask {
    return 0, ErrIDOutOfBounds
  }
  base := p & DataPoint((1<<dataBits)-1) // preserve bits 0..4
  return base | (DataPoint(id) << dataBits), nil
}
```

**Properties:**
- **Self-describing** — type is encoded in the key. Document never needs an external type lookup to cast the correct slice.
- **Stable** — the same field in the same schema version always produces the same DataPoint.
- **Opaque to Document** — Document does not interpret the ID or its relationship to schema structure.
- **Schema-derived** — generated by schema, never by Document.
- **Deterministic** — enables a path -> DataPoint cache that converges to a fixed point after warmup, after which all field addressing is a single integer map lookup regardless of nesting depth.

**Positions map key:**
The full `int32(DataPoint)` is used as the map key, including type bits. Two fields with the same ID but different types produce different keys and are entirely independent entries. This is correct because they occupy different typed slices.

---

## Hole Management

Holes reuse `DataPoint` as their encoding. A hole stores the `DataType` of the freed slot and the slice index that is now available for reuse. No separate `Hole` type is needed.

```go
// Creating a hole when a position is freed:
hole, _ := NewDataPoint(point.Type(), sliceIndex)
dc.holes = append(dc.holes, hole)

// Claiming a hole of a given type (LIFO scan, swap-and-pop removal):
func (dc *DataContainer) claimHole(typ DataType) int32 {
  for i := len(dc.holes) - 1; i >= 0; i-- {
    if dc.holes[i].Type() == typ {
      idx := dc.holes[i].ID()
      dc.holes[i] = dc.holes[len(dc.holes)-1]
      dc.holes = dc.holes[:len(dc.holes)-1]
      return idx
    }
  }
  return -1
}
```

**Properties:**
- LIFO scan (backwards) — recently freed slots are reused first, improving cache locality.
- O(h) scan where h is the number of holes. h should be small in practice.
- Swap-and-pop removal is O(1) and avoids shifting.

---

## DataContainer

`DataContainer` is the storage engine. It holds up to 16 typed slices, one per `DataType`, accessed via `unsafe.Pointer` to avoid interface boxing. It is embedded in `Document`.

```go
type DataContainer struct {
  data   [16]unsafe.Pointer // index = DataType iota value; lazily initialised
  positions map[int32]int32  // int32(DataPoint) -> slice index (-1 = null)
  holes   []DataPoint    // freed slice positions available for reuse
}
```

### Storage Layout

Each slot in `data` corresponds to a `DataType` by its iota index. The pointer stored is a pointer **to the slice header** (`*[]T`), not to the backing array. This is critical for append safety: when `append` reallocates the backing array, it updates the slice header in place. Because `data[i]` points to the header, not into the array, the pointer remains valid after any reallocation.

```
data[TypeUnknown]    -> *[]any
data[TypeInt]      -> *[]int64
data[TypeFloat]     -> *[]float64
data[TypeString]     -> *[]string
data[TypeBool]      -> *[]bool
data[TypeBytes]     -> *[][]byte
data[TypeGeometry]    -> *[][][]float64
data[TypeRecord]     -> *[]map[string]*DataContainer
data[TypeArrayUnknown]  -> *[][]any
data[TypeArrayInt]    -> *[][]int64
data[TypeArrayFloat]   -> *[][]float64
data[TypeArrayString]  -> *[][]string
data[TypeArrayBool]   -> *[][]bool
data[TypeArrayBytes]   -> *[][][]byte
data[TypeArrayObject]  -> *[][]*DataContainer
data[TypeArrayGeometry] -> *[][][][]float64
```

Slots are **lazily initialised** on first write. An untouched slot holds a nil pointer — no allocation occurs for unused types.

**Memory cost of `data`:** `16 × 8 bytes = 128 bytes` on 64-bit systems, always, regardless of how many slots are populated.

### Slot initialisation

```go
func (dc *DataContainer) slot(typ DataType, initialSize ...int) unsafe.Pointer {
  if dc.data[typ] == nil {
    size := 8
    if len(initialSize) > 0 {
      size = initialSize[0]
    }
    dc.initSlice(typ, size)
  }
  return dc.data[typ]
}
```

`initSlice` allocates the appropriate typed slice and stores a pointer to its header in `data[typ]`. Initial capacity defaults to 8.

### Append safety

```go
// Correct — appends through the stored pointer to the header.
// If the backing array grows, the header is updated in place.
// data[TypeInt] remains valid.
ptr := (*[]int64)(dc.slot(TypeInt))
*ptr = append(*ptr, value)

// Wrong — takes a copy of the header value.
// After append with growth, the copy is stale.
// data[TypeInt] still points to the old header.
ints := *(*[]int64)(dc.slot(TypeInt))
ints = append(ints, value) // dc.data[TypeInt] is now stale if reallocation occurred
```

Always append through the pointer (`*ptr = append(*ptr, value)`), never through a copied header.

### GC safety

`unsafe.Pointer` is traced by the Go GC — as long as `DataContainer` (and therefore `Document`) is reachable, all slice headers and their backing arrays are reachable. The risk arises only if a pointer is ever widened to `uintptr`: the GC cannot trace `uintptr` values, and the backing array could be collected or moved. Never convert `data[i]` to `uintptr` except as an atomic expression within a single `unsafe.Pointer` operation.

---

## Document Structure

```go
type Document struct {
  DataContainer  
}
```

### Embedding cost

Embedding `DataContainer` is **zero runtime cost** compared to a named field. The embedded struct's fields are laid out inline within `Document`'s memory — there is no extra pointer, no heap indirection, and no wrapper allocation.

```
sizeof(Document) == sizeof(DataContainer)
```

What embedding adds beyond a named field is **method promotion**: all `DataContainer` methods (`SetInt`, `GetInt`, `Clear`, `Walk`, etc.) are accessible directly on `Document` without qualification. Callers write `doc.SetInt(point, value)` rather than `doc.data.SetInt(point, value)`. The compiler generates identical code either way — promotion is purely syntactic.

**`Clear()` scope:** The promoted `Clear()` resets `DataContainer` state only — typed slice lengths, `positions`, and `holes`. 
This is correct because the pool is schema-version-bound: every `Document` returned by a given `Collection` always has the same `schema
`.

### Creation

```go
func NewDocument() *Document {
  return &Document{
    DataContainer: *NewDataContainer(),
  }
}

func NewDataContainer() *DataContainer {
  return &DataContainer{
    positions: make(map[int32]int32),
    holes:   make([]DataPoint, 0),
  }
}
```

Typed slices inside `DataContainer` are lazily initialised on first use. Only `positions` and `holes` are allocated at construction time.

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

**Null semantics:** When a field transitions to null its current slice position is **immediately freed into holes**. The positions entry becomes `-1` and holds no index. The freed slot is available for reuse by any other field of the same type.

---

## Core Operations

### Clear (for pooling)

```go
func (dc *DataContainer) Clear() {
  clear(dc.positions) // Go 1.21+ — zeroes entries, retains bucket allocation
  dc.holes = dc.holes[:0]

  for i, ptr := range dc.data {
    if ptr == nil {
      continue
    }
    switch DataType(i) {
    case TypeUnknown:
      s := (*[]any)(ptr); *s = (*s)[:0]
    case TypeInt:
      s := (*[]int64)(ptr); *s = (*s)[:0]
    case TypeFloat:
      s := (*[]float64)(ptr); *s = (*s)[:0]
    case TypeString:
      s := (*[]string)(ptr); *s = (*s)[:0]
    case TypeBool:
      s := (*[]bool)(ptr); *s = (*s)[:0]
    case TypeBytes:
      s := (*[][]byte)(ptr); *s = (*s)[:0]
    case TypeGeometry:
      s := (*[][][]float64)(ptr); *s = (*s)[:0]
    case TypeRecord:
      s := (*[]map[string]*DataContainer)(ptr); *s = (*s)[:0]
    case TypeArrayUnknown:
      s := (*[][]any)(ptr); *s = (*s)[:0]
    case TypeArrayInt:
      s := (*[][]int64)(ptr); *s = (*s)[:0]
    case TypeArrayFloat:
      s := (*[][]float64)(ptr); *s = (*s)[:0]
    case TypeArrayString:
      s := (*[][]string)(ptr); *s = (*s)[:0]
    case TypeArrayBool:
      s := (*[][]bool)(ptr); *s = (*s)[:0]
    case TypeArrayBytes:
      s := (*[][][]byte)(ptr); *s = (*s)[:0]
    case TypeArrayObject:
      s := (*[][]*DataContainer)(ptr); *s = (*s)[:0]
    case TypeArrayGeometry:
      s := (*[][][][]float64)(ptr); *s = (*s)[:0]
    }
  }
}
```

`Clear` resets slice **lengths** to zero while preserving **capacity**. Backing arrays survive intact. Combined with pooling, a warmed `DataContainer` performs zero allocations on reuse — it overwrites existing memory.

The mutation is done through the stored pointer (`*s = (*s)[:0]`), not through a local copy. This is mandatory: taking a copy of the slice header and truncating it locally would leave `data[i]` pointing to the old untruncated header.

**Complexity:** O(16) constant sweep of `data` + O(m) for `clear(positions)` where m = number of set fields.

---

### Get Operations

```go
func (dc *DataContainer) GetInt(point DataPoint) (int64, bool, error) {
  if point.Type() != TypeInt {
    return 0, false, ErrTypeMismatch
  }
  idx, exists := dc.positions[int32(point)]
  if !exists {
    return 0, false, nil // not set
  }
  if idx < 0 {
    return 0, true, nil  // null
  }
  return (*(*[]int64)(dc.slot(TypeInt)))[idx], true, nil
}
```

All 16 types follow this identical pattern. **Complexity:** O(1) — one integer map lookup + one slice index dereference.

**Return convention:** `(value, isSet, error)`
- `false, false, nil` -> not set
- `zero, true, nil` -> explicitly null
- `value, true, nil` -> has value
- `_, _, err` -> type mismatch

---

### Set Operations

```go
func (dc *DataContainer) SetInt(point DataPoint, value int64) error {
  if point.Type() != TypeInt {
    return ErrTypeMismatch
  }
  key := int32(point)
  if idx, exists := dc.positions[key]; exists && idx >= 0 {
    // Live position — update in place, no allocation
    (*(*[]int64)(dc.slot(TypeInt)))[idx] = value
    return nil
  }
  // Try to reuse a freed position
  if idx := dc.claimHole(TypeInt); idx >= 0 {
    (*(*[]int64)(dc.slot(TypeInt)))[idx] = value
    dc.positions[key] = idx
    return nil
  }
  // No hole available — append to slice
  return dc.AppendInt(point, value)
}

func (dc *DataContainer) AppendInt(point DataPoint, value int64) error {
  ptr := (*[]int64)(dc.slot(TypeInt))
  idx := int32(len(*ptr))
  if idx >= identifierMask {
    return ErrContainerFull
  }
  *ptr = append(*ptr, value)
  dc.positions[int32(point)] = idx
  return nil
}
```

**Complexity:**
- Update existing value: O(1)
- Insert, hole available: O(h), h typically < 10
- Insert, no hole: O(1) amortised

All 16 types have corresponding `Set`, `Append`, and `Get` methods following this pattern.

---

### SetNull

```go
func (dc *DataContainer) SetNull(point DataPoint) {
  key := int32(point)
  if idx, exists := dc.positions[key]; exists && idx >= 0 {
    // Free the current slice position immediately
    hole, _ := NewDataPoint(point.Type(), idx)
    dc.holes = append(dc.holes, hole)
  }
  dc.positions[key] = -1
}
```

Setting a field null frees its slice position into holes immediately. The freed slot is available for reuse by any field of the same type. The positions entry is set to `-1` so the field is IsSet=true, IsNull=true.

**Complexity:** O(1)

---

### Unset

```go
func (dc *DataContainer) Unset(point DataPoint) {
  key := int32(point)
  if idx, exists := dc.positions[key]; exists && idx >= 0 {
    hole, _ := NewDataPoint(point.Type(), idx)
    dc.holes = append(dc.holes, hole)
  }
  delete(dc.positions, key)
}
```

Unset removes the field entirely — it becomes IsSet=false. If the field held a value, its slice position is freed into holes. If the field was null, no position is freed (null fields hold no slice index).

**Complexity:** O(1)

---

### State Checks

```go
func (dc *DataContainer) IsSet(point DataPoint) bool {
  _, exists := dc.positions[int32(point)]
  return exists
}

func (dc *DataContainer) IsNull(point DataPoint) bool {
  idx, exists := dc.positions[int32(point)]
  return exists && idx < 0
}

func (dc *DataContainer) HasValue(point DataPoint) bool {
  idx, exists := dc.positions[int32(point)]
  return exists && idx >= 0
}
```

**Complexity:** O(1) for all three.

---

### Walk (serialisation / deserialisation)

`Walk` exposes `positions` and the `slot` accessor directly to the caller. This enables zero-copy serialisation and in-place deserialisation without boxing values through `any`.

```go
func (dc *DataContainer) Walk(
  walker func(
    positions map[int32]int32,
    slot func(t DataType, initialSize ...int) unsafe.Pointer,
  ) (any, error),
) (any, error) {
  return walker(dc.positions, dc.slot)
}
```

**Serialisation example:**

```go
result, err := doc.Walk(func(positions map[int32]int32, slot func(DataType, ...int) unsafe.Pointer) (any, error) {
  ints := *(*[]int64)(slot(TypeInt))
  strings := *(*[]string)(slot(TypeString))

  for point, idx := range positions {
    p := DataPoint(point)
    if idx < 0 {
      encoder.WriteNull(p)
      continue
    }
    switch p.Type() {
    case TypeInt:
      encoder.WriteInt(p, ints[idx])
    case TypeString:
      encoder.WriteString(p, strings[idx])
    // ...
    }
  }
  return encoder.Bytes(), nil
})
```

**Deserialisation example:**

```go
doc.Clear()
doc.Walk(func(positions map[int32]int32, slot func(DataType, ...int) unsafe.Pointer) (any, error) {
  // Pre-allocate to schema minimum counts to avoid appends during decode
  ints := (*[]int64)(slot(TypeInt, schema.MinIntCount()))
  for decoder.HasInt() {
    point, value, index := decoder.NextInt()
    if index < int32(len(*ints)) {
      (*ints)[index] = value
      positions[int32(point)] = index
    } else {
      doc.AppendInt(point, value)
    }
  }
  return nil, nil
})
```

**Warning:** `Walk` grants direct mutable access to `positions` and the slice headers. The caller is responsible for maintaining invariants: all positive indices in `positions` must be valid indices into their respective typed slice, and holes must reflect any positions freed outside of the normal `Unset`/`SetNull` path.

**Complexity:** O(1) — Walk itself is a direct delegation. Cost is entirely determined by what the walker does.

---

## Collection

`Collection` binds a schema to a pool of `Document` instances and maintains caches for path < -> DataPoint resolution.

### Structure

```go
type Collection struct {
  schema interface{} // opaque schema reference

  // DataPoint computation and path resolution caches
  // Lock-free reads via sync.Map for concurrent hot paths
  selectors sync.Map // string path -> DataPoint
  paths   sync.Map // DataPoint  -> string path

  // Document pool — schema-version-bound
  pool sync.Pool
}
```

### Schema Interface

```go
type Schema interface {
  // ComputeDataPoint derives a stable DataPoint for the given field path.
  ComputeDataPoint(path string) (DataPoint, error)

  // FindPath resolves a DataPoint back to its field path.
  FindPath(point DataPoint) string
}
```

Document and Collection treat the schema as fully opaque beyond this interface. The schema is responsible for all DataPoint generation, versioning, and path semantics.

---

### Collection Operations

```go
func NewCollection(schema interface{}) *Collection {
  c := &Collection{schema: schema}
  c.pool.New = func() any {
    return NewDocument("", schema)
  }
  return c
}

func (c *Collection) GetDataPoint(path string) (DataPoint, error) {
  if cached, ok := c.selectors.Load(path); ok {
    return cached.(DataPoint), nil
  }
  point, err := c.schema.(Schema).ComputeDataPoint(path)
  if err != nil {
    return 0, err
  }
  c.selectors.Store(path, point)
  c.paths.Store(point, path)
  return point, nil
}

func (c *Collection) GetPath(point DataPoint) string {
  if cached, ok := c.paths.Load(point); ok {
    return cached.(string)
  }
  path := c.schema.(Schema).FindPath(point)
  c.paths.Store(point, path)
  c.selectors.Store(path, point)
  return path
}

func (c *Collection) Acquire(id string) *Document {
  doc := c.pool.Get().(*Document)
  doc.Clear()   // reset DataContainer state only; schema is invariant
  doc.id = id   // assign identity for this use
  return doc
}

func (c *Collection) Release(doc *Document) {
  doc.Clear()
  c.pool.Put(doc)
}
```

**Cache convergence:** After the first request that touches every field in a schema, both `selectors` and `paths` caches are fully populated. All subsequent requests perform only sync.Map reads, which are lock-free. The cache is effectively a compile-once lookup table per schema at runtime.

**Pool behaviour:** `sync.Pool` objects may be collected between GC cycles. Under sustained throughput the pool stays warm and setup cost is never paid again. Under bursty traffic a quiet period followed by a spike may pay one round of allocation before the pool re-warms. Per-endpoint pools (one `Collection` per schema) prevent capacity mismatch between schemas.

**Complexity:**
- `GetDataPoint` (cached): O(1) lock-free read
- `GetDataPoint` (uncached): O(schema navigation), cached on return
- `GetPath` (cached): O(1) lock-free read
- `Acquire` / `Release`: O(16) for `Clear` sweep + O(m) for map reset

---

## Error Types

```go
var (
  ErrTypeMismatch = errors.New("document: type mismatch")
  ErrContainerFull = errors.New("document: container full")
  ErrIDOutOfBounds = errors.New("document: id out of bounds")
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
| `GetDataPoint` (cached) | O(1) | Lock-free sync.Map read |
| `GetDataPoint` (uncached) | O(schema) | Cached on return |

---

## Memory Layout

```go
// Document with 3 fields: name (string), age (int), email (string, null)

nameSel, _ := collection.GetDataPoint("name")  // e.g. DataPoint encoding TypeString + id=1
ageSel, _  := collection.GetDataPoint("age")  // e.g. DataPoint encoding TypeInt  + id=2
emailSel, _ := collection.GetDataPoint("email") // e.g. DataPoint encoding TypeString + id=3

// After: doc.SetString(nameSel, "Alice"), doc.SetInt(ageSel, 30), doc.SetNull(emailSel)

doc.positions = {
  int32(nameSel): 0, // name  -> strings[0]
  int32(ageSel):  0, // age  -> ints[0]
  int32(emailSel): -1, // email -> null, no slice position held
}

// *(*[]string)(doc.data[TypeString]) = ["Alice"]
// *(*[]int64)(doc.data[TypeInt])   = [30]
// doc.holes = [] // email was set directly to null, no prior value to free
```

---

## Hole Reuse Example

```go
p1, _ := NewDataPoint(TypeString, 1)
p2, _ := NewDataPoint(TypeString, 2)
p3, _ := NewDataPoint(TypeString, 3)
p4, _ := NewDataPoint(TypeString, 4)

doc.SetString(p1, "A") // strings[0]
doc.SetString(p2, "B") // strings[1]
doc.SetString(p3, "C") // strings[2]

doc.Unset(p2)
// holes = [DataPoint{TypeString, id=1}] ← index 1 is free
// strings = ["A", "B", "C"]       <- backing array unchanged; B still in memory but unreachable

doc.SetString(p4, "D") // claims hole -> strings[1]
// holes = []
// strings = ["A", "D", "C"]
```

---

## Design Rationale

### Why unsafe.Pointer instead of typed slice fields?

The schema is not known at compile time. A fixed struct with 16 named typed fields would require a parallel lookup mechanism to map runtime field identities to struct fields — negating the compile-time safety benefit while adding structural rigidity. `DataContainer` with `[16]unsafe.Pointer` provides the same dense typed storage with runtime flexibility. The type information lives in `DataPoint`, not in the struct layout.

### Why embed DataContainer in Document rather than a named field?

Zero cost, and method promotion removes qualification noise at call sites. `doc.SetInt(...)` reads more cleanly than `doc.data.SetInt(...)` with no trade-off. `Clear()` scope is unambiguous because `id` and `schema` are pool-invariant.

### Why a flat -1 sentinel for null instead of encoding the null index?

SetNull immediately frees the position into holes, so there is no index to recover — the slot is already reused or available. A flat sentinel is simpler and the hole mechanism handles reuse correctly.

### Why holes use DataPoint encoding instead of a separate Hole type?

`DataPoint` already encodes type and a 27-bit integer. A hole needs exactly those two pieces of information: the type of the freed slot and the index within the typed slice. Reusing `DataPoint` eliminates a type, reduces cognitive load, and means the hole scan and claim code uses the same accessors as everything else.

### Why the positions map key is the full int32(DataPoint)?

Type bits are included in the key. Two fields with the same 27-bit ID but different types produce different keys. This is correct: they live in different typed slices, they are logically independent fields, and collapsing them to the same key would require additional disambiguation on every lookup. Full key = no ambiguity.

### Why integer keys for positions instead of string keys?

Integer hashing in Go is O(1) with near-zero cost — the hash is essentially the key itself passed through a fast mixer. String hashing is O(len(string)) and involves pointer chasing. At high throughput across millions of documents the difference compounds. DataPoints as integer keys are one of the primary reasons this design outperforms `map[string]any`.

### Why pool documents?

A single `Document` at steady state allocates nothing on reuse: `Clear()` resets lengths to zero, `clear(positions)` retains bucket allocation, and the backing arrays behind each typed slice are preserved. Every request that reuses a pooled document skips all allocation and GC pressure entirely. For high-throughput API endpoints this is the difference between a flat GC profile and one that grows with request rate.

---

## Testing Considerations

### Unit Tests

1. **State transitions** — all combinations of Set -> SetNull -> Unset -> Set
2. **Hole reuse** — verify LIFO order, type matching, swap-and-pop correctness
3. **Type safety** — mismatched Get/Set operations return ErrTypeMismatch
4. **Append safety** — set many fields of the same type to force multiple reallocations; verify all previously set fields remain accessible with correct values
5. **Clear correctness** — verify all slice lengths are zero, positions is empty, holes is empty; verify backing array capacities are preserved
6. **Pooling** — acquire, populate, release, re-acquire; verify state is fully reset
7. **Concurrent access** — Collection cache (GetDataPoint, GetPath) is safe for concurrent use; Document is not

### Property Tests (invariants that must always hold)

1. **Hole invariant** — `len(holes) + count(positions with idx ≥ 0) ≤ len(typed slice)` for each type
2. **Position invariant** — all `idx ≥ 0` in positions are valid indices into their typed slice (`idx < len(slice)`)
3. **Type invariant** — `DataPoint(key).Type()` matches the typed slice the index refers to
4. **Null invariant** — no position in `positions` with `idx = -1` has a corresponding entry in `holes`

---

## Frequently Asked Questions

### Q: Why not just use map[string]any?

**A:** Three compounding problems. First, every string key allocation and every `any` box is a heap object the GC must track — under high throughput this produces continuous GC pressure proportional to request rate. Second, nested access like `data["user"].(map[string]any)["address"]` chains map lookups and type assertions, each of which can fail silently or panic. Third, `map[string]any` cannot be pooled in a way that recovers its key allocation cost — `clear(m)` retains buckets but you still pay for string key hashing and interface boxing on every write. This design eliminates all three problems.

### Q: How does this handle nested structures?

**A:** Nested fields are addressed by DataPoints whose 27-bit IDs are derived from their full path at schema definition time. From Document's perspective there is no nesting — every field is a flat integer key regardless of depth. The schema is responsible for encoding path structure into IDs. A path -> DataPoint cache in `Collection` means accessing `user.address.city` is a single map lookup after warmup, identical in cost to accessing a top-level field.

### Q: What about arrays and typed lists?

**A:** Arrays are first-class types. `TypeArrayInt` holds `[]int64` values, `TypeArrayString` holds `[]string` values, `TypeArrayBytes` holds `[][]byte` values, `TypeArrayObject` holds `[]*DataContainer` values, `TypeArrayGeometry` holds `[][][]float64` values, and so on. There is no opaque byte encoding — arrays are stored as typed Go slices and accessed without decoding. Array-of-arrays schemas map to `TypeArrayUnknown`, which is the general escape hatch for element types that have no dedicated slot.

### Q: Can I mix different schema versions?

**A:** No. A `Collection` is bound to a specific schema and semantic version. A `Document` acquired from a `Collection` must only be used with DataPoints derived from that same schema version. DataPoints from a different version may have the same bit pattern but different semantics, leading to silent type confusion or wrong-slot access.

### Q: How do I serialise a document?

**A:** Use `Walk`, which exposes `positions` and the `slot` accessor. Iterate `positions`, extract the DataPoint from the key, use `.Type()` to determine which slice to read from, and use the index to read the value. The schema's `FindPath` resolves DataPoints back to human-readable field names for text formats like JSON.

### Q: What is the maximum number of fields?

**A:** The 27-bit ID field supports up to 134,217,727 distinct field identifiers per DataType. In practice the limit is the capacity of the typed slices, bounded by `identifierMask` (134,217,727) per type. Documents with more than a few thousand fields are uncommon.

### Q: Is Document thread-safe?

**A:** No. `Document` is not thread-safe — use one document per goroutine. `Collection` is thread-safe — `sync.Map` caches allow concurrent reads from multiple goroutines, and `sync.Pool` is goroutine-safe. The intended pattern is: share one `Collection` across all goroutines handling a given endpoint; each goroutine acquires its own `Document` for the duration of a request.
