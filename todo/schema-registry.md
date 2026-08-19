# Migration Plan: Byte-Backed `CompiledSchema` & `bits.Registry[K, T]`

## Overview
This migration plan aligns **Anansi Schema Binary Encoding Spec v1.2** ([`todo/schema_encoding.md`](file:///home/augustine/projects/go-anansi/todo/schema_encoding.md)) with **[`core/bits/registry.go`](file:///home/augustine/projects/go-anansi/core/bits/registry.go)** (`bits.Registry[K, T]`).

Instead of writing custom registry management code, schema registries instantiate `bits.Registry[K, *CompiledSchema]`, storing raw compiled schema payloads as entries inside `bits.Registry` and returning zero-copy `*CompiledSchema` views (`T`) automatically via `NewView`.

---

## 1. Architectural Integration & `bits.Registry` Configuration

### 1.1 `Config[K, *CompiledSchema]` Setup
A schema registry is instantiated as a `bits.Registry[K, *CompiledSchema]`:

```go
type SchemaKey struct {
    ID      uint8
    Version uint8
    Epoch   uint8
}

func (k SchemaKey) Hash64() uint64 {
    return uint64(k.ID) | (uint64(k.Version) << 8) | (uint64(k.Epoch) << 16)
}

type SchemaRegistry = bits.Registry[SchemaKey, *definition.CompiledSchema]

func NewSchemaRegistry(spec bits.HandleSpec) *SchemaRegistry {
    return bits.NewRegistry(bits.Config[SchemaKey, *definition.CompiledSchema]{
        HandleSpec: spec,
        HashKey: func(k SchemaKey) uint64 {
            return k.Hash64()
        },
        NewView: func(h bits.Handle, data []byte) *definition.CompiledSchema {
            return definition.NewCompiledSchemaFromBytes(data)
        },
    })
}
```

### 1.2 `CompiledSchema` View Creation
When `registry.Get(key)` or `registry.Set(key, payload)` is called:
1. `bits.Registry` locates/allocates the contiguous byte slice for the schema payload in its underlying snapshot buffer.
2. `NewView(handle, slice)` executes `definition.NewCompiledSchemaFromBytes(slice)`.
3. The returned `*CompiledSchema` holds a zero-copy pointer (`data []byte`) directly referencing the bytes stored in `bits.Registry`.

---

## 2. `CompiledSchema` Layout & Memory Model

```go
type CompiledSchema struct {
    // Core slice pointing directly to entry bytes in bits.Registry snapshot
    data []byte

    // Navigation directory cached absolute offsets (bytes 16–143 of header)
    stringTableOffset   uint64
    schemasOffset       uint64
    descriptorsOffset   uint64
    localOffsetsOffset  uint64
    fieldTypesOffset    uint64
    metadataOffset      uint64
    coldTrailerOffset   uint64

    // Dispatch flags (Word 1)
    flags uint8

    // Read-only memoized path/address cache
    addrMu     sync.RWMutex
    addrCache  map[uint64][]addrCacheEntry
    pathByAddr map[uint32]ResolvedPath
    nameByAddr map[uint32]string

    // Cold trailer lazy load
    coldOnce sync.Once
    coldData *coldTrailerData
}
```

---

## 3. Implementation Phases

### Phase 1: Binary Encoding & Decoding (`definition` / `encoding`)
- Implement header packing/unpacking (Word 0 big-endian, Word 1 big-endian, 128-byte Navigation Directory little-endian).
- Implement Section 3 (Global String Table) and Section 8 (Metadata Records — `FieldMeta` & `SchemaMeta` 24-byte layouts).
- Implement Section 9 Cold Trailer packing/unpacking.
- Implement `definition.NewCompiledSchemaFromBytes(data []byte) *CompiledSchema`.

### Phase 2: Refactor `CompiledSchema` Accessors & Zero-Copy Views
- Replace direct slice accesses (`cs.Descriptors[i]`, `cs.FieldsMeta[i]`) with zero-copy accessor methods:
  - `cs.Descriptor(i int) FieldDescriptor`
  - `cs.SchemaSlot(i int) SchemaSlot`
  - `cs.LocalOffset(i int) uint32`
  - `cs.FieldType(i int) FieldType`
  - `cs.FieldMeta(i int) FieldMeta`
  - `cs.SchemaMeta(i int) SchemaMeta`
- Refactor all call sites across `core/document/`, `core/encoding/json/`, `core/data/`, etc.

### Phase 3: `bits.Registry` Integration & Linker
- Update `Link()` to compile schemas directly into the binary layout.
- Use `bits.Registry[K, *CompiledSchema]` for schema registries across the codebase.

### Phase 4: Testing & Verification
- Run tests in development mode:
  ```bash
  ANANSI_ENV=development make test
  ```

---

## 4. Summary of Benefits
- **Zero Duplication**: Leverages `bits.Registry[K, T]` generic indexing, persistent entries, hole management, defragmentation, and copy-on-write snapshots.
- **Zero-Copy Views**: `NewView` instantiates lightweight `*CompiledSchema` views directly wrapping registry byte slices.
- **Spec Compliance**: Fully satisfies **Anansi Schema Binary Encoding Spec v1.2**.
