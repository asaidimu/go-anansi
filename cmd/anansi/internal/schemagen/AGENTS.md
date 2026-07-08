## Project overview

This is a Go project using the **go-anansi** framework — a schema-aware document persistence platform.
It uses SQLite for development (via Playground) and supports PostgreSQL/MySQL for production (via Setup).

The project is managed with the **anansi** CLI tool which generates migrations from JSON schema files.

---

## Schema file format

Schemas are JSON files (default glob: `schemas/**/*.schema.json`). Each schema defines a collection's structure, indexes, constraints, and nested sub-schemas. Field and entity IDs are UUID v7.

```jsonc
{
  "version": "0.1.0",           // semver
  "name": "Example",             // collection name (must match filename)
  "description": "...",          // optional

  "fields": {                    // UUID v7 -> field definition
    "019d7775-6563-7c55-a6f3-ac8f087d89d1": {
      "name": "email",
      "description": "...",      // optional
      "type": "string",          // required
      "required": true,          // default false
      "deprecated": false,       // default false
      "unique": false,           // default false
      "nullable": false,         // default false
      "default": "unknown",      // optional (string/number/boolean/array/object/null)
      "schema": {                // optional — reference for complex types
        "id": "<nested-schema-uuid>"
      }
    }
  },

  "indexes": {                   // UUID v7 -> index definition
    "019d7775-6563-7c55-a6f3-ac8f087d89d2": {
      "name": "by_email",
      "type": "btree",           // "btree" | "gin" | "gist" | "hash" | "brin"
      "fields": ["email"],
      "order": "asc",            // optional
      "unique": true,            // optional
      "condition": {             // partial index
        "field": "email",
        "operator": "is_not_null",
        "value": null
      }
    }
  },

  "constraints": {               // UUID v7 -> constraint definition
    "019d7775-6563-7c55-a6f3-ac8f087d89d3": {
      "name": "email_format",
      "description": "...",
      "operator": "must",        // logical operator
      "rules": [                 // constraint group
        {
          "predicate": "regex_match",
          "fields": ["email"],
          "parameters": "^.+@.+\\..+$"
        }
      ]
    }
  },

  "schemas": {                   // UUID v7 -> nested schema
    "019d7775-6563-7c55-a6f3-ac8f087d89d4": {
      "name": "Address",
      "description": "...",
      "type": "object",          // required for nested
      "fields": {
        "019d7775-6563-7c55-a6f3-ac8f087d89d5": {
          "name": "street",
          "type": "string"
        }
      },
      "concrete": false          // optional; concrete = inline, not ref
    }
  },

  "metadata": {                  // arbitrary key/value pairs
    "author": "dev"
  }
}
```

### Field types

| Type        | Description                     |
|-------------|---------------------------------|
| `string`    | UTF-8 text                     |
| `number`    | 64-bit float                    |
| `integer`   | 64-bit signed integer           |
| `decimal`   | Arbitrary precision decimal     |
| `boolean`   | true/false                      |
| `array`     | Ordered list of values          |
| `enum`      | One of a set of literal values  |
| `object`    | Structured nested document      |
| `record`    | Map of string -> value          |
| `union`     | One of multiple schema variants |
| `composite` | Combines multiple schemas       |
| `geometry`  | GeoJSON geometry                |
| `bytes`     | Binary data                     |

---

## CLI commands

### `anansi scaffold <dir>`

Creates a fully-runner project in `<dir>` with go.mod, main.go, example schema, migration files, anansi.json, schemas.lock.json, and AGENTS.md. Refuses to run in a non-empty directory.

  Flags:
    --dry-run   print what would be done without making changes

### `anansi schema migrate`

Scans schema files matching the glob pattern, computes diffs against the lockfile, and generates Go migration files. Updates the lockfile and regenerates `registry.go`.

  Flags:
    --glob       glob pattern for schema files (overrides anansi.json)
    --lockfile   lockfile path (overrides anansi.json)
    --out        output directory (overrides anansi.json migrations_dir)
    --check      exit non-zero if migrations need regeneration
    --dry-run    print what would be done without making changes

### `anansi schema new <name>`

Creates a blank schema file with the given name, version 0.1.0, and empty fields.

  Flags:
    --dir       output directory (default .)
    --dry-run   print what would be done without making changes

