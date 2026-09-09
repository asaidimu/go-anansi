---
title: "Architecture"
description: "The package layout, what each core/ subdirectory does, and how the pieces fit together. Read this when you need a mental map before diving into a specific area."
---

# Architecture

Go-Anansi is organized around a single principle: **the schema is the source
of truth; everything else is layered on top of it.** The package layout
reflects that — schema definition lives at the center, and every other layer
depends on it without reaching across.

## What Anansi is — and isn't

Anansi is a **data-layer toolkit**, not an application framework. The
package layout below reflects that: there is no `core/http`, no `core/auth`,
no `core/cli-app`. Anansi handles schemas, codegen, queries, migrations,
persistence, and events. Everything above the data layer is yours to bring.

This is deliberate. A data layer that stays narrow composes cleanly with
whatever HTTP framework, auth system, or logger you've already chosen. A
data layer that tries to be a library forces you into its opinions.

## Package layout

| Package | Role |
| --- | --- |
| `anansi` (root) | `Setup` (production) and `Playground` (dev) entry points plus `SetupConfig` / `PlaygroundConfig`. |
| `core/document` | The preferred document container — `Document`, `DocumentModel`, `document.New[T]`, `DocumentPool`. Container/pool-backed; used by all generated code. |
| `core/data` | The older map-backed document pipeline. Still used for cross-cutting contracts (`data.Documenter`, `data.DocumentIDField`, `data.DocumentSet`) and the reverse direction (`data.SchemaFrom[T]`). Deprecated for new model code. |
| `core/schema` (`definition`) | The schema type, graph validator, normalization, and migration generation. |
| `core/query` | The query DSL, `QueryEngine`, partitioning, and the `DatabaseInteractor` interface (`core/query/native`). |
| `core/persistence` | `base.Collection`, `collection.ModelCollection[P]` (typed wrapper with caching), transactions, the collection registry. |
| `core/events` | The generic publish/subscribe event bus. |
| `core/sanitize` | PII / secret scrubbing at the boundary. |
| `core/reflect` | Tag registry and field-mapping internals. |
| `core/encoding/anansi` | The Anansi binary wire format (Go side). |
| `sqlite` | The reference `executor` / `query` implementation. |
| `codegen` | `golang`, `typescript`, and `faker` generators. |
| `cmd/anansi` | The CLI. |
| `packages/anansi` | TypeScript wire-format implementation (cross-language conformance with Go). |

## The unopinionated core

Anansi's core is deliberately minimal: schema definition, document
containers, a query engine, and a persistence interface. Everything else —
caching, audit, encryption, sanitization, observability — is **injected** by
you, typically through decorators wrapping `Persistence` or `Collection`.

This is why there's no built-in auth middleware, no opinionated event
transport, no specific logger integration. The toolkit provides the seams;
you bring your own implementations.

## Why schema-driven

Three properties fall out of schema-driven design that are hard to get any
other way.

### No drift between code and data model

The schema is the source of truth for storage, validation, migrations, AND
codegen. There's no second source to fall out of sync.

### Cross-language coherence

The same schema drives Go codegen AND the TypeScript wire format. The two
implementations are verified byte-for-byte equivalent by CI.

### Pluggable backends

The `DatabaseInteractor` interface abstracts the storage backend. SQLite is
the reference; other SQL or NoSQL backends can be added without touching the
schema or query DSL.

## Where each concern lives

| Concern | Where it lives |
| --- | --- |
| Schema format | `core/schema/definition` + [reference](/reference/schema-format) |
| Schema IR (compiled form) | [Schema IR](/reference/schema-ir) (proposal) |
| Validation | Graph validator in `core/schema` |
| Migrations | `core/schema` (generation) + `migrations/` (artifacts) |
| Query DSL | `core/query` — see [Query DSL](/reference/query-dsl) |
| Code generation | `codegen/golang`, `codegen/typescript`, `codegen/faker` |
| Persistence | `core/persistence/base.Collection`, `core/persistence/collection.ModelCollection[P]` |
| Events | `core/events` + `core/persistence/base` (event types) |
| Caching | `core/cache` (primitive) + `ModelCollection` id-cache + `LiveCollection` |
| Transactions | `core/persistence` (`p.Transact`, `coll.Transact`) |
| Sanitization | `core/sanitize` |
| SQLite backend | `sqlite/executor`, `sqlite/query` |
| Wire format | `core/encoding/anansi` (Go) + `packages/anansi` (TS) |

## The two document pipelines

Anansi ships two document implementations. Knowing which one to use matters:

| Pipeline | When to use |
| --- | --- |
| **`document`** (preferred) | All new model code. Container/pool-backed; what all generated code and `ModelCollection` use. Pooled, fast, the future. |
| **`data`** (legacy) | Cross-cutting contracts only (`Documenter`, `DocumentIDField`, `DocumentSet`) and the reverse direction (`SchemaFrom[T]`). Map-backed; incurs map allocations and pools nothing. Deprecated for new models. |

The `document` package is what `data.New[T]`-style code in older examples
should be migrated to. New code should reach for `document.New[T]`.

## Related

- [Data flow](/explanations/data-flow) — the 5-step request path from your
  call to the database.
- [Schema as source of truth](/explanations/schema-as-source-of-truth) —
  why this design choice matters.
- [Hybrid query engine](/explanations/hybrid-query-engine) — how a query is
  partitioned between SQL and in-memory.
- [Wire format](/explanations/wire-format) — the Go ⇄ TypeScript
  cross-language guarantee.
