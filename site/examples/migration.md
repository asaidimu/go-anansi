---
title: Migration
description: "Generate and apply schema migrations on startup. The canonical edit-preview-generate-codegen loop in action."
---

# Migration

The [`example/migration`](https://github.com/asaidimu/go-anansi/tree/main/example/migration)
directory walks through the canonical schema-change workflow with a real
migration file.

## What it shows

- Editing a schema (adding a field).
- Running `anansi migrate generate --dry-run` to preview.
- Running `anansi migrate generate` to produce a versioned migration under
  `migrations/`.
- Regenerating structs with `anansi codegen golang`.
- Applying migrations on startup via `migrations.Apply(...)`.

The directory includes `anansi.json`, `schemas/products.schema.json`,
`schemas.lock.json`, and two real migration files showing a major and a
minor schema bump.

## How to run

```bash
cd example/migration
ANANSI_ENV=development go run .
```

## Read next

- [Tutorial: Schema change workflow](/tutorial/schema-change-workflow)
- [Reference: Migration semantics](/reference/migration-semantics)
- [Reference: Schema versioning](/reference/schema-versioning)
