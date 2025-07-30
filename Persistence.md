# Anansi Persistence Layer: Core Concepts

This document outlines the fundamental concepts and architecture of the Anansi persistence layer. Its primary goal is to provide a clear, schema-driven, and extensible framework for data persistence in Go applications.

## 1. Core Philosophy: Schema-First Development

The central principle of Anansi is **schema-first development**. Instead of defining data structures in Go code and then mapping them to a database (the typical ORM approach), Anansi inverts this model:

1.  **Define the Contract First**: You define your data models using a declarative JSON format (`schema.SchemaDefinition`). This schema is the single source of truth for a data entity's structure, constraints, and indexes.
2.  **Runtime Agnostic**: The application interacts with the persistence layer through this schema, which is loaded at runtime. This decouples the application logic from the underlying database structure.
3.  **Dynamic and Flexible**: Because schemas are loaded at runtime, it opens up possibilities for dynamic data models and simplifies schema evolution.

## 2. Key Architectural Components

The persistence layer is built on a set of interfaces that clearly separate responsibilities.

### 2.1. `persistence.PersistenceInterface`

This is the main entry point to the Anansi framework. It's a high-level API that provides access to the entire persistence system. Its key responsibilities include:

-   **Collection Management**: Creating, deleting, and retrieving collections (`Create`, `Delete`, `Collection`, `Collections`).
-   **Transaction Management**: Providing atomic, all-or-nothing execution blocks (`Transact`).
-   **Schema Management**: Retrieving schema definitions (`Schema`).
-   **Global Observability**: Managing event subscriptions that apply to the entire persistence layer (`RegisterSubscription`, `UnregisterSubscription`, `Subscriptions`).

### 2.2. `persistence.PersistenceCollectionInterface`

This interface represents a single "collection" of data, which typically maps to a database table. It provides the core **CRUD (Create, Read, Update, Delete)** operations for documents within that collection.

-   **`Create(data any)`**: Inserts one or more documents.
-   **`Read(query *query.Query)`**: Retrieves documents using the QueryDSL.
-   **`Update(params *CollectionUpdate)`**: Modifies existing documents.
-   **`Delete(query *query.QueryFilter, unsafe bool)`**: Removes documents.
-   **`Validate(data any, loose bool)`**: Validates data against the collection's schema.
-   **Collection-Scoped Observability**: Manages subscriptions and metadata specific to this collection.

### 2.3. `query.DatabaseInteractor`

This is the **lowest-level abstraction** in the persistence stack. It is the "database driver" interface for Anansi. Its sole responsibility is to execute primitive database operations.

-   **DDL (Data Definition Language)**: `CreateCollection`, `DropCollection`, `CreateIndex`.
-   **DML (Data Manipulation Language)**: `SelectDocuments`, `InsertDocuments`, `UpdateDocuments`, `DeleteDocuments`.
-   **Transaction Control**: `StartTransaction`, `Commit`, `Rollback`.

To support a new database (e.g., PostgreSQL), you would primarily need to implement this interface.

### 2.4. `query.QueryBuilder` and `query.Query` (The DSL)

Anansi uses a declarative Domain-Specific Language (DSL) to express queries.

-   **`query.QueryBuilder`**: A fluent API used to construct a query in a programmatic and type-safe way (e.g., `NewQueryBuilder().Where("age").Gt(30).OrderByAsc("name")`).
-   **`query.Query`**: The resulting immutable struct that represents the query. This object is then passed to the `Read` method.

This approach separates the *intent* of the query from its *execution*.

### 2.5. `query.QueryGenerator`

This component is responsible for translating the abstract `query.Query` DSL object into a concrete, database-specific SQL query string and its corresponding parameters. For example, the `sqlite.SqliteQuery` implementation knows how to generate SQLite-compatible SQL.

## 3. The Data Flow: From Query to Results

Understanding the data flow for a `Read` operation is key to understanding Anansi:

1.  **Build the Query**: The developer uses the `query.QueryBuilder` to create a `query.Query` object.
2.  **Initiate Read**: The `query.Query` object is passed to `PersistenceCollection.Read()`.
3.  **Execution Orchestration**: The `persistence.Executor` takes over. It analyzes the query and determines what needs to be fetched from the database.
4.  **SQL Generation**: The `Executor` uses the database-specific `query.QueryGenerator` (e.g., `sqlite.SqliteQuery`) to translate the `query.Query` into a SQL string.
5.  **Database Interaction**: The `Executor` passes the generated SQL to the `query.DatabaseInteractor` (e.g., `sqlite.SQLiteInteractor`), which executes the query against the database.
6.  **Post-Processing**: The raw data returned from the database is processed in-memory by the `query.DataProcessor`. This is where Go-based computed fields and custom filters are applied.
7.  **Return Result**: The final, processed data is returned as a `query.QueryResult`.

## 4. Schema Registry and Versioning

A crucial, but less visible, component is the **Schema Registry**.

-   **Centralized Management**: Anansi maintains an internal collection (a table named `anansi_schemas`) to store all registered `SchemaDefinition`s.
-   **Logical vs. Physical Naming**: The registry maps a logical collection name (e.g., "users") to a physical table name (e.g., "users_v1_abc"). This is the foundation for zero-downtime migrations.
-   **Versioning**: Each schema has a version. The registry tracks the currently "active" version for each logical collection. Future work will leverage this for seamless schema migrations.

## 5. Extensibility

Anansi is designed to be extended:

-   **New Databases**: Implement the `query.DatabaseInteractor` and `query.QueryGenerator` interfaces.
-   **Custom Logic**: Inject custom Go functions via the `schema.FunctionMap` during `persistence.NewPersistence` initialization to create powerful, in-memory computed fields and filters.

By separating these concerns, Anansi provides a robust and flexible foundation for building data-driven applications in Go.
