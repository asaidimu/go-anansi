# Monorepo Decomposition Plan

Date: 2026-09-02

## Overview

go-anansi has matured to the point where its internal components are independently
valuable general-purpose tools. The codebase contains a dynamic typesafe data
container, a performant cache, a structured error system, and a schema-driven
codec — all of which solve problems Go developers face outside of persistence.

This plan decomposes go-anansi into standalone modules that can be imported
independently, while preserving go-anansi as the composition layer that brings
them together.

## Module Map

| New Module | Source | Purpose |
|---|---|---|
| `github.com/asaidimu/go-anansi/pkg/errors` | `core/common/error.go`, `core/common/errors.go` | Structured error system: SystemError, Issues, severity levels, i18n |
| `github.com/asaidimu/go-anansi/pkg/cache` | `core/cache/` | Sharded CLOCK-eviction cache with sensible defaults |
| `github.com/asaidimu/go-anansi/pkg/data` | `core/data/container/` | Dynamic typesafe data container with 27-bit addressing |
| `github.com/asaidimu/go-anansi/pkg/schema` | `core/schema/definition/`, `core/schema/meta/` | Schema definition, validation, compilation, binary serialization |
| `github.com/asaidimu/go-anansi/v8` (rewired) | everything else | Persistence layer, imports the above |

## Dependency Order

```
Phase 1 (parallel — no inter-dependencies):
  pkg/errors    ← zero deps
  pkg/cache     ← zero deps
  pkg/data      ← zero deps

Phase 2 (depends on Phase 1):
  pkg/schema    ← needs pkg/errors (Issue type) + pkg/data (DataContainer in compiled schema)

Phase 3 (rewire):
  go-anansi     ← imports all four; internal packages switch to pkg/* paths
  hestia        ← imports all four; internal packages switch to pkg/* paths
```

---

## Phase 1: `pkg/errors`

### Source Files

- `core/common/error.go` (901 lines) — SystemError, Issue, Issues, constructors, sentinels
- `core/common/errors.go` (10 lines) — logical operator error sentinels

### Export Surface

**Types:**
- `SystemError` struct — 9 exported fields, 17 methods (Error, Unwrap, WithPath, WithOperation, WithCause, WithIssue, WithIssues, WithMessage, WithMessagef, WithCode, WithSeverity, WithRetryable, IsRetryable, Translate, Sanitize, ToIssue, IsError/IsWarning/IsInfo, HasErrors, ContainsCode)
- `Issue` struct — 7 exported fields, 7 methods (NormalizedPath, String, IsError/IsWarning/IsInfo, WithMessagef, Translate)
- `Issues` type (`[]Issue`) — 6 methods (String, NormalizePaths, HasErrors, FilterBySeverity, ContainsCode, Translate)
- `TranslationCatalog` interface — `Get(locale, code string) string`
- `TransformFunc` type — `func(string) string`

**Constructors:**
- `NewSystemError(code string, message ...string) *SystemError`
- `SystemErrorFrom(err error, code ...string) *SystemError`

**Constants:**
- `SeverityError`, `SeverityWarning`, `SeverityInfo`

**Sentinel Errors:**
- `ErrInputCannotBeNil`, `ErrInputCannotBeNilPointer`, `ErrNoMetadata`, `ErrMetadataKeyNotFound`, `ErrFailedToCalculateHash`, `ErrHashMismatch`
- `ErrEmptConditions`, `ErrInvalidOperator`, `ErrInvalidNotOperand`, `ErrUnmarshalFailed`, `ErrMarshalFailed`

### What Stays in `core/common/`

After extraction, `core/common/` retains only non-error utilities:
- `DocumentIDField`, `MetadataField`, `MetadataChecksum`, etc. (field name constants)
- `Version` type (if not moved to `pkg/schema`)
- Any remaining helper functions

### Migration Steps

