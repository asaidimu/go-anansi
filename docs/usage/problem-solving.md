# Problem Solving

# Problem Solving and Troubleshooting with Anansi

This section provides guidance on common issues encountered when using Anansi, along with diagnostic steps and potential solutions.

## Troubleshooting Common Issues

### 1. Database Connection Errors

*   **Symptom**: `Failed to open database connection: ...` or `Error looking up schema collection: ...`
*   **Check**: 
    *   Is the database file path correct and accessible by the application?
    *   Are there sufficient file system permissions?
    *   For SQLite, is the SQLite C library properly installed on your system? (See [Installation & Setup](#-installation--setup)).
*   **Fix**: Verify file paths and permissions. Ensure `libsqlite3` is installed and accessible to your Go build environment.

### 2. Schema Definition or Validation Errors

*   **Symptom**: `Failed to unmarshal user schema JSON: ...` or `Provided data does not conform to the collections schema`.
*   **Check**: 
    *   **JSON Unmarshaling**: Is your schema JSON string syntactically correct and does it match the `schema.SchemaDefinition` Go struct structure (case, field names)? Use a JSON linter.
    *   **Validation Issues**: When `collection.Validate` or `collection.Create` returns a validation error, inspect the `schema.ValidationResult.Issues` array for detailed error codes, messages, and paths (e.g., `REQUIRED_FIELD_MISSING`, `TYPE_MISMATCH`, `ENUM_VIOLATION`).
*   **Fix**: Correct JSON syntax. Adjust your data `map[string]any` to match the schema's `FieldType`, `Required`, `Unique`, `Values` (for enums) constraints.

### 3. Query Execution Errors

*   **Symptom**: `Failed to read data from collection ...`, `Failed to insert data ...`, etc., often with underlying SQL errors like `SQL logic error`.
*   **Check**: 
    *   **QueryDSL Correctness**: Is your `query.QueryDSL` structure valid? For complex queries, try simplifying the DSL to isolate the problematic part.
    *   **Data Types**: Are the data types in your query (`FilterValue`) consistent with the schema's `FieldType` in the database? Anansi attempts coercion, but mismatches can occur.
    *   **SQLite-Specific SQL**: If the underlying error hints at SQL syntax, remember that `json_extract` for nested fields requires correct JSON paths (e.g., `$.path.to.field`).
    *   **`RETURNING` Clause**: For `InsertDocuments` in SQLite, the `RETURNING *` clause requires SQLite version `3.35.0` or newer. An older version will cause a syntax error.
*   **Fix**: Review the `QueryDSL` construction. Ensure data types are compatible. Update SQLite to a newer version if `RETURNING` is failing.

### 4. Transaction Errors

*   **Symptom**: `Commit not applicable: not in a transactional context` or `Cannot start a new transaction from an existing transactional interactor`.
*   **Check**: 
    *   `Commit` and `Rollback` methods should *only* be called on the `DatabaseInteractor` instance returned by `StartTransaction`. 
    *   You cannot start a nested transaction from an already transactional `DatabaseInteractor` instance.
*   **Fix**: Ensure your transaction logic correctly manages the `DatabaseInteractor` instances. The original `interactor` remains non-transactional, while `StartTransaction` returns a new, transactional one.

### 5. Go-based Function Errors (Computed Fields, Custom Filters)

*   **Symptom**: Errors originating from within your `ComputeFunction` or `PredicateFunction`, often related to type assertions or nil pointers.
*   **Check**: 
    *   **Type Assertions**: Ensure you are safely asserting types of `row` values (which are `any` or `map[string]any`) and `args` (`FilterValue`) within your Go functions.
    *   **Registration**: Is the function name/operator correctly registered in the `schema.FunctionMap` when `persistence.NewPersistence` is called?
*   **Fix**: Implement robust error handling and type checking within your custom Go functions. Verify that the function is correctly registered.

## Error Reference

This section lists common error conditions and their related diagnostic paths and resolution strategies. For a complete list of specific error types, refer to the `reference.errors` section in the API documentation.

| Error Code / Type        | Symptom                                 | Diagnosis                                                         | Resolution                                                                    |
| :----------------------- | :-------------------------------------- | :---------------------------------------------------------------- | :---------------------------------------------------------------------------- |
| `Failed to open database` | Application crashes on startup or DB op. | Check `sql.Open` call, database path, file permissions.           | Correct DB path, ensure permissions, verify SQLite3 C library installation.   |
| `JSON Unmarshal Error`   | Schema or complex field unmarshaling fails. | Invalid JSON syntax or structure mismatch with Go struct.         | Validate JSON with linter, ensure Go struct fields match JSON keys/types.     |
| `REQUIRED_FIELD_MISSING` | Validation fails for missing data.      | A field marked `"required": true` in schema is absent.         | Provide the required field in the input data.                                 |
| `TYPE_MISMATCH`          | Validation fails for incorrect type.    | Data type does not match `schema.FieldType` (e.g., string for int). | Provide data of the correct type, or ensure Anansi's coercion handles it.      |
| `ENUM_VIOLATION`         | Validation fails for enum field.        | Value not in `"values"` array for `FieldTypeEnum`.                 | Provide a value from the allowed `values` list in the schema.                 |
| `UNIQUE_CONSTRAINT_VIOLATION` | Insert/Update fails due to uniqueness. | Data violates a `"unique": true` field or `IndexTypeUnique`.    | Provide a unique value for the constrained field.                             |
| `UNKNOWN_PREDICATE`      | Custom filter/computed field error.     | Go function name/operator not registered in `schema.FunctionMap`. | Register the custom function with `RegisterComputeFunction` or `RegisterFilterFunction`. |
| `INVALID_DATA_TYPE_FOR_CREATE` | `Create` method complains about input. | Input to `Create` is not `map[string]any` or `[]map[string]any`. | Ensure `Create` is called with the correct `map[string]any` or slice type.   |
| `TRANSACTION_CONTEXT_ERROR` | Commit/Rollback fails on wrong interactor. | Calling transactional methods on a non-transactional `DatabaseInteractor`. | Only call `Commit`/`Rollback` on the instance returned by `StartTransaction`. |
| `DELETE_WITHOUT_WHERE_CLAUSE` | Delete operation rejected for safety.   | Attempting `collection.Delete(filter, false)` with empty `filter`. | Provide a filter or set `unsafe` to `true` (use with extreme caution).      |
| `NESTED_SCHEMA_NOT_FOUND` | Validation of nested objects fails.     | `FieldSchema.ID` refers to a `NestedSchemaDefinition` not in `SchemaDefinition.NestedSchemas`. | Define the referenced `NestedSchemaDefinition` in the parent schema. |

For more detailed error information, consult the `reference.errors` section, which provides specific Go error types, properties, and propagation behavior.

---
### 🤖 AI Agent Guidance

```json
{
  "decisionPoints": [
    "IF database_error_encountered THEN consult_logging_and_error_reference ELSE debug_application_logic",
    "IF schema_validation_fails THEN analyze_validation_result_issues ELSE proceed_with_data_processing",
    "IF custom_go_function_error THEN verify_function_registration_and_internal_logic ELSE consult_anansi_core_logic",
    "IF transaction_fails THEN check_rollback_reason ELSE confirm_commit_status",
    "IF collection_operation_returns_unexpected_count THEN re_evaluate_query_filters_or_update_data ELSE proceed"
  ],
  "verificationSteps": [
    "Check: `os.Stderr` and `zap.Logger` output for detailed error messages and stack traces.",
    "Check: `schema.ValidationResult.Valid` and `schema.ValidationResult.Issues` for specific validation failures.",
    "Check: Database logs (e.g., SQLite trace) for executed SQL queries and database-level errors.",
    "Check: Return value of `Transact` function for `nil` (committed) or `error` (rolled back).",
    "Check: Input data types match expected `schema.FieldType` values, especially for nested JSON or `enum` types."
  ],
  "quickPatterns": [
    "Pattern: Logging Database Errors\n```go\n_, err := db.Exec(\"INSERT INTO ...\")\nif err != nil {\n    logger.Error(\"Database operation failed\", zap.Error(err))\n}\n```",
    "Pattern: Inspecting Validation Issues\n```go\nresult, _ := collection.Validate(data, false)\nfor _, issue := range result.Issues {\n    fmt.Printf(\"Validation Issue: %s (Path: %s)\\n\", issue.Message, issue.Path)\n}\n```",
    "Pattern: Debugging Transaction Rollback\n```go\nresult, err := persistenceSvc.Transact(func(tx persistence.PersistenceTransactionInterface) (any, error) { /* ... */ return nil, fmt.Errorf(\"simulated error\") })\nif err != nil {\n    fmt.Printf(\"Transaction rolled back due to: %v\\n\", err)\n}\n```",
    "Pattern: Verifying Data Types in Go Functions\n```go\nval, ok := row[\"my_field\"].(string)\nif !ok { return nil, fmt.Errorf(\"expected string\") }\n```"
  ],
  "diagnosticPaths": [
    "Error `Failed to open database connection` -> Symptom: Application cannot start or connect -> Check: `dbFileName` path, file permissions, `libsqlite3` availability -> Fix: Correct path/permissions, install `libsqlite3`.",
    "Error `Provided data does not conform to the collections schema` (from Validate/Create) -> Symptom: Data rejected -> Check: `validationResult.Issues` for `REQUIRED_FIELD_MISSING`, `TYPE_MISMATCH` etc. -> Fix: Adjust input data to match schema requirements.",
    "Error `failed to execute SELECT query` (from Read) -> Symptom: No data retrieved or SQL error logged -> Check: `QueryDSL` correctness, nested field paths (`json_extract` syntax), data existence -> Fix: Refine `QueryDSL`, ensure correct field access, verify data in DB.",
    "Error `Cannot start a new transaction from an existing transactional interactor` -> Symptom: Nested transaction attempt fails -> Check: `StartTransaction` creates a *new* transactional interactor, do not call it on an already transactional one -> Fix: Use the correct `DatabaseInteractor` instance (`tx` from `Transact` callback for nested operations).",
    "Error `unregistered Go compute function` -> Symptom: Computed field/custom filter fails -> Check: Function name in `AddComputed`/`Custom` matches key in `schema.FunctionMap`, and `FunctionMap` was passed to `NewPersistence` -> Fix: Correct registration or function name."
  ]
}
```

---
*Generated using Gemini AI on 6/28/2025, 10:32:05 PM. Review and refine as needed.*