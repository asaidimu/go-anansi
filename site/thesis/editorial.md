---
title: "Editorial: the plan vs reality"
description: "How the original thesis survived implementation — what held, what transformed, what is still pending."
---

# Editorial: the plan vs reality

The thesis in this section was written optimistically, before
implementation, for the original anansi system. The three chapters here
keep its ambitions, premises, and voice, revised in retrospect against
what go-anansi, hestia, and hedwig actually became. The old system's API
specifications were deliberately left behind — that material now lives,
corrected, in the [reference](/reference/schema-format). What follows is
the accounting: which parts of the plan survived contact with reality,
which transformed, and which are still open.

## What held

**The single source of truth.** The core bet — one schema document that
storage, validation, migration, and codegen all build from, with nothing
hand-duplicated — survived intact and got stronger. The schema proved
versatile beyond the original sketch: it already has four live
representations (JSON documents, Go structs, the query DSL's view of it,
and Go structs-plus-tags via DTO derivation), which is why a fifth
(GraphQL) reads as entirely possible rather than speculative.

**Registry, versioning, migrations.** Field IDs pin identity across
renames; the lockfile pins the registry; migrations are versioned with
dry-run, squash, and rollback — in Go *and* reimplemented as a streaming
engine in TypeScript. The unglamorous machinery the thesis hand-waved at
("version tracking and dependency management") turned out to be the load
bearing wall, and it got built.

**Codegen as the bridge.** `anansi codegen golang` (structs, enums,
projections, typed collections), `typescript`, and `faker` do what the
thesis promised code generation would do — with the one correction the
implementation forced: the generated file is overwritten every run, so
custom code lives in purpose-named files, never in the output.

## What transformed

**Persistence abstraction.** The thesis imagined a framework abstracting
SQL, NoSQL, and key-value stores behind one interface. What got built is
narrower and, frankly, better for it: one reference backend (embedded
SQLite) behind a `DatabaseInteractor` seam, with the query engine
partitioning work between SQL pushdown and an in-memory residual. hestia
layers Postgres pluggability on top. The abstraction survived as a seam,
not as a framework.

**Visual modeling.** There is no product called Data Studio. What exists
instead is better evidence for the thesis: hedwig's collections studio —
a visual composer (fields, indexes, compositions, preview, relationship
graph) managing anansi collections in a production ERP UI. Same idea,
different name, earned rather than announced.

**Multi-language SDKs.** The thesis promised SDKs in multiple languages.
What exists is sharper: Go does everything; TypeScript implements the wire
format, validation, and migrations byte-compatibly — no query engine, no
codegen, no persistence. A codec with guarantees, not an SDK with
pretensions.

## What is still open

**Model replication.** The original complaint — customer data defined five
times (docs, SQL, Go structs, OpenAPI, TypeScript) — is four-fifths closed:
JSON, Go, query, and DTO-derivation all read the same document. The fifth,
API contracts, is in progress: hestia carries a docs endpoint intended to
become an OpenAPI spec endpoint, which would derive the contract from the
schema like everything else.

**GraphQL.** Possible, unstarted. The four existing representations suggest
the schema has the expressive range for it; nobody has needed it enough to
build it.

## How to read what follows

- The **problem statement** (`enterprise-systems-evolution`) and the
  **thesis** (`schema-driven-development`) stand as written. The layers
  drifted; the diagnosis didn't.
- The **vision** (`evolution-framework`) is directionally right but names
  products that shipped under other names — read it with the mapping above.
- The old API specifications are gone from this section, not from history:
  the ideas they described (registry, versioned migration, predicate
  validation) reappear in go-anansi's `core/schema/definition` in
  different clothes. Consult the [reference](/reference/schema-format)
  for the system as built.
