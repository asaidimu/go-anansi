# TODO: Failing Tests and Architectural Refactoring (Revised)

This document outlines the currently failing tests, an analysis of why they are failing, and a proposed architectural shift to address the underlying issues.

## Failing Tests

### 1. `TestPersistence_Transact`
- **File:** `tests/integration/persistence/persistence_test.go:254`
- **Error:** `Received unexpected error: update operation requires a valid metadata block, found, 
 {
 "balance": 80,
 "id": "A"
}`
- **What it tests:** This test verifies that the persistence layer can correctly handle a series of operations within a transaction.
- **How it fails:** The test fails during an update operation within a transaction. The error message indicates that the `_metadata_` field, which is required for optimistic locking and other internal consistency checks, is missing from the update payload. The document is read from the database (and thus contains the metadata), but the `Update` function in the `collection` package (or one of its decorators) is stripping this metadata before passing the operation to the database interactor.

### 2. `TestInsert_Integration` & `TestComplexTypes_Integration`
- **File:** `tests/integration/sqlite/dal_test.go:121` and `tests/integration/sqlite/dal_test.go:242`
- **Error:** `Received unexpected error: near ")": syntax error`
- **What it tests:** These tests check the integration of the Data Access Layer (DAL) with the SQLite backend for insert operations.
- **How it fails:** The tests fail due to a SQL syntax error. The `SQLiteInsertValues.Value()` method in `sqlite/query/insert.go` does not initialize the `fields` slice when a schema is present. This results in an empty column list in the generated `INSERT` statement (e.g., `INSERT INTO my_table () VALUES (...)`), which is a syntax error.

### 4. `TestSelect`
- **File:** `tests/unit/sqlite/builder_test.go:37`
- **Error:** `Received unexpected error: failed to get schema from query: query target schema is required`
- **Panic:** `runtime error: invalid memory address or nil pointer dereference`
- **What it tests:** This unit test checks the functionality of the SQLite query builder for SELECT statements.
- **How it fails:** The test fails because it does not provide a schema to the query builder. The `SchemaFromQuery` function, which is called by the builder, requires a schema to be present in the query target. The subsequent panic is a result of the test attempting to access the `Raw()` method on a `nil` query object that was returned due to the error.

## Analysis of Failures

The failing tests point to a few key issues:

1.  **Incorrect State Handling in the Persistence Layer:** The `TestPersistence_Transact` failure shows that the `_metadata_` field is not being correctly preserved during an update operation. This is a critical bug that breaks optimistic locking and data consistency.

2.  **Bugs in the SQLite Query Builder:** The `INSERT` failures are due to a clear bug in the query builder where it fails to generate a valid query when a schema is provided.

3.  **Lack of Robustness in the Query Building Process:** The `TestSelect` failure, while technically a test issue, highlights a lack of robustness in the query building process. The system should be more resilient to missing schemas, perhaps by fetching them from the schema registry, or at least by failing with a more informative error.

## Proposed Architectural Shift

While the query builder and executor are decoupled at the interface level, the implementation of the query building process is fragile. To address these issues and make the system more robust, I propose the following architectural refinements:

### 1. Introduce a `UnitOfWork` Pattern

-   **Problem:** The current transaction management is implicit and allows for bugs like the metadata stripping issue to occur.
-   **Solution:** I will introduce a `UnitOfWork` pattern to explicitly manage all the operations within a transaction. The `UnitOfWork` will be responsible for:
    -   Tracking all created, updated, and deleted documents.
    -   Enforcing persistence-layer rules, such as ensuring that all updated documents have a valid `_metadata_` field. This will be done by centralizing the logic for handling document state within the `UnitOfWork`.
    -   Orchestrating the commit of the transaction, which will involve calling the appropriate DAL methods.
-   **Benefits:** This will make the transaction management more explicit and robust, preventing entire classes of bugs related to state management within a transaction.

### 2. Improve the Robustness of the Query Building Process

-   **Problem:** The query builder has bugs and is not resilient to missing information.
-   **Solution:** I will refactor the `sqliteFactory` to be more robust:
    -   **Fix the `INSERT` bug:** The `SQLiteInsertValues.Value()` method will be fixed to correctly initialize the `fields` slice when a schema is provided.
    -   **Improve Schema Handling:** The query builder will be enhanced to fetch a schema from the `registry` if one is not explicitly provided in the query. This will make the builder easier to use and prevent errors like the one in `TestSelect`.
    -   **Add More Validation:** The builder will include more validation checks to catch errors early, such as the empty column list in the `INSERT` statement, and provide more informative error messages.
-   **Benefits:** This will result in a more robust, reliable, and easier-to-use query building process.

By implementing these changes, I will not only fix the currently failing tests but also create a more robust, maintainable, and extensible persistence layer for `go-anansi`.