### `anansi schema squash <collection>`

Consolidates all intermediate migrations for a collection into a single combined migration. Intermediate files remain on disk for reference. Updates the lockfile with `sub_migrations` and regenerates `registry.go`.

  Flags:
    --lockfile   lockfile path (overrides anansi.json)
    --out        migration output directory (overrides anansi.json)
    --dry-run    print what would be done without making changes

### `anansi schema normalize <path>`

Rewrites a schema file to canonical JSON ordering (fields/indexes/constraints/nested schemas sorted by key). Does NOT change the content.

  Flags:
    --dry-run    print what would be done without making changes

### `anansi schema typescript`

Generates a TypeScript types file for all schemas (single .ts file).

  Flags:
    --glob      glob pattern for schema files (overrides anansi.json)
    --out       output TypeScript file (overrides anansi.json)
    --dry-run   print what would be done without making changes

### `anansi schema agents`

Generates a comprehensive AGENTS.md file in the current directory documenting the project's architecture, schema format, API, migration flow, and best practices.

  Flags:
    --out       output directory (default .)
    --dry-run   print what would be done without making changes

### `anansi version`

Prints the compiled-in version string (set via `-X main.Version` at build time).

---

## Configuration file

```jsonc
// anansi.json
{
  "schema": {
    "glob": "schemas/**/*.schema.json",    // glob for schema files
    "lockfile": "schemas.lock.json",       // tracking state
    "migrations_dir": "migrations/"        // output for generated Go files
  },
  "tsgen": {
    "out": "types.ts"                      // output TypeScript file
  }
}
```

---

## Migration lifecycle

           +--------------+
           |  Edit schema |
           |  .json file  |
           +------+-------+
                  |
                  v
     +------------------------+
     | anansi schema migrate  |
     |  * computes diff       |
     |  * detects phase       |
     |  * generates .go file  |
     |  * updates lockfile    |
     |  * regenerates registry|
     +------+-----------------+
            |
            v
    +---------------+
    |  go run .     |  <-- calls migrations.Apply()
    |  (apply in    |      which bootstraps collections
    |   runtime)    |      and applies pending migrations
    +------+-------+
           |
           v
     +------------+
     | anansi     |
     | schema     |  <-- (optional) consolidate steps
     | squash     |
     +------------+

### Migration phases

- **`schema_only`** — only DDL changes (add/drop column, create/drop index). No data transform needed. No transformer stub generated.
- **`full`** — requires a table copy + data transform. A transformer stub (panics by default) is generated; replace with real logic.
- **`ddl`** — backend-specific DDL with fallback to full if not supported in-place.

### What triggers `full` phase

- Field removed
- Field renamed (`name` changed)
- Field type changed
- Field default changed
- Field required/unique/nullable/deprecated changed
- Field's `schema` reference changed
- Index modified
- Constraint added/removed/modified
- Nested schema added/removed/modified

Adding a `required` field triggers `full`. Adding an optional field triggers `schema_only`.

### Version bumps

- `BumpPatch` — new optional field, description changes
- `BumpMinor` — new field (optional), removed non-unique index
- `BumpMajor` — required field added, field removed, type change, constraint/schema/index changes

### Version impact table

| Change                               | Bump       |
|--------------------------------------|------------|
| Field added (optional)               | BumpMinor  |
| Field added (required)               | BumpMajor  |
| Field removed                        | BumpMajor  |
| Field type changed                   | BumpMajor  |
| Field renamed                        | BumpMajor  |
| Field unique changed (false -> true) | BumpMajor  |
| Field unique changed (true -> false) | BumpMinor  |
| Field required changed               | BumpMajor  |
| Field default changed                | BumpMajor  |
| Field deprecated changed             | BumpMinor  |
| Index added (unique)                 | BumpMajor  |
| Index removed (unique)               | BumpMinor  |
| Index modified (fields)              | BumpMajor  |
| Index unique changed                 | BumpMajor  |
| Constraint added                     | BumpMajor  |
| Constraint modified (predicate)      | BumpMajor  |
| Nested schema added/removed          | BumpMajor  |
| Nested schema type/values changed    | BumpMajor  |

### Transformer pattern

When `anansi schema migrate` generates a `full` phase migration, it includes a transformer stub:

