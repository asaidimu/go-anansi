# Refactor Spec: Lazy Allocation for `DataContainer`

**Status:** Proposed
**Target:** `container` package — `data_container.go`, `pool.go`
**Author:** [author]
**Related:** pool residency, zero‑copy decode, `Walk` deserialization

---

## 1. Summary

Make `DataContainer`'s two largest always‑present allocations — the fixed `data [16]unsafe.Pointer` array (128 B) and the eagerly‑constructed `positions` map (48 B) — **lazily allocated on first write**. Reduce the default typed‑slice capacity from 8 to 2 and route schema‑derived counts through the existing `slot(typ, size)` hint.

Net effect on 64‑bit Go:

| State | Before | After |
|---|---:|---:|
| Empty (fresh / from pool after fetch) | **~240 B** | **~64 B** |
| Written, 1 int field, default cap | 328 B | **328 B** |
| Written, 8 types, default cap | 944 B | **944 B** |
| Written, 8 types, cap 2 (sparse schema) | 944 B | **560 B** |

Empty‑container footprint drops **~73%** with **zero regression** on written containers. Sparse written containers additionally gain from the lower default cap. Dense written containers are unaffected (they grow to cap 8+ regardless).

We explicitly **reject** an `aux *auxState` pointer for `holes`+`backing` and a fused `side *sideState` design, because both grow *written* containers — the case that matters for pooled servers (see §4.6).

---

## 2. Motivation

1. **Empty is common but not free.** `Pool.Get` returns a cleared container; handlers that pass it through unmodified (filtered queries, negative paths, `Filter` views) pay 240 B each. Across a 10k‑doc batch where 30% are empty, that's ~720 KB of pure overhead per batch.
2. **The current design front‑loads cost.** Every container pays for 16 type slots and a writable map even though most containers touch 1–4 types and some touch none.
3. **Default cap 8 over‑allocates sparse schemas.** A container writing one int field allocates 64 B of backing for 8 B used. Schema hints already exist (`slot(typ, size)`), but the public `Set*`/`Append*` path never passes them.
4. **No behavioral change is needed.** All read predicates (`Get*`, `IsSet`, `IsNull`, `HasValue`, `Position`, `Length`, `Backing`) are already nil‑safe on a nil map and nil `data`. Only the write paths and `Walk` need guards.

---

## 3. Scope

**In scope:**
- `DataContainer.data` changes from inline array to `*[16]unsafe.Pointer`.
- `positions` map allocated on first write instead of at construction.
- `slot` split into `slot` (write, lazily allocates `data`) and `slotRead` (read, no nil check).
- `NewDataContainer` and `Pool.New` no longer allocate the map.
- `Clear` and `Pool.Put` guard `data == nil`.
- `Walk` ensures `positions` before invoking the walker.
- `initSlice` default capacity lowered from 8 to 2; hints remain supported.

**Out of scope:**
- `aux *auxState` for `holes`+`backing` (rejected — §4.6).
- Fused `side *sideState` (rejected — §4.6).
- Public `Set*`/`Append*` signature changes (no — hints go through `Walk`/`Reserve`).
- Changing `DataPoint`/`DataContainerKey` layouts.
- Changing `Collection`, `Pool` ownership semantics.

---

## 4. Design

### 4.1 `data` becomes a lazily‑allocated pointer

```go
type DataContainer struct {
    data      *[16]unsafe.Pointer // nil until first write
    positions map[int64]int32     // nil until first write
    holes     []DataContainerKey
    backing   []byte
}
```

Struct size drops from 184 B (rounded 192) to 64 B. `data` costs an additional 128 B on first write, but that allocation is amortized over the container's life and is retained across `Clear` for pool reuse — same as today's inline cost once written.

### 4.2 `slot` / `slotRead` split

Reads must not pay for the write path's nil check. Splitting is safe because `Get*` only reaches the slice when `positions[key] >= 0`, which proves `slot` was already called for `key.Type()`.

```go
// Write path — allocates on demand.
func (d *DataContainer) slot(typ DataType, initialSize ...int) unsafe.Pointer {
    if d.data == nil {
        d.data = new([16]unsafe.Pointer)
    }
    if d.data[typ] == nil {
        size := 2 // lowered default; see §4.5
        if len(initialSize) > 0 {
            size = initialSize[0]
        }
        d.initSlice(typ, size)
    }
    return d.data[typ]
}

// Read path — caller has already proven idx >= 0 in positions.
func (d *DataContainer) slotRead(typ DataType) unsafe.Pointer {
    return d.data[typ]
}
```

Every `GetX` swaps `d.slot(TypeX)` for `d.slotRead(TypeX)`. There are ~16 of these — mechanical change.

### 4.3 Lazy `positions` map

All read predicates are already nil‑safe (`len(nil) == 0`, `m[k]` on nil returns zero, `delete(nil, k)` is a no‑op). Only map **assignment** panics on nil, so `ensurePositions` is required in exactly two places:

