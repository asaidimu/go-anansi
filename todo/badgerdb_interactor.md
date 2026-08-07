# Proposal: BadgerDB as the Default Interactor

This proposal makes **BadgerDB** (dgraph-io/badger) the default `query.DatabaseInteractor`
for go-anansi — replacing SQLite as the out-of-the-box datastore — while keeping SQLite
fully supported. The change is architectural (a new storage engine) but not user-facing
(the public `Setup`/`Playground`/`Collection` APIs stay identical — only the engine behind
`Interactor` changes).

Status: **draft for review**. Nothing below has been implemented.

## 1. Why BadgerDB

- **Pure Go, embedded, zero CGO** — a single-binary deploy story like SQLite, no runtime deps.
- **LSM-tree durability** with ACID transactions at the KV level and MVCC versioned reads
  (`commitTs`). Badger gives us versioned snapshots for free, which the persistence layer
  can later expose for time-travel or point-in-time reads.
- **Best-in-class write throughput** through append-only value logs plus lazy compaction.
- **Streams natively** (`badger.Iterator` with prefetch control) — a natural fit for
  `SelectStream`.
- **Schemaless** — the migration layer can add/drop/alter "columns" with no table rebuild.

"Default interactor" here means Playground + the recommended production setup, not removal
of SQLite support.

## 2. Where it plugs in — architecture fit

The engine boundary is `core/query/interactor.go`:

- `DatabaseInteractor` — select/stream/insert/update/delete documents
- `SchemaManager` — collection and index lifecycle
- `DocumentPoolRegistrar` — schema-bound container pools on the write path

The existing generic scaffolding (`native.NativeInteractor[T]` over `QueryFactory[T]` +
`QueryExecutor[T]`, used by the `sqlite/` dialect) is built around a *relational* model.
Badger is a **KV store** and does not fit that shape cleanly, so we add a **dedicated
`badger/` package** that implements `query.DatabaseInteractor` (and `SchemaManager`,
`DocumentPoolRegistrar`) directly, KV-native. The engine is relation-agnostic, so
`core/persistence`, collections, and the `core/query` DSL need **no changes**.

Crucially, the **`QueryPartitioner` runs in the persistence engine**, not inside the
interactor. So a Badger interactor only needs to execute the "DB-residual" it advertises;
everything else is filtered/sorted/aggregated in Go by the engine. We advertise a small
capability set and lean on the partitioner.

## 3. The interactor package — `badger/`

```
badger/
  interactor.go     // BadgerInteractor: DatabaseInteractor implementation
  store.go          // DB open/close, collection namespacing, value-log GC
  keys.go           // key encoding
  scan.go           // row + stream iteration
  index.go          // secondary index write/scan
  capabilities.go   // query.Capabilities and helper predicates
  schema.go         // SchemaManager DDL (namespaces + schema meta record)
  transaction.go    // txn wrapper + write serialization
  badger_test.go
```

Constructor: `NewBadgerInteractor(dir string, opts ...Option) (query.DatabaseInteractor, error)`.
Pass `dir == ""` (or `Option{InMemory: true}`) for an in-memory store via
`badger.Options.InMemory` — used by `Playground`, tests, and the default production setup.

### 3.1 Document row model

Each document is one KV entry under its id, namespaced per collection:

- `d.<collection>.<id>` → document value (the canonical go-anansi row encoding).
- `s.<collection>` → a stable schema-metadata record (`definition.Schema` snapshot) used to
  decide whether a collection exists and to guide the write path.

Row encoding reuses the go-anansi encoding used by the SQLite RETURNING scan, and
`SelectDocuments`/`InsertDocuments`/`UpdateDocuments` materialize `*document.Document`s
through the registered `document.DocumentPool` (`DocumentPoolRegistrar`) — the same pooled,
schema-bound fast path.

### 3.2 Query shape support

The Badger scan path respects `Query.Shape` and `Query.DocumentPool` exactly like the
SQLite/Native path, so result rows scan directly into pooled containers when the partitioner
says the row shape is pool-safe.

## 4. Mapping `DatabaseInteractor` methods

