# Container-Backed Document Refactor

Status: **Proposed — for review before code**

## 1. Goal

Replace the map-backed `data.Document` / `[]map[string]any` representation with the
container-backed `document.Document` down the entire persistence chain:

```
collection/ModelCollection → engine (QueryEngine) → interactor (native/ephemeral) → result
```

Wire type at the engine/interactor boundary is `document.Document`. Two
representations: **container-backed** (schema-addressed, pooled, `Release()`
returns containers to the per-schema `container.Pool`) and **record-view**
(map-backed, `Release` no-op) for shape-changing query outputs. Compiled schemas
and pools are maintained **per real schema, never per query shape**.

All container-backed documents are constructed via a `*document.Collection`
(owns the compiled schema + pool, cached once per real schema). All consumers
that stop owning a document release it.

## 2. Non-negotiable rules

1. **Pooled + auto-release**: any document built from a pool must be released by
   whoever consumes it last. `ModelCollection` releases after binding; the engine
   releases post-processing intermediates; `SelectStream` documents are released
   by the reader.
2. **No map round-trips** on the hot path: results flow as `*document.Document`,
   never converted to `map[string]any` just to be re-wrapped. Native row-scans
   write directly into pooled containers (`ReadRows`); the executor's
   `map[string]any` return type is eliminated.
3. **`document.Collection` is the only container constructor** (`newDocument`,
   `New`, `FromMap`, `FromStruct`, `Patch`). It is built once per real schema
   and cached (name+version keyed). Record-view docs have no schema/pool.
4. **Post-processing must not allocate per record**: Filter/Sort/Paginate mutate
   or slice the same `[]*document.Document`; Project allocates a new doc per
   output and releases the source.
5. `data.Documenter` remains the read/write interface everywhere (query results,
   `DocumentSet`, `CollectionUpdate.Set`). `document.Document` already satisfies it.

## 3. Current state (the map seams)

| Seam | Location | Today |
|---|---|---|
| `DatabaseInteractor` | `core/query/interactor.go:102` | `[]map[string]any` in/out |
| `RawQueryResult.Data` | `core/query/interactor.go:79` | `any` |
| `QueryResult.Data` | `core/query/dsl.go:343` | `[]map[string]any` |
| Engine post-processing | `core/query/engine.go:184` | `[]map[string]any` |
| `QueryHelper` | `core/query/helper.go` | `[]map[string]any` everywhere |
| `base.Collection.Read` | `core/persistence/collection/base.go:131` | `data.NewDocumentSet(docs.Data, ctx)` re-wraps maps |
| `base.Collection.CreateOne` | `base.go:112` | `interactor.InsertDocuments(ctx, sc, values)` map batch |
| `base.Collection.Update` | `base.go:170` | `updatesMap` + map results |
| `managedCollection.CreateMany` | `managed.go:77` | `data.NewDocument(doc)` validation |
| Native interactor | `core/query/native/interactor.go:74,104,152` | `[]map[string]any` |
| Ephemeral interactor | `core/ephemeral/interactor.go:31` | `[]map[string]any` |
| Bind bridge | `core/document/bind.go:58` | container → `data.Document` → struct |
| Migration | `core/persistence/migration/migration.go:75,108` | `SelectDocuments` → map batch → `InsertDocuments` |
| ModelCollection | `core/persistence/collection/model_collection.go` | map-backed `data.Document` results |

## 4. New types and signatures

### 4.1 Per-schema collection registry — keyed by REAL schema, not query

`*document.Collection` = `{ cs *definition.CompiledSchema; pool *container.Pool }`.
Compiled schemas and pools are maintained **per real schema, never per query
shape**. There is no per-query result-schema cache.

```go
// core/document/result.go
var collections sync.Map // key: schema name + version -> *Collection

// CollectionFor returns (cached) a Collection bound to s — enrich + compile +
// link + pool, done once per real schema. Keyed by name+version so migration
// invalidates it.
func CollectionFor(s *definition.Schema) (*Collection, error)
```

