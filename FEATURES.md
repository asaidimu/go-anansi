## Current Features & Capabilities

*   **Flexible Data Modeling:**
    *   `Document` type for schema-aware, flexible data structures.
    *   Comprehensive JSON serialization and deserialization for `Document` and Go structs.
    *   Support for path-based access to nested data within documents.
    *   Data transformation, diffing, and merging utilities.
    *   Robust type coercion capabilities.

*   **Robust Persistence Layer:**
    *   Abstracted persistence interface supporting various data stores.
    *   Centralized Collection Registry for managing schema definitions and versions.
    *   Automatic bootstrapping and management of an internal schema metadata store (`_schemas_`).
    *   Transactional operations for atomic data and schema changes.
    *   Event-driven architecture for persistence operations.

*   **Powerful Query Engine:**
    *   Domain-Specific Language (DSL) for expressive data querying.
    *   Support for filtering, sorting, pagination (offset and cursor-based), and field projection.
    *   Aggregation functions (e.g., count, sum) and distinct operations.
    *   In-memory query helper for testing and simple use cases.
    *   Comprehensive join capabilities (Inner, Left, Right, Full).

*   **Schema Management & Validation:**
    *   Declarative schema definition including field types, constraints, and indexes.
    *   Data validation against defined schemas.
    *   Mechanisms for tracking and applying schema changes (migrations).

*   **SQLite Database Integration:**
    *   Concrete implementation of the persistence layer for SQLite databases.
    *   Automatic SQL generation for CRUD operations based on schema definitions.

*   **Developer Experience & Utilities:**
    *   Extensive unit, integration, and end-to-end test suites.
    *   Generic utility functions for common tasks (e.g., type coercion, JSON handling, pointer helpers).

## Capabilities & Possible Patterns and Usage

The `go-anansi` library is designed with extensibility and flexibility at its core, enabling powerful architectural patterns and usage scenarios beyond typical CRUD operations.

*   **Application-Level Row-Level Security (RLS):** Leverage the sophisticated Query Engine to implement fine-grained access control and data filtering directly within your application logic. This allows for robust RLS without relying on database-specific features, as demonstrated in `example/complex`.
*   **Multi-Store & Local-First Architectures:** The abstract `Persistence` interface acts as a powerful facade, allowing you to compose and manage multiple concrete persistence implementations. This enables advanced patterns like local-first desktop applications (e.g., SQLite for local storage synced with a remote MongoDB server), data sharding, or read-replica management, all abstracted behind a unified API.
*   **Event-Driven Observability & Extensibility:** The comprehensive event system provides powerful hooks for real-time observability, auditing, and custom business logic. Events emitted during persistence operations can be used for logging, monitoring, analytics, cache invalidation, or triggering external workflows.
*   **Schema-Driven Development:** Utilize the declarative schema definitions to drive not only data validation and migrations but also potentially code generation, API documentation, or dynamic UI forms, ensuring consistency across your application stack.
*   **Pluggable Components:** Nearly every core component, from the `DatabaseInteractor` to the `QueryEngine` and `SchemaManager`, is defined by interfaces, allowing developers to easily swap out or extend functionality with custom implementations tailored to specific needs.

## Feature Proposals

### Persistence Layer Health Check (`persistence.HealthCheck`)

**Problem:**
The current persistence layer relies on its internal registry metadata to track collections. If a physical collection is externally modified or deleted (e.g., a database table is dropped manually), the application will only discover this inconsistency at runtime when an operation is attempted on the missing collection, leading to unexpected errors.

**Proposal:**
Introduce a `HealthCheck` method within the `persistence` package to proactively verify the integrity and consistency of the persistence layer upon application startup or on demand.

**API (Proposed):**
```go
// In core/persistence/base/base.go (interface)
type Persistence interface {
    // ... existing methods ...
    HealthCheck(ctx context.Context) (*HealthCheckReport, error)
}

// In core/persistence/base/types.go (or a new health.go file)
type HealthCheckReport struct {
    OverallHealthy bool
    Checks         []HealthCheckResult
}

type HealthCheckResult struct {
    Name    string
    Healthy bool
    Message string
    Error   error
}
```

**Checks to Include:**
1.  **Database Connectivity:** Verify connection to the underlying database.
2.  **Registry Integrity:** Ensure the internal `_schemas_` collection is accessible and its metadata can be read.
3.  **Physical Collection Existence:** For every collection registered in `_schemas_`, verify that its corresponding physical storage (e.g., database table) exists.

**Benefits:**
*   Proactive identification of persistence layer inconsistencies.
*   Improved debugging and operational visibility.
*   Foundation for more robust self-healing or reconciliation mechanisms.

### Enforcing Document Schema at Creation and Runtime Validation (`collection.Document()`)

**Problem:**
When creating new documents, developers currently need to manually ensure they conform to the collection's defined schema, including setting default values or ensuring required fields are present. This can lead to boilerplate code, potential inconsistencies, and deferred validation errors. Additionally, while schemas define types and constraints, there's no immediate runtime enforcement when values are set on a `data.Document`. This can lead to invalid data being stored, only to be caught during persistence operations or later, making debugging harder.

**Proposal:**
Enhance the `Collection` interface with a `Document()` method that returns a new `data.Document` instance. This document would be pre-populated with default values as defined in the collection's active schema and structured to reflect the schema's layout. This acts as a "schema-aware" document factory. Furthermore, `data.Document` instances obtained via `collection.Document()` will be "schema-aware." This means that attempts to set values that violate the associated schema's type or constraints (e.g., assigning a number to a string field) will result in an immediate error (or panic for `MustSet` operations). This runtime validation will be implemented by returning a new struct that embeds `data.Document` and overrides its `Set` and `SetNested` methods.

**API (Proposed):**
```go
// In core/persistence/base/base.go (interface)
type Collection interface {
    // ... existing methods ...
    Document() data.Document // Returns a new Document pre-populated based on the collection's schema
}

// Internal mechanism (conceptual):
// The returned data.Document would be an instance of a new struct (e.g., SchemaAwareDocument)
// that embeds data.Document and overrides its Set/SetNested methods with validation logic
// based on the collection's schema. Other Document methods would be promoted automatically.
```

**Benefits:**
*   **Simplified Document Creation:** Developers can quickly get a valid, schema-conforming document instance.
*   **Improved Data Consistency:** Ensures new documents adhere to the schema from the point of creation, reducing later validation failures.
*   **Reduced Boilerplate:** Eliminates the need for manual default value assignment and initial structuring.
*   **Enhanced Developer Experience:** Provides a more intuitive and safer way to interact with schema-bound collections.
*   **Immediate Feedback:** Developers receive instant notification of schema violations during development, preventing invalid data from propagating.
*   **Stronger Type Safety:** Enforces schema types and constraints at the point of data modification.
*   **Reduced Debugging Time:** Catches data integrity issues earlier in the development cycle.

## Quality of Life & Dev Ex

*   **Enhanced Error Handling & Messaging:** Standardize error structures and ensure all errors returned by the library are clear, actionable, and provide sufficient context for debugging. This includes consistent error wrapping and custom error types where appropriate.
*   **Comprehensive Logging & Tracing:** Implement more granular and configurable logging for key operations within the persistence layer, allowing developers to easily diagnose issues. Consider integration points for distributed tracing.
*   **Improved Documentation & Examples:** Expand the existing documentation with more practical examples, usage patterns, and clear explanations of core concepts. Ensure the `README.md` and `example/` directories are up-to-date and easy to follow.
*   **Simplified Configuration:** Streamline the setup process for the persistence layer, potentially by providing more sensible defaults or clearer configuration options for database connections and registry settings.