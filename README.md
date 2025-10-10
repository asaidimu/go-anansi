# `go-anansi` 🕸️

**go-anansi** is a powerful and flexible Go persistence and query framework designed for building robust, schema-driven applications. It provides a clean, abstract interface for interacting with various data stores, coupled with a rich query language and comprehensive schema management capabilities. Inspired by modern database patterns, Anansi streamlines complex data operations, offering extensibility, observability, and strong data integrity.

[![Go Reference](https://pkg.go.dev/github.com/asaidimu/go-anansi/v6?tab=doc)](https://pkg.go.dev/github.com/asaidimu/go-anansi/v6)
[![Go Report Card](https://goreportcard.com/badge/github.com/asaidimu/go-anansi/v6)](https://goreportcard.com/report/github.com/asaidimu/go-anansi/v6)
[![License: Proprietary](https://img.shields.io/badge/License-Proprietary-red.svg)](./LICENSE.md)
[![Latest Version](https://img.shields.io/github/v/release/asaidimu/go-anansi?include_prereleases&sort=semver)](https://github.com/asaidimu/go-anansi/releases)

## 🔗 Quick Links

*   [Overview & Features](#-overview--features)
*   [Installation & Setup](#-installation--setup)
*   [Usage Documentation](#-usage-documentation)
    *   [Basic CRUD Operations](#basic-crud-operations)
    *   [Advanced Usage: Decorators & Joins](#advanced-usage-decorators--joins)
    *   [Building Queries with the Query Language](#building-queries-with-the-query-language)
*   [Project Architecture](#-project-architecture)
*   [Development & Contributing](#-development--contributing)
*   [Additional Information](#-additional-information)

---

## ⚡ Overview & Features

`go-anansi` is a Go library providing an opinionated yet flexible approach to data persistence. It abstracts away the complexities of direct database interaction through a clean, schema-driven API, allowing developers to focus on business logic. At its core, Anansi centralizes schema definitions, automates data validation, and offers a powerful, natural language-like query DSL that maps to structured queries for underlying data stores. Its design emphasizes pluggability, enabling seamless integration with different database backends and advanced architectural patterns.

This framework is built to handle complex data modeling, ensure transactional integrity, and provide extensive observability through an event-driven architecture. Whether you're building a simple CRUD application or a sophisticated multi-store system with row-level security, `go-anansi` offers the tools to manage your data reliably and efficiently.

### Key Features

*   **Flexible Data Modeling:**
    *   `Document` type for schema-aware, flexible data structures with comprehensive JSON serialization/deserialization.
    *   Path-based access, data transformation, diffing, merging, and robust type coercion.
*   **Robust Persistence Layer:**
    *   Abstracted `Persistence` interface supporting various data stores (e.g., SQLite via provided implementation).
    *   Centralized `Collection Registry` for managing schema definitions and versions.
    *   Automatic bootstrapping and management of an internal schema metadata store (`_schemas_`).
    *   Transactional operations for atomic data and schema changes with support for concurrent operations.
    *   Event-driven architecture for persistence operations, offering hooks for observability and custom logic.
*   **Powerful Query Engine:**
    *   Domain-Specific Language (DSL) for expressive data querying (see [QUERYLANG.md](./QUERYLANG.md)).
    *   Support for filtering, sorting, pagination (offset and cursor-based), field projection, and computed fields.
    *   Aggregation functions (e.g., `COUNT`, `SUM`, `AVG`, `MIN`, `MAX`) and distinct operations.
    *   Comprehensive join capabilities (`INNER`, `LEFT`, `RIGHT`, `FULL`).
    *   Query partitioning for mixed database and in-memory processing based on interactor capabilities.
*   **Schema Management & Validation:**
    *   Declarative schema definition including field types, constraints, and indexes.
    *   Data validation against defined schemas, with support for nested objects, arrays, sets, and unions.
    *   Mechanisms for tracking and applying schema changes (migrations) and rollbacks (planned).
*   **SQLite Database Integration:**
    *   Concrete implementation of the persistence layer for SQLite databases.
    *   Automatic SQL generation for CRUD and DDL operations based on schema definitions.
*   **Developer Experience & Utilities:**
    *   Extensive testing suites (unit, integration, E2E).
    *   Generic utility functions for common tasks (type coercion, JSON handling, pointer helpers).

### Capabilities & Usage Patterns

`go-anansi` promotes advanced architectural patterns beyond basic CRUD:

*   **Application-Level Row-Level Security (RLS):** Leverage the Query Engine to implement fine-grained access control and data filtering directly within your application logic. This allows for robust RLS without relying on database-specific features.
*   **Multi-Store & Local-First Architectures:** The abstract `Persistence` interface acts as a powerful facade, allowing you to compose and manage multiple concrete persistence implementations. This enables advanced patterns like local-first desktop applications (e.g., SQLite for local storage synced with a remote server), data sharding, or read-replica management.
*   **Event-Driven Observability & Extensibility:** The comprehensive event system provides powerful hooks for real-time observability, auditing, and custom business logic. Events emitted during persistence operations can be used for logging, monitoring, analytics, cache invalidation, or triggering external workflows.
*   **Schema-Driven Development:** Utilize declarative schema definitions to drive not only data validation and migrations but also potentially code generation, API documentation, or dynamic UI forms, ensuring consistency across your application stack.
*   **Pluggable Components:** Nearly every core component, from the `DatabaseInteractor` to the `QueryEngine` and `SchemaManager`, is defined by interfaces, allowing developers to easily swap out or extend functionality with custom implementations.

---

## 📦 Installation & Setup

### Prerequisites

*   [Go 1.22+](https://go.dev/doc/install) (The project uses Go 1.24.4, but typically 1.22+ is compatible)
*   A compatible database driver for your chosen backend (e.g., `github.com/mattn/go-sqlite3` for SQLite).

### Installation Steps

To add `go-anansi` to your Go project:

```bash
go get github.com/asaidimu/go-anansi/v6
```

### Configuration

The `anansi.Setup` function is the primary entry point for initializing the persistence layer. It requires a `SetupConfig` struct to configure the database interactor, logger, document factory, and optional decorators.

Here's a basic example of how to configure `anansi` with an in-memory SQLite database:

```go
package main

import (
	"database/sql"
	"log"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"go.uber.org/zap"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// 1. Setup In-Memory SQLite Database
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// 2. Create Database Interactor for SQLite
	executor, err := sqliteExecutor.NewSQLiteInteractor(db, logger)
	if err != nil {
		log.Fatalf("Failed to create SQLite interactor: %v", err)
	}
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil {
		log.Fatalf("Failed to create native interactor: %v", err)
	}

	// 3. Setup Document Factory Config (required for document hashing/metadata)
	factoryConfig := data.DocumentFactoryConfig{}

	// 4. Setup Decorators (optional, none for basic setup)
	decorators := &utils.Decorators{}

	// 5. Initialize Anansi Persistence Layer
	cfg := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		FactoryConfig: factoryConfig,
		Decorators:    decorators,
		Schemas:       []schema.SchemaDefinition{}, // Optional: initial schemas
	}
	p, err := anansi.Setup(cfg)
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")
	// `p` is now your entry point to the persistence layer.
}
```

### Verification

To confirm successful installation and setup, you can run the provided examples or your own test code after initializing the `anansi` persistence layer. A successful initialization without panics or errors indicates that the core components are correctly configured and communicating.

---

## 📖 Usage Documentation

`go-anansi` provides a high-level API for interacting with your data. This section covers basic CRUD operations, advanced patterns, and how to construct queries using the powerful Query DSL.

### Basic CRUD Operations

Once the `anansi` persistence layer is set up, you can define schemas and interact with collections.

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	coreutils "github.com/asaidimu/go-anansi/v6/core/utils"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"go.uber.org/zap"
	_ "github.com/mattn/go-sqlite3"
)

// Product schema definition
func getProductSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:    "Product",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":    {Name: "id", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
			"name":  {Name: "name", Type: "string", Required: coreutils.BoolPtr(true)},
			"price": {Name: "price", Type: "number", Required: coreutils.BoolPtr(true)},
			"stock": {Name: "integer", Type: "integer", Required: coreutils.BoolPtr(true)},
		},
	}
}

func main() {
	// ... (Anansi setup as shown in Configuration section)
	logger, _ := zap.NewDevelopment() // Assuming logger setup
	db, _ := sql.Open("sqlite3", "file::memory:?cache=shared") // Assuming DB setup
	executor, _ := sqliteExecutor.NewSQLiteInteractor(db, logger)
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, _ := native.NewNativeInteractor(executor, queryFactory, logger)
	factoryConfig := data.DocumentFactoryConfig{}
	decorators := &utils.Decorators{}
	p, _ := anansi.Setup(anansi.SetupConfig{
		Interactor: interactor, Logger: logger, FactoryConfig: factoryConfig, Decorators: decorators,
	})
	// ...

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create "products" collection
	productSchema := getProductSchema()
	productsCollection, err := p.CreateCollection(ctx, *productSchema)
	if err != nil {
		log.Fatalf("Failed to create products collection: %v", err)
	}
	logger.Info("Products collection created.")

	// Create Products
	product1 := data.MustNewDocument(map[string]any{"id": "P001", "name": "Laptop", "price": 1200.00, "stock": 50})
	product2 := data.MustNewDocument(map[string]any{"id": "P002", "name": "Mouse", "price": 25.00, "stock": 200})

	_, err = productsCollection.CreateOne(ctx, product1)
	if err != nil {
		log.Fatalf("Failed to create product P001: %v", err)
	}

	// Read Products
	allProductsQuery := query.NewQueryBuilder().Build() // Empty query to read all
	readResult, err := productsCollection.Read(ctx, &allProductsQuery)
	if err != nil {
		log.Fatalf("Failed to read all products: %v", err)
	}
	if readResult.Count > 0 {
		for _, doc := range readResult.Data.([]data.Document) {
			logger.Info(fmt.Sprintf("Found product: ID=%s, Name=%s, Price=%.2f, Stock=%d",
				doc["id"], doc["name"], doc["price"], doc["stock"]))
		}
	}

	// Update Product (P001 stock)
	updateProduct1 := data.MustNewDocument(map[string]any{"id": "P001", "stock": 45})
	filterP001 := query.NewQueryBuilder().Where("id").Eq("P001").Build().Filters
	_, err = productsCollection.Update(ctx, &base.CollectionUpdate{Data: updateProduct1, Filter: filterP001})
	if err != nil {
		log.Fatalf("Failed to update product P001: %v", err)
	}

	// Delete Product (P002)
	filterP002 := query.NewQueryBuilder().Where("id").Eq("P002").Build().Filters
	_, err = productsCollection.Delete(ctx, filterP002, false)
	if err != nil {
		log.Fatalf("Failed to delete product P002: %v", err)
	}
}
```

### Advanced Usage: Decorators & Joins

`go-anansi` supports `CollectionDecorators` for custom logic (e.g., validation, security) and powerful query joins.

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	coreutils "github.com/asaidimu/go-anansi/v6/core/utils"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"go.uber.org/zap"
	_ "github.com/mattn/go-sqlite3"
)

// NegativeAmountValidator is a CollectionDecorator that prevents transactions with negative amounts.
type negativeAmountValidator struct {
	next   base.Collection
	logger *zap.Logger
}
func NegativeAmountValidator(logger *zap.Logger) utils.CollectionDecorator {
	return func(next base.Collection) base.Collection {
		return &negativeAmountValidator{next: next, logger: logger}
	}
}
func (d *negativeAmountValidator) validateAmount(doc data.Document) error { /* ... */ return nil }
func (d *negativeAmountValidator) CreateOne(ctx context.Context, doc data.Document) (base.CreateResult, error) {
	if err := d.validateAmount(doc); err != nil {
		return base.CreateResult{Status: base.StatusFailedValidation, Data: doc, Issues: []common.Issue{{Message: err.Error()}}}, err
	}
	return d.next.CreateOne(ctx, doc)
}
// ... (other base.Collection methods implemented for negativeAmountValidator) ...

func main() {
	// ... (Anansi setup as shown in Configuration section)
	logger, _ := zap.NewDevelopment()
	db, _ := sql.Open("sqlite3", "file::memory:?cache=shared")
	executor, _ := sqliteExecutor.NewSQLiteInteractor(db, logger)
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, _ := native.NewNativeInteractor(executor, queryFactory, logger)
	factoryConfig := data.DocumentFactoryConfig{}

	// Add our custom NegativeAmountValidator to the collection decorators
	decorators := &utils.Decorators{
		CollectionDecorators: []utils.DecoratorFunc[base.Collection]{
			(utils.DecoratorFunc[base.Collection])(NegativeAmountValidator(logger)), // Explicit cast
		},
	}
	p, _ := anansi.Setup(anansi.SetupConfig{
		Interactor: interactor, Logger: logger, FactoryConfig: factoryConfig, Decorators: decorators,
	})
	// ...

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Assume collections 'User', 'Account', 'LedgerTransaction' are created
	usersCollection, _ := p.Collection(ctx, "User")
	accountsCollection, _ := p.Collection(ctx, "Account")
	transactionsCollection, _ := p.Collection(ctx, "LedgerTransaction")

	// Populate Data (simplified)
	user1 := data.MustNewDocument(map[string]any{"id": "U001", "name": "Alice"})
	account1 := data.MustNewDocument(map[string]any{"id": "A001", "userId": "U001", "balance": 1000.00})
	tx1 := data.MustNewDocument(map[string]any{"id": "T001", "accountId": "A001", "amount": 200.00, "type": "deposit", "timestamp": time.Now().Unix()})

	usersCollection.CreateOne(ctx, user1)
	accountsCollection.CreateOne(ctx, account1)
	transactionsCollection.CreateOne(ctx, tx1)

	// Attempt to create an invalid transaction (negative amount)
	invalidTx := data.MustNewDocument(map[string]any{"id": "T003", "accountId": "A001", "amount": -10.00, "type": "withdrawal", "timestamp": time.Now().Unix()})
	_, err := transactionsCollection.CreateOne(ctx, invalidTx)
	if err != nil {
		logger.Info(fmt.Sprintf("Successfully prevented invalid transaction: %v", err))
	} else {
		log.Fatalf("ERROR: Invalid transaction (negative amount) was created!")
	}

	// Complex Query with Joins: Get all transactions for Alice (User U001)
	aliceTransactionsQuery := query.NewQueryBuilder().
		From("LedgerTransaction"). // Main collection
		LeftJoin("Account").On(query.QueryFilter{
			Condition: &query.FilterCondition{Field: "LedgerTransaction.accountId", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{FieldRefVal: &query.FieldReference{Field: "Account.id"}}}},
		).End().
		LeftJoin("User").On(query.QueryFilter{
			Condition: &query.FilterCondition{Field: "Account.userId", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{FieldRefVal: &query.FieldReference{Field: "User.id"}}}},
		).End().
		Where("User.id").Eq("U001").
		Build()

	txResult, err := transactionsCollection.Read(ctx, &aliceTransactionsQuery)
	if err != nil {
		log.Fatalf("Failed to query transactions for Alice: %v", err)
	}

	if txResult.Count > 0 {
		for _, doc := range txResult.Data.([]data.Document) {
			ledgerTx := doc["LedgerTransaction"].(map[string]any)
			user := doc["User"].(map[string]any)
			logger.Info(fmt.Sprintf("Transaction ID: %s, User Name: %s", ledgerTx["id"], user["name"]))
		}
	}
}
```

### Building Queries with the Query Language

`go-anansi` features a powerful natural language-like Query DSL, which maps directly to a structured JSON Query DSL. This allows for expressing complex data retrieval operations in an intuitive yet unambiguous way.

For a complete specification of the query grammar, including all clauses, operators, and functions, please refer to the dedicated [QUERYLANG.md](./QUERYLANG.md) document.

#### Core Query Structure

A query is composed of optional clauses, typically starting with a `WHERE` or `SEARCH` clause.

```
[WHERE <filters> | SEARCH <text_search_config>]
[SORT BY <sort_config>]
[PAGINATE <pagination_config>]
[INCLUDE <projection_inclusion>]
[EXCLUDE <projection_exclusion>]
[COMPUTE <computed_fields>]
[JOIN <join_config>]
[AGGREGATE <aggregation_config>]
[GROUP BY <grouping_fields>]
[HAVING <having_filters>]
[HINT <hint_config>]
```

**Case Sensitivity Rules:**

*   Keywords (WHERE, AND, INCLUDE, AS, etc.): Case-insensitive
*   Identifiers (field names, function names, aliases): Case-sensitive
*   String literals: Case-sensitive and enclosed in double quotes

#### Example Query

Here's a complex example demonstrating many features of the Query DSL:

```
WHERE
  AND(
    status == "active",
    lastPurchaseDate > DATE_SUB(CURRENT_DATE(), "P90D"),
    lifetimeValue > 1000,
    IS_HIGH_RISK_CUSTOMER(customerId)
  )
SORT BY lifetimeValue DESC, lastName ASC
PAGINATE OFFSET 0 LIMIT 25
INCLUDE firstName, lastName, email, region, contactInfo { phone, email }
EXCLUDE password, internalNotes
COMPUTE
  CASE
    WHEN lifetimeValue > 5000 THEN "Platinum"
    WHEN lifetimeValue > 2000 THEN "Gold"
    WHEN lifetimeValue > 1000 THEN "Silver"
    ELSE "Bronze"
  END AS loyaltyStatus,
  DATE_SUB(subscriptionEndDate, "P7D") AS renewalReminder,
  GET_CUSTOMER_SEGMENT(customerId, lifetimeValue) AS segment
JOIN LEFT orders AS customerOrders ON customerOrders.customerId == id
  WHERE customerOrders.orderDate > DATE_SUB(CURRENT_DATE(), "P90D")
  INCLUDE orderId, totalAmount, items { productId, quantity }
AGGREGATE COUNT(*) AS totalOrders, SUM(customerOrders.totalAmount) AS totalSpent
GROUP BY region, loyaltyStatus
HAVING AND(COUNT(*) > 5, totalSpent > 1000)
HINT USE INDEX idx_customer_status, MAX_TIME 30
```

---

## 🏛️ Project Architecture

`go-anansi` is structured around a set of decoupled interfaces and concrete implementations, promoting modularity and extensibility.

### Core Components

*   **`Anansi`**: The top-level entry point (`anansi.go`). It's responsible for the initial setup and configuration of the entire persistence layer, including the `DatabaseInteractor`, `Logger`, `DocumentFactory`, and `Decorators`.
*   **`Persistence` (Interface: `core/persistence/base.Persistence`)**: The main facade for all data operations. It manages `Collections`, `Schemas`, and orchestrates global concerns like event subscriptions and transactions.
    *   **`managedPersistence`**: A decorator ensuring that persistence operations are not performed after the instance is closed.
    *   **`eventsPersistence`**: A decorator that wraps persistence methods with event emission, publishing start, success, and failure events to the `EventBus`.
*   **`Collection` (Interface: `core/persistence/base.Collection`)**: Represents a single collection of documents. It provides the core CRUD operations (`CreateOne`, `CreateMany`, `Read`, `Update`, `Delete`), validation, and collection-specific metadata/subscriptions.
    *   **`managedCollection`**: A decorator for `Collection` that handles automatic metadata injection (versioning, hashing), optimistic locking checks, and resolution of logical-to-physical collection names for joins.
    *   **`eventsCollection`**: A decorator that wraps collection methods with event emission, publishing document-specific events to the `EventBus`.
*   **`CollectionRegistry` (Interface: `core/persistence/base.CollectionRegistry`)**: Manages the lifecycle and versions of all schemas and their corresponding physical collections in the database. It stores this metadata in an internal `_schemas_` collection.
*   **`DatabaseInteractor` (Interface: `core/query.DatabaseInteractor`)**: The core abstraction for database-specific interactions. It defines methods for document CRUD (`InsertDocuments`, `SelectDocuments`, `UpdateDocuments`, `DeleteDocuments`), DDL operations (`CreateCollection`, `DropCollection`, `CreateIndex`, `DropIndex`), and transaction management (`StartTransaction`, `Commit`, `Rollback`, `HasTransaction`).
    *   **`NativeInteractor` (`core/query/native/interactor.go`)**: A generic implementation that uses `database/sql` drivers. It delegates SQL execution to a `QueryExecutor` and SQL generation to a `QueryFactory`.
    *   **`EphemeralDatabaseInteractor` (`core/ephemeral/interactor.go`)**: An in-memory, non-persistent implementation primarily used for testing.
*   **`QueryEngine` (`core/query/engine.go`)**: Responsible for processing `Query` DSL. It partitions queries into parts that can be executed directly by the `DatabaseInteractor` and parts that require in-memory post-processing by the `QueryHelper`, based on the `DatabaseInteractor`'s `Capabilities`.
*   **`SchemaManager` (Interface: `core/query.SchemaManager`)**: Part of `DatabaseInteractor`, specifically handles DDL operations (creating/dropping tables/indexes).
*   **`QueryFactory` (Interface: `core/query/native.QueryFactory`)**: Converts the generic `query.Query` DSL into native SQL statements for a specific database (e.g., `sqlite/query/builder.go`).
*   **`Document` (`core/data/document.go`)**: A flexible `map[string]any` wrapper with added functionalities for metadata management (creation timestamps, versioning, SHA256 hashing for integrity), path-based access, type-safe getters, transformation, and binding to Go structs.
*   **`EventBus` (`github.com/asaidimu/go-events`)**: A central eventing system used throughout the persistence layer to emit lifecycle events for documents, collections, and transactions, enabling decoupled observability and extensions.

### Data Flow

A typical operation, like `Collection.CreateOne`, involves several layers:

1.  **Application Call**: An application calls `anansi.Setup()` once to initialize the `Persistence` layer, then obtains a `Collection` handle, e.g., `productsCollection.CreateOne(ctx, doc)`.
2.  **Event Emission (Start)**: The `eventsCollection` decorator intercepts the call and emits a `DocumentCreateStart` event.
3.  **Managed Collection Logic**: The `managedCollection` decorator takes over:
    *   It wraps the incoming `data.Document` to ensure system-level metadata (creation time, hash, version) is managed correctly.
    *   It performs schema validation against the collection's active schema. If validation fails, it returns an error with `ValidationResult`.
4.  **Base Collection & Transaction Management**: The `baseCollection` receives the (validated, metadata-enriched) document. It then uses the `transaction.Execute` helper:
    *   `transaction.Execute` checks the context for an existing transaction. If none, it starts a new database transaction via `DatabaseInteractor.StartTransaction()`.
    *   It puts the transactional `DatabaseInteractor` into the `context.Context` (keyed by `__transaction__`) for all subsequent nested operations within the callback.
    *   It records any asynchronous operations (`Async` calls) within the transaction's `sync.WaitGroup`.
5.  **Database Interaction**: The `baseCollection` calls `DatabaseInteractor.InsertDocuments(ctx, schema, docs)`.
6.  **SQL Generation**: The `NativeInteractor` (for SQLite) uses its `QueryFactory` (e.g., `sqlite/query/builder.go`) to convert the `InsertDocuments` request into a raw SQL statement with parameters.
7.  **SQL Execution**: The `NativeInteractor` then passes the generated SQL and parameters to its `QueryExecutor` (e.g., `sqlite/executor/executor.go`), which executes the query against the `database/sql` connection or transaction.
8.  **Result Handling**: The `QueryExecutor` returns the raw results, which are then mapped back to `data.Document` instances.
9.  **Transaction Finalization**: After the `CreateOne` operation (and any `Async` operations) complete, `transaction.Execute` commits or rolls back the database transaction based on the success or failure of the operations.
10. **Event Emission (Success/Failure)**: The `eventsCollection` decorator emits `DocumentCreateSuccess` or `DocumentCreateFailed` events, including the final document (or error details).

### Transaction Management (`Transact` and `Async`)

The transaction management system is designed to provide atomic operations and safely coordinate concurrent tasks.

*   **`Persistence.Transact(ctx, callback)`**: This is the primary method for initiating a transactional block. The `callback` function receives a `context.Context` and a `BasePersistence` instance (which is essentially the full `Persistence` API, but scoped to the current transaction).
*   **Nested Transactions**: If `Transact` is called from within an existing transaction's context, it will participate in the parent transaction rather than creating a new one. This ensures that all operations in the nested calls are part of the same atomic unit.
*   **`BasePersistence.Async(ctx, f)`**: This method allows you to execute a function `f` in a new goroutine *within the scope of the current transaction*. The `Transact` block will wait for all `Async` operations to complete before deciding to commit or rollback. If any `Async` operation returns an error, the entire transaction will be rolled back. This is crucial for concurrent, yet transactional, background tasks.

### Extension Points

*   **`DatabaseInteractor`**: Implement this interface for new database backends.
*   **`QueryFactory`**: Provide a custom SQL builder for your `DatabaseInteractor`.
*   **`CollectionDecorators`**: Inject custom logic (e.g., auditing, encryption, advanced validation) around any `Collection` method.
*   **`PersistenceDecorators`**: Inject custom logic around any top-level `Persistence` method.
*   **`EventBus`**: Subscribe to `PersistenceEvents` to react to data changes, build audit logs, invalidate caches, or trigger external systems.

---

## 🛠️ Development & Contributing

### Development Setup

To work on `go-anansi` locally:

1.  **Clone the repository**:
    ```bash
    git clone https://github.com/asaidimu/go-anansi.git
    cd go-anansi
    ```
2.  **Ensure Go modules are tidy**:
    ```bash
    go mod tidy
    ```
3.  **Build the project**:
    ```bash
    go build -v ./...
    ```

### Scripts

The project includes a simple `Makefile` for common development tasks:

*   `make build`: Compiles all Go packages in the module.
*   `make test`: Runs all unit, integration, and E2E tests with verbose output and cleans the test cache.

### Testing

Comprehensive tests are vital for a persistence framework. `go-anansi` includes unit, integration, and end-to-end tests.

To run all tests:

```bash
make test
# or
go clean -testcache && go test -v ./...
```

The project has identified a number of potential test gaps (documented in `test-gap.md`). These areas are actively being addressed to further improve test coverage and robustness, focusing on edge cases, complex interactions, and comprehensive error handling.

### Contributing Guidelines

We welcome contributions! Please follow these general guidelines:

1.  **Fork the repository** and create your branch from `main`.
2.  **Write clear, concise, and well-documented code** in Go.
3.  **Ensure tests are written** for new features and bug fixes, and that all existing tests pass.
4.  **Follow conventional commits** for your commit messages (e.g., `feat(module): add new feature`, `fix(bug): resolve issue`).
5.  **Open a Pull Request** with a detailed description of your changes.

### Issue Reporting

If you find a bug or have a feature request, please open an issue on the [GitHub Issue Tracker](https://github.com/asaidimu/go-anansi/issues).

*   **For bugs**: Provide clear steps to reproduce, expected behavior, actual behavior, and relevant environment details (Go version, database type).
*   **For feature requests**: Describe the use case and the problem it solves.

---

## 📚 Additional Information

## Performance Considerations

Performance depends heavily on your choice of database backend. 

### Transaction Concurrency
The abstraction layer provides consistent transaction semantics across all supported databases, but this comes with different performance characteristics:

- **High-concurrency databases** (PostgreSQL, CockroachDB, MongoDB 4.0+): Full concurrent transaction support
- **Limited-concurrency databases** (SQLite, some NoSQL systems): Transactions are serialized with mutex locks for safety

Choose your database backend based on your concurrency requirements.

### Troubleshooting

*   **`Persistence instance is closed`**: This error typically occurs if you try to perform operations on the `Persistence` or `Collection` interfaces after `p.Close()` has been called. Ensure `p.Close()` is called only when the application is shutting down.
*   **`Collection not found`**: Verify that the schema for the collection was successfully created using `p.CreateCollection()` and that you are using the correct logical name.
*   **`Unique constraint violation`**: This indicates an attempt to insert or update a document with a value in a `unique` field that already exists.
*   **`Hash mismatch`**: This error suggests that a `data.Document` has been modified externally without its internal metadata (hash, version) being updated. `managedCollection` performs an optimistic lock check on update operations, which relies on a valid hash.
*   **Transaction Deadlocks**: While `go-anansi`'s `Transact` and `Async` methods are designed for safe concurrency, complex application logic or long-running `Async` operations can still lead to database-level deadlocks. Monitor database logs for deadlock events and optimize your transactional boundaries.

### FAQ

*   **What databases are supported?**
    Currently, `go-anansi` provides a concrete implementation for **SQLite**. The framework is designed with an abstract `DatabaseInteractor` interface, allowing for easy integration with other SQL (PostgreSQL, MySQL) or NoSQL databases in the future.
*   **How do I define schemas?**
    Schemas are defined using the `schema.SchemaDefinition` struct in Go, or can be loaded from JSON files. They include field types, required flags, uniqueness constraints, and can define nested object structures.
*   **Can I use custom validation logic?**
    Yes, you can define custom predicate functions and register them with the schema's `FunctionMap`. These can then be used in `Constraints` within your schema definitions.
*   **Is `go-anansi` an ORM?**
    While `go-anansi` provides an abstraction over database interactions, schema management, and a query DSL, it's not a full-fledged Object-Relational Mapper (ORM) in the traditional sense (e.g., GORM, Hibernate). It focuses on schema-driven document persistence, offering a more flexible data model similar to NoSQL databases, while being able to work with relational backends.

### Changelog & Roadmap

*   **Changelog**: For a detailed history of changes, including breaking changes and new features across versions, please refer to [CHANGELOG.md](./CHANGELOG.md).
*   **Roadmap**: Future enhancements and planned features include:
    *   **Persistence Layer Health Check**: Proactive verification of database integrity and consistency.
    *   **Enforcing Document Schema at Creation and Runtime Validation**: Enhancements to `Collection.Document()` to return schema-aware documents with immediate validation feedback.
    *   **Enhanced Error Handling & Messaging**: Further standardization of error structures for clarity and debugging.
    *   **Comprehensive Logging & Tracing**: More granular and configurable logging with distributed tracing integration points.
    *   **Simplified Configuration**: Streamlining setup processes and providing more sensible defaults.

### License

`go-anansi` is released under the **Anansi Platform Proprietary License**. This is a source-available license and **NOT an open-source license**. Commercial use of the Software requires a separate, written commercial license from CyberSync Printers & Stationers Kenya.

For full details, please see [LICENSE.md](./LICENSE.md).

### Acknowledgments

This project is developed by CyberSync Printers & Stationers Kenya. We are grateful for the inspiration from various database and Go projects that informed `go-anansi`'s design principles.