```go
m.Transformer = func(ctx context.Context, doc data.Document) (data.Document, error) {
    panic("migrations: Collection_0_0_0_to_0_1_0: implement transformer or remove this line")
}
```

Replace the panic with real transform logic:

```go
m.Transformer = func(ctx context.Context, doc data.Document) (data.Document, error) {
    email, err := doc.Get("email")
    if err != nil {
        return doc, nil // optional field, skip
    }
    doc.Set("email_normalized", strings.ToLower(email.(string)))
    return doc, nil
}
```

Panics in transformers are caught and returned as errors, so the migration is safely rolled back.

### Squash mechanics

`anansi schema squash <collection>` consolidates N intermediate migrations into one:

1. Computes the combined diff from the earliest history version to the current one.
2. Generates a single migration file (`<uuid>_squash_<name>_<from>_to_<to>.go`) with a chained transformer that calls each sub-migration's `Plan().Transformer` in order.
3. Updates the lockfile: `history` is reduced to a single entry, `sub_migrations` lists the original filenames for reference.
4. Intermediate migration files are NOT deleted — they remain on disk.
5. Registry's `Squash` map includes the squash entry with `SubMigrations []Migration` metadata.

---

## Core API reference

### Package `anansi` (root)

```go
// Production setup
func Setup(config SetupConfig) (base.Persistence, error)

// Dev playground (SQLite)
func Playground(cfg PlaygroundConfig) (base.Persistence, func(), error)
```

#### SetupConfig

| Field                          | Type                           | Required | Description                              |
|--------------------------------|--------------------------------|----------|------------------------------------------|
| `Interactor`                   | `query.DatabaseInteractor`     | Yes      | Concrete database backend                |
| `Logger`                       | `*zap.Logger`                  | No       | Defaults to zap.NewNop()                 |
| `EventBus`                     | `events.EventBus[...]`         | Yes      | Pub/sub backbone                         |
| `DocumentFactoryConfig`        | `data.DocumentFactoryConfig`   | Yes      | Document hashing, metadata, sanitization |
| `Decorators`                   | `*utils.Decorators`            | Yes      | Cross-cutting concerns                   |
| `Schemas`                      | `[]*definition.Schema`         | No       | Auto-create on first start               |

#### PlaygroundConfig

| Field                 | Type                          | Default       | Description                      |
|-----------------------|-------------------------------|---------------|----------------------------------|
| `DBPath`              | `string`                      | `:memory:`    | SQLite DSN / path                |
| `EnableLogging`       | `bool`                        | false         | zap.NewDevelopment()             |
| `EnableEvents`        | `bool`                        | false         | WatermillEventBus                |
| `EnableSanitization`  | `bool`                        | false         | Default sanitization patterns    |
| `Schemas`             | `[]*definition.Schema`        | nil           | Auto-create schemas              |
| `Logger`              | `*zap.Logger`                 | nil           | Custom logger                    |
| `CustomSanitizerConfig` | `*data.FieldMaskConfig`      | nil           | Custom sanitizer config          |

### Package `core/data` — Document & DocumentSet

**Document** is the primary data unit. It has three parts:
- **ID** (string, immutable, auto-generated)
- **Data** (map[string]any) — user-managed fields, accessed via `Get`/`Set`
- **Metadata** (map[string]any) — system and custom metadata

```go
// Creation
func NewDocument(data any, ctx ...context.Context) (*Document, error)
func MustNewDocument(data any, ctx ...context.Context) *Document

// Reading user data
func (d *Document) Get(key string) (any, error)
func (d *Document) GetString(key string) (string, error)
func (d *Document) GetInt(key string) (int, error)
func (d *Document) GetFloat64(key string) (float64, error)
func (d *Document) GetBool(key string) (bool, error)
func (d *Document) GetTime(key string) (time.Time, error)
func (d *Document) GetArray(key string) ([]any, error)
func (d *Document) GetDocument(key string) (*Document, error)
func (d *Document) GetDocumentArray(key string) ([]*Document, error)

// Writing user data
func (d *Document) Set(key string, value any) error
func (d *Document) SetIfNotExists(key string, value any) bool

// Document identity
func (d *Document) ID() string
func (d *Document) Version() (int, error)

// Metadata access
func (d *Document) Metadata() map[string]any
func (d *Document) SetMetadataValue(key string, value any) error
func (d *Document) GetMetadataValue(key string) (any, error)

// Serialization
func (d *Document) ToMap() map[string]any
func (d *Document) Data() map[string]any
func (d *Document) ToJSON(pretty bool) ([]byte, error)

// Cloning & merging
func (d *Document) Clone() *Document
func (d *Document) Merge(others ...*Document)
func (d *Document) DeepMerge(others ...*Document)

// Checks
func (d *Document) HasKey(key string) bool
func (d *Document) HasPath(path string) bool
func (d *Document) Keys() []string
func (d *Document) Len() int
func (d *Document) IsEmpty() bool
```

