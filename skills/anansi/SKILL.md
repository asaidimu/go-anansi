---
name: anansi
description: Build applications on Go-Anansi, the schema-driven hybrid persistence layer for Go. Use this whenever the user is working with anansi anywhere — writing or editing a `.schema.json`, running `anansi codegen`, wiring up a `Playground`/`Setup`, using generated `ModelCollection` types, building `core/query` DSL queries, using projections (`ReadAs`/`CreateFrom`/`UpdateFrom`), generating or squashing migrations, or adding events/decorators. Also use it when the user says "anansi", "go-anansi", "schema-driven persistence", "declare a schema and generate models", or asks to scaffold a new collection/document model in a Go project that already imports `github.com/asaidimu/go-anansi`. If the task is *inside* the go-anansi framework's own source (core/schema, core/query, sqlite, codegen), do not use this skill — that is a contributing task.
---

# Building applications with Go-Anansi

Go-Anansi is a **schema-driven persistence framework**. You declare your data
model once as a JSON schema — then anansi persists it to SQLite, validates and
migrates documents, and generates type-safe Go code (structs, enums,
projections, and a typed collection wrapper). The schema is the single source
of truth; everything else is layered on top of it.

This skill is about **building applications with anansi** — going from a feature
request to a working schema + generated models + CRUD/query code. It is not
about modifying anansi's internals.

The mental model that drives every step: **schema first, then generate, then
code against the generated types.** Resist the temptation to hand-write document
maps everywhere. Declare the shape once, generate, and reuse.

> **Which document pipeline? Use `document.*`.** Anansi ships two document
> implementations. Prefer the **`document`** package (`document.Document`,
> `document.DocumentModel`, `document.New[T]`, `document.DocumentPool`) — it is
> the container/pool-backed fast path that all generated code and the
> `ModelCollection` use. The older map-backed **`data`** pipeline
> (`data.Document`, `data.DocumentModel`) is **deprecated** for building new
> models: it incurs map allocations and pools nothing. Keep using the `data`
> package only where it is a cross-cutting contract (`data.Documenter` interface,
> `data.DocumentIDField`, `data.DocumentSet`, sanitization config/convenience
> constructors). See `references/data-integrity.md`.

---

## The anansi development loop

A typical feature flows through these stages. Do them in this order; jump in at
whatever stage matches what the user is doing.

1. **Locate or scaffold the project.** An anansi project has an `anansi.json`
   config, a `schemas/` directory of `*.schema.json` files, `migrations/`, and
   (usually) generated Go model files. If none exists, `anansi scaffold [dir]`.
2. **Author the schema(s).** One JSON schema per collection, or a root schema
   with nested `schemas`. Field map keys must be stable IDs — **UUIDv7** in
   production mode.
3. **Generate Go code.** `anansi codegen golang` emits structs, a `New<X>`
   constructor, a `<X>s` typed collection, and the idempotent singleton
   (`Init<X>Model` / `<X>Model`). For TypeScript consumers use `anansi codegen
   typescript`; `anansi codegen faker` for fake data.
4. **Wire persistence.** `anansi.Playground(...)` for dev, `anansi.Setup(...)`
   for production. Pass the schemas; collections are created on first start.
5. **Code CRUD/query/business logic** against the generated collection and the
   `core/query` DSL.
6. **Evolve.** When the schema changes, follow the canonical **schema-change
   workflow** in §2 (`migrate generate --dry-run` → `migrate generate` →
   `codegen golang`); optionally `migrate squash`; extend logic.

---

**Do not go digging through the anansi source code to discover patterns.**
Everything you need to build a feature is in this SKILL.md and its reference
files. The anansi API you'll use is fully specified here — exploring
`core/schema`, `core/query`, or the codegen internals is a waste of time (and
risks stale/incorrect guesses from internal types). Only read anansi's own
config (`anansi.json`, existing `.schema.json` files) to match conventions.

---

## 1. Scaffold / baseline

```bash
anansi scaffold myapp                    # interactive in a TTY; accepts defaults
anansi scaffold myapp --no-interactive   # never prompt, all defaults
```

