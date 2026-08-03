# Go Code Generation & Model Collection

`go-anansi` ships a Go code generator (`codegen/golang`) that compiles a schema
definition (the JSON format described in `schema.json` / `schema_rules.md`) into
type-safe Go model code. The generated code builds on two runtime pieces:

- **`data.DocumentModel`** — the embedded struct providing `id`, `metadata`, and
  the promoted `Document()`/`Patch()` methods.
- **`collection.ModelCollection[P]`** — a type-safe wrapper over
  `base.Collection`, fixed to one model but able to read/write documents
  through any number of *projections*.

This document explains the input schema, the generated output, and how the
model collection binds them together.

---

## 1. Generation modes

Generation is layered. The `mode` selects how much of the stack is emitted:

| Mode | Emits | Typical use |
| --- | --- | --- |
| `structs` | Plain structs, aliases, enums — **no** `DocumentModel` embed | Pure DTOs |
| `model` | `structs` + the root struct with `data.DocumentModel` and a `NewX` constructor + projection structs | Models without a persistence wrapper |
| `collection` (`full`, default) | `model` + the typed collection wrapper, name constant, singleton, `InitXModel`/`XModel` accessors | Full persistence stack |

```go
gen := golang.NewGoGenerator(&golang.GeneratorConfig{
    PackageName: "user",
    TagConfig:   golang.DefaultTagConfig(),
})
src, err := gen.Generate(schemaJSON) // []byte
```

The `TagConfig` controls which struct tags are emitted (default: `json` and
`anansi`). The `anansi` tag is the canonical schema tag consumed by
`core/data` for document conversion and binding.

---

## 2. Input schema

The generator reads a single root schema plus nested schemas. Projections are
declared under `metadata.projections` (see §5).

```json
{
  "name": "User",
  "fields": {
    "f1": { "name": "email", "type": "string", "required": true },
    "f2": { "name": "name",  "type": "string", "required": true },
    "f3": { "name": "age",   "type": "integer", "nullable": true },
    "f4": { "name": "role",  "type": "enum", "schema": { "type": "string", "values": ["admin", "member"] } }
  },
  "metadata": {
    "projections": {
      "UserCreate":  { "fields": { "include": ["email", "name", "age"], "required": ["email", "name"] } },
      "UserSummary": { "fields": { "include": ["email", "name"] } },
      "UserUpdate":  { "fields": { "exclude": ["role"], "optional": ["age"] } }
    }
  }
}
```

---

## 3. Generated code

From the schema above, `full` mode produces the following (annotated).

### 3.1 Enums

Named and inline enums become typed string constants:

```go
type UserRoleEnum string

const (
    UserRoleEnumAdmin UserRoleEnum = "admin"
    UserRoleEnumMember UserRoleEnum = "member"
)
```

### 3.2 The model struct

The root struct embeds `document.DocumentModel` (the `_id_`/`_metadata_` system
fields) and every field carries its `anansi` tag. Field shape is driven by
`required`/`nullable`:

- **Required, non-nullable** → value type (`Email string`).
- **Optional or nullable** → pointer type (`Age *int64`, `Role *UserRoleEnum`).

When the schema does **not** declare the system fields, codegen additionally
emits shadow `ID`/`Metadata` fields mirroring the embedded model's fields, plus
an explicit `GetID()`. The shadows expose the system fields as ordinary struct
fields (so `document.New` keeps them in sync with the embed); if the schema
already declares a field that would claim the Go name `ID` or `Metadata`, the
shadow is renamed to `ModelID`/`ModelMetadata`. When the schema **does** declare
`_id_`/`_metadata_` (e.g. after `anansi schema normalize`), they are emitted as
ordinary schema fields instead — never duplicated:

```go
type User struct {
    document.DocumentModel
    ID string            `json:"id,omitempty" anansi:"_id_,required=true,omitempty"`
    Email string         `anansi:"email,required=true" json:"email"`
    Name  string         `anansi:"name,required=true" json:"name"`
    Age   *int64         `anansi:"age,required=false,nullable=true" json:"age,omitempty"`
    Role  *UserRoleEnum  `anansi:"role,required=false,type=enum" json:"role,omitempty"`
    Metadata map[string]any `json:"metadata,omitempty" anansi:"_metadata_,required=true,omitempty"`
}

// GetID returns the document identifier for User.
func (m *User) GetID() string {
    return m.ID
}

// NewUser creates and initializes a new User
func NewUser(model User) *User {
    return document.New(&model)
}
```

