# Project Roadmap: go-anansi

This document outlines the current state of the `go-anansi` library and a roadmap for its future development.

## Production Readiness Assessment

**Current Status: Not Production Ready**

The `go-anansi` library is currently in an early development phase and is **not yet suitable for production environments**. Key reasons for this assessment include:

*   **Lack of Comprehensive Testing:** There is currently no established test suite (unit, integration, or end-to-end tests). This is the most critical missing component for ensuring reliability and correctness.
*   **Incomplete Core Features:** Several fundamental features, such as full transaction management, metadata retrieval, and schema migration/rollback, are currently stubbed or partially implemented.
*   **Limited Error Handling and Logging:** While basic logging is present, a more robust and configurable logging strategy, along with comprehensive error handling, is needed for production-grade stability and debugging.

## Roadmap

This roadmap prioritizes tasks to achieve production readiness, followed by further enhancements.

### Phase 1: Immediate Priorities (Achieving Production Readiness)

The primary focus of this phase is to establish a solid foundation for production use.

*   **1.1 Comprehensive Testing Suite (CRITICAL)**
    *   Implement a robust suite of unit tests for all core logic (persistence, query, schema).
    *   Develop integration tests for database interactions (SQLite adapter).
    *   Establish end-to-end tests for key workflows (e.g., collection creation, CRUD operations, basic queries).
    *   Integrate testing into CI/CD pipeline.
*   **1.2 Complete Core Persistence Features**
    *   **Transaction Management:** Fully implement `persistence.Transact` to ensure atomic operations across multiple database calls.
    *   **Metadata API:** Complete the implementation of `persistence.Metadata` and `collection.Metadata` to provide comprehensive introspection capabilities.
    *   **Schema Migration & Rollback:** Fully implement `collection.Migrate` and `collection.Rollback` methods, including data transformation logic.
*   **1.3 Enhanced Error Handling and Logging**
    *   Review and standardize error types and propagation throughout the library.
    *   Implement structured logging with `go.uber.org/zap` for all critical operations, including configurable log levels.
    *   Add metrics and tracing integration points (e.g., OpenTelemetry) for better observability.

### Phase 2: Short-Term Goals (Post-Production Readiness)

Once the library is deemed production-ready, these features will enhance its usability and robustness.

*   **2.1 Advanced QueryDSL Features (Initial)**
    *   Full support for cursor-based pagination in `query.QueryDSL`.
    *   Implement basic aggregation functions (Count, Sum, Avg, Min, Max) in the query processor.
*   **2.2 Improved Documentation**
    *   Update the `README.md` to reflect the current state and capabilities.
    *   Generate and maintain comprehensive GoDoc documentation for all public APIs.
    *   Expand usage examples in the `examples/` directory.
*   **2.3 Concurrency and Performance Review**
    *   Conduct a thorough review of concurrency patterns and potential bottlenecks.
    *   Implement performance benchmarks for key operations.

### Phase 3: Medium-Term Goals (Feature Expansion)

This phase focuses on adding more advanced capabilities and flexibility.

*   **3.1 Advanced QueryDSL Features (Continued)**
    *   Implement support for window functions (e.g., Rank, Row Number).
    *   Add support for join operations (`InnerJoin`, `LeftJoin`, `RightJoin`, `FullJoin`).
    *   Integrate query hints for performance optimization.
*   **3.2 Extensible Predicate and Function System**
    *   Refine the `PredicateMap` and `FunctionMap` to allow easier registration and discovery of custom validation predicates and computed functions.
*   **3.3 Enhanced Schema Management**
    *   Implement schema validation (`collection.Validate`) based on field constraints defined in `SchemaDefinition`.
    *   Add support for more complex field types and their validation rules.

### Phase 4: Long-Term Goals (Future Vision)

These are ambitious goals that represent the long-term vision for `go-anansi`.

*   **4.1 Additional Database Adapters**
    *   Develop `Mapper`, `QueryExecutor`, and `QueryGenerator` implementations for other popular databases (e.g., PostgreSQL, MySQL, NoSQL databases).
*   **4.2 Static Type Mapping & Code Generation**
    *   Explore and implement optional static type mapping capabilities to generate Go structs directly from schema definitions, improving type safety and developer experience.
*   **4.3 Scheduled Tasks Integration**
    *   Implement `core.TaskInfo` to enable scheduling and execution of background jobs directly within the persistence framework.
*   **4.4 Distributed Transactions/Sagas**
    *   Investigate and potentially implement patterns for distributed transactions or sagas for complex microservice architectures.
