# Decorators: cross-cutting concerns

Read this when you're **hardening or extending** a collection with
cross-cutting behavior — row-level security, validation, auditing, encryption,
cache busting — without touching the collection's own logic. Verified against
the framework source.

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
— see `references/persistence-setup.md`.