| Method | Badger mapping |
|---|---|
| `SelectDocuments` | Range scan `d.<collection>` over `id`; when filters use an indexed field, walk the index keyspace (`i.<collection>.<index>.<value>.<id>`) to obtain ordered ids, then materialize documents. |
| `SelectStream` | `Iterator` over `d.<collection>` with `PrefetchValues: false`, emitting normalized rows on a channel. |
| `InsertDocuments` | One read-write `txn`: `db.Set(d.<collection>.<id>, row)` per doc + writes to each secondary index bucket; returns pooled documents. |
| `UpdateDocuments` | Read-modify-write in one `txn`: gather matching ids (filter scan), mutate rows, rewrite and refresh affected index entries; if `returning`, return the updated pooled documents. |
| `DeleteDocuments` | Same match, delete document keys + index entries; return affected count. |
| `Query` (raw) | Translate the raw/templated query over the KV space; count-only queries read index bucket lengths. `RawQueryResult` shape preserved. |
| `SchemaManager` DDL | See §6 — mostly metadata + index-namespace setup; no table rebuild. |
| `StartTransaction`/`Commit`/`Rollback` | One read-write `badger.Txn`; Badger allows only one open RW txn, so writes serialize — advertise `RequiresTransactionSerialization: true` and let the persistence layer's existing mutex handle it. |
| `HasTransaction` | `isTx` flag mirroring NativeInteractor. |

Note: Badger allows only **one** read-write transaction at a time, but **concurrent
read-only** transactions (snapshot isolation via MVCC) are fully supported.

## 5. Capabilities — lean on the partitioner

Badger is a KV store, not a query engine: it wins on scans, ranges, and equality. Advertise
**conservatively** and let `QueryPartitioner` + the in-memory residual do the heavy lifting:

```go
func (b *BadgerInteractor) Capabilities() query.Capabilities {
    return query.Capabilities{
        RequiresTransactionSerialization: true,
        SchemaEvolution: query.SchemaEvolution{
            AddColumn: true,        // schemaless — no DDL required
            DropColumn: true,
            RenameColumn: true,     // just metadata
            AlterColumnType: true,
            AddConstraint: true,
            DropConstraint: true,
        },
        SupportedLogicalOperators:      eq/and/or/not,
        SupportedComparisonOperators:   Eq/Neq/In/Gt/Lt/Gte/Lte on indexed fields,
        SupportedExpressionOperators:   nil,
        SupportedFunctions:             nil,
        SupportedJoinTypes:             nil,
        SupportedAggregationFunctions:  nil,   // engine aggregates in Go
        SupportedPaginationTypes:       limit/offset + prefix-scan cursor,
        SupportedTextSearchTypes:       EXACT (via index) only,
        Sorting:  SortingCapabilities{SupportsNullsOrdering: false, SupportsExpression: false},
        SupportsGroupBy:  false,
        SupportsDistinct: false,
        SupportsNestedFields: false,
        MaxWhereConditions: 0, MaxJoinClauses: 0,
        ReturnOnUpdate: true,
    }
}
```

Corollary: complex filters, aggregations, joins, and function-based sorting fall back to the
in-memory residual automatically — no cross-check needed. Indexed-single-field equality,
range, `In`, and `Contains`/`Exists` get real keyspace pushdown.

## 6. Schema manager / migration

Collections are **schemaless** at the KV layer, so `SchemaManager` operations are cheap:

- `CreateCollection` — write a `s.<collection>` meta record (schema snapshot + version).
- `CollectionExists` — `db.Get(s.<collection>)`.
- `DropCollection` — delete the collection keyspace; value-log GC later reaps the data.
- `CreateIndex`/`DropIndex` — write/clear the index bucket; no row rebuild.
- `AddColumn`/`DropColumn`/`RenameColumn`/`AlterColumnType` — `true` (no-op / metadata)
  because documents carry their own shape; row rewrite is only triggered by data
  transformation, never by DDL.

So `migrate` becomes a **data-transform-only** operation (like SQLite's full-table-replacement
path, but without a rename/swap dance since there is no table to rebuild). The existing
migration chapter (`core/persistence/migration`) works unchanged — it just falls into the
"no table rebuild needed" branch.