`anansi scaffold` creates the project baseline: `anansi.json`, a `schemas/` dir
(with a starter `example.schema.json`), `migrations/`, `schemas.lock.json`, a
`metadata.schema.json`, and an `AGENTS.md`. For a standalone app it also writes
`go.mod` + `main.go` (SQLite `Playground`, `migrations.Apply`, a demo create/
read).

**The CLI defaults are just starting points — every prompt has a default you
can change.** Interactively (or via flags) you pick the layout:

- **Where the project lives** (`dir`, default `.`)
- **Shape:** new standalone app, or add anansi to an existing Go module
- **Schemas directory** (`--schemas-dir`, default `schemas`)
- **Migrations directory** (`--migrations-dir`, default `migrations`)
- **Lockfile path** (`--lockfile`, default `schemas.lock.json`)

These are written **relative** into `anansi.json` (`schema.glob` derives from
the schemas dir, e.g. `src/defs/**/*.schema.json`), so the project can be
organised however you like and `codegen`/`migrate` keep working from any CWD.

**Adding anansi to an existing project (library mode):**

```bash
anansi scaffold --existing \
  --schemas-dir src/defs --migrations-dir src/migrations
```

Library mode never touches the existing project: no `go.mod`/`main.go` is
written, and **existing files are never overwritten** — an existing `AGENTS.md`,
`metadata.schema.json`, schema files, lockfile, or generated migration/registry
files are left exactly as they are. If the project already owns a schema,
scaffold skips generating its example migration/lockfile/registry entirely; run
`anansi migrate generate && anansi codegen golang` afterwards. It refuses to
re-run on a project that already has an `anansi.json`.

> Agents should use explicit flags with `--no-interactive` (no TTY) and can rely
> on library mode being non-destructive. See `references/tooling-config.md` for
> the full `anansi.json` config reference and every CLI flag.

---

## 2. Authoring schemas

See **`references/schema-format.md`** for the full format. The essentials:

- A schema is a JSON object with `name`, `version`, and a `fields` map.
- **Write field map keys as the field NAME, not a UUID you invent.** An LLM is
  unreliable at producing UUIDv7s, and anansi's `normalize` rewrites them for
  you anyway — so author human-friendly keys like `"shipping_address": {...}`.
  See "Field ID behavior" below, and the change workflow at the end of this
  section.
- Each field: `name`, `type`, plus `required` / `nullable` / `unique` /
  `default` / `deprecated`. Nested information (element type, enum values,
  object shape) lives in `schema`.
- Types: `string`, `number`, `integer`, `boolean`, `bytes`, `array`, `set`,
  `enum`, `object`, `record`, `union`, `composite`, `geometry`.
- Grouped/nested types live under a top-level `schemas` map; those nested
  schemas must use exactly one mode (a `fields`-based schema, or a type-only
  type descriptor — never both).
- **Author everything in the schema, including projections** (under
  `metadata.projections`). Codegen derives BOTH the root model struct AND the
  projection DTOs from the schema. Do not define the schema and the Go
  structs/DTOs separately — the schema is the only source of truth.
- `required` drives the generated Go type: required → value type, optional →
  pointer type. This has real effects — a field marked optional comes back as
  `*T`, so get `required` right up front.

### Field ID behavior (write names, not UUIDs)

Anansi keys field ordering and its lockfile off field IDs, and **production mode
requires UUIDv7 field IDs** (`ERR_REGISTRY_INVALID_SCHEMA` otherwise). Because
hand-typed UUIDs are unreliable, follow this convention:

- While authoring or editing a schema, set each field's `fields` key to the
  field **name** (e.g. `"shipping_address": { ... }`). This is deliberate and
  stable.
- Run `anansi schema normalize <schema>` — or just use the change workflow
  below, whose `migrate generate` normalizes as a side effect — to rewrite
  field-name keys to UUIDv7. Normalize also injects the required `_id_` and
  `_metadata_` system fields and the `_metadata_` nested schema.
- In dev mode (`ANANSI_ENV=development`), field-name keys work as-is; when
  moving to production you must normalize first.
