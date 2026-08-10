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
- Integrity wiring (below) and `data.NewDocumentFactory*`. **Sanitization has
  moved out of `data` into its own `core/sanitize` package** (see §3) — it is no
  longer configured through `data.DocumentFactoryConfig`.

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

## 3. Sanitization: safe logging and responses

Sensitive fields (passwords, tokens, PII) must not escape into logs, metrics,
or client responses. Anansi lets you declare **field-masking policies** applied
when a document is serialized for observability.

> **Events are full-fidelity.** The persistence event bus emits the real
> input/output so consumers can do real work with them — masking is never
> applied at emit time. Sanitization happens at the **rendering edge**: event
> consumers mask copies when they write logs, dashboards, or outbound payloads
> (see "log the masked copy" below).

Sanitization lives in its own **`core/sanitize`** package — it is **not** a
`data` factory concern. `data.DocumentFactoryConfig` only holds metadata
providers now; sanitizers are wired via `sanitize.Configure`.

### Configuration (policies)

`sanitize.FieldMaskConfig` (core/sanitize/policy.go) declares per-field rules:

- `DefaultPolicy` — used when no explicit rule matches a field.
- `Fields map[string]MaskedFieldPolicy` — mask *by exact field name*.
- `Patterns []PatternRule` — mask by regex on field name/path.
- Policies: **`MaskRedact`** (replace with `***`), **`MaskHash`** (replace with
  a short hash keyed by `HashSecret`), **`MaskObscure`** (show first/last few
  chars, `ObscureConfig`), **`MaskPreserve`** (keep as-is).
- `sanitize.NewSecureDefaultConfig()` pre-masks the known-sensitive fields;
  this is what `Playground{EnableSanitization:true}` wires in (customize via
  `CustomSanitizerConfig`).

### Wiring: a process-wide registry

`sanitize.Configure` applies global + scoped policies to the process-wide
default registry (`sanitize.Registry()`, an `*sanitize.SanitizationRegistry`),
replacing anything previously configured:

```go
if err := sanitize.Configure(sanitize.Config{
    Global: sanitize.NewSecureDefaultConfig(),
    Scoped: map[string]*sanitize.FieldMaskConfig{
        "admin": {
            Fields: map[string]sanitize.MaskedFieldPolicy{
                "password":   sanitize.MaskRedact,
                "cardNumber": sanitize.MaskObscure,
            },
        },
    },
}, logger); err != nil {
    log.Fatalf("configure sanitization: %v", err)
}
```

For a private registry use `sanitize.NewSanitizationRegistry(logger)`.

### Scoping and runtime use

- `Setup`/`Playground` no longer accept sanitizer configs (the `GlobalSanitizer`
  field on `data.DocumentFactoryConfig` is gone). Call `sanitize.Configure`
  yourself at startup, before creating documents; `Playground{
  EnableSanitization:true, CustomSanitizerConfig:*sanitize.FieldMaskConfig}`
  remains the convenient dev shortcut.
- `sanitize.Registry()` is the shared process registry and also `Register`s/
  `Unregister`s scopes dynamically; it exposes `List`, `Has`, `HasGlobal`,
  `Export`/`Import`, and `SetPersistence` + `LoadFromPersistence` for a policy
  store (the example service wires a `SanitizationPolicyStore` into it).
- At the call site, `document.Sanitize(ctx)` (or `.Sanitize()` on `data.Document`)
  resolves scope from the context (`common.ContextWithSanitizationScope`) and
  returns a **masked copy** safe to ship to logs, metrics, or a client.
  In event handlers that come off the bus (full-fidelity), sanitize the copy
  you render — the `event.Input.Sanitize(...)` pattern below.

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
- Any field that touches PII/secrets → declare a `sanitize.FieldMaskConfig`,
  `sanitize.Configure(...)` it, and log `Sanitize(ctx)` copies.