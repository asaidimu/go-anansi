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
