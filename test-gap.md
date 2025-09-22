# Comprehensive Test Gaps for go-anansi

This document outlines potential test gaps across the `go-anansi` codebase, identified through a high-level code review. The goal is to ensure robustness, correctness, and coverage of various functionalities, including edge cases, error handling, and interactions between components.

## 1. `anansi.go` (Setup and Initialization)

*   **Error Propagation:**
    *   Verify that `Setup` correctly returns errors originating from `data.ConfigureDocumentFactory`, `persistence.NewPersistence`, or `p.CreateCollections`.
*   **Idempotency:**
    *   Test calling `Setup` multiple times. Ensure the `persistenceInstance` remains the same and no unexpected side effects or re-initializations occur on subsequent calls.
*   **Empty Schemas:**
    *   Test `Setup` with an empty `config.Schemas` slice. Ensure it initializes correctly without attempting to create collections.
*   **Nil Dependencies:**
    *   Test `Setup` with `nil` values for `config.Interactor`, `config.Logger`, or `config.FactoryConfig`. Verify that appropriate errors are returned or panics are handled gracefully by the respective components.

## 2. `core/data/document.go` (Document Operations)

*   **`getValueByPath` and `getNestedValue` (Dot-Notation Access):**
    *   **Complex Paths (Array Indexing):** Test with paths involving array indexing (e.g., `array[0].field`, `array[index].nested.field`). The current implementation appears to only handle `map[string]any` and `Document` types for traversal, not array indexing. This is a significant gap if array traversal is expected. **[Status: Not Covered]**
    *   **Invalid Path Segments:** Test with paths where an intermediate segment is not a map/document (e.g., `a.b.c` where `b` is an integer). `document_operations_test.go` tests this for `SetNested`, but not for `GetNested`. **[Status: Partially Covered]**
    *   **Paths with Special Characters:** If keys can contain dots, how are they handled? This is not tested. **[Status: Not Covered]**

*   **Type Coercion Methods (`GetString`, `GetInt`, `GetFloat64`, `GetBool`, `GetTime`, `GetDocument`, `GetDocumentArray`):**
    *   **`nil` values:** Test coercion of `nil` values for all `GetX` methods. `type_coercion_test.go` has good coverage, but `nil` values are not explicitly tested. **[Status: Partially Covered]**

*   **`Clone` and `DeepMerge`:**
    *   **Deep Copy Verification:** Ensure that `Clone` truly creates a deep copy, meaning modifications to the cloned document's nested maps/documents/slices do not affect the original. `document_operations_test.go` uses `Clone`, but a specific test to verify the deep copy is missing. **[Status: Not Covered]**
    *   **Complex Merges:** Test `DeepMerge` with documents containing nested arrays, mixed types, and conflicts at various depths. `document_adv_test.go` has a basic test for `DeepMerge`. More complex scenarios are not tested. **[Status: Partially Covered]**
    *   **Empty Documents:** Test merging with empty documents. **[Status: Not Covered]**

*   **Utility Methods (`Keys`, `Values`, `Len`, `IsEmpty`, `HasKey`, `HasPath`, `Equals`, `AsMap`, `Metadata`, `StripMetadata`):**
    *   **`Equals`:** Test with documents having same content but different order of keys (should be equal). Test with documents having different types for same key (should be unequal). There are no explicit tests for `Equals`. **[Status: Not Covered]**
    *   **`AsMap`:** Ensure recursive conversion of nested `Document` types and `map[string]any` into standard `map[string]any`. There are no specific tests for `AsMap`. **[Status: Not Covered]**
    *   **Edge Cases:** Test utility methods with empty documents, and documents with only metadata. **[Status: Partially Covered]**

## 3. `core/persistence/base/types.go` (Interfaces and Eventing)

*   **All Interface Implementations:**
    *   **Complete Coverage:** While there is good coverage for `Persistence` and `Collection` interfaces, a systematic check against every method is needed. **[Status: Partially Covered]**
    *   **Error Paths:** Many error paths are tested (e.g., not found, invalid input). However, more tests are needed for errors originating from the database interactor. **[Status: Partially Covered]**
    *   **Edge Cases:** Some edge cases are tested (e.g., empty database). More tests for empty collections and `nil` inputs are needed. **[Status: Partially Covered]**

