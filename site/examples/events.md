---
title: Events
description: "Subscribe to persistence lifecycle events. Cache invalidation, audit logs, and downstream side effects — the canonical pattern."
---

# Events

The [`example/events`](https://github.com/asaidimu/go-anansi/tree/main/example/events)
directory shows how to subscribe to persistence lifecycle events and react
to creates, updates, and deletes.

## What it shows

- Subscribing to `DocumentCreateSuccess`, `DocumentUpdateStart`,
  `DocumentDeleteSuccess`.
- Using the event payload (`event.Input`) for audit logging.
- Unsubscribing via the returned token.
- Wiring events + persistence together through `PlaygroundConfig`.

## How to run

```bash
cd example/events
ANANSI_ENV=development go run .
```

You should see audit log lines for every CRUD operation, printed by the
event subscriber.

## Read next

- [Guide: Events & subscriptions](/guides/events-subscriptions)
- [Guide: Observability](/guides/observability) — Stats(), event
  counters, realtime via events + your own transport.
- [Guide: Decorators](/guides/decorators) — for *interception* (auth,
  validation) as opposed to *notification*.
