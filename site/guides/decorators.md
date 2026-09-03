---
title: Decorators
description: "Harden cross-cutting concerns — utils.Decorators for RLS, audit, validation, encryption."
---
Read this when you're **hardening or extending** a collection with
cross-cutting behavior — row-level security, validation, auditing, encryption,
cache busting — without touching the collection's own logic.

---

## What are decorators?

Decorators are **middleware wrapped around every collection operation** — the
prescribed place for cross-cutting concerns: row-level security, validation,
auditing, encryption, cache busting. They are `func(next base.Collection)
base.Collection`: you build a wrapper struct that embeds `base.Collection`,
override the methods you care about, and delegate the rest.

```go
type NegativeAmountValidator struct {
    base.Collection
    logger *zap.Logger
}

func (d *NegativeAmountValidator) CreateOne(ctx context.Context, doc *data.Document) (base.CreateResult, error) {
    if amt, err := doc.GetInt("amount"); err == nil && amt < 0 {
        return base.CreateResult{Status: base.StatusFailedValidation, Data: doc}, errors.New("amount cannot be negative")
    }
    return d.Collection.CreateOne(ctx, doc)   // delegate to the inner collection
}

func NegativeAmountValidator(logger *zap.Logger) utils.CollectionDecorator {
    return func(next base.Collection) base.Collection {
        return &NegativeValidator{Collection: next, logger: logger}
    }
}
```

Wire them through `utils.Decorators`:

```go
decorators := &utils.Decorators{
    PersistenceDecorators: []utils.DecoratorFunc[base.Persistence]{ ... }, // wrap the whole facade
    CollectionDecorators:  []utils.DecoratorFunc[base.Collection]{
        NegativeAmountValidator(logger),
    },
}
```

Internally a `DecoratorFunc[T]` is just `func(T) T`, and `ApplyDecorators`
chains them left-to-right — the first decorator becomes the outermost layer.
Subscriptions and all CRUD flow through the decorated collection, so a
validator that fails `CreateOne` also stops bad writes from inside a
`ModelCollection`.

Wire the `*utils.Decorators` into `anansi.Setup(SetupConfig{ Decorators: ... })`
— see `/guides/persistence-setup`.
## A worked example: audit log decorator

Audit logs are the canonical use case for a collection decorator. They need
to fire on every mutation, capture who did what, and not be skippable by
business logic that bypasses a method.

```go
package audit

import (
    "context"
    "time"

    "github.com/asaidimu/go-anansi/v8/core/data"
    "github.com/asaidimu/go-anansi/v8/core/persistence/base"
    "github.com/asaidimu/go-anansi/v8/utils"
    "go.uber.org/zap"
)

type AuditLog struct {
    base.Collection
    logger *zap.Logger
}

func (d *AuditLog) CreateOne(ctx context.Context, doc data.Documenter) (base.CreateResult, error) {
    res, err := d.Collection.CreateOne(ctx, doc)
    actor := ctx.Value(actorKey{}).(string)
    d.logger.Info("audit: create",
        zap.String("actor", actor),
        zap.String("id", res.DocumentID),
        zap.Any("doc", doc),
        zap.Time("at", time.Now().UTC()),
        zap.Error(err))
    return res, err
}

func (d *AuditLog) Update(ctx context.Context, params *base.CollectionUpdate) (*base.ReadResult, error) {
    res, err := d.Collection.Update(ctx, params)
    actor := ctx.Value(actorKey{}).(string)
    d.logger.Info("audit: update",
        zap.String("actor", actor),
        zap.Any("filter", params.Filter),
        zap.Int("count", res.Count),
        zap.Time("at", time.Now().UTC()),
        zap.Error(err))
    return res, err
}

// AuditLog wires the decorator. Pass this to utils.Decorators.CollectionDecorators.
func AuditLog(logger *zap.Logger) utils.DecoratorFunc[base.Collection] {
    return func(next base.Collection) base.Collection {
        return &AuditLog{Collection: next, logger: logger}
    }
}
```

Wire it:

```go
decorators := &utils.Decorators{
    CollectionDecorators: []utils.DecoratorFunc[base.Collection]{
        audit.AuditLog(logger),
        // other decorators — order matters: leftmost is outermost
    },
}

p, err := anansi.Setup(anansi.SetupConfig{
    Interactor: interactor,
    Logger:     logger,
    Decorators: decorators,
    Schemas:    schema.GetSchemas(),
})
```

## Order matters

Decorators chain left-to-right; the first decorator in the slice becomes the
outermost layer. For audit + encryption, decide whether audit should log the
plaintext (audit outer) or ciphertext (audit inner):

```go
CollectionDecorators: []utils.DecoratorFunc[base.Collection]{
    audit.AuditLog(logger),        // outermost — sees plaintext, logs plaintext
    encrypt.EncryptFields(keys),   // inner — encrypts before hitting DB
}
```

vs:

```go
CollectionDecorators: []utils.DecoratorFunc[base.Collection]{
    encrypt.EncryptFields(keys),   // outer — encrypts first
    audit.AuditLog(logger),        // inner — sees ciphertext, logs ciphertext
}
```

Pick deliberately. Logging plaintext is more useful for forensics; logging
ciphertext is safer if audit logs leave the trust boundary.

## When to use a decorator vs an event

| Want to... | Use |
| --- | --- |
| ...prevent the operation (auth, validation) | Decorator |
| ...transform the input (encrypt, sanitize) | Decorator |
| ...observe the result (audit, metrics, push) | Either, but events are simpler |
| ...short-circuit on failure | Decorator (events can't) |

Events are fire-and-forget — they cannot prevent the operation. Decorators
can. See [Events & subscriptions](/guides/events-subscriptions) for the full
distinction.

## Related

- [Persistence setup](/guides/persistence-setup) — wiring decorators into
  `anansi.Setup`.
- [Sanitization](/guides/sanitization) — the built-in sanitization layer is
  itself a decorator that ships with the library.
- [Events & subscriptions](/guides/events-subscriptions) — when to use
  events instead.
- [Collection internals](/reference/collection-internals) — where decorators
  sit in the request path.
