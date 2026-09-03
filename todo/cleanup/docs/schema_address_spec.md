# Schema Address Space
## Specification & Implementation Guidelines

**Version 1.0**  
Companion to: *Schema IR Specification*

---

# 1. Overview

The `Address` function maps any dot-separated field path defined by a schema to a unique **27-bit integer coordinate** — its **DataPoint ordinal**. This coordinate is the stable, compile-time identity of that location in the document space the schema defines.

The address space has two fundamental properties:

- **Finite codomain**  
  The codomain is bounded at `2^27 − 1` addressable points, with `0` reserved as a sentinel.

- **Infinite domain**  
  Cyclic schemas define an infinite set of valid paths. Enumeration over the full path space is impossible.

The resolution strategy is:

1. Enumerate the **acyclic projection** of the schema contiguously from the **front** of the address space.
2. Allocate **blocks from the back** for each cyclic re-entry point.

The two regions never overlap.

Every valid path — no matter how deep into a cycle — resolves to **exactly one ordinal**.

### Key property

`Address()` is a **surjection** from an infinite path space onto a finite ordinal space.  
It is deterministic, path-length bounded, and requires **no runtime cycle detection**.

---

# 2. Core Concepts

## 2.1 Schema as a Path Blueprint

A schema defines the **complete, closed set of valid paths** for any document of its class.

Every path that can legally exist in a conforming document is statically implied by the schema. No valid path can be discovered at runtime that was not already encoded in the schema at compile time.

This means `Address()` resolves against a **closed address space**.  
The **compiler** is the sole authority on which paths exist.

---

## 2.2 Ordinal Assignment — Pre-Order DFS

Ordinals are assigned by a **pre-order depth-first traversal** of the schema graph starting at the root schema.

Within each schema, fields are visited in **UUID lexicographic order**.  
This ordering is stable across compilations given the same source.

Every node in the traversal — including **intermediate (non-leaf) fields** — receives an ordinal.

`Ordinal 0` is reserved as the sentinel for invalid or unresolvable paths.

---

## 2.3 Acyclic Projection

The **acyclic projection** of a schema is the schema graph with all **back-edges removed**.

It is always:

- finite
- fully enumerable

The **front region** of the address space is built entirely from this projection.

A **back-edge** is any field whose target schema has already been visited during the DFS.

Back-edge fields themselves receive front ordinals.  
However, their targets are **not recursed into** during front enumeration.

---

## 2.4 Blocks — The Back Region

Each unique **cyclic target schema** gets a **block** allocated from the top of the address space downward.

A block is a contiguous range of ordinals that mirrors the **acyclic subtree** of the target schema, repeated once per back-edge field that enters it.

Block size depends on the **branching factor of the target schema**, not a fixed power of two.

This eliminates waste and makes the **maximum addressable depth** an emergent property of the schema structure.

---

## 2.5 Maximum Depth as an Emergent Property

```

available = 2²⁷ − 1 − FrontSize
max_depth = floor(available / BlockSize[schema])

```

This is a **compile-time invariant**.

Documents exceeding this depth fall outside the finite address space and are rejected by the compiler.

---

# 3. Address Space Layout

The 27-bit address space is partitioned into three regions.

| Region | Range | Contents |
|------|------|------|
| Sentinel | `0` | Reserved. Returned by `Address()` on failure |
| Front region | `1 .. FrontSize` | All paths in the acyclic projection |
| Back region | `FrontSize+1 .. 2²⁷−1` | Cyclic paths |

---

## 3.1 Front Region

Ordinals `1 .. FrontSize` are assigned by **pre-order DFS** over the acyclic projection.

`FrontSize` equals the total number of paths in the acyclic projection of the root schema.

### Contiguous subtree property

For any field **F** with ordinal **N**:

```

N .. N + subtree_size(F) − 1

```

contains all paths reachable through **F**.

This enables **O(1) subtree range queries**.

---

## 3.2 Back Region

Blocks are allocated from `2²⁷ − 1` downward.

- The first block occupies the **highest addresses**
- Deeper levels occupy progressively **lower addresses**

Block assignment order:

Cyclic target schemas are processed in **UUID lexicographic order**.

### Block size

```

BlockSize[i] = AcyclicSubtreeSize[i] × BackEdgeCount[i]

```

---

## 3.3 Within a Block

Inside a block, the enumeration mirrors the acyclic subtree exactly.

```

slice_base =
BlockBase[target]
− (depth × BlockSize[target])
+ (entryOrdinal × AcyclicSubtreeSize[target])

````

`entryOrdinal` identifies which back-edge field entered the cyclic schema.

---

# 4. Compiler Data Structures

The compiler produces a **CompiledAddressSpace**.

It contains all tables needed to resolve any path in **O(path length)** time.

## 4.1 CompiledAddressSpace

```go
type CompiledAddressSpace struct {

    FieldOrdinals [128][127]uint32

    FieldNames [128]map[string]uint8

    BackEdgeOrdinal [128][127]uint8

    BlockBases [128]uint32

    BlockSize [128]uint32

    AcyclicSubtreeSize [128]uint32

    FrontSize uint32
}
````

---

# 5. Build Algorithm

The compiler builds `CompiledAddressSpace` in **three phases**.

---

## 5.1 Phase 1 — Discover

Perform DFS from the root schema.

For each field:

* If not schema-bearing → record as acyclic node
* If schema-bearing and **target not visited** → recurse
* If schema-bearing and **target visited** → record as back-edge

Outputs:

* ordered list of acyclic nodes
* set of cyclic target schemas
* back-edge lists per target

---

## 5.2 Phase 2 — Assign

Assign ordinals and block addresses.

Steps:

1. Assign front ordinals `1..N`
2. `FrontSize = N`

