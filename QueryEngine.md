# Query Engine Architecture

## Overview

This document outlines the architecture of the query engine, designed to efficiently execute `QueryDSL` queries by intelligently splitting the workload between a primary database and the Go runtime. The core principle is a proactive, capabilities-based partitioning model that maximizes database performance while allowing for powerful, in-memory data transformations.

## Core Components

The query engine consists of four main components:

1.  **Executor**: The central orchestrator of a query. It manages the overall execution flow, coordinating between the other components.

2.  **DatabaseInteractor**: A database-specific interface responsible for all direct communication with the database (e.g., `sqlite.SQLiteInteractor`). A key responsibility of this component is to declare its features via a `Capabilities` struct.

3.  **QueryPartitioner**: The "brain" of the engine. This utility is responsible for analyzing a user's `QueryDSL` against the `Capabilities` of the `DatabaseInteractor` and splitting it into two distinct, actionable parts.

4.  **QueryHelper**: A comprehensive, universal engine for performing `QueryDSL` operations (filtering, sorting, pagination, projection, aggregation) on in-memory slices of `schema.Document` objects.

## The Problem with the Old Model (`DataProcessor`)

The previous architecture involved a `DataProcessor` and a `QueryHelper`, leading to several issues:

*   **Duplicated Logic**: Both components contained complex and overlapping logic for evaluating filters, projections, and expressions in-memory. This increased maintenance overhead and the risk of behavioral divergence.
*   **Reactive Approach**: The `Executor` would send a query to the database and receive a list of `skippedOperators` that the database couldn't handle. This was a reactive, brittle mechanism.
*   **Lack of Planning**: The system couldn't proactively plan the query. It couldn't, for example, ask the database to fetch fields that were not part of the final projection but were required for an in-memory computation step.

## The New Architecture: Capabilities-Based Partitioning

The new architecture replaces the `DataProcessor` entirely and elevates the `QueryHelper` to be the single source of truth for in-memory evaluation. This is achieved through an intelligent, multi-stage partitioning process managed by the `Executor`.

### Step 1: Capability Declaration

The `DatabaseInteractor` for a given database (e.g., SQLite, PostgreSQL) implements a `Capabilities()` method. This method returns a struct that explicitly declares which features the database supports natively.

```go
// Example Capabilities struct
type Capabilities struct {
    SupportedOperators map[query.ComparisonOperator]struct{}
    SupportedFunctions map[string]struct{}
    SupportsJoins      bool
    SupportsGroupBy    bool
    // ... and other features
}
```

### Step 2: Intelligent Partitioning

When a query is executed, the `Executor` uses the `QueryPartitioner` to split the user's `QueryDSL` into two new objects:

1.  **`dbQuery`**: Contains all operations that the database **can** handle, based on its declared `Capabilities`.
2.  **`postProcessingQuery`**: Contains all operations that must be performed in-memory by the `QueryHelper`.

### Step 3: Dependency Analysis & Augmentation

This is the most critical phase. The partitioner recognizes that the `postProcessingQuery` has data dependencies that must be satisfied by the `dbQuery`. It performs a deep traversal of the `postProcessingQuery` to find every field required for in-memory operations.

The key is to **traverse the `FilterValue` structure and identify all `FieldReference` nodes**. This provides an explicit, unambiguous declaration of field dependencies without needing to guess based on function arguments.

Dependencies are collected from:
*   **Custom Filters**: The fields involved in custom filter conditions.
*   **Computed Fields & Case Expressions**: The arguments of any `FunctionCall` and any `FieldReference` used within `CASE` expressions.
*   **In-Memory Sorting**: The fields the `QueryHelper` will need to sort on.
*   **In-Memory Aggregations**: The fields to be grouped on and aggregated.
*   **Final Projection**: The fields the user ultimately requested in the output.

The partitioner then **augments the `dbQuery`'s projection**, adding all discovered dependencies to its `Include` list. This ensures that the database fetches all necessary raw materials for the subsequent in-memory step, even if those fields are not part of the final result.

### Step 4: Execution Flow

1.  The `Executor` sends the augmented `dbQuery` to the `DatabaseInteractor`, which generates and executes the database-specific query (e.g., SQL).
2.  The database returns a set of rows. This data is guaranteed to contain all fields needed for both the final result and any in-memory processing.
3.  The `Executor` passes the database results and the `postProcessingQuery` to the `QueryHelper`.
4.  The `QueryHelper` executes the in-memory operations (custom filters, computed fields, sorting, etc.) and applies the original user-requested projection to shape the final result.

## Benefits of the New Architecture

*   **Eliminates Duplication**: The `QueryHelper` becomes the single, authoritative engine for in-memory query logic, and the `DataProcessor` is removed.
*   **Robust & Predictable**: The proactive planning based on declared capabilities is far more reliable than the old reactive `skippedOperators` model.
*   **Clear Separation of Concerns**: Each component has a distinct and well-defined responsibility.
*   **Extensibility**: Adding a new database backend is a clean process of implementing the `DatabaseInteractor` interface and declaring its unique `Capabilities`. The rest of the engine adapts automatically.