- **Never** hand-edit an existing UUIDv7 field ID to a new value — that breaks
  the lockfile linkage for that field. Preserve existing IDs.

### The schema-change workflow (canonical — follow exactly)

When adding, modifying, or removing fields of an existing schema, do this
sequence and nothing else by hand:

1. **Edit the schema file directly.**
   - Do **not** alter existing field IDs.
   - Only modify field properties or the set of fields.
   - New field → set its `fields` key to match the field's **name** (e.g.
     `"shipping_address": { "name": "shipping_address", "type": "string" }`).
   - Removing a field → delete its entry completely, preserving all other
     fields.
2. **Preview the migration:**
   ```bash
   anansi migrate generate --dry-run
   ```
   (validates the schema and shows what would be normalized + migrated).
3. **Generate the migration:**
   ```bash
   anansi migrate generate
   ```
   (rewrites field-name keys to UUIDv7, injects system fields, and writes a
   versioned migration under `migrations/`).
4. **Regenerate structs and DTOs:**
   ```bash
   anansi codegen golang
   ```
   (re-emits the model structs AND projection DTOs from the updated schema.)

> This single workflow is how "make a schema change" always ends. It also means
> you never hand-write structs — the schema edit + `codegen` produces them.

---

## 3. Code generation

```bash
anansi codegen golang                # full mode: structs + collection + singleton
anansi codegen golang --mode structs # DTOs only, no persistence wrapper
anansi codegen golang --mode model   # model + projections, no collection
anansi codegen typescript            # mirror TS types
anansi codegen faker                 # fake data
anansi agents                        # install this skill locally (git root) ...
anansi agents --global               # ... or into the user's agent skills dir
```

Generated output (mode-dependent) — **canonical, post-normalize form** (the
schema on disk declares `_id_`/`_metadata_`, so they are emitted as ordinary,
schema-typed fields — no shadowing):

```go
type Product struct {            // always embeds DocumentModel
    document.DocumentModel
    ID       string         `anansi:"_id_,required=true" json:"_id_"`
    Name     string         `anansi:"name,required=true" json:"name"`
    Price    float64        `anansi:"price,required=true" json:"price"`
    Metadata *ProductMetadata `anansi:"_metadata_,required=false" json:"_metadata_,omitempty"`
}

// ProductMetadata is the typed struct derived from the injected _metadata_
// nested schema (checksum, created, updated, signature, version) — NOT
// map[string]any. One per collection (root name + "Metadata"), so schemas
// sharing a package get distinct types and keep their own tags.

func NewProduct(model Product) *Product { return document.New(&model) }

type Products struct{ *collection.ModelCollection[*Product] }
const ProductsCollectionName = "Product"

func InitProductsModel(p base.Persistence, logger *zap.Logger, ...) (*Products, error)
func ProductsModel() (*Products, error)   // idempotent singleton
```

> **Do generated structs always shadow? No.** Whether codegen emits *shadow*
> `ID`/`Metadata` fields or ordinary ones depends entirely on whether the
> on-disk schema declares `_id_`/`_metadata_`:
> - **After `schema normalize` / `migrate generate`** (which is the canonical
>   workflow — they write `_id_`/`_metadata_` into the schema file), codegen
>   emits **ordinary fields** exactly as above: `ID string json:"_id_"` and
>   `Metadata *<Root>Metadata json:"_metadata_"` (the type is named
>   `rootName + "Metadata"`, e.g. `ProductMetadata`, so multiple schemas in a
>   package each get their own struct), with the underscore JSON keys.
> - **Before normalization** (schema lacks system fields), codegen falls back
>   to **shadow** fields: `ID string json:"_id_,omitempty"` and
>   `Metadata map[string]any json:"_metadata_,omitempty"` (renamed
>   `ModelID`/`ModelMetadata` if the schema claims those Go names).
>
> Both forms use the underscore JSON keys `_id_`/`_metadata_` (so data
> round-trips), and both still embed `document.DocumentModel`. The generated
> file is overwritten on every run — keep custom methods in SEPARATE files.
> See `references/schema-format.md` -> "Generated Go".

