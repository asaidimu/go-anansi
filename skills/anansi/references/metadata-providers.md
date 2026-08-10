# Custom metadata providers (xattrs)

Read this when you're **hardening** documents with custom metadata — tracing
correlation ids, caller identity, audit context — written automatically at
document finalization. Verified against the framework source
(`core/data/factory.go` and the generated `metadata.go`/`providers.go`).

---

## How do I add custom metadata (xattrs)?

Each document automatically carries reserved system metadata: `created`,
`updated`, `version`, `checksum`, `signature` (see the `data.MetadataCreated/
Updated/Version/Checksum/Signature` constants). **Custom** metadata is any other
key. You add it by declaring a **metadata provider** in the declarative
metadata file, then wiring it into the runtime document factory. The pattern is:

> **declare in `metadata.schema.json` → regenerate → implement the generated
> stub → wire it into `data.ConfigureDocumentFactory`.**

### 1. Declare the provider in `metadata.schema.json`

The scaffold ships a starter file with one `trace` provider. Add your own as
another entry under `providers`. Each provider has a `name` (its map key), a
`description`, and `fields` (each with `name` and `type` in the normal field
format):

```json
{
  "name": "_metadata_",
  "providers": {
    "trace": {
      "description": "Tracing correlation ids",
      "fields": {
        "trace_id": { "name": "trace_id", "type": "string" },
        "span_id":  { "name": "span_id",  "type": "string" }
      }
    },
    "requestor": {
      "description": "Caller identity",
      "fields": {
        "user_id": { "name": "user_id", "type": "string" }
      }
    }
  }
}
```

Only keys declared here are written. Any provider that returns a key **not
declared in this file** is dropped during the `populateMetadata` pass — declare
first, fill later.

### 2. Regenerate

Run the usual change workflow so codegen picks up the new provider:

```bash
anansi migrate generate && anansi codegen golang
```

This writes/updates two files next to your migrations:

- **`metadata.go`** (regenerated every run) exposes:
  - `MetadataSchema() *definition.NestedSchema` — the merged `_metadata_` schema
    (base fields + all declared provider fields),
  - `MetadataDependencies() []*definition.NestedSchema` — referenced dependency
    schemas, deduplicated by name,
  - per-provider `<Ident>Schema()` (e.g. `TraceSchema()`, `RequestorSchema()`) —
    `Ident` is the provider name with its first letter capitalized,
  - `MetadataProviderConfigs() []data.MetadataProviderConfig` — every provider
    as a ready-to-wire config (see step 3),
  - `ValidateMetadataSchema() error` — fails if a provider declared in
    `metadata.schema.json` isn't wired into the runtime factory.
- **`providers.go`** (write-once — existing content is never touched) is a stub
  per provider that you implement. Add a new provider to `metadata.schema.json`
  and regenerate: its stub is **appended** automatically, so you only ever
  write the body:

```go
// TraceProvider fills the "trace" metadata fields (span_id, trace_id).
func TraceProvider(ctx context.Context, doc data.Documenter) (map[string]any, error) {
	sc := trace.SpanContextFromContext(ctx)
	return map[string]any{
		"trace_id": sc.TraceID().String(),
		"span_id":  sc.SpanID().String(),
	}, nil
}
```

The signature is always `func(ctx context.Context, doc data.Documenter)
(map[string]any, error)`. `doc` is the document being built, so a provider can
read `_id_`, existing metadata, or the document's own fields. Return `nil, nil`
to contribute nothing. `metadata.go` is regenerated and must not be edited;
`providers.go` keeps your edited bodies verbatim.

### 3. Wire the provider into the document factory

`data.ConfigureDocumentFactory` is a one-time startup call. You don't hand-
assemble the configs — codegen emits a helper for exactly that:

```go
cfg := data.DocumentFactoryConfig{
	Providers: migrations.MetadataProviderConfigs(), // one entry per provider in metadata.schema.json
}
err := data.ConfigureDocumentFactory(cfg, logger)
```

`MetadataProviderConfigs()` builds every `data.MetadataProviderConfig` from the
generated schemas (`<Ident>Schema()`, `MetadataDependencies()`) and the
provider funcs in `providers.go`. Then call `migrations.ValidateMetadataSchema()`
on startup/test to fail fast if a declared provider wasn't wired in.
(Sanitizers are orthogonal — and no longer a factory concern. Prefer
`sanitize.Configure(sanitize.Config{Global: sanitize.NewSecureDefaultConfig()}, logger)`
for production; see `references/data-integrity.md` §3.)

### Contracts

- **Reserved keys are off-limits.** A provider may not set `created`, `updated`,
  `version`, `checksum`, or `signature`; doing so → `ERR_INVALID_METADATA`.
- **Undeclared keys are dropped** during the `populateMetadata` pass, so a
  provider result that names a field not in `metadata.schema.json` is discarded.
- **Single application point.** `data.ApplyMetadataProviders` runs at document
  finalization and is used by both the container-backed (`document`) and
  map-backed (`data`) pipelines — wired providers work for generated
  `ModelCollection` models too.
- **Wiring order.** The factory is a `sync.Once` singleton — configure it once
  at startup, before any document is created.
- **Reading back.** The generated model's typed `Metadata` struct (or the
  document's `_metadata_` map) carries the custom keys after reads.

> The framework's own `DocumentFactoryConfig` is a normal struct you build and
> pass; see `core/data/factory.go` for `ConfigureDocumentFactory` /
> `ApplyMetadataProviders` / `GetMetadataSchema`.
