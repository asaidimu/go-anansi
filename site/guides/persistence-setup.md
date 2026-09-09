---
title: Persistence Setup
description: "Wire persistence: Setup vs Playground, the DatabaseInteractor contract, augmenting the query engine, embedded SQLite for desktop/CLI."
---
Read this when you're at the **wire persistence** stage of the development
loop: choosing the entry point, wiring the interactor, binding generated
collections, or plugging in a different query backend.

---

## How do I set up persistence? (Setup vs Playground)

There are two entry points (both in package `anansi`). They are mutually
exclusive per process via a `sync.Once` guard — **only the first call does
work**, and every later call returns the same singleton `Persistence` and the
cached first error.

### `Playground` — development only

Fast path for tests and prototyping. In-memory SQLite by default; optional
dev logging, an event bus, and sanitization. **Not for production.** Returns a
cleanup closure you must `defer`.

```go
p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
    DBPath:     ":memory:",          // or "file.db" for a durable file
    EnableLogging: true,             // zap.NewDevelopment()
    EnableEvents:  true,             // spins up an in-memory go-events bus
    Schemas:    schema.GetSchemas(), // []*definition.Schema
    EnableSanitization: true,        // mask sensitive fields in logs/responses
})
if err != nil { log.Fatal(err) }
defer cleanup()
```

If `DBPath` is `":memory:"` the DSN is used as-is; otherwise it becomes
`file:<path>?cache=shared&_fk=1`.

### Setup — production

`anansi.Setup(anansi.SetupConfig{...})` gives you **every** knob. If you omit
the logger it falls back to `zap.NewNop()` (silent), so supply one.

```go
decorators := &utils.Decorators{
    CollectionDecorators: []utils.DecoratorFunc[base.Collection]{
        MyDecorator(logger),   // see /guides/decorators
    },
}

p, err := anansi.Setup(anansi.SetupConfig{
    Interactor:  interactor,              // query.DatabaseInteractor (required)
    Logger:      logger,                 // *zap.Logger — never nil in prod
    EventBus:    bus,                    // event bus; nil disables events
    DocumentFactoryConfig: data.DocumentFactoryConfig{},
    Decorators:  decorators,             // *utils.Decorators
    Schemas:     schema.GetSchemas(),    // auto-created if missing
})
```

Steps performed inside (in order): configure the global `Document` factory →
build the core persistence object (applying decorators) → for each schema not
already present, `CreateCollections`. Schemas you forget to pass will simply
not exist ("collection not found").

The **interactor** is the database backend. The SQLite one is built from a
`*sql.DB`:

```go
executor, _    := sqliteExecutor.NewSQLiteExecutor(db, logger)
queryFactory   := sqliteQuery.NewSQLiteFactory(logger)
interactor, _  := native.NewNativeInteractor(executor, queryFactory, logger)
```

Any type satisfying `query.DatabaseInteractor` works, so `Setup` is how you
bring PostgreSQL/MySQL or a multi-store/local-first arrangement.

### Setting up persistence with an event bus

`SetupConfig.EventBus` is a `core/events.EventBus[base.PersistenceEvent]`.
It is optional: `nil` disables event emission entirely. The events engine sits
on the go-events v2 bus (v1 and the old default Watermill bus are gone), and
the anansi wrapper speaks the request-struct `Subscribe` contract.

Build a bus, wrap it in the adapter, then hand it to `Setup`:

```go
import (
    pbase    "github.com/asaidimu/go-anansi/v8/core/persistence/base"
    pevents  "github.com/asaidimu/go-anansi/v8/core/persistence/events"
    gutils   "github.com/asaidimu/go-anansi/v8/utils"
    goevents "github.com/asaidimu/go-events/v2"
)

// Fully in-memory, ephemeral bus (playground/dev):
rawBus, err := gutils.NewInMemoryGoEventsBus("app")    // *goevents.EventBus
require.NoError(t, err)
defer rawBus.Close()
bus := pevents.NewGoEventsBusAdapter[pbase.PersistenceEvent](rawBus)

// Or a durable bus backed by a directory (survives restarts, enables replay):
cfg := goevents.DefaultConfig("/var/lib/myapp/events", "public")
rawBus, err = goevents.NewEventBus(cfg)               // Pebble state dir
require.NoError(t, err)
bus = pevents.NewGoEventsBusAdapter[pbase.PersistenceEvent](rawBus)

p, err := anansi.Setup(anansi.SetupConfig{ Interactor: interactor, Logger: logger, EventBus: bus })
```

Subscriptions are **live-only by default** (events published before `Subscribe`
are not delivered). Opt into replaying history per subscription:

```go
id := p.Subscribe(ctx, base.SubscriptionOptions{
    Event:       base.DocumentCreateSuccess,
    Callback:    func(ctx context.Context, ev base.PersistenceEvent) error { … ; return nil },
    Replay:      true,             // deliver already-published history first
    ReplayCursor: time.Unix(…),    // optional: start replay from here
})
```

`ReplayCursor` accepts `time.Time`, `uuid.UUID`, or a string UUID. To delete a
subscription, call `p.Unsubscribe(ctx, id)`. See `/guides/observability`
for full subscribe/unsubscribe detail.

### Writing your own event bus

The go-events adapter is the recommended, not the only, backend. `SetupConfig.EventBus`
is the `core/events.EventBus[T]` interface — **just two methods** — so a custom bus
(in-memory fan-out, Redis, NATS, an external service) drops in with no change to the
persistence layer:

