# Schema Package Refactoring Plan - Phase 1: Relocate Document Validator

## Objective

Relocate the document validation logic from `core/schema/validator.go` to a new sub-package `core/schema/validator`. The primary goals are to improve modularity and organization within the `schema` package while **maintaining the existing public API** and ensuring **all unit and integration tests continue to pass**.

## Current State

The main document validation logic resides in `core/schema/validator.go`. Its primary exported function is `schema.NewDocumentValidator`.

## Proposed New Structure (Partial)

```
core/schema/
├── api.go             // New: Facade to maintain original public API for validator
├── definition.go      // Unchanged for now
├── errors.go          // Unchanged
├── json.go            // Unchanged
├── semantics.go       // Unchanged
├── utils.go           // Unchanged
└── validator/         // NEW: Sub-package for document validation logic
    └── document.go    // Moved and renamed from core/schema/validator.go
```

## Steps

1.  **Create `core/schema/validator` directory:**
    *   Create the new subdirectory: `mkdir -p core/schema/validator`

2.  **Move and Rename `validator.go`:**
    *   Move `core/schema/validator.go` to `core/schema/validator/document.go`.
    *   **Action:** `mv core/schema/validator.go core/schema/validator/document.go`

3.  **Update Package and Imports in `document.go`:**
    *   Read the content of `core/schema/validator/document.go`.
    *   Change the package declaration from `package schema` to `package validator`.
    *   Update import paths to fully qualify types from the parent `schema` package (e.g., `schema.FieldDefinition`, `schema.FunctionMap`).
    *   **Action:** Read, modify content, and `write_file` back to `core/schema/validator/document.go`.

4.  **Refactor `NewDocumentValidator` in `document.go`:**
    *   Change the function signature of the exported `NewDocumentValidator` in `core/schema/validator/document.go` to `func New(schemaDef *schema.SchemaDefinition, fmap *schema.FunctionMap) (*DocumentValidator, error)`. This will be the new public entry point for the `validator` sub-package.
    *   **Action:** Read, modify content, and `write_file` back to `core/schema/validator/document.go`.

5.  **Create API Facade (`core/schema/api.go`):**
    *   Create a new file `core/schema/api.go`.
    *   This file will house the original `schema.NewDocumentValidator` function to maintain backward compatibility.
    *   The implementation of this facade function will now delegate to `validator.New` from the new sub-package.
    *   **Action:** `write_file` for `core/schema/api.go` with the facade function.

6.  **Update References to `NewDocumentValidator`:**
    *   Any other files within the `core/schema` package (like `semantics.go` or `json.go`, if they were using it) that directly called `schema.NewDocumentValidator` will need their import statements adjusted to use the facade function correctly (though ideally, the facade should prevent most external changes). This step might involve updating internal calls within `schema` to use the `schema.NewDocumentValidator` facade.

7.  **Run All Tests:**
    *   Execute `go test ./...` from the project root.
    *   Address any compilation errors (e.g., missing imports, undefined types) or test failures immediately until all tests pass.

This phased approach will isolate the changes, making debugging and verification straightforward.
