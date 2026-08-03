# Go Struct Schema Specification (`anansi` Tag Spec) For Models

This specification defines how idiomatic Go structs map deterministically to the **Meta‑Schema JSON Definition** (see `schema.json`), and how that mapping satisfies the structural rules in `schema_rules.md`.

It covers **domain models** – structs embedding `data.DocumentModel` for persistence, as well as plain data containers used for serialisation and API exchange. Models use the `anansi` struct tag for schema metadata and the `json` tag for wire format.

The specification prioritises **Developer Experience (DX)** and **Model correctness**:

- **Idiomatic Go Unions**: Modeled as structs with optional pointer fields (`*TypeA`, `*TypeB`) where only one field is set.
- **Idiomatic Go Composites**: Modeled using struct embeddings, with no other fields on the composite container.
- **Default Field Promotion**: Embedded structs flatten into top‑level fields by default, unless the field is explicitly tagged as `type=composite`.
- **Order‑Preserving IDs**: Schema and Field UUIDs are generated deterministically as **RFC 4122 UUIDv7**, with the timestamp component encoding discovery/declaration order — so sorting a JSON object's keys as strings recovers the original Go ordering, with no separate `index` property needed anywhere in the meta‑schema.
- **Auto‑Initialization**: `data.New[T]()` generates UUID identities, sets metadata timestamps, and wires the parent reference for promoted `Document()` and `Patch()` methods.

---

## 1. Type Inference Engine

Unless explicitly overridden by a `type=` tag directive, the schema extractor inspects native Go types and infers the target schema type automatically:

| Native Go Type | Inferred Schema `type` | Reference Form | Notes |
| --- | --- | --- | --- |
| `string` | `string` | None* | Primitive type |
| `int`, `int8`–`int64`, `uint`–`uint64` | `integer` | None* | Whole number |
| `float32`, `float64` | `number` | None* | IEEE 754 double‑precision |
| `bool` | `boolean` | None* | Boolean |
| `[]byte` | `bytes` | None | Binary payload |
| `any`, `interface{}` | `unknown` | None | Unconstrained escape hatch |
| `[][]float64`, `[][]float32` | `geometry` | None | Numerical tuple array (see §12, known limitation) |
| `struct` (named) | `object` | Named Reference (`Form 1`) | References sub‑struct schema ID |
| `map[string]T` (primitive `T`) | `record` | Inline Type Descriptor (`Form 3`) | Typed key‑value map with primitive values |
| `map[string]T` (struct `T`) | `record` | Named Reference (`Form 1`) | Map of complex objects |
| `[]T` (primitive `T`) | `array` | Inline Type Descriptor (`Form 3`) | Array of primitive values |
| `[]T` (struct `T`) | `array` | Named Reference (`Form 1`) | Array of schema references |
| `*T` (Pointer) | *(Base Type)* | *(Base Form)* | Automatically sets `nullable: true` |

\* **Exception**: when a primitive appears as a variant inside a `type=union` container, it is *not* left inline — see **§3.1 (Primitive Union Variants)**. Outside of a union, primitives never get a `schemas` entry and never carry a `schema` reference (per `schema_rules.md` Rule 3).

> **Important**: When the element type `T` is itself a struct, the `array` field must reference the struct's schema via a **Named Reference** (`{ "id": "..." }`). When `T` is a primitive scalar (`string`, `number`, etc.), the `array` field may use an **Inline Type Descriptor** (`{ "type": "string" }`). This aligns with **Rule 20** (Form 3) and **Rule 7** (Array field semantics).

---

## 2. Struct Tag Syntax Reference

All field‑level overrides use the single `anansi` tag namespace:

```go
`anansi:"[<n>][,flag1][,key1=value1]"`
```

### Directives Summary

