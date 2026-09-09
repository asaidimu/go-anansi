# Anansi schema format reference

The schema is a JSON file (conventionally `<Name>.schema.json`) describing one
collection. It is the single source of truth for storage, validation,
migrations, and codegen. This reference covers the JSON format and the Go types
codegen emits.

## Top-level structure

```json
{
  "name": "Products",
  "version": "1.0.0",
  "description": "optional",
  "fields": { "<FieldId>": { ... }, ... },
  "schemas": { "<SchemaId>": { "name": "...", "fields": { ... } }, ... },
  "indexes": { ... },
  "constraints": { ... },
  "metadata": { "projections": { ... } }
}
```

- `name` and `version` are required.
- `fields` is a map. The keys are stable field IDs. **While authoring, use the
  field's name as the key** (e.g. `"shipping_address": { ... }`); anansi's
  `normalize` step rewrites them to UUIDv7. Production mode requires UUIDv7
  keys (`ERR_REGISTRY_INVALID_SCHEMA` otherwise); dev mode
  (`ANANSI_ENV=development`) accepts field-name keys as-is. IDs must be
  globally unique across ALL schemas in the document, including nested ones.
- `schemas` holds named nested schemas (objects, arrays/sets of objects,
  records, unions, composites).
- `indexes` and `constraints` are optional; `metadata.projections` declares
  projection shapes — author them in the schema; codegen emits the DTOs.

## Field properties

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

| Key | Meaning |
| --- | --- |
| `name` | Field name (snake_case convention in examples). |
| `type` | One of: `string`, `number`, `integer`, `boolean`, `bytes`, `array`, `set`, `enum`, `object`, `record`, `union`, `composite`, `geometry`. |
| `required` | Present in every record. Drives Go value vs pointer type. |
| `nullable` | May hold a null. |
| `unique` | Value must be unique across all records. |
| `default` | Default value (stored, used by codegen). |
| `deprecated` | Structurally valid; codegen may warn/suppress. |
| `schema` | Nested type information (see below). |

## `schema` (nested type info) forms

- **Array/set of scalars** — inline element type:
  ```json
  { "type": "array", "schema": { "id": "", "type": "string" } }
  ```
- **Enum** — element type + value list:
  ```json
  { "type": "enum", "schema": { "type": "string", "values": ["admin", "member"] } }
  ```
- **Reference to a nested schema** (object / array-of-objects / record):
  ```json
  {
    "type": "object",
    "schema": { "id": "019d7775-6565-7ed7-..." }
  }
  ```
  The `id` must match a schema under the top-level `schemas` map. A `record`
  (key-value map) can also just be `{ "type": "record" }` (free-form map).

## Nested schema modes (exclusivity rule)

Each entry under `schemas` must use exactly ONE of:

- **Schema mode**: has `name` + `fields` (+ optional `indexes`,
  `constraints`, `metadata`). Describes a full object shape.
- **Type mode**: describes a type (`type` + `schema`) only — e.g. the element
  type of an array, the value type of a record, union/composite variants.
- **Enum mode**: `type: "enum"` with `values`.

Mixing schema-mode fields and type-mode properties in one nested schema is
invalid. `object` is never a valid Type-mode type — an object is expressed by
referencing a schema-mode nested schema that has `fields`.

## Projections (`metadata.projections`)

```json
"metadata": {
  "projections": {
    "ProductSummary": { "fields": { "include": ["name", "price"] } },
    "ProductCreate":  { "fields": { "include": ["name", "price", "stock"], "required": ["name", "price"] } },
    "ProductUpdate":  { "fields": { "exclude": ["stock"], "optional": ["price"] } },
    "OrderCreate": {
      "fields": {
        "include": ["total"],
        "required": ["total"],
        "tags": {
          "total": { "input": "arguments.{name}", "schema": "{type}:{required}" }
        }
      }
    }
  }
}
```

| Key | Meaning |
| --- | --- |
| `fields.include` | Whitelist (default: all root fields). |
| `fields.exclude` | Remove from the final set. |
| `fields.required` | Force `required=true` (value type). |
| `fields.optional` | Force `required=false` (pointer type). |
| `fields.tags` | Custom struct tags per field with `{name}`, `{type}`, `{required}`, `{nullable}`, `{default}`, `{goName}` placeholders. |

Resolution: base set → `include` → `exclude` → required/optional overrides →
tags. Codegen **fails fast** on: unknown fields, `include` ∩ `exclude`,
`required` ∩ `optional`, overrides referencing fields outside the set, or a
projection name colliding with the root model type. Overriding `required`
changes both the emitted type and the `anansi` tag (e.g. an optional base field
upgraded to required becomes a value type).

### Custom tags (projections only)

`tags` is declared **per projection** under `metadata.projections.<name>.fields.tags`:
a map of field name → { tag key → template }. Templates expand `{name}`,
`{type}`, `{required}`, `{nullable}`, `{default}`, `{goName}` placeholders from
the field. The projection above renders:

```go
Total decimal.Decimal `anansi:"total,required=true" json:"total" input:"arguments.total" schema:"decimal:true"`
```

Notes:

