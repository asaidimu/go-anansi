# Refactor Plan

This document outlines a plan to refactor the codebase to reduce duplication and improve maintainability.

## 1. Consolidate Logical Operator Logic

- **Observation**: The logical operator evaluation logic is duplicated in `core/common/logical.go` and `core/logical/logical.go`.
- **Plan**:
    1. Remove the `core/logical/logical.go` file.
    2. Update any code that imports `core/logical/logical.go` to import `core/common/logical.go` instead.
    3. Ensure that the `LogicalOperator` type from `core/common/types.go` is used consistently.

## 2. Centralize Persistence Types and Errors

- **Observation**: Persistence-related types and errors are scattered across multiple packages. For example, `core/persistence/types.go` and `core/persistence/base/types.go` define similar event types and structures. Errors are defined in `core/persistence/base/errors.go` but could be consolidated.
- **Plan**:
    1. Move all persistence-related types from `core/persistence/types.go` to `core/persistence/base/types.go`.
    2. Remove the `core/persistence/types.go` file.
    3. Update all imports to point to `core/persistence/base/types.go`.
    4. Consolidate all persistence-related errors into `core/persistence/base/errors.go`.

## 3. Consolidate Utility Functions

- **Observation**: There are several utility packages and functions that could be consolidated. For example, `core/utils/utils.go` and `core/persistence/utils/utils.go`.
- **Plan**:
    1. Move all common utility functions to the `core/utils` package.
    2. Remove the `core/persistence/utils/utils.go` file and update imports.
    3. Review other utility packages for potential consolidation.
