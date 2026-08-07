# Domain modeling with anansi: identify the domain, then drive the schema from it

Read this when you're **designing a collection's schema for a real product
domain** — a payment gateway, a chat app, an analytics pipeline. It pairs with
`references/schema-format.md` (the schema JSON format) and `references/caching.md`
(the pooling machinery). Verified against the framework source.

The guide is two steps:

1. **Identify the domain** the feature requires.
2. **Decide the shape** by running the domain through a decision matrix — each
   answer points to exactly one schema construct.

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

## Step 2 — Decide the best option. Based on what? A six-question matrix.

Each question asks the *domain* (not the schema) something, and the answer maps
to a concrete schema construct.

| # | Question you ask the domain | Answer → modeling move |
| --- | --- | --- |
| **Q1 Row cardinality** | Does the hot path read **one row or thousands**? | one → struct ergonomics, don't micro-tune; thousands → raw pooled docs, tune the columns. |
| **Q2 Presence** | Does **every** row carry all members? | always-all → **composite / root fields**; mutually **exclusive kinds** → **separate collections** or **enum + record**. |
| **Q3 Discriminant** | Is "which kind / which state" the **core invariant**? | yes → a **required `enum`** field — explicit, indexable, validated. |
| **Q4 Value semantics** | Is the field **money, agg-time, display-time, an open set, or a blob?** | money → `int64` minor units / `decimal`; agg-time → `integer` epoch; display-time → `string`; bounded set → `enum`; unbounded → `record`/`string`. |
| **Q5 Hidden columns** | Which fields does the **hot list/scan** project? | what's listed stays **on the schema**; heavy/rarely-read fields live behind a **projection**. |
| **Q6 Null vs absent** | Does the domain **truly** distinguish null from absent/zero? | no → `required` + `default`; yes → `nullable`. |

> **Step-2 takeaway:** decide based on **cardinality, presence-exclusivity,
> the real discriminant, value semantics, hot-path columns, and null-vs-absent.**
> The decision matrix *is* "what to decide, by what."

### Composite vs union — the Q2/Q3 trap (polymorphism is cursed)

This is worth calling out because real domains keep tripping it.

- **`composite`** = "every row has all these parts" (embedded mixins, flattened).
  Structure and columnar-friendly.
- **`union`** = "exactly one of these pointers is set" — which, in Go, is what it
  degrades to (`docs/dto_spec.md` Pattern C), and in anansi it is structurally
  discriminated (no tag; `resolved_schema.go:compileUnionField`).

**Polymorphic values are cursed on the backend.** In Go a "union" isn't a sum
type: the compiler can't prove exhaustiveness, variants share a nullable-pointer
representation, and the discriminator is *shape*, so validation is
order-dependent and ambiguous the moment two variants share a field; querying,
indexing, and migration all have to re-derive "which variant" in SQL. A union
even forces values into the interface (`[]any`) column, losing the columnar win.

**The modeling rule that replaces the union:**

1. **Distinct entities → separate collections**, related by ID. The single
   biggest move.
2. **Kinds of one thing → `required` `enum` discriminant + shaped payload**
   (record / nested schema). Explicit, validated, indexable — a tagged union
   done the boring way.
3. **"Always has all parts" → `composite`**, never a union.
4. **Finite states → `enum`, never stringly tags.**

Only reach for `type=union` when the value is genuinely short-lived and
self-contained (a transient envelope at one boundary) — and even then prefer
enum + record.

---

## Structs vs raw documents — once you've identified the domain

Binding a document to a Go struct is **not a correctness lever**. Type safety
comes from the schema (by construction) and correctness from validation — both
happen on the document whether or not you bind it (`Document` carries
`*definition.CompiledSchema`; `SetX/GetX` dispatch on the embedded `DataType`).
So binding is **a cost decision**: one value-conversion pass plus scattered
allocations per row.