- [ ] Create `pkg/errors/` directory
- [ ] Move `core/common/error.go` → `pkg/errors/system_error.go`
- [ ] Move `core/common/errors.go` → `pkg/errors/sentinels.go`
- [ ] Create `pkg/errors/go.mod` with module path `github.com/asaidimu/go-anansi/pkg/errors`
- [ ] Add any remaining constants/types from `core/common/` that are needed by the error types
- [ ] Run `go build ./...` and `go test ./...` in `pkg/errors/`
- [ ] Update `go-anansi/core/common/` to re-export from `pkg/errors` (type aliases for backward compatibility)
- [ ] Verify all existing tests pass with re-exports

### Affected Consumers

- **go-anansi:** 30+ packages import `common.SystemError`, `common.NewSystemError`, `common.SystemErrorFrom`, `common.Issue`, `common.Issues`
- **hestia:** 20+ packages import the same

### Backward Compatibility Strategy

Use Go type aliases in `core/common/` to maintain backward compatibility:
```go
type SystemError = errors.SystemError
var NewSystemError = errors.NewSystemError
var SystemErrorFrom = errors.SystemErrorFrom
type Issue = errors.Issue
type Issues = errors.Issues
```

This means existing code compiles without changes. Over time, consumers can migrate
to direct `pkg/errors` imports.

---

## Phase 1: `pkg/cache`

### Source Files

- `core/cache/cache.go` — `managedCache[T]`, `RepositoryCache[T]` interface, `CacheStatus`, `CacheStats`
- `core/cache/config.go` — `CacheConfig`, `DefaultCacheConfig()`
- `core/cache/simple_cache.go` — `inMemoryCache[T]` (unbounded variant)
- `core/cache/cache_test.go`
- `core/cache/cache_bench_test.go`

### Export Surface

**Interface:**
- `RepositoryCache[T any]` — 14 methods: Get, GetStatus, Set, SetWithTTL, Nullify, NullifyWithTTL, TTL, Persist, Evict, Keys, Clear, Clone, Stats, Close

**Constructors:**
- `NewManagedCache[T any](cfg CacheConfig, cloneFn func(T) (T, error), onEvict ...func(key string, value T)) RepositoryCache[T]`
- `NewInMemoryCache[T any](cloneFn func(T) (T, error)) RepositoryCache[T]` (unbounded, test-only)

**Types:**
- `CacheConfig` struct — 12 fields (MaxEntries, PositiveTTL, NegativeTTL, JanitorInterval, JanitorBatchSize, ShardCount, MaxKeyLength, CompactionThreshold, EvictionHighWatermark, EvictionLowWatermark, EvictionInterval, EvictionBatchSize, Logger)
- `CacheStatus` int — `CacheMiss`, `CacheHitPositive`, `CacheHitNegative`
- `CacheStats` struct — 12 fields (Size, PositiveCount, NegativeCount, Hits, Misses, NegativeHits, Evictions, HardCapEvictions, WatermarkEvictions, Expirations, Compactions, EvictorActive)

**Functions:**
- `DefaultCacheConfig() CacheConfig` — 10k entries, 30min positive TTL, 1min negative TTL, 16 shards

**Constants:**
- `DefaultTTL` (0) — use cache-configured default
- `NoExpiration` (-1) — never expire

### Internal Types (not exported)

- `managedCache[T]` — sharded CLOCK-eviction implementation
- `cacheShard[T]` — single independently-locked partition
- `cacheItem[T]` — per-entry value in a `list.Element`
- `inMemoryCache[T]` — simple unbounded implementation

### External Dependencies

**None.** Only stdlib: `container/list`, `context`, `fmt`, `hash/maphash`, `log/slog`, `sync`, `sync/atomic`, `time`.

### Migration Steps

