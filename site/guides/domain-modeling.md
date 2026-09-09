---
title: Domain Modeling
description: "How to design a collection schema for a domain — cardinality, presence-exclusivity, discriminant, value semantics, hot columns, null-vs-absent."
---
Read this when you're **designing a collection's schema for a real product
domain** — a payment gateway, a chat app, an analytics pipeline — or need to
**work correctly against the persistence layer** once that schema exists. It
pairs with `/reference/schema-format` (the schema JSON format) and
`/reference/collection-internals`/`/guides/caching` (the
pool/cache machinery). Verified against the `go-anansi` v8 source
(`core/schema/definition`, `core/data/container`, `core/document`,
`core/persistence/collection`) — every code sample below is checked against
real method signatures, not illustrative pseudocode.

The guide is three parts:

1. **Identify the domain** the feature requires.
2. **Decide the shape** by running the domain through a seven-question
   decision matrix — each answer points to exactly one schema construct.
3. **Use the persistence layer correctly** once the schema exists — binding
   cost, pooling/`Release`, and the three-state field model.

---

## Step 1 — Identify the domain

Ask: **what does this feature's hot path actually do with the data?**

| Domain | Signature | Examples | Pressure |
| --- | --- | --- | --- |
| **Mechanical** | numbers over many rows, read-then-discard | fee/currency math, settlement sweeps, batch reconciliation, metrics | throughput, columnar |
| **Ergonomic** | strings / object-shaped rows, low cardinality | chat messages, profiles, presence, CMS content | developer ergonomics |
| **Structural** | append-mostly, kind-tagged, time-ordered | telemetry, inbox, webhooks, audit trail | ordering, tags, narrow reads |

Everything after this follows that identification.

---

## Step 2 — Decide the best option. Based on what? A seven-question matrix.

Each question asks the *domain* (not the schema) something, and the answer maps
to a concrete schema construct.

| # | Question you ask the domain | Answer → modeling move |
| --- | --- | --- |
| **Q1 Row cardinality** | Does the hot path read **one row or thousands**? | one → bind to a struct/`ReadAs` projection, don't micro-tune; thousands → range over `Read`'s raw `Documenter`s and `Get`/`Release` per row, no binding. |
| **Q2 Presence** | Does **every** row carry all members? | always-all, one named sub-shape → **`object`**; always-all, merging 2+ named fragments → **`composite`**; mutually **exclusive kinds** → **separate collections** or **enum + record**. |
| **Q3 Discriminant** | Is "which kind / which state" the **core invariant**? | yes → a **required `enum`** field — explicit, indexable, validated. |
| **Q4 Value semantics** | Is the field **money, agg-time, display-time, an open set, spatial, or binary**? | money → `integer` minor units / `decimal`; agg-time → `integer` epoch; display-time → `string`; bounded set → `enum`; unbounded → `record`/`string`; spatial → `geometry`; blob/hash/encoded payload → `bytes`. |
| **Q5 Hidden columns** | Which fields does the **hot list/scan** project? | what's listed stays **on the schema**; heavy/rarely-read fields live behind a **projection**. |
| **Q6 Null vs absent** | Does the domain **truly** distinguish null from absent/zero? | no → `required` + `default`; yes → `nullable`. |
| **Q7 Access pattern** | Does the hot path **filter, sort, or join** on this field, and must it stay unique? | yes, equality/sort → an **index** (`type: normal`); yes, uniqueness beyond a single field → a **compound `unique` index**; a cross-field invariant that isn't a lookup → a **constraint**. |

> **Step-2 takeaway:** decide based on **cardinality, presence-exclusivity,
> the real discriminant, value semantics, hot-path columns, null-vs-absent,
> and what the hot path actually queries by.**
> The decision matrix *is* "what to decide, by what."

### Composite vs union vs object — the Q2/Q3 trap (polymorphism is cursed)

This is worth calling out because real domains keep tripping it, and because
`object` and `composite` are easy to conflate.

- **`object`** = "this field is **one** reusable named sub-schema" (e.g. a
  `BillingAddress` embedded once). It flattens into the parent's key space at
  link time — `classifyField` in `core/schema/definition/link.go` returns
  `(TypeUnknown, KindObject, false)` for it, meaning the field itself carries
  no value slot; its children get their own keys in the *same* container. Zero
  extra allocation for a single embed.
- **`composite`** = "this field is **two or more** named schema fragments
  merged into one flattened shape" — a mixin, not a single reference. The
  library's own meta-schema does exactly this: `Constraint` is declared
  `"type": "composite", "schema": [{"id": "<BaseFields>"}, {"id":
  "<ConstraintUnion>"}]`, merging a `name`/`description` base with a
  discriminated-union payload into one field. If you only have one sub-shape,
  you want `object`, not `composite` — `composite` is for combining several.
