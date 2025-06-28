# Anansi (Go Implementation)

[![Go Reference](https://pkg.go.dev/badge/github.com/asaidimu/go-anansi.svg)](https://pkg.go.dev/github.com/asaidimu/go-anansi)
[![Build Status](https://github.com/asaidimu/go-anansi/workflows/Test%20Workflow/badge.svg)](https://github.com/asaidimu/go-anansi/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Anansi is a comprehensive toolkit for defining, versioning, migrating, and persisting structured data, enabling schema-driven development with powerful runtime validation and adaptable storage layers. This repository provides the **Go implementation** of the Anansi persistence and query framework.

---

## 📚 Table of Contents

*   [Overview & Features](#-overview--features)
*   [Installation & Setup](#-installation--setup)
*   [Usage Documentation](#-usage-documentation)
    *   [Defining Schemas](#defining-schemas)
    *   [Initializing Persistence](#initializing-persistence)
    *   [Creating Collections](#creating-collections)
    *   [Basic CRUD Operations](#basic-crud-operations)
    *   [Data Validation](#data-validation)
    *   [Event Subscriptions](#event-subscriptions)
    *   [Transaction Management](#transaction-management)
    *   [Advanced Querying with QueryDSL](#advanced-querying-with-querydsl)
    *   [In-memory Go Functions (Computed Fields & Custom Filters)](#in-memory-go-functions-computed-fields--custom-filters)
*   [Project Architecture](#-project-architecture)
*   [Development & Contributing](#-development--contributing)
*   [Roadmap & Future Enhancements](#-roadmap--future-enhancements)
*   [Additional Information](#-additional-information)
    *   [Troubleshooting](#troubleshooting)
    *   [FAQ](#faq)
    *   [License](#license)
    *   [Acknowledgments](#acknowledgments)

---

## ✨ Overview & Features

Anansi is designed to bring a robust, schema-first approach to data persistence in Go applications. By externalizing data models into declarative JSON schema definitions, it allows for dynamic table creation, powerful querying, and a clear pathway for future data migrations and versioning. This framework aims to provide a high degree of flexibility and extensibility by abstracting the underlying storage mechanism.

The current implementation focuses on providing a production-ready SQLite adapter, demonstrating the core capabilities of the Anansi framework. While SQLite is the primary target for initial development, the architecture is built to support other database systems through a pluggable `persistence.DatabaseInteractor` interface. This project is still under active development, with several advanced features defined in interfaces awaiting full implementation.

**Key Features:**

*   **Schema-Driven Data Modeling**: Define your data structures using declarative JSON schemas (`schema.SchemaDefinition`) that include field types, constraints (required, unique, default), and indexing.
*   **Pluggable Persistence Layer**: Anansi is built around the `persistence.DatabaseInteractor` interface, allowing easy integration with various database systems. The initial release provides a comprehensive SQLite adapter.
*   **Declarative Query DSL**: Construct complex queries using a fluent `query.QueryBuilder` API, which is then translated into efficient SQL statements by the underlying query generator.
*   **Comprehensive CRUD Operations**: Perform `Create`, `Read`, `Update`, and `Delete` operations on your collections through a unified API.
*   **Nested JSON Field Querying**: Seamlessly query and filter on data stored within JSON object fields in your database, treating them as first-class fields using `json_extract` for SQLite.
*   **In-memory Go-based Processing**: Extend query capabilities with custom Go functions for:
    *   **Computed Fields**: Define new fields dynamically by applying Go logic to retrieved data.
    *   **Custom Filters**: Implement complex, non-SQL-standard filtering logic in Go after initial database retrieval.
*   **Table & Index Management**: Programmatically create and manage database tables and indexes directly from your schema definitions, supporting `IF NOT EXISTS`, `DROP TABLE IF EXISTS`, and various index types.
*   **Atomic Insert Operations**: Utilizes `RETURNING *` for `INSERT` statements (where supported, e.g., SQLite 3.35+) to atomically fetch inserted records, including auto-generated IDs and default values.
*   **Robust Data Validation**: Validate data against defined schema constraints at runtime.
*   **Transaction Management**: Support for atomic database operations within a transaction.
*   **Event System**: Built-in event emission (`persistence.PersistenceEvent`) for various data operations, allowing for subscription and reactive programming.
*   **Structured Logging**: Integrates `go.uber.org/zap` for detailed debugging and operational insights throughout the persistence layer.

---

## 🚀 Installation & Setup

### Prerequisites

Before you begin, ensure you have the following installed:

*   **Go**: Version `1.24.4` or newer (as specified in `go.mod`). You can download it from [golang.org/dl](https://golang.org/dl/).
*   **SQLite3**: The `github.com/mattn/go-sqlite3` driver requires the SQLite C library to be present on your system. Most Linux distributions and macOS come with it pre-installed. For Windows, you might need to install it manually (e.g., via MSYS2 or by downloading pre-compiled binaries).

### Installation Steps

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/asaidimu/go-anansi.git
    cd go-anansi
    ```
2.  **Download dependencies:**
    ```bash
    go mod tidy
    ```
3.  **Build the project (optional, for executable):**
    ```bash
    go build -v ./...
    # Or, to build the main executable:
    go build -o anansi-example main.go
    ```

### Verification

To verify your installation and see Anansi in action, run the example `main.go` file:

```bash
go run main.go
```

You should see output similar to this:

```
Starting fresh: removed existing user.db (if any).
Defining User schema from JSON string...
User schema unmarshaled successfully from JSON.
Creating 'users' table...
'users' table created successfully.
Document added to collection 'users', {document:create:success}
Document added to collection 'users', {document:create:success}
Document added to collection 'users', {document:create:success}
Inserting sample data...
Sample data inserted successfully.

Querying data from 'users' table:
-------------------------------------------------------------------
ID         Name                 Email                     Age   Active    
-------------------------------------------------------------------
2          Alice Smith          alice2@example.com        27    true      
3          Alex Smith           alice3@example.com        28    false     
-------------------------------------------------------------------
Database created successfully at: user.db
You can inspect this database file using the 'sqlite3' command-line tool:
1. Open your terminal.
2. Navigate to the directory where 'main.go' and 'user.db' are located.
3. Run: sqlite3 user.db
4. Inside the sqlite3 prompt, you can run SQL commands:
    - .tables (to list tables)
    - .schema users (to view table schema)
    - SELECT * FROM users; (to view data)
    - .quit (to exit)
-------------------------------------------------------------------
ID         Name                 Email                     Age   Active    
-------------------------------------------------------------------
2          Alice Smith          alice2@example.com        27    true      
3          Alex Smith           alice3@example.com        28    false     
-------------------------------------------------------------------
Database connection closed.
Dropped 'users' table...
```

This confirms that the application can connect to SQLite, define a schema, create a table, insert data, query it, and manage transactions using the Anansi framework.

---

## 💡 Usage Documentation

Anansi operates on the principle of defining your data structure as a schema, then using that schema to interact with the persistence layer.

### Defining Schemas

Schemas are defined using the `schema.SchemaDefinition` struct, which can be easily unmarshaled from JSON. This allows for externalizing your data models.

**Example (`userSchemaJSON` from `main.go`):**

```json
{
    "name": "users",
    "version": "1.0.0",
    "description": "Schema for user profiles",
    "fields": {
        "id": {
            "name": "id",
            "type": "integer",
            "required": false,
            "unique": true,
            "description": "Unique identifier for the user"
        },
        "name": {
            "name": "name",
            "type": "string",
            "required": true,
            "description": "Full name of the user"
        },
        "email": {
            "name": "email",
            "type": "string",
            "required": true,
            "unique": true,
            "description": "Email address, must be unique"
        },
        "age": {
            "name": "age",
            "type": "integer",
            "required": false,
            "description": "Age of the user (optional)"
        },
        "is_active": {
            "name": "is_active",
            "type": "boolean",
            "required": true,
            "default": true,
            "description": "User account active status"
        }
    },
    "indexes": [
        {
            "name": "pk_user_id",
            "fields": ["id"],
            "type": "primary"
        },
        {
            "name": "idx_user_email",
            "fields": ["email"],
            "type": "unique"
        }
    ]
}
```

### Initializing Persistence

To interact with your database, you'll need to initialize a `persistence.DatabaseInteractor` (e.g., `sqlite.SQLiteInteractor`) and then pass it to `persistence.NewPersistence`.

```go
package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/asaidimu/go-anansi/core/persistence"
	"github.com/asaidimu/go-anansi/core/schema"
	"github.com/asaidimu/go-anansi/sqlite"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"go.uber.org/zap"
)

func main() {
	dbFileName := "my_app.db"
	if err := os.Remove(dbFileName); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to remove existing database file %s: %v", dbFileName, err)
	}

	db, err := sql.Open("sqlite3", dbFileName)
	if err != nil {
		log.Fatalf("Failed to open database connection: %v", err)
	}
	defer db.Close()

	// Initialize with a logger (optional) and default interactor options
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Create an SQLite DatabaseInteractor.
	// The interactor handles low-level DB operations and DDL.
	interactor := sqlite.NewSQLiteInteractor(db, logger, sqlite.DefaultInteractorOptions(), nil)

	// Initialize the core persistence service.
	// This service manages collections, schemas, and orchestrates operations.
	// An empty schema.FunctionMap is passed for now; see "In-memory Go Functions" section.
	persistenceService, err := persistence.NewPersistence(interactor, schema.FunctionMap{})
	if err != nil {
		log.Fatalf("Failed to initialize persistence: %v", err)
	}
	fmt.Println("Persistence service initialized.")

	// ... now use persistenceService to create collections, etc.
}
```

### Creating Collections

Once `persistence.NewPersistence` is initialized, you can create a collection (which maps to a database table) using your schema definition.

```go
// userSchema is your schema.SchemaDefinition unmarshaled from JSON
collection, err := persistenceService.Create(userSchema)
if err != nil {
	log.Fatalf("Failed to create collection 'users': %v", err)
}
fmt.Println("'users' table created successfully.")
```

### Basic CRUD Operations

Anansi provides methods for common database operations.

#### Create (Insert)

```go
import (
	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-anansi/core/schema"
)

// Single record insert
userData := map[string]any{
    "name":      "Alice Smith",
    "email":     "alice@example.com",
    "age":       30,
    "is_active": true,
}

// The Create method accepts map[string]any or []map[string]any
insertedResult, err := collection.Create(userData) // Returns *query.QueryResult
if err != nil {
    log.Fatalf("Failed to insert user: %v", err)
}
fmt.Printf("Inserted user ID: %v\n", insertedResult.Data.(schema.Document)["id"])

// Batch inserts
batchData := []map[string]any{
    {"name": "Bob Johnson", "email": "bob@example.com", "age": 25, "is_active": true},
    {"name": "Charlie Brown", "email": "charlie@example.com", "age": 35, "is_active": false},
}
insertedBatchResult, err := collection.Create(batchData)
if err != nil {
    log.Fatalf("Failed to batch insert users: %v", err)
}
fmt.Printf("Batch inserted %d users.\n", insertedBatchResult.Count)
```

#### Read (Query)

Read operations leverage the `query.QueryBuilder` to construct complex queries.

```go
import "github.com/asaidimu/go-anansi/core/query"

// Query all active users younger than 28, excluding the 'age' field from the output.
q := query.NewQueryBuilder().
    WhereGroup(query.LogicalOperatorAnd).
        Where("is_active").Eq(true).
        Where("age").Lt(28).
    End().
    Select().
        Exclude("age").
    End().
    Build()

result, err := collection.Read(&q) // Read takes a pointer to QueryDSL
if err != nil {
    log.Fatalf("Failed to read data: %v", err)
}

// Results are []schema.Document (map[string]any)
rows := result.Data.([]schema.Document)
for _, row := range rows {
    // Note: 'age' is excluded by the projection in this query
    fmt.Printf("User: ID=%v, Name=%v, Email=%v, Active=%v\n",
        row["id"], row["name"], row["email"], row["is_active"])
}
```

#### Update

```go
import "github.com/asaidimu/go-anansi/core/persistence"

// Update the user whose email is 'alice@example.com' to have age 31 and name 'Alice M. Smith'
updates := map[string]any{"age": 31, "name": "Alice M. Smith"}
filter := query.NewQueryBuilder().Where("email").Eq("alice@example.com").Build().Filters

updateParams := &persistence.CollectionUpdate{
	Data:   updates,
	Filter: filter,
}

rowsAffected, err := collection.Update(updateParams)
if err != nil {
    log.Fatalf("Failed to update user: %v", err)
}
fmt.Printf("Updated %d rows.\n", rowsAffected)
```

#### Delete

```go
// Delete inactive users
filter := query.NewQueryBuilder().Where("is_active").Eq(false).Build().Filters

// By default, DELETE requires a filter for safety.
// To delete all records (DANGER!), set unsafe to true.
rowsAffected, err := collection.Delete(filter, false)
if err != nil {
    log.Fatalf("Failed to delete users: %v", err)
}
fmt.Printf("Deleted %d rows.\n", rowsAffected)

// Drop an entire collection (table)
deleted, err := persistenceService.Delete("users")
if err != nil {
	log.Fatalf("Failed to drop collection: %v", err)
}
fmt.Printf("Collection 'users' deleted: %t\n", deleted)
```

### Data Validation

Anansi allows you to validate data against a collection's schema constraints at runtime.

```go
import "github.com/asaidimu/go-anansi/core/schema"

invalidUserData := map[string]any{
    "name": "Invalid User",
    // 'email' is required but missing
    "age": 20,
}

validationResult, err := collection.Validate(invalidUserData, false) // `false` for strict validation
if err != nil {
    fmt.Printf("Error during validation: %v\n", err)
}

if !validationResult.Valid {
    fmt.Println("Validation failed! Issues found:")
    for _, issue := range validationResult.Issues {
        fmt.Printf("  Code: %s, Message: %s, Path: %s, Severity: %s\n",
            issue.Code, issue.Message, issue.Path, issue.Severity)
    }
} else {
    fmt.Println("Validation successful!")
}
```

### Event Subscriptions

Anansi provides an event system allowing you to subscribe to various persistence lifecycle events.

```go
import (
	"context"
	"fmt"
	"github.com/asaidimu/go-anansi/core/persistence"
)

// Register a subscription to be notified when a document is created successfully
subscriptionId := collection.RegisterSubscription(persistence.RegisterSubscriptionOptions{
    Event: persistence.DocumentCreateSuccess,
    Label: persistence.StringPtr("log_new_user"),
    Description: persistence.StringPtr("Logs details of newly created users."),
    Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
        if event.Collection != nil {
            fmt.Printf("EVENT: Document added to collection '%s'. Input: %+v, Output: %+v\n",
                *event.Collection, event.Input, event.Output)
        }
        return nil
    },
})

fmt.Printf("Subscribed to DocumentCreateSuccess with ID: %s\n", subscriptionId)

// Later, to unsubscribe:
// collection.UnregisterSubscription(subscriptionId)

// To get all active subscriptions for this collection:
// subs, _ := collection.Subscriptions()
// for _, sub := range subs {
//     fmt.Printf("Active Subscription: ID=%s, Event=%s, Label=%s\n", *sub.Id, sub.Event, *sub.Label)
// }
```

### Transaction Management

Anansi supports executing multiple operations within a single database transaction.

```go
import "context"

_, err = persistenceService.Transact(func(tx persistence.PersistenceTransactionInterface) (any, error) {
    // Get a collection instance operating within this transaction
    txCollection, err := tx.Collection("users")
    if err != nil {
        return nil, fmt.Errorf("failed to get transactional collection: %w", err)
    }

    // Perform operations within the transaction.
    // If any operation fails, the entire transaction will be rolled back.
    _, err = txCollection.Create(map[string]any{
        "name":      "Transaction User 1",
        "email":     "tx1@example.com",
        "is_active": true,
    })
    if err != nil {
        return nil, fmt.Errorf("tx create 1 failed: %w", err)
    }

    _, err = txCollection.Create(map[string]any{
        "name":      "Transaction User 2",
        "email":     "tx2@example.com",
        "is_active": false,
    })
    if err != nil {
        return nil, fmt.Errorf("tx create 2 failed: %w", err)
    }

    // This data will only be visible in the database if the transaction commits successfully.
    fmt.Println("Transaction operations completed, preparing to commit.")
    return nil, nil // Return nil, nil for success, or an error to rollback
})

if err != nil {
    log.Fatalf("Transaction failed and was rolled back: %v", err)
} else {
    fmt.Println("Transaction committed successfully.")
}
```

### Advanced Querying with QueryDSL

The `query.QueryBuilder` provides a rich API for constructing declarative queries:

```go
import "github.com/asaidimu/go-anansi/core/query"

// Example: Get users, order by age descending, with pagination, and select specific fields.
queryDSL := query.NewQueryBuilder().
    Where("age").Gt(20). // Filter: age > 20
    OrderByDesc("age"). // Sort by age descending
    Limit(10).Offset(0). // Paginate: 10 results, from start
    Select().
        Include("name", "email"). // Project: only name and email
    End().
    Build()

result, err := collection.Read(&queryDSL) // Read takes a pointer to QueryDSL
if err != nil {
    log.Fatalf("Failed to read data with advanced query: %v", err)
}
fmt.Println("--- Advanced Query Results ---")
for _, r := range result.Data.([]schema.Document) {
    fmt.Printf("Name: %v, Email: %v\n", r["name"], r["email"])
}
```

**Supported QueryDSL Features:**

*   **Filters**:
    *   Comparison Operators: `Eq`, `Neq`, `Lt`, `Lte`, `Gt`, `Gte`, `In`, `Nin`, `Contains`, `NotContains`, `StartsWith`, `EndsWith`, `Exists`, `NotExists`.
    *   Logical Operators: `WhereGroup` with `And`, `Or` for nested conditions.
*   **Sorting**: `OrderByAsc`, `OrderByDesc` for single or multiple fields.
*   **Pagination**: `Limit`, `Offset` for traditional pagination. (Cursor-based pagination is defined in interfaces but currently not fully implemented in SQL generation.)
*   **Projection**: `Select().Include(...)` or `Select().Exclude(...)` to control returned fields.
    *   `IncludeNested`: Placeholder for future nested document projection.
    *   `AddComputed`: For Go-based computed fields (see below).
    *   `AddCase`: For Go-based `CASE` expressions.
*   **Joins**: `InnerJoin`, `LeftJoin`, `RightJoin`, `FullJoin`. (Defined in interfaces but currently not fully implemented in SQL generation.)
*   **Aggregations**: `Count`, `Sum`, `Avg`, `Min`, `Max`. (Defined in interfaces but currently not fully implemented in SQL generation.)
*   **Window Functions**: `Window` with `PartitionBy`, `OrderBy`. (Defined in interfaces but currently not fully implemented in SQL generation.)
*   **Query Hints**: `UseIndex`, `ForceIndex`, `NoIndex`, `MaxExecutionTime`. (Defined in interfaces but currently not fully implemented in SQL generation.)

### In-memory Go Functions (Computed Fields & Custom Filters)

Anansi allows you to register custom Go functions to perform operations that are either too complex for standard SQL or operate on data after initial database retrieval (e.g., on JSON fields that SQLite doesn't natively support querying efficiently). These functions are registered via the `schema.FunctionMap` when initializing `persistence.NewPersistence`.

```go
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/asaidimu/go-anansi/core/persistence"
	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-anansi/core/schema"
	"github.com/asaidimu/go-anansi/sqlite"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

func main() {
	dbFileName := "go_functions.db"
	if err := os.Remove(dbFileName); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to remove existing database file %s: %v", dbFileName, err)
	}
	db, err := sql.Open("sqlite3", dbFileName)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 1. Define a schema with a JSON 'metadata' field
	schemaJSON := `{
		"name": "items",
		"version": "1.0.0",
		"fields": {
			"id": {"name": "id", "type": "integer", "required": false, "unique": true},
			"name": {"name": "name", "type": "string", "required": true},
			"metadata": {"name": "metadata", "type": "object"}
		},
		"indexes": [{"name": "pk_item_id", "fields": ["id"], "type": "primary"}]
	}`
	var itemSchema schema.SchemaDefinition
	if err := json.Unmarshal([]byte(schemaJSON), &itemSchema); err != nil {
		log.Fatalf("Failed to unmarshal item schema: %v", err)
	}

	// 2. Prepare the FunctionMap with your custom Go functions
	customFunctions := schema.FunctionMap{
		"item_display": query.ComputeFunction(func(row schema.Document, args query.FilterValue) (any, error) {
			name, ok := row["name"].(string)
			if !ok {
				return nil, fmt.Errorf("name is not a string")
			}
			// Access nested JSON field 'metadata.category'
			if meta, ok := row["metadata"].(map[string]any); ok {
				if category, ok := meta["category"].(string); ok {
					return fmt.Sprintf("%s (%s)", name, category), nil
				}
			}
			return name, nil // Fallback if no category
		}),
		"is_heavy": query.PredicateFunction(func(doc schema.Document, field string, args query.FilterValue) (bool, error) {
			// This custom filter checks if 'metadata.weight_kg' > 1.5
			if meta, ok := doc["metadata"].(map[string]any); ok {
				if weight, ok := meta["weight_kg"].(float64); ok {
					return weight > 1.5, nil
				}
			}
			return false, nil // Not heavy or no weight defined
		}),
	}

	// 3. Initialize Persistence with the custom functions
	interactor := sqlite.NewSQLiteInteractor(db, logger, sqlite.DefaultInteractorOptions(), nil)
	persistenceService, err := persistence.NewPersistence(interactor, customFunctions)
	if err != nil {
		log.Fatalf("Failed to initialize persistence: %v", err)
	}

	collection, err := persistenceService.Create(itemSchema)
	if err != nil {
		log.Fatalf("Failed to create items collection: %v", err)
	}

	// Insert some data with nested JSON
	collection.Create(map[string]any{"name": "Laptop", "metadata": map[string]any{"category": "electronics", "weight_kg": 1.8}})
	collection.Create(map[string]any{"name": "Desk Chair", "metadata": map[string]any{"category": "furniture", "material": "mesh"}})
	collection.Create(map[string]any{"name": "Mouse", "metadata": map[string]any{"category": "electronics", "wireless": true}})

	// Query using the registered Compute Function
	fmt.Println("\nQuerying with Go functions:")
	qWithGoFuncs := query.NewQueryBuilder().
		Where("id").Gt(0). // Base filter for all data (SQL side)
		Select().
			Include("id", "name", "metadata"). // Ensure required fields are selected from DB
			AddComputed("item_display_name", "item_display"). // Use registered compute function
		End().
		Build()

	result, err := collection.Read(&qWithGoFuncs)
	if err != nil {
		log.Fatalf("Failed to query with computed field: %v", err)
	}
	fmt.Println("--- Results with Computed Field ---")
	for _, r := range result.Data.([]schema.Document) {
		fmt.Printf("ID: %v, Name: %v, Display: %v, Metadata: %v\n", r["id"], r["name"], r["item_display_name"], r["metadata"])
	}

	// Query using the custom Go filter
	qWithGoFilter := query.NewQueryBuilder().
		Where("id").Custom("is_heavy", true). // Use registered Go filter function
		Select().
			Include("id", "name", "metadata"). // Ensure metadata is fetched for the Go filter to work
		End().
		Build()

	resultFilter, err := collection.Read(&qWithGoFilter)
	if err != nil {
		log.Fatalf("Failed to query with custom filter: %v", err)
	}
	fmt.Println("\n--- Results with Custom Filter (is_heavy) ---")
	for _, r := range resultFilter.Data.([]schema.Document) {
		fmt.Printf("ID: %v, Name: %v\n", r["id"], r["name"])
	}
}
```

**Important Note on Go Functions**:
Go-based filters and computed fields operate on data *after* it has been retrieved from the database. This means they are executed **in-memory**. For very large datasets, using highly selective SQL filters first is crucial for performance. Go functions are best suited for complex logic that cannot be expressed easily in SQL, or for operations on `TEXT` fields containing JSON that would otherwise require complex JSON functions in SQL.

---

## 🏗️ Project Architecture

Anansi is structured to be modular and extensible, separating core persistence concepts from their concrete database implementations.

### Core Components

*   **`core/persistence/`**: This package defines the top-level interfaces (`PersistenceInterface`, `PersistenceCollectionInterface`, `PersistenceTransactionInterface`) for interacting with the persistence layer. It also houses the `Executor` (orchestrates queries) and the event system.
*   **`core/query/`**: This package contains the declarative `QueryDSL` structure and the fluent `QueryBuilder` API for constructing queries. It also defines the `QueryGenerator` interface for translating DSL to SQL, and the `DataProcessor` for in-memory Go-based filtering and computed fields.
*   **`core/schema/`**: This package defines the foundational `SchemaDefinition` for describing data models, including field types, constraints, indexes, and migration primitives. It also includes the `Validator` for schema-based data validation.
*   **`sqlite/`**: This package provides the concrete implementation for SQLite databases.
    *   `SQLiteInteractor`: Implements the `persistence.DatabaseInteractor` interface, handling low-level SQL execution (DDL and DML) and transaction management for SQLite.
    *   `SqliteQuery`: Implements the `query.QueryGenerator` interface, translating Anansi's `QueryDSL` into SQLite-compatible SQL, including handling nested JSON field access via `json_extract`.

### Data Flow for Queries (`collection.Read`)

1.  A user constructs a `query.QueryDSL` object using `query.NewQueryBuilder()`.
2.  The `PersistenceCollection.Read()` method receives the `query.QueryDSL`.
3.  It delegates the operation to the internal `Executor`.
4.  The `Executor` analyzes the `QueryDSL` to determine all fields required from the database, including dependencies for any registered Go-based functions.
5.  It then uses the `query.QueryGenerator` (implemented by `sqlite.SqliteQuery`) to translate the SQL-executable parts of the `QueryDSL` into an SQL query string and parameters. This includes handling field path translation for nested JSON objects.
6.  The `Executor`'s `DatabaseInteractor` (implemented by `sqlite.SQLiteInteractor`) executes this SQL query against the `sql.DB` connection.
7.  Retrieved rows are read from `*sql.Rows` and converted into a generic `schema.Document` (map[string]any) slice, performing schema-aware type conversions (e.g., SQLite `INTEGER` to Go `bool` for `FieldTypeBoolean`).
8.  **Post-SQL Processing**: The `Executor` (specifically its internal `DataProcessor`) then applies any registered **Go-based filter functions** and **Go-based computed field functions** on these in-memory `schema.Document` objects.
9.  Finally, the `Executor` applies the final projection (include/exclude fields) as specified in the `QueryDSL`.
10. The processed `query.QueryResult` is returned to the caller.

### Extension Points

Anansi is designed with extensibility in mind through its interfaces:

*   **`persistence.DatabaseInteractor`**: To support a new SQL database (e.g., PostgreSQL, MySQL), you would implement this interface to define how DDL and DML operations are performed for that specific database system.
*   **`query.QueryGeneratorFactory` / `query.QueryGenerator`**: For each new database, a new `QueryGenerator` implementation would be required to transform the generic `QueryDSL` into the database's specific SQL dialect.
*   **`schema.FunctionMap`**: Allows injecting custom Go functions for computed fields and advanced filtering logic directly into the query processing pipeline.

### Static Type Mapping & Code Generation (Planned Enhancement)

While Anansi currently operates with dynamic data structures (`map[string]any`), we're planning to add optional static type mapping capabilities that would position Anansi as a unique hybrid persistence framework in the Go ecosystem.

**Planned Functionality:**

*   **Automatic Struct Generation**: Generate Go structs directly from your schema definitions, complete with appropriate tags and type annotations:
    ```go
    // Generated from your users schema definition
    type User struct {
        ID       int64  `json:"id" anansi:"primary_key" db:"id"`
        Name     string `json:"name" anansi:"required" db:"name"`
        Email    string `json:"email" anansi:"required,unique" db:"email"`
        Age      *int   `json:"age" anansi:"optional" db:"age"`
        IsActive bool   `json:"is_active" anansi:"required,default=true" db:"is_active"`
    }
    ```
*   **Reflection-Based Mapping**: Seamlessly convert between `[]schema.Document` results and strongly-typed structs, with intelligent type coercion and null handling.
*   **Dual Interface Support**: Continue supporting both dynamic and static approaches within the same application:
    ```go
    // Dynamic approach (current)
    userData := map[string]any{"name": "Alice", "email": "alice@example.com"}
    result, _ := collection.Create(userData)

    // Static approach (planned)
    user := User{Name: "Alice", Email: "alice@example.com"}
    result, _ := collection.CreateTyped(&user) // Planned method

    // Mixed querying
    dynamicResults, _ := collection.Read(query)         // Returns *query.QueryResult with Data as []schema.Document
    typedResults, _ := collection.ReadAs[User](query)   // Returns []User (planned method)
    ```

**Strategic Benefits:**

*   **Best of Both Worlds**: Anansi would uniquely offer the flexibility of schema-driven development with the safety and performance of static typing when desired.
*   **Migration Path**: Applications can start with dynamic schemas for rapid prototyping and evolve to static types for production stability, all within the same framework.
*   **Runtime Schema Evolution**: Unlike traditional ORMs, your schemas remain the source of truth and can evolve at runtime, even when using generated structs.
*   **Enhanced Developer Experience**: Generated structs would provide compile-time safety, better IDE support, and improved refactoring capabilities while maintaining Anansi's core architectural principles.

**Implementation Considerations:**
This enhancement would maintain backward compatibility with existing dynamic operations while adding:

*   Code generation tooling integrated with your build process.
*   Intelligent type mapping between schema definitions and Go types.
*   Validation integration leveraging schema constraints.
*   Performance optimizations through cached reflection operations.
*   Relationship mapping for future foreign key support.

The goal is to create a Go persistence framework that seamlessly bridges the gap between dynamic, schema-driven development and traditional static ORM approaches, giving developers the flexibility to choose the right tool for each use case within a single, cohesive framework.

---

## 🛠️ Development & Contributing

Contributions are welcome! Please follow these guidelines.

### Development Setup

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/asaidimu/go-anansi.git
    cd go-anansi
    ```
2.  **Install dependencies:**
    ```bash
    go mod tidy
    ```
3.  **Run tests to ensure everything is working:**
    ```bash
    make test
    ```

### Scripts

The project includes a `Makefile` for common development tasks:

*   `make build`: Builds the entire Go module, compiling all packages.
*   `make test`: Runs all unit tests with verbose output.
*   `make clean`: Removes generated executables and temporary files.

### Testing

Tests are written using Go's built-in `testing` package and `github.com/stretchr/testify` for assertions.

*   To run all tests:
    ```bash
    go test -v ./...
    ```
*   To run tests for a specific package (e.g., `sqlite`):
    ```bash
    go test -v ./sqlite
    ```

### Contributing Guidelines

As this project is still a work in progress, detailed contribution guidelines will be expanded. For now, please consider the following:

1.  **Fork the repository** and create your branch from `main`.
2.  **Write clear, concise commit messages** following a conventional style (e.g., `feat: add new feature`, `fix: resolve bug`).
3.  **Ensure existing tests pass**, and add new tests for your features or bug fixes.
4.  **Adhere to Go best practices** and clean code principles.
5.  **Open a Pull Request** with a clear description of your changes.

### Issue Reporting

If you find a bug or have a feature request, please open an issue on the [GitHub Issue Tracker](https://github.com/asaidimu/go-anansi/issues).

---

## 🧭 Roadmap & Future Enhancements

Anansi is under active development. The current focus is on solidifying the core persistence logic and the SQLite adapter. Many capabilities are already defined in the `core` interfaces but are currently stubbed or partially implemented in the `persistence` layer.

**Key areas for future development include:**

*   **Schema Versioning & Migrations**: Full implementation of `core/schema.Migration`, `core/schema.SchemaMigrationHelper`, and associated persistence methods (`Collection.Migrate`, `Collection.Rollback`) to allow declarative schema evolution and data transformation between versions.
*   **Events & Observability**: Further development of the `core/persistence.PersistenceEvent` system, including triggers and a comprehensive metadata API (`core/persistence.MetadataFilter`, `core/persistence.CollectionMetadata`) for real-time insights and reactive programming.
*   **Scheduled Tasks**: Full implementation of `core/persistence.TaskInfo` to enable scheduling and execution of background jobs directly managed by the persistence layer.
*   **Advanced QueryDSL Features**:
    *   Full support for cursor-based pagination.
    *   Aggregation functions (`Count`, `Sum`, `Avg`, `Min`, `Max`).
    *   Window functions (`Rank`, `Row Number`).
    *   Join operations (`InnerJoin`, `LeftJoin`, etc.).
    *   Query Hints for performance optimization.
*   **More Database Adapters**: Develop `persistence.DatabaseInteractor` and `query.QueryGenerator` implementations for other popular databases (e.g., PostgreSQL, MySQL, NoSQL databases).

---

## ℹ️ Additional Information

### Troubleshooting

*   **`database/sql: unknown driver "sqlite3"`**: Ensure you have imported the SQLite driver with a blank import: `_ "github.com/mattn/go-sqlite3"`.
*   **`... not found: [database/sqlite3]` or similar build errors**: This often means the SQLite C library is not available on your system or not found by the Go toolchain.
    *   **Linux**: `sudo apt-get install sqlite3 libsqlite3-dev` (Debian/Ubuntu) or `sudo yum install sqlite-devel` (RHEL/CentOS).
    *   **macOS**: SQLite3 is usually pre-installed. If not, try `brew install sqlite`.
    *   **Windows**: This can be more complex. You might need to install MSYS2 and then `pacman -S mingw-w64-x86_64-sqlite3`, ensuring your Go environment uses the `mingw-w64` toolchain.
*   **`SQL logic error or missing database INSERT ... RETURNING`**: Your SQLite version might be too old. The `RETURNING` clause requires SQLite version `3.35.0` or newer.

### FAQ

*   **What is Anansi?**
    Anansi is a Go framework designed for schema-driven data persistence. It allows you to define your data models declaratively and interact with various databases through a unified API, supporting advanced querying and post-database processing with Go functions.
*   **What databases are supported?**
    Currently, a robust SQLite adapter is implemented and actively developed. The architecture is pluggable, making it possible to support other SQL and NoSQL databases in the future.
*   **Why use Go functions for filters and computed fields?**
    This feature provides flexibility to implement complex business logic directly in Go, especially for scenarios where standard SQL is insufficient or less performant (e.g., complex calculations, custom string matching, or operations on semi-structured JSON data where full native JSON querying is not available or efficient in the underlying database). It acts as an in-memory post-processing step, augmenting the core SQL capabilities.

### License

This project is licensed under the MIT License - see the [LICENSE.md](LICENSE.md) file for details.

### Acknowledgments

Developed by Saidimu.