- [ ] Create `pkg/cache/` directory
- [ ] Move `core/cache/cache.go` → `pkg/cache/cache.go`
- [ ] Move `core/cache/config.go` → `pkg/cache/config.go`
- [ ] Move `core/cache/simple_cache.go` → `pkg/cache/simple_cache.go`
- [ ] Move tests and benchmarks
- [ ] Create `pkg/cache/go.mod` with module path `github.com/asaidimu/go-anansi/pkg/cache`
- [ ] Rename `NewManagedCache` → `NewCache` (keep `NewManagedCache` as deprecated alias)
- [ ] Run `go build ./...` and `go test ./...`
- [ ] Update `go-anansi/core/cache/` to re-export from `pkg/cache`
- [ ] Verify all existing tests pass

### Affected Consumers

- **go-anansi:** `core/persistence/collection/`, `core/persistence/registry/`, `core/bits/`
- **hestia:** `core/runtime/dispatch/input.go`

### Backward Compatibility

```go
type RepositoryCache[T any] = cache.RepositoryCache[T]
type CacheConfig = cache.CacheConfig
var DefaultCacheConfig = cache.DefaultCacheConfig
var NewManagedCache = cache.NewCache // deprecated alias
```

---

## Phase 1: `pkg/data`

### Source Files

- `core/data/container/data_point.go` — `DataPoint`, `DataType`, address encoding
- `core/data/container/data_container.go` — `DataContainerKey`, `DataContainer`, typed accessors
- `core/data/container/pool.go` — `Pool` (sync.Pool wrapper)
- `core/data/container/collection.go` — `Collection` (slice of containers)

### Export Surface

**Address Types:**
- `DataPoint` (int32) — 32-bit descriptor: null flag (1 bit) + DataType (4 bits) + unique ID (27 bits)
- `DataContainerKey` (int64) — packed field descriptor (32 bits) + DataPoint (32 bits)
- `DataType` (uint8) — 16 variants: Unknown, Int, Float, String, Bool, Bytes, Geometry, Record, ArrayUnknown, ArrayInt, ArrayFloat, ArrayString, ArrayBool, ArrayBytes, ArrayObject, ArrayGeometry

**Constructors:**
- `NewDataPoint(typ DataType, id ...int32) (DataPoint, error)`
- `NewDataContainerKey(dp DataPoint, descriptor uint32) DataContainerKey`
- `NewDataContainer() *DataContainer`
- `NewPool() *Pool`
- `NewCollection(pool *Pool) *Collection`

**DataContainer Methods:**
- Typed Set/Append/Get for all 16 DataTypes (48 methods total)
- State: SetNull, Unset, IsSet, IsNull, HasValue, Length, Clear
- Backing: AcquireBacking, OwnBacking, Backing
- Walk: iteration over all slots

**Pool Methods:**
- Get, Put, Clone, Acquire, Walk

**Collection Methods:**
- Append, Len, At, Each, Filter, FilterCopy, Project, Reduce, Release

**Errors:**
- `ErrTypeMismatch`, `ErrBucketFull`, `ErrIDOutOfBounds`

### Internal Types (not exported)

- None significant — all main types are exported

### External Dependencies

**None.** Only `fmt`, `sync`, `unsafe`.

### Key Design Property

The 27-bit address scheme provides 134,217,728 unique field addresses — more than
sufficient for any practical OLTP DTO. Any implementation that can reliably produce
unique 27-bit addresses can use the DataContainer as its backing store. This makes
the container a general-purpose data structure, not an anansi-specific one.

### Migration Steps

- [ ] Create `pkg/data/` directory
- [ ] Move `core/data/container/data_point.go` → `pkg/data/data_point.go`
- [ ] Move `core/data/container/data_container.go` → `pkg/data/data_container.go`
- [ ] Move `core/data/container/pool.go` → `pkg/data/pool.go`
- [ ] Move `core/data/container/collection.go` → `pkg/data/collection.go`
- [ ] Move tests
- [ ] Create `pkg/data/go.mod` with module path `github.com/asaidimu/go-anansi/pkg/data`
- [ ] Run `go build ./...` and `go test ./...`
- [ ] Update `go-anansi/core/data/container/` to re-export from `pkg/data`
- [ ] Update `go-anansi/core/schema/definition/compiled.go`, `link.go`, `serialize.go` to import `pkg/data`
- [ ] Verify all existing tests pass

