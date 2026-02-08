# Document Specification - Final
**Version: 1.0**

## Design Philosophy

1. **Schema Opacity** - Document has no knowledge of schema internals
2. **Efficient Memory** - Minimize memory overhead and GC pressure
3. **Fast Access** - O(1) operations for common cases (get, set, delete)
4. **Explicit State** - Clear distinction between: not set, null/empty, and has value
5. **Minimal Complexity** - Only essential data structures

---

## Type System


### FieldType

```go
type FieldType uint8

const (
    TypeInt FieldType = iota
    TypeFloat
    TypeString
    TypeBool
    TypeBytes
    TypeDecimal
)
```

**Type Coverage:**
- `TypeInt` - All integer values (int8, int16, int32, int64, uint variants)
- `TypeFloat` - Floating-point values (float32, float64)
- `TypeString` - UTF-8 strings
- `TypeBool` - Boolean values
- `TypeBytes` - Variable-length data (arrays, blobs, nested structures, binary data)
- `TypeDecimal` - Fixed-point values (stored as int128 coefficient + int32 scale).

---

## Field Identification

### FieldSelector

```go
type FieldSelector int32
```

**Bit Layout: Type(3) : Depth(9) : Offset(9) : Index(9) = 30 bits total**

```
┌──────────────┬──────────┬───────────┬───────────┬───────────┐
│ Reserved(2b) │ Type(3b) │ Depth(9b) │ Offset(9b)│ Index(9b) │
└──────────────┴──────────┴───────────┴───────────┴───────────┘
     0-1            2-4        5-13      14-22       23-31
```

**Components:**

| Component | Bits | Range | Purpose |
|-----------|------|-------|---------|
| **Type** | 3 | 0-7 | Storage array identifier (int, float, string, bool, bytes) |
| **Depth** | 9 | 0-511 | Nesting level in schema (0 = root) |
| **Offset** | 9 | 0-511 | Parent's index in definition order |
| **Index** | 9 | 1-511 | Position in definition order (0 reserved for special use) |

**Constants:**

```go
const (
    typeBits   = 3
    depthBits  = 9
    offsetBits = 9
    indexBits  = 9

    typeMask   = 0x7        // 0b111
    depthMask  = 0x1FF      // 0b111111111
    offsetMask = 0x1FF
    indexMask  = 0x1FF
)
```

**Packing:**

```go
func PackSelector(typ FieldType, depth, offset, index uint16) FieldSelector {
    return FieldSelector(typ) |
           FieldSelector(depth)<<typeBits |
           FieldSelector(offset)<<(typeBits+depthBits) |
           FieldSelector(index)<<(typeBits+depthBits+offsetBits)
}
```

**Unpacking:**

```go
func (sel FieldSelector) Type() FieldType {
    return FieldType(sel & typeMask)
}

func (sel FieldSelector) Depth() uint16 {
    return uint16(sel>>typeBits) & depthMask
}

func (sel FieldSelector) Offset() uint16 {
    return uint16(sel>>(typeBits+depthBits)) & offsetMask
}

func (sel FieldSelector) Index() uint16 {
    return uint16(sel>>(typeBits+depthBits+offsetBits)) & indexMask
}
```

**Properties:**
- **Stable** - Same selector for same field across all documents
- **Opaque** - Document doesn't interpret internal structure
- **Compact** - Fits in int32 with 2 bits to spare
- **Schema-derived** - Computed by schema, not document

---

## Hole Management

### Hole Encoding

```go
type Hole int32
```

**Bit Layout: Type(3) : Index(29)**

```
┌──────────┬────────────────────────────────┐
│ Type(3b) │      Index(29 bits)            │
└──────────┴────────────────────────────────┘
   0-2              3-31
```

**Purpose:** Tracks deleted/reusable positions in storage arrays

**Constants:**

```go
const (
    holeTypeBits  = 3
    holeIndexBits = 29

    holeTypeMask  = 0x7
    holeIndexMask = 0x1FFFFFFF
)
```

**Packing:**

```go
func PackHole(typ FieldType, index int32) Hole {
    return Hole(typ) | Hole(index)<<holeTypeBits
}
```

**Unpacking:**

```go
func (h Hole) Type() FieldType {
    return FieldType(h & holeTypeMask)
}

func (h Hole) Index() int32 {
    return int32(h>>holeTypeBits) & holeIndexMask
}
```

---

## Document Structure