For each cyclic target schema:

```
BlockSize = AcyclicSubtreeSize × BackEdgeCount
BlockBases = previous_block_base − BlockSize
```

Back-edge fields receive `BackEdgeOrdinal`.

Populate `FieldNames` from `SchemaMetadata.Fields`.

### Invariant

```
FrontSize + Σ(BlockSize[i] × max_depth[i]) < 2²⁷ − 1
```

---

## 5.3 Phase 3 — Verify

All compiler invariants must be verified before emitting the address space.

---

# 6. Address() — Resolution Algorithm

`Address()` resolves a dot path to an ordinal.

Returns `0` on failure.

## Signature

```go
func Address(
    cs  *CompiledSchema,
    as  *CompiledAddressSpace,
    path string,
) uint32
```

---

## Resolution State

| Variable  | Initial | Role                |
| --------- | ------- | ------------------- |
| schemaIdx | 0       | current schema      |
| blockBase | 0       | current region base |
| depth     | 0       | cycle depth         |

---

## Per-Segment Logic

For each segment:

1. Resolve name → field index
2. Fetch ordinal
3. If final segment → return `blockBase + ordinal`
4. Otherwise follow schema

If cyclic target:

```
depth++

blockBase =
    BlockBases[target]
    − (depth × BlockSize[target])
    + (entryOrdinal × AcyclicSubtreeSize[target])
```

Otherwise:

```
blockBase += ordinal
```

---

## Full Implementation

```go
func Address(cs *CompiledSchema, as *CompiledAddressSpace, path string) uint32 {

    segments := strings.Split(path, ".")
    if len(segments) == 0 {
        return 0
    }

    var (
        schemaIdx = uint8(0)
        blockBase = uint32(0)
        depth     = uint32(0)
    )

    for i, segment := range segments {

        fieldIdx, ok := as.FieldNames[schemaIdx][segment]
        if !ok {
            return 0
        }

        ordinal := as.FieldOrdinals[schemaIdx][fieldIdx]

        if i == len(segments)-1 {
            return blockBase + ordinal
        }

        fdBase := uint32(cs.SchemaOffsets[schemaIdx]) & 0xFFFF
        fd     := cs.Descriptors[fdBase+uint32(fieldIdx)]

        if !isSchemaBearing(fd) {
            return 0
        }

        target := uint8((fd & FDMaskTargetSchema) >> 23)

        if as.BlockBases[target] != 0 {

            entryOrdinal := uint32(as.BackEdgeOrdinal[schemaIdx][fieldIdx])

            depth++

            blockBase =
                as.BlockBases[target] -
                (depth * as.BlockSize[target]) +
                (entryOrdinal * as.AcyclicSubtreeSize[target])

        } else {

            blockBase = blockBase + ordinal
        }

        schemaIdx = target
    }

    return 0
}
```

---

# 7. Worked Example

## Schema

```
Order
├── id
├── status
├── parent → Order
├── related → Order
└── lines → LineItem
    ├── product
    ├── quantity
    └── price
```

---

## Phase 1

Acyclic nodes:

```
id
status
parent
related
lines
lines.product
lines.quantity
lines.price
```

Back-edges:

```
parent
related
```

---

## Phase 2

| Ordinal | Path           |
| ------- | -------------- |
| 1       | id             |
| 2       | status         |
| 3       | parent         |
| 4       | related        |
| 5       | lines          |
| 6       | lines.product  |
| 7       | lines.quantity |
| 8       | lines.price    |

```
FrontSize = 8
AcyclicSubtreeSize[Order] = 8
BackEdgeCount[Order] = 2
BlockSize = 16
BlockBases = 134,217,719
```

---

## Example Resolutions

| Path                 | Ordinal   |
| -------------------- | --------- |
| id                   | 1         |
| lines.product        | 6         |
| parent.id            | 134217704 |
| related.id           | 134217712 |
| parent.lines.product | 134217709 |
| parent.parent.id     | 134217688 |

Each path resolves to a **unique ordinal**.

---

# 8. Compiler Invariants

The compiler must verify:

* `Ordinal 0` is never assigned
* Address space not exhausted
* Front and back regions do not overlap
* `AcyclicSubtreeSize` correct for each schema
* BackEdgeOrdinal unique per cyclic target
* FieldNames contains all fields
* All non-back-edge schema targets appear in the front enumeration
* Block bases strictly decreasing

---

# 9. Complexity & Guarantees

| Property            | Value               |
| ------------------- | ------------------- |
| Address time        | O(path length)      |
| Address allocations | Zero                |
| Build time          | O(fields × schemas) |
| Collision freedom   | Guaranteed          |
| Determinism         | Guaranteed          |
| Cycle safety        | Guaranteed          |
| Max depth           | Emergent            |

---

# 10. Implementation Guidelines

## 10.1 Build Order

Phase 1 must complete before Phase 2.

---

## 10.2 FieldNames Population

`FieldNames` must be populated from `SchemaMetadata.Fields`.

---

## 10.3 Union and Composite Fields

Union/composite fields are schema-bearing but encode target `0` in the descriptor.

Variant resolution must disambiguate the next segment.

Ambiguous paths return `0`.

---

## 10.4 Back-Edge Detection

Back-edges are determined strictly by **schema visit state during DFS**.

---

## 10.5 BlockBases Initialization

All `BlockBases` entries must initialize to `0`.

---

## 10.6 Depth Overflow

If

```
depth × BlockSize[target] < FrontSize
```

the path exceeds addressable depth.

`Address()` returns `0`.

---

## 10.7 Testing

Tests must verify:

* all acyclic paths resolve correctly
* cyclic paths at every depth resolve uniquely
* invalid paths return `0`
* ordinal → path → ordinal round-trip works
