# Sanitization: setup and usage

Sanitization applies **field-masking policies at the boundary** — before a
document reaches a log, an event payload, or a client response. The persisted
document keeps the real value; you ship a **masked copy**. It lives in
`core/sanitize` and is wired through a process-wide **registry** — it is **not**
configured through the `data` document factory (the old `GlobalSanitizer` on
`data.DocumentFactoryConfig` is gone).

> Which trigger phrases should route here? "mask fields", "redact PII",
> "sanitize before logging", "hide passwords/tokens/cards in responses",
> "field-masking policies", "scoped sanitization".

---

## 1. Concepts

**`sanitize.FieldMaskConfig`** (core/sanitize/policy.go) declares the rules for
one scope:

- `Fields map[string]MaskedFieldPolicy` — mask by **exact field name** (or
  dotted path).
- `Patterns []PatternRule` — mask by **regex** on field name/path; later entries
  are checked in order.
- `DefaultPolicy` — used when nothing else matches (defaults to `preserve`).
- `ObscureConfig` — prefix/suffix/`Replacement`/optional `MaxLength` for obscure.
- `HashSecret` — hex HMAC key (>=16 bytes) for hash; see §5.
- `Scope`, `Description` — identification.

**Policies** (`sanitize.MaskedFieldPolicy`):

| Policy | Result |
| --- | --- |
| `MaskRedact` | `***` |
| `MaskHash` | `[HASH:<hex8>]` — HMAC-SHA256, per-sanitizer secret |
| `MaskObscure` | first/last N chars visible, middle masked (default `**...**`) |
| `MaskPreserve` | unchanged |

**Resolution order** — exact `Fields` match first, then `Patterns` in order,
then `DefaultPolicy`.

**Registry** — a `*sanitize.SanitizationRegistry` holds a `Global` policy plus
named scoped policies. `sanitize.Registry()` returns the process-wide default,
created lazily. Registered scopes are **merged with the global policy** at
sanitizer-build time.

---

## 2. Setup (one-time, at startup)

### Programmatic (recommended)

`sanitize.Configure` applies global + scoped policies to the default registry,
replacing anything previously configured. It validates every config, and on
error the previous configuration stays intact.

```go
if err := sanitize.Configure(sanitize.Config{
    Global: sanitize.NewSecureDefaultConfig(), // pre-masks password, token, api_key, email, card...
    Scoped: map[string]*sanitize.FieldMaskConfig{
        "Admin": {
            Fields: map[string]sanitize.MaskedFieldPolicy{
                "salary":   sanitize.MaskRedact,
                "email":    sanitize.MaskObscure,
            },
            DefaultPolicy: sanitize.MaskPreserve,
        },
    },
}, logger); err != nil {
    log.Fatalf("configure sanitization: %v", err)
}
```

Call it **before** creating/sanitizing documents (once, guarded like any other
startup singleton).

### Dev shortcut

```go
p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
    DBPath:              ":memory:",
    EnableSanitization:  true,
    CustomSanitizerConfig: &sanitize.FieldMaskConfig{ ... },
})
```

`Playground` calls `sanitize.Configure` internally when `EnableSanitization` is
on (using `NewSecureDefaultConfig()` unless `CustomSanitizerConfig` is set).

### Persistent policy store (optional)

Register the registry with a store and hydrate policies at boot — the example
service wires a `SanitizationPolicyStore` (a persistence collection) this way:

```go
reg := sanitize.Registry()
reg.SetPersistence(store)                 // implements sanitize.SanitizationPersistence
if err := reg.LoadFromPersistence(ctx); err != nil {
    return nil, nil, err
}
```

---

## 3. Scoping (which policy applies)

The registry resolves policies **from the request context**. Collections are
the default scopes, so putting the collection on the context is sufficient:

```go
sanitizationCtx := common.ContextWithCollectionName(ctx, collectionName)
safe, err := doc.Sanitize(sanitizationCtx)
```

Or use arbitrary explicit scopes:

```go
ctx = common.ContextWithSanitizationScope(ctx, "admin")
safe, err := doc.Sanitize(ctx)
```

- **Multiple scopes compose**: pass extra contexts to `Sanitize(ctx, otherCtx)`;
  policies merge per field and the **most restrictive wins**
  (`redact` > `hash` > `obscure` > `preserve`).
- **No scope** → global policy applies.
- **No global configured** → `Sanitize` returns an unchanged copy (no error);
  the registry is a no-op until configured.

---

## 4. Using it (at the boundary)

Sanitization is **only as good as what you emit** — always log/ship the masked
copy, never the raw `` `ToMap()` ``:

```go
safe, err := doc.Sanitize(ctx)
if err != nil {
    logger.Error("sanitization failed", zap.Error(err))
}
logger.Info("order", zap.Any("order", safe.ToMap()))     // masked
logger.Info("order", zap.Any("order", doc.ToMap()))      // DON'T — raw secrets
```

Batch reads return record views; sanitize the whole set at once:

```go
docs, err := result.Data.Sanitize(r.Context())   // masks every row
res := s.Response.WriteJSON(w, http.StatusOK, map[string]any{
    "documents": docs.ToMaps(),
}, r)
```

Log-string variant: `doc.SafeString(ctx)`.

These run identically on both pipelines — generated `document.Document` models
and legacy `data.Document` — and on the record views persistence returns.

**Performance note:** container-backed documents (`document`/pool pipeline) are
masked in the **typed container**: the document is cloned once and the walk
rewrites only policy-matched string leaves in place — no `map[string]any` is
materialized and no container is rebuilt. Record views (the persistence egress
shape) are already maps and are sanitized as such. Policy-masking into a
non-string slot (int/bool/...) returns an error, matching the map pipeline's
behavior.

---

## 5. Runtime policy management

The registry is mutable at runtime — the example service exposes `/api/v1/
sanitization/*` endpoints over it:

- `reg.SetGlobal(cfg)` / `reg.Register(scope, cfg)` / `reg.Unregister(scope)`
- `reg.List()`, `reg.Has(scope)`, `reg.HasGlobal()`, `reg.Count()`
- `reg.Export()` / `reg.Import(policies)` — bulk config transfer
- `reg.OnUpdate(func(scopeID string))` — hook to react to policy changes
- Tests: `sanitize.ResetForTesting()` clears the default registry.

`Register`/`SetGlobal` validate the config and persist through the policy store
when one is attached (`r.persistence.Save`).

---

## 6. Rules of thumb

- **Log the sanitized copy.** The whole point is that raw secrets never reach
  observability or the wire. Prefer `SafeString(ctx)` / `Sanitize(ctx).ToMap()`
  everywhere.
- **Start with the secure default**, then relax or sharpen with scoped configs
  rather than hand-building a global `Fields` map.
- **`MaskHash` is secret-scoped.** Without a configured `HashSecret`, each
  sanitizer draws a random key at construction, so hashes are stable within one
  sanitizer but **not reproducible across processes/restarts**. Set
  `HashSecret` (hex, >=16 bytes) when you need cross-process correlation.
- **`MaskObscure` with `MaxLength`** normalizes output width so values of
  different lengths don't leak their length.
- **Reserved metadata** (`created`, `updated`, `version`, `checksum`,
  `signature`) is never masked, so integrity fields survive sanitization.