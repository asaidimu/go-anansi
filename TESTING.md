# SQLite Query Package Testing Plan

This document outlines the plan for improving the test coverage of the `sqlite/query` package.

## Current Test Coverage

The existing tests cover the basic functionality of the query builder, including:

- **SELECT** statements with simple and complex conditions, joins, and nested fields.
- **INSERT** statements with simple data.
- **UPDATE** statements with simple data and conditions.
- **DELETE** statements with simple conditions.
- Integration tests that verify the correctness of the generated queries against a real SQLite database.

## Tested Functionalities

The following functionalities have been successfully tested:

- **Builder State:** The query builder correctly resets its internal state between `Build` calls, preventing parameter contamination between queries.
- **WHERE Clause Operators:**
    - `IN` and `NOT IN` operators work as expected.
    - Basic comparison operators (`>`, `<=`, `=`) work correctly with different data types (integers, floats, booleans).
- **Multiple Joins:** The builder can correctly construct queries with multiple `JOIN` clauses (`INNER JOIN`, `LEFT JOIN`).
- **CASE Statements:** The builder can correctly construct `CASE...WHEN...ELSE...END` statements in the `SELECT` clause.

## Discovered Limitations

During testing, the following limitations were discovered in the query builder:

- The `FilterConditionBuilder` is missing methods for `Between`, `IsNull`, and `IsNotNull`.
- The `Custom` method on the `FilterConditionBuilder` does not support the `LIKE` and `NOT LIKE` operators.
- The API for `GroupBy` and `Aggregate` is not intuitive and caused significant difficulty during testing. Further investigation is needed to determine the correct usage.
- The query builder does not correctly handle the `SubqueryValue` type, treating it as a simple value instead of a subquery.
- The query builder converts all numeric types to `float64`, which may lead to precision issues.

## Proposed Tests

I propose to add the following tests to improve the robustness of the `sqlite/query` package:

### 1.  Builder Unit Tests (`builder_test.go`)

- **More Complex SELECT Statements:**
    - Test `SELECT` statements with multiple joins (INNER, LEFT, RIGHT).
    - Test `SELECT` statements with aggregate functions (COUNT, SUM, AVG, MIN, MAX).
    - Test `SELECT` statements with more complex `CASE` statements.
- **More data types in WHERE clauses:**
    - Test `WHERE` clauses with `IN` and `NOT IN` operators.
- **More Complex INSERT Statements:**
    - Test `INSERT` statements with complex data types (e.g., JSON objects, arrays).
    - Test bulk `INSERT` statements.
- **More Complex UPDATE Statements:**
    - Test `UPDATE` statements with complex data types (e.g., JSON objects, arrays).
    - Test `UPDATE` statements with multiple conditions.
- **More Complex DELETE Statements:**
    - Test `DELETE` statements with multiple conditions.
- **Error Handling:**
    - Test that the builder returns an error when given an invalid query.
    - Test that the builder returns an error when given invalid data.

### 2.  Integration Tests (`builder_integration_test.go`)

- **Replicate all new unit tests in an integration test:**
    - For each new unit test added to `builder_test.go`, add a corresponding integration test to `builder_integration_test.go` to verify the query executes correctly against a real SQLite database.
- **Test with a more complex database schema:**
    - Add more tables with different data types and relationships to the test database.
- **Test with a larger dataset:**
    - Add more data to the test database to test the performance of the generated queries.
- **Test transactions:**
    - Test that the query builder works correctly within a transaction.
    - Test that transactions are rolled back correctly on error.

## Implementation Plan

1.  Implement the new unit tests in `builder_test.go`.
2.  Implement the new integration tests in `builder_integration_test.go`.
3.  Run all tests to ensure that the new tests pass and that existing tests are not broken.
