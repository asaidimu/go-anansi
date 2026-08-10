> # ⚠️ ALPHA
>
> **Go-Anansi is in alpha.** The API is unstable and may change without notice.
> Backward compatibility is not guaranteed between releases — expect breaking
> changes, rough edges, and missing documentation. Not recommended for
> production use yet. Feedback and bug reports are welcome.

# Go-Anansi: A Schema-Driven, Hybrid Persistence Layer for Go

![Go Version](https://img.shields.io/badge/Go-1.27%2B-00ADD8?style=for-the-badge&logo=go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen?style=for-the-badge)
![Status: Alpha](https://img.shields.io/badge/Status-Alpha-orange)

Go-Anansi is a schema-driven persistence framework for Go. You declare your
data model once as a JSON schema, and Go-Anansi:

- **Persists and queries** documents against a SQLite backend through a hybrid
  query engine that pushes what it can to SQL and runs the residual in memory.
- **Validates, migrates, and events** every document through a pluggable
  pipeline built on the decorator pattern.
- **Generates type-safe Go code** (`codegen`) — structs, enums, projections,
  and a typed collection wrapper (`collection.ModelCollection[P]`) backed by
  generic shape methods.

The design goal is an **unopinionated core**: schemas are the single source of
truth, and everything else (migrations, caching, custom business logic,
observability) is injected by you.

## Quick Links

- [Key Features](#key-features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Projections](#projections)
- [Command-Line Interface](#command-line-interface)
- [Query DSL](#query-dsl)
- [Events & Decorators](#events--decorators)
- [Documentation](#documentation)
- [Project Architecture](#project-architecture)
- [Development & Contributing](#development--contributing)
- [License](#license)

---

## Key Features

- **Schema-driven development** — one JSON schema per collection is the single
  source of truth for storage, validation, migrations, and code generation.
- **Type-safe code generation** — `anansi codegen golang` emits Go structs
  (embedding `data.DocumentModel`), enums, and a typed
  `collection.ModelCollection[P]` wrapper with CRUD, validation, caching, and
  subscribe/unsubscribe built in.
- **Projections** — schema-declared field subsets with `required` overrides
  and custom struct tags; read/write any shape through generic methods
  (`ReadAs`, `CreateFrom`, `UpdateFrom`).
- **Hybrid query engine** — a `Query` DSL (filtering, sorting, pagination,
  projection, joins, aggregations) is partitioned into a database query and an
  in-memory residual query based on the backend's declared capabilities.
- **Declarative validation** — graph-based validator supporting field types,
  constraints, indexes, nested schemas, conditional logic, and circular
  dependency detection.
- **Migration tooling** — `anansi migrate generate` produces migration files
  from schema changes; versioned, squashable.
- **Event-driven** — publish/subscribe on persistence lifecycle events
  (`DocumentCreateSuccess`, `DocumentUpdateStart`, ...).
- **Decorator pattern** — wrap `Persistence`/`Collection` to inject cross-cutting
  concerns (auth, audit, caching, sanitization, custom validation).
- **SQLite reference implementation** — pluggable `DatabaseInteractor`
  interface means other SQL/NoSQL backends can be added.

---

## Requirements

- **Go 1.27 or later.** The generated collection wrapper and the shape methods
  rely on generic methods on concrete types, which require Go 1.27.
- **SQLite.** The driver is bundled; the reference `DatabaseInteractor`
  implementation targets SQLite.

## Installation

```bash
go get github.com/asaidimu/go-anansi/v8@latest
```

> The module is versioned at `v8` (module path
> `github.com/asaidimu/go-anansi/v8`). Older major versions (`v7` and below)
> are unmaintained.

---

## Quick Start

### 1. Declare a schema

Schemas live in JSON files (`.schema.json`). Field map keys are stable UUIDs
(`id` attributes); `name`, `type`, `required`, `nullable`, and `unique`
describe each field.

`schemas/products.schema.json`:

```json
{
  "version": "1.0.0",
  "name": "Products",
  "fields": {
    "019fb22d-a9a1-7e3e-8924-8f09eb81a096": {
      "name": "name",
      "required": true,
      "unique": true,
      "type": "string"
    },
    "019fb22d-a9a1-7e3f-8e2c-3427aaf6b775": {
      "name": "price",
      "required": true,
      "type": "number"
    },
    "019fb22d-a9a1-7e40-b7e2-8ca46b4c788a": {
      "name": "stock",
      "required": true,
      "type": "integer"
    }
  }
}
```

### 2. Generate Go code

```bash
go run github.com/asaidimu/go-anansi/v8/cmd/anansi@latest codegen golang
```

This emits `products.go` (into the package configured in your scaffold), containing:

```go
type Product struct {
    data.DocumentModel
    Name  string  `anansi:"name,required=true" json:"name"`
    Price float64 `anansi:"price,required=true" json:"price"`
    Stock int64   `anansi:"stock,required=true" json:"stock"`
}

// NewProduct creates and initializes a new Product
func NewProduct(model Product) *Product { return document.New(&model) }

// Products is a type-safe collection for Product
type Products struct {
    *collection.ModelCollection[*Product]
}

const ProductsCollectionName = "Products"

// InitProductsModel + ProductsModel (idempotent singleton wiring)
```

Extend the model in separate files (`products_utils.go`) — the generated file
is overwritten on every codegen run.

### 3. Use it

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/asaidimu/go-anansi/v8"
    "github.com/asaidimu/go-anansi/v8/core/data"
    "github.com/asaidimu/go-anansi/v8/core/query"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewDevelopment()
    defer logger.Sync()

    p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
        EnableEvents: true,
        Schemas:      schemas, // []*definition.Schema, from your registry
    })
    if err != nil {
        log.Fatal(err)
    }
    defer cleanup()

    if _, err := products.InitProductsModel(p, logger); err != nil {
        log.Fatal(err)
    }
    productsModel, err := products.ProductsModel()
    if err != nil {
        log.Fatal(err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Create
    laptop := data.New(&products.Product{Name: "Laptop", Price: 1200.00, Stock: 50})
    created, err := productsModel.Create(ctx, laptop)
    if err != nil {
        log.Fatal(err)
    }

    // Find by ID
    found, err := productsModel.FindByID(ctx, created.ID)
    if err != nil {
        log.Fatal(err)
    }

    // Query (price > 100)
    q := query.NewQueryBuilder().Where("price").Gt(100.0).Build()
    results, err := productsModel.Read(ctx, &q)

    // Partial update (zero fields skipped)
    updated, err := productsModel.Update(ctx, created.ID, &products.Product{Stock: 45})

    _ = found
    _ = results
    _ = updated
}
```

### 4. Shape operations (projections)

Projections are declared in schema `metadata` and emitted as structs. They are
read/written through generic shape methods — no projection-specific accessors
are generated; you pick the operation and the shape at the call site.

```json
{
  "version": "1.0.0",
  "name": "Products",
  "fields": { "...": "..." },
  "metadata": {
    "projections": {
      "ProductSummary": { "fields": { "include": ["name", "price"] } },
      "ProductCreate":  { "fields": { "include": ["name", "price", "stock"], "required": ["name", "price"] } }
    }
  }
}
```

```go
// read as a summary shape (only name/price bound)
q := query.NewQueryBuilder().Where("name").Eq("Laptop").Build()
summaries, err := productsModel.ReadAs[*products.ProductSummary](ctx, &q)

// create from a create shape
created, err := productsModel.CreateFrom[*products.ProductCreate](ctx,
    &products.ProductCreate{Name: "Mouse", Price: 25.00, Stock: 200})

// update from an update shape (partial; system fields untouched)
updated, err := productsModel.UpdateFrom[*products.ProductUpdate](ctx, id, &products.ProductUpdate{Stock: 45})
```

---

## Projections

Projections are declared under `metadata.projections.<Name>.fields` in a
schema. Each entry describes a field subset and optional constraint/tag
overrides:

| Key | Meaning |
| --- | --- |
| `include` / `exclude` | Membership (no `include` ⇒ all root fields minus `exclude`) |
| `required` / `optional` | Override `required` (drives value-vs-pointer type + `anansi:"…,required=…"`) |
| `tags` | Custom struct tags per field, with `{prop}` placeholders: `{name}`, `{type}`, `{required}`, `{nullable}`, `{default}`, `{goName}` |

Projection structs embed `data.DocumentModel`, so they satisfy
`data.DocumentModelProvider` and are valid type arguments for the shape
methods `ReadAs[R]`, `CreateFrom[R]`, and `UpdateFrom[R]` on
`*collection.ModelCollection[P]`.

Codegen validates projections **fail-fast**: unknown fields, `include` ∩
`exclude`, `required` ∩ `optional`, and tag references to fields outside the
final set are all errors.

See [`docs/codegeneration-go.md`](docs/codegeneration-go.md) for the full
specification.

---

## Command-Line Interface

The `anansi` CLI (`cmd/anansi`) orchestrates the development loop:

```bash
anansi scaffold [dir]            # Create a new project with default config
anansi codegen golang [glob...]  # Generate Go structs + collection wrappers
anansi codegen typescript        # Generate TypeScript types from schemas
anansi codegen faker             # Generate fake data from schemas
anansi migrate generate          # Generate migration files from schema changes
anansi migrate squash <col>      # Consolidate intermediate migrations
anansi version                   # Print version
```

Configuration lives in an `anansi.yaml` at the project root (defaults: schema
glob `schemas/**/*.schema.json`, lockfile `schemas.lock.json`, migrations in
`migrations/`).

---

## Query DSL

The `core/query` package provides a fluent DSL for building queries, translated
to SQL where the backend supports it:

```go
q := query.NewQueryBuilder().
    From("Orders").
    Where("total").Gt(100.0).
    Sort("createdAt", query.Descending).
    Limit(50).
    Build()
```

The engine partitions each query: the parts the `DatabaseInteractor` supports
run in the backend, the residual runs in-memory. See [`QUERYLANG.md`](QUERYLANG.md)
for the full grammar and its JSON mapping.

---

## Events & Decorators

Every persistence operation emits lifecycle events on a generic event bus:

```go
unsub := productsModel.Subscribe(ctx, base.SubscriptionOptions{
    Event: base.DocumentCreateSuccess,
    Callback: func(ctx context.Context, event base.PersistenceEvent) error {
        logger.Info("created", zap.Any("doc", event.Input))
        return nil
    },
})
defer productsModel.Unsubscribe(ctx, unsub)
```

Cross-cutting concerns are injected with decorators rather than baked into the
core:

```go
p, err := anansi.Setup(anansi.SetupConfig{
    Interactor:    interactor, // core/query/native.NativeInteractor over sqlite
    Logger:        logger,
    EventBus:      bus,
    FactoryConfig: data.DocumentFactoryConfig{},
    Decorators:    &utils.Decorators{ /* your CollectionDecorators */ },
    Schemas:       schemas,
})
```

---

## Documentation

- [`docs/codegeneration-go.md`](docs/codegeneration-go.md) — Go code generation,
  the model collection, and projections.
- [`docs/dto_spec.md`](docs/dto_spec.md) — the `anansi` struct tag spec and
  Go → schema inference.
- [`docs/schema_ir.md`](docs/schema_ir.md) — the schema JSON representation.
- [`docs/schema_address_spec.md`](docs/schema_address_spec.md),
  [`docs/schema_versioning_model.md`](docs/schema_versioning_model.md),
  [`docs/migration_semantics.md`](docs/migration_semantics.md) — schema
  addressing, versioning, and migration semantics.
- [`QUERYLANG.md`](QUERYLANG.md) — the query DSL grammar.
- [`search.md`](search.md) — the pluggable full-text search proposal.
- [`test-gap.md`](test-gap.md) — known test coverage gaps.
- [`example/`](example/) — runnable examples: `basic` (typed CRUD via
  generated models), `api` (REST server), `events`, `migration`, `complex`,
  `benchmark`.

---

## Project Architecture

- **`anansi` (root package)** — `Setup` (production) and `Playground` (dev)
  entry points plus `SetupConfig`/`PlaygroundConfig`.
- **`core/data`** — `Document` (a `map[string]any` with fluent APIs),
  `DocumentModel` (the struct embed providing `id`/`metadata`),
  auto-initialization via `data.New[T]`, binding and sanitization.
- **`core/schema`** (`definition`) — the schema type, graph validator,
  normalization, and migration generation.
- **`core/query`** — the query DSL, `QueryEngine`, partitioning, and the
  `DatabaseInteractor` interface (`core/query/native`).
- **`core/persistence`** — `base.Collection`, `collection.ModelCollection[P]`
  (typed wrapper with caching), transactions, and the collection registry.
- **`core/events`** — the generic publish/subscribe event bus.
- **`sqlite`** — the reference `executor`/`query` implementation.
- **`codegen`** — `golang`, `typescript`, and `faker` generators.
- **`cmd/anansi`** — the CLI.

### Data flow

1. `Persistence.Collection(name)` looks up the schema in the registry and
   instantiates a `Collection`.
2. `Collection.Read(query)` hands a `Query` to the `QueryEngine`.
3. The `QueryEngine` partitions the query: database-supported parts → SQL via
   the `DatabaseInteractor`; the residual → in-memory `QueryHelper`.
4. Results come back as documents, then bind into typed structs
   (`data.DocumentModel` embeds) via `BindToWithContext`.
5. Throughout CRUD, decorators and the event bus observe and can intercept.

---

## Development & Contributing

```bash
git clone https://github.com/asaidimu/go-anansi.git
cd go-anansi
go mod tidy
make build    # compile the project
make test     # run all unit + integration tests
```

Notes for contributors:

- The module currently requires Go 1.27 (generic methods). CI pins the
  matching `rc` toolchain.
- Golden codegen outputs live in `codegen/golang/testdata/`; regenerate with
  `go test ./codegen/golang/ -update`.
- `bin/bump.sh` upgrades the module's major version across the codebase — run
  with `--dry-run` first.
- See [`test-gap.md`](test-gap.md) for areas that need more test coverage.
- Report bugs and feature requests via
  [GitHub issues](https://github.com/asaidimu/go-anansi/issues).

Contributing guidelines: fork, branch, write tests, run `make test`, and open a
pull request with a description of the change.

---

## License

This project is licensed under the MIT License — see the [LICENSE.md](LICENSE.md)
file for details.

## Acknowledgments

Go-Anansi is a product of CyberSync Printers & Stationers — BN-P7SABM5J.