```go
type EventBus[T any] interface {
    Emit(ctx context.Context, eventType string, event T)
    Subscribe(req SubscriptionRequest[T]) func()   // returns an unsubscribe func
}
```

`SubscriptionRequest[T]` carries `EventType`, a `Handler func(ctx, T) error`, optional
`Filters`, and `Replay`/`Cursor` (`core/events/types.go`). For persistence the concrete
type is `EventBus[base.PersistenceEvent]`; the layer above it talks only through
`EventEmitter[T]`, so nothing else needs to know your bus's internals.

Two caveats for a custom implementation:
- **You own `Replay`/`Cursor` semantics.** The go-events adapter maps `Replay`→live-only and
  translates `Cursor` to a UUID; a custom bus decides what those fields mean (or ignores them).
- **You own payload handling.** `Emit` delivers the fully-typed `base.PersistenceEvent` (not
  bytes). The go-events adapter JSON-encodes internally; your bus passes the struct by value
  or encodes as it likes — anansi mandates no wire format.

Wire it exactly like the adapter: `anansi.Setup(SetupConfig{ EventBus: myBus })`.

---

## Embedded / desktop / CLI / local-first usage

anansi is an **embedded library, not a client-server framework** — there is no
DB daemon and no network involved, so it is a natural fit for desktop apps,
CLIs, local-first stores, and single-process tools.

- **No service to run.** Storage is SQLite in-process via `database/sql`
  (`sqlite/executor`); `SetupConfig.DBPath` is a local file path and the DSN
  becomes `file:<path>?cache=shared&_fk=1`. `":memory:"` gives a volatile
  in-process store. There is no external DB to deploy.
- **Events are in-process.** The go-events v2 bus is embedded — either
  in-memory (`NewInMemoryGoEventsBus`) or durable on Pebble
  (`goevents.DefaultConfig(baseDir, key)`). No broker to run.
- **Concurrency matches a desktop app.** `Pool`, `CompiledSchema`, and read-only
  `Collection` are goroutine-safe; a `Document` is **not** — use one per
  goroutine (worker jobs, UI threads). The `ModelCollection` cache makes reads
  locally fast.
- **Just bind it to a UI.** It's pure Go backend, so it slots behind Fyne / Gio /
  Wails / shared-code GUIs or any desktop shell.
- **Same workflow.** Write `.schema.json` → `anansi codegen` → use generated
  `ModelCollection`s and migrations exactly as on a server.

### Caveat: the default SQLite driver is CGo

`mattn/go-sqlite3` (bundled) requires a C toolchain. Cross-compiling to another
OS (e.g. Linux → Windows/macOS) needs that target's C cross-toolchain; easiest is
building on each platform. For **pure-Go, CGo-free** binaries, swap the backend
through the interactor seam: `SetupConfig.Interactor` accepts any
`query.DatabaseInteractor`, and the store is already `database/sql`-based, so
wire the `modernc.org/sqlite` driver (registers as `sqlite`, no CGo) — keeping
schema / query / codegen intact while going fully static for one-binary
distribution.

---

## What does the Playground actually do?

`anansi.Playground` is `Setup`'s convenience wrapper, wired for development:

1. defaults `DBPath` to `":memory:"`;
2. builds a logger (`zap.NewDevelopment` if `EnableLogging`, else Nop);
3. optionally builds an in-memory go-events bus (`EnableEvents`);
4. optionally applies a secure-default sanitization policy (`sanitize.Configure`,
   `EnableSanitization`);
5. opens a SQLite DB, builds `SQLiteExecutor` + `SQLiteFactory` +
   `native.NewNativeInteractor`, and calls `Setup` with that interactor.

Returns `(persistence, cleanup, err)` where `cleanup` closes the DB **and**
the bus. It is **not** the production path and panics if called after a real
`Setup`. Use it for tests, examples, and rapid prototyping — then graduate to
`Setup` when you want a real interactor / broker / logging.

---

## How do I augment the query engine?

The query engine is deliberately pluggable through a handful of interfaces;
you swap out the backend rather than patch the engine:

- **`query.DatabaseInteractor`** is the backend contract: `SchemaManager` (DDL),
  and `SelectDocuments`/`UpdateDocuments`/`InsertDocuments`/`DeleteDocuments`,
  `SelectStream`, raw `Query`, `StartTransaction`/`Commit`/`Rollback`,
  `Capabilities()`. Implement it (e.g. the SQLite `native.NewNativeInteractor`)
  to back Anansi with any store.
- **SQL pushdown is governed by `Capabilities()`.** The `QueryEngine`'s
  `QueryPartitioner` sends to the DB the portions the backend reports it can
  handle natively and runs the *residual* in-memory (complex filters, custom
  functions, some projections) against the returned records. So "augmenting"
  the engine = either (a) make a backend expose more capabilities so more is
  pushed down, or (b) extend the in-memory helper for residual logic.
- **`InteractorOptions`** tune DDL per backend: `IfNotExists`, `DropIfExists`,
  `CreateIndexes`, `CollectionPrefix`, `SchemaName`.
- **`query.DocumentPoolRegistrar`** (optional capability) lets an interactor
  adopt a collection's container/document pool for write-path `RETURNING`
  scans, keeping a single pooled buffer per schema.

So: to "augment", implement/extend a `DatabaseInteractor` + `SchemaManager`
and advertise capabilities; the engine partners queries across SQL + memory
for you automatically.