> **Shadow naming.** The `_id_` shadow is emitted as `ID` unless the schema
> already declares a field that claims that Go name (e.g. a field named `id`),
> in which case the shadow is renamed to `ModelID`. `GetID()` always returns the
> `_id_` shadow, so `GetID()` is the record identifier. The shadow `Metadata`
> field is the readable form of `_metadata_` on the struct; it is populated on
> reads and left out of partial updates so system-managed metadata is never
> clobbered.

### 3.3 The collection wrapper

The wrapper embeds `*collection.ModelCollection[*User]`, so every CRUD method
is promoted onto `*Users`:

```go
// Users is a type-safe collection for User
type Users struct {
    *collection.ModelCollection[*User]
}

const UsersCollectionName = "User"

var (
    usersModelMu sync.Mutex
    usersModel   *Users
)
```

Initialization is idempotent and retry-safe:

```go
func InitUsersModel(p base.Persistence, logger *zap.Logger, opts ...collection.ModelCollectionOptions[*User]) (*Users, error) {
    // ... builds the raw collection, then:
    mc, err := collection.NewModelCollection[*User](raw, logger, opts...)
    // ...
    usersModel = &Users{ModelCollection: mc}
    return usersModel, nil
}

func UsersModel() (*Users, error) { /* returns the singleton */ }
```

### 3.4 Projection structs

Every projection is emitted as a struct embedding `data.DocumentModel`, which
makes it a valid `data.DocumentModelProvider`:

```go
type UserSummary struct {
    data.DocumentModel
    Email string `anansi:"email,required=true" json:"email"`
    Name  string `anansi:"name,required=true" json:"name"`
}
```

Codegen deliberately emits **no** accessor methods on the collection wrapper
for projections. How a projection is read or written is a caller decision —
they are consumed through the generic shape methods (§4.2) with the caller
picking the operation and the argument order, not a codegen-prescribed API.

---

## 4. The model collection

`collection.ModelCollection[P]` is the runtime behind all of this. It wraps a
`base.Collection` and fixes a single model `P` — always a **pointer** to a
struct embedding `data.DocumentModel`.

```go
mc, err := collection.NewModelCollection[*User](raw, logger)
```

### 4.1 Standard CRUD (fixed to `P`)

| Method | Behaviour |
| --- | --- |
| `Create(ctx, doc P)` / `CreateMany` | Persist and return the hydrated model (IDs/timestamps generated) |
| `FindByID(ctx, id)` / `Read(ctx, q)` | Read into `P`; optional read-through cache |
| `Update(ctx, id, update P)` / `UpdateMany` | Partial update; zero fields skipped |
| `Replace(ctx, id, replacement P)` | Full replacement |
| `DeleteByID` / `DeleteMany` | Delete, evicting cache entries |
| `Validate` / `ValidatePartial` | Schema validation |

`ModelCollectionOptions[P]` configures caching and auto-load:

```go
mc, err := collection.NewModelCollection[*User](raw, logger,
    collection.ModelCollectionOptions[*User]{
        CacheConfig: &cache.CacheConfig{MaxEntries: 100},
        AutoLoad:    true,
    },
)
```

### 4.2 Shape operations (projections)

Because a projection still embeds `data.DocumentModel`, it is a valid type
argument for the generic shape methods — one collection instance serves any
subset of the schema. The caller supplies the projection type explicitly:

```go
// read documents as a summary shape (only email/name bound)
q := query.NewQueryBuilder().Where("name").Eq("Ada").Build()
summaries, err := users.ReadAs[*UserSummary](ctx, &q)

// partial update applied from the projection's fields
updated, err := users.UpdateFrom[*UserUpdate](ctx, id, &UserUpdate{Age: newAge})

// create a user from a create shape (no role supplied)
created, err := users.CreateFrom[*UserCreate](ctx, &UserCreate{Email: "a@b.c", Name: "Ada"})
```