| Directive | Type | Description | Example |
| --- | --- | --- | --- |
| **`name`** | String | Field name in JSON payload. Converts struct field to `snake_case` if omitted. `-` skips the field. | `anansi:"email_address"` |
| **`required`** | Boolean | Sets `required: true` or `false`. Default: `true` for non‑pointer types, `false` for pointer types (unless overridden). | `anansi:"email,required=true"` or `anansi:"email,required=false"` |
| **`nullable`** | Boolean | Sets `nullable: true` or `false`. Default: `true` for pointer types, `false` otherwise. | `anansi:"bio,nullable=true"` |
| **`default`** | Value | Sets the default value (must match the field type). | `anansi:"role,default=member"` |
| **`type`** | Enum | Overrides inferred type. Allowed: `decimal`, `enum`, `union`, `composite`, `geometry`, `bytes`, `unknown`. | `anansi:"content,type=union"` |
| **`values`** | Pipe‑List | Defines inline enum values. Must match the underlying scalar type. | `anansi:"status,type=enum,values=active\|pending\|inactive"` |

> **Note on Model semantics**: Flags like `deprecated` or `unique` are **not** supported. Models are domain entities; deprecation and uniqueness are persistence‑layer concerns. If you need such metadata, consider adding them manually to the final schema JSON or using a separate schema extension mechanism.

### Tag resolution order

When binding a Document back to a struct, the runtime reads the `anansi` tag first. If absent, it falls back to the `doc` tag for backward compatibility (both use the same parser). The `json` tag is used only for serialization — it does not influence schema generation or binding.

---

## 3. Model Pattern

Domain models embed `data.DocumentModel` to inherit identity, metadata, and lifecycle methods:

```go
type User struct {
    data.DocumentModel
    Username string `json:"username" anansi:"username"`
    Email    string `json:"email" anansi:"email,required"`
}
```

### Initialization

Use `data.New[T]` to create an initialized model with auto-generated UUID v7 identity and metadata:

```go
user := data.New(&User{Username: "alice", Email: "alice@example.com"})
// user.ID is set, user.Metadata has created/updated timestamps and version
```

`data.New[T]` returns the same pointer for chaining:

```go
doc, _ := user.Document()    // promoted from DocumentModel — converts to full Document
patch, _ := user.Patch()     // partial Document suitable for partial updates
```

Code generated by `anansi schema codegen golang` produces constructors automatically:

```go
user := NewUser()  // returns *User initialized via data.New
user.Username = "alice"
```

### Typed Collection Wrapper

For each root schema struct, the generator emits a type-safe collection wrapper embedding `base.ModelCollection[T, *T]`:

```go
type Users struct {
    base.ModelCollection[User, *User]
}

func (c *Users) Create(ctx context.Context, model *User) (*User, error) {
    return c.ModelCollection.Create(ctx, model)
}
// ... FindByID, Read, Update, DeleteByID, etc.
```

---

## 4. Schema Discovery, Deduplication, and Traversal Order

The generator builds a complete set of schemas by traversing the type graph starting from the root struct, **depth‑first, in Go struct field declaration order** (embedded fields are visited in the position they're declared, exactly as they'd appear if you expanded them inline). This traversal order is what the ID scheme in §5 encodes — it is not just an implementation detail, it's part of the spec.

- **Recursive traversal**: For each field, in declaration order, the generator inspects its type. If the type is a struct (named or anonymous), it must be processed to produce a schema.
- **Stop conditions**: The traversal stops when encountering:
  - Primitive types (`string`, `number`, `integer`, `decimal`, `boolean`, `bytes`, `unknown`) — **except** as described in §4.1.
  - `geometry` and other built‑in types that are not structs.
  - Types that have already been processed (see deduplication below).