**Naming gotcha:** the collection wrapper is the schema name + `s` — schema
`User` → `Users`, but schema `Products` → `Productss` (the `s` is appended
unconditionally, no de-duplication). The collection name constant always
equals the schema name (`ProductssCollectionName = "Products"`), so wiring a
collection for a schema named `Products` uses `Productss`/`ProductssModel()`.

**Reverse direction (DTO → schema):** when you have Go structs (or want the
schema to be derived from code, not a hand-written JSON), use
`data.SchemaFrom[T]` / `data.SchemaFromWithTag[T](tag)`. It emits a schema
JSON document from a struct's `anansi` tags (name, required/nullable, type
override, enum `values`, default) — pointers → nullable, anonymous structs are
flattened, dotted names create nested schemas. Feed the output through
`definition.FromJSON` to get a `*definition.Schema`. Full tag grammar and
caveats: `references/schema-format.md` -> "DTO → schema (reverse direction)".

**Compose schemas for API envelopes:** to wrap a payload schema (DTO-derived
or hand-written) in a request/response envelope, use
`definition.Schema.WithSchema(sub)` — it embeds the sub-schema as a nested
body and returns its `SchemaId`, which you mount from an envelope root field
via `SchemaReference{ID: bodyID}`. ID collisions are auto-remapped, and the
receiver is never mutated. Walkthrough + example:
`references/schema-format.md` -> "Composed schemas (request/result
envelopes)".

---

## 4. Wiring persistence

Dev / experiments use `Playground` (in-memory SQLite, optional logging,
events, sanitization):

```go
p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
    DBPath:        ":memory:", // or "data.db"
    EnableLogging: true,
    EnableEvents:  true,
    Schemas:       schema.GetSchemas(), // []*definition.Schema
})
if err != nil {
    log.Fatalf("playground: %v", err)
}
defer cleanup()
```

