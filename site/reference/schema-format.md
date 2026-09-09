---
title: Schema Format
description: "The schema JSON document: what it is, a complete example first, then each part in reading order — identity, fields, nesting, indexes, constraints, extensions."
---
A schema is one JSON document per collection: the shape of the records, the
rules they obey, and the metadata the tooling needs. Storage, validation,
migrations, and codegen all build from it — nothing is hand-duplicated
downstream. Start from the example, then read each part in order.

## A complete example

This is the authoring form — field keys are names, system fields absent.
`normalize` rewrites it into the production form (see
[Names vs IDs](#names-vs-ids)):

```json
{
  "name": "Products",
  "version": "1.0.0",
  "description": "Sellable items in the catalog.",
  "fields": {
    "name":  { "name": "name", "type": "string", "required": true },
    "price": { "name": "price", "type": "number", "required": true },
    "status": { "name": "status", "type": "enum", "required": true,
                "schema": { "type": "string",
                            "values": ["draft", "live", "retired"] } },
    "shipping_address": { "name": "shipping_address", "type": "object",
                "schema": { "id": "<address schema id>" } }
  },
  "schemas": {
    "<address schema id>": {
      "name": "address",
      "fields": {
        "city": { "name": "city", "type": "string", "required": true },
        "zip":  { "name": "zip",  "type": "string", "required": false }
      }
    }
  },
  "indexes": {
    "by_name": { "name": "by_name", "fields": ["name"], "type": "unique" }
  },
  "constraints": {
    "price_sane": { "name": "price_sane", "fields": ["price"],
                    "predicate": "<a registered predicate>",
                    "parameters": 0 }
  },
  "metadata": {
    "projections": {
      "ProductSummary": { "fields": { "include": ["name", "price"] } }
    }
  }
}
```

The rest of this page walks the example top to bottom. Each section answers
three questions: what the key holds, who consumes it, and what can go wrong.

## Identity: `name`, `version`, `description`

- `name` (required) names the collection. Generated code, the registry, and
  the collection handle all use it.
- `version` (required) names this revision. The collection registry tracks
  one active version per collection, and migrations record version
  transitions — bump it when the shape changes (`migrate generate` handles
  this in the schema-change workflow).
- `description` is free-form text, carried through untouched.

## Fields

`fields` maps field IDs to definitions. Each definition:

```json
"019fb22d-...": {
  "name": "email",
  "type": "string",
  "required": true,
  "nullable": false,
  "unique": true,
  "default": null,
  "deprecated": false,
  "description": "...",
  "schema": { ... }
}
```

| Key | Holds | Consumed by |
| --- | --- | --- |
| `name` | The field's address. Queries, projections, indexes, and constraints all use `name` — never the ID. Snake_case convention. | Everything addressable |
| `type` | One of `string`, `number`, `integer`, `decimal`, `boolean`, `bytes`, `array`, `enum`, `object`, `record`, `union`, `composite`, `geometry`, `unknown`. | Storage (column types), validation, codegen mapping, query paths |
| `required` | Present in every record. | Validation rejects writes missing it; storage emits `NOT NULL`; codegen emits a value type instead of a pointer |
| `nullable` | May hold null (pointers imply it). | Validation; codegen pointer types |
| `unique` | Storage-enforced uniqueness. | Storage emits a `UNIQUE` constraint; violations surface as backend unique-constraint errors |
| `default` | Default literal (not allowed on `composite`/`union`). | Storage emits the column `DEFAULT`; codegen carries it into the `anansi` tag |
| `deprecated` | Intent-to-remove marker. Tracked by schema diff, enforced by nothing. | Tooling signal only |
| `description` | Free-form text. | Carried through untouched |
| `schema` | Nested type info (next section). Absent for scalars and `geometry`. | Storage, validation, codegen |

## Names vs IDs

Names and IDs do different jobs, and confusing them is the most common
authoring mistake:

- **Names** are the human label and the *only* addressing scheme.
- **IDs** (`fields` map keys) are the stable identity that migrations and
  the lockfile pin a field to. They must be unique across ALL schemas in
  the document, including nested ones ([Rule 1](/reference/schema-rules)).

The workflow follows from that split. **Author with names as keys**
(`"shipping_address": { ... }`); `anansi schema normalize` rewrites keys to
UUIDv7 and injects the `_id_` / `_metadata_` system fields. Production mode
requires UUIDv7 keys (`ERR_REGISTRY_INVALID_SCHEMA` otherwise); dev mode
(`ANANSI_ENV=development`) accepts names as-is. `migrate generate`
normalizes as a side effect.

**Never invent, reuse, or reassign an ID.** A new field gets a fresh ID from
normalize; changing an existing field's ID breaks its lockfile linkage —
it reads as a delete plus an add.

## Nested schemas (`schemas`)

Complex field types reference entries under `schemas` by `id`. Each entry
uses exactly ONE mode (mixing them is invalid):

- **Schema mode**: `name` + `fields` (+ optional `indexes`, `constraints`,
  `metadata`). A full object shape, like `address` above.
- **Type mode**: `type` + `schema` only — an element, value, or variant type.
- **Enum mode**: `type: "enum"` with `values`.

The `schema` forms per type:

- **Array of scalars** — inline element type:
  `{ "type": "array", "schema": { "type": "string" } }`
- **Array of objects** — reference: `{ "type": "array", "schema": { "id": "…" } }`
- **Enum** — `{ "type": "enum", "schema": { "type": "string", "values": […] } }`
- **Object** — reference only; `object` is never inline:
  `{ "type": "object", "schema": { "id": "…" } }`
- **Record** — free-form `{ "type": "record" }`, or typed via reference/inline.
- **Union / composite** — variant references:
  `{ "type": "union", "schema": [{ "id": "…" }, { "id": "…" }] }`

## Indexes and constraints

Both are declarations the backends enforce — addressed by field **name**:

- `indexes` maps index IDs to `{ name, description?, order?, condition?,
  fields, type, unique? }`, where `type` is one of `normal`, `unique`,
  `primary`, `spatial`, `fulltext`. Storage turns these into DDL
  (`unique` → a `UNIQUE` index).
- `constraints` maps constraint IDs to validation rules: a single rule
  `{ name, description?, fields?, predicate, parameters }` or a group
  `{ name, rules, operator }`. Predicate *names* come from the
  caller-supplied predicate map handed to the validator — the schema only
  declares which predicate applies with what parameters.

## `metadata`: opaque extension space

`metadata` is `map[string]any`: compiled, linked, and diffed as data, never
interpreted by the schema pipeline. The one well-known key is a codegen
extension, not canonical format:

**`metadata.projections` (codegen only).** Defined and solely consumed by
`anansi codegen golang`: each projection selects a field subset
(`include`/`exclude`, `required`/`optional` overrides, per-field custom
`tags`) that codegen emits as a DTO struct. The full DSL lives where it
belongs — [Projections](/tutorial/projections) for the workflow and
[Codegen modes](/reference/codegen-modes) for the emission rules.

## Producing schemas

Schemas reach the shape above three ways:

1. **Hand-written JSON**, as in the example.
2. **Derived from a Go struct** via `data.SchemaFrom[T]` — the DTO shapes
   the schema without hand-writing JSON. The `anansi` tag grammar behind it
   is specified in [Struct tag spec](/reference/struct-tag-spec).
3. **Composed in Go** — embed an existing schema as a nested schema with
   `definition.Schema.WithSchema(sub)`, which returns the composed schema
   plus a `SchemaId` you mount from a root field via
   `SchemaReference{ID: …}`. Non-mutating (use the result); ID collisions
   are remapped; nothing is auto-mounted. Recommended for API envelopes:
   derive the payload (`SchemaFrom[T]` or hand-written) → `WithSchema` →
   `WithField`/`WithFieldEnsured` for envelope fields → `ToJSON()`/`AsMap()`
   or straight to `anansi.Setup`.

## Authoring tips

- **Write field keys as field names, never hand-invented UUIDs.** An LLM
  cannot be trusted to emit valid UUIDv7s, and normalize fixes them anyway.
- `anansi schema normalize <schema>` rewrites field-name keys to UUIDv7 and
  injects the `_id_` / `_metadata_` system fields (with the `_metadata_`
  nested schema) that production collections require.
- `anansi migrate generate` runs the same normalization as a side effect, so
  the schema-change workflow (`migrate generate --dry-run` →
  `migrate generate` → `anansi codegen golang`) both normalizes and migrates
  in one pass.
- If your schema needs system `_id_` / `_metadata_` fields visible in the
  struct, run `anansi schema normalize` — the fields are then emitted as
  ordinary schema fields.