- **`union`** = "exactly one of these pointers is set" — which, in Go, is what
  it degrades to (`docs/dto_spec.md` Pattern C), and in anansi it is
  structurally discriminated (no tag; `resolved_schema.go` /
  `link.go:classifyField`, `rf.Union != nil` branch).

**Polymorphic values are cursed on the backend.** In Go a "union" isn't a sum
type: the compiler can't prove exhaustiveness, variants share a nullable-pointer
representation, and the discriminator is *shape*, so validation is
order-dependent and ambiguous the moment two variants share a field; querying,
indexing, and migration all have to re-derive "which variant" in SQL. Concretely,
`classifyField` maps a `union` field to `container.TypeUnknown` — the scalar
`any` channel (the same one `unknown` uses) — not a dedicated column, so a
union field gets no typed storage of its own; only its flattened variant
children do. Note this is the scalar `any` slot, not the array-of-unknown
(`[]any`/`TypeArrayUnknown`) slot — that one is reserved for actual array
fields whose element type couldn't be resolved.

**The modeling rule that replaces the union:**

1. **Distinct entities → separate collections**, related by ID. The single
   biggest move.
2. **Kinds of one thing → `required` `enum` discriminant + shaped payload**
   (record / nested schema). Explicit, validated, indexable — a tagged union
   done the boring way.
3. **One always-present sub-shape → `object`; several merged fragments →
   `composite`.** Never a union for either case.
4. **Finite states → `enum`, never stringly tags.**

Only reach for `type: union` when the value is genuinely short-lived and
self-contained (a transient envelope at one boundary) — and even then prefer
enum + record. Telling evidence for how rare it should be: across this
library's own example apps and integration-test schemas, `union` and
`composite` appear **only** inside anansi's own meta-schema (describing
its own JSON format) — no application-level example schema uses either.

---

## Q7 in practice: indexes and constraints

Indexes and constraints are real, first-class parts of the schema format
(`FieldDefinition`'s sibling top-level `indexes`/`constraints` maps in
`core/schema/meta/schema.json`) but are easy to miss because they're
optional and often skipped in quick examples. They're a modeling decision,
not an afterthought — put them in the schema the same way you put `enum` or
`required` there.

**Indexes** live in a top-level `indexes` map, sibling to `fields`, keyed by
ID exactly like fields. Verified shape (from a real schema,
`example/benchmark/schemas/order.schema.json`):

```json
"indexes": {
  "019fc802-2195-7e1c-b348-891d65d35f00": {
    "name": "idx_status",
    "fields": ["status"],
    "type": "normal"
  }
}
```

- `fields` is an array — list 2+ fields for a compound index.
- `type` is one of `normal` / `unique` / `primary` / `spatial` / `fulltext`
  (`IndexDefinition` in `core/schema/meta/schema.json`). Use `unique` for a
  compound uniqueness rule that a single field's `unique: true` can't express;
  use `spatial` for a `geometry` field you'll query by containment/proximity.
- A single field's own uniqueness is simpler to express directly on the field
  (`"unique": true`, as `_id_` does in every schema) — reach for a top-level
  index when you need compound fields, a non-default order, or a
  spatial/fulltext type.

**Constraints** live in a top-level `constraints` map with the same
key-by-ID shape. Verified shape (from
`core/schema/definition/constraints_test.go`):

```json
"constraints": {
  "019fc802-...": {
    "name": "valid_date_range",
    "description": "end_date must be after start_date",
    "fields": ["start_date", "end_date"],
    "predicate": "is_valid"
  }
}
```

Be accurate about what this buys you: a constraint's `predicate` is a name
your application registers a validation function against
(`definition.Predicate func(PredicateParams) []common.Issue`) — it is **not**
a fixed built-in vocabulary like SQL `CHECK`. If you reach for a constraint,
you also own writing and registering the predicate it names; there's no
`"predicate": "gt"` you get for free. For a same-collection cross-field
invariant you're not ready to wire a predicate for, a decorator-level
`Validate` hook (`/guides/decorators`) is often the pragmatic default
until you do.

---

## Structs vs raw documents — once you've identified the domain

Binding a document to a Go struct is **not a correctness lever**. Type safety
comes from the schema (by construction) and correctness from validation —
both happen on the document whether or not you bind it. So binding is **a
cost decision**: `BindToWithContext`/reflection-driven struct population plus
its own allocations, per row.

