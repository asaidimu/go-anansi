# Go-Anansi Production Readiness Roadmap - July 15, 2025

This roadmap outlines the critical steps required to bring `go-anansi` to a production-ready state by July 15, 2025. The focus is on addressing identified test failures and conducting a comprehensive code review.

## 1. Test Failures Analysis and Prioritization

The recent test run (`go test -v ./...`) revealed several critical failures across `ephemeral` and `persistence` modules. These must be addressed to ensure stability and correctness.

### 1.1. `tests/unit/ephemeral` Failures

The `InMemoryInteractor` tests are failing, indicating issues with the in-memory database implementation.

*   **`TestInMemoryInteractor_SelectDocuments` (Contains/NotContains filters):**
    *   **Issue:** Filters are returning incorrect counts and/or wrong data. For "Contains", it returned 2 items instead of 1, and "Alice" instead of "Charlie". For "NotContains", it returned 2 items instead of 3.
    *   **Potential Cause:** The `evaluateCondition` logic for `Contains` and `NotContains` operators, particularly the `fmt.Sprintf("%v", fieldValue)` conversion, might be causing unexpected string representations for non-string data types, leading to incorrect matches.
*   **`TestInMemoryInteractor_UpdateDocuments`:**
    *   **Issue:** Incorrect number of documents updated (expected 2, got 1). Field values are not being updated correctly (e.g., `age` not updated, `city` missing).
    *   **Potential Cause:** The update logic in `UpdateDocuments` might not be correctly identifying all matching documents or applying updates to all fields as intended. The `maps.Copy` operation might not be behaving as expected for all scenarios.
*   **`TestInMemoryInteractor_DeleteDocuments`:**
    *   **Issue:** Incorrect number of documents deleted (expected 2, got 1). Remaining document counts are incorrect.
    *   **Potential Cause:** Similar to `UpdateDocuments`, the deletion logic might not be correctly identifying documents for deletion or managing the remaining documents in the collection.

### 1.2. `tests/unit/persistence` Failures

These failures point to deeper issues within the core persistence layer, including error handling, data structure consistency, and mock interactions.

*   **`TestCollectionBase_Create/Insert_Failed`:**
    *   **Issue:** Error message mismatch: Expected "Failed to insert data into collection", got "db error".
    *   **Potential Cause:** Inconsistent error message handling or propagation from the underlying database interactor.
*   **`TestCollectionBase_Read/Successful_Read`:**
    *   **Issue:** Data structure mismatch: `QueryResult.Data` expected to be `[]schema.Document` but received `schema.Document`.
    *   **Potential Cause:** The `Read` method or its underlying components are returning a single `schema.Document` instead of a slice containing it, violating the expected return type.
*   **`TestCollectionBase_Read/No_Documents_Found`:**
    *   **Issue:** Data structure mismatch: `QueryResult.Data` expected `[]schema.Document{}` but received `[]schema.Document(nil)`.
    *   **Potential Cause:** Similar to the above, an empty slice is expected, but a `nil` slice is returned. While functionally similar, the test expects an initialized empty slice.
*   **`TestCollectionBase_Update/Successful_Update_with_Optimistic_Locking`:**
    *   **Issue:** Return value mismatch: Expected `int(2)`, got `<nil>`.
    *   **Potential Cause:** The optimistic locking mechanism or the `UpdateDocuments` method is not correctly returning the count of updated documents, or there's a type assertion issue.
*   **`TestCollectionBase_Validate/Validation_Failed_-_Missing_Required_Field`:**
    *   **Issue:** Error message mismatch: Expected "name is required", got "Required field 'name' is missing".
    *   **Potential Cause:** Inconsistent error message generation from the validation logic.
