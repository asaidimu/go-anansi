---
title: "REST API example"
description: "A net/http server that integrates Anansi for persistence. Shows schema-driven request/response envelopes via Schema.WithSchema, middleware, and production-style persistence wiring with anansi.Setup."
---

# REST API example

The [`example/api`](https://github.com/asaidimu/go-anansi/tree/main/example/api)
directory contains a working `net/http` server that integrates Anansi for
persistence. It demonstrates how to wire Anansi into a typical Go HTTP
service without an external web framework.

## What it shows

- A clean layered structure: `internal/api` (HTTP handlers), `internal/app`
  (persistence wiring), `internal/response` (helpers), `schema/` (schema
  registry).
- Loading schemas from JSON with a `SchemaLoader`.
- Production persistence wiring via `anansi.Setup` (not `Playground`) —
  explicit `DatabaseInteractor`, `*zap.Logger`, event bus, and decorators.
- Schema-driven request/response envelopes via `definition.Schema.WithSchema(...)`
  — the same schema type used for storage is composed into an envelope schema
  for API exchange.
- Graceful shutdown with signal handling.
- Error normalization through `common.SystemError`.

## How to run

```bash
cd example/api
ANANSI_ENV=development go run .
```

The server listens on a configurable address. Hit the endpoints with curl or
your HTTP client; see
[`example/api/spec.md`](https://github.com/asaidimu/go-anansi/blob/main/example/api/spec.md)
for the endpoint list and example request/response bodies.

## What this example is — and isn't

This is **not** a framework. Anansi doesn't ship a router, middleware system,
or request lifecycle. The example uses Go's standard `net/http` package and
shows how to wire Anansi into it — your HTTP layer is yours.

If you're using a different HTTP framework (chi, gin, echo, fiber), the
persistence wiring in `internal/app/` ports directly. The schema-driven
envelope pattern via `Schema.WithSchema` works the same way regardless of
HTTP layer.

## Read next

- [Persistence setup](/guides/persistence-setup) — the `Setup` vs `Playground`
  distinction and the production wiring this example uses.
- [Schema format](/reference/schema-format) — including the "Composed schemas"
  section for the `Schema.WithSchema` envelope pattern.
- [Sanitization](/guides/sanitization) — scrubbing PII from responses before
  they leave the server.
