---
title: Benchmark
description: "Bind cycle benchmarks for profiling. Useful for measuring codegen output and document binding overhead."
---

# Benchmark

The [`example/benchmark`](https://github.com/asaidimu/go-anansi/tree/main/example/benchmark)
directory contains bind cycle benchmarks for profiling.

## What it shows

- A single `Order` schema with realistic field count and types.
- `bind_cycle_test.go` measuring document → struct binding overhead.
- The full codegen + migration + registry pattern, in a benchmark-friendly
  layout.
- An `AGENTS.md` showing how an Anansi project documents itself for AI
  agents.

## How to run

```bash
cd example/benchmark
ANANSI_ENV=development go test -bench .
```

## Read next

- [Reference: Collection internals](/reference/collection-internals) —
  where binding sits in the request path.
- [Guide: Caching](/guides/caching) — `ModelCollection` as the fast path,
  `Release()` and document pooling.
