---
title: Caching
description: "The one cache primitive (core/cache RepositoryCache), the ModelCollection id-cache, LiveCollection artifact repos, and the document Release()/pooling contract."
---
Read this when you're **deciding what to cache and how**: the single cache
primitive, the opt-in `ModelCollection` id-cache, `LiveCollection` artifact
repos, and `Release()` hygiene for pooled documents. Verified against the
source.

---

## What are the caching layers?

There is **one cache primitive** and **two consumers**. No collection holds a
document cache by default, and there is no container-level cache — if you
don't opt in, every read hits the database.

### The primitive: `core/cache.RepositoryCache[T]`

`RepositoryCache[T]` is a sharded, TTL-aware key-value store with negative
caching:

- `Get` / `GetStatus` (hit / confirmed-absent / miss), `Set` / `SetWithTTL`,
  `Nullify` (confirmed-absent marker), `Evict`, `Clear`, `Keys`, `TTL`,
  `Persist`, `Stats`, `Clone`, `Close`.
- Positive and negative entries carry independent TTLs (`PositiveTTL` /
  `NegativeTTL`, overridable per key; `cache.NoExpiration` opts out).
- Capacity is bounded by `MaxEntries` with a background watermark evictor and
  a synchronous per-shard safety net; a janitor sweeps expired entries.
  `Close()` stops the background goroutines — call it, or they leak.
- `cache.DefaultCacheConfig()` gives sane production defaults (10k entries,
  30m positive / 1m negative TTL).

You rarely touch this directly. You get one via the two consumers below.

### Consumer 1: the `ModelCollection` id-cache

`ModelCollection[P]` optionally caches **model instances keyed by document
ID**. Opt in through `ModelCollectionOptions`, which the generated
`Init<X>Model` passes through:

```go
productsModel, err := products.InitProductsModel(p, logger,
    collection.ModelCollectionOptions[*products.Product]{
        CacheConfig: &cache.CacheConfig{MaxEntries: 5000},
        AutoLoad:    true, // small collections: preload on startup
    })
```

- `Cache` takes a pre-built cache; `CacheConfig` builds a managed one;
  both nil means **no caching**. `AutoLoad` preloads everything (requires
  one of the above; only sensible for small collections).
- `FindByID` consults the cache first: a hit returns without SQL, a cached
  negative returns not-found without SQL, a miss reads through.
- Writes stay coherent: creates/updates populate the entry, deletes evict
  or nullify it, bulk writes clear the affected range.
- `Close()` on the model stops the cache's goroutines.

### Consumer 2: `LiveCollection[T]` artifact repos

`LiveCollection[T]` is a **key-value repository for compiled artifacts** —
read-through values computed from documents and keyed by a domain field
(rendered templates, compiled configs, permission sets). It embeds
`base.Collection`, so it passes anywhere a collection is expected.

```go
repo, err := collection.NewLiveRepository(ctx, collection.LiveRepositoryOptions[*CompiledTemplate]{
    Collection: coll,
    Processor:  templateProcessor{}, // DocumentProcessor: Create/Destroy
    QueryKey:   "slug",              // document field path used as cache key
    AutoLoad:   true,
})
if err != nil { /* ... */ }
defer repo.Close() // REQUIRED: stops janitor/evictor goroutines

tpl, ok := repo.Get("welcome-email") // shared instance — do not mutate
```

- `Processor` (`DocumentProcessor[T]`) compiles a `data.Documenter` into an
  artifact (`Create`), tears it down on eviction (`Destroy`), and deep-copies
  it for snapshots (`CloneState`; `Compile` is the deprecated alias of
  `Create`).
- `QueryKey` is the document field path used as the cache key; `QueryFunc`
  optionally supplies full custom queries per key instead.
- `Get` is read-through with negative caching. **`Get` returns the shared
  cached instance, not a copy** — never mutate it; state changes go through
  `Set`/`SetWithTTL`, which atomically replace the entry.
- `Set`/`Unset` are **memory-only**; they never touch the database. Writes
  through the embedded collection methods re-verify and refresh the cache.
- Realtime UIs subscribe to collection **events**
  (`Subscribe`/`Unsubscribe`); `LiveCollection` itself has no subscription
  API — pair it with the event bus and your own transport.

---

## What happens if I fail to Release() documents?

Raw reads return **pooled container-backed documents** (`document.Document`)
that are yours to `Release()`. Releasing returns the document's containers to
its schema pool for reuse (a no-op for views). The `ModelCollection` wrapper
releases internally — raw `base.Collection` access does not, so every `Read`
needs a matching `Release`.

- A correctly-written loop **releases each document** once it has copied out
  what it needs (e.g. onto its own stack value), letting the next iteration
  reuse the same buffers (zero-allocation hot loops).
- **If you fail to `Release()`**: the pool can't reuse those buffers, so
  memory pressure grows under load. Treat a pooled document as valid only
  until released — never retain one beyond its read.
- Interactors implementing `query.DocumentPoolRegistrar` accept a schema pool
  and scan rows straight into pooled documents with no map intermediate.

Rule of thumb: **release on every path.** `defer doc.Release()` right after a
successful read keeps hot loops allocation-free without leaking buffers.

---

## Why is the ModelCollection the fast path?

Three compounding effects, all in one wrapper:

1. **One shared pool.** The model embeds the collection's
   `*document.DocumentPool`, so every operation reuses the same compiled
   schema and container pool — no per-call setup, no map materialization on
   the hot path.
2. **Typed binding.** Reads bind straight from container slots into your
   struct (`BindToWithContext`, with a box-free fast path); the wrapper
   releases pooled documents internally, so callers never manage buffers.
3. **The id-cache above.** With `Cache`/`CacheConfig` set, `FindByID`
   skips the database on hits and confirmed-absent keys.

---

## Projections and caching

`ReadAs[R]` / `CreateFrom[R]` / `UpdateFrom[R]` pick a different **shape**
for one operation — the caching story doesn't change: the id-cache stores
model instances keyed by `GetID()`, and pooled-document hygiene applies the
same way. Declare shapes under `metadata.projections` and see
[Projections](/tutorial/projections) for the full workflow.

---

## Related

- [Collection internals](/reference/collection-internals) — the request path
  your cached reads travel.
- [Persistence setup](/guides/persistence-setup) — wiring collections.
- [Events & subscriptions](/guides/events-subscriptions) — realtime via the
  event bus.