*   **Event Emission:**
    *   **All Event Types:** `persistence_events_test.go` confirms that events are emitted for document, collection, and transaction operations. **[Status: Covered]**
    *   **Event Data Accuracy:** The tests confirm that events are received, but they do not deeply inspect the `PersistenceEvent` struct to ensure all its fields are correctly populated. **[Status: Partially Covered]**
    *   **Subscription Functionality:** `persistence_test.go` and `collection_test.go` cover subscription and unsubscription. **[Status: Covered]**

*   **`Transact` Method:**
    *   **Commit/Rollback:** `persistence_test.go` and `cart_simulation_test.go` have good tests for commit and rollback. **[Status: Covered]**
    *   **Nested Transactions:** `transaction_test.go` and `persistence_test.go` cover nested transactions. **[Status: Covered]**
    *   **Concurrency:** `concurrent_transactions_test.go` covers concurrent transactions. **[Status: Covered]**
    *   **Panic Handling:** `persistence_test.go` has a test for panic handling. **[Status: Covered]**

*   **Schema Management (`Migrate`, `Rollback`):**
    *   **Success Paths:** No tests exist for `Migrate` and `Rollback`. **[Status: Not Covered]**
    *   **Failure Paths:** No tests exist for `Migrate` and `Rollback`. **[Status: Not Covered]**
    *   **Dry Run:** No tests exist for `Migrate` and `Rollback`. **[Status: Not Covered]**
    *   **Version Management:** No tests exist for `Migrate` and `Rollback`. **[Status: Not Covered]**
    *   **Data Transformation:** No tests exist for `Migrate` and `Rollback`. **[Status: Not Covered]**

*   **Metadata (`Metadata`, `CollectionMetadata`):**
    *   **Filtering:** No tests exist for `Metadata` with `MetadataFilter`. **[Status: Not Covered]**
    *   **Data Accuracy:** `persistence_test.go` has basic tests for metadata accuracy. **[Status: Partially Covered]**
    *   **Force Refresh:** No tests exist for `Collection.Metadata` with `forceRefresh`. **[Status: Not Covered]**

*   **`Collection` Interface Methods (CRUD, Validate, Capabilities):**
    *   **`CreateOne`/`CreateMany`:** `collection_crud_test.go` and `collection_test.go` have good coverage. **[Status: Covered]**
    *   **`Read`:** `collection_crud_test.go` and `collection_test.go` have good coverage. **[Status: Covered]**
    *   **`Update`:** `collection_crud_test.go` and `collection_test.go` have good coverage, but `CollectionUpdate.Recover` is not tested. **[Status: Partially Covered]**
    *   **`Delete`:** `collection_crud_test.go` and `collection_test.go` have good coverage, including the `unsafe` flag. **[Status: Covered]**
    *   **`Validate`:** `collection_test.go` has good coverage, including the `loose` option. **[Status: Covered]**
    *   **`Capabilities`:** `collection_test.go` has a basic test for `Capabilities`. **[Status: Partially Covered]**

## 4. `core/persistence/registry/registry.go` (Collection Registry)

*   **`CreateCollections` (from previous review):**
    *   **Success Scenarios:** Test with multiple unique schemas, and schemas with different initial versions.
    *   **Failure Scenarios (Transactional Rollback):** Test with invalid schemas in batch, duplicate schema name/version in batch, existing collection name in batch, physical name collision in batch, simulated database errors during physical creation or registry persistence. Verify complete rollback.
    *   **Edge Cases:** Test with an empty slice of schemas, and a single schema in the batch.
*   **`DropCollection`:**
    *   **Non-existent Collection:** Test dropping a collection that doesn't exist.
    *   **`DeletePhysicalData`:** Test with `true` (verify physical collections dropped) and `false` (verify only registry entry removed).
    *   **Error during Physical Drop/Registry Delete:** Simulate errors and verify transaction rollback.
*   **`PruneVersion`:**
    *   **Non-existent Collection/Version:** Test pruning for non-existent entities.
    *   **Pruning Active Version:** Explicitly test attempting to prune the active version and verify the expected error.
    *   **Error during Physical Drop/Registry Update:** Simulate errors and verify transaction rollback.