*   **`TestPersistence_Collection/Successful_Collection_Retrieval`:**
    *   **Issue:** Panic due to unexpected mock call: `SelectDocuments` called more times than expected.
    *   **Potential Cause:** This is a critical issue. It suggests a logic error in how `Persistence.Collection` interacts with its `DatabaseInteractor` mock, possibly calling `SelectDocuments` multiple times when only one call is mocked, or with unexpected arguments. This could indicate a deeper architectural or usage pattern issue with mocks.

## 2. Actionable Roadmap

To achieve production readiness by July 15th, the following actions are prioritized:

### Phase 1: Stabilize Core Functionality (Estimated Completion: July 11th)

*   **2.1. Fix `tests/unit/persistence` Critical Failures:**
    *   **2.1.1. Address `TestPersistence_Collection` Panic:** Investigate and resolve the unexpected mock call in `Persistence.Collection`. This is paramount as it indicates a fundamental issue in how the persistence layer interacts with its dependencies.
    *   **2.1.2. Resolve `TestCollectionBase_Read` Data Structure Mismatches:** Ensure `QueryResult.Data` consistently returns `[]schema.Document{}` for empty results and `[]schema.Document` for single results, as expected by tests.
    *   **2.1.3. Correct `TestCollectionBase_Update` Optimistic Locking Return:** Ensure the `UpdateDocuments` method correctly returns the count of updated documents, especially with optimistic locking.
*   **2.2. Fix `tests/unit/ephemeral` Core Logic:**
    *   **2.2.1. Refine `SelectDocuments` Filter Logic (Contains/NotContains):** Review and correct the `evaluateCondition` logic for `Contains` and `NotContains` operators, paying close attention to type handling and string conversions to ensure accurate filtering.
    *   **2.2.2. Debug `UpdateDocuments` and `DeleteDocuments` Logic:** Thoroughly review the update and delete mechanisms in `InMemoryInteractor` to ensure correct document identification, modification, and removal, as well as accurate count reporting.

### Phase 2: Refine Error Handling and Code Review (Estimated Completion: July 13th)

*   **2.3. Standardize Error Messages:**
    *   **2.3.1. Harmonize Error Messages in `persistence` and `schema`:** Ensure consistent and user-friendly error messages across the `persistence` and `schema` modules, specifically addressing the mismatches found in `TestCollectionBase_Create/Insert_Failed` and `TestCollectionBase_Validate/Validation_Failed_-_Missing_Required_Field`.
*   **2.4. Comprehensive Code Review:**
    *   **2.4.1. Review `core/persistence`:** Focus on `collection.go`, `executor.go`, `persistence.go`, and `registry.go` for best practices, error handling, concurrency safety (especially around `sync.Mutex` usage), and adherence to Go idioms.
    *   **2.4.2. Review `core/ephemeral`:** Examine `interactor.go` for efficiency, correctness of filter evaluations, and data manipulation.
    *   **2.4.3. Review `core/query` and `core/schema`:** Ensure the query DSL and schema definition/validation logic are robust, extensible, and correctly implemented.
    *   **2.4.4. Review `sqlite` module:** Assess `interactor.go`, `mapper.go`, and `query.go` for correct SQL generation, database interactions, and error handling.

### Phase 3: Final Verification and Documentation (Estimated Completion: July 15th)

*   **2.5. Re-run All Tests:** After all fixes and code review changes, run the complete test suite (`go test -v ./...`) to ensure all tests pass and no regressions have been introduced.
*   **2.6. Performance Benchmarking (If applicable):** If there are existing benchmarks, run them to identify any performance regressions. If not, consider adding basic benchmarks for critical paths.
*   **2.7. Update Documentation:**
    *   **2.7.1. Review and update `README.md` and `docs/`:** Ensure all documentation reflects the current state of the codebase, including any new features, breaking changes, or important usage notes.
    *   **2.7.2. Add `CHANGELOG.md` entries:** Document all changes made during this production readiness effort.

This roadmap provides a structured approach to achieving production readiness. Each step is actionable and prioritized to address the most critical issues first.
