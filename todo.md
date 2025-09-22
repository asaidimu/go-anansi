# Test Gap TODO List

This list prioritizes test gaps based on their potential impact on data integrity, correctness, and core functionality.

---

### High Importance

*These items address fundamental gaps in data integrity, transaction safety, and core feature correctness.*

#### Simple Complexity

1.  **Test Type Coercion with `nil` Values:**
    *   **File:** `tests/unit/data/type_coercion_test.go`
    *   **Details:** For each type-safe accessor (`GetString`, `GetInt`, etc.), add a test case where the document field's value is `nil`. Verify that the methods return a `ErrTypeMismatch` or another appropriate error instead of panics.

2.  **Test `GetNested` with Invalid Intermediate Path:**
    *   **File:** `tests/unit/data/document_operations_test.go`
    *   **Details:** Create a test that attempts to traverse a path where an intermediate segment is not a map or document (e.g., `doc.GetNested("a.b.c")` where `b` is an integer). Verify that it returns a `data.ErrCannotTraverse` error.

#### Moderate Complexity

1.  **Verify `Clone` Performs a Deep Copy:**
    *   **File:** `tests/unit/data/document_adv_test.go`
    *   **Details:** Create a document with nested maps and slices. Clone it. Modify a value within the nested map and a value in the nested slice of the *cloned* document. Assert that the original document remains unchanged.

2.  **Implement Array Indexing for `GetNested`:**
    *   **File:** `tests/unit/data/document_operations_test.go`
    *   **Details:** This is a feature gap. The `GetNested` function should be updated to parse and handle array indices in path strings (e.g., `users[0].name`). Add tests with valid indices, out-of-bounds indices, and non-array fields to ensure correctness and proper error handling (`ErrPathSegmentNotFound`, `ErrIndexOutOfRange`).

#### Complex Complexity

1.  **Test Transactional Rollback in Registry Operations:**
    *   **File:** `tests/unit/persistence/registry_test.go`
    *   **Details:** This requires mocking the `query.SchemaManager`. Create tests for `CreateCollection`, `DropCollection`, and `AddSchemaVersion` where the underlying physical operation (e.g., `schemaManager.CreateCollection`) is mocked to return an error. Verify that the registry transaction is rolled back and no changes are persisted in the `_schemas` collection.

2.  **Implement and Test Schema Migrations (`Migrate`, `Rollback`):**
    *   **File:** `tests/integration/persistence/migration_test.go` (new file)
    *   **Details:** This is a major feature gap. Create a comprehensive test suite for the schema migration and rollback functionality.
        *   Test a successful migration to a new version.
        *   Test a successful rollback to a previous version.
        *   Test a failing migration (e.g., due to a data transformation error) and verify that the schema version is not updated.
        *   Test the `dryRun` option for both `Migrate` and `Rollback` to ensure no actual changes are made.

---

### Medium Importance

*These items address more complex scenarios, improve robustness, and ensure the correctness of secondary features.*

#### Simple Complexity

1.  **Test `Document.Equals` Method:**
    *   **File:** `tests/unit/data/document_operations_test.go`
    *   **Details:** Add a dedicated test for the `Equals` method.
        *   Test two documents with the same keys and values but different key order (should be equal).
        *   Test two documents where a key has a different value (should be unequal).
        *   Test two documents where a key has a value of a different type (should be unequal).

2.  **Test `GetNested` with Special Characters in Keys:**
    *   **File:** `tests/unit/data/document_operations_test.go`
    *   **Details:** If the system design allows keys to contain dots (`.`), a mechanism to escape them in path notation is needed. Add a test to verify that `GetNested` can correctly retrieve values from keys containing special characters, assuming an escaping mechanism is implemented.

#### Moderate Complexity

1.  **Test `DeepMerge` with Complex Scenarios:**
    *   **File:** `tests/unit/data/document_adv_test.go`
    *   **Details:** Expand the tests for `DeepMerge`.
        *   Test merging documents with nested arrays (e.g., appending, overwriting).
        *   Test merging into an empty document and merging an empty document into a populated one.
        *   Test conflicts at various depths.

2.  **Test `Collection.Update` with `Recover` Option:**
    *   **File:** `tests/integration/persistence/collection_crud_test.go`
    *   **Details:** Create a test where an update operation would normally fail due to a schema validation error. Then, run the same update with the `Recover` flag set to `true` and verify that the invalid fields are stripped and the update succeeds.

3.  **Test Metadata Filtering:**
    *   **File:** `tests/integration/persistence/persistence_test.go`
    *   **Details:** Add a test for the `p.Metadata()` method that uses a `MetadataFilter`. Create several collections and then filter the metadata results to include only a subset of them, verifying the filter was applied correctly.

4.  **Test Registry Cache Behavior:**
    *   **File:** `tests/unit/persistence/registry_test.go`
    *   **Details:** Test the caching layer of the collection registry.
        *   **Cache Hit:** Call `GetRegistryEntry` twice for the same collection and mock the underlying database call to ensure it's only called once.
        *   **Cache Invalidation:** Call `InvalidateCache` and verify that the next call to `GetRegistryEntry` results in a database call.

---

### Low Importance

*These items are "nice-to-haves" that complete test coverage for utility functions and less critical edge cases.*

#### Simple Complexity

1.  **Test `AsMap` Recursive Conversion:**
    *   **File:** `tests/unit/data/document_operations_test.go`
    *   **Details:** Create a document with a nested `data.Document`. Call `AsMap()` on the parent document and verify that the nested document has been recursively converted to a standard `map[string]any`.

2.  **Test Utility Methods on Empty/Metadata-Only Documents:**
    *   **File:** `tests/unit/data/document_operations_test.go`
    *   **Details:** For methods like `Keys`, `Values`, `Len`, `IsEmpty`, and `HasKey`, add test cases that run against a completely empty document and a document that contains only the `_metadata` field.

3.  **Test `forceRefresh` on `Collection.Metadata`:**
    *   **File:** `tests/integration/persistence/collection_crud_test.go`
    *   **Details:** This would likely require mocking to be effective. Call `collection.Metadata` once, then call it again with `forceRefresh: true`. Verify that the underlying interactor method to fetch metadata is called both times.
