---
title: "Your first schema"
description: "Declare a Products collection as a JSON schema. Field types, required vs optional, nested schemas, projections, and why field IDs are UUIDv7."
---

# Your first schema

A schema is a JSON file (conventionally `<Name>.schema.json`) describing one
collection. It is the single source of truth for storage, validation,
migrations, and codegen.

Create `schemas/products.schema.json`:

```json
{
  "version": "1.0.0",
  "name": "Products",
  "fields": {
    "name": {
      "name": "name",
      "required": true,
      "unique": true,
      "type": "string"
    },
    "price": {
      "name": "price",
      "required": true,
      "type": "number"
    },
    "stock": {
      "name": "stock",
      "required": true,
      "type": "integer"
    }
  }
}
```

## Field map keys: write names, not UUIDs

Anansi keys field ordering and the lockfile off field IDs, and **production
mode requires UUIDv7 field IDs**. Hand-typed UUIDs are unreliable, so follow
this convention:

- While authoring, set each field's `fields` key to the field **name**
  (e.g. `"shipping_address": { ... }`). This is deliberate and stable.
- Run `anansi migrate generate` later — it normalizes field-name keys to
  UUIDv7 and injects the required `_id_` and `_metadata_` system fields.
- In dev mode (`ANANSI_ENV=development`), field-name keys work as-is.
- **Never** hand-edit an existing UUIDv7 field ID to a new value — that
  breaks the lockfile linkage for that field.

## Field properties

| Key | Meaning |
| --- | --- |
| `name` | Field name (snake_case convention). |
| `type` | One of `string`, `number`, `integer`, `boolean`, `bytes`, `array`, `set`, `enum`, `object`, `record`, `union`, `composite`, `geometry`. |
| `required` | Present in every record. Drives Go value vs pointer type. |
| `nullable` | Allowed to be null. |
| `unique` | Enforced uniqueness (typically via an index). |
| `default` | Default value when not provided. |
| `deprecated` | Marks the field as deprecated. |
| `description` | Human-readable description; surfaced in godoc. |
| `schema` | Nested type descriptor (for arrays/sets/objects/etc.). |

The `required` flag has real downstream effects. A field marked `required`
becomes a Go **value type** in codegen; one marked optional becomes a
**pointer type**. Get `required` right up front — it shapes the API you'll
code against.

## Type system

| Type | Go representation | Notes |
| --- | --- | --- |
| `string` | `string` | UTF-8. |
| `number` | `float64` | IEEE 754. |
| `integer` | `int64` | 64-bit signed. |
| `boolean` | `bool` | |
| `bytes` | `[]byte` | Binary payload. |
| `array` | `[]T` | Ordered, duplicates allowed. |
| `set` | `[]T` | Unique elements. |
| `enum` | typed enum | Codegen emits a typed enum. |
| `object` | struct | References a nested schema. |
| `record` | `map[string]T` | Key-value map. |
| `union` | struct with optional pointer fields | Idiomatic Go union. |
| `composite` | struct with embeddings | Field promotion. |
| `geometry` | `[][]float64` | Numerical tuple array. |

## Nested schemas

For grouped or nested types, use a top-level `schemas` map. Each nested
schema must use exactly one mode — a `fields`-based schema OR a type-only
descriptor, never both:

```json
{
  "name": "Order",
  "version": "1.0.0",
  "fields": {
    "customer": { "name": "customer", "required": true, "type": "object",
                   "schema": { "id": "customerSchema" } }
  },
  "schemas": {
    "customerSchema": {
      "name": "Customer",
      "fields": {
        "name":  { "name": "name",  "required": true, "type": "string" },
        "email": { "name": "email", "required": true, "type": "string", "unique": true }
      }
    }
  }
}
```

## Projections

Projections are declared under `metadata.projections` in the schema. Each
entry describes a field subset and optional constraint/tag overrides:

```json
{
  "metadata": {
    "projections": {
      "ProductSummary": { "fields": { "include": ["name", "price"] } },
      "ProductCreate":  {
        "fields": { "include": ["name", "price", "stock"] },
        "required": ["name", "price"]
      }
    }
  }
}
```

Codegen derives **both** the root model struct AND the projection DTOs from
the schema. Do not define the schema and the Go structs separately — the
schema is the only source of truth.

| Projection key | Meaning |
| --- | --- |
| `include` / `exclude` | Membership (no `include` ⇒ all root fields minus `exclude`). |
| `required` / `optional` | Override `required` for this projection only. |
| `tags` | Custom struct tags per field, with `{name}`, `{type}`, `{required}`, `{nullable}`, `{default}`, `{goName}` placeholders. |

## Validate the schema

Before generating code, validate the schema:

```bash
anansi schema validate schemas/products.schema.json
```

This runs the graph validator: it checks for unknown fields, `include` ∩
`exclude` conflicts, `required` ∩ `optional` conflicts, circular dependencies
between nested schemas, and tag references to fields outside the final
projection set. Errors come back as `ERR_*` codes with line/column
diagnostics.

## Next

[Code generation basics →](/tutorial/codegen-basics) — turn this schema into
Go structs and a typed collection wrapper.

> **Reference:** the full schema format, including the DTO → schema reverse
> path (`data.SchemaFrom[T]`) and composed envelope schemas
> (`Schema.WithSchema`), is in [Schema format](/reference/schema-format).