The generic shape methods are: `ReadAs[R]`, `CreateFrom[R]`, `UpdateFrom[R]`.
There is no shape-based find-by-id or read-by-id — fetching by id is
`FindByID`/`Read` on the model type, or an id-filtered `ReadAs`. Because the
caller picks both the operation and the projection type at the call site,
codegen never has to guess how a projection will be used.

Implementation notes:

- **ReadAs** binds the full stored document into `R`; fields the projection
  omits are simply dropped.
- **CreateFrom / UpdateFrom** build the document from the shape with
  `structToMap`, which skips the `DocumentModel` system embed on partial
  writes — an update never clobbers `id`/`metadata`.
- Shape results are **not** stored in the model-typed cache (`R ≠ P`).
- Generic shape methods require **Go 1.27** (generic methods on concrete
  types). `go.mod` must declare `go 1.27rc1` or later.

### 4.3 Cache semantics

When caching is enabled, `P` instances are keyed by document `_id`. Reads
populate the cache, `FindByID` serves positive/negative hits, and
writes/deletes keep it coherent. `AutoLoad` preloads the whole collection at
construction.

---

## 5. Projection DSL reference

Projections are declared under `metadata.projections`, keyed by name. Each
entry describes a field subset and optional constraint/tag overrides over the
root schema's fields:

| Key | Type | Meaning |
| --- | --- | --- |
| `fields.include` | `[]string` | Whitelist; default is all root fields |
| `fields.exclude` | `[]string` | Remove fields from the final set |
| `fields.required` | `[]string` | Force `required=true` on those fields |
| `fields.optional` | `[]string` | Force `required=false` on those fields |
| `fields.tags` | `map[string]map[string]string` | Custom struct tags per field, with `{prop}` placeholders |

```json
{
  "UserCreate": {
    "fields": {
      "include":  ["email", "name", "age"],
      "required": ["email", "name"],
      "tags": {
        "email": { "input": "arguments.{name}" }
      }
    }
  }
}
```

Resolution order: start from the base field set → apply `include` (whitelist)
→ apply `exclude` → apply `required`/`optional` flag overrides → append
`tags`.

### 5.1 Custom tags and `{prop}` placeholders

Tag values may reference the field's resolved properties:

| Placeholder | Example |
| --- | --- |
| `{name}` | `email` |
| `{type}` | `string` |
| `{required}` / `{nullable}` | `true` / `false` |
| `{default}` | `admin` (empty when unset) |
| `{goName}` | `Email` |

The tag above emits `Email string \`anansi:"email,required=true" json:"email" input:"arguments.email"\``.

### 5.2 Fail-fast rules

Codegen errors on: unknown fields, `include` ∩ `exclude`, `required` ∩
`optional`, `required`/`optional`/`tags` referencing fields outside the final
set, and projection names that collide with the root model type.

### 5.3 `required` has real effects

Overriding `required` changes both the emitted Go type and the `anansi` tag:

- `email` is required in the base schema → `Email string` (value).
- `age` is optional → `Age *int64` (pointer), even inside a projection.
- A projection marking a base-optional field as `required` upgrades it to a
  value type with `required=true`.

---

## 6. Wiring it together

```go
p, cleanup, _ := anansi.Playground(anansi.PlaygroundConfig{Schemas: schemas})
defer cleanup()

if _, err := user.InitUsersModel(p, logger); err != nil {
    log.Fatal(err)
}

users, _ := user.UsersModel()

created, _ := users.CreateFrom[*user.UserCreate](ctx, &user.UserCreate{Email: "a@b.c", Name: "Ada"})
id := created.Model().ID

q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build()
summaries, _ := users.ReadAs[*user.UserSummary](ctx, &q) // []*UserSummary
got, _ := users.FindByID(ctx, id)                        // *User (promoted from ModelCollection)
_ = summaries, got
```

---

## 7. Related documents

- `dto_spec.md` — the `anansi` tag spec and Go → schema inference (the reverse
  direction).
- `schema_ir.md` — the schema JSON representation consumed by codegen.
- `AGENTS.md` — run `make test` to regenerate and verify golden outputs
  (`codegen/golang/testdata/*.golden`, `-update` flag).
