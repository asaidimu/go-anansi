---
title: "Hybrid query engine"
description: "Why Anansi partitions every query into a database-supported part and an in-memory residual. What gets pushed to SQL, what doesn't, and why."
---

# Hybrid query engine

Every query in Anansi is **partitioned**: the parts the `DatabaseInteractor`
supports run in the backend (SQL via SQLite), the residual runs in-memory.
This is not a fallback — it's the design.

## Why partition

SQL backends are good at filters, sorts, joins, and simple aggregations.
They're bad at:

- Custom compute functions (`total * tax_rate` where `tax_rate` is a Go
  callback, not a column).
- Custom filter operators (e.g. "match this document if its embedding is
  within cosine distance X of the query vector").
- Cross-collection joins that span backends.
- Aggregations with non-SQL-reducible final steps.

A pure-SQL engine forces you to express everything in SQL or to give up and
do everything in memory. A pure-in-memory engine gives up on indexes and
pushdown. Anansi's partitioner takes the middle path: push what you can, run
the residual in Go.

## What gets pushed to SQL

- Simple filters: `Where("price").Gt(100.0)`, `Where("name").Eq("Laptop")`,
  `Where("id").In(ids...)`.
- Sorts: `Sort("createdAt", Descending)`.
- Pagination: `Limit(50)`, `Offset(20)`.
- Simple joins on indexed columns: `InnerJoin("Orders", "id", "order_id")`.
- Standard aggregations with `GroupBy`: `Sum`, `Avg`, `Count`, `Min`, `Max`.
- Arithmetic pushdown: `Increment("stock", 1)`, `AddComputed("total",
  query.ADD, "subtotal", "tax")`.

## What stays in memory

- Custom compute functions registered via `RegisterComputeFunction`.
- Custom filter operators registered via `RegisterFilterFunction`.
- Aggregations with non-SQL-reducible final steps.
- Anything the backend's declared `Capabilities` say it can't do.

## A worked example

```go
q := query.NewQueryBuilder().
    From("Orders").
    Where("total").Gt(100.0).           // pushes to SQL: WHERE total > 100.0
    Sort("createdAt", query.Descending). // pushes to SQL: ORDER BY created_at DESC
    Limit(50).                            // pushes to SQL: LIMIT 50
    AddComputed("taxAmount", "MULTIPLY", // pushes to SQL: total * 0.08
        query.FieldRef("total"),
        query.Literal(0.08)).
    Build()
```

The full query compiles to roughly:

```sql
SELECT id, total, created_at, (total * 0.08) AS taxAmount
FROM Orders
WHERE total > 100.0
ORDER BY created_at DESC
LIMIT 50
```

Now consider a query with a custom compute function:

```go
q := query.NewQueryBuilder().
    From("Products").
    AddComputed("relevance", "cosine_similarity",
        query.FieldRef("embedding"),
        query.Literal(queryVector)).
    Build()
```

`cosine_similarity` isn't a SQL function — it's a Go callback. The
partitioner pulls the matching rows from SQL and runs the cosine similarity
in memory. The query succeeds; it just doesn't push the compute step.

## The stable subset

::: warning Stability warning
Only a subset of the query engine is stable. The `core/query` engine
(`QueryEngine`, `QueryPartitioner`, `QueryHelper`) is slated for an overhaul.
**Do not build against its internals.**
:::

The stable, documented surface is:

- The fluent **QueryBuilder** DSL.
- **CASE** expressions (`AddCase` / `When` / `Else`).
- **Aggregations** (`GroupBy` + `Count` / `Sum` / `Avg` / `Min` / `Max`).
- **Arithmetic pushdown** (`Increment` and `AddComputed` with the `ADD` /
  `MULTIPLY` / `SUBTRACT` / `DIVIDE` operators).
- Partitioning of simple filter / sort / paginate queries into DB + residual.

## Red flags — don't build against these yet

- `RegisterComputeFunction` / `RegisterFilterFunction` have **no** public
  startup hook today (the engine is built internally in
  `core/persistence/persistence/base.go`). Calling them at the wrong time
  is a silent no-op.
- `AddComputed` with a custom function name (other than the arithmetic
  operators above).
- Custom filter operators.
- Explicit join/subquery negotiation.

For anything the DSL can't express, prefer raw `p.Query(ctx,
&query.RawQuery{...})` and skip the partitioner entirely.

## How to know what pushed

The `QueryEngine` doesn't currently log its partitioning decisions. To
inspect what's being pushed, enable query logging on the interactor:

```go
interactor, _ := native.NewNativeInteractor(executor, queryFactory, logger)
// With EnableLogging=true on Playground, the interactor logs the SQL it runs.
```

If you see SQL that doesn't include your filter, that filter is running
in-memory. If you see a `SELECT *` with no `WHERE`, the entire query is
running in memory (often a sign the partitioner gave up).

## Related

- [Query DSL](/reference/query-dsl) — the full method surface.
- [Collection internals](/reference/collection-internals) — where the
  QueryEngine sits in the request path.
- [Query engine overhaul RFC](/rfc/query-engine-overhaul) — the planned
  rewrite.
