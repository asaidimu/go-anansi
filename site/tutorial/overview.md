---
title: "Overview"
description: "What Go-Anansi is, who it's for, what you'll build in the tutorial, and the mental model that makes it click."
---

# Overview

Go-Anansi is a **schema-driven data layer for Go**. You declare your data model
once as a JSON schema. Anansi persists it to SQLite, validates and migrates
documents through a pluggable pipeline, and generates type-safe Go code —
structs, enums, projections, and a typed collection wrapper with CRUD,
validation, and subscriptions built in.

The design goal is an **unopinionated core**: schemas are the single source of
truth, and everything else — migrations, caching, custom business logic,
observability — is injected by you.

## What Anansi is — and isn't

Anansi is a **data-layer toolkit**, not an application framework. It gives you
schemas, codegen, a query engine, migrations, and an event bus. It does not
ship an HTTP router, auth middleware, a logger integration, or a frontend. You
bring those.

This scope is deliberate. Anansi does the data layer well and stays out of the
rest. If you want batteries-included, this isn't it.

## Who this is for

- Go developers building services that need persistence, validation, and type
  safety — without an ORM's magic.
- Teams that want schemas to drive codegen across Go and TypeScript from one
  source of truth.
- Projects that need a hybrid query engine — push what you can to SQL, run the
  residual in memory.

## What you'll build

In this tutorial you'll declare a `Products` schema, generate Go models, run
CRUD and projections against an in-memory SQLite, and evolve the schema with a
migration. By the end you'll have the mental model to read the
[Guides](/guides/domain-modeling) and [Reference](/reference/schema-format)
sections productively.

The full tutorial takes about 15–20 minutes if you type the code along.

## The mental model

Three things make Anansi click.

### 1. Schema first, then generate, then code against generated types

Resist the temptation to hand-write document maps. Declare the shape once in a
`.schema.json` file, run `anansi codegen golang`, and reuse the generated
structs. The schema is the source of truth — not the code.

### 2. The generated collection wrapper is your API surface

`anansi codegen golang` emits not just structs but a typed
`collection.ModelCollection[P]` wrapper (`Products` for a `Product` schema).
This wrapper exposes CRUD methods (`CreateProduct`, `FindProducts`,
`UpdateProduct`, and friends), validation, and the projection shape methods
(`ReadAs[R]`, `CreateFrom[R]`, `UpdateFrom[R]`). You code against *that*, not
against raw `map[string]any` documents.

### 3. Partitioning is everywhere

Every query is partitioned: the parts the `DatabaseInteractor` supports run in
the backend (SQL via SQLite), the residual runs in-memory. The same principle
applies to the persistence stack itself — decorators wrap, events observe, the
core stays unopinionated.

## What you'll need

- **Go 1.27 or later.** The generated collection wrapper relies on generic
  methods on concrete types, which require Go 1.27.
- **SQLite.** The driver is bundled; the reference implementation targets
  SQLite.
- **The `anansi` CLI.** Install with
  `go install github.com/asaidimu/go-anansi/v8/cmd/anansi@latest`.

## A 30-second taste

If you want to see the end state before you commit to the tutorial, here's
the loop in full. Each piece is explained in the pages that follow.

```bash
# 1. Install the CLI
go install github.com/asaidimu/go-anansi/v8/cmd/anansi@latest

# 2. Scaffold a project
anansi scaffold myapp && cd myapp

# 3. Declare a schema (in schemas/products.schema.json)
#    — covered in "Your first schema"

# 4. Generate Go code
anansi codegen golang

# 5. Run it
ANANSI_ENV=development go run .
```

```go
// main.go — the parts that matter
p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
    DBPath:        ":memory:",
    EnableLogging: true,
    EnableEvents:  true,
    Schemas:       schemas, // []*definition.Schema from your registry
})
defer cleanup()

if _, err := products.InitProductsModel(p, logger); err != nil {
    log.Fatalf("init products model: %v", err)
}
productsModel, _ := products.ProductsModel()

ctx := context.Background()
created, err := productsModel.CreateProduct(ctx,
    document.New(&products.Product{Name: "Laptop", Price: 1200.00, Stock: 50}))
if err != nil {
    log.Fatalf("create: %v", common.SystemErrorFrom(err).ToIssue())
}

q := query.NewQueryBuilder().Where("price").Gt(100.0).Build()
expensive, err := productsModel.FindProducts(ctx, &q)
```

## Next

[Installation →](/tutorial/installation) — set up the CLI and scaffold your
first project.