### Affected Consumers

- **go-anansi:** `core/schema/definition/` (compiled.go, link.go, serialize.go) — the only direct consumer
- **hestia:** none directly (uses through schema compilation)

---

## Phase 2: `pkg/schema`

### Source Files

- `core/schema/definition/` (~50 files, ~8000 lines)
- `core/schema/meta/` (meta-schema validation)
- `core/schema/exports.go` (top-level re-exports)

### Export Surface

**Schema Types:**
- `Schema`, `BaseSchema`, `NestedSchema` structs
- `Field`, `FieldName`, `FieldId`, `FieldProperties`
- `FieldType` enum (14 variants): Unknown, Int, Float, String, Bool, Bytes, Geometry, Record, ArrayUnknown, ArrayInt, ArrayFloat, ArrayString, ArrayBool, ArrayBytes, ArrayObject, ArrayGeometry
- `Index`, `IndexType` (5 variants), `IndexConditionUnion`
- `Constraint`, `ConstraintUnion`, `ConstraintKind`, `ConstraintRule`, `ConstraintGroup`
- `SchemaReference`, `FieldSchemaReference`, `ResourceReference`
- `LiteralValue`, `LiteralType`

**Compilation:**
- `Compile(sc *Schema) (*ResolvedSchema, error)`
- `Link(rs *ResolvedSchema) (*CompiledSchema, error)`
- `ResolvedSchema`, `ResolvedNestedSchema`, `ResolvedField`
- `CompiledSchema`, `FieldDescriptor`, `SchemaSlot`, `FieldMeta`, `SchemaMeta`

**Validation:**
- `NewDocumentValidator(schema *Schema, fmap PredicateMap) (*DocumentValidator, error)`
- `NewDocumentValidatorWithConfig(schema *Schema, fmap PredicateMap, config ValidationConfig) (*DocumentValidator, error)`
- `DocumentValidator`, `ValidationGraph`, `ValidationConfig`, `ValidationMode`
- `DefaultValidationConfig() ValidationConfig`

**Diff:**
- `Diff(oldSchema, newSchema *Schema) (*SchemaDiff, error)`
- `VersionImpact(diff *SchemaDiff) VersionBump`
- `SchemaDiff`, `SemanticChange`, `ChangeKind` (16 variants), `Operation`

**Serialization:**
- `SerializeCompiledSchema(cs *CompiledSchema) ([]byte, error)`
- `DeserializeCompiledSchema(data []byte) (*CompiledSchema, error)`

**Factory:**
- `FromJSON(data []byte) (*Schema, error)`

**Walker:**
- `Schema.Walk(...)` with `NodeType` (21 variants) and `NodeContext`

**JSON Builder:**
- `JSONBuilder` — high-performance schema-driven JSON serialization

**Modification:**
- `WithField()`, `WithIndex()`, `WithSchema()` — immutable schema transforms

**Meta:**
- `meta.Validate()`, `meta.Normalize()` — meta-schema validation

### Internal Dependencies to Resolve

| Current Dep | Used For | Replacement |
|---|---|---|
| `common.Issue` | Validation errors | Import `pkg/errors` |
| `common.Version` | Schema versioning | Local `type Version [3]int` |
| `common.LogicalOperator` | Constraint groups | Local enum |
| `common.ComparisonOperator` | Index constraints | Local enum |
| `bits.ResultSet` | Cycle detection in validator | Inline ~50 lines |
| `types/decimal` | Decimal type checking | Local `isDecimal()` guard |
| `utils.GetValueByParts` | Path resolution | Copy function (small) |
| `container.DataContainer` | Compiled schema defaults/enums | Import `pkg/data` |

### External Dependencies After Extraction