Production uses `Setup` with an explicit `DatabaseInteractor`, `*zap.Logger`,
event bus, and `*utils.Decorators`. Both are guarded by `sync.Once` — only the
first call does work and returns the singleton. Load schemas from embedded JSON
with `definition.FromJSON` (see any example's `schema/registry.go`).

After wiring, bind a generated model:

```go
if _, err := products.InitProductsModel(p, logger); err != nil {
    log.Fatalf("init products model: %v", err)
}
products, _ := products.ProductsModel()
```

---

## 5. CRUD & projections

`*collection.ModelCollection[P]` (embedded in the generated `<X>s`) gives you
all CRUD as promoted methods:

| Method | Behaviour |
| --- | --- |
| `Create(ctx, doc)` / `CreateMany` | Persist, return hydrated struct (ID generated) |
| `FindByID` / `Read(ctx, q)` | Read into the model |
| `Update(ctx, id, partial)` / `UpdateMany` | Partial update; zero fields skipped |
| `Replace(ctx, id, full)` | Full replace |
| `DeleteByID` / `DeleteMany` | Delete (evicts cache) |
| `Validate` / `ValidatePartial` | Schema validation |

**Projections** are declared in schema `metadata.projections` and emitted as
structs that still embed `document.DocumentModel`. Read or write any shape through
the generic shape methods — the caller picks the operation and the shape at the
call site, no per-projection accessors:

```go
q := query.NewQueryBuilder().Where("name").Eq("Laptop").Build()
summaries, err := products.ReadAs[*products.ProductSummary](ctx, &q)

created, err := products.CreateFrom[*products.ProductCreate](
    ctx, &products.ProductCreate{Name: "Mouse", Price: 25.00, Stock: 200})

updated, err := products.UpdateFrom[*products.ProductUpdate](
    ctx, id, &products.ProductUpdate{Stock: 45})
```

Shape methods: `ReadAs[R]`, `CreateFrom[R]`, `UpdateFrom[R]`. They require
Go 1.27+ (generic methods on a concrete type), so the module's `go.mod` must
declare `go 1.27rc1` or later.

### The raw collection & shutdown

The generated `<X>s` is the model wrapper over a raw `base.Collection`. When you
need document-shaped access (scripting, bulk scans, generated queries), get the
raw handle from the persistence object: `coll, err := p.Collection(ctx,
"Product")`. Two differences from the model wrapper:

- **Raw `Read` returns pooled documents that are yours to `Release()`.** The
  model wrapper releases internally; raw `Read` does not.
- **Raw `CreateOne` returns a status, not just an error.** Check
  `res.Status` (`StatusCreated` / `StatusFailedValidation` / `StatusFailedPersistence`)
  — validation failures come back as a status.

**Closing a collection is NOT closing the database.** `coll.Close()` only stops
the managed cache's background goroutines and is a **no-op when no cache is
configured** (the default — a cache is only created if you pass `Cache` /
`CacheConfig` to `Init<X>Model`). It never touches the embedded collection or
the DB. So it is not mandatory to call on the default singleton model; it's
good hygiene to `defer coll.Close()` when a cache *may* be enabled (it's
idempotent). The **store** is closed separately: `cleanup()` from `Playground`,
or closing your `DatabaseInteracter`. See
`references/collection-internals.md`.

---

## 6. Query DSL

> **Stability warning — only a subset of the query engine is stable.** The
> `core/query` engine (`QueryEngine`, `QueryPartitioner`, `QueryHelper`) is a
> known area slated for an overhaul, so **do not build against its internals**
> and treat anything beyond the covered surface as provisional. The stable,
> documented subset is: the fluent **QueryBuilder** DSL, **CASE** expressions
> (`AddCase`/`When`/`Else`), **aggregations** (`GroupBy` + `Count`/`Sum`/`Avg`/
> `Min`/`Max`), **arithmetic pushdown** (`Increment` and `AddComputed` with the
> `ADD`/`MULTIPLY`/... operators), and partitioning of simple
> filter/sort/paginate queries into DB + residual.
> Red flags: `RegisterComputeFunction` / `RegisterFilterFunction` have **no**
> public startup hook today (the engine is built internally —
> `core/persistence/persistence/base.go`), `AddComputed` with a custom function
> name, custom filter operators, and explicit join/subquery negotiation.
> Prefer raw `p.Query(ctx, &query.RawQuery{...})` for anything the DSL can't
> express.

`core/query` offers a fluent builder whose queries are partitioned into SQL
(what the backend supports) plus an in-memory residual:

```go
q := query.NewQueryBuilder().
    From("Orders").
    Where("total").Gt(100.0).   // Eq, Neq, Gt, Gte, Lt, Lte, In, Nin, Contains, ...
    Sort("createdAt", query.SortDirectionDesc).
    Limit(50).
    Build()
```

Compose with `AndFilter`/`OrFilter`/`WhereGroup`, joins (`InnerJoin`,
`LeftJoin`, ...), aggregations (`Sum`, `Avg`, `Count`, `Min`, `Max`),
pagination (`Limit`/`Offset`), and `Select`. See
**`references/query-dsl.md`** for the full method surface.

Query by id: `query.Where(data.DocumentIDField).Eq(id)`.

---

## 7. Harden & extend: caching, decorators, metadata, realtime

When the user moves beyond basic CRUD — caching, cross-cutting concerns,
reactive systems, custom document metadata, or swapping the query backend —
route to the phase-aligned reference below. All content verified.

Phase signals → reference:

- **Wire persistence** (`Setup` vs `Playground`, interactor, query-engine
  augmentation) → `references/persistence-setup.md`
- **Choose docs vs structs for a domain** (schema-first type safety,
  mechanical numeric bulk vs ergonomic string paths) →
  `references/domain-modeling.md`
- **Atomic multi-write** (transfer, order+lines; `p.Transact` /
  `coll.Transact`, nesting, hooks) → `references/transactions.md`
- **Cache it** (`ModelCollection` options, `LiveCollection`, `Release()`
  pooling, the fast path) → `references/caching.md`
- **RLS/audit/validate/encrypt** (decorators) → `references/decorators.md`
- **Custom doc metadata** (declare → regenerate → stub → wire) →
  `references/metadata-providers.md`
- **realtime/notify/metrics/observability** (events, `Stats()`, cleanup
  checklist) → `references/observability.md`

A few signals: "cache it" → `ModelCollection` options or `LiveCollection`;
"RLS/audit/validate/encrypt" → decorators; "react/notify/metrics/dedupe" →
events; "read-through compiled artifacts keyed by a domain field" →
`LiveCollection`; "trace id / caller on every doc" → metadata providers.
Realtime = events + your own transport (no built-in sockets).

---

## 8. Migrations & events (when needed)

- **Migrations:** the migration lifecycle is the canonical **schema-change
  workflow** in §2: edit schema → `anansi migrate generate --dry-run` →
  `anansi migrate generate` → `anansi codegen golang`. `migrate generate` both
  normalizes the schema (field-name keys → UUIDv7) and writes a versioned
  migration under `migrations/`; `anansi migrate squash <col>` consolidates
  intermediate migrations. If migrations exist, apply them on startup.
- **Events/observability:** subscribe to lifecycle events with
  `coll.Subscribe(ctx, base.SubscriptionOptions{ Event: base.DocumentCreateSuccess, Callback: ... })`.
  Cross-cutting concerns (auth, audit, caching) go in `utils.Decorators`.

---

## Verifying your work

The framework fails hard on invalid schemas (`ERR_...` codes) and codegen
fail-fast validates projections. Prefer the real CLI over hand-rolled JSON.
When reviewing output, check:

- Field keys are the field **name** (pre-normalize) or valid UUIDv7 — never a
  made-up UUID.
- `required`/`nullable` matches the desired Go type (pointer vs value).
- Projection `include`/`exclude`/`required`/`optional` don't conflict (codegen
  errors otherwise).
