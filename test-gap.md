# Test Gaps for Persistence Layer Changes (CreateCollections)

This document outlines potential test gaps and scenarios that should be explicitly covered to ensure the robustness and correctness of the newly introduced `CreateCollections` functionality and related refactorings.

## 1. `CreateCollections` - Success Scenarios

*   **Multiple Unique Schemas:**
    *   Test the creation of a batch containing 2, 5, and 10 distinct and valid schemas. Verify that all collections are successfully created, are accessible, and their registry entries are correct.
*   **Schemas with Different Versions (Initial Creation):**
    *   While `CreateCollections` is for initial creation, ensure that if a batch contains schemas that are logically related but have distinct names and initial versions (e.g., `user_v1`, `user_v2` as separate collections), they are handled correctly.

## 2. `CreateCollections` - Failure Scenarios (Transactional Rollback)

The core principle of `CreateCollections` is atomicity. If any part of the operation fails, the entire transaction should roll back, leaving no partial state.

*   **Invalid Schema in Batch:**
    *   Provide a batch where at least one schema definition is invalid (e.g., missing `Name`, invalid `FieldDefinition` type, circular references if schema validation supports it).
    *   **Expected:** The entire `CreateCollections` operation should fail, and *no* physical collections or registry entries from the batch should be created.
*   **Duplicate Schema Name/Version in Batch:**
    *   Provide a batch containing two or more schemas with the exact same `Name` and `Version`.
    *   **Expected:** The `prepareCollectionData` phase should detect this duplicate and return an error, preventing any database operations.
*   **Existing Collection Name in Batch:**
    *   Attempt to create a batch where one or more schema names already exist in the registry (i.e., a collection with that name has been previously created).
    *   **Expected:** The `prepareCollectionData` phase should detect the existing collection and return an error, preventing any database operations.
*   **Physical Name Collision in Batch:**
    *   If `generatePhysicalName` could, under specific circumstances, produce the same physical name for two different logical schemas within the same batch, this scenario should be tested. (This is highly dependent on the `generatePhysicalName` implementation, but worth considering).
    *   **Expected:** The `prepareCollectionData` phase should detect this conflict and return an error.
*   **Simulated Database Error during Physical Collection Creation:**
    *   Introduce a mock or controlled error condition that causes `manager.CreateCollection` to fail for one of the schemas during `createCollectionsInTransaction`.
    *   **Expected:** The transaction should roll back, and none of the collections in the batch (including those that would have succeeded) should be created.
*   **Simulated Database Error during Registry Entry Persistence:**
    *   Introduce a mock or controlled error condition that causes `collection.CreateOne` (for registry entry persistence) to fail for one of the schemas during `createCollectionsInTransaction`.
    *   **Expected:** The transaction should roll back, and none of the collections in the batch should be created.

## 3. `CreateCollections` - Edge Cases

*   **Empty Slice of Schemas:**
    *   Call `CreateCollections` with an empty `[]schema.SchemaDefinition{}`.
    *   **Expected:** It should return an empty slice of `RegistryEntry` and no error.
*   **Single Schema in Batch:**
    *   Call `CreateCollections` with a single schema in the slice (e.g., `CreateCollections([]{mySchema})`).
    *   **Expected:** It should behave identically to calling `CreateCollection(mySchema)`, ensuring consistency between the single and bulk creation methods.

## 4. Renamed Functions (`Create` to `CreateCollection`, `Collections` to `ListCollections`)

*   **Comprehensive Codebase Scan:**
    *   Perform a static analysis or search across the entire codebase (not just the modified files) to ensure all instances of the old function names (`Persistence.Create`, `BasePersistence.Collections`) have been correctly updated to their new counterparts (`Persistence.CreateCollection`, `BasePersistence.ListCollections`). This is more of a linting/refactoring verification.

## 5. Concurrency

*   **Concurrent `CreateCollections` Calls:**
    *   Design a test where multiple goroutines concurrently attempt to call `CreateCollections`, especially with schemas that might lead to conflicts (e.g., two goroutines trying to create a collection with the same name).
    *   **Expected:** The transactional nature should prevent data corruption, but the error messages and overall behavior under contention should be verified. This might require more sophisticated integration tests.

## 6. Performance (Non-Functional)

*   **Large Batch Performance:**
    *   While not a functional test gap, consider adding performance tests for `CreateCollections` with a very large number of schemas (e.g., 100, 1000). This would help identify potential bottlenecks related to the number of DDL operations or transaction size.
