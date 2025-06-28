### TODO List (Prioritized)

### Short-Term / Low-Complexity

- **Documentation**:
    - Update the `README.md` to reflect the current codebase.
    - Add more detailed documentation for all public APIs.
- **Improved Logging**:
    - Integrate `go.uber.org/zap` logger more comprehensively for detailed debugging and operational insights throughout the persistence layer.
- **Testing**:
    - Add more comprehensive unit and integration tests to ensure the stability and correctness of the framework.
- **Data Validation**:
    - Implement schema validation (`collection.Validate`) based on field constraints defined in `SchemaDefinition`.

### Medium-Term / Medium-Complexity

- **Advanced QueryDSL Features**:
    - Full support for cursor-based pagination.
    - Aggregation functions (Count, Sum, Avg, Min, Max).
- **Transaction Management**:
    - Expand the `core.PersistenceTransactionInterface` to provide robust, multi-operation transactional support.
- **Events & Observability**:
    - Implement the `core.PersistenceEvent` system, including subscriptions, triggers, and a comprehensive metadata API (`core.MetadataFilter`, `core.CollectionMetadata`).

### Long-Term / High-Complexity

- **Schema Versioning & Migrations**:
    - Implement `core.Migration` and `core.SchemaMigrationHelper`.
    - Implement the `Migrate` and `Rollback` methods in `core/persistence/collection.go`.
- **Advanced QueryDSL Features (Continued)**:
    - Window functions (Rank, Row Number).
    - Join operations (`InnerJoin`, `LeftJoin`, etc.).
    - Query Hints for performance optimization.
- **Scheduled Tasks**:
    - Implement `core.TaskInfo` to enable scheduling and execution of background jobs.
- **Static Type Mapping & Code Generation**:
    - Add optional static type mapping capabilities to generate Go structs from schema definitions.
- **More Database Adapters**:
    - Develop `Mapper`, `QueryExecutor`, and `QueryGenerator` implementations for other popular databases (e.g., PostgreSQL, MySQL, NoSQL databases).