**Note:** `Get`/`Set` only work on user data (not `_id` or `_metadata_`). Use `ID()` and `Metadata()` for system fields.

#### DocumentSet

```go
type DocumentSet []*Document

func (ds DocumentSet) Query() *FluentQuery
```

#### FluentQuery

```go
func (fq *FluentQuery) Where(key string, value any) *FluentQuery
func (fq *FluentQuery) WhereFunc(predicate func(*Document) bool) *FluentQuery
func (fq *FluentQuery) WhereField(key string) *FieldComparison  // .Equals, .GreaterThan, .LessThan, .Contains, .In
func (fq *FluentQuery) SortBy(key string) *FluentQuery
func (fq *FluentQuery) SortByDesc(key string) *FluentQuery
func (fq *FluentQuery) Limit(n int) *FluentQuery
func (fq *FluentQuery) Offset(n int) *FluentQuery
func (fq *FluentQuery) Execute() DocumentSet
func (fq *FluentQuery) Count() int
func (fq *FluentQuery) First() (*Document, bool)
func (fq *FluentQuery) Exists() bool
```

### Package `core/query` — Database queries

```go
type Query struct {
    Target    *QueryTarget    // collection + schema
    Filter    *QueryFilter    // field conditions
    Sort      []SortField     // field + direction
    Paginate  *Pagination     // offset + limit
}

type QueryFilter struct {
    Conditions []QueryCondition   // logical conditions
    Operator   LogicalOperator    // "and" | "or"
}

type QueryCondition struct {
    Field    string
    Operator ComparisonOperator  // eq, neq, gt, gte, lt, lte, in, nin, exists, regex, etc.
    Value    any
}
```

### Package `core/persistence/base` — Migration & Persistence

```go
// Persistence interface (main entry point)
type Persistence interface {
    BasePersistence
    CreateCollection(ctx context.Context, sc *definition.Schema) (Collection, error)
    CreateCollections(ctx context.Context, schemas []*definition.Schema) error
    HasCollection(ctx context.Context, name string) (bool, error)
    Transact(ctx context.Context, cb func(context.Context, BasePersistence) (any, error)) (any, error)
    Subscribe(ctx context.Context, opts SubscriptionOptions) string
    Unsubscribe(ctx context.Context, id string)
    Rollback(ctx context.Context, name string, version *string, dryRun *bool) (Collection, error)
    Migrate(ctx context.Context, name string, migration any, dryRun *bool) (Collection, error)
    Close(ctx context.Context)
}

// BasePersistence (usable inside transactions)
type BasePersistence interface {
    Collection(ctx context.Context, name string) (Collection, error)
    ListCollections(ctx context.Context) ([]string, error)
    Delete(ctx context.Context, name string) (bool, error)
    Schema(ctx context.Context, name string, version ...string) (*definition.Schema, error)
    Metadata(ctx context.Context, filter *MetadataFilter) (Metadata, error)
    Async(ctx context.Context, f func(context.Context) (any, error)) Future
    Query(ctx context.Context, rawQuery *query.RawQuery) (*query.RawQueryResult, error)
}

// Collection operations
type Collection interface {
    CreateOne(ctx context.Context, doc *data.Document) (CreateResult, error)
    CreateMany(ctx context.Context, docs []*data.Document) ([]CreateResult, error)
    Read(ctx context.Context, q *query.Query) (*ReadResult, error)
    Update(ctx context.Context, params *CollectionUpdate) (*ReadResult, error)
    Delete(ctx context.Context, q *query.QueryFilter, unsafe bool) (int, error)
    Validate(ctx context.Context, data *data.Document, partial bool) ([]common.Issue, bool)
    Metadata(ctx context.Context, filter *MetadataFilter, forceRefresh bool) *CollectionMetadata
    Subscribe(ctx context.Context, opts SubscriptionOptions) string
    Unsubscribe(ctx context.Context, id string)
    Capabilities(ctx context.Context) *query.Capabilities
}

// Migration plan (used by generated code)
type MigrationPlan struct {
    Description string
    Target      *definition.Schema
    Diff        *definition.SchemaDiff
    VersionBump definition.VersionBump
    Phase       MigrationPhase
    Transformer TransformerFunc
}

func NewSchemaOnlyMigration(target *definition.Schema, description string) *MigrationPlan
func NewFullMigration(target *definition.Schema, description string, transformer TransformerFunc) *MigrationPlan
func (p *MigrationPlan) ComputeDiff(currentSchema *definition.Schema) error
func (p *MigrationPlan) TargetVersion(current *common.Version) *common.Version
func (p *MigrationPlan) NeedsDataMigration() bool
func (p *MigrationPlan) ResolvePhase(canApplyInPlace func(*definition.SchemaDiff) bool)

// Migration phases
const (
    PhaseSchemaOnly MigrationPhase = "schema_only"
    PhaseDDL        MigrationPhase = "ddl"
    PhaseFull       MigrationPhase = "full"
)

// Transformer function for data migration
type TransformerFunc func(ctx context.Context, sourceDoc data.Document) (data.Document, error)
```