- **Mechanical domain / raw docs.** The hot path is numbers over many rows.
  `Document.Get`/`MustGet`/`GetOr` are the only accessors on
  `*document.Document` — there is **no** `MustGetFloat`/`MustGetInt`-style
  typed accessor; every read comes back as `any` and you type-assert once.
  Skip the projection/bind step and range over the collection's own
  `Read` result, releasing each document back to the pool as you go:

  ```go
  q := query.NewQueryBuilder().
      From("Payments").
      Where("status").Eq("captured").
      Build()

  result, err := coll.Read(ctx, &q)
  if err != nil {
      return decimal.Zero, err
  }

  total := decimal.Zero
  for _, doc := range result.Data {
      amountCents, err := doc.Get("amount_cents") // integer minor units -> int64
      if err != nil {
          doc.Release()
          return decimal.Zero, err
      }
      total = total.Add(decimal.NewFromInt(amountCents.(int64)))
      doc.Release() // returns the pooled container; do this once you're done with the row
  }
  return total, nil
  ```

  If the field were `type: decimal` instead of `integer` minor units, `Get`
  returns the **canonical decimal string** (`scalarDataType` maps
  `FieldTypeDecimal` to `container.TypeString`) — never a `float64`. Read it
  with `decimal.NewFromString(doc.MustGet("amount").(string))`, not
  `doc.MustGetFloat(...)` (that method doesn't exist, and widening a decimal
  to `float64` is exactly the precision loss the library's `decimal` type
  exists to prevent).

- **Ergonomic domain / structs**: strings, single-row, API/socket edges — bind
  to generated structs via `ModelCollection[P]`, or to a narrower shape via
  `ReadAs[R]`. `FindByID` hits the id-cache before touching documents/DB when
  the collection was built with a cache; `ReadAs`/`CreateFrom`/`UpdateFrom`
  bind each result into `R` and call `doc.Release()` for you afterward — you
  don't manage pooling yourself on this path.

---

## Container-aware heuristics (the "columnar common sense")

Your schema decides the backing array: `DataContainer`'s typed slices
(`[]int64`, `[]float64`, `[]string`, ...) are picked per `DataType`, with
`TypeUnknown` (`any`) as the fallback for `unknown` and `union` fields, and a
**separate** `TypeRecord` (`map[string]any`) slot dedicated to `record`
fields. So the community of common-sense rules is literally about columns:

- **Real types → real columns.** Give hot fields concrete types so they land
  in a typed slice (`integer`→`[]int64`, `number`→`[]float64`,
  `decimal`/`string`→`[]string`, `bytes`→`[][]byte`, `geometry`→
  `[][]float64`). `record` gets its own boxed `map[string]any` slot — distinct
  from, not merged with, the scalar `any` slot that `unknown`/`union` share.
  Reserve `record` for genuinely unbounded/open payloads.
