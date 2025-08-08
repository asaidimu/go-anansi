# Refactoring Plan: `JoinConfiguration.Target`

## 1. Objective

This document outlines the plan to refactor the `Target` field within the `JoinConfiguration` struct in `core/query/dsl.go`. The goal is to change the type of `Target` from a simple `string` to the more descriptive and flexible `QueryTarget` struct. This will allow for more complex join scenarios, such as joining with a subquery or providing a schema for the join target.

## 2. Background

The current implementation of `JoinConfiguration` uses a `string` to identify the target of a join operation.

**`core/query/dsl.go`:**
```go
// JoinConfiguration defines a join operation with another table.
type JoinConfiguration struct {
	Type       JoinType                 `json:"type"`
	Target     string                   `json:"target"` // TODO: Refactor this to be a QueryTarget
	Alias      *string                  `json:"alias,omitempty"`
	On         *QueryFilter             `json:"on"`
	Projection *ProjectionConfiguration `json:"projection,omitempty"`
}
```

The `QueryTarget` struct provides a richer way to define a query target:

**`core/query/dsl.go`:**
```go
type QueryTarget struct {
	Name   string  `json:"name,omitempty"`
	Alias  *string `json:"alias,omitempty"`
	Schema *schema.SchemaDefinition `json:"schema,omitempty"`
}
```

This refactoring will align the `JoinConfiguration` with other parts of the query DSL that may already use `QueryTarget`, making the entire DSL more consistent.

## 3. Proposed Refactoring

The `Target` field of `JoinConfiguration` will be changed from `string` to `QueryTarget`. Additionally, the redundant `Alias` field in `JoinConfiguration` will be removed in favor of the `Alias` field within `QueryTarget`.

**`core/query/dsl.go` (After Refactoring):**
```go
// JoinConfiguration defines a join operation with another table.
type JoinConfiguration struct {
	Type       JoinType                 `json:"type"`
	Target     QueryTarget              `json:"target"` // Changed from string
	On         *QueryFilter             `json:"on"`
	Projection *ProjectionConfiguration `json:"projection,omitempty"`
}
```

## 4. Impact Analysis & Affected Files

This is a breaking change. All code that constructs or processes `JoinConfiguration` objects will need to be updated. A search of the codebase will be required to identify all affected files.

Likely affected areas include:
- Query building logic (e.g., `core/query/builder.go`)
- Database-specific implementations (e.g., `sqlite/query.go`)
- Unit tests for query functionality (e.g., `tests/unit/query/*_test.go`)

## 5. Action Plan

1.  **Modify `JoinConfiguration` struct:**
    *   In `core/query/dsl.go`, change the `Target` field from `string` to `QueryTarget`.
    *   In `core/query/dsl.go`, remove the `Alias *string` field from `JoinConfiguration`.

2.  **Update `JoinConfiguration` Instantiation:**
    *   Search for all places where `JoinConfiguration` is instantiated.
    *   Update the instantiation to use the new structure. For example:
        *   **Before:** `Target: "users", Alias: &"u"`
        *   **After:** `Target: query.QueryTarget{Name: "users", Alias: &"u"}`

3.  **Update Code that Accesses `JoinConfiguration.Target`:**
    *   Search for code that reads `joinConfig.Target` or `joinConfig.Alias`.
    *   Update it to access `joinConfig.Target.Name` and `joinConfig.Target.Alias` instead.

4.  **Update `sqlite` backend:**
    *   Inspect `sqlite/query.go` and other files in the `sqlite` package.
    *   Update the logic that translates the `JoinConfiguration` into SQL `JOIN` clauses to work with the new `QueryTarget` structure.

5.  **Update Unit Tests:**
    *   Update any tests in `tests/unit/` that use `JoinConfiguration`. This will likely include tests for query building and execution.
    *   Ensure tests cover the new capabilities of `QueryTarget` in joins (e.g., using an alias).

6.  **Documentation:**
    *   Update any documentation (e.g., in `QUERYLANG.md` or other docs) that refers to how joins are constructed.

## 6. Potential Risks and Considerations

*   **Breaking Change:** This is a significant breaking change. Any consumers of this library will need to update their code. The version of the library should be bumped according to semantic versioning (e.g., a major version bump).
*   **Complexity of `QueryTarget`:** `QueryTarget` can also contain a `Schema`. The refactoring should ensure that this is handled correctly, even if the initial implementation only uses the `Name` and `Alias` fields. The query processing logic needs to be aware of the possibility of a schema being present.
*   **Subqueries as Join Targets:** This refactoring paves the way for supporting subqueries as join targets in a more structured manner, by potentially extending `QueryTarget` or how it's used in the future.