### Package `core/persistence/migration` — Data migrator

```go
type DataMigrator interface {
    Migrate(ctx context.Context, collectionName, sourceVersion, destVersion string, transformer Transformer) (string, error)
}

type DefaultDataMigrator struct { /* ... */ }

func NewDefaultDataMigrator(interactor query.DatabaseInteractor, registry base.CollectionRegistry) *DefaultDataMigrator
func (m *DefaultDataMigrator) Migrate(ctx context.Context, collectionName, sourceVersion, destVersion string, transformer Transformer) (string, error)
```

### Package `core/schema/definition` — Schema & diff

```go
// Schema types
type Schema struct { Version *Version; Name string; Description string; Fields map[FieldId]Field; Indexes map[IndexID]Index; Constraints map[ConstraintId]Constraint; Schemas map[SchemaId]NestedSchema; Metadata map[string]any }
type Field struct { Name FieldName; Description string; Required bool; Deprecated bool; Unique bool; Nullable bool; Type FieldType; Default LiteralValue; Schema FieldSchemaReference }
type Index struct { Name string; Description string; Order string; Fields []FieldName; Type IndexType; Unique bool; Condition IndexConditionUnion }
type Constraint struct { Name string; Description string; /* union: ConstraintRule or ConstraintGroup */ }

// Schema serialization
func FromJSON(data []byte) (*Schema, error)
func (s *Schema) ToJSON() []byte

// Diff engine
func Diff(old, new *Schema) (*SchemaDiff, error)
func VersionImpact(diff *SchemaDiff) VersionBump

type SchemaDiff struct { Changes []SemanticChange }
type SemanticChange struct { Kind ChangeKind; EntityId string; Forward []Operation; Backward []Operation }
type ChangeKind byte        // FieldAdded, FieldRemoved, FieldModified, IndexAdded, etc.
type OperationType byte     // OpAdd, OpRemove, OpSet, OpCollectionInsert, OpCollectionDelete

// Version bump constants
const (
    BumpNone  VersionBump = 0
    BumpPatch VersionBump = 1
    BumpMinor VersionBump = 2
    BumpMajor VersionBump = 3
)
func (b VersionBump) Apply(v common.Version) common.Version

// Schema manipulation
func (s *Schema) DeepCopy() *Schema
func (s *Schema) WithField(id FieldId, field Field) *Schema
func (s *Schema) WithFieldEnsured(field *Field) (*Schema, FieldId, bool, error)
func (s *Schema) WithIndex(id IndexID, index Index) *Schema
func (s *Schema) WithIndexEnsured(index *Index) (*Schema, bool, error)
```

### Package `core/common` — Version & versioning

