---
title: Query engine overhaul
description: "Plans a rewrite of core/query (QueryEngine, QueryPartitioner, QueryHelper)."
rfcStatus: draft
rfcId: query-engine-overhaul
---
the library's query engine (`core/query`) currently needs an overhaul. This
is a **known gap / open item**, not a reverted trial.

## What is wrong

- The engine is **constructed internally** and not reachable from user code:
  `core/persistence/persistence/base.go:52` builds it as
  `query.NewQueryEngine(interactor.Capabilities(), logger)` and it is never
  exposed through `anansi.SetupConfig` or `persistence.NewPersistence`.
  Consequences:
  - `RegisterComputeFunction` / `RegisterFilterFunction`
    (`core/query/engine.go:45,50`) are effectively **dead in userland** — custom
    `AddComputed(alias, fn, …)` names and custom filter operators can't be
    registered at startup.
  - There is no way to tune the LRU `QueryCache` (fixed at size 100), add
    query hooks, or swap the `QueryPartitioner`/`QueryHelper`.
- A **hybrid dual-implementation smell**: an older query path coexists with the
  capabilities-based `QueryEngine`. The partitioner splits a `Query` into a
  DB part + a residual, but how far each backend pushes down depends entirely
  on `Capabilities()`. Only a subset of the DSL/engine surface is exercised
  and therefore **stable**; the rest is provisional.

## The goal of the overhaul

Make the query engine a first-class, externally configurable component:

- Thread an engine-config hook from `anansi.SetupConfig` through
  `Setup` → `NewPersistence` → `newBasePersistence`, so the caller can
  register compute/filter functions; tune cache size; and install
  hooks/wrappers **before** collections are built.
- Deliberately declare which portions of `core/query` are stable (the DSL
  builders, CASE, aggregations, arithmetic pushdown, partitioning of simple
  queries) vs in-flux (custom compute registration, post-processing matrix,
  join/subquery resolution, capability negotiation), and route
  documentation accordingly.
- Decide the fate of the pre-partitioning query path and collapse it into the
  capabilities-based engine.

## References

- `core/query/engine.go` — `NewQueryEngine`, `RegisterComputeFunction`,
  `RegisterFilterFunction`, the LRU `QueryCache`.
- `core/query/interface.go` — `Capabilities`, `ComputeFunction`,
  `PredicateFunction`, `QueryPartitionerInterface`.
- `core/persistence/persistence/base.go` — where the engine is instantiated
  today (`newBasePersistence`).
- `anansi.go` — `SetupConfig` (where a `QueryEngine`/config hook would live).
- `skills/anansi/SKILL.md` — "Note: only a subset of the query engine is
   stable" warning.