- **Absence is free; null and boxed slots are not.** The container is sparse —
  only *set* fields allocate (`claimHole`/`positions` in
  `core/data/container/data_container.go`). Wide-with-optional costs nothing
  per row (don't narrow tables out of relational instinct). `nullable` costs a
  `positions` map entry — use `required` + `default` unless the null is real
  (Q6). At the API level this is directly visible on `Document.Get`: a null
  field returns `(nil, nil)`; an absent field returns `(nil, err)` — Q6 is the
  difference between those two return shapes for every caller of `Get`.
- **Every field access is a map hop, then contiguous.** `Get`/`Set` resolve
  through `positions` hashing then stride the typed slice. Batch a row's reads
  into one pass; after the lookup, values are a cache-resident contiguous walk.
- **A single `object` and a `composite` both flatten into the parent's key
  space — zero extra container either way.** The allocation cost that
  actually differs is **array of objects vs everything else**: `type: array`
  with an object item schema gets its own `*DataContainer` **per element**
  (`TypeArrayObject`); a lone `object`/`composite` field does not. Flatten
  what's always-present and singular; reserve arrays-of-objects for genuinely
  repeating structures.
- **Projections are your narrow-column lever.** A projection (`metadata.
  projections`, `ReadAs[Proj]`) materializes only the projected fields. Put
  big/rarely-read payloads behind a projection off the hot list path.
- **Money: `integer` minor units or `decimal`, never float.** `decimal` is
  stored as a canonical string specifically so it's never widened to
  `float64` — correctness *and* the string column, not a numeric one.
- **Times: integer epoch if you aggregate, string if you only display.**
- **A type change is an identity change.** Treat a type change across schema
  versions as destroy-and-rebuild, not rename.
- **`_id_` survives projections; it is the join key.** Keep natural keys as
  `unique` fields (or a top-level index) and reference by `_id_`.

---

## Worked: payment gateway (mechanical domain) through the matrix

Hot path: many rows, numeric aggregation, settlement sweeps.

| Q | Answer | Move |
| --- | --- | --- |
| payment `status` | kinds (not all-at-once); **the core state machine** (Q3) | **required `enum`** |
| `amount_cents` | money (Q4) | `integer` (minor units), flat |
| `settled_at` | aggregate time (Q4) | `integer` epoch |
| `billing` | always present, **one** named sub-shape (Q2) | **`object`** (flattens, zero extra container) |
| webhook payload | rarely-read, sent not scanned (Q5) | **projection** |
| lookups by merchant (Q7) | hot-path filter, not unique | **index** on `merchant_id` |

```json
{
  "version": "1.0.0",
  "name": "Payments",
  "fields": {
    "019fb22d-0000-7000-0000-000000000001": { "name": "order_id",     "required": true, "type": "string" },
    "019fb22d-0000-7000-0000-000000000002": { "name": "merchant_id",  "required": true, "type": "string" },
    "019fb22d-0000-7000-0000-000000000003": { "name": "amount_cents", "required": true, "type": "integer" },
    "019fb22d-0000-7000-0000-000000000004": { "name": "status",       "required": true, "type": "enum",
      "schema": { "type": "string", "values": ["authorized","captured","succeeded","failed"] } },
    "019fb22d-0000-7000-0000-000000000005": { "name": "settled_at",   "type": "integer" },
    "019fb22d-0000-7000-0000-000000000006": { "name": "billing",      "required": true, "type": "object",
      "schema": { "id": "019fb22d-0000-7000-0000-00000000a1" } }
  },
  "schemas": {
    "019fb22d-0000-7000-0000-00000000a1": {
      "name": "BillingAddress",
      "fields": {
        "019fb22d-0000-7000-0000-00000000b1": { "name": "line1",   "required": true, "type": "string" },
        "019fb22d-0000-7000-0000-00000000b2": { "name": "country", "required": true, "type": "string" }
      }
    }
  },
  "indexes": {
    "019fb22d-0000-7000-0000-00000000c1": { "name": "idx_merchant", "fields": ["merchant_id"], "type": "normal" }
  },
  "metadata": {
    "projections": {
      "PaymentWebhook": { "fields": { "include": ["order_id","status","amount_cents"] } }
    }
  }
}
```

`billing` is `object`, not `composite` — it references exactly one named
sub-schema. Reach for `composite` only if you needed to merge `BillingAddress`
with a second, independent fragment (e.g. a shared `Auditable` mixin) into the
same field.

## Worked: chat (ergonomic domain) through the same matrix

Hot path: many rows, but *strings*; the ergonomics rule softens the raw-doc
rule. `conversation_id` is a **filter, not a shape** → no union, just an
indexed field (Q7). `content` is a big string → **`MessageList` projection
excludes it** off the hot path (Q5).

```json
{
  "version": "1.0.0",
  "name": "Messages",
  "fields": {
    "019fb22d-0000-7000-0000-000000000002": { "name": "conversation_id", "required": true, "type": "string" },
    "019fb22d-0000-7000-0000-000000000003": { "name": "sender_id",       "required": true, "type": "string" },
    "019fb22d-0000-7000-0000-000000000004": { "name": "content",         "required": true, "type": "string" },
    "019fb22d-0000-7000-0000-000000000005": { "name": "sent_at",         "required": true, "type": "string" }
  },
  "indexes": {
    "019fb22d-0000-7000-0000-000000000006": {
      "name": "idx_conversation_recent",
      "fields": ["conversation_id", "sent_at"],
      "type": "normal"
    }
  },
  "metadata": {
    "projections": {
      "MessageList": { "fields": { "exclude": ["content"] } }
    }
  }
}
```

The compound `idx_conversation_recent` index (Q7) is what makes "recent
messages in this conversation" a hot-path-cheap query — a single-field index
on `conversation_id` alone wouldn't help the `sent_at` ordering.

---

## Cross-links

- `/reference/schema-format` — schema JSON format, field types, projections,
  `WithSchema` composition. (As of this writing it doesn't yet detail the
  `indexes`/`constraints` shapes — this file's Q7 section is the source of
  truth for those until that's filled in.)
- `/reference/collection-internals` — the request path layer-by-layer,
  `DocumentPool`/`Read`/`Release`, and a debugging table.
- `/guides/caching` — the `ModelCollection` id-cache fast path and the
  document `Release()`/pooling contract.
- `/reference/query-dsl` — the fluent `QueryBuilder`, shaping/filtering
  collections.