*   **`AddSchemaVersion`:**
    *   **Invalid Schema:** Test adding a new version with an invalid schema definition.
    *   **Existing Version:** Test adding a version that already exists.
    *   **Error during Physical Creation/Registry Update:** Simulate errors and verify transaction rollback.
    *   **Physical Name Collision:** Test if generated physical name conflicts with an existing one.
*   **`SetActiveVersion`:**
    *   **Non-existent Collection/Version:** Test setting active version for non-existent entities.
    *   **Already Active Version:** Test setting the active version to the one that is already active.
    *   **Error during Registry Update:** Simulate errors and verify transaction rollback.
*   **`GetRegistryEntry`:**
    *   **Cache Hit/Miss:** Explicitly test scenarios where the entry is in cache and where it's not.
    *   **Database Error:** Simulate a database error during `loadFromDatabase`.
*   **`List`:**
    *   **Empty Registry:** Test when no collections are registered.
    *   **Mixed Cache/DB:** Test scenarios where some entries are in cache and some are not.
    *   **Database Error:** Simulate a database error during `loadAllFromDatabase`.
*   **Cache Management (`InvalidateCache`, `RefreshCache`, etc.):**
    *   **`InvalidateCache`:** Test invalidating a specific entry and all entries.
    *   **`RefreshCache`:** Test manual refresh.
    *   **Background Refresh:** Ensure the goroutine starts and stops correctly.
    *   **Concurrency with Cache:** Test concurrent reads/writes to the cache for thread safety.

## 5. `core/query/dsl.go` (Query DSL Structure)

*   **`FilterValue`:** Test queries with `FilterValue` containing each of its possible types, including nested `ArrayVal` and complex `FunctionCall` arguments.
*   **`QueryFilter`:** Test all combinations (`Condition`, `Group`, `TextSearchQuery`), deeply nested `FilterGroup`s, all `ComparisonOperator`s with various data types, `FieldReference`, `SubqueryValue`, and `FunctionCall`.
*   **`TextSearchQuery`:** Test all `TextSearchType`s, `TextOperator`s, `CaseSensitive` options, and empty/nil `Fields`.
*   **`SortConfiguration`:** Test multiple sort fields with mixed directions, and non-existent fields.
*   **`PaginationOptions`:** Test various combinations of `Limit` and `Offset`, including zero, negative, and very large values.
*   **`ProjectionConfiguration`:** Test `Include`/`Exclude` combinations, nested projections, non-existent fields, `ComputedFieldExpression` (with `FunctionCall`), and `CaseExpression`.
*   **`JoinConfiguration`:** Test all `JoinType`s, complex `On` clauses, multiple joins, self-joins, and non-existent targets/fields.
*   **`AggregationConfiguration`:** Test all `AggregationType`s, `Groups`, `Filter`, and non-existent fields.
*   **`QueryUnion`:** Test all `Type`s, multiple queries, and conflicting projections/sorts.
*   **`QueryDistinctConfig`:** Test `IsDistinct` (boolean) and `Fields` (distinct on specific columns), and combinations with other query features.
*   **`QueryHint`:** Ensure hints don't cause errors if unsupported by the executor.
*   **`IsEmpty`:** Test with truly empty and partially defined queries.

## 6. `core/query/engine.go` (Query Execution Engine)

*   **Query Partitioning (`e.partitioner.Partition(dsl)`):**
    *   **All DSL Features:** Test how *every* feature of the `Query` DSL is partitioned (fully database-executable, fully in-memory, mixed partitioning).
    *   **Unsupported Features:** Test queries containing features that `Interactor.Capabilities()` explicitly states are *not* supported. Verify correct movement to `postProcessingQuery`.
    *   **Complex Combinations:** Test queries with multiple features (e.g., filter + sort + projection + aggregation + join).
*   **Cache (`QueryCache`):**
    *   **Cache Hit/Miss:** Test scenarios where queries hit and miss the cache.
    *   **Cache Invalidation/Eviction:** Test behavior when cache is full.
    *   **Cache Key Generation:** Ensure consistent keys for identical queries and different keys for different queries.
*   **`Interactor.SelectDocuments`:**
    *   **Error Handling:** Simulate errors from the `BaseDatabaseInteractor`.
    *   **Empty Results:** Test when `SelectDocuments` returns an empty list.
