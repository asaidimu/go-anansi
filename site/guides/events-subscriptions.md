---
title: "Events & subscriptions"
description: "Subscribe to persistence lifecycle events — DocumentCreateStart, DocumentCreateSuccess, and friends. Route cache invalidation, metrics, and audit through the event bus."
---

# Events & subscriptions

Every persistence operation emits lifecycle events on a generic event bus.
Subscribe to react to creates, updates, deletes — for cache invalidation,
audit logs, downstream side effects, or push notifications.

## Subscribing

```go
ctx := context.Background()

unsub := productsModel.Subscribe(ctx, base.SubscriptionOptions{
    Event: base.DocumentCreateStart,
    Callback: func(ctx context.Context, event base.PersistenceEvent) error {
        logger.Info("create starting",
            zap.String("type",       string(event.Type)),
            zap.String("collection", *event.Collection),
            zap.Any("input",         event.Input))
        return nil
    },
})
defer productsModel.Unsubscribe(ctx, unsub)
```

`Subscribe` returns an opaque token. Pass it to `Unsubscribe` to stop
receiving — important if your subscriber holds resources (a channel, a
goroutine, an open file).

## The full event enumeration

Events live in `core/persistence/base/types.go` as `PersistenceEventType`
constants. The string values are wire-stable — they're what gets serialized
when events cross a bus boundary.

| Constant | Wire value | When fired |
| --- | --- | --- |
| `DocumentCreateStart` | `document:create:start` | Just before a create attempt. |
| `DocumentCreateSuccess` | `document:create:success` | After a document was successfully created. |
| `DocumentUpdateStart` | `document:update:start` | Just before an update operation begins. |
| `DocumentUpdateSuccess` | `document:update:success` | After an update committed. |
| `DocumentDeleteStart` | `document:delete:start` | Just before a delete operation begins. |
| `DocumentDeleteSuccess` | `document:delete:success` | After a delete committed. |

There are also specialized event types for non-document-lifecycle concerns:

| Type | When fired |
| --- | --- |
| `TelemetryEvent` | Arbitrary telemetry publication. |
| `PersistenceOperationEvent` | Document-level operations (create/update/delete). |
| `MigrationEvent` | Schema migration operations. |
| `RollbackEvent` | Schema rollback operations. |
| `TransactionEvent` | Database transaction operations. |

The `PersistenceEvent` struct carries the event `Type`, the source
`Collection` name (as `*string`), the `Input` document (for create/update),
and the result (for success events).

## The callback contract

```go
type EventCallbackFunction func(ctx context.Context, event PersistenceEvent) error
```

The callback receives the original context (with any tracing/cancellation
propagated) and the event. Return an error to signal failure — the bus
logs it but does NOT propagate it back to the caller. Events are
**fire-and-forget**: returning an error does not roll back the operation
that fired the event.

If you need to roll back on failure, use a [decorator](/guides/decorators)
instead. Decorators wrap the call and can short-circuit it; events cannot.

## Multi-event subscriptions

You can subscribe to multiple events by calling `Subscribe` once per event.
The bus dispatches in registration order; there's no built-in fan-out
parallelism. If you need parallel dispatch, fan out inside your callback:

```go
callback := func(ctx context.Context, event base.PersistenceEvent) error {
    go handleAsync(event)  // your own goroutine
    return nil
}
```

## Realtime

Anansi has **no built-in socket transport**. Realtime is events + your own
transport — subscribe to lifecycle events and push them through WebSockets,
SSE, NATS, or whatever your service already speaks.

```go
unsub := productsModel.Subscribe(ctx, base.SubscriptionOptions{
    Event: base.DocumentCreateSuccess,
    Callback: func(ctx context.Context, event base.PersistenceEvent) error {
        // Marshal event.Input to your wire format and push to clients.
        return hub.Broadcast(event.Input)
    },
})
```

See [Observability](/guides/observability) for the full pattern including
metrics counters, Stats(), and the cleanup checklist.

## Cross-cutting concerns

Events are best for *notification*. For *interception* (auth, validation,
encryption, audit) use [Decorators](/guides/decorators) — they wrap the
Persistence or Collection and can short-circuit the call, return an error,
or transform the input before it reaches the database.

The rule of thumb:

| Want to... | Use |
| --- | --- |
| ...react after the fact (logs, metrics, push) | Events |
| ...prevent the operation (auth, validation) | Decorators |
| ...transform the input (encrypt, sanitize) | Decorators |
| ...observe the result (cache invalidation) | Events |

## Cleanup

Always pair `Subscribe` with `Unsubscribe` (typically via `defer`). The bus
holds a strong reference to the callback; if you forget to unsubscribe, the
callback (and anything it closes over) leaks for the lifetime of the
process.

For request-scoped subscriptions, use a context with a timeout and
unsubscribe when the context expires.

## Related

- [Decorators](/guides/decorators) — for interception, not notification.
- [Observability](/guides/observability) — Stats(), event counters, realtime.
- [Persistence setup](/guides/persistence-setup) — wiring the event bus.
