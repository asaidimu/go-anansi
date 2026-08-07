# Transactions: atomic multi-operation units of work

Read this when the user needs **atomicity across multiple writes** — a transfer,
an order + line items, a bulk upsert where "all or nothing" matters. Verified
against the framework source (`core/persistence/transaction/transaction.go`,
`core/persistence/persistence/base.go`, `core/persistence/collection/base.go`).

---

## Two entry points

There are two ways in, both thin wrappers over the same core engine
(`transaction.Execute`):

1. **Persistence facade** — `p.Transact(ctx, func(tctx, tx BasePersistence))`.
   Best for units of work spanning multiple collections.
2. **Collection level** — `coll.Transact(ctx, func(tctx) (any, error))`.
   Atomic within (or across) calls made with the given context. `ModelCollection`
   gets this through its embedded `base.Collection`; `managedCollection` and
   decorators forward to the wrapped collection.

Both: **return an error → rollback; return nil → commit.** Error or success,
the transaction is finalized exactly once.

### Persistence facade (multi-collection)

```go
p.Transact(ctx, func(tctx context.Context, tx base.BasePersistence) (any, error) {
    accounts, _ := tx.Collection(tctx, "accounts")   // reads AND writes via tx
    accounts.Update(tctx, &base.CollectionUpdate{Set: debitAlice,  Filter: ...})
    accounts.Update(tctx, &base.CollectionUpdate{Set: creditBob,  Filter: ...})
    return nil, nil                                  // commit
})
```

`tx` is a fresh `BasePersistence` re-bound to the transactional interactor —
**use it, not `p`, for every read and write inside the callback.** On
interactors whose `Capabilities().RequiresTransactionSerialization` is set, the
facade holds a mutex so overlapping top-level transactions don't interleave.

### Collection level (single collection, or joining)

```go
_, err := coll.Transact(ctx, func(tctx context.Context) (any, error) {
    coll.CreateOne(tctx, order)            // must pass tctx — that's what joins
    coll.CreateMany(tctx, []data.Documenter{lineA, lineB})
    return nil, nil
})
```

Note: inside the callback you can use the **same `coll`** — the context carries
the transaction, so any operation given `tctx` runs on the transactional
interactor (`getCurrentInteractor` picks it up automatically).

---

## Core engine: `transaction.Execute`

Both APIs call `transaction.Execute(ctx, interactor, logger, callback)`
(`transaction.go:179`). Its rules:

- **Already inside a transaction?** `GetCurrentTransaction(ctx)` finds one in
  the context and the callback **joins it** — it's registered via
  `AddOperation()` and runs on the existing transaction. No new DB transaction
  is opened, so **nesting is free and flat**, and the outer caller decides
  commit/rollback.
- **Not inside one?** It calls `interactor.StartTransaction(ctx)`, wraps the new
  interactor in a `base.Transaction`, runs the callback, then decides the
  outcome. Commit happens only if **both** the callback returned nil **and** no
  async operation reported an error; otherwise it rolls back.

### Context propagation is the golden rule

The transaction travels **in the context** under `transaction.TxKey`
(`GetCurrentTransaction`). That's the only link between a transaction and the
operations that join it. So:

> **Pass the transaction context into every inner read/write.** An operation
> called with the original `ctx` silently runs outside the transaction.

If you need the transaction handle explicitly (for hooks, concurrency, the raw
interactor), read it back:

```go
tx, ok := transaction.GetCurrentTransaction(tctx)  // base.Transaction
tx.GetInteractor()                                 // query.DatabaseInteractor (transactional)
tx.IsActive()                                      // not yet committed/rolled back
```

### Concurrent operations inside one transaction

`base.Transaction` coordinates parallel work. `AddOperation()` returns a cleanup
func that reports the operation's error; commit waits for **all** operations
and the whole transaction rolls back if **any** fails:

```go
tx, _ := transaction.GetCurrentTransaction(tctx)
done := tx.AddOperation()
go func() { done(someAsyncWork()) }()
```

`WaitForOperations(ctx)` blocks until they finish (or the context is done,
yielding `ErrTransactionTimeout`).

### Hooks

```go
tx, _ := transaction.GetCurrentTransaction(tctx)
tx.OnCommit(func() { invalidateCaches() })   // after successful commit
tx.OnRollback(func() { notifyFailure() })    // after rollback
```

Hooks run once, in order, after finalize; both lists are cleared afterwards.

---

## What events fire?

The lifecycle events (`base.TransactionStart`, `base.TransactionSuccess`,
`base.TransactionFailed`, each carrying `TransactionID`) are emitted around the
transaction. Critically, **CRUD emissions inside a transaction are queued and
only published at commit** (`withEventEmission`) — a rolled-back write never
leaks a success event. Reads run on the current interactor (the transactional
one if inside), never emitting from the write path.

---

## Error semantics

| Sentinel | Meaning |
|---|---|
| `ErrTransactionTimeout` | context deadline hit while waiting on ops |
| `ErrTransactionAlreadyFinalized` | commit/rollback on a finalized tx |
| `ErrTransactionNoActive` | finalize with no active DB transaction |
| `ErrTransactionAsyncOperationFailed` | an async op reported an error |
| `ErrTransactionCommitFailed` | commit failed (then rollback attempted) |
| `ErrTransactionFailed` | generic failure (e.g. rollback + callback both errored) |

All in `core/persistence/base/errors.go`; on a commit failure the engine also
attempts a rollback before returning.

---

## Pointers & gotchas

- **Use `tx`/`tctx`, not the outer `ctx`/`p`**, inside the callback — this is
  the #1 source of "why wasn't this atomic?" bugs.
- **Nesting is flat join, not savepoints** — an inner `Transact` that returns an
  error reports it to the parent, and the parent decides final outcome.
- **Short transactions.** The whole point is atomicity; holding a transaction
  across slow I/O or user interaction blocks writers on serializing backends.
- `data.ConfigureDocumentFactory` and schema setup are unrelated to this —
  transactions operate on already-created collections.
