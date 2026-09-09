---
title: Basic CRUD
description: "The smallest end-to-end Anansi demo. Schema declaration, codegen, typed CRUD with ModelCollection. The template the tutorial is based on."
---

# Basic CRUD

The [`example/basic`](https://github.com/asaidimu/go-anansi/tree/main/example/basic)
directory contains the smallest end-to-end Anansi demo: three schemas
(`products`, `users`, `carts`), codegen output, and a `main.go` that wires
`Playground`, runs CRUD, and queries with the DSL.

## What it shows

- Declaring multiple schemas in one project.
- Running `anansi codegen golang` to emit structs + collection wrappers.
- Wiring `anansi.Playground` with in-memory SQLite.
- Calling `Create`, `FindByID`, `Read` (with a `QueryBuilder`), `Update`.
- Subscribing to a lifecycle event.

## How to run

```bash
cd example/basic
ANANSI_ENV=development go run .
```

You should see log lines for each CRUD operation and the event subscription
firing on create.

## Read next

- [Tutorial: CRUD with generated models](/tutorial/crud-with-models)
- [Reference: Query DSL](/reference/query-dsl)
- [Reference: Collection internals](/reference/collection-internals)