Rationale (cost): `SchemaFromQuery` produces a distinct schema per query shape,
and `generateResultSchemaName` (schema.go:423) only distinguishes operation
*kinds* (`products_result`, `products_joined_result`, ...) — two different
projections collide on the same name, so a name-keyed cache is wrong and a
correct shape-keyed cache needs a structural fingerprint + LRU eviction, plus a
fresh `container.Pool` per shape (which never reaches steady-state cardinality).
Bounding compiled schemas to the number of real schemas avoids all of that.
`SchemaFromQuery` leaves the hot path entirely.

Two-tier result documents, wire type stays `*document.Document`:

| Result shape | Representation | Construction | Cost |
|---|---|---|---|
| Subset of one real schema (plain read, filter/sort/paginate, DB-side projection, create/update/insert returns) | **container-backed**, pooled | materialize against `CollectionFor(targetSchema)` — unreturned columns are absent slots | cached once per schema, ~zero per query |
| Shape-changing (joins, aggregations, projection aliases/computed) | **record-view** (map-backed `*document.Document`) | wrap driver row map | no compile, no pool, no cache |

### 4.2 Public record-view constructor

The record fallback uses the existing `Document.record map[string]any` view
(document.go:61): `isRecord()` (document.go:144) routes `Get`/`Set`/`GetNested`/
`SetNested`/`ToMap` through the map, and record views carry `pool = nil` so
`Release()` is already a no-op. Today `newRecordView` (document.go:137) is
unexported (used by nested record fields, normalize, sanitize, merge). Expose a
public top-level constructor for executor wrapping:

```go
// core/document/record.go
// FromRecord wraps a schema-free row map as a record-view document. Release is
// a no-op (no pool); the caller must not mutate the map after handing it over.
func FromRecord(m map[string]any, ctx ...context.Context) *Document
```

Note: record-view `ID()` returns `""` (document.go:152) — fine for join/
aggregation rows, which have no single identity and are not model-bound
(`ModelCollection` reads are container mode).

**Memory tradeoff (deliberate)**: record-mode allocates a `map[string]any` per
row, container-mode reuses pooled containers. This is not a regression — today
*every* query result is `[]map[string]any`; the two-tier design moves plain
reads onto pools (reduction) and leaves shape-changing queries at the current
allocation level. Acceptable because computed projections are inherently
schema-free, join rows are ephemeral/single-use (no pooling payoff, and each
distinct join shape would need its own compiled schema + pool), and shape-
changing queries are typically lower cardinality than bulk reads. Their map
memory is also GC-able, whereas pools retain capacity — favorable for
long-running servers.

Optional follow-ups if profiling shows shape-changing queries matter:
- **Generic record pool**: `sync.Pool` of `map[string]any` reused via `clear()`
  — cuts record-mode allocs without per-shape compilation.
- **Bounded join LRU**: cache ~64 compiled join-result schemas + pools for
  container-backed join results (join shapes form a small bounded set in
  practice).

### 4.3 Interactor interface (`core/query/interactor.go:102`)

```go
type DatabaseInteractor interface {
    // Read
    SelectDocuments(ctx context.Context, schema *definition.Schema, dsl *Query) ([]*document.Document, int64, error)
    SelectStream(ctx context.Context, sc *definition.Schema, dsl *Query) (<-chan *document.Document, <-chan error, error)
    // Write
    UpdateDocuments(ctx context.Context, schema *definition.Schema,
        updates data.Documenter, computedUpdates map[string]Query, filters *QueryFilter, returning bool) ([]*document.Document, int64, error)
    InsertDocuments(ctx context.Context, sc *definition.Schema, records []data.Documenter) ([]*document.Document, error)
    DeleteDocuments(ctx context.Context, schema *definition.Schema, filters *QueryFilter, unsafeDelete bool) (int64, error)
    // ... unchanged: SchemaManager methods, Capabilities, Transact helpers
}
```

