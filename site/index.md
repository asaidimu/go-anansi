---
layout: home

hero:
  name: Go-Anansi
  text: A schema-driven data layer for Go
  tagline: Declare your data model as a JSON schema. Get type-safe Go structs, a typed collection wrapper, hybrid query pushdown, versioned migrations, and a cross-language wire format. No ORM magic, no app framework.
  image:
    src: /hero.svg
    alt: Go-Anansi
  actions:
    - theme: brand
      text: Start the tutorial
      link: /tutorial/overview
    - theme: alt
      text: View on GitHub
      link: https://github.com/asaidimu/go-anansi

features:
  - icon: 🧬
    title: Schema as source of truth
    details: One JSON schema per collection drives storage, validation, migrations, and code generation. No drift between your data model and your code.
    link: /explanations/schema-as-source-of-truth
    linkText: Why this matters →
  - icon: ⚡
    title: Type-safe codegen
    details: Generate Go structs, enums, projections, and a typed ModelCollection[P] wrapper with CRUD, validation, and subscriptions built in. Requires Go 1.27.
    link: /tutorial/codegen-basics
    linkText: Try it →
  - icon: 🔄
    title: Hybrid query engine
    details: A fluent DSL partitioned into SQL — what the backend supports — plus an in-memory residual. Push what you can; run the rest.
    link: /reference/query-dsl
    linkText: Read the spec →
  - icon: 🧩
    title: Decorators over hooks
    details: Wrap Persistence or Collection to inject audit, caching, encryption, sanitization. No magic lifecycle, everything is composable.
    link: /guides/decorators
    linkText: See the pattern →
  - icon: 🛡️
    title: Versioned migrations
    details: anansi migrate generate produces versioned migrations from schema changes. Squash intermediate history when it grows. Apply on startup.
    link: /tutorial/schema-change-workflow
    linkText: Workflow →
  - icon: 🌐
    title: Go ⇄ TypeScript wire format
    details: A sibling TypeScript package implements the Anansi wire format bit-exact with the Go codec, verified by CI byte-for-byte.
    link: /explanations/wire-format
    linkText: How it works →
---

## What Anansi is — and isn't

Go-Anansi is a **data-layer toolkit**. It gives you schemas, codegen, a query
engine, migrations, and an event bus. You bring your own HTTP server, your own
auth, your own logger — Anansi stays out of the way above the persistence
layer.

**Anansi is:**

- A schema-driven persistence library for Go, backed by SQLite.
- A code generator that emits typed Go structs and collection wrappers.
- A hybrid query engine that pushes to SQL where it can.
- A migration generator that produces versioned, squashable migration files.
- A cross-language wire format shared between Go and TypeScript.

**Anansi is not:**

- An HTTP framework. There's no router, no middleware, no request lifecycle.
- An auth system. Decorators can wrap persistence for auth, but Anansi ships
  no built-in auth.
- An ORM. There's no model hooks DSL, no relationship traversal, no lazy
  loading. Schemas describe documents, not object graphs.
- A full-stack framework. The TypeScript package is a wire-format codec, not a
  backend.

This scope is deliberate. Anansi does the data layer well and lets you compose
the rest. If you want a batteries-included framework, this isn't it.

## What do you want to do?

<div class="phase-picker">

  <a class="phase-card" href="/tutorial/overview">
    <div class="phase-card__signal">I'm new here</div>
    <div class="phase-card__title">Start the tutorial</div>
    <div class="phase-card__desc">Declare a schema, generate models, run CRUD against in-memory SQLite. About 15 minutes.</div>
  </a>

  <a class="phase-card" href="/guides/persistence-setup">
    <div class="phase-card__signal">Wire persistence</div>
    <div class="phase-card__title">Setup vs Playground</div>
    <div class="phase-card__desc">Production wiring with an explicit DatabaseInteractor, logger, event bus, and decorators. Embedded SQLite for CLI/desktop apps.</div>
  </a>

  <a class="phase-card" href="/guides/caching">
    <div class="phase-card__signal">Cache it</div>
    <div class="phase-card__title">ModelCollection & pooling</div>
    <div class="phase-card__desc">Read-through id cache, LiveCollection, the document Release() contract, why ModelCollection is the fast path.</div>
  </a>

  <a class="phase-card" href="/guides/decorators">
    <div class="phase-card__signal">Harden it</div>
    <div class="phase-card__title">Audit / encrypt / validate</div>
    <div class="phase-card__desc">Cross-cutting concerns via the decorator pattern. Wrap Persistence or Collection, intercept calls.</div>
  </a>

  <a class="phase-card" href="/guides/sanitization">
    <div class="phase-card__signal">Scrub PII</div>
    <div class="phase-card__title">Sanitization</div>
    <div class="phase-card__desc">Mask or redact fields before they reach logs, events, or responses. Per-collection scoping, runtime policy.</div>
  </a>

  <a class="phase-card" href="/guides/observability">
    <div class="phase-card__signal">Operate</div>
    <div class="phase-card__title">Metrics & realtime</div>
    <div class="phase-card__desc">Stats(), event counters, the cleanup checklist. Realtime is events + your own transport.</div>
  </a>

  <a class="phase-card" href="/reference/query-dsl">
    <div class="phase-card__signal">Query it</div>
    <div class="phase-card__title">Query DSL</div>
    <div class="phase-card__desc">Filter, sort, paginate, join, aggregate, compute. Pushdown to SQL where the backend allows.</div>
  </a>

  <a class="phase-card" href="/guides/transactions">
    <div class="phase-card__signal">Atomic writes</div>
    <div class="phase-card__title">Transactions</div>
    <div class="phase-card__desc">p.Transact for multi-collection, coll.Transact for single, with nesting, hooks, and error sentinels.</div>
  </a>

</div>

## Alpha state

Go-Anansi is in **alpha**. The API is unstable and may change without notice
between releases. Backward compatibility is not guaranteed yet. The schema
format, codegen surface, and the fluent QueryBuilder DSL are the most stable
parts; the query engine internals and the event API are most likely to shift.
Not recommended for production use today — feedback and bug reports via
[GitHub issues](https://github.com/asaidimu/go-anansi/issues) are very welcome.

## Two surfaces, one schema

<div class="vp-doc">

### Go — the primary surface

```bash
go get github.com/asaidimu/go-anansi/v8@latest
```

Declare a schema, run `anansi codegen golang`, get typed structs and a
`ModelCollection[P]` wrapper. Persist to SQLite, query through a hybrid DSL,
harden with decorators. [Start the tutorial →](/tutorial/overview)

### TypeScript — the wire format only

```bash
bun add @asaidimu/anansi
```

A self-contained TS implementation of the Anansi wire format: schema
compile/link/address, packet codecs (Dense / Sparse / Batch row + columnar),
ZSTD + BLAKE3 + AES-256-GCM, validation. No Go runtime, no codegen step.
[Read the wire format explainer →](/explanations/wire-format)

</div>