- **Mechanical domain / raw docs.** The hot path is numbers over many rows; keep
  the inner loop on raw pooled documents and bind only at the edges:

  ```go
  pool, _ := coll.DocumentPool(ctx) // schema-compiled, pooled containers
  for rows := coll.ReadScan(ctx, &q); rows.Next(); {
      doc, _ := pool.FromJSON(rows.Raw)
      total := decimal.New(doc.MustGetFloat("amount"))   // columnar stride, no bind
      fee   := decimal.New(doc.MustGetFloat("fee"))
      aggregates[dst] = aggregates[dst].Add(total.Add(fee))
      pool.Release(doc)
  }
  ```

- **Ergonomic domain / structs**: strings, single-row, API/socket edges — bind to
  generated structs via `ModelCollection`; the cache absorbs the per-row cost
  (`FindByID` hits the id-cache before touching documents/DB).

---

## Container-aware heuristics (the "columnar common sense")

Your schema decides the backing array: `Container.data[DataType]` holds concrete
typed slices (`[]int64`, `[]float64`, `[]string`) and falls back to `[]any` for
`TypeUnknown`. So the community of common-sense rules is literally about columns:

- **Real types → real columns.** Give hot fields concrete types so they land in a
  typed slice; free-form `record`/untyped values fall to the boxed `[]any`
  column. Reserve free-form for genuinely unbounded payloads.
- **Absence is free; null and `[]any` are not.** The container is sparse — only
  *set* fields allocate. Wide-with-optional costs nothing per row (don't narrow
  tables out of relational instinct). `nullable` costs a sparse-map entry — use
  `required` + `default` unless the null is real (Q6).
- **Every field access is a map hop, then contiguous.** `Get/Set` resolves through
  `positions` hashing then strides the typed slice. Batch a row's reads into one
  pass; after the lookup the values are a cache-resident contiguous walk.
- **Composite flattens into the same columns; nested objects don't.** A composite
  member shares the parent's typed slices (zero extra container); a nested object
  is its own `DataContainer` allocation. Flatten what is always-present and hot.
- **Projections are your narrow-column lever.** `ReadAs[Proj]` materializes only
  the projected fields (container only allocates for what you read). Put
  big/rarely-read payloads behind a projection off the hot list path.
- **Money: `integer` minor units or `decimal`, never float.** Correctness *and*
  the numeric column.
- **Times: integer epoch if you aggregate, string if you only display.**
- **A type change is an identity change.** The 64-bit key embeds the `DataType`;
  treat a type change across schema versions as destroy-and-rebuild, not rename.
- **`_id_` survives projections; it is the join key.** Keep natural keys as
  `unique` fields and reference by `_id_`.

---

## Worked: payment gateway (mechanical domain) through the matrix

Hot path: many rows, numeric aggregation, settlement sweeps.

| Q | Q2 answer | Q3 | Q5 | Move |
| --- | --- | --- | --- | --- |
| payment `status` | kinds (not all-at-once) | **yes — it's the state machine** | — | **required `enum`** |
| `amount` | — | — | — | `integer` (minor units), flat |
| `settled_at` | — | — | — | `integer` epoch (aggregate) |
| `BillingAddress` | always present | — | — | **composite** (flattened) |
| webhook payload | — | — | rarely-read / for send | **projection** |

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
  "metadata": {
    "projections": {
      "PaymentWebhook": { "fields": { "include": ["order_id","status","amount_cents"] } }
    }
  }
}
```

## Worked: chat (ergonomic domain) through the same matrix

Hot path: many rows, but *strings*; the ergonomics rule softens the raw-doc rule.
`conversation_id` is a **filter, not a shape** → no union, just a field. `content`
is a big string → **`MessageList` projection excludes it** off the hot path.

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
  "metadata": {
    "projections": {
      "MessageList": { "fields": { "exclude": ["content"] } }
    }
  }
}
```

---

## Cross-links

- `references/schema-format.md` — schema JSON format, field types, projections,
  `WithSchema` composition.
- `references/collection-internals.md` — DocumentPool, FromJSON/FromStruct,
  Release.
- `references/caching.md` — the ModelCollection fast path and cache.
- `references/query-dsl.md` — shaping/filtering collections.