- The generated model isn't hand-modified (extend in separate files).

---

## Reference files

- `references/schema-format.md` — the schema JSON format, field types,
  nested schemas, projections (incl. custom tags), generated Go shape, and the
  DTO → schema reverse path (`data.SchemaFrom[T]`) plus composed envelope
  schemas via `Schema.WithSchema`.
- `references/domain-modeling.md` — how to design a collection schema for a
  domain: step 1 identify the domain (mechanical/ergonomic/structural), step 2
  run a six-question decision matrix (cardinality, presence-exclusivity,
  discriminant, value semantics, hot columns, null-vs-absent), composite vs
  union/polymorphism guidance, and structs-vs-raw-docs — with worked payment and
  chat schemas.
- `references/query-dsl.md` — the full `core/query` builder DSL.
- `references/persistence-setup.md` — wire phase: `Setup` vs `Playground`,
  the interactor/query-backend contract, and how to augment the query engine
  through `DatabaseInteractor`/`Capabilities`.
- `references/transactions.md` — atomic units of work: facade `p.Transact`
  (multi-collection) vs `coll.Transact`, nesting/context propagation,
  concurrent ops + hooks, events, error sentinels.
- `references/caching.md` — cache it: the caching layers (`core/cache`
  primitive, `ModelCollection` id-cache, `LiveCollection`), the document
  `Release()`/pooling contract, and why `ModelCollection` is the fast path
  (`ReadAs`/`CreateFrom`/`UpdateFrom` projections).
- `references/decorators.md` — harden: cross-cutting decorators
  (`utils.Decorators`, RLS/audit/validate/encrypt).
- `references/metadata-providers.md` — harden: the full custom-metadata-provider
  walkthrough (declare → regenerate → implement stub → wire into
  `data.ConfigureDocumentFactory`).
- `references/observability.md` — operate: metrics (`Stats()`, event counters),
  realtime via events + your own transport, and the lifecycle/cleanup
  checklist.
- `references/collection-internals.md` — the request path from your call to
  the database, layer by layer (ModelCollection → decorators → events →
  managed → polyfill → base → QueryEngine → interactor), plus every method on
  a collection as "What happens when I..." / "How do I...", and a
  debugging table mapping errors to the layer that threw them.
- `references/tooling-config.md` — the `anansi.json` config reference and the
  full CLI flag surface (layout, codegen, migrate, scaffold).

Read one when you need its details; don't load them all up front.