Notes:
- `updates map[string]any` → `data.Documenter` (callers already hold `params.Set`,
  which is a `Documenter`).
- `records []map[string]any` → `[]data.Documenter`. Native executors read them via
  the container (typed slots) or `ToMap()` for dialect-specific binding; ephemeral
  reads via `ToMap()` until Phase C.
- `DeleteDocuments` unchanged (returns count).
- `RawQueryResult.Data` stays `any` (raw driver rows are opaque).

### 4.4 QueryResult (`core/query/dsl.go:343`)

```go
type QueryResult struct {
    Data           []*document.Document   // was []map[string]any
    Count          int
    Total          *int
    PaginationInfo *PaginationInfo
}
```

Engine (`engine.go:54`) passes `interactor.SelectDocuments` output straight
through when post-processing is empty; otherwise runs `runPostProcessing` over
`[]*document.Document`. Filter/Sort/Distinct/Paginate preserve the same docs;
`Project` emits record-view docs for user-defined output shapes (see 4.4).

### 4.5 QueryHelper (`core/query/helper.go`) — recordView

All record access goes through one small interface so both `*document.Document`
and map-backed records (ephemeral store rows, raw results) work:

```go
type recordView interface {
    HasPath(path string) bool
    GetNested(path string) (any, error)
    ToMap() map[string]any
}
```

- `*document.Document` satisfies it natively (container slot resolution, no map).
- `mapRecord(map[string]any)` adapter satisfies it for legacy map rows.

Method signature changes (operate on `[]recordView`, or `[]*document.Document`
where mutation is required):

| Method | Change |
|---|---|
| `Match(record recordView, filters...)` | `GetNested` + `HasPath` |
| `Filter(records []recordView, ...)` | same slice, no copy |
| `ApplyDistinct(records []recordView)` | canonical key via sorted `ToMap()` keys; keep determinism |
| `Sort(records []recordView)` | `GetNested` per sort key |
| `Paginate(records []recordView)` | slicing only |
| `Project(records []*document.Document)` | new record-view `*document.Document` per output (no schema, no pool); `GetNested` in / record set out; **release source docs** |
| `projectRecord` | replace `value.(map[string]any)` asserts (`helper.go:669,671,687,689`) with `GetDocument`/`recordView` |

### 4.6 Collection layer

`base.Collection.Read` (`base.go:131`):
- Engine already returned `[]*document.Document`; wrap directly:
  `ReadResult.Data = data.DocumentSet(docs)` — no `NewDocumentSet` re-conversion.
- ReadResult owns the docs until the caller (`ModelCollection`/user) releases.

`base.Collection.CreateOne` (`base.go:112`):
- Pass the input docs (already `data.Documenter`) straight into
  `interactor.InsertDocuments(ctx, sc, values)` where `values` is
  `[]data.Documenter` built from the `DocumentSet` — drop the `ToMaps()` round-trip.
- Returned docs are `[]*document.Document` → `CreateResult.Docs` wraps them.

`base.Collection.Update` (`base.go:170`):
- `updatesMap` → pass `params.Set` (`data.Documenter`) directly.
- Result docs `[]*document.Document` → `UpdateResult.Docs`.

`managedCollection.CreateMany` (`managed.go:77`):
- Replace `data.NewDocument(doc)` / `data.MustNewDocument(doc, ctx)` validation
  with schema-bound construction via the result collection
  (`FromStruct`/`FromMap`), reusing `currentSchema()` (`managed.go:54`).

### 4.7 Native executor builds `document.Document` directly

The row-scan must construct documents itself — no `map[string]any` ever exists
for schema-resident results. It never derives a per-query result schema.

**Container mode** (query output ⊆ one real schema — the common case): the
interactor passes the target schema's cached collection; `ReadRows` writes typed
columns into pooled containers. This works because `ReadRows`
(sqlite/executor/utils.go:22) already does schema-driven per-column type
conversion (`sc.FindField` + `fromSQLiteValue`); writing into a pooled container
instead of a `map[string]any` is strictly easier. Unreturned columns are simply
absent slots — no projected-subset compile needed.