- **Deduplication via registry**: A single registry (map) is maintained for the entire generation run, keyed by the **fully qualified type name** (e.g., `github.com/myproject/User`, or a primitive's bare name such as `int64` — see §4.1). The registry assigns each newly‑discovered type the next sequential **discovery ordinal**, starting at `0` for the root type. When a type is first encountered, the generator:
  1. Computes its schema ID using the deterministic UUIDv7 method (§5), consuming the type's discovery ordinal.
  2. Adds a new entry to the top‑level `schemas` map with that ID.
  3. Processes the type's fields recursively (if it has fields).
  4. Marks the type as processed in the registry, alongside its assigned ID.
- **Subsequent references**: If the same type is encountered again (e.g., multiple fields of the same type, or circular references), the generator **does not** reprocess it and **does not** consume a new ordinal. It simply creates a **Named Reference** (`{ "id": "<uuid>" }`) using the already‑computed ID. This ensures each type appears exactly once in `schemas`, with all references pointing to the same ID.
- **Circular references** (Rule 16): The registry prevents infinite loops. When a cycle is detected, the generator finds the first struct already in the registry and uses a named reference, halting further recursion on that branch.
- **Anonymous structs**: They are given a synthetic fully qualified name derived from the parent type and field name (e.g., `ParentType_FieldName`). They are processed like named structs — including consuming a discovery ordinal — but are not represented as a distinct Go type; they still appear in the `schemas` map with a generated name.
- **No fixed depth limit**: The generator traverses until all reachable types have been discovered. In rare cases of extremely deep non‑cyclic nesting (e.g., >10,000 levels), an iterative implementation may be needed to avoid stack limits, but such nesting is unrealistic for models.

### 4.1 Primitive Union Variants (Registry Exception)

Rule 20 mandates that `SchemaReferenceArray` (Form 2 — used exclusively by `union` and `composite` fields) **never** contains inline entries; every element must be `{ "id": "..." }`. This creates an unavoidable exception to the "primitives are stop conditions" rule above:

> **When a union variant's pointee type is a primitive scalar** (e.g. `Number *int64` inside a `type=union` container), the generator **must** register a Type‑mode schema for that primitive in the same registry described above, keyed by the primitive's bare Go type name (`int64`, `string`, `bool`, …) rather than a package path. It consumes a discovery ordinal exactly like a struct would.
>
> This registration is **global and shared**: every `*int64` used as a union variant anywhere in the type graph — regardless of which struct declares it — resolves to the *same* synthesized schema (name suggestion: `<type>_type`, e.g. `int64_type`), by the same dedup rule that applies to structs. The synthesized schema has no `fields`; it's a bare Type‑mode entry, e.g. `{ "name": "int64_type", "type": "integer" }`.
>
> Outside of a union context, this exception does not apply — a plain `*int64` field (not part of a union container) stays inline, per §1.

This is the only place primitives enter the `schemas` map. Composite variants never trigger this exception, because Rule 5 requires every composite member to be effectively object‑shaped, and primitives never qualify — see §7 Pattern B.

---

## 5. Deterministic UUIDv7 ID Resolution

Developers do not manually specify UUIDs. IDs are derived deterministically, and — this is the important part — **the derivation is order‑preserving by construction**, so that consumers can recover Go's declaration order from a schema without any extra metadata field.

```
timestamp_ms(ordinal) = FIXED_EPOCH_MS + ordinal
rand_bits             = SHA256(seed_string)   // supplies rand_a (12 bits) and rand_b (62 bits)

SchemaID = UUIDv7(timestamp_ms(discovery_ordinal), rand_bits(FullyQualifiedTypeName))
FieldID  = UUIDv7(timestamp_ms(field_ordinal),      rand_bits(OwningSchemaID + FieldName))
```

Where:

- `FIXED_EPOCH_MS` is a fixed constant (e.g. `2026-01-01T00:00:00.000Z` in Unix milliseconds), giving ~285,000 years of ordinal headroom at 1ms resolution — far beyond any realistic struct's field or type count.
- `discovery_ordinal` is the type's position in the **global** discovery registry described in §4 (0 for the root type, incrementing as new types — including synthesized primitive schemas per §4.1 — are first encountered).
- `field_ordinal` is the field's position in its **owning schema's flattened field list** — i.e., the order fields actually appear after default‑embedding promotion (§7 Pattern A) has been applied. This ordinal is scoped **per schema**, not global: it restarts at `0` for every schema's own `fields` map.
- `OwningSchemaID` is always the ID of the schema whose `fields` map the field will physically live in. For a promoted (flattened) embedded field, this is the **parent/promoting schema's ID**, never the embedded struct's own identity — see §7 Pattern A for why this matters.
- The SHA‑256 hash supplies the 74 random bits required by UUIDv7 (`rand_a` and `rand_b`). The version (`0b0111`) and variant (`0b10`) bits are set per RFC 4122 regardless of the ordinal or hash content.

**Why the timestamp encodes order, not a shared constant:** UUIDv7 sorts lexicographically by its timestamp prefix. JSON objects (`fields`, `schemas`) don't guarantee key order on the wire or after re‑serialization — but if the timestamp component is a strictly increasing function of discovery/declaration order, then **sorting the map's keys as UUID strings deterministically reconstructs the original Go ordering**, with no separate `index`/`order` property required anywhere in the meta‑schema. A single shared constant timestamp (as opposed to an ordinal‑derived one) would throw this property away entirely and provide no benefit over a plain hash‑based ID.

**Why arrays don't need this trick:** `SchemaReferenceArray` (Form 2, used by `union`/`composite`) is a JSON **array**, and JSON arrays already preserve order natively. The ordinal‑timestamp scheme in this section applies only to the two JSON **objects** in the meta‑schema that are keyed by UUID — `schemas` and each schema's `fields` — because those are the only places order can otherwise be lost.

Because a type's schema ID is derived from its discovery ordinal plus a hash of its fully qualified name, **every reference to the same type across the schema resolves to the same ID**, and re‑running the generator against unchanged source produces byte‑identical output.

---

## 6. Required, Nullable, and Optional Semantics

| Situation | Default `required` | Default `nullable` | How to override |
| --- | --- | --- | --- |
| Non‑pointer type (e.g. `string`) | `true` | `false` | Use `required=false` or `nullable=true` |
| Pointer type (e.g. `*string`) | `false` | `true` | Use `required=true` or `nullable=false` |

Both flags are explicit booleans. The generator **must** set the corresponding properties in the meta‑schema field definition.

---

## 7. Handling Anonymous (Inline) Structs

When a field's type is an anonymous struct, e.g.

```go
type Container struct {
    Info struct {
        Name string `anansi:"name,required"`
    } `anansi:"info"`
}
```

the generator **must** create a synthetic named schema for the anonymous struct. The schema name is derived deterministically from the parent type and field name, e.g., `Container_Info`. This schema is added to the top‑level `schemas` map (consuming the next discovery ordinal, per §4), and the field `info` is generated as an `object` with a named reference to that schema.

This ensures compliance with **Rule 8** (object fields must reference a schema with `fields`) and **Rule 20** (inline type descriptors are not allowed for `object`).

---

## 8. Go Structural Patterns & Meta‑Schema Mapping

### Pattern A: Default Embeddings (Top‑Level Field Promotion)

In Go, embedding a struct without a field tag promotes its fields to the parent struct. The schema generator **flattens** these fields directly into the parent's `fields` dictionary, in the position the embed itself appears among the parent's declared fields.

```go
type AuditMixin struct {
    CreatedAt time.Time `anansi:"created_at,required"`
    UpdatedAt time.Time `anansi:"updated_at,required"`
}

type User struct {
    data.DocumentModel
    AuditMixin // Promoted directly into User's field map, ordinals 0 and 1
    Username string `anansi:"username,required"` // ordinal 2
}
```

*Generated Output*: `User` contains `created_at`, `updated_at`, and `username` directly in its `fields` dictionary, in that order. No separate schema is created for `AuditMixin` (unless it is referenced elsewhere as its own named type, in which case it's discovered and registered independently on that reference, unaffected by the promotion happening here).

**System-model embeds (`data.DocumentModel`, `document.DocumentModel`):** unlike user-defined embeds, a registered system model's own `_id_`/`_metadata_` fields **are** flattened into the parent (only the internal `parent` field is skipped via `anansi:"-"`). Generated models may additionally declare their own non-anonymous fields carrying the reserved `_id_`/`_metadata_` names (shadow fields — see `docs/codegeneration.md` §3.2). When both exist, the shadow wins: the embed's `_id_`/`_metadata_` are skipped so the schema never contains duplicate system fields, and the shadow's own tags (required, default) drive the extracted field.

**Field ID scoping for promoted fields (why this matters):** Per §5, `created_at`'s `FieldID` hashes `User`'s schema ID + `"created_at"`, and its ordinal is `0` — its position in `User`'s own flattened field list — **not** any ordinal or identity belonging to `AuditMixin`. If `AuditMixin` is embedded into a second struct (say `Order`), that struct's `created_at` gets a *different* `FieldID`, because it hashes against `Order`'s schema ID instead. This is required by Rule 1 (global field ID uniqueness): if promoted fields instead inherited an identity tied to the unregistered `AuditMixin` type, every struct embedding it would collide on the same field ID.

---

### Pattern B: Composite Schema (`type=composite`)

A **Composite** represents a field that must satisfy *all* referenced sub‑schemas simultaneously. In Go, this is modeled as a struct that **embeds** the component structs, and contains **nothing else**.

When a field is tagged with `type=composite`, the generator **must not** flatten the embedded structs into the parent. Instead, it inspects the field's struct type, extracts all embedded struct types (each discovered/registered independently per §4, satisfying Rule 5's requirement that composite members be Schema‑mode object schemas), and generates a `composite` field whose `schema` is a **Named Reference Array (Form 2)**, in embed declaration order — no ordinal‑encoding trick needed here, since it's a JSON array (§5). The container struct itself is **not** registered as a schema.

**Validation rule**: a `type=composite`‑tagged struct **must** consist exclusively of embedded struct fields. If the generator encounters a directly declared (non‑embedded) field on a composite container, it **must reject with an error** rather than silently discarding that field's data — the container itself has no schema of its own to hold it, so there's no valid place for it to end up in the output.

```go
type TextContent struct {
    Body string `anansi:"body,required"`
}

type MediaContent struct {
    URL string `anansi:"url,required"`
}

// Composite container embedding sub‑structs — nothing else is allowed here
type AttachmentComposite struct {
    TextContent
    MediaContent
}

type Post struct {
    data.DocumentModel
    ID         string              `anansi:"id,required"`
    Attachment AttachmentComposite `anansi:"attachment,type=composite"`
}
```

*Generated Schema Output*:

```json
{
  "name": "attachment",
  "type": "composite",
  "schema": [
    { "id": "<UUID-of-TextContent>" },
    { "id": "<UUID-of-MediaContent>" }
  ]
}
```

---

### Pattern C: Union Schema (`type=union`)

A **Union** represents a payload where **exactly one** of the declared options is populated. In Go models, this is expressed as a struct containing **optional pointer fields** – one per variant, and nothing else.

A field tagged with `type=union` must have a struct type where **every field is an optional pointer** (`*T`). If any field is non‑pointer, or the container has fewer than two pointer fields, the generator **must reject with an error** — a union needs at least two mutually‑exclusive variants to mean anything. The generator does **not** create a schema for the container struct itself. Instead, it builds a `union` field with a `SchemaReferenceArray` (Form 2), in declared field order, containing the schema of each pointer type's pointee:

- If the pointee is a **struct**, it's discovered/registered normally (§4) and referenced by its real ID.
- If the pointee is a **primitive scalar**, the exception in **§4.1** applies: the generator synthesizes and registers a shared, deduplicated Type‑mode schema for that primitive, and references *that*.

```go
// Union container: only one pointer field will be populated at runtime
type ContentUnion struct {
    Text   *TextContent  `anansi:"text"`
    Media  *MediaContent `anansi:"media"`
    Number *int64        `anansi:"number"`
}

type Message struct {
    data.DocumentModel
    ID      string       `anansi:"id,required"`
    Payload ContentUnion `anansi:"payload,type=union"`
}
```

*Generated Schema Output*:

```json
{
  "name": "payload",
  "type": "union",
  "schema": [
    { "id": "<UUID-of-TextContent>" },
    { "id": "<UUID-of-MediaContent>" },
    { "id": "<UUID-of-int64_type>" }
  ]
}
```

```json
{
  "name": "int64_type",
  "type": "integer"
}
```

Note that `int64_type` above is registered once, globally, in `schemas` — any other `*int64` union variant elsewhere in the type graph references this same schema ID rather than generating a duplicate.

The Go type system cannot itself enforce "exactly one populated" at compile time — that's a runtime/data‑level concern matching Rule 6 ("data must match ONE of the referenced schemas"), not something the generator can validate structurally beyond checking that the container shape is well‑formed (all‑pointer, ≥2 variants).

---

### Pattern D: Enums (Inline vs. Named)

1. **Inline Enum**: Defined on a scalar field using `values=a|b|c`. The generator will produce an `enum` field with an **Inline Type Descriptor (Form 3)** containing the `type` and `values` keys. No schema is registered.

```go
type Order struct {
    data.DocumentModel
    Status string `anansi:"status,type=enum,values=draft|submitted|fulfilled"`
}
```

*Generated Output*:

```json
{
  "name": "status",
  "type": "enum",
  "schema": { "type": "string", "values": ["draft", "submitted", "fulfilled"] }
}
```

2. **Named Enum**: If the enum is defined as a separate type and reused, the generator will reference its schema via `schema: { "id": "..." }`, and the type is discovered/registered like any other named type (consuming a discovery ordinal per §4). The enum schema itself is defined in `schemas` with `type`, `values`, and no `fields`.

---

## 9. Inline Type Descriptor Rules

The generator may produce **Inline Type Descriptors (Form 3)** only in the following contexts:

- As the `schema` of an **`array`** field, when the element type is a **scalar primitive** (`string`, `number`, `integer`, `decimal`, `boolean`, `bytes`, `unknown`).
  Example: `{ "type": "array", "schema": { "type": "string" } }` → `Array<string>`.
- As the `schema` of a **`record`** field, when the value type is a **scalar primitive**.
  Example: `{ "type": "record", "schema": { "type": "integer" } }` → `Record<string, integer>`.
- As the `schema` of an **`enum`** field (inline enum) – it must contain `type` (scalar) and `values`.
  Example: `{ "type": "enum", "schema": { "type": "string", "values": ["a","b"] } }`.

**Inline descriptors are never allowed** for the structural types: `array`, `object`, `enum` (unless as above), `union`, `composite`, `geometry`, or anywhere inside a `SchemaReferenceArray` (Form 2) — that's why primitive union variants require the synthesized-schema exception in §4.1 rather than staying inline. Inline descriptors are also **not** allowed for `object`, or for `array`/`record` when the element/value type is a struct — those must use named references.

The generator must reject any attempt to create an inline descriptor that violates these rules.

---

## 10. Default Value Type Matching

Default values provided via `default=...` **must** be compatible with the field's declared type. The generator must perform a type check and produce a validation error if the default value cannot be coerced to the field type.

---

## 11. Constraints and Indexes

This specification focuses on the structural mapping of Go models to the meta‑schema's `fields` and `schemas`. **Constraints** and **indexes** are **not** automatically generated from Go struct tags.

If your schema requires constraints or indexes (e.g., for validation or storage), you must add them manually to the resulting JSON definition after generation. The generator may optionally support a future extension for specifying constraints via tags, but that is out of scope for this version.

---

## 12. Field Name Casing

Field names are converted to `snake_case` by default if no explicit `name` is provided. This aligns with common JSON conventions and is compatible with the meta‑schema (which does not impose casing rules). Custom names can be set using `anansi:"customName"` – they are used as‑is.

---

## 13. Generated Code Pattern

When `anansi schema codegen golang` processes a schema JSON file, it emits:

1. **Type aliases** for non‑struct schemas (e.g., `type Email = string`)
2. **Enum types** with typed constants
3. **Nested structs** — value types referenced by fields, with `anansi` tags
4. **Root model struct** — embeds `data.DocumentModel`, user fields with `anansi` tags
5. **`New<T>()` constructor** — calls `data.New[T]` for auto‑initialization
6. **Typed collection wrapper** `<T>s` — embeds `base.ModelCollection[T, *T]` with full CRUD delegation

Example generated output for a `Product` schema:

```go
type Product struct {
    data.DocumentModel
    Name  string  `json:"name" anansi:"name"`
    Price float64 `json:"price" anansi:"price"`
    Stock int     `json:"stock" anansi:"stock"`
}

func NewProduct() *Product {
    return data.New(&Product{})
}

type Products struct {
    base.ModelCollection[Product, *Product]
}

func (c *Products) Create(ctx context.Context, model *Product) (*Product, error) {
    return c.ModelCollection.Create(ctx, model)
}
// ... additional CRUD methods
```

---

## 14. Known Limitations

- **Geometry element precision**: `geometry` is only inferred from `[][]float64` / `[][]float32`, which always maps to IEEE‑754 `number` precision for the inner values. `schema_rules.md` (Rule 10) permits `integer` and `decimal` as valid inner numeric types for geometry, but there's no Go‑native slice‑of‑arbitrary‑precision‑decimal type this generator infers from, and no tag directive is defined to override a geometry field's *element* type (as opposed to the field's own type). If a `decimal`‑precision geometry is genuinely needed, add it manually to the generated JSON, consistent with how constraints and indexes are handled in §11.
- **Union runtime exclusivity**: as noted in §8 Pattern C, "exactly one variant populated" is a data‑level invariant the generator cannot enforce at the Go type level — only the container's shape (all‑pointer, ≥2 variants) is checked at generation time.

---

## Summary of Key Rules for Implementation

1. **Schema Discovery**: Recursively traverse all reachable types, depth‑first, in Go field declaration order; stop at primitives (except §4.1) and already‑visited types.
2. **Deduplication**: Maintain a single global registry keyed by fully qualified type name; each type appears exactly once in `schemas`, including synthesized primitive schemas for union variants.
3. **Order‑Preserving UUIDv7**: Timestamp component encodes discovery ordinal (schemas) or per‑schema declaration ordinal (fields); random component comes from a SHA‑256 hash for global uniqueness. This lets consumers recover source order by sorting map keys — no separate `index` property needed. Arrays (`SchemaReferenceArray`) don't need this, since JSON arrays already preserve order.
4. **Required/Nullable**: Follow explicit boolean flags with sensible pointer/non‑pointer defaults.
5. **Composite**: Model via embedded structs in a container struct with *no other fields*; tag with `type=composite`; generate a `composite` field with a Form 2 array of named references, in embed order.
6. **Union**: Model via a struct of ≥2 optional pointers and nothing else; tag with `type=union`; generate a `union` field with a Form 2 array of named references. Primitive variants get a synthesized, globally‑deduplicated Type‑mode schema (§4.1) — they are never left inline inside the array.
7. **Anonymous Structs**: Create synthetic named schemas, discovered and ordinal‑assigned like any other type.
8. **Inline Descriptors**: Only for scalar primitives as element/value types, or for inline enums. **Never** for structural types (`array`, `object`, etc.) as the field's own type, nor for struct element types, nor anywhere inside a Form 2 array.
9. **Structural Types**: Always use named references for `object`, `array` of structs, `enum` (if reused), `composite`, `union`, `geometry`.
10. **Default Values**: Must match the field type.
11. **Model‑specific**: Root structs embed `data.DocumentModel` and get `New<T>()` constructors plus typed collection wrappers. Nested structs are plain value types without identity.
12. **Tag namespace**: All schema metadata uses `anansi` tags. The `json` tag is wire format only. Legacy `schema` tags are silently accepted by the binding layer for migration purposes but are not generated.

---

This specification ensures that the generated meta‑schemas are fully compliant with the rules defined in `schema_rules.md`, are byte‑reproducible across builds, and preserve Go's natural field ordering without requiring any additional ordering metadata in the meta‑schema itself.
