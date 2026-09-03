---
title: "Schema change workflow"
description: "The canonical edit-preview-generate-codegen loop. Never hand-edit UUIDs. Always end a schema change with migrate generate + codegen golang."
---

# Schema change workflow

When adding, modifying, or removing fields of an existing schema, follow this
sequence and nothing else by hand.

## 1. Edit the schema file directly

- Do **not** alter existing field IDs.
- Only modify field properties or the set of fields.
- New field → set its `fields` key to match the field's **name**:

  ```json
  "shipping_address": {
    "name": "shipping_address",
    "type": "string",
    "required": true
  }
  ```
- Removing a field → delete its entry completely, preserving all other fields.

## 2. Preview the migration

```bash
anansi migrate generate --dry-run
```

This validates the schema and shows what would be normalized + migrated.
Always run this first — it catches validation errors and UUID collisions
before anything is written.

Sample output (paraphrased):

```
Schema: Products
  Current version: 1.0.0
  Target version:  1.1.0

  Changes:
    + field "shipping_address" (string, required)
      would normalize to UUID 019fb22d-a9a1-7e40-b7e2-8ca46b4c788a
    ~ field "price" constraint changed: required=true → required=false

  Migration file would be written to:
    migrations/019fb22d-a9a1-7e40-b7e2-8ca46b4c788a_Products_minor.go
```

## 3. Generate the migration

```bash
anansi migrate generate
```

This rewrites field-name keys to UUIDv7, injects system fields (`_id_`,
`_metadata_`), and writes a versioned migration under `migrations/`:

```
migrations/
├── 019f4078-a5c3-795c-ae90-ac15750f5ffb_Products_major.go
├── 019f4079-e76e-704d-86eb-c90dbb3ffc69_Products_minor.go
├── metadata.go
├── providers.go
└── registry.go
```

The filename encodes the schema UUID, collection name, and bump type
(`_major` / `_minor`). `registry.go` is regenerated to import the new
migration.

## 4. Regenerate structs and DTOs

```bash
anansi codegen golang
```

Re-emits the model structs AND projection DTOs from the updated schema.

## 5. Apply migrations on startup

The scaffolded `main.go` calls `migrations.Apply(...)` on startup. Keep that
pattern — it detects pending migrations and runs them in order:

```go
// main.go
if err := migrations.Apply(ctx, p); err != nil {
    log.Fatalf("apply migrations: %v", err)
}
```

`Apply` is idempotent. Already-applied migrations are skipped; the version is
tracked per collection in the lockfile.

## Why this loop

This single workflow is how "make a schema change" always ends. It also means
you never hand-write structs — the schema edit + codegen produces them. The
lockfile (`schemas.lock.json`) tracks every field ID across revisions so
renames don't accidentally break existing data.

## Squashing migrations

When `migrations/` grows large, consolidate intermediate migrations:

```bash
anansi migrate squash Products
```

This rewrites history so the squashed collection has a single migration
reflecting the current schema state. Safe to do between releases — the
lockfile preserves the field ID linkage.

## Versioning: major vs minor bumps

Anansi uses semantic versioning at the schema level:

| Bump type | Triggered by |
| --- | --- |
| **Major** (1.0.0 → 2.0.0) | Removing a field, changing a field type, narrowing a constraint (e.g. `nullable=false` → `true`), renaming. Existing data may become invalid. |
| **Minor** (1.0.0 → 1.1.0) | Adding a new field, widening a constraint (e.g. `nullable=true` → `false`), adding a projection. Existing data is unaffected. |

The validator infers the bump type from the diff; you don't choose it. See
[Schema versioning](/reference/schema-versioning) for the full rules.

## Common pitfalls

- **Hand-editing a UUIDv7 field ID.** This breaks the lockfile linkage for
  that field. Preserve existing IDs at all costs.
- **Skipping `--dry-run`.** The normalize step rewrites your schema file in
  place; preview first to avoid surprises.
- **Editing the generated Go file directly.** It's overwritten on every
  codegen run. Put custom code in `<model>_utils.go`.
- **Forgetting to commit the lockfile.** The lockfile is the source of truth
  for field ID stability. Always commit `schemas.lock.json` alongside schema
  changes — without it, a teammate's `migrate generate` will produce
  different UUIDs and break the build.

## Worked example

Adding a `description` field to `Products`:

```bash
# 1. Edit schemas/products.schema.json — add the field under "fields":
#    "description": { "name": "description", "type": "string" }

# 2. Preview
anansi migrate generate --dry-run

# 3. Generate
anansi migrate generate
# → migrations/019fb22d-..._Products_minor.go written

# 4. Regenerate Go
anansi codegen golang
# → products.go now includes a Description string field

# 5. Commit
git add schemas/products.schema.json schemas.lock.json \
        migrations/ products.go
git commit -m "feat(schema): add Products.description (1.0.0 → 1.1.0)"
```

## Next

You've finished the tutorial. From here:

- [Guides](/guides/domain-modeling) — task-oriented how-tos for caching,
  transactions, decorators, sanitization, and more.
- [Reference](/reference/schema-format) — the full schema format and query
  DSL specs.
- [Explanations](/explanations/architecture) — why Anansi is shaped the way
  it is.