```go
type Document struct {
    // Storage arrays - densely packed, type-specific
    ints    []int64
    floats  []float64
    strings []string
    bools   []bool
    decimals []decimal.Decimal
    bytes   [][]byte

    // Tracking structures
    holes     []int32           // Reusable positions (Hole encoded)
    positions map[int32]int32   // Selector → index mapping
}
```

### Storage Arrays

**Independent, densely-packed arrays:**

```go
ints    []int64   // All integer fields
floats  []float64 // All float fields
strings []string  // All string fields
bools   []bool    // All boolean fields
bytes   [][]byte  // All variable-length data
```

**Properties:**
- No alignment between types - `ints[0]` and `strings[0]` are independent
- No gaps (except via holes array)
- Type-specific for cache locality and zero allocation overhead
- Grow independently based on actual usage

### Tracking Structures

#### holes []int32

**Purpose:** Track deleted positions for reuse

**Structure:**
- Array of `Hole` values (type:index encoded)
- Only stores actually deleted positions
- Typically very small (<10 entries)

**Operations:**
- **Push:** Append when field deleted
- **Pop:** Remove last matching type when inserting
- **Scan:** Linear search for matching type (fast due to small size)

**Example:**

```go
// After deleting string at index 5 and int at index 3:
holes = [
    PackHole(TypeString, 5),  // 0x00000028
    PackHole(TypeInt, 3),     // 0x00000018
]
```

#### positions map[int32]int32

**Purpose:** Fast O(1) field lookup

**Key:** FieldSelector (int32)
**Value:** Array index (int32, can be negative)

**Index Encoding:**
- **Positive (≥0):** Field has value at this index
- **Negative (<0):** Field is explicitly null/empty
  - `-1` = null at logical index 0
  - `-2` = null at logical index 1
  - etc.
- **Absent:** Field not set

**Examples:**

```go
positions = {
    0x00000009: 0,    // name at strings[0], has value
    0x00000012: -1,   // age is null (logical index 0)
    0x00001889: 5,    // bestFriend.name at strings[5], has value
}

// To recover actual null index: abs(value) - 1
// -1 → index 0
// -2 → index 1
```

---

## Field States

A field in a document can be in exactly one of three states:

| State | positions Entry | Meaning | IsSet() | IsNull() | Get() |
|-------|----------------|---------|---------|----------|-------|
| **Not Set** | Absent | Field never touched | false | false | error |
| **Null** | Negative | Explicitly set to null | true | true | zero value |
| **Has Value** | Positive | Contains actual data | true | false | actual value |

**State Transitions:**

```
Not Set ──Set(value)──→ Has Value
   ↓                        ↓
   └──SetNull()──→ Null ←──┘
                     ↓
                  Delete()
                     ↓
                  Not Set
```

---

## Core Operations

### Document Creation

```go
func NewDocument() *Document {
    return &Document{
        ints:      make([]int64, 0, 16),
        floats:    make([]float64, 0, 16),
        strings:   make([]string, 0, 16),
        bools:     make([]bool, 0, 16),
        bytes:     make([][]byte, 0, 16),
        holes:     make([]int32, 0, 4),
        positions: make(map[int32]int32, 16),
    }
}
```

**Initial Capacities:**
- Storage arrays: 16 (covers typical documents)
- Holes: 4 (typically 0-2 holes)
- Positions: 16 (matches storage)

---

### Clear (for pooling)

```go
func (d *Document) Clear() {
    // Reset slices to length 0, preserving capacity
    d.ints = d.ints[:0]
    d.floats = d.floats[:0]
    d.strings = d.strings[:0]
    d.bools = d.bools[:0]
    d.bytes = d.bytes[:0]
    d.holes = d.holes[:0]

    // Clear map
    clear(d.positions)  // Go 1.21+
    // Or for older Go:
    // for k := range d.positions {
    //     delete(d.positions, k)
    // }
}
```

**Complexity:** O(m) where m = number of set fields

---

### Get Operations

