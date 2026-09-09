---
title: Collection Internals
description: "The request path from your call to the database — ModelCollection through decorators, events, managed, polyfill, base, QueryEngine, interactor."
---
This file answers the debugging question that decides whether a bug lives in
*your code* or in the library: **when you call a method on a collection,
what exactly runs, in what order, before the database is touched — and where
do the errors you see actually come from?**

Every layer below is real code in the library, verified against source. Read
the stack first, then the per-method walkthroughs. When something misbehaves,
find which layer produced your symptom and you'll know whether to fix your
call site or file a library issue.

## Table of contents

- [The layered stack (outermost → database)](#the-layered-stack-outermost--database)
- [The request path, method group by method group](#the-request-path-method-group-by-method-group)
- [Debugging: which layer threw what?](#debugging-which-layer-threw-what)
- [Raw `base.Collection` methods](#raw-basecollection-methods)
- [ModelCollection methods (the generated `<X>s`)](#modelcollection-methods-the-generated-xs)
- [Ownership and cleanup rules](#ownership-and-cleanup-rules)

---

## The layered stack (outermost → database)

A collection handle you hold is never a single object. It is an onion of
decorators, each adding one job. The generated `<X>s` (a `ModelCollection[P]`)
is the outermost layer **only when you use the generated models**; the raw
`base.Collection` is the same onion minus the `ModelCollection` wrapper.

```
you (user code)
  │
  ▼
ModelCollection[P]   ── generated <X>s; ONLY when using models.
  │                    converts model<->document, owns id-cache,
  │                    binds results into your structs, Releases pooled docs
  ▼
[your CollectionDecorators]   ── utils.Decorators.CollectionDecorators,
  │                             applied per collection at persistence/base.go:146
  │                             (RLS / validation / audit — outermost = runs first)
  ▼
eventsCollection     ── collection/events.go. Emits Document*Start/Success/Failed
  │                    events around every CRUD call. Transaction-aware: inside a
  │                    transaction it queues emissions until commit.
  ▼
managedCollection    ── collection/managed.go. Schema/versioning & integrity:
  │                    validates, bumps _metadata_.version, sets _metadata_.updated,
  │                    optimistic-lock via params.Version, prepares queries (resolves
  │                    join/subquery physical names), sanitizes physical names out
  │                    of error messages.
  ▼
polyfillCollection   ── collection/polyfill.go. Emulates RETURNING-updates
  │                    (fetch ids → update → fetch docs) when the backend's
  │                    Capabilities.ReturnOnUpdate is false.
  ▼
baseCollection       ── collection/base.go. The real CRUD. Resolves the active
  │                    schema, owns the schema-bound document pool, runs the
  │                    QueryEngine on reads, and calls the DatabaseInteractor
  │                    inside transaction.Execute for writes.
  ▼
QueryEngine          ── core/query/engine.go. Read path only. Partitions your
  │                    query into a SQL (database) part + an in-memory residual
  │                    part, executes both, re-applies your projection.
  ▼
DatabaseInteractor   ── core/query/interactor.go. The concrete backend contract
                        (SQLite, PostgreSQL, ...). SelectDocuments/InsertDocuments/
                        UpdateDocuments/DeleteDocuments → SQL → database.
```

So the *order of responsibility* is: **model binding → your concerns →
events → integrity/validation → driver polyfill → schema+engine+interactor.**

---

## The request path, method group by method group

### Writes (CreateMany / Update / Delete)

1. `ModelCollection` (if used) converts your struct to a `Documenter`
   (`toDocumenter` → the collection's pooled `FromStruct`), then calls the raw
   collection. After the call it binds the returned document(s) back into your
   struct and `Release()`s pooled containers.
2. **Your decorators** run first (outermost). They may reject the operation
   (`CreateOne` returns `StatusFailedValidation`) or rewrite the document.
3. `eventsCollection` emits `DocumentCreateStart` (or Update/Delete start).
4. `managedCollection`:
   - Create: **validates** each document against the active schema first.
     Invalid → `StatusFailedValidation` + `ERR_VALIDATION_FAILED`, never sent
     to the DB. Valid → goes through.
   - Update: rejects empty payloads (`ERR_PERSISTENCE_EMPTY_UPDATE`),
     validates the partial (loose), injects the `_metadata_.version` increment
     and `_metadata_.updated` timestamp as computed fields, applies your
     `Version` as an optimistic-lock filter (`version = N`), and prepares the
     filter/compute (resolving subquery/join physical names).
   - Delete: prepares the filter; enforces the `unsafe` guard
     (`nil` filter without `unsafe` → `ERR_DANGEROUS_DELETE`).
5. `polyfillCollection.Update`: if you asked to return documents but the
   backend can't `RETURNING`, it runs fetch-ids → update → fetch-docs inside
   one transaction.
6. `baseCollection` resolves the active schema and executes on the
   **DatabaseInteractor inside a transaction** (a fresh implicit transaction
   unless the context already carries one via `Transact`):
   - Create → `interactor.InsertDocuments` → returns `[]*document.Document`.
   - Update → `interactor.UpdateDocuments(set, compute, filter, returning)`.
   - Delete → `interactor.DeleteDocuments(filter, unsafe)`.
7. Success → `Document*Success` event (queued until commit if transactional).

### Reads (Read)

1. `ModelCollection` (if used): consults its id-cache first for `FindByID`
   (positive hit → return immediately, negative hit → `ErrRecordNotFound`
   without touching the DB). Otherwise calls the raw collection, binds each
   returned document into your struct, `Release()`s the pooled containers, and
   re-populates the cache.
2. Your decorators → `eventsCollection.Read` emits `DocumentReadStart`.
3. `managedCollection.Read` **clones your query** (never mutates yours),
   recursively resolves every join/subquery target to its physical name,
   sets the main `Target`, forces the `_metadata_` projection into the result
   (`ensureMetadataProjection`), and forces `Pagination.IncludeTotal=true`.
4. `baseCollection.Read` resolves the active schema + document pool, attaches
   the pool to the query, and hands it to the **QueryEngine**:
   a. Partition: the `QueryPartitioner` splits your DSL into `dbQuery`
      (what the backend's `Capabilities` can run) + `postProcessingQuery`
      (the in-memory residual). Partitioning is cached by query-hash.
   b. Execute `dbQuery` via `interactor.SelectDocuments` (rows → pooled
      `document.Document`s).
   c. If the residual is non-empty: materialize rows to maps, run the
      residual (filters → sort → paginate; aggregations short-circuit), then
      re-apply **your** projection and rebuild documents.
5. Result: `ReadResult{Data, Count, Total, PaginationInfo}`.

> Read never emits from a write transaction — reads run on the current
> interactor (transactional if inside one), which `getCurrentInteractor`
> resolves from the context.

### Validation, metadata, schema, subscriptions

- `Validate(ctx, doc, partial)`: `baseCollection` resolves the current
  `DocumentValidator` and validates `doc.ToMap()` (strict or partial). Pure —
  no DB, no events, no decorators.
- `Metadata`/`Schema`: resolve the active schema from the provider, no DB.
- `Subscribe`/`Unsubscribe`: register/deregister a callback on the shared
  event emitter, filtered to this collection's name. `DocumentPool(ctx)`
  returns the schema-bound container pool owned by `baseCollection` (lazily
  compiled, rebuilt when the schema version changes, registered with the
  interactor for write-path scans).

---

## Debugging: which layer threw what?

Find your symptom in the left column; the layer and error tell you whether the
bug is in your code or the library.

| Symptom / error | Layer | Meaning |
| --- | --- | --- |
| `ERR_VALIDATION_FAILED` / `StatusFailedValidation` | managedCollection | Your document violates the active schema. **Your schema or data.** |
| `ERR_PERSISTENCE_EMPTY_UPDATE` | managedCollection | `Update` with no Set and no Compute. **Your call site.** |
| `ERR_DANGEROUS_DELETE` / `ERR_DELETE_REQUIRES_FILTER` | managedCollection / baseCollection | `Delete(nil, false)`. Pass a filter or `unsafe=true`. **Your call site.** |
| `ERR_UNIQUE_CONSTRAINT_VIOLATION`, `ERR_TYPE_MISMATCH` | interactor | The backend rejected the row. **Your data / schema types.** |
| `ERR_PERSISTENCE_INSERT/READ/UPDATE/DELETE_DOCUMENTS_FAILED` | baseCollection | The interactor call failed after your input passed all checks. **Library or driver** — look at the wrapped cause. |
| `ERR_QUERY_PARTITIONING_FAILED` | QueryEngine | The DSL couldn't be split. **Your query** is likely malformed. |
| `ERR_QUERY_DB_EXECUTION_FAILED` | QueryEngine | `SelectDocuments` failed. **Your query or driver.** |
| `ERR_PERSISTENCE_RESOLVE_SCHEMA_FAILED` | baseCollection | Schema not registered. Did you pass the schema to `Setup`/`Playground`? **Your wiring.** |
| `ERR_PERSISTENCE_RESOLVE_DOCUMENT_POOL_FAILED` | baseCollection | Container pool couldn't compile from the schema. **Your schema** (invalid). |
| `ErrRecordNotFound` | ModelCollection | Cache negative hit, or read-back came back empty. **Your id / expectations** — unless cache is stale (see cache semantics in `/guides/caching`). |
| Panics: `ErrExplicitMetadataProjectionForbidden` | managedCollection | You explicitly `Include("_metadata_")` in a projection. Let managed inject it. **Your query.** |
| `ERR_INVALID_UPDATE_PARAMS` | baseCollection / managedCollection | `Update` with `params == nil` or `Filter == nil`. **Your call site.** |
| No event fires, but data changes | eventsCollection / bus wiring | Event bus `nil` in `Setup` (`EnableEvents` false in Playground), or you subscribed to the wrong event / forgot `Unsubscribe`. **Your wiring.** |
| Events fire inside a transaction before commit | eventsCollection | Emissions are queued until commit — by design (`withEventEmission`). Not a bug. |
| Versioned `Update` unexpectedly matches 0 rows | managedCollection | Your `params.Version` doesn't match the stored `_metadata_.version` → optimistic-lock rejection. **Your version / concurrency.** |
| `Update` returns `Count > 0` but empty `Data` | polyfillCollection | Return-on-update requested, backend can't, and the final fetch failed. Update succeeded; re-read. **Driver capability, not your data.** |

General rule: **errors prefixed `ERR_PERSISTENCE_*` originate inside the
library layers; errors like `ERR_VALIDATION_*`, `ERR_QUERY_*`, and the
safety guards are usually your input's fault.** If the wrapped cause chain ends
at the interactor/driver, check the SQL/data; if it ends in a library
layer's own logic (`WithOperation("ManagedCollection.Update")` etc.), it's a
library bug — report it.

---

# All collection methods, concretely

Every method below is split into **"What happens when I ..."** (the
mechanics/ordering, useful for debugging) and **"How do I ..."** (the correct
usage). Methods are grouped by layer: the raw `base.Collection` contract, then
the `ModelCollection` surface you actually call from generated code.

## Raw `base.Collection` methods

Get the raw handle from the persistence object (the `p` returned by
`Setup`/`Playground`, a `base.Persistence`): `coll, err := p.Collection(ctx,
name)`. All methods below run the full onion (decorators → events → managed →
base → interactor) but work in `data.Documenter` terms and return pooled
documents — see ownership rules at the bottom.

### CreateOne / CreateMany

**What happens when I call `CreateOne(ctx, doc)`?**
It delegates to `CreateMany` with a one-element slice. Each document goes
through: your decorators → `DocumentCreateStart` → managed **schema
validation** (fail → `StatusFailedValidation`, never hits the DB) → an implicit
transaction → `interactor.InsertDocuments` → `DocumentCreateSuccess`. The
returned `CreateResult.Data` is the **final, enriched document** (id, metadata,
normalized types), not your input.

**How do I use it?**
```go
res, err := coll.CreateOne(ctx, doc)          // doc is a data.Documenter
switch res.Status {
case base.StatusCreated:            id := res.Data.ID()
case base.StatusFailedValidation:   issues := res.Issues
case base.StatusFailedPersistence:  // res.Error is a *common.SystemError
}
```
Check `res.Status` per document, not just the error — validation failures come
back as a status, not an error.

### Read

**What happens when I call `Read(ctx, q)`?**
Your query is cloned, join/subquery targets are resolved to physical names,
the `_metadata_` projection is forced in, and `IncludeTotal` is forced on.
Then `baseCollection` attaches the document pool and the `QueryEngine`
partitions the query: SQL-capable parts → `interactor.SelectDocuments`, the
rest → in-memory residual (filter/sort/paginate/aggregate), with your
projection re-applied last. You get `ReadResult{Data, Count, Total,
PaginationInfo}` where `Data` is a `data.DocumentSet` of pooled documents.

**How do I use it?**
```go
q := query.NewQueryBuilder().Where("status").Eq("shipped").Build()
res, err := coll.Read(ctx, &q)
for _, doc := range res.Data {
    doc.Release()               // pooled documents: release after you're done
}
```
**Ownership:** `ReadResult.Data` documents are pooled and **yours to
`Release()`**. `ModelCollection.Read` does this for you; raw `Read` does not.

### Update

**What happens when I call `Update(ctx, params)`?**
Rejects nil filter / empty payload. Validates the partial document (loose),
injects `_metadata_.version += 1` and `_metadata_.updated` as computed fields,
applies your `Version` as an optimistic-lock filter, resolves subqueries, then
runs in a transaction. If `ReturnDocument=true` and the driver can't return
updated rows, the polyfill does fetch-ids → update → fetch-docs atomically.

**How do I use it?**
```go
cu := base.NewCollectionUpdate().
    WithFilter(query.NewQueryBuilder().Where("status").Eq("pending").Build().Filters).
    SetField("status", "shipped")
res, err := coll.Update(ctx, cu)               // res.Count > 0 if matched
// optimistic locking:
cu.WithVersion(currentVersion)
// return the updated document(s):
cu.WithReturnDocument(true)
```
`Update` takes a `*base.CollectionUpdate`, not a document. Use the fluent
helpers (`SetField`, `WithComputedField`, `WithFilter`, `WithVersion`,
`WithReturnDocument`).

### Delete

**What happens when I call `Delete(ctx, filter, unsafe)`?**
The filter is prepared (subquery resolution), then run in a transaction.
`nil` filter + `unsafe=false` → `ERR_DANGEROUS_DELETE`. Returns the number of
deleted documents.

**How do I use it?**
```go
f := query.NewQueryBuilder().Where("status").Eq("cancelled").Build().Filters
n, err := coll.Delete(ctx, f, false)   // err if no filter
n, err = coll.Delete(ctx, nil, true)   // nuke the collection (be careful)
```

### Validate

**What happens when I call `Validate(ctx, doc, partial)`?**
Resolves the current `DocumentValidator` and validates `doc.ToMap()` against
the active schema. No DB, no events. Returns `([]common.Issue, bool)`.

**How do I use it?**
```go
issues, ok := coll.Validate(ctx, doc, false)      // strict
issues, ok = coll.Validate(ctx, partialDoc, true) // partial
```

### Subscribe / Unsubscribe

**What happens when I call `Subscribe(ctx, opts)`?**
Registers your callback on the shared event emitter, scoped to this collection
(via the collection-name filter in the event payload). Returns a subscription
id. `Unsubscribe` removes it.

**How do I use it?**
```go
id := coll.Subscribe(ctx, base.SubscriptionOptions{
    Event: base.DocumentCreateSuccess,
    Callback: func(ctx context.Context, ev base.PersistenceEvent) error {
        return nil
    },
})
defer coll.Unsubscribe(ctx, id)
```
**Always** `Unsubscribe` — the emitter holds a reference until you do.

### Schema / Metadata / Capabilities / DocumentPool

**What happens when I call these?** They resolve the **active** schema version
from the provider (no DB): `Schema()` → `*definition.Schema`;
`Metadata(ctx, filter, force)` → `*CollectionMetadata`; `Capabilities(ctx)` →
the backend's `*query.Capabilities` (this drives partitioning — see
`/guides/persistence-setup`); `DocumentPool(ctx)` → the schema-bound container pool (lazily compiled,
invalidated on schema version change).

**How do I use them?**
```go
sc, _ := coll.Schema(ctx)                    // current schema
caps := coll.Capabilities(ctx)               // e.g. caps.ReturnOnUpdate
pool, _ := coll.DocumentPool(ctx)            // build pooled documents
doc, _ := pool.FromStruct(&myModel, document.WithContext(ctx))
```

### Transact

**What happens when I call `Transact(ctx, fn)`?** Starts a database
transaction (or joins an existing one) and runs `fn` with a transaction-scoped
context. All writes inside use the transactional interactor; emissions are
deferred until commit. `fn` returning an error rolls back; success commits.
Full semantics (nesting, concurrency, hooks, events, errors) in
`/guides/transactions`.

**How do I use it?**
```go
_, err := coll.Transact(ctx, func(tctx context.Context) (any, error) {
    coll.CreateOne(tctx, docA)
    coll.CreateOne(tctx, docB)
    return nil, nil
})
```

---

## ModelCollection methods (the generated `<X>s`)

These all run the full raw stack *plus* the Model layer: convert struct →
pooled document, call the raw collection, **bind the returned document back
into your struct**, `Release()` pooled containers, and (if caching) update the
id-cache. The cache is keyed by `GetID()`. Any method that can't bind fails
loudly with `WithOperation("ModelCollection.<X>")` — so a binding error is a
struct/schema mismatch, **your code**.

### Create / CreateMany

**What happens when I call `Create(ctx, doc)`?** Converts `doc` via
`toDocumenter` → `raw.CreateOne` → binds the persisted result back into a
fresh `P` → caches it (if cache enabled) → returns it. The returned struct
carries the generated `_id_` and metadata.

**How do I use it?**
```go
created, err := orders.Create(ctx, orders.Order{Number: "ORD-1", Email: "a@b.c", Total: 120.00, Status: "pending"})
```
```go
created, err := orders.CreateMany(ctx, []orders.Order{...})
```

### FindByID

**What happens when I call `FindByID(ctx, id)`?** If caching is on: a positive
cache hit returns the cached model **without touching the DB**; a negative hit
returns `ErrRecordNotFound` immediately. Otherwise it issues
`Read(Where(_id_).Eq(id).Limit(1))`. A miss with an empty result **nullifies**
the id in the cache (negative caching) and returns `ErrRecordNotFound`.

**How do I use it?**
```go
o, err := orders.FindByID(ctx, "0190...")     // err == ErrRecordNotFound if absent
```

### Read / ReadAs

**What happens when I call `Read(ctx, q)`?** Runs the full raw Read, then binds
each pooled document into a fresh `P`, releasing containers as it goes, and
refills the id-cache. `ReadAs[R]` does the same but binds into projection shape
`R` and does **not** touch the model-typed cache.

**How do I use it?**
```go
q := query.NewQueryBuilder().Where("status").Eq("shipped").Build()
all, err := orders.Read(ctx, &q)
summaries, err := orders.ReadAs[*orders.OrderSummary](ctx, &q)
```

### Update / UpdateMany / Replace / UpdateFrom

**What happens when I call `Update(ctx, id, partial)`?**
Builds `CollectionUpdate{Filter: _id_=id, Set: toPartialDocumenter(partial),
ReturnDocument: true}`, merges caller `opts` (caller filter wins over id;
Compute merged; Version passed through), runs the raw Update, binds the
updated document, refreshes the cache. `UpdateMany` returns the affected count
and clears the cache. `Replace` uses the full `toDocumenter` (all fields) on
the id filter. `UpdateFrom[R,S]` binds the result into shape `S`.

**How do I use it?**
```go
updated, err := orders.Update(ctx, id, orders.OrderUpdate{Status: "shipped"})
n, err := orders.UpdateMany(ctx, filter, orders.OrderUpdate{Status: "cancelled"})
replaced, err := orders.Replace(ctx, id, orders.Order{...})
// atomic server-side increment via opts:
updated, err := orders.Update(ctx, id, orders.OrderUpdate{},
    base.CollectionUpdate{Compute: map[string]query.Query{ "total": incrQuery }})
```
Shape variant: `orders.UpdateFrom[*orders.OrderUpdate, *orders.OrderSummary](ctx, id, update)`.
Zero fields in a partial are skipped — a partial update **only sets what's
populated**.

### DeleteByID / DeleteMany

**What happens when I call `DeleteByID(ctx, id)`?** Runs raw
`Delete(_id_=id, false)`. Zero rows → `ErrRecordNotFound`. Evicts the cache.
`DeleteMany(ctx, f, unsafe)` clears the whole cache.

**How do I use it?**
```go
err := orders.DeleteByID(ctx, id)
n, err := orders.DeleteMany(ctx, filter, false)
```

### Validate / ValidatePartial

**What happens when I call `Validate(ctx, doc, loose)`?** Converts the model
to a document and runs raw `Validate` (strict or partial), releasing the
converted doc after. Returns an error wrapping any issues.

**How do I use it?**
```go
if err := orders.Validate(ctx, newOrder, false); err != nil { ... }
if err := orders.ValidatePartial(ctx, patch, true); err != nil { ... }
```

### Close

**What happens when I call `Close()`?** Stops the managed cache's background
goroutines (janitor/evictor). Idempotent. Does **not** touch the underlying
collection or database.

**How do I use it?** `defer orders.Close()` right after binding the model, when
you configured a cache.

---

## Ownership and cleanup rules (the three things that bite)

1. **Release pooled documents you hold.** Raw `ReadResult.Data` is pooled and
   yours to release; `ModelCollection` releases what it consumed internally.
   Never use a document after `Release()`.
2. **Unsubscribe; Close only if a cache exists.** Subscriptions hold emitter
   references — always `Unsubscribe`. `Close()` stops the managed cache's
   background goroutines and is **a no-op when no cache is configured** (the
   default — a cache is created only via `Cache`/`CacheConfig` options). It
   does **not** close the collection or the database. Close the store with
   `cleanup()` from `Playground`, or by closing your interactor/DB. On the
   singleton model without a cache, omitting `Close()` leaks nothing; `defer`
   it when a cache may be enabled (it's idempotent).
3. **Your queries are never mutated.** `managedCollection.Read` clones your
   query, and `ModelCollection` merges options rather than overwriting — a
   caller-supplied `Filter` overrides the id filter, and Compute maps are
   merged, not replaced.