```go
func (d *DataContainer) ensurePositions() {
    if d.positions == nil {
        d.positions = make(map[int64]int32, 8)
    }
}
```

Call sites:
- `AppendX` — before `d.positions[int64(key)] = idx` (every `SetX` miss funnels through here, so one site per type, or one shared helper).
- `SetNull` — before `d.positions[k] = -1`.
- `Walk` — before invoking the walker (§4.4).

`Unset` (`delete` on nil map) needs no guard. `Clear`'s `clear(d.positions)` is nil‑safe.

`NewDataContainer` and `Pool.New` drop their `make(map…)`:

```go
func NewDataContainer() *DataContainer { return &DataContainer{} }
```

and

```go
p.pool.New = func() any { return &DataContainer{} }
```

### 4.4 `Clear`, `Pool.Put`, `Walk`

**`Clear`** wraps the typed‑slice reset loop in `if d.data != nil`. `d.holes = d.holes[:0]` and `d.backing` retention are unchanged (both nil‑safe).

**`Pool.Put`** guards the recursion:

```go
if doc.data != nil {
    if ptr := doc.data[TypeArrayObject]; ptr != nil {
        children := *(*[][]*DataContainer)(ptr)
        for _, group := range children {
            for _, child := range group {
                p.Put(child)
            }
        }
    }
}
```

**`Walk`** ensures `positions` before invoking the walker. This is the only place where the previous contract ("positions is non‑nil") changes; ensuring it here keeps walkers simple and costs ~48 B only when a walk occurs (which already implies a live container):

```go
func (d *DataContainer) Walk(walker ...) (any, error) {
    d.ensurePositions()
    return walker(d.positions, d.slot)
}
```

Update the `Walk` doc comment to state that `positions` is guaranteed non‑nil for the walker.

### 4.5 Lower default capacity; preserve schema hints

`initSlice` default drops from 8 to 2. The hint channel already exists — `slot(typ, initialSize...)` — and the `Walk` deserialization path passes `schema.MinIntCount()` per the doc example. No public API change is required; deserializers that want exact sizing continue to use `slot` via `Walk`.

Optionally add an explicit pre‑fill helper if the codebase has deserializers that bypass `Walk`:

```go
// Reserve pre-sizes the slice for typ if it hasn't been allocated at >= n.
// Intended for deserializers that know their column counts up front.
func (d *DataContainer) Reserve(typ DataType, n int) {
    if n > 0 {
        d.slot(typ, n)
    }
}
```

`Reserve` is additive and non‑breaking; ship it only if there's a real caller.

### 4.6 Rejected alternatives (and why)

**`aux *auxState` for `holes`+`backing`** — Moves 48 B of cold fields behind a pointer. Empty gains another 40 B, but every written container pays **+8 B** (pointer rounding) **plus** an extra heap object **plus** a deref on `SetNull`/`Unset`/`Backing`. `Clear` retains `aux` for backing reuse, so the extra 8 B is permanent in the pool. Net negative for pooled servers.

**Fused `side *sideState`** — Collapses `positions`+`aux` into one pointer, reaching 16 B empty. But the pointee must round separately (`8 + 56 = 64` vs. 56 inline), so written containers grow, and every `positions` access acquires a nil check on the hot path (`GetInt`, `GetString`, `IsSet`, …). Net negative.

**Guideline for future optimization:** pooled servers are resident on *written* containers. Optimize the written case; only shrink empty if it's cost‑neutral on written.

---

## 5. Invariants to preserve

1. `IsSet(k) == false` ⇔ `k` absent from `positions` (or `positions == nil`).
2. `IsNull(k)` ⇔ `positions[k] < 0`.
3. `HasValue(k)` ⇔ `positions[k] >= 0`.
4. Every positive index in `positions` is a valid index into `data[key.Type()]`.
5. `holes` reflects every freed position outside of `SetNull`/`Unset` (walker responsibility, unchanged).
6. Strings returned by `GetString` alias `backing`; the container retains `backing` for its lifetime (unchanged).
7. `Clear` retains `data`, `positions`, `holes` capacity, and `backing` (unchanged).
8. `Pool.Put` recurses into `TypeArrayObject` children before `Clear` (unchanged).
9. `Walk` exposes a non‑nil `positions` map to the walker (new guarantee, enforced in `Walk`).
10. `Get*` on an absent key returns `(zero, false, nil)` without allocation (unchanged; nil map).

---

## 6. Testing plan

**Unit tests**
- Empty container: `Length()==0`, all `Get*` return `(zero, false, nil)`, `IsSet/IsNull/HasValue` false, `Backing()==nil`, no panic.
- `SetX`/`AppendX` on empty → allocates `data` and `positions`, subsequent `Get` works, `Length()==1`.
- `SetNull` on empty container → allocates map, sets `positions[k]=-1`, `IsNull` true.
- `Unset` on empty container → no panic, no allocation.
- `Clear` on a never‑written container → no panic, no allocation.
- `Walk` on empty container → walker receives non‑nil map.
- `Pool.Put` on never‑written container → no panic, recursion skipped.
- `Pool.Put` on container with `TypeArrayObject` children → children returned (unchanged).
- `Clone` of an empty source → empty target; `Clone` of a written source → equivalent target.
- `Reserve(TypeInt, 64)` followed by 64 `SetInt` calls → zero reallocation (use a slice‑growth probe if feasible; otherwise assert capacity via `Walk`).