*   **`QueryHelper` Interaction (`runPostProcessing`):**
    *   **Order of Operations:** Verify correct order of in-memory processing steps (filter, join, aggregation, sort, paginate).
    *   **Error Propagation:** Simulate errors from `QueryHelper` methods.
    *   **Projection Application:** Ensure the `dsl.Projection` is correctly applied as the *final* step.
*   **Custom Functions (`RegisterComputeFunction`, `RegisterFilterFunction`):**
    *   **Registration:** Test registering functions with valid/invalid names.
    *   **Execution:** Test queries that utilize these registered custom functions.
    *   **Error Handling:** Test custom functions that return errors.
*   **Edge Cases:** Test with empty DSL, nil `schemaDef`.

## 7. `sqlite/executor/executor.go` (SQLite Executor)

*   **`Query` and `Exec`:**
    *   **Successful Execution:** Test with various valid SQL queries (SELECT, INSERT, UPDATE, DELETE, DDL) and parameters.
    *   **Error Handling:** Test with invalid SQL, database connection errors, constraint violations, and permissions errors.
    *   **Empty Results:** Test `Query` when no rows are returned.
    *   **`RowsAffected`:** For `Exec`, verify the correct `RowsAffected` count.
*   **`QueryStream`:**
    *   **Successful Streaming:** Test with queries returning small, medium, and large numbers of rows.
    *   **Error during Query/Row Reading:** Simulate errors and verify `errChan` receives them.
    *   **Context Cancellation:** Test cancelling the `ctx` during streaming.
    *   **Empty Result Set:** Test streaming for a query that returns no rows.
*   **Transaction Management (`BeginTransaction`, `Commit`, `Rollback`):**
    *   **Successful Transaction:** Begin -> Exec/Query -> Commit. Verify changes are persisted.
    *   **Rollback:** Begin -> Exec/Query -> Rollback. Verify changes are *not* persisted.
    *   **Nested Transactions:** Test calling `BeginTransaction` when already in a transaction.
    *   **Commit/Rollback without Transaction:** Test calling `Commit` or `Rollback` when not in a transaction.
    *   **Error during Commit/Rollback:** Simulate database errors.
    *   **Context Cancellation:** Test cancelling the `ctx` during transaction operations.
*   **`Close`:**
    *   Test closing non-transactional and transactional executors. Verify implicit rollback for transactional ones.
    *   Test calling `Close` multiple times.

## 8. `sqlite/query/builder.go` (SQLite Query Builder)

*   **`Build` Method (Overall Dispatch and Error Handling):**
    *   **Unsupported `StatementType`:** Test with an unsupported `native.StatementType`.
    *   **Error Propagation:** Ensure errors from `buildXTree` methods are correctly propagated.
*   **`buildSelectTree` (and all other `buildXTree` methods):**
    *   **Comprehensive DSL Coverage:** For *each* `buildXTree` method, test every possible combination and edge case of the `query.Query` DSL that it's expected to handle (filters, sort, pagination, projection, joins, distinct, aggregations, union, hints).
    *   **SQL Injection Prevention:** Crucially, ensure that the generated SQL is always parameterized and that no direct string concatenation of user-provided values occurs in a way that could lead to SQL injection.
    *   **Schema Mapping:** Verify correct mapping of DSL field names to actual column names.
    *   **Type Conversion:** Ensure values from the DSL are correctly converted to SQLite-compatible types and parameters are bound correctly.
    *   **Edge Cases:** Test empty queries, queries with only one clause, and queries with very large numbers of filters/joins/parameters.
*   **`buildCreateTableTree` and `buildDropTableTree`:**
    *   **Schema Definition:** Test with various `schema.SchemaDefinition` types (different field types, constraints, indexes).
    *   **Primary Keys, Indexes, Constraints:** Verify correct translation into `CREATE TABLE` and `CREATE INDEX` statements.
    *   **`IF NOT EXISTS`/`IF EXISTS`:** Ensure these are used where appropriate.
*   **`buildCreateIndexTree` and `buildDropIndexTree`:**
    *   Test creating and dropping various types of indexes.
*   **Parameterization (`paramCounter`, `aliases`):**
    *   Verify correct incrementing of `paramCounter` and unique parameter generation.
    *   Verify correct management of `aliases` for complex queries.