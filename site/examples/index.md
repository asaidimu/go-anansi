---
title: "Examples"
description: "A catalog of runnable examples shipped with go-anansi. Each example is a self-contained Go program (or TS/Go pair) demonstrating a specific Anansi capability."
---

# Examples

The repository ships with seven runnable examples under
[`example/`](https://github.com/asaidimu/go-anansi/tree/main/example). Each is
a self-contained program demonstrating a specific Anansi capability — schema
declaration, codegen, CRUD, query, events, migration, or the cross-language
wire format.

## Catalog

| Example | What it shows | How to run |
| --- | --- | --- |
| [Basic CRUD](/examples/basic) | Schema declaration, codegen, typed CRUD with ModelCollection. The starting point. | `cd example/basic && ANANSI_ENV=development go run .` |
| [REST API](/examples/api) | A net/http server integrating Anansi for persistence, with middleware, response helpers, and production-style wiring. | `cd example/api && ANANSI_ENV=development go run .` |
| [Events](/examples/events) | Subscribing to lifecycle events for audit logging and downstream side effects. | `cd example/events && ANANSI_ENV=development go run .` |
| [Migration](/examples/migration) | Generating and applying schema migrations on startup; the canonical schema-change workflow. | `cd example/migration && ANANSI_ENV=development go run .` |
| [Complex schema](/examples/complex) | Nested schemas, projections, enums, composites, and unions in one schema. | `cd example/complex && ANANSI_ENV=development go run .` |
| [Benchmark](/examples/benchmark) | Bind cycle benchmarks for profiling codegen output and document binding overhead. | `cd example/benchmark && ANANSI_ENV=development go test -bench .` |
| [Go ⇄ TS round trip](/examples/encoding-roundtrip) | Encode a document in Go, decode it in TypeScript, verify byte-for-byte equivalence. | `cd example/encoding && ./run.sh` |

## Picking an example

- **First time?** Start with [Basic CRUD](/examples/basic) — it's the
  smallest end-to-end demo and the template the tutorial is based on.
- **Building an HTTP service?** [REST API](/examples/api) shows how to wire
  Anansi into a `net/http` server with production-style persistence.
- **Evolving a schema?** [Migration](/examples/migration) walks through the
  canonical schema-change workflow with a real migration file.
- **Going cross-language?** [Go ⇄ TS round trip](/examples/encoding-roundtrip)
  is the canonical demo of the wire format's cross-language guarantee.

## Running examples

Most examples need `ANANSI_ENV=development` because their fixtures use plain
field IDs (production mode requires UUIDv7):

```bash
cd example/basic
ANANSI_ENV=development go run .
```

See [Testing](/contribute/testing) for why this flag exists and when to drop
it.