```go
type Version struct { Major int; Minor int; Patch int; TagType TagRank; TagVersion int }

func NewVersion(version string) (*Version, error)
func MustNewVersion(version string) *Version
func (v Version) String() string
func (v Version) BumpMajor() Version
func (v Version) BumpMinor() Version
func (v Version) BumpPatch() Version
func (v *Version) Compare(other *Version) int

// Versionable tracks version history
type Versionable struct { /* ... */ }
func NewVersionable(registry *VersionRegistry, current string, history ...string) (*Versionable, error)
func (v *Versionable) Current() *Version
func (v *Versionable) History() []*Version
func (v *Versionable) BumpMajor() (*Versionable, error)
func (v *Versionable) BumpMinor() (*Versionable, error)
func (v *Versionable) BumpPatch() (*Versionable, error)
func (v *Versionable) Prerelease(tag string) (*Versionable, error)
func (v *Versionable) Promote(tag string) (*Versionable, error)
func (v *Versionable) Stabilize() (*Versionable, error)

// Default tags: alpha(10), beta(20), rc(30), stable(255)
```

---

## Event system

```go
// Event types
const (
    DocumentCreateStart   PersistenceEventType = "document:create:start"
    DocumentCreateSuccess PersistenceEventType = "document:create:success"
    DocumentCreateFailed  PersistenceEventType = "document:create:failed"
    DocumentReadStart     PersistenceEventType = "document:read:start"
    DocumentReadSuccess   PersistenceEventType = "document:read:success"
    DocumentReadFailed    PersistenceEventType = "document:read:failed"
    DocumentUpdateStart   PersistenceEventType = "document:update:start"
    DocumentUpdateSuccess PersistenceEventType = "document:update:success"
    DocumentUpdateFailed  PersistenceEventType = "document:update:failed"
    DocumentDeleteStart   PersistenceEventType = "document:delete:start"
    DocumentDeleteSuccess PersistenceEventType = "document:delete:success"
    DocumentDeleteFailed  PersistenceEventType = "document:delete:failed"
    MigrateStart          PersistenceEventType = "migrate:start"
    MigrateSuccess        PersistenceEventType = "migrate:success"
    MigrateFailed         PersistenceEventType = "migrate:failed"
    TransactionStart      PersistenceEventType = "transaction:start"
    TransactionSuccess    PersistenceEventType = "transaction:success"
    TransactionFailed     PersistenceEventType = "transaction:failed"
    // ... plus CollectionCreate*, CollectionUpdate*, CollectionDelete*, Telemetry, etc.
)

type PersistenceEvent struct {
    Type          PersistenceEventType
    Timestamp     int64
    Operation     string
    Collection    *string
    Input         any
    Output        any
    Error         *string
    Issues        []common.Issue
    Query         any
    TransactionID *string
    Duration      *int64
    Context       map[string]any
}

func (p Persistence) Subscribe(ctx context.Context, opts SubscriptionOptions) string
type SubscriptionOptions struct {
    Event       PersistenceEventType
    Label       *string
    Description *string
    Callback    EventCallbackFunction
}
type EventCallbackFunction func(ctx context.Context, event PersistenceEvent) error
```

---

## Generated registry (`migrations/registry.go`)

```go
type Migration struct {
    UUID          string
    Collection    string
    From          string
    To            string
    File          string
    Plan          func() *base.MigrationPlan
    SubMigrations []Migration  // populated for squash entries
}

var Plain = []Migration{/* sorted by UUID */}
var Squash = []Migration{/* sorted by UUID */}

func All() []Migration       // merged and sorted

func Apply(ctx context.Context, p base.Persistence) error
```

`Apply()` reconciles the database:
1. For each collection, groups migrations by collection name.
2. If the collection doesn't exist, bootstraps from the first migration's target.
3. Walks remaining migrations in order, checking if the current schema version matches `From`.
4. Calls `p.Migrate()` to apply each matching plan.

---

## Lockfile (`schemas.lock.json`)