**Fuzz / differential**
- Randomized op sequence (`Set*`, `SetNull`, `Unset`, `Clear`, `Get*`) on the refactored container vs. a baseline copy of the current implementation — assert identical observable state after each op.

**Benchmarks**
- `BenchmarkNewDataContainer` — expect 240 → 64 B and one fewer allocation.
- `BenchmarkGetInt_Hit`, `BenchmarkGetString_Hit` — expect no regression (slotRead adds no branch).
- `BenchmarkSetInt_Miss` — expect one extra alloc on first write only; amortized over a filled container, expect ≤1% regression.
- `BenchmarkWalk_Deserialize` — expect improvement from cap 2 default.
- `BenchmarkPool_Roundtrip` (get, write 1 field, put) — expect flat vs. today.

**Memory profile**
- `testing.AllocsPerRun` for empty `NewDataContainer()` → 0 → 1 alloc (struct only, 64 B).
- `runtime.ReadMemStats` before/after a 10k‑doc mixed batch to confirm the expected empty‑case savings.

---

## 7. Benefit analysis

### 7.1 Empty containers

| Component | Before | After |
|---|---:|---:|
| `DataContainer` struct | 192 | 64 |
| Empty `hmap` | 48 | 0 |
| **Total** | **240** | **64** |

**73% reduction.** Materializes for:
- `Pool.Get` → `Put` churn on negative/filtered paths.
- `Filter` views (no allocation at all today, but the source containers are cheaper).
- Deserialized docs whose fields were all skipped.

### 7.2 Written containers

**Cost‑neutral at default cap** (both 328 B for one int field at cap 8), **strictly better at cap 2** for sparse schemas (~40 B saved per written type), **unchanged** for dense schemas that grow to cap ≥8. No pointer chasing added to `Get*`. One extra allocation on the first write per container — dwarfed by the typed‑slice allocation that always follows.

### 7.3 Pool residency

Resident cost per pooled container is unchanged (328 B written, ~64 B if it has never been written since `Pool.New`). The map and `data` pointer are retained across `Clear`, so steady‑state memory matches today. The gains are on the *flow* — containers that are fetched, not written, and returned — plus the sparse‑schema cap‑2 win on the written population.

### 7.4 Non‑quantified

- Smaller struct → cheaper GC scan (1 pointer instead of 16 when empty).
- Clearer invariants: `data == nil` means "no typed storage yet"; `positions == nil` means "no fields set". Both are already the effective semantics.
- Removes a foot‑gun: `NewDataContainer` no longer eagerly allocates a map nobody may use.

---

## 8. Rollout and risk

**Risk:** Low. The nil‑safety of the read paths is already relied upon today (a `DataContainer{}` literal works for reads). The two real hazards are:

1. **Map assignment on nil** — audited: only `AppendX`, `SetNull`, and `Walk` assign. All guarded.
2. **`data` deref on nil** — audited: only `slot`, `slotRead`, `Clear`, `Pool.Put` touch `d.data`. `slot` allocates; `slotRead` is reachable only after `slot`; `Clear`/`Put` are guarded.

**Migration:** Single PR. No public API changes. `Walk`'s contract tightens (non‑nil `positions`) but is enforced internally, so callers are unaffected.

**Validation gate:** Differential fuzz must pass 10M ops. `BenchmarkGet*_Hit` must show < 2% regression. Memory profile of the 10k‑doc batch must show ≥ 60 B/doc reduction on the empty‑heavy workload.

**Rollback:** Revert the PR. No on‑disk or wire format changes — `DataContainerKey` and `DataPoint` layouts are untouched.

---

## 9. Checklist

- [ ] Change `data [16]unsafe.Pointer` → `data *[16]unsafe.Pointer`
- [ ] Add `slotRead`; rewrite all 16 `Get*` to use it
- [ ] Update `slot` with nil‑data guard and default cap 2
- [ ] Add `ensurePositions`; call from `AppendX`, `SetNull`, `Walk`
- [ ] Drop map allocation from `NewDataContainer` and `Pool.New`
- [ ] Guard `Clear`'s typed‑slice loop with `if d.data != nil`
- [ ] Guard `Pool.Put`'s `TypeArrayObject` recursion with `if d.data != nil`
- [ ] Update `Walk` doc comment (positions guaranteed non‑nil)
- [ ] Update `Walk` example in doc comment if needed (slot returns same type)
- [ ] Add/refresh unit tests per §6
- [ ] Add differential fuzz harness
- [ ] Run benchmarks and record before/after in PR description
- [ ] (Optional) Add `Reserve` if a non‑`Walk` deserializer needs hints
- [ ] Update package‑level doc comment to mention lazy allocation invariants
