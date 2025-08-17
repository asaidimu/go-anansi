# Metadata Refactor TODO

## Problem
- Metadata logic is spread across multiple layers.
- Document creation does not guarantee metadata unless explicitly initialized.
- Contracts are not consistently enforced:
  1. Root `_metadata_` must always be stored/fetched (even under projections).
  2. Nested documents must never carry `_metadata_`.

## Goals
- All `Document` instances automatically carry system metadata.
- Users can extend metadata with custom providers + schemas.
- Enforcement of both metadata contracts is automatic.
- Registry/query/persistence layers become metadata-agnostic.
- Keep the factory hidden — public API remains simple.

---

## Plan

### 1. Internal `documentFactory` (singleton)
- Hidden inside `data` package.
- Responsible for:
  - Injecting system metadata (`version`, `created`, `updated`, `hash`).
  - Merging user metadata providers.
  - Normalizing nested documents to strip `_metadata_`.

### 2. Public Constructors
- Keep existing API unchanged:
  ```go
  doc := data.NewDocument(map[string]any{})
  doc := data.MustNewDocument(myAny)
  doc, err := data.FromJSON(bytes)

* Internally, all constructors delegate to `documentFactory`.

### 3. Enforce Metadata Contracts

* **Root metadata always present**
  * Factory injects `_metadata_` automatically.
  * Persistence wrapper forces `_metadata_` into projections on reads.
  * **No nested metadata**
  * Factory calls `Normalize()` on document creation.

### 4. User Metadata Extensions

* Provide a way to register metadata providers + schemas:

  ```go
  data.RegisterMetadataSchema("custom_meta", nestedSchemaDef, providerFn)
  ```
* These are merged into root `_metadata_` during creation.

### 5. Remove Metadata Logic from Higher Layers

* Delete `createEntryMetadata()` from `managedCollection`.
* Replace hard-coded `_metadata_` schema in `registry.RegistrySchema()` with a call to `DocumentFactory.MetadataSchemas()`.
* Drop `registry.EnrichSchema()` from `sqliteFactory.Build()`.

## Outcome

* All `Document` instances are metadata-aware by default.
* Users extend metadata via registration, not ad-hoc enrichment.
* Persistence/query layers don’t know about metadata.
* Public API stays clean and familiar — factory is an internal detail.
