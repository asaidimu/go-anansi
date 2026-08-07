# Documents, integrity, and sanitization

This file answers three things users get wrong daily: (1) which document type
to use, (2) what `checksum`/`signature` actually protect, and (3) how to stop
sensitive data leaking into logs and events. All verified against source.

---

## 1. Which document pipeline to use — prefer `document.*`

Anansi ships **two** document implementations. They share the `data.Documenter`
interface, so they interoperate, but they differ in memory model and guarantees.

### `document` — the recommended pipeline (container/pool-backed)

- `document.Document` (core/document/document.go:44) stores data in a
  schema-addressed `container.DataContainer` allocated from a per-schema
  `document.DocumentPool`.
- **Zero-copy fast path**: documents are pooled and `Release()`d; no per-op map
  allocations. This is what generated models and `ModelCollection` use.
- `document.DocumentModel` is the embed you put in a struct model; it registers
  with the data binder so binding/DTO reasoning works identically to `data`.
- Build with `document.New[T](&model)` or `pool.FromStruct(model,
  document.WithContext(ctx))`.

#### Decoding JSON bytes into a pooled `document.Document`

Given a schema and a serialized JSON document, the efficient way to materialize
it is the schema-bound pool — **compile the schema once, decode many**:

```go
pool, err := document.NewDocumentPool(schema)   // Compile + Link once
doc, err  := pool.FromJSON(jsonBytes)           // pooled decode per call
defer pool.Release(doc)                         // return container to the pool
```

`FromJSON` (core/document/pool.go:130) grabs a pooled `DataContainer`, runs the
schema-driven codec `cjson.DecodeJSONInto` (core/encoding/json/decoder.go:41)
straight into it — no `map[string]any` intermediate, no reflection — then fills
`_id_` (payload wins, else generated), metadata defaults, and the checksum.

Alternatives, in order of allocation-friendliness:

- **`DocumentPool.FromJSON`** — recommended; pooled + schema-driven, the
  default fast path.
- **`cjson.DecodeJSONIntoUnsafe(cs, data, doc, pool)`** — skips the
  string-copy allocation that dominates decode memory, in exchange for a
  buffer-lifetime contract: the caller must not mutate/reuse/pool `data` while
  the decoded `Document` (or anything read from it) is alive. Use only on
  paths that own the input for the document's lifetime (decode → serve →
  discard).
- **`RowScan.ScanRow`** (core/document/scanner.go) — when the bytes come from a
  SQLite row, prefer scanning rows straight into pooled containers; this skips
  JSON entirely and is how the `ModelCollection` read path hydrates docs.

Pooled container docs sourced this way must be `Release()`d back to their pool
when done (see `references/caching.md`).

**Use `document.*` for all new models.** It is the pool-backed path the whole
`ModelCollection` fast path (see `references/caching.md`) is built on.

### `data` — the legacy map-backed pipeline (deprecated for models)

- `data.Document` / `data.DocumentModel` (core/data) build from `map[string]any`
  (`data.MustNewDocument(map)`, `data.NewDocumentFromStruct`). Each document
  allocates maps; nothing is pooled.
- Older examples (the v6 `.readme.md`) use this pipeline. It is **deprecated**
  for authoring new models.

### Keep the `data` package for cross-cutting contracts only

These are shared interfaces and helpers, not the map-backed *document* — keep
using them:

- `data.Documenter` — the interface both pipelines implement
- `data.DocumentIDField` (`"_id_"`), `data.MetadataField` (`"_metadata_"`), and
  the `Metadata*` path/field constants
- `data.DocumentSet`, `data.DocumentDiff`, `data.MapDocumentSet`
- Sanitization / integrity wiring (below), and `data.NewDocumentFactory*`

Rule of thumb: if it returns/accepts a **`*Document`** or **embeds a
`DocumentModel`**, use the `document` package. If it's a **type/interface/
constant/helper**, the `data` package is fine.

---

## 2. The integrity envelope: `checksum` and `signature`