## 7. Secondary indexes

For each named `definition.Index`, Badger keeps a bucket:

```
i.<collection>.<index>.<fieldpath>.<value>.<id>
```

- Supports equality, `In`, and range scans by walking `value` in byte order.
- Multi-field indexes compose the `value` segments.
- Unique constraints are enforced inside `InsertDocuments` via index existence checks.

Cost note: every write also writes index keys — the same cost profile as SQLite secondary
indexes and standard for LSM occupancy. Index updates happen in the same transaction that
writes the row, keeping data + index consistent.

## 8. Transactions

- `StartTransaction` returns a `BadgerInteractor` bound to a single read-write `badger.Txn`.
- Because Badger is single-writer, concurrent writes funnel through one mutex;
  `RequiresTransactionSerialization` lets the persistence layer own that lock (already
  supported — SQLite uses it in private-DB mode).
- Reads use MVCC snapshot isolation and run concurrently.

Operational note: the value log needs background `RunValueLogGC`/table GC; the store runs it
on a timer and on `Close`. This is a known embedded-Badger operational concern (unlike
SQLite's single-file WAL that auto-checkpoints) and must be documented.

## 9. Making it the default

- **`anansi.Setup`**: when `Interactor` is `nil`, construct `badger.NewBadgerInteractor`
  (in-memory) instead of the SQLite default.
- **`anansi.Playground`**: use the in-memory Badger store by default; `PlaygroundConfig` gets
  a `Storage: sqlite | badger` knob (default `badger`).
- **README + examples** point at the Badger path; SQLite becomes an explicitly documented option.
- **codegen / `cmd/anansi`** untouched — engine-agnostic.

Backwards-compat: `sqlite/` remains fully supported; the default flip is one flag, so it is a
one-file behavioral change to revert.

## 10. Testing + benchmark plan

- `badger/` unit tests: encode/decode, index round-trip, transactions (commit/rollback),
  schema manager, capabilities.
- Parity conformance tests mirrored from `tests/integration/persistence` run against the
  Badger interactor through the same harness.
- Migrations end-to-end (schema-change workflow) on Badger.
- Benchmarks in `example/benchmark`: SQLite vs Badger insert/read/stream on large datasets.

## 11. Trade-offs / risks

**Pros**: write-scale and latency wins on append-heavy workloads; MVCC versioned reads; no
CGO; schemaless column evolution; streaming reads; concurrent read snapshots.

**Cons / open questions**:
- **Not a relational engine** — joins and complex aggregations fall to the in-memory engine.
  For genuinely relational workloads SQLite/PostgreSQL remains superior.
- **Single-process** — Badger cannot be opened by multiple processes on the same store; for
  multi-instance deployments, keep SQLite/PG.
- **Manual GC** — `RunValueLogGC` must run (timer + `Close`), or the value log balloons.
- **One writer txn** — writes contend under one mutex; benchmark to confirm the target
  workload's throughput is acceptable.
- **Index fan-out** — every write fans into index buckets.
- **Feature-parity gap** — some operations SQLite pushed down now run in Go; grow the
  capability set incrementally.

## 12. Rollout / milestones

1. `badger/` skeleton: open store, key encoding, `SelectDocuments` + `InsertDocuments`;
   pass the integration persistence parity suite.
2. `SchemaManager` + secondary indexes + transactions.
3. Update/Delete/Stream/raw `Query`; document-pool fast path.
4. Default flip in `Playground` (in-memory) + `Setup`; README updates.
5. Benchmarks, value-log GC operationalization, docs; push down more capability bits as wins.

## Conclusion

BadgerDB is a strong default: zero-dependency embedded, high write throughput, snapshots,
streaming reads, and constant-time column evolution — matching go-anansi's
schemaless-at-rest philosophy, while keeping SQL relational engines available by explicit
choice. The change is additive through the existing `DatabaseInteractor` boundary, SQLite
stays, and nothing on the user-facing API surface changes.

(This is a proposal, not an accepted change — please read §1 and §11 carefully before we
start.)
