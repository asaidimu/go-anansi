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
