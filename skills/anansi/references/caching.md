# Caching, pooling, and live collections

Read this when you're **optimizing or building reactive features**: avoiding
round-trips with the collection/container caches, the ModelCollection fast
path, release/pooling hygiene, and `LiveCollection`. Verified against the
framework source.

---

## When I want to cache data, what do I use?

Caching is built into **collections** and **containers** — you don't manage a
cache yourself; you tune one that's already in the read/write path.

### Collection cache (document cache)

A `base.Collection` optionally holds a document cache. Lookups and writes are
cache-aware, so hot documents avoid a DB round-trip. Override it:

```go
type MyCollection struct {
    base.Collection
}
func (c *MyCollection) NewDocumentContainer() base.DocumentContainer { ... }
func (c *MyCollection) GetCachedDocument(...) ... { ... }
func (c *MyCollection) SetCachedDocument(...) ... { ... }
```

- Lookups first check the cache; on a hit they return without hitting SQL.
- Writes update the cache: a successful `CreateOne` stores the created
  document; `UpdateOne` writes the new version; `DeleteOne` evicts.
- Keep `_id_` and `_rev_` in sync — stale cache entries return stale documents.

### Container cache (subcollection index)

The generated container type has a `SubCollections` map with its own cached
index. Reads that hit the container cache skip the DB entirely.

---

## What happens if I fail to Release() documents?

Document pooling is a **contained optimization**: it lives in the container /
`ModelCollection` plumbing, and it is **opt-in**. Default reads return fresh,
normal `data.Document` values — pooling only kicks in when a
`DocumentPoolRegistrar` container adopts a schema's pool.

- A correctly-written loop **returns the document to the pool** so the next
  iteration reuses the same buffer (zero-allocation hot loops).
- **If you fail to `Release()`**: for a *read-only* flow the consequences are
  usually bounded — a later pool acquisition may still see stale data, so
  treat pool-backed documents as valid only until released. For a *write* flow
  that reuses the buffer, an unreleased doc can be **overwritten in place**
  before you're done with it. In both cases the failure mode is "same buffer,
  newer contents," **not** a crash.
- Call `Release` promptly: the moment you've copied out what you need (e.g.
  onto your own stack value). The `PooledContainer`'s `Release` returns the
  pooled document so the buffer can be reused.

Rule of thumb: **default to non-pooled unless you are chasing allocation
profile in a hot loop.** If you opt in, release on every path and never retain
a pooled document beyond its read.

---

## Why is the ModelCollection the fast path?

Because it **collapses the O(DB round-trip per document) path into a single
round-trip plus in-memory joins.** Reads aren't per-relation N+1s:

- **One bulk read** pulls all matching rows for the collection into memory
  (`Pagination` when you page, or everything when you don't).
- **All references and inverse references are resolved in-memory** from the
  same dataset — no extra queries per relation.
- Each model is then `BuildRead` from the in-memory index, applying
  **projections** (see the section below) as it goes.
- Your **write changes take effect immediately** on the in-memory index, so a
  `ModelCollection` you mutated is internally consistent for follow-up reads
  without waiting on the DB.

So on read-heavy paths, `ModelCollection` lets you fan out over many related
models with a fixed, small number of queries — that's the "fast path".

### Projections: `ReadAs`, `CreateFrom`, `UpdateFrom`

Projections reshape how models are read/written. The pattern is: you declare a
variant of a model in the schema, then map the primary model's fields into the
variant.

```go
collection, _ := mcols.Setup(model.Schemas(), ...)
res, _ := collection.ReadAs(ctx, &ReadQuery{
    Projection: "summary", // the variant name from the schema
})
```

- **`ReadAs`** reads through a projection: the returned documents only carry
  the projected fields; writes that flow through `ReadAs` documents write only
  those fields back.
- **`CreateFrom` / `UpdateFrom`** copy a *source* model's scalar fields into a
  destination model, skipping the source's foreign keys. The value-level merge
  is delegated to the destination collection's generated `from*` funcs.
- **Filtering on `_id_`** still works inside a projection: `_id_` is always
  kept so `ReadAs` results are addressable.
- Projections belong to the **schema → codegen** loop: declare the variant in
  the `.schema.json`, run `anansi codegen golang`, and the generated model gets
  the projection methods. Wire the `Projection` into the `ReadQuery`.

---

## What use is LiveCollection?

`LiveCollection` is a **reactive in-memory mirror** of a collection that
tracks changes as they happen. Use it to:

- **Watch a collection from outside** a transactional flow — it emits change
  events (via its `Subscribe`) whenever the underlying collection mutates, so
  UIs and services observe the same live view without re-querying.
- **Compose against a live dataset**: it implements the same collection
  interface, so queries run against the *current* in-memory state, not a
  snapshot.
- **Keep multiple live views in sync** (e.g. per-UI live sets) — subscribe and
  forward to each view's `Set`.

Concretely:

```go
live, err := live.NewLiveCollection(ctx, baseCollection, ...)
// mutate baseCollection, then read back through live — you see the change
```

and a follower can subscribe:

```go
sub := live.Subscribe(ctx)          // receive change events
for event := range sub.Channel() {  // react to each mutation
    ...
}
```

Because the mirror applies each change as it lands, `LiveCollection` is the
building block for realtime UIs and long-running reactive services; the
cleanup contract is `Unsubscribe` (stop following) plus `Release`-style hygiene
on the underlying documents.
