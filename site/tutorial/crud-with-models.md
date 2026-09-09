---
title: "CRUD with generated models"
description: "Wire Playground persistence, bind the generated model, and run Create, FindByID, Read with a query, Update, Delete. The core loop against the promoted ModelCollection methods."
---

# CRUD with generated models

## Wire persistence

Dev / experiments use `Playground` — in-memory SQLite, optional logging,
events, and sanitization:

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/asaidimu/go-anansi/v8"
    "github.com/asaidimu/go-anansi/v8/core/common"
    "github.com/asaidimu/go-anansi/v8/core/document"
    "github.com/asaidimu/go-anansi/v8/core/persistence/base"
    "github.com/asaidimu/go-anansi/v8/core/query"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewDevelopment()
    defer logger.Sync()

    p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
        DBPath:        ":memory:",   // or "file.db" for persistent storage
        EnableLogging: true,
        EnableEvents:  true,
        Schemas:       schemas,      // []*definition.Schema from your registry
    })
    if err != nil {
        log.Fatalf("playground: %v", err)
    }
    defer cleanup()

    // Wire the generated model. Idempotent — safe to call multiple times.
    if _, err := products.InitProductsModel(p, logger); err != nil {
        log.Fatalf("init products model: %v", err)
    }
    productsModel, err := products.ProductsModel()
    if err != nil {
        log.Fatalf("products model: %v", err)
    }

    // Subscribe to a lifecycle event.
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    unsub := productsModel.Subscribe(ctx, base.SubscriptionOptions{
        Event: base.DocumentCreateStart,
        Callback: func(ctx context.Context, event base.PersistenceEvent) error {
            logger.Info("create starting",
                zap.String("collection", *event.Collection),
                zap.Any("input", event.Input))
            return nil
        },
    })
    defer productsModel.Unsubscribe(ctx, unsub)

    // --- Create ---
    laptop := document.New(&products.Product{
        Name:  "Laptop",
        Price: 1200.00,
        Stock: 50,
    })
    laptop, err = productsModel.Create(ctx, laptop)
    if err != nil {
        log.Fatalf("create: %v", common.SystemErrorFrom(err).ToIssue())
    }
    logger.Info("created", zap.String("id", laptop.ID))

    // --- CreateMany ---
    created, err := productsModel.CreateMany(ctx, []*products.Product{
        {Name: "Mouse", Price: 25.00, Stock: 200},
        {Name: "Keyboard", Price: 75.00, Stock: 150},
    })
    if err != nil {
        log.Fatalf("create many: %v", err)
    }
    _ = created

    // --- Find by ID ---
    found, err := productsModel.FindByID(ctx, laptop.ID)
    if err != nil {
        log.Fatalf("get: %v", err)
    }
    _ = found

    // --- Query (price > 100) ---
    q := query.NewQueryBuilder().Where("price").Gt(100.0).Build()
    expensive, err := productsModel.Read(ctx, &q)
    if err != nil {
        log.Fatalf("query: %v", err)
    }
    logger.Info("expensive", zap.Int("count", len(expensive)))

    // --- Partial update (zero fields skipped) ---
    updated, err := productsModel.Update(ctx, laptop.ID, &products.Product{
        Stock: 45, // only stock is updated
    })
    if err != nil {
        log.Fatalf("update: %v", err)
    }
    _ = updated

    // --- Delete ---
    if err := productsModel.DeleteByID(ctx, laptop.ID); err != nil {
        log.Fatalf("delete: %v", err)
    }
}
```

## The CRUD methods

Codegen generates no per-schema method names. `Products` embeds
`*collection.ModelCollection[*Product]`, so every method below is a promoted
`ModelCollection` method working directly on `*Product`:

| Method | Behaviour |
| --- | --- |
| `Create(ctx, *Product) (*Product, error)` | Persist; return hydrated struct with ID generated. |
| `CreateMany(ctx, []*Product) ([]*Product, error)` | Batch create. |
| `FindByID(ctx, id) (*Product, error)` | Read by ID (id-cache aware when caching is configured). |
| `Read(ctx, *query.Query) ([]*Product, error)` | Read with a query. |
| `Update(ctx, id, *Product) (*Product, error)` | Partial update; zero fields skipped. |
| `Replace(ctx, id, *Product) (*Product, error)` | Full replace. |
| `DeleteByID(ctx, id) error` | Delete by ID. Evicts cache. |
| `Validate(ctx, *Product, loose) error` | Schema validation. |

When you need finer control (raw results, status codes, batched writes),
grab the raw `base.Collection` from `p.Collection(ctx, "Product")` — same
collection, unwrapped.

## The raw collection

For document-shaped access — scripting, bulk scans, generated queries — no
separate handle needed. `Products` embeds `base.Collection`, so the model
itself satisfies the raw interface:

```go
var coll base.Collection = productsModel // the model IS a raw collection

// Read returns pooled documents that are YOURS to Release().
result, err := coll.Read(ctx, &query.Query{})
defer func() {
    for _, doc := range result.Data {
        doc.Release()
    }
}()

// CreateOne returns a status, not just an error.
// Build the document through the collection's own pool — never data.MustNewDocument (deprecated).
pool, err := coll.DocumentPool(ctx)
if err != nil { /* ... */ }
doc, err := pool.FromMap(map[string]any{
    "name":  "Mouse",
    "price": 25.00,
    "stock": 200,
})
if err != nil { /* validation failure */ }
defer pool.Release(doc)
res, err := coll.CreateOne(ctx, doc)
if err != nil { /* driver failure */ }
switch res.Status {
case base.StatusCreated:
    // ok
case base.StatusFailedValidation:
    // res.Issues has the validation errors
case base.StatusFailedPersistence:
    // interactor call failed
}
```

Two differences from the model-typed methods on the same object:

- **Raw `Read` returns pooled documents that are yours to `Release()`.** The
  model-typed methods release internally; raw `Read` does not. Forgetting to
  release causes memory pressure under load.
- **Raw `CreateOne` returns a status, not just an error.** Check
  `res.Status` (`StatusCreated` / `StatusFailedValidation` /
  `StatusFailedPersistence`) — validation failures come back as a status,
  not an error.

## Closing a collection vs closing the store

`productsModel.Close()` stops the model's cache background goroutines. It's
a **no-op when no cache is configured** (the default — a cache is only
created if you pass `Cache` / `CacheConfig` to `Init<X>Model`). It never
touches the embedded collection or the DB. (The raw `base.Collection` has
no `Close` method — closing is a model-wrapper concern.)

The **store** is closed separately: `cleanup()` from `Playground`, or closing
your `DatabaseInteractor`. Don't conflate the two.

See [Collection internals](/reference/collection-internals) for the full
layer-by-layer request path.

## Error handling

Anansi uses `common.SystemError` to wrap low-level errors with richer context.
Convert any returned error:

```go
if err != nil {
    sysErr := common.SystemErrorFrom(err)
    log.Fatalf("operation failed: %v", sysErr.ToIssue())
}
```

`ToIssue()` returns a `common.Issue` with a code, message, and severity —
useful for structured logging or returning to API callers.

## Next

[Projections →](/tutorial/projections) — read and write field subsets
through the generic shape methods `ReadAs[R]`, `CreateFrom[R]`,
`UpdateFrom[R]`.