- **Regular (root) fields cannot carry custom tags** — `tags` exists only inside
  a projection's `fields`. There is no per-field `tags` on the base `Field`.
- All projection references (`include`, `exclude`, `required`, `optional`,
  `tags`) address fields by **field `name`**, never by field ID. A field is
  addressable only if it is declared in the schema's `fields` map. So ordinary
  user fields can be tagged whether or not the schema is normalized
  (normalization only rewrites the `fields` map *keys* to UUIDv7, leaving each
  field's `name` intact).
- To tag a **system field** (`_id_`, `_metadata_`), you MUST normalize first:
  pre-normalize those fields don't exist in `rootFields` (they're only
  synthesized as shadow fields at render) so a projection referencing `_id_`
  fails with `references unknown field "_id_"`. Post-normalize they are declared
  as ordinary named fields and are taggable like any other.
- Unknown placeholders (e.g. `{frobnicate}`) fail fast at codegen; a non-string
  tag value fails too ("must be a string").

## Generated Go

`anansi codegen golang` (default mode `full`) emits per schema:

```go
// enum (named or inline) -> typed string constants
type UserRoleEnum string
const ( UserRoleEnumAdmin UserRoleEnum = "admin"; UserRoleEnumMember UserRoleEnum = "member" )

// model struct: embeds a DocumentModel (id/metadata system fields)
type User struct {
    document.DocumentModel
    ID    string        `json:"_id_,omitempty" anansi:"_id_,required=true,omitempty"`
    Email string        `anansi:"email,required=true" json:"email"`
    Age   *int64        `anansi:"age,required=false,nullable=true" json:"age,omitempty"`
    Role  *UserRoleEnum `anansi:"role,required=false,type=enum" json:"role,omitempty"`
}
func NewUser(model User) *User { return document.New(&model) }

// collection wrapper + idempotent singleton
type Users struct { *collection.ModelCollection[*User] }
const UsersCollectionName = "User"
func InitUsersModel(p base.Persistence, logger *zap.Logger, opts ...collection.ModelCollectionOptions[*User]) (*Users, error)
func UsersModel() (*Users, error)

// each projection -> struct embedding document.DocumentModel
type UserSummary struct { document.DocumentModel; Email string `anansi:"email,required=true" json:"email"` }
```

Key facts:

- **Type rules**: required non-nullable → value type; optional/nullable →
  pointer. This is why `required` must be right up front.
- **ID/Metadata shadows**: when the schema doesn't declare the `_id_` /
  `_metadata_` system fields, codegen emits shadow `ID`/`Metadata` struct
  fields (renamed to `ModelID`/`ModelMetadata` if the schema claims that Go
  name) plus an explicit `GetID()`. If the schema does declare them (e.g. after
  `anansi schema normalize`), they're emitted as ordinary fields — never
  duplicated.
- **Metadata struct is per collection**: the `_metadata_` nested schema (the
  system integrity envelope injected by normalize) generates a *typed* struct
  named `rootName + "Metadata"` — `UserMetadata`, `OrderMetadata`, etc. — not a
  bare `Metadata`. Each schema in a package gets its own struct, so multiple
  collections in one package neither collide nor leak each other's tags. It
  carries `Checksum`, `Created`, `Updated`, `Signature *string`, `Version
  float64`; the root field is `Metadata *UserMetadata json:"_metadata_"`.
- **Overwrite warning**: the generated file is fully regenerated every run.
  Extend models in separate files (e.g. `user_utils.go`).
- Codegen has no projection accessors — projections are used via the generic
  shape methods `ReadAs[R]` / `CreateFrom[R]` / `UpdateFrom[R]`, which require
  Go 1.27+ (`go 1.27rc1` in `go.mod`).

## DTO → schema (reverse direction): `data.SchemaFrom[T]`

Schemas are the source of truth, but anansi also supports the **reverse
direction**: derive a schema JSON document directly from a Go struct via
`data.SchemaFrom[T]` (in `core/data`). This is a public, tested feature — the
DTO shapes the schema without hand-writing a `.schema.json`:

```go
import "github.com/asaidimu/go-anansi/v8/core/data"

type Order struct {
    ID    int       `anansi:"order_id,required=true"`
    Total decimal.Decimal `anansi:"total,required=true"`
    Note  *string   `anansi:"note"` // pointer → nullable
    Tags  []string  `anansi:"tags"` // array with inline element type
}

schemaJSON, err := data.SchemaFrom[Order]()  // []byte, JSON schema
s, err := definition.FromJSON(schemaJSON)     // parse into *definition.Schema
```

**Variants:**
- `data.SchemaFrom[T](omitSystemField ...bool) ([]byte, error)` — default
  `anansi` tag; `omitSystemField[0]=true` skips embedded registered system
  models (e.g. `document.DocumentModel`).
- `data.SchemaFromWithTag[T](tag string, omitSystemField ...bool)` — use a
  custom tag for field-name resolution (dot-separated paths allowed), falling
  back to the `anansi` tag name, then snake-cased Go field name.
- `data.ExtractDTOSchemaDirect(target any, ...)` / `...WithTag` — same, but
  accepts a value instead of a type parameter and streams JSON directly.