```jsonc
{
  "version": "1",
  "schemas": {
    "Example": {
      "path": "schemas/example.schema.json",
      "hash": "sha256:<hex>",
      "version": "0.1.0",
      "schema": { /* full schema definition */ },
      "migration_file": "019d7775..._Example_minor.go",
      "history": [
        { "version": "0.1.0", "schema": {…}, "migration_file": "…" }
      ],
      "sub_migrations": [  // only for squashed entries
        "019d7775..._Example_minor.go",
        "019d8885..._Example_major.go"
      ]
    }
  }
}
```

---

## Common patterns

### Minimal scaffold application

```go
package main

import (
    anansi "github.com/asaidimu/go-anansi/v7"
    "github.com/asaidimu/go-anansi/v7/core/data"
    "github.com/asaidimu/go-anansi/v7/core/query"
    "MPKG"  // replace with actual module path
)

func main() {
    p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{DBPath: "data.db"})
    defer cleanup()

    migrations.Apply(ctx, p)

    coll, _ := p.Collection(ctx, "Example")
    coll.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "hello"}))
    result, _ := coll.Read(ctx, &query.Query{})
}
```

### Production setup

```go
p, err := anansi.Setup(anansi.SetupConfig{
    Interactor: pgInteractor,
    Logger:     logger,
    EventBus:   eventBus,
    DocumentFactoryConfig: data.DocumentFactoryConfig{ /* ... */ },
    Decorators: &utils.Decorators{},
    Schemas:    schemas,
})
```

### Using ModelCollection (type-safe CRUD)

```go
type User struct {
    ID    string `anansi:"_id"`
    Name  string `anansi:"name"`
    Email string `anansi:"email"`
}

// Bind users to a typed model collection
users := model.NewCollection[User, User](coll)
user, _ := users.Create(ctx, User{Name: "Alice", Email: "alice@example.com"})
users.FindByID(ctx, user.ID)
users.Read(ctx, &query.Query{Filter: &query.QueryFilter{...}})
```

### Writing a transformer

```go
m.Transformer = func(ctx context.Context, doc data.Document) (data.Document, error) {
    val, err := doc.Get("old_field")
    if err != nil {
        return doc, nil
    }
    doc.Set("new_field", val)
    return doc, nil
}
```

### Listening to events

```go
p.Subscribe(ctx, SubscriptionOptions{
    Event:       base.DocumentCreateSuccess,
    Label:       ptr("audit"),
    Description: ptr("log all creates"),
    Callback: func(ctx context.Context, event base.PersistenceEvent) error {
        log.Printf("document created: %v", event.Output)
        return nil
    },
})
```

### Transactions

```go
result, err := p.Transact(ctx, func(ctx context.Context, tx base.BasePersistence) (any, error) {
    coll, _ := tx.Collection(ctx, "Example")
    coll.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "tx-test"}))
    return nil, nil
})
```

---

## Testing

```bash
go test ./...          # run all tests
go test ./migrations/  # test migration code
```

The scaffolded project is tested by running through the full lifecycle:
1. `anansi scaffold` creates a runnable project
2. `anansi schema migrate` generates migrations
3. `anansi schema squash` consolidates them
4. `go run .` applies migrations and creates documents

---

## Error handling

The framework uses structured errors via `common.SystemError`:

```go
// Creating errors
common.SystemErrorFrom(data.ErrKeyNotFound).
    WithOperation("myfunction").
    WithPath("field.name").
    WithMessage("custom message").
    WithCause(err)

// Common error constructors (from common package)
var (
    ErrAlreadyConfigured = errors.New("versioning strategy has already been configured")
    ErrHistoryFull       = errors.New("version history is full (max 1024 versions)")
    ErrInvalidAddress    = errors.New("invalid history address")
    ErrNotPrerelease     = errors.New("operation requires pre-release version")
    ErrAlreadyStable     = errors.New("version is already stable")
    ErrVersionNotFound   = errors.New("version not found in history")
)
```

Data package errors:

```go
var (
    ErrKeyNotFound       = errors.New("key not found in document data")
    ErrReadOnlyField     = errors.New("field is read-only")
    ErrKeyEmpty          = errors.New("key cannot be empty")
    ErrTypeConversion    = errors.New("type conversion failed")
    ErrTypeMismatch      = errors.New("type mismatch")
    ErrInvalidTargetType = errors.New("invalid target type")
    ErrNoMetadata        = errors.New("document has no metadata")
    ErrSignatureInvalid  = errors.New("signature is invalid")
)
```