- `pkg/errors` (for Issue type)
- `pkg/data` (for DataContainer in compiled schema)
- `github.com/google/uuid` (modification.go only)

### Migration Steps

- [ ] Create `pkg/schema/` directory
- [ ] Resolve `common.Version` → local `type Version [3]int` in `pkg/schema/types.go`
- [ ] Resolve `common.LogicalOperator`, `common.ComparisonOperator` → local enums in `pkg/schema/types.go`
- [ ] Resolve `bits.ResultSet` → inline cycle detection helper in `pkg/schema/validator/cycle.go`
- [ ] Resolve `types/decimal` → local `func isDecimal(t reflect.Type) bool` in `pkg/schema/validator/utils.go`
- [ ] Resolve `utils.GetValueByParts` → copy to `pkg/schema/validator/path.go`
- [ ] Move `core/schema/definition/*.go` → `pkg/schema/` (flattened or subdirectory)
- [ ] Move `core/schema/meta/*.go` → `pkg/schema/meta/`
- [ ] Create `pkg/schema/go.mod` with dependencies on `pkg/errors` and `pkg/data`
- [ ] Run `go build ./...` and `go test ./...`
- [ ] Update `go-anansi/core/schema/` to re-export from `pkg/schema`
- [ ] Verify all existing tests pass

### Affected Consumers

- **go-anansi:** 100+ files import `core/schema/definition`
- **hestia:** 39 files import `core/schema/definition`

### Backward Compatibility

Use type aliases in `core/schema/definition/` to maintain backward compatibility.
The top-level `core/schema/exports.go` already provides re-exports.

---

## Phase 3: Rewire `go-anansi`

### Steps

- [ ] Update `go-anansi/go.mod` to require the four new modules
- [ ] Replace `core/common/error.go` imports with `pkg/errors`
- [ ] Replace `core/cache/` imports with `pkg/cache`
- [ ] Replace `core/data/container/` imports with `pkg/data`
- [ ] Replace `core/schema/definition/` imports with `pkg/schema`
- [ ] Remove duplicated code from `core/` packages (keep only re-exports)
- [ ] Run full test suite: `go test ./...`
- [ ] Run hestia test suite: `go test ./...`
- [ ] Verify `hestia service generate --all` still works

---

## Phase 4: Rewire `hestia`

### Steps

- [ ] Update `hestia/go.mod` to require the four new modules (directly or via go-anansi)
- [ ] Replace `common.SystemError` imports with `pkg/errors`
- [ ] Replace `cache.NewManagedCache` imports with `pkg/cache`
- [ ] Replace `schema/definition` imports with `pkg/schema`
- [ ] Run full test suite
- [ ] Verify `hestia service generate --all` still works

---

## Risk Assessment

| Risk | Severity | Mitigation |
|---|---|---|
| Breaking API changes during extraction | High | Type aliases in original packages maintain backward compatibility |
| Subtle coupling discovered mid-extraction | Medium | Phase 1 items (errors, cache, data) have zero inter-dependencies — safe to extract first |
| `pkg/schema` internal dependency resolution | Medium | The 5 dependencies to resolve are small, well-isolated functions/types |
| Test breakage across modules | Medium | Run full test suites after each phase; use `replace` directives during development |
| `unsafe.Pointer` in DataContainer | Low | Move as-is; the unsafe usage is contained and well-understood |

---

## Success Criteria

- [ ] `pkg/errors` compiles and passes tests independently
- [ ] `pkg/cache` compiles and passes tests independently
- [ ] `pkg/data` compiles and passes tests independently
- [ ] `pkg/schema` compiles and passes tests with `pkg/errors` + `pkg/data`
- [ ] `go-anansi` compiles and passes all tests with rewired imports
- [ ] `hestia` compiles and passes all tests with rewired imports
- [ ] `hestia service generate --all` produces identical output
- [ ] No circular dependencies between new modules
- [ ] Each new module has a standalone README with usage examples not mentioning anansi