- Results are cached globally per `(reflect.Type, tag, omit)`.

**The `anansi` struct tag grammar** (parsed by `parseSchemaTag`):

| Tag form | Meaning |
| --- | --- |
| `anansi:"name"` | Field name; first comma-separated part before any `k=v` |
| `anansi:"-"` | Skip the field entirely |
| `anansi:"name,required=true"` | `required` flag |
| `anansi:"name,nullable=true"` | `nullable` flag (pointers also imply nullable) |
| `anansi:"name,type=<override>"` | Type override: `composite`, `union`, `enum`, or a field type |
| `anansi:"name,values=a\|b\|c"` | Enum values, `\|`-separated |
| `anansi:"name,default=<lit>"` | Default literal (not allowed on `composite`/`union`) |

**Behavior notes:**
- Field names resolve: custom tag → `anansi` tag name → snake-cased Go name.
- Anonymous embedded structs are **flattened**; `omitSystemField` skips
  `document.DocumentModel`-style embeds (name-shadowing via `_id_`/`_metadata_`
  fields works as usual).
- Dot-separated names (`anansi:"billing.address"`) build **synthetic nested
  schemas** automatically.
- Generated field IDs are **deterministic UUIDv7s** (from field ordinal + name),
  and inline array/element schemas carry no `id` (Rule 20), so output
  round-trips through `definition.FromJSON` without drift.

## Composed schemas (request/result envelopes) via `Schema.WithSchema`

To build an **envelope schema** (API request/response, wrapper around a payload
body) from an existing schema — or a DTO-derived schema — embed it as a nested
schema with `definition.Schema.WithSchema(sub)`:

```go
import (
    "github.com/asaidimu/go-anansi/v8/core/common"
    "github.com/asaidimu/go-anansi/v8/core/data"
    "github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// 1. Payload schema — hand-written or DTO-derived:
payloadSchema, err := func() (*definition.Schema, error) {
    raw, err := data.SchemaFrom[Order]()
    if err != nil {
        return nil, err
    }
    return definition.FromJSON(raw)
}()

// 2. Envelope schema that composes the payload as a nested schema:
envelope := &definition.Schema{
    Version:    common.MustNewVersion("1.0.0"),
    BaseSchema: definition.BaseSchema{Name: "OrderRequest"},
}
envelopeWithBody, bodyID, err := envelope.WithSchema(payloadSchema)
if err != nil { /* ... */ }

// 3. Mount the composed body from a root field:
payloadField := definition.Field{
    Name: "payload",
    Required: true,
    FieldProperties: definition.FieldProperties{
        Type:   definition.FieldTypeObject,
        Schema: definition.NewSchemaReference(definition.SchemaReference{ID: bodyID}),
    },
}
envelopeFinal, _, _, err := envelopeWithBody.WithFieldEnsured(&payloadField)
if err != nil { /* ... */ }
```

**How it works:**

- `WithSchema(sub)` returns `(*Schema, SchemaId, error)`. The returned
  `SchemaId` is the handle you attach to a root field via a
  `SchemaReference{ID: ...}` — nothing is auto-mounted, so the envelope's
  own field layout stays under your control.
- `sub.Fields` become the composed body's fields (a **nested** schema — the
  sub's root fields are NOT spliced into the receiver's root level). `sub`'s
  own nested schemas (`sub.Schemas`) are merged into the receiver's nested
  registry.
- **ID collisions are handled**: if a nested `SchemaId` from `sub` already
  exists in the receiver, it is remapped to a fresh UUIDv7 and every field
  reference to the old ID within the merged subtree is rewritten, so the body
  stays internally consistent.
- **Non-mutating**: `WithSchema` deep-copies the receiver; use its result.
  A `nil` sub returns an error.
- The receiver keeps its own root fields untouched — composition adds the
  nested body alongside them, not into them. (Field *flattening* — merging a
  sub's root fields into the receiver's root level — is a different operation
  and is not provided; `WithSchema` is for nesting.)

**Recommended flow for API envelopes:** derive the payload with
`data.SchemaFrom[T]` (or author its schema directly) → `WithSchema` to embed it
→ `WithField`/`WithFieldEnsured` to add envelope-level fields (id, meta,
paging, status) that reference the composed body → `ToJSON()`/`AsMap()` to
write the final `.schema.json`, or hand the `*definition.Schema` straight to
`anansi.Setup`.

## Authoring tips

- **Write field keys as field names, never hand-invented UUIDs.** An LLM cannot
  be trusted to emit valid UUIDv7s, and normalize fixes them anyway.
- `anansi schema normalize <schema>` rewrites field-name keys to UUIDv7 and
  injects the `_id_` / `_metadata_` system fields (with the `_metadata_` nested
  schema) that production collections require.
- `anansi migrate generate` runs the same normalization as a side effect, so
  the schema-change workflow (`migrate generate --dry-run` → `migrate generate`
  → `anansi codegen golang`) both normalizes and migrates in one pass.
- If your schema needs system `_id_` / `_metadata_` fields visible in the
  struct, run `anansi schema normalize` — the fields are then emitted as
  ordinary schema fields.
