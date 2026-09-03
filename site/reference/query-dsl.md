---
title: Query Dsl
description: "Full method surface of the core/query QueryBuilder DSL — Where, Sort, Limit, joins, aggregations, partitioning."
---
> **Stability note:** only a subset of the query engine is stable — see the
> "Query DSL" section of SKILL.md. The QueryBuilder, CASE, aggregations, and
> arithmetic (`ADD`/... ) pushdown are stable; `RegisterComputeFunction` /
> `RegisterFilterFunction` custom registration has **no** public startup hook
> yet, so custom-compute and custom-operator features below are provisional.

`core/query` provides a fluent builder for filtering, sorting, pagination,
projection, joins, and aggregation. Queries are partitioned by the engine: what
the active `DatabaseInteractor` supports runs in the backend (SQL), the
residual runs in-memory. The DSL is identical either way — you just write the
query and read the results.

## Building a query

```go
import "github.com/asaidimu/go-anansi/v8/core/query"

q := query.NewQueryBuilder().
    From("Orders").                 // optional; collections often already know their schema
    Where("total").Gt(100.0).       // operators: Eq, Neq, Gt, Gte, Lt, Lte, In, Nin, ...
    Sort("createdAt", query.SortDirectionDesc).   // SortDirectionAsc / SortDirectionDesc
    Limit(50).
    Build()                         // returns query.Query

results, err := coll.Read(ctx, &q)  // *base.QueryResponse with .Data
```

`Build()` returns a `Query` value; pass a pointer (`&q`) to `Read`.

## Filter operators

`Where("field")` returns a filter builder with:

| Method | Meaning |
| --- | --- |
| `Eq(v)` | equals |
| `Lt(v)` / `Lte(v)` | less than / less than or equal |
| `Gt(v)` / `Gte(v)` | greater than / greater than or equal |
| `In(...v)` / `Nin(...v)` | in list / not in list |
| `Contains(v)` / `NotContains(v)` | substring / not substring |
| `Exists()` / `NotExists()` | field present / absent |

Multiple `Where(...)` calls AND together. For an OR/AND group, use
`WhereGroup(common.LogicalOr)` and chain the conditions, terminating with
`End()`:

```go
q := query.NewQueryBuilder().
    WhereGroup(common.LogicalOr).
        Where("status").Eq("open").
        Where("status").Eq("pending").
    End().
    Build()
```

The `AndFilter`/`OrFilter` methods combine whole existing `QueryFilter` values.

## Sorting & pagination

```go
query.NewQueryBuilder().
    Sort("createdAt", query.SortDirectionDesc).  // or OrderByDesc / OrderByAsc
    ThenSortBy("id", query.SortDirectionAsc).
    Limit(50).
    Offset(0).
    Build()
```

## Projection (field selection)

```go
q := query.NewQueryBuilder().
    Select().Include("name", "price"). // or .Exclude(...)
    Build()
```

## Joins

The join condition is a `QueryFilter` whose value can reference another
collection's field via a `FieldReference`:

```go
q := query.NewQueryBuilder().
    From("Account").
    LeftJoin("User").On(query.QueryFilter{
        Condition: &query.FilterCondition{
            Field:    "Account.userId",
            Operator: query.ComparisonOperatorEq,
            Value: query.FilterValue{
                FieldRefVal: &query.FieldReference{Type: "field", Field: "User._id_"},
            },
        },
    }).End().
    Where("User._id_").Eq("U001").
    Build()
```

Join builders: `Join`, `InnerJoin`, `LeftJoin`, `RightJoin`, `FullJoin` — each
`.On(query.QueryFilter{...})` then `.End()`. Joined results nest under the
target collection name (with `.Alias("...")` to rename).

## Aggregation & grouping

```go
q := query.NewQueryBuilder().
    GroupBy("status").
    Count("id", "total").
    Sum("total", "revenue").
    Avg("total", "average").
    Min("total", "min_total").
    Max("total", "max_total").
    Build()
```

Also `Distinct`, `DistinctBy`, and set ops `Union`, `UnionAll`, `Intersect`,
`Except`.

## Text search

```go
q := query.NewQueryBuilder().TextSearch("title").Contains("anansi").Build()
```

## Querying by document id

```go
q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build()
```

## Notes

- The builder also exposes `Aggregate`, `Increment`, `AddHint`, `UseIndex`,
  `ForceIndex`, `NoIndex`, `MaxExecutionTime`, `Clone`, `Reset`, and
  `Validate()` (returns `QueryValidationResult`).
- Reads through a `ModelCollection` return model-typed results; projection
  reads use `ReadAs[R]` (see SKILL.md).