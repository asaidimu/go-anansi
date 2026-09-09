---
title: "Schema as source of truth"
description: "Why Anansi declares the schema once and derives everything else from it. The cost of dual sources of truth, and what falls out of this design choice."
---

# Schema as source of truth

Anansi's central design decision is that **one JSON schema per collection
drives storage, validation, migrations, AND code generation.** Everything
else — Go structs, TypeScript types, migration files, the wire format — is
derived. There is no second source of truth.

## The cost of dual sources

In a typical Go service, your data model lives in three or four places at
once:

- The database schema (SQL `CREATE TABLE`).
- The Go structs (often with `gorm` or `sqlc` tags).
- The API contract (OpenAPI, Protobuf, or hand-written).
- The frontend types (TypeScript interfaces, often copy-pasted).

These drift. A column added in the database isn't reflected in the Go struct
until someone remembers. The API contract lags further. The frontend types
lag further still. Every drift is a bug waiting to surface in production —
usually at 2am, usually in a way that's hard to reproduce.

Anansi collapses all four into one: the `.schema.json` file. Run
`anansi codegen golang` and the Go structs update. Run
`anansi codegen typescript` and the frontend types update. The database
schema is derived from the same source via migrations. Validation is free —
the schema declares `required`, `unique`, `nullable`, types, and the
validator enforces them with no extra code from you.

## What falls out of this

### No code-database drift

The schema IS the database shape (modulo migrations, which are themselves
derived from schema diffs). There's no second `CREATE TABLE` to keep in
sync.

### No frontend-backend drift

The TypeScript package consumes the same schema format and produces
byte-compatible wire packets. A field added to the schema appears in both Go
code and TS types after one codegen run.

### Validation is free

The schema declares `required`, `unique`, `nullable`, types — the validator
enforces them with no extra code from you. Add a constraint to the schema,
regenerate, and every `Create` and `Update` enforces it.

### Migrations are diffs

A schema change produces a migration automatically. You don't write
`ALTER TABLE` by hand — you edit the schema, run `anansi migrate generate`,
and the migration file appears under `migrations/`.

## What it costs you

This design isn't free. The costs are real and worth naming.

### You commit to JSON schemas as the source of truth

If your team prefers Go-struct-first (e.g. `ent`, `gorm` codegen, `sqlc`),
this is a real shift. The schema is now JSON, not Go. That's a different
workflow and a different mental model.

There's a reverse path — `data.SchemaFrom[T]` derives a schema from a Go
struct's `anansi` tags — but the forward direction (schema → code) is
canonical. The reverse exists for migration and integration, not as the
primary workflow.

### Codegen is in the loop

Every schema change requires a codegen run. This is a CI step, not a manual
one, but it's an extra step. If your team isn't set up to run codegen in CI
and commit the output, you'll have a bad time.

### The schema format is Anansi-specific

It's not JSON Schema, it's not Protobuf, it's not OpenAPI. The format is
documented in [Schema format](/reference/schema-format), and it's stable
within a major version, but it's Anansi's own.

If you need to interoperate with tools that consume JSON Schema or OpenAPI,
you'll need to write a translator. None ships today.

## When this design pays off

- **Long-lived projects** where the data model evolves over years. Drift is
  expensive; collapsing the sources of truth is a one-time tax for an
  ongoing return.
- **Multi-language projects.** The Go ⇄ TypeScript wire format is the
  strongest case — both sides consume the same schema, both sides produce
  identical bytes.
- **Teams that want schema review as a code-review step.** PRs that change
  the schema are explicit, reviewable, and produce visible diffs in codegen
  output. Reviewers see both the schema change AND the generated code change.

## When this design doesn't fit

- **Rapid prototyping with no DB.** If you're sketching a service that won't
  ship with persistence, the schema ceremony is overhead.
- **Projects that need a different backend's query language.** Anansi's
  query DSL is its own; if you're deeply invested in raw SQL or a specific
  ORM's DSL, the partitioning layer adds indirection without payoff.
- **Teams that hand-write SQL for performance.** Anansi pushes what it can
  to SQL, but the partitioner has its own opinion about what to push. If you
  need absolute control over the SQL, the abstraction may frustrate you.

## Related

- [Architecture](/explanations/architecture) — where each layer lives.
- [Schema format](/reference/schema-format) — the JSON format reference.
- [Codegen modes](/reference/codegen-modes) — what codegen emits and how.
- [Wire format](/explanations/wire-format) — the cross-language guarantee.