**Record mode** (joins / aggregations / alias or computed projections): the
interactor flags the query as shape-changing and the executor wraps the driver
row map in a record-view `*document.Document` (no compile, no pool).

**`QueryExecutor[T]` change** (`core/query/native/types.go:119`):

```go
type NativeQuery[T any] struct {
    Query            Query[T]
    Schema           *definition.Schema  // target schema (type conversion)
    ResultCollection *document.Collection // NEW: cached CollectionFor(targetSchema), container mode
    UseRecords       bool                // NEW: record mode for shape-changing queries
}

type QueryExecutor[T any] interface {
    Query(ctx context.Context, query NativeQuery[T]) ([]*document.Document, int64, error)         // was []map[string]any
    QueryStream(ctx context.Context, query NativeQuery[T]) (<-chan *document.Document, <-chan error, error)
    Exec(ctx context.Context, query NativeQuery[T]) (int64, error)                               // unchanged
    ExecuteQuery(ctx context.Context, query NativeQuery[T]) (*query.RawQueryResult, error)       // unchanged; raw stays map-shaped
    // ...
}
```

The interactor decides the mode from the query structure: joins, aggregations,
or alias/computed projections → `UseRecords = true`; otherwise container mode
with the target's cached collection.

Row-scan (container mode):

```go
// sqlite/executor/utils.go — ReadRows
d, err := query.ResultCollection.NewBare()   // pooled container, target schema
// per column:
cv, _ := fromSQLiteValue(fieldDef, value)
_ = d.SetNested(col, cv)                      // single-segment target columns
docs = append(docs, d)
```

- `readRowsToDocs` stream path (utils.go:104) emits `*document.Document` per row.
- `ExecuteQuery` (raw SQL) keeps `RawQueryResult.Data any = []map[string]any`:
  arbitrary SQL columns are not schema-shaped.

### 4.8 Ephemeral interactor

`core/ephemeral/interactor.go:31` — Phase A keeps its inline map pipeline
(Filter/Join/aggregate over go-store `map[string]any` rows) and wraps at the
boundary: container mode via `document.CollectionFor(targetSchema)` + `FromMap`
for shape-preserving queries; record-view docs for joins/aggregations. Phase C
migrates it to `QueryHelper`/recordView so it stops materializing
`[]map[string]any`.

### 4.9 ModelCollection (stage-2 results)

Reads now return pooled `*document.Document` from `ReadResult.Data`. After the
model is bound, the collection releases the document:

```go
// in ReadAs / CreateFrom / UpdateFrom / FindOneByID ...
defer func() {
    if d, ok := any(result).(*document.Document); ok { d.Release() }
}()
```

The existing stage-1 input-release (`d.Release()` after Create/CreateMany/Update/
UpdateMany/Replace/Validate) stays. Ownership: collection binds then releases.

### 4.10 Bind bridge (`core/document/bind.go:58`)

Replace `asDataDocument()` (container → `data.Document` map wrapper → struct) with
**direct container binding**: resolve each target struct field's container slot
from the compiled schema (`ResolvePath`/`Address`) and read the slot directly.
Kills the intermediate map allocation and keeps the bind in `document`.

### 4.11 Migration (`core/persistence/migration/migration.go:75,108`)

- `SelectDocuments` now returns `[]*document.Document`.
- dstSchema may differ from srcSchema, so cross-schema inserts keep correctness
  over allocations: `ToMap()` the source docs, rebuild with the dst result
  collection, pass `[]data.Documenter` to `InsertDocuments`, then release source.
- Source docs released after the insert.

## 5. Compatibility and ripple

The interface change breaks every fake implementer of `query.DatabaseInteractor`
and every caller of the interactor. Grep-anchored list:

- `core/query/engine.go:86` — `SelectDocuments` call.
- `core/persistence/migration/migration.go:75,108`.
- `core/persistence/collection/base.go:112,170,215`.
- `core/query/native/interactor.go` + `core/ephemeral/interactor.go`.
- `core/query/native/types.go` — `QueryExecutor[T]`/`NativeQuery[T]` (returns
  `[]*document.Document`, carries `ResultCollection`).
- `sqlite/executor/executor.go` + `utils.go` — row-scan builds containers.
- Test fakes across `core/**/*_test.go`, `tests/**` implementing
  `query.DatabaseInteractor` (in-memory mocks) — mechanical signature update.
- `query.QueryResult` consumers: `core/query/engine.go`, integration/API layers
  that marshal `result.Data` — `document.Document` marshals identically via its
  `ToMap`/JSON path, so serialization is unchanged.

## 6. Implementation phases (ephemeral-first)

Every phase ends with `ANANSI_ENV=development go test ./... -count=1` green.

- **Phase A — signatures + boundary wrap.** New interactor/`QueryResult`
  signatures; `CollectionFor` registry (per real schema, name+version keyed);
  ephemeral + native wrap maps at the boundary (`FromMap`); `base.Collection`/
  `managedCollection` changes; migration; ModelCollection result release; fix
  test fakes. Internal map pipelines still intact.
- **Phase B — native executor builds containers.** Extend `NativeQuery[T]` with
  `ResultCollection` + `UseRecords`; `ReadRows`/`readRowsToDocs` write into
  pooled containers (container mode) or record-view docs (shape-changing
  queries); `QueryExecutor[T]` returns `[]*document.Document`.
- **Phase C — recordView.** Rewrite `QueryHelper` to `recordView`; project
  allocates new docs + releases sources; ephemeral drops its `[]map[string]any`
  intermediate; engine post-processing works on `[]*document.Document`.
- **Phase D — direct binding.** Replace `asDataDocument()` bridge in `bind.go`;
  document-package tests + benchmarks.
- **Phase E — cleanup.** Delete map re-conversion helpers now unused
  (`data.NewDocumentSet` call sites in persistence), `go vet ./...`,
  `gofmt`, optional `benchstat` on a representative query.

## 7. Risks

- **Per-query schema compile cost**: mitigated by the two-tier model — compiled
  schemas/pools are bounded to real schemas (name+version keyed, invalidated on
  migration); query shapes never compile. Verify with a benchmark that the
  common read path does zero `definition.Compile`/`Link` calls.
- **Record-mode detection**: the interactor must flag shape-changing queries
  (joins, aggregations, alias/computed projections) correctly; a missed flag
  makes `SetNested` fail on a schema-free column. Centralize the predicate and
  unit-test it.
- **Distinct determinism**: current map canonicalization must produce identical
  ordering when reading from containers — verify with existing distinct tests.
- **Projection nested maps** (`helper.go:669–689`): nested objects/documents must
  resolve via `GetDocument`, not `value.(map[string]any)`.
- **`SelectStream` ownership**: reader must `Release()` each item; document the
  contract on the channel.
- **Migration cross-schema insert**: explicit `ToMap` bridge (correctness path).
- **`QueryResult` JSON consumers**: verify serialization output is byte-identical
  after the `Data` type change.

## 8. Acceptance criteria

1. No `data.NewDocument` / `data.MustNewDocument` / `NewDocumentSet` in
   persistence read/create/update paths.
2. `QueryResult.Data` is `[]*document.Document`; interactor methods accept and
   return `document.Document` / `data.Documenter`; `QueryExecutor[T]` no longer
   materializes `[]map[string]any`.
3. ModelCollection auto-releases bound results (proven by a
   `require.Panics`-after-release test, as in the stage-1 release test).
4. Full suite green: `ANANSI_ENV=development go test ./... -count=1` exit 0;
   `go vet ./...` exit 0; gofmt clean.
5. Representative query benchmark shows no alloc regression (target: fewer
   `[]map[string]any` allocations on the read path).
