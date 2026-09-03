---
title: "Code generation basics"
description: "Run anansi codegen golang to emit structs, a constructor, a typed collection wrapper, and the idempotent singleton. What the generated file looks like, where to put custom code."
---

# Code generation basics

With a schema in place, generate the Go code:

```bash
anansi codegen golang
```

This emits `products.go` (into the package configured in your `anansi.json`)
containing a struct, a constructor, a typed collection wrapper, and the
singleton wiring.

## What you get

The generated file looks roughly like this — the exact shape depends on
whether your schema has been normalized (has `_id_`/`_metadata_` declared):

```go
// products.go — generated, do not edit by hand.

package models

import (
    "github.com/asaidimu/go-anansi/v8/core/document"
    "github.com/asaidimu/go-anansi/v8/core/persistence/base"
    "github.com/asaidimu/go-anansi/v8/core/persistence/collection"
    "go.uber.org/zap"
)

type Product struct {
    document.DocumentModel
    ID       string           `anansi:"_id_,required=true" json:"_id_"`
    Name     string           `anansi:"name,required=true" json:"name"`
    Price    float64          `anansi:"price,required=true" json:"price"`
    Stock    int64            `anansi:"stock,required=true" json:"stock"`
    Metadata *ProductMetadata `anansi:"_metadata_,required=false" json:"_metadata_,omitempty"`
}

// ProductMetadata is the typed struct derived from the injected _metadata_
// nested schema (checksum, created, updated, signature, version) — NOT
// map[string]any.
type ProductMetadata struct {
    Checksum  string    `anansi:"checksum,required=false" json:"checksum,omitempty"`
    Created   time.Time `anansi:"created,required=true"  json:"created"`
    Updated   time.Time `anansi:"updated,required=true"  json:"updated"`
    Signature []byte    `anansi:"signature,required=false" json:"signature,omitempty"`
    Version   string    `anansi:"version,required=true"  json:"version"`
}

func NewProduct(model Product) *Product { return document.New(&model) }

type Products struct{ *collection.ModelCollection[*Product] }

const ProductsCollectionName = "Product"

// InitProductsModel wires the collection into the persistence object.
// Idempotent — safe to call multiple times.
func InitProductsModel(p base.Persistence, logger *zap.Logger) (*Products, error)

// ProductsModel returns the wired singleton. Panics if Init was not called.
func ProductsModel() (*Products, error)
```

### Key points

- The struct always embeds `document.DocumentModel` — so it satisfies
  `data.DocumentModelProvider` and works with the shape methods.
- The generated `ID` and `Metadata` fields are **ordinary fields** when the
  on-disk schema declares `_id_`/`_metadata_` (which `migrate generate`
  ensures). Before normalization, codegen falls back to **shadow fields** —
  see [Schema format](/reference/schema-format) for the full rules.
- `Products` wraps a `*collection.ModelCollection[*Product]` — that's where
  CRUD, validation, caching, and the shape methods live.
- `InitProductsModel` and `ProductsModel` form the **idempotent singleton**
  pattern. Call `InitProductsModel(p, logger)` once at startup; later code
  calls `ProductsModel()` to fetch the wired instance.

### Where did `CreateProduct` / `FindProducts` come from?

The codegen emits **promoted methods** on `Products` that delegate to the
underlying `ModelCollection[*Product]`. The exact method names depend on the
schema name; for a `Product` schema, you'll typically see:

| Generated method | Underlying call |
| --- | --- |
| `CreateProduct(ctx, *Product)` | `Create(ctx, doc)` |
| `CreateProducts(ctx, []*Product)` | `CreateMany(ctx, docs)` |
| `GetProduct(ctx, id)` | `FindByID(ctx, id)` |
| `ListAllProducts(ctx)` | `Read(ctx, nil)` |
| `FindProducts(ctx, *query.Query)` | `Read(ctx, q)` |
| `UpdateProduct(ctx, id, *Product)` | `Update(ctx, id, partial)` |
| `DeleteProduct(ctx, id)` | `DeleteByID(ctx, id)` |

These are convenience wrappers — you can always drop down to the underlying
`ModelCollection` methods if you need finer control.

## Codegen modes

```bash
anansi codegen golang                # full mode: structs + collection + singleton
anansi codegen golang --mode structs # DTOs only, no persistence wrapper
anansi codegen golang --mode model   # model + projections, no collection
anansi codegen typescript            # mirror TS types from the same schemas
anansi codegen faker                 # fake data from the schemas
```

Use `--mode structs` when you only need DTOs (e.g. for an API layer that
doesn't talk to the DB). Use `--mode model` when you want the model and
projections but plan to wire the collection yourself.

See [Codegen modes](/reference/codegen-modes) for the full spec.

## Where to put custom code

**The generated file is overwritten on every codegen run.** Extend the model
in separate files (`products_utils.go`) — never edit the generated file.

```go
// products_utils.go — your custom code, safe across codegen runs.

package models

import "context"

// ExpensiveProducts returns products priced above the threshold.
func (p *Products) ExpensiveProducts(ctx context.Context, threshold float64) ([]*Product, error) {
    q := query.NewQueryBuilder().Where("price").Gt(threshold).Build()
    return p.FindProducts(ctx, &q)
}
```

## Naming gotcha

The collection wrapper is the schema name + `s` — schema `User` → `Users`,
but schema `Products` → `Productss` (the `s` is appended unconditionally,
no de-duplication). The collection name constant always equals the schema
name (`ProductssCollectionName = "Products"`), so wiring a collection for a
schema named `Products` uses `Productss` / `ProductssModel()`.

If this bothers you, name your schema `Product` (singular) — the wrapper
becomes `Products`, which reads naturally.

## Next

[CRUD with generated models →](/tutorial/crud-with-models) — wire persistence
and run your first `CreateProduct` / `GetProduct` / `FindProducts` /
`UpdateProduct`.
