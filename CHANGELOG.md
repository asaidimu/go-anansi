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