Each document's `_metadata_` reserves `created`, `updated`, `version`,
`checksum`, and `signature`. The `data` factory computes the last two (this is
the *one* reason you might still touch `data`); after that the document is
tamper-evident:

- **`Checksum()`** — a SHA-256 over the canonical (deterministically
  ordered) serialized document, with `checksum`/`signature` excluded from the
  hash input (data/factory.go:455-500). Detects any accidental or malicious
  mutation of the payload.
- **`Signature()`** — an RSA-PSS signature of that hash (factory.go:503),
  produced with the factory's private key. `verifySignature` checks it against
  the public key. Detects that the document (and its checksum) really came from
  the trusted signer — not forged.
- Both live on `data.Documenter` (interface.go:`Checksum()`, `Signature()`).

**What this means for you**

- If `Checksum`/`Signature` don't match what you expect, the document was
  altered after it was finalized — treat it as untrustworthy.
- To use signatures you must configure the signing key in the document factory
  (`data.DocumentFactoryConfig` before `data.ConfigureDocumentFactory` runs).
- The `ModelCollection`/`document` container pipeline stores these fields in
  the metadata container but does **not** recompute the RSA envelope on every
  read — verify explicitly when you need authenticity.

---

## 3. Sanitization: safe logging and events

Sensitive fields (passwords, tokens, PII) must not escape into logs or event
payloads. Anansi lets you declare **field-masking policies** applied when a
document is serialized for observability.

### Configuration (policies)

`data.FieldMaskConfig` (core/data/sanitize.go:106) declares per-field rules:

- `DefaultPolicy` — used when no explicit rule matches a field.
- `Fields map[string]MaskedFieldPolicy` — mask *by exact field name*.
- `Patterns []PatternRule` — mask by regex on field name/path.
- Policies: **`MaskRedact`** (replace with `***`), **`MaskHash`** (replace with
  a short hash keyed by `HashSecret`), **`MaskObscure`** (show first/last few
  chars, `ObscureConfig`).
- The pipeline also exposes `NewSecureDefaultConfig()` which pre-masks the
  known-sensitive fields; this is what `Playground{EnableSanitization:true}`
  wires in (customize via `CustomSanitizerConfig`).

```go
// Playground: turn it on for anything touching real-ish data.
p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
    EnableSanitization: true,
    CustomSanitizerConfig: &data.FieldMaskConfig{
        Scope: "app",
        Fields: map[string]data.MaskedFieldPolicy{
            "password": data.MaskRedact,
            "cardNumber": data.MaskObscure,
        },
    },
})
```

### Scoping and runtime use

- `data.DocumentFactoryConfig.GlobalSanitizer` (+ `ScopedSanitizers
  map[scopeID]*FieldMaskConfig`) configure the factory
  (`data.ConfigureDocumentFactory(cfg, logger)`), so `Setup` and `Playground`
  pick them up.
- `data.RegisterScopedSanitizer` / `UnregisterScopedSanitizer` /
  `ListScopedSanitizers` allow a sanitization registry shared across the process
  (the example service uses a `SanitizationPolicyStore` wired to
  `data.GetSanitizationRegistry()`).
- At the call site, `document.Sanitize(ctx)` (or `.Sanitize()` on `data.Document`)
  returns a **masked copy** safe to ship to logs, events, or a client.

```go
// Never log the raw document — log its sanitized form
logger.Info("order",
    zap.Any("order", event.Input.Sanitize(ctx).ToMap()),
)
```

Sanitization is **only as good as what you log.** Always log the sanitized
document/event, never the raw `ToMap()`; that is the whole point of the
`event.Input.Sanitize(...)` pattern you'll see in event callbacks.

---

## Checklist

- New models: **`document.DocumentModel`**; you rarely need `data.*` documents.
- Need tamper/authenticity evidence → configure the factory signing key, read
  `data.Documenter.Checksum()` / `.Signature()`.
- Any field that touches PII/secrets → declare a `FieldMaskConfig`, enable
  sanitization, and log `Sanitize(ctx)` copies.