```go
func (d *Document) GetInt(sel FieldSelector) (int64, bool, error) {
    if sel.Type() != TypeInt {
        return 0, false, ErrTypeMismatch
    }

    idx, exists := d.positions[int32(sel)]
    if !exists {
        return 0, false, nil  // Not set
    }

    if idx < 0 {
        return 0, true, nil  // Explicitly null
    }

    return d.ints[idx], true, nil  // Has value
}

func (d *Document) GetFloat(sel FieldSelector) (float64, bool, error) {
    if sel.Type() != TypeFloat {
        return 0, false, ErrTypeMismatch
    }

    idx, exists := d.positions[int32(sel)]
    if !exists {
        return 0, false, nil
    }

    if idx < 0 {
        return 0, true, nil
    }

    return d.floats[idx], true, nil
}

func (d *Document) GetString(sel FieldSelector) (string, bool, error) {
    if sel.Type() != TypeString {
        return "", false, ErrTypeMismatch
    }

    idx, exists := d.positions[int32(sel)]
    if !exists {
        return "", false, nil
    }

    if idx < 0 {
        return "", true, nil
    }

    return d.strings[idx], true, nil
}

func (d *Document) GetBool(sel FieldSelector) (bool, bool, error) {
    if sel.Type() != TypeBool {
        return false, false, ErrTypeMismatch
    }

    idx, exists := d.positions[int32(sel)]
    if !exists {
        return false, false, nil
    }

    if idx < 0 {
        return false, true, nil
    }

    return d.bools[idx], true, nil
}

func (d *Document) GetBytes(sel FieldSelector) ([]byte, bool, error) {
    if sel.Type() != TypeBytes {
        return nil, false, ErrTypeMismatch
    }

    idx, exists := d.positions[int32(sel)]
    if !exists {
        return nil, false, nil
    }

    if idx < 0 {
        return nil, true, nil
    }

    return d.bytes[idx], true, nil
}
```

**Return Values:**
- `(value, isSet, error)`
- `isSet=false` → field not set (value is zero value)
- `isSet=true, value=zero` → field explicitly null
- `isSet=true, value≠zero` → field has value
- `error≠nil` → type mismatch

**Complexity:** O(1) - single map lookup

---

### Set Operations

```go
func (d *Document) SetInt(sel FieldSelector, value int64) error {
    if sel.Type() != TypeInt {
        return ErrTypeMismatch
    }

    sid := int32(sel)

    // Check if already exists
    if idx, exists := d.positions[sid]; exists {
        if idx < 0 {
            // Was null, now has value - need new slot
            d.positions[sid] = d.allocateInt(value)
        } else {
            // Update in place
            d.ints[idx] = value
        }
        return nil
    }

    // New field
    d.positions[sid] = d.allocateInt(value)
    return nil
}

func (d *Document) allocateInt(value int64) int32 {
    // Check holes for matching type
    for i := len(d.holes) - 1; i >= 0; i-- {
        hole := Hole(d.holes[i])
        if hole.Type() == TypeInt {
            // Reuse hole
            idx := hole.Index()
            d.ints[idx] = value

            // Remove hole (swap with last, then pop)
            d.holes[i] = d.holes[len(d.holes)-1]
            d.holes = d.holes[:len(d.holes)-1]

            return idx
        }
    }

    // No holes - append
    idx := int32(len(d.ints))
    d.ints = append(d.ints, value)
    return idx
}

// Similar implementations for:
// - SetFloat + allocateFloat
// - SetString + allocateString
// - SetBool + allocateBool
// - SetBytes + allocateBytes
```

**Complexity:**
- **Update existing:** O(1)
- **Insert with hole:** O(h) where h = number of holes (typically <10)
- **Insert without hole:** O(1)

**Note:** Holes are scanned backwards (LIFO) for cache locality

---

### Set Null

```go
func (d *Document) SetNull(sel FieldSelector) error {
    typ := sel.Type()
    sid := int32(sel)

    // Check if already exists
    if idx, exists := d.positions[sid]; exists {
        if idx >= 0 {
            // Had value, now null - create hole
            d.holes = append(d.holes, int32(PackHole(typ, idx)))

            // Zero out value (help GC for reference types)
            d.zeroValue(typ, idx)
        }
        // Mark as null (we use -1 for simplicity, could track actual null indices)
        d.positions[sid] = -1
        return nil
    }

    // New null field
    d.positions[sid] = -1
    return nil
}

func (d *Document) zeroValue(typ FieldType, idx int32) {
    switch typ {
    case TypeInt:
        d.ints[idx] = 0
    case TypeFloat:
        d.floats[idx] = 0
    case TypeString:
        d.strings[idx] = ""
    case TypeBool:
        d.bools[idx] = false
    case TypeBytes:
        d.bytes[idx] = nil
    }
}
```

**Complexity:** O(1)

**Purpose:**
- Distinguish between "not set" and "explicitly null"
- Important for JSON null vs undefined semantics
- Critical for sparse updates (only send changed fields)

---

### Delete

```go
func (d *Document) Delete(sel FieldSelector) {
    sid := int32(sel)

    idx, exists := d.positions[sid]
    if !exists {
        return  // Already deleted or never existed
    }

    // If had value (not null), create hole
    if idx >= 0 {
        typ := sel.Type()
        d.holes = append(d.holes, int32(PackHole(typ, idx)))
        d.zeroValue(typ, idx)
    }

    // Remove from positions
    delete(d.positions, sid)
}
```

**Complexity:** O(1)

**Effect:**
- Field becomes "not set" (absent from positions)
- Storage slot becomes reusable (added to holes)
- Value zeroed to help GC

---

### State Checks

```go
func (d *Document) IsSet(sel FieldSelector) bool {
    _, exists := d.positions[int32(sel)]
    return exists
}

func (d *Document) IsNull(sel FieldSelector) bool {
    idx, exists := d.positions[int32(sel)]
    return exists && idx < 0
}

func (d *Document) HasValue(sel FieldSelector) bool {
    idx, exists := d.positions[int32(sel)]
    return exists && idx >= 0
}
```

**Complexity:** O(1) for all

---

### Iteration

```go
func (d *Document) Walk(fn func(FieldSelector, any, bool) error) error {
    for sid, idx := range d.positions {
        sel := FieldSelector(sid)

        isNull := idx < 0

        var value any
        if !isNull {
            switch sel.Type() {
            case TypeInt:
                value = d.ints[idx]
            case TypeFloat:
                value = d.floats[idx]
            case TypeString:
                value = d.strings[idx]
            case TypeBool:
                value = d.bools[idx]
            case TypeBytes:
                value = d.bytes[idx]
            }
        }

        if err := fn(sel, value, isNull); err != nil {
            return err
        }
    }

    return nil
}
```

**Callback Signature:** `func(selector FieldSelector, value any, isNull bool) error`

**Parameters:**
- `selector` - Field identifier
- `value` - Field value (zero value if isNull=true)
- `isNull` - Whether field is explicitly null

**Complexity:** O(m) where m = number of set fields

---

## Collection

### Structure

```go
type Collection struct {
    schema *Schema  // Schema reference

    // Selector computation & caching
    selectors sync.Map  // path (string) → FieldSelector
    paths     sync.Map  // FieldSelector → path (string)

    // Document pooling
    pool sync.Pool
}
```

### Schema Interface

```go
type Schema interface {
    // Compute selector for a path
    ComputeSelector(path string) (FieldSelector, error)

    // Reverse: find path from selector.
    FindPath(sel FieldSelector) string
}
```

**Properties:**
- Schema is opaque to Document and Collection
- Only interface: path <-> selector conversion

---

### Model Operations

```go
func NewCollection(schema *Schema) *Collection {
    dm := &Collection{
        schema: schema,
    }

    dm.pool.New = func() any {
        return NewDocument()
    }

    return dm
}

func (dm *Collection) GetSelector(path string) (FieldSelector, error) {
    // Check cache
    if cached, ok := dm.selectorCache.Load(path); ok {
        return cached.(FieldSelector), nil
    }

    // Compute via schema
    selector, err := dm.schema.ComputeSelector(path)
    if err != nil {
        return 0, err
    }

    // Cache bidirectionally
    dm.selectorCache.Store(path, selector)
    dm.pathCache.Store(selector, path)

    return selector, nil
}

func (dm *Collection) GetPath(sel FieldSelector) string {
    // Check cache
    if cached, ok := dm.pathCache.Load(sel); ok {
        return cached.(string)
    }

    // Compute via schema
    path := dm.schema.FindPath(sel)

    // Cache bidirectionally
    dm.pathCache.Store(sel, path)
    dm.selectorCache.Store(path, sel)

    return path
}

func (dm *Collection) Acquire() *Document {
    doc := dm.pool.Get().(*Document)
    doc.Clear()
    return doc
}

func (dm *Collection) Release(doc *Document) {
    doc.Clear()
    dm.pool.Put(doc)
}
```

**Complexity:**
- **GetSelector (cached):** O(1)
- **GetSelector (uncached):** O(schema navigation)
- **GetPath (cached):** O(1)
- **GetPath (uncached):** O(schema navigation)

---

## Error Types

```go
var (
    ErrFieldNotSet  = errors.New("document: field not set")
    ErrTypeMismatch = errors.New("document: type mismatch")
    ErrUnknownType  = errors.New("document: unknown type")
)
```

## Performance Characteristics

| Operation | Complexity | Typical Cost | Notes |
|-----------|------------|--------------|-------|
| **Get** | O(1) | Single map lookup + array access |
| **Set (update)** | O(1) | Map lookup + array write |
| **Set (new, no holes)** | O(1) | Map insert + array append |
| **Set (new, with holes)** | O(h) | h typically <10 |
| **SetNull** | O(1) | Map update + hole append |
| **Delete** | O(1) | | Map delete + hole append |
| **IsSet** | O(1) | Single map lookup |
| **IsNull** | O(1) | Map lookup + comparison |
| **HasValue** | O(1) | Map lookup + comparison |
| **Walk** | O(m)  | m = number of set fields |
| **Clear** | O(m) | Reset slices + clear map |

**Where:**
- h = number of holes (typically 0-10)
- m = number of set fields

---

## Usage Examples

### Basic Usage

```go
// Create schema
schema := LoadSchema("person.json")

// Create model
model := NewCollection(schema)

// Get selectors (cached after first call)
nameSel, _ := model.GetSelector("name")
ageSel, _ := model.GetSelector("age")
emailSel, _ := model.GetSelector("email")

// Acquire document from pool
doc := model.Acquire()
defer model.Release(doc)

// Set values
doc.SetString(nameSel, "Alice")
doc.SetInt(ageSel, 30)
doc.SetNull(emailSel)  // Explicitly null

// Get values
name, isSet, _ := doc.GetString(nameSel)
// name = "Alice", isSet = true

email, isSet, _ := doc.GetString(emailSel)
// email = "", isSet = true (null)

phone, isSet, _ := doc.GetString(phoneSel)
// phone = "", isSet = false (not set)

// Check state
doc.IsSet(emailSel)      // true
doc.IsNull(emailSel)     // true
doc.HasValue(emailSel)   // false

doc.IsSet(phoneSel)      // false
doc.IsNull(phoneSel)     // false
```

### Iteration Example

```go
doc.Walk(func(sel FieldSelector, value any, isNull bool) error {
    path := model.GetPath(sel)

    if isNull {
        fmt.Printf("%s = null\n", path)
    } else {
        fmt.Printf("%s = %v\n", path, value)
    }

    return nil
})

// Output:
// name = Alice
// age = 30
// email = null
```

### Batch Updates

```go
// Efficient updates - reuses existing slots
doc.SetInt(ageSel, 31)      // O(1) update
doc.SetString(nameSel, "Bob")  // O(1) update

// Delete and recreate - reuses holes
doc.Delete(emailSel)
doc.SetString(emailSel, "bob@example.com")  // Reuses hole
```

### Sparse Updates (Patch Semantics)

```go
// Original document
doc.SetString(nameSel, "Alice")
doc.SetInt(ageSel, 30)
doc.SetString(emailSel, "alice@example.com")

// Apply patch - only change age and set email to null
patch := model.Acquire()
patch.SetInt(ageSel, 31)
patch.SetNull(emailSel)

// Merge patch into doc
patch.Walk(func(sel FieldSelector, value any, isNull bool) error {
    if isNull {
        doc.SetNull(sel)
    } else {
        // Type-switch based on sel.Type()
        switch sel.Type() {
        case TypeInt:
            doc.SetInt(sel, value.(int64))
        case TypeString:
            doc.SetString(sel, value.(string))
        // ... etc
        }
    }
    return nil
})

model.Release(patch)

// Result:
// name = "Alice" (unchanged)
// age = 31 (updated)
// email = null (changed from value to null)
```

---

## Design Rationale

### Why separate type-specific arrays?

**Benefits:**
1. **No interface boxing** - Values stored directly, not as interface{}
2. **Cache locality** - Related values packed together
3. **Type safety** - Compile-time guarantees via FieldSelector.Type()
4. **Memory efficiency** - No 16-byte interface overhead per value

**Trade-off:** More arrays to manage, but benefit far outweighs cost

---

### Why negative indices for null?

**Benefits:**
1. **Explicit null vs not-set distinction** - Critical for JSON/BSON semantics
2. **No extra storage** - Reuses positions map
3. **Fast null checks** - Single comparison
4. **No storage waste** - Null fields don't consume array slots

---

### Why holes array instead of scanning?

**Benefits:**
1. **Small size** - Only deleted fields, typically 0-10 entries
2. **Fast append** - O(1) on delete
3. **Fast scan** - O(h) where h is small
4. **LIFO order** - Better cache locality (recent holes reused first)

---

### Why single positions map instead of multiple?

**Benefits:**
1. **Lower overhead** - One map instead of five
2. **Simpler code** - Single lookup point
3. **Better cache locality** - All lookups hit same map
4. **Null encoding** - Negative indices eliminate need for separate null tracking

---

### Why pool documents?

**Benefits:**
1. **Reduced GC pressure** - Reuse allocations
2. **Faster creation** - No re-allocation of slices
3. **Memory stability** - Capacities preserved across reuse
4. **Production-ready** - Essential for high-throughput systems

**Usage pattern:**
```go
doc := model.Acquire()
defer model.Release(doc)
// Use doc...
```

---

## Implementation Notes

### Go-Specific Optimizations

1. **sync.Map for caches** - Lock-free reads for hot paths
2. **sync.Pool for documents** - Goroutine-safe pooling
3. **clear() for map reset** - Faster than delete loop (Go 1.21+)
4. **Slice capacity preservation** - Reuse backing arrays across Clear()

### Memory Layout

```go
// Document with 3 fields: name(string), age(int), email(string/null)

nameSel := PackSelector(TypeString, 0, 0, 1)    // 0x00000009
ageSel := PackSelector(TypeInt, 0, 0, 2)        // 0x00000012
emailSel := PackSelector(TypeString, 0, 0, 3)   // 0x0000001A

doc.positions = {
    0x00000009: 0,   // name at strings[0]
    0x00000012: 0,   // age at ints[0]
    0x0000001A: -1,  // email is null
}

doc.strings = ["Alice"]
doc.ints = [30]
doc.holes = []  // No deleted fields
```

### Hole Reuse Example

```go
// Start with 3 strings
doc.SetString(sel1, "A")  // strings[0]
doc.SetString(sel2, "B")  // strings[1]
doc.SetString(sel3, "C")  // strings[2]

// Delete middle one
doc.Delete(sel2)
// holes = [PackHole(TypeString, 1)]
// strings = ["A", "", "C"]  (zeroed but space preserved)

// Insert new string - reuses hole
doc.SetString(sel4, "D")  // strings[1] (reused!)
// holes = []
// strings = ["A", "D", "C"]
```

---

## Testing Considerations

### Unit Tests

1. **State transitions** - All combinations of set/null/delete
2. **Hole reuse** - Verify LIFO order and type matching
3. **Type safety** - Mismatched Get/Set operations
4. **Pooling** - Verify Clear() resets all state
5. **Concurrent access** - Collection cache thread-safety

### Property Tests

1. **Hole invariant** - Sum of holes + set fields ≤ array length
2. **Position invariant** - All positive indices < array length
3. **Type invariant** - positions[sel].Type() == array type

---

## Frequently Asked Questions

### Q: Why not just use map[string]any?

**A:** While simple, it has 5.8× memory overhead, 25× more GC objects, and 2× slower access. For systems processing millions of documents, this compounds dramatically.

---

### Q: How does this handle nested structures?

**A:** Nested structures are flattened into paths (e.g., "user.address.city") which the schema converts to selectors. Document only sees flat selectors.

---

### Q: What about arrays/lists?

**A:** Arrays are stored as `[]byte` (TypeBytes) with custom encoding (length-prefixed, etc.). Document treats them as opaque blobs. Schema handles array semantics.

---

### Q: Can I mix different schema versions?

**A:** Yes, as long as selectors remain stable across versions. Schema evolution is schema's responsibility, not document's.

---

### Q: How do I serialize a document?

**A:** Walk the positions map and encode based on your format (JSON, BSON, Protobuf, etc.). Schema provides path names for human-readable formats.

---

### Q: What's the maximum document size?

**A:** Limited by selector encoding:
- 512 depth levels
- 512 fields per level
- ~262k total fields (512²)

In practice, documents >1000 fields are rare.

---

### Q: Is this thread-safe?

**A:** Document is NOT thread-safe. Collection IS thread-safe (caches and pool). Use one document per goroutine, share the model.
