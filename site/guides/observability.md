---
title: Observability
description: "Operate — Stats() and event counters, realtime via events + your own transport, cleanup checklist."
---
Read this when you're **operating or hardening** a running service: exporting
metrics, wiring events for observability, building realtime features, and the
goroutine/cache cleanup checklist.

---

## What metrics can I collect?

There's no built-in metrics SDK, but two things surface numbers:

- **Cache stats**: `cache.NewManagedCache` exposes a `Stats()` snapshot
  (hits/misses/entries/evictions/...) designed "suitable for export to metrics
  systems". Poll it periodically and forward to your metrics reporter.
- **Event counters**: the event bus (go-events v2, in-memory by default) is the
  observability seam. The
  lifecycle emits: `DocumentCreateSuccess/Failed`, `DocumentReadSuccess/Failed`,
  `DocumentUpdateSuccess/Failed`, `DocumentDeleteSuccess/Failed`,
  `TransactionStart/Success/Failed`. Count events (+ errors) at the bus to drive
  CRUD rate/latency/error dashboards. Each event carries `Collection`,
  `TransactionID`, etc.

So: turn on events, subscribe to count, and export `Stats()`. That's your
metrics story without extra deps.

---

## Can I build realtime systems with anansi?

Yes, but **not via built-in websockets.** Anansi is async/evented at the
persistence layer — there is no socket server in the box. What realtime you
get is built on events:

- **Subscribe to a collection's lifecycle events** with
  `coll.Subscribe(ctx, base.SubscriptionOptions{ Event: ..., Callback: ... })`
  (remember to `Unsubscribe`). Subscriptions are **live-only by default**; set
  `Replay: true` (and optionally `ReplayCursor`) to have already-published
  history delivered first. The CRUD events fire on every mutation.
- Aggregate those into your own push layer (websocket/SSE/gRPC stream) in app
  code: on `DocumentCreateSuccess/UpdateSuccess`, broadcast the returned
  document to clients subscribed to that collection.
- `LiveCollection` is the server-side "reactive view" half: a processor that
  turns documents into artifacts and keeps them fresh as writes come in — feed
  reads from `Get` and push the same artifacts on events. (See
  `/guides/caching` for the full `LiveCollection` contract.)

So the shape is: **persistence is synchronous, eventing makes it reactive, and
you bridge events → transport.** The eventing half is Anansi's responsibility;
the transport is yours.

---

## Realtime lifecycle & cleanup checklist

- Call `defer cleanup` / `Close()` on `Playground` (DB + bus) and on
  `LiveCollection`/`ModelCollection` with a managed cache (background
  goroutines: janitor, watermark evictor).
- Always `Unsubscribe` sockets/leases to avoid goroutine leaks (`goleak` runs
  in `make test`).
- `Release()` documents you hold from the persistence layer; let the
  `ModelCollection` release the pooled containers it consumed. (See
  `/guides/caching` for what failing to `Release` costs.)