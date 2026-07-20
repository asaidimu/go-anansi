## [8.0.6](https://github.com/asaidimu/go-anansi/compare/v8.0.5...v8.0.6) (2026-07-20)


### Bug Fixes

* fast push ([75efbc2](https://github.com/asaidimu/go-anansi/commit/75efbc27f6fdda81bcea5f26a956de3cc10c3027))

## [8.0.5](https://github.com/asaidimu/go-anansi/compare/v8.0.4...v8.0.5) (2026-07-19)


### Bug Fixes

* fix cmd version ([e50f4ab](https://github.com/asaidimu/go-anansi/commit/e50f4ab8732aabd5de289193c7ed069a504979b6))
* fix sqlite null check ([4364b4f](https://github.com/asaidimu/go-anansi/commit/4364b4f97918093303ebfb011f1b7876c7267ee1))

## [8.0.3](https://github.com/asaidimu/go-anansi/compare/v8.0.2...v8.0.3) (2026-07-15)


### Bug Fixes

* fix bug in sql generation ([1259821](https://github.com/asaidimu/go-anansi/commit/12598210ef514684bbe76d66dcf62c619a1db790))

## [8.0.2](https://github.com/asaidimu/go-anansi/compare/v8.0.1...v8.0.2) (2026-07-13)


### Bug Fixes

* fix validator and implement a schema for queries ([989f8ed](https://github.com/asaidimu/go-anansi/commit/989f8ed52c41c46a7dff3be0679c7ca5a62c5bd6))

## [8.0.1](https://github.com/asaidimu/go-anansi/compare/v8.0.0...v8.0.1) (2026-07-09)


### Bug Fixes

* fix import path ([bf8c374](https://github.com/asaidimu/go-anansi/commit/bf8c374dd48127d34cb8989e82229f0e549ec095))

# [8.0.0](https://github.com/asaidimu/go-anansi/compare/v7.1.1...v8.0.0) (2026-07-09)


* feat(core)!: introduce comprehensive subquery support and document integrity ([fa9a295](https://github.com/asaidimu/go-anansi/commit/fa9a295233f06a9138f71af0a42617a3e52738f0))
* feat(schema,document)!: introduce graph-based validator and refactor document internals ([73c66b3](https://github.com/asaidimu/go-anansi/commit/73c66b3d2ade1cad36e464094fde3e4621266049)), closes [hi#performance](https://github.com/hi/issues/performance)
* refactor(core,errors)!: Revamp error handling with immutable fluent API and i18n ([49e2540](https://github.com/asaidimu/go-anansi/commit/49e25408a8ebe889200798ede17eea34006bf6d1))
* refactor(core)!: Remove core document data model and schema IR ([95a1a78](https://github.com/asaidimu/go-anansi/commit/95a1a78a24c2bf0fd2ead3396c4cc163b71b63c8))


### Bug Fixes

* add compiled schema and uuidv7 constraint ([54e709f](https://github.com/asaidimu/go-anansi/commit/54e709f224c502610ef9c2374964b4d837267620))
* **document:** implement Document Specification v2.0 ([b3ea318](https://github.com/asaidimu/go-anansi/commit/b3ea318a2347122edf59802a411ac58b21f4558e))
* fix build command ([b595f05](https://github.com/asaidimu/go-anansi/commit/b595f057acdd60b6b9c7985a70ddface801bb818))
* fix package version ([781da1e](https://github.com/asaidimu/go-anansi/commit/781da1ed433308df5d1cf94d34e574d6b920d1f4))
* moved faker to the main binary ([5e4543d](https://github.com/asaidimu/go-anansi/commit/5e4543d383dcb6f8ad25b2f99828dcb9d1b34c16))


### Code Refactoring

* **core/document, schema:** Redesign data container and graph-based schema validation ([045f511](https://github.com/asaidimu/go-anansi/commit/045f511221721f1fbc12838170784b78c62f9cee))


### Features

* add PaginationInfo metadata to query results ([446cbc8](https://github.com/asaidimu/go-anansi/commit/446cbc83fe4bc556ae99aaee6325de16a74757c2))
* completed moving core to new schema ([0414e5b](https://github.com/asaidimu/go-anansi/commit/0414e5be8c5b23b5f6bd70bb7c4e9d09bb10bc3d))
* **core:** introduce JSON Schema validation, patch, and versioning system ([f71b359](https://github.com/asaidimu/go-anansi/commit/f71b359f265599d1557b1400ffb933e2a0f5ebc6))
* **data-sanitization:** introduce comprehensive and context-aware data sanitization ([3ea7494](https://github.com/asaidimu/go-anansi/commit/3ea7494e50b60df137d3b3255deb26ddf37fcff3))
* implement migration and some tooling ([5e24709](https://github.com/asaidimu/go-anansi/commit/5e247096270a8decd70dc1ce4d3fed3bb2af677c))
* implement migration and some tooling ([fcffac5](https://github.com/asaidimu/go-anansi/commit/fcffac594f653797d01d9dbb2097972d67381796))
* **query:** include total match count in query results ([fe5f1cf](https://github.com/asaidimu/go-anansi/commit/fe5f1cfd2fef7c2defc0e4c991085aecf83facef))
* **schema/ir:** introduce decimal type and improve schema validation ([b76edcf](https://github.com/asaidimu/go-anansi/commit/b76edcf1b23af0120315e976dd8269bf31f60c37))
* **schema:** enhance schema definitions and refine JSON serialization ([4a4f654](https://github.com/asaidimu/go-anansi/commit/4a4f654617e7199a62ee46b656bc139f56644246))
* **schema:** enhance validation engine with recursive schema support and MetaSchema ([61bf48e](https://github.com/asaidimu/go-anansi/commit/61bf48e4e454f5ef9f6834315658c06d08a266f2))
* **schema:** Introduce FieldSets for explicit conditional field definitions ([b0eb9d5](https://github.com/asaidimu/go-anansi/commit/b0eb9d58fa157cb08d411be410c47a72e657c4c8))
* **schema:** Introduce schema modification utilities and factory methods ([83c834e](https://github.com/asaidimu/go-anansi/commit/83c834e4f6fc31a99677bf13df95b182be67850f))


### BREAKING CHANGES

* All direct imports of "github.com/asaidimu/go-anansi/v6/core/document", "github.com/asaidimu/go-anansi/v6/core/schema/ir", and "github.com/asaidimu/go-anansi/v6/core/schema/validate" must be removed. The core persistence layer will be re-architected with new data structures and schema representation. Existing code relying on `document.Document` for data handling or `ir.Schema` for schema introspection will require significant rewriting.
* The method signatures for `document.Document.GetRecord`,
`SetRecord`, and `AppendRecord` have been updated.
The map value type for nested records has changed from `*document.Document` to `any`.
Direct consumers of these low-level `Document` methods will need to adjust their
code to handle `any` when interacting with nested records.
For example, `doc.GetRecord("path")` will now return `(map[string]any, bool, error)` instead of
`(map[string]*document.Document, bool, error)`.
* **core/document, schema:** This refactoring introduces breaking changes to the `core/document` and `core/schema/ir` packages.
- The `document.DataContainer` type has been removed; its functionality is now directly part of `document.Document`.
- Field identifiers throughout the `core/schema/ir` package, particularly in `ResolvedConstraints`, `ResolvedIndexes`, and `CompiledSchema.Store`, now use `document.DocumentKey` instead of `document.DataPoint`.
- The `ir.Predicate` function signature has changed from `func(data *document.DataContainer, fields []document.DataPoint, args any) bool` to `func(data *document.Document, fields []document.DocumentKey, args any) bool`.
- Any custom `ir.Predicate` implementations or direct interactions with `document.DataContainer` or `document.DataPoint` (when resolving schema fields) will need to be updated.
* **schema:** - The `data.SanitizationPersistence.Save` method signature has changed from `(ctx context.Context, scope string, config *data.FieldMaskConfig)` to `(ctx context.Context, config *data.FieldMaskConfig)`. The `data.FieldMaskConfig` struct must now embed the `Scope` field. Implementations of this interface need to be updated.
- The `core/schema.NestedSchemaFields.IsArray()` method was renamed to `IsLegacyFieldsArray()`. Direct calls to `IsArray()` will result in a compilation error. `IsFieldSets()` and `IsConditionalSets()` were added to provide explicit checks for the new structure.
- While `core/schema.NestedSchemaFields.FieldsArray` is maintained for backward compatibility, new conditional field definitions should utilize the `core/schema.NestedSchemaFields.FieldSets` map for improved clarity and addressability.
* The core error handling system has undergone a major overhaul.
- The common.Issue and common.SystemError internal structures have changed. Any direct access to Issue.Message, Issue.Path, SystemError.Message, SystemError.Code may behave differently or require adaptation to the new fluent API.
- The core/json.ValidationError type has been removed; validation errors from core/json/schema now return *common.SystemError directly.
- The return type of core/persistence/base.CreateResultSet.Issues() has changed from []CreateIssue to []common.Issue. Consumers of batch creation results must update their error handling logic.
- The API error response format in the example/api has been standardized to reflect the common.SystemError model, potentially impacting existing API clients that parse error details.
- Error codes and messages for common persistence and schema operations have been refined.
* - data.Document.AsMap() has been renamed to data.Document.ToMap(). Update all calls from document.AsMap() to document.ToMap().
- data.DocumentSlice is deprecated; use data.NewDocumentSet for document slice conversions.
- data.Document.Flatten() and data.Unflatten() methods have been removed. Re-implement any logic relying on these methods.
- The base.Collection.Update method signature has changed to return a *base.ReadResult, not an int. Adjust code calling this method to match the new return type.
- The query.DatabaseInteractor.UpdateDocuments method signature has changed to include a 'returning' boolean and return updated documents. Adjust code calling this method to match the new parameters and return types.
- base.ReadResult.Data type changed from []*data.Document to data.DocumentSet. Adjust code accessing this field.
- data.DocumentID constant renamed to data.DocumentIDField. Replace usages of data.DocumentID with data.DocumentIDField.
- schema.SchemaDefinition.MustAddIndex has been renamed to schema.SchemaDefinition.AddIndex and now returns the modified schema. Update calls accordingly.
- The CollectionUpdate struct now includes a ReturnDocument boolean field. Set this field as needed for update operations that require returning the modified documents.

## [7.1.1](https://github.com/asaidimu/go-anansi/compare/v7.1.0...v7.1.1) (2026-07-05)


### Bug Fixes

* add compiled schema and uuidv7 constraint ([e94c867](https://github.com/asaidimu/go-anansi/commit/e94c86799bf5efd7fdd9840f55ee22d2fe122439))
* fix build command ([4f03b6e](https://github.com/asaidimu/go-anansi/commit/4f03b6e7eaf28ebf57ce30afd98e041e28805b7f))

# [7.1.0](https://github.com/asaidimu/go-anansi/compare/v7.0.1...v7.1.0) (2026-07-01)


### Features

* add PaginationInfo metadata to query results ([383b0a9](https://github.com/asaidimu/go-anansi/commit/383b0a94f3449fb04da0f1570dd5c3ac29811a62))

## [7.0.1](https://github.com/asaidimu/go-anansi/compare/v7.0.0...v7.0.1) (2026-06-22)


### Bug Fixes

* fix package version ([2fa0dbe](https://github.com/asaidimu/go-anansi/commit/2fa0dbe696b1d84879d0df5aec92b7a9a622ca12))

# [7.0.0](https://github.com/asaidimu/go-anansi/compare/v6.0.0...v7.0.0) (2026-06-20)


* feat(core)!: introduce comprehensive subquery support and document integrity ([57ec704](https://github.com/asaidimu/go-anansi/commit/57ec7044fb05c743b9aa5d6701be01c3cb3c8382))
* feat(engine)!: overhaul query engine and introduce schema registry ([82e7437](https://github.com/asaidimu/go-anansi/commit/82e74378b873692e21d328af65fa44a61ec7f766))
* feat(schema,document)!: introduce graph-based validator and refactor document internals ([9830da5](https://github.com/asaidimu/go-anansi/commit/9830da5ef19d9d6e29ff64d58e860635b990a519))
* refactor(core,errors)!: Revamp error handling with immutable fluent API and i18n ([42d02c6](https://github.com/asaidimu/go-anansi/commit/42d02c6c067e71b8dec3292b267581c000acd4f6))
* refactor(core)!: Remove core document data model and schema IR ([b571bfd](https://github.com/asaidimu/go-anansi/commit/b571bfdb92037da530090b9b107226fc274087b2))
* refactor(data, persistence, query)!: standardize Document as struct and refine core APIs ([bfeb95f](https://github.com/asaidimu/go-anansi/commit/bfeb95f0f3270f012532a3e18c734f12d8c0e1a0))
* refactor(data, persistence, query)!: standardize Document as struct and refine core APIs ([76c306a](https://github.com/asaidimu/go-anansi/commit/76c306a5acde111ac183a6cb2b63956cf3725824))
* refactor(schema)!: revamp schema definition to support discriminated unions and registry patterns ([4f09fbe](https://github.com/asaidimu/go-anansi/commit/4f09fbe0997e0883109afe9bb1d265686de23e10))


### Bug Fixes

* **document:** implement Document Specification v2.0 ([0fabb26](https://github.com/asaidimu/go-anansi/commit/0fabb26e95b532ec3d2275eedd945a5278411401))


### Code Refactoring

* **core/document, schema:** Redesign data container and graph-based schema validation ([9688f52](https://github.com/asaidimu/go-anansi/commit/9688f52f18f5cbb8e8345a867616a4492a5a87b8))
* **core:** introduce structured errors, revamp documentation, enhance dev experience ([6bbd2d7](https://github.com/asaidimu/go-anansi/commit/6bbd2d76156ccb6e92e2688b803ebca644ecb6df))


### Features

* **api:** standardize context propagation across persistence interfaces ([f04631e](https://github.com/asaidimu/go-anansi/commit/f04631e9acbb1268d999b72dc8de13bd46824223))
* Centralize utilities, enhance error handling, and introduce example API ([0bd6e31](https://github.com/asaidimu/go-anansi/commit/0bd6e318861a0c7a8e58d6289a90ab4b44ff8b19))
* completed moving core to new schema ([2e7a5c8](https://github.com/asaidimu/go-anansi/commit/2e7a5c803cd0f956b5d4ae932a380926583446dc))
* **core,data,persistence:** introduce automatic ID generation and enhanced integrity ([f305d27](https://github.com/asaidimu/go-anansi/commit/f305d27c855e346cafe4e4055bc49b7f6d36768e))
* **core:** enhance document creation and refactor schema validator ([b96a91c](https://github.com/asaidimu/go-anansi/commit/b96a91c9914dffe6eecc698bff0b2ce3080a14b2))
* **core:** introduce business rule engine and enhance persistence ([546796a](https://github.com/asaidimu/go-anansi/commit/546796a908cddca24f970468a9f0281f7e84a3a5))
* **core:** introduce JSON Schema validation, patch, and versioning system ([0102602](https://github.com/asaidimu/go-anansi/commit/0102602ced9721c78c6f2ecc3a77dee3c9fcdec7))
* **core:** overhaul persistence and query architecture with schema registry ([5e68ee9](https://github.com/asaidimu/go-anansi/commit/5e68ee90bb0b974ca4332fc2e7c6c69bfde32074))
* **core:** propagate context, enhance DDL, and refactor transactions ([fe3afb4](https://github.com/asaidimu/go-anansi/commit/fe3afb4035880f675523d134e3ec7797363a48d3))
* **data-sanitization:** introduce comprehensive and context-aware data sanitization ([c6da140](https://github.com/asaidimu/go-anansi/commit/c6da1404604aac3305dc62c1f07acf314f1acd99))
* **data, anansi, persistence:** Introduce document sanitization and enhance core persistence ([49282e0](https://github.com/asaidimu/go-anansi/commit/49282e097ac781e7431d61dc7d1e43df5e044718))
* Enhance anansi setup and provide comprehensive examples ([693fbfb](https://github.com/asaidimu/go-anansi/commit/693fbfba87a709e5b7bd6c3c5ae562bcf7d4b26a))
* Persistence layer refactor (WIP) ([aea9c41](https://github.com/asaidimu/go-anansi/commit/aea9c413edc5b067e6be9d9e407f2675f45996d9))
* **persistence:** add transaction-aware goroutine spawning and refine transaction model ([9d1b7d3](https://github.com/asaidimu/go-anansi/commit/9d1b7d339d36bf2314839bd43df82bbafcaa9665))
* **persistence:** allow Collection.Read to execute raw queries ([a412b7a](https://github.com/asaidimu/go-anansi/commit/a412b7a3d7c3cc2de58672d4638bf7d3edc955c1))
* **persistence:** Introduce atomic bulk collection creation ([13c9796](https://github.com/asaidimu/go-anansi/commit/13c979619820f569bf1dbb5fd3a2c44e6fffaefc))
* **persistence:** introduce type-safe ModelCollection and refactor internals ([cc5d8ad](https://github.com/asaidimu/go-anansi/commit/cc5d8ad2218e9fa72e85a90e2261a06793f9d9e7))
* **query:** implement cursor and enhance offset pagination ([17b7ceb](https://github.com/asaidimu/go-anansi/commit/17b7cebf0d8ce0184ee13f0e7bada03ac245b631))
* **query:** include total match count in query results ([0b1a20a](https://github.com/asaidimu/go-anansi/commit/0b1a20a7c0eb05eaccfd630d8e9af8136b0f4487))
* **query:** introduce raw query execution with templated collections ([8f19773](https://github.com/asaidimu/go-anansi/commit/8f19773d8fd546edc9767ed4203f2526e2be04e1))
* Refactor core/data package for enhanced document and metadata handling ([f0fd435](https://github.com/asaidimu/go-anansi/commit/f0fd435fa32b56ae07a022dedbaec0cb156cdffd))
* **schema/ir:** introduce decimal type and improve schema validation ([8b8d3f2](https://github.com/asaidimu/go-anansi/commit/8b8d3f20443be6af8f863d66cef5a810eb29a80a))
* **schema:** add robust JSON schema validation for definitions ([b1967a5](https://github.com/asaidimu/go-anansi/commit/b1967a57a8f30db129c7f63e779f66434a59d03d))
* **schema:** enhance schema definitions and refine JSON serialization ([172613f](https://github.com/asaidimu/go-anansi/commit/172613fb4d6fc68bd8e0be3840d35208f36a0eca))
* **schema:** enhance validation engine with recursive schema support and MetaSchema ([5230c87](https://github.com/asaidimu/go-anansi/commit/5230c87f92272d30913f1f3b26d94fbc828b0568))
* **schema:** Introduce FieldSets for explicit conditional field definitions ([fcb3e22](https://github.com/asaidimu/go-anansi/commit/fcb3e22b8d7fc6c9e1b7d43fd086ca81888a1bdd))
* **schema:** Introduce schema modification utilities and factory methods ([34ba386](https://github.com/asaidimu/go-anansi/commit/34ba3868cab0f78c168173f0a2e39409d94ae0ea))
* **sqlite:** add SQLite persistence layer and query engine ([de5ffa6](https://github.com/asaidimu/go-anansi/commit/de5ffa68bd9d8ad521fcb33eaf0f99794228e1f0))


### BREAKING CHANGES

* All direct imports of "github.com/asaidimu/go-anansi/v6/core/document", "github.com/asaidimu/go-anansi/v6/core/schema/ir", and "github.com/asaidimu/go-anansi/v6/core/schema/validate" must be removed. The core persistence layer will be re-architected with new data structures and schema representation. Existing code relying on `document.Document` for data handling or `ir.Schema` for schema introspection will require significant rewriting.
* The method signatures for `document.Document.GetRecord`,
`SetRecord`, and `AppendRecord` have been updated.
The map value type for nested records has changed from `*document.Document` to `any`.
Direct consumers of these low-level `Document` methods will need to adjust their
code to handle `any` when interacting with nested records.
For example, `doc.GetRecord("path")` will now return `(map[string]any, bool, error)` instead of
`(map[string]*document.Document, bool, error)`.
* **core/document, schema:** This refactoring introduces breaking changes to the `core/document` and `core/schema/ir` packages.
- The `document.DataContainer` type has been removed; its functionality is now directly part of `document.Document`.
- Field identifiers throughout the `core/schema/ir` package, particularly in `ResolvedConstraints`, `ResolvedIndexes`, and `CompiledSchema.Store`, now use `document.DocumentKey` instead of `document.DataPoint`.
- The `ir.Predicate` function signature has changed from `func(data *document.DataContainer, fields []document.DataPoint, args any) bool` to `func(data *document.Document, fields []document.DocumentKey, args any) bool`.
- Any custom `ir.Predicate` implementations or direct interactions with `document.DataContainer` or `document.DataPoint` (when resolving schema fields) will need to be updated.
* **schema:** - The `data.SanitizationPersistence.Save` method signature has changed from `(ctx context.Context, scope string, config *data.FieldMaskConfig)` to `(ctx context.Context, config *data.FieldMaskConfig)`. The `data.FieldMaskConfig` struct must now embed the `Scope` field. Implementations of this interface need to be updated.
- The `core/schema.NestedSchemaFields.IsArray()` method was renamed to `IsLegacyFieldsArray()`. Direct calls to `IsArray()` will result in a compilation error. `IsFieldSets()` and `IsConditionalSets()` were added to provide explicit checks for the new structure.
- While `core/schema.NestedSchemaFields.FieldsArray` is maintained for backward compatibility, new conditional field definitions should utilize the `core/schema.NestedSchemaFields.FieldSets` map for improved clarity and addressability.
* The core error handling system has undergone a major overhaul.
- The common.Issue and common.SystemError internal structures have changed. Any direct access to Issue.Message, Issue.Path, SystemError.Message, SystemError.Code may behave differently or require adaptation to the new fluent API.
- The core/json.ValidationError type has been removed; validation errors from core/json/schema now return *common.SystemError directly.
- The return type of core/persistence/base.CreateResultSet.Issues() has changed from []CreateIssue to []common.Issue. Consumers of batch creation results must update their error handling logic.
- The API error response format in the example/api has been standardized to reflect the common.SystemError model, potentially impacting existing API clients that parse error details.
- Error codes and messages for common persistence and schema operations have been refined.
* - data.Document.AsMap() has been renamed to data.Document.ToMap(). Update all calls from document.AsMap() to document.ToMap().
- data.DocumentSlice is deprecated; use data.NewDocumentSet for document slice conversions.
- data.Document.Flatten() and data.Unflatten() methods have been removed. Re-implement any logic relying on these methods.
- The base.Collection.Update method signature has changed to return a *base.ReadResult, not an int. Adjust code calling this method to match the new return type.
- The query.DatabaseInteractor.UpdateDocuments method signature has changed to include a 'returning' boolean and return updated documents. Adjust code calling this method to match the new parameters and return types.
- base.ReadResult.Data type changed from []*data.Document to data.DocumentSet. Adjust code accessing this field.
- data.DocumentID constant renamed to data.DocumentIDField. Replace usages of data.DocumentID with data.DocumentIDField.
- schema.SchemaDefinition.MustAddIndex has been renamed to schema.SchemaDefinition.AddIndex and now returns the modified schema. Update calls accordingly.
- The CollectionUpdate struct now includes a ReturnDocument boolean field. Set this field as needed for update operations that require returning the modified documents.
* - The data.Document type is now a struct instead of map[string]any. Direct map access (doc["key"]) is deprecated; use doc.Get/MustGet/Set methods.
- core/data/context.go and its types (ContextualDocument, ContextBuilder) are removed. Use doc.WithContext(ctx) and doc.Context().
- base.Collection and base.Persistence interfaces now use *data.Document (or []*data.Document). Implementations must update.
- Document.DeepMerge modifies the receiver in place, no longer returning a new document.
- MaskRedact output changed from [REDACTED] to ***.
* - The data.Document type is now a struct instead of map[string]any. Direct map access (doc["key"]) is deprecated; use doc.Get/MustGet/Set methods.
- core/data/context.go and its types (ContextualDocument, ContextBuilder) are removed. Use doc.WithContext(ctx) and doc.Context().
- base.Collection and base.Persistence interfaces now use *data.Document (or []*data.Document). Implementations must update.
- Document.DeepMerge modifies the receiver in place, no longer returning a new document.
- MaskRedact output changed from [REDACTED] to ***.
* - Schema definitions using `schema.SchemaDefinition` or `schema.NestedSchemaDefinition` must be updated to align with the new discriminated union and `NestedSchemaFields` structures.
- The `IsStructured` field in `NestedSchemaDefinition` is removed.
- `StructuredFieldsMap` and `StructuredFieldsArray` in `NestedSchemaDefinition` are replaced by `Fields *NestedSchemaFields`.
- `schema.IndexDefinition` and `schema.Constraint` objects in schema definitions, field definitions, and migrations are now wrapped within `schema.IndexOrReference` and `schema.ConstraintRule` respectively.
- The `core/schema/codegen` package and its functionalities (e.g., `StructGenerator`) have been removed. Consumers relying on this package will need to adapt or implement alternative code generation.
- JSON payloads for schema changes and migrations will require updates due to structural modifications, including `SchemaChangeModifyPropertyPayload` and `SchemaChangeModifyConstraintPayload`'s new payloads.
* **core:** The error contract for almost all public functions returning errors has changed. Consumers who relied on `errors.Is` or `errors.As` with previous custom error types (e.g., `*DocumentError`, `*EphemeralError`, `*PersistenceError`, `*CollectionError`, `*RegistryError`, `*QueryError`, `*NativeError`, `*LexerError`, `*SchemaError`, `*CodegenError`) must now adapt their code to use `*common.SystemError`. Error identification should primarily be done using the `Code` field of `common.SystemError` or by checking for `*common.SystemError` with `errors.As`.
* 1. The core `core/query/FilterValue` type has been fundamentally changed from `any` to a structured union type. This affects the `query.QueryBuilder` API, direct usage of `query.QueryFilter`, and parameter signatures for custom Go-based query functions. All existing query constructions and custom function implementations that interact with `FilterValue` will require updates.
2. Several core QueryDSL data structures have been modified or replaced. This includes changes to `ProjectionComputedItem`, `CaseExpression`, `JoinConfiguration`, `AggregationConfiguration`, `QueryResult`, and `PaginationResult`. Review your code for direct usage of these types and update to match the new struct definitions.
3. The `persistence.NewPersistence` constructor now requires a `*zap.Logger` as its third argument. All initialization calls to `NewPersistence` across your application must be updated to provide a logger instance.
4. The `persistence.CollectionUpdate` struct now includes a `Version *int` field to facilitate optimistic concurrency control. While this addition does not typically break Go compilation for well-formed code, it introduces a new field that consumers should be aware of, particularly if performing struct literal initialization without named fields.
5. The project's license has changed from the MIT License to a Proprietary, Source-Available License. This is a fundamental change to the terms of use and distribution. All users must review and comply with the updated `LICENSE.md` file for details, especially concerning commercial use.
6. The `core/schema/NestedSchemaDefinition` field `isStructured` (internal) is now `IsStructured *bool` (exported), which can impact direct schema manipulation if previously accessing unexported fields via reflection.

# [6.0.0](https://github.com/asaidimu/go-anansi/compare/v5.1.0...v6.0.0) (2025-07-04)


* feat(persistence)!: overhaul schema and collection lifecycle management ([ecdff52](https://github.com/asaidimu/go-anansi/commit/ecdff52edf27de52945e8afc34bd54cda053b21b))


### BREAKING CHANGES

* The Migrate and Rollback methods have been removed from the PersistenceCollectionInterface and are now available on the PersistenceInterface with revised signatures. Additionally, the internal _schemas collection's structure has changed, introducing logical and physical name fields and directly embedding schema.SchemaDefinition, which may affect direct interactions with schema records.

# [5.1.0](https://github.com/asaidimu/go-anansi/compare/v5.0.1...v5.1.0) (2025-06-29)


### Features

* **query:** enhance QueryBuilder, DataProcessor and add comprehensive testing ([b826a23](https://github.com/asaidimu/go-anansi/commit/b826a233f5552b32a4f853d4d48cbdec7efd6950))

## [5.0.1](https://github.com/asaidimu/go-anansi/compare/v5.0.0...v5.0.1) (2025-06-29)


### Bug Fixes

* **module:** bump module path to v5 and update imports ([fb35199](https://github.com/asaidimu/go-anansi/commit/fb351993c106047b4e4c10131da4adbe370733b0))

# [5.0.0](https://github.com/asaidimu/go-anansi/compare/v4.0.0...v5.0.0) (2025-06-29)


* feat(module)!: update Go module path to v2 ([4569af7](https://github.com/asaidimu/go-anansi/commit/4569af78a00e00bd7baa14b3f20f7e485518c348))


### Bug Fixes

* **persistence:** refine internal logic and enhance documentation ([2b14fbc](https://github.com/asaidimu/go-anansi/commit/2b14fbc184ce5b17536b178953c381de0bc8b063))


### BREAKING CHANGES

* Consumers of this module must update their import paths
from github.com/asaidimu/go-anansi to github.com/asaidimu/go-anansi/v2
to ensure compatibility and continue receiving future updates.

# [4.0.0](https://github.com/asaidimu/go-anansi/compare/v3.0.0...v4.0.0) (2025-06-28)


* feat(core)!: streamline persistence API and introduce new examples ([a2cdb3a](https://github.com/asaidimu/go-anansi/commit/a2cdb3a922acb3b8d5826ee847ce4a75a421b3b0))


### BREAKING CHANGES

1. persistence.PersistenceInterface.Transaction() method has been removed. Use persistence.Persistence.Transact() helper for atomic operations.
2. persistence.PersistenceInterface.Metadata() and PersistenceTransactionInterface.Metadata() signatures have changed, removing includeCollections, includeSchemas, and forceRefresh boolean parameters.
3. persistence.NewPersistence() now returns persistence.PersistenceInterface instead of the concrete *persistence.Persistence type. Callers explicitly using *persistence.Persistence will need to update to use the interface.
4. Default DropIfExists option in sqlite.DefaultInteractorOptions() has been removed. Users relying on the explicit false default should now set DropIfExists: false in persistence.InteractorOptions if needed.

# [3.0.0](https://github.com/asaidimu/go-anansi/compare/v2.0.0...v3.0.0) (2025-06-28)


* feat(persistence)!: implement dynamic collection and subscription listing ([b5c2d83](https://github.com/asaidimu/go-anansi/commit/b5c2d83910fff121203b2dd860e5c3f5f85ded31))


### BREAKING CHANGES

* - The Persistence.Metadata method signature has changed: includeCollections and includeSchemas boolean parameters have been removed.
- The CollectionMetadata struct no longer includes LastModifiedAt and LastModifiedBy fields.
- The internal CollectionEvent struct has been removed, replaced by direct usage of PersistenceEvent.

# [2.0.0](https://github.com/asaidimu/go-anansi/compare/v1.0.0...v2.0.0) (2025-06-28)


* refactor(core)!: reorganize core package into query and schema modules ([af5a999](https://github.com/asaidimu/go-anansi/commit/af5a999dcd26c1a188d89717bdcccf1304f1ff01))


### BREAKING CHANGES

* The 'core' package has been refactored into 'core/query' and 'core/schema' for better modularity and separation of concerns.
- Schema definition types (e.g., core.SchemaDefinition, core.FieldDefinition, core.Issue, core.ValidationResult) are now found in 'github.com/asaidimu/go-anansi/core/schema'.
- Query-related types (e.g., core.QueryDSL, core.QueryFilter, core.QueryBuilder, core.ComparisonOperator, core.LogicalOperator, core.Document (now schema.Document), core.ComputeFunction, core.PredicateFunction, core.DataProcessor) are now found in 'github.com/asaidimu/go-anansi/core/query'.
- Top-level persistence interfaces and implementations (e.g., core.PersistenceInterface, core.PersistenceCollectionInterface, core.PersistenceEvent, core.RegisterSubscriptionOptions, core.CollectionUpdate) are now found in 'github.com/asaidimu/go-anansi/core/persistence'.
Consumers of the library must update their import paths to reflect these changes.

# 1.0.0 (2025-06-28)


* feat(anansi)!: Introduce Anansi Go persistence and query framework ([599abfa](https://github.com/asaidimu/go-anansi/commit/599abfa51cccc1dc1d6a56f3dc5aafdf0b33437a))
* feat(anansi)!: Introduce Anansi Go persistence and query framework ([3103006](https://github.com/asaidimu/go-anansi/commit/31030064bf964a1825b1d5e2680f4ae0f365c6d2))
* refactor(persistence)!: Formalize QueryDSL and QueryFilter parameters ([e0c54af](https://github.com/asaidimu/go-anansi/commit/e0c54af88f009a28c63c2e687fe48f5be5fd72ef))


### BREAKING CHANGES

* The PersistenceCollectionInterface Read, Update, and Delete method signatures have changed for improved type safety.
- Read now expects *core.QueryDSL instead of any.
- Delete now expects *core.QueryFilter instead of any.
- CollectionUpdate.Filter now expects *core.QueryFilter instead of any.

Client code implementing or directly calling these interfaces must update method signatures and argument types. For example, pass &queryBuilder.Build() instead of queryBuilder.Build().
* The Go module path has changed from github.com/asaidimu/persistence to github.com/asaidimu/go-anansi. Users must update their go.mod files to reflect the new import path.
* The Go module path has changed from github.com/asaidimu/persistence to github.com/asaidimu/go-anansi. Users must update their go.mod files to reflect the new import path.
