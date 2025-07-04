# Anansi (Go Implementation)

[![Go Reference](https://pkg.go.dev/badge/github.com/asaidimu/go-anansi/v6.svg)](https://pkg.go.dev/github.com/asaidimu/go-anansi/v6)
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
    ```

### Verification

To verify your installation and see Anansi in action, run the basic example:

```bash
go run examples/basic/main.go
```

You should see output similar to this, demonstrating schema definition, collection creation, and basic CRUD operations:

```
INFO Anansi persistence service initialized for inventory tracking. {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO 'inventory_items' collection created successfully. {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Adding new items to inventory... {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO --- Current Inventory --- {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Found multiple items: {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Item {"timestamp": "2025-06-28T10:00:00.000Z", "ID": "...", "Name": "Keyboard", "Quantity": 25, "Last Updated": {}}
INFO Item {"timestamp": "2025-06-28T10:00:00.000Z", "ID": "...", "Name": "Laptop", "Quantity": 10, "Last Updated": {}}
INFO Item {"timestamp": "2025-06-28T10:00:00.000Z", "ID": "...", "Name": "Mouse", "Quantity": 50, "Last Updated": {}}
INFO ------------------------- {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Updating quantity for 'Laptop'... {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Laptop quantity updated {"timestamp": "2025-06-28T10:00:00.000Z", "rows_affected": 1}
INFO --- Current Inventory --- {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Found multiple items: {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Item {"timestamp": "2025-06-28T10:00:00.000Z", "ID": "...", "Name": "Keyboard", "Quantity": 25, "Last Updated": {}}
INFO Item {"timestamp": "2025-06-28T10:00:00.000Z", "ID": "...", "Name": "Laptop", "Quantity": 8, "Last Updated": {}}
INFO Item {"timestamp": "2025-06-28T10:00:00.000Z", "ID": "...", "Name": "Mouse", "Quantity": 50, "Last Updated": {}}
INFO ------------------------- {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Deleting 'Mouse' from inventory... {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Mouse deleted {"timestamp": "2025-06-28T10:00:00.000Z", "rows_affected": 1}
INFO --- Current Inventory --- {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Found multiple items: {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Item {"timestamp": "2025-06-28T10:00:00.000Z", "ID": "...", "Name": "Keyboard", "Quantity": 25, "Last Updated": {}}
INFO Item {"timestamp": "2025-06-28T10:00:00.000Z", "ID": "...", "Name": "Laptop", "Quantity": 8, "Last Updated": {}}
INFO ------------------------- {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Reading items with quantity less than 20: {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Found 1 low stock item: {"timestamp": "2025-06-28T10:00:00.000Z"}
INFO Item {"timestamp": "2025-06-28T10:00:00.000Z", "ID": "...", "Name": "Laptop", "Quantity": 8, "Last Updated": {}}
```
This confirms that the application can connect to SQLite, define a schema, create a table, insert data, query it, and manage transactions using the Anansi framework.

---

## 💡 Usage Documentation

Anansi operates on the principle of defining your data structure as a schema, then using that schema to interact with the persistence layer.

### Defining Schemas

Schemas are defined using the `schema.SchemaDefinition` struct, which can be easily unmarshaled from JSON. This allows for externalizing your data models.

**Example (`inventorySchemaJSON` from `examples/basic/main.go`):**

```json
{
  "name": "inventory_items",
  "version": "1.0.0",
  "description": "Schema for tracking inventory items",
  "fields": {
    "id": { "name": "id", "type": "string", "required": true, "unique": true },
    "item_name": { "name": "item_name", "type": "string", "required": true, "unique": true },
    "description": { "name": "description", "type": "string", "required": false },
    "quantity": { "name": "quantity", "type": "integer", "required": true },
    "last_updated": { "name": "last_updated", "type": "datetime", "required": true }
  },
  "indexes": [
    { "fields": ["item_name"], "unique": true },
    { "fields": ["quantity"] }
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

	"github.com/asaidimu/go-anansi/v6/core/persistence"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/sqlite"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"go.uber.org/zap"
)

func main() {
	// Setup logger
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.TimeKey = "timestamp"

	logger, err := config.Build()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// 1. Open SQLite database connection
	db, err := sql.Open("sqlite3", "./inventory.db")
	if err != nil {
		logger.Fatal("Failed to open database", zap.Error(err))
	}
	defer db.Close()

	// 2. Initialize SQLite Interactor with default options
	interactorOptions := sqlite.DefaultInteractorOptions()
	interactor := sqlite.NewSQLiteInteractor(db, logger, interactorOptions, nil)

	// 3. Initialize the Anansi Persistence service
	// An empty schema.FunctionMap is passed for now; see "In-memory Go Functions" section.
	persistenceSvc, err := persistence.NewPersistence(interactor, schema.FunctionMap{})
	if err != nil {
		logger.Fatal("Failed to initialize persistence service", zap.Error(err))
	}
	logger.Info("Anansi persistence service initialized.")

	// ... now use persistenceSvc to create collections, etc.
}
```

### Creating Collections

Once `persistence.NewPersistence` is initialized, you can create a collection (which maps to a database table) using your schema definition.

```go
// inventorySchema is your schema.SchemaDefinition unmarshaled from JSON
var inventorySchema schema.SchemaDefinition
if err := json.Unmarshal([]byte(inventorySchemaJSON), &inventorySchema); err != nil {
    logger.Fatal("Failed to unmarshal inventory schema", zap.Error(err))
}

var inventoryCollection persistence.PersistenceCollectionInterface
if inventoryCollection, err = persistenceSvc.Collection(inventorySchema.Name); err != nil {
    // Collection doesn't exist, create it
    inventoryCollection, err = persistenceSvc.Create(inventorySchema)
    if err != nil {
        logger.Fatal("Failed to create 'inventory_items' collection", zap.Error(err))
    }
}
logger.Info("'inventory_items' collection created successfully.")
```

### Basic CRUD Operations

Anansi provides methods for common database operations.

#### Create (Insert)

```go
import (
	"time"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// Single record insert
item1 := map[string]any{
    "id":           uuid.New().String(),
    "item_name":    "Laptop",
    "description":  "High-performance notebook",
    "quantity":     int64(10), // Use int64 for integer types in Anansi documents
    "last_updated": time.Now(),
}
// The Create method accepts map[string]any or []map[string]any
_, err = inventoryCollection.Create(item1) // Returns *query.QueryResult
if err != nil {
    logger.Error("Failed to add Laptop", zap.Error(err))
}

// Batch inserts
item2 := map[string]any{ /* ... */ }
item3 := map[string]any{ /* ... */ }
batchData := []map[string]any{item2, item3}

// Pass a slice of maps for batch insertion
_, err = inventoryCollection.Create(batchData)
if err != nil {
    logger.Error("Failed to batch insert items", zap.Error(err))
}
```

#### Read (Query)

Read operations leverage the `query.QueryBuilder` to construct complex queries.

```go
import "github.com/asaidimu/go-anansi/v6/core/query"

// Query all items, ordered by name ascending
readQuery := query.NewQueryBuilder().OrderBy("item_name", query.SortDirectionAsc).Build()

result, err := inventoryCollection.Read(&readQuery) // Read takes a pointer to QueryDSL
if err != nil {
    logger.Error("Failed to read inventory items", zap.Error(err))
    return
}

// Results are []schema.Document (map[string]any) if multiple, or schema.Document if single
if result.Count > 0 {
	if itemDocs, ok := result.Data.([]schema.Document); ok {
		for _, itemDoc := range itemDocs {
			fmt.Printf("Item: ID=%v, Name=%v, Quantity=%v\n", itemDoc["id"], itemDoc["item_name"], itemDoc["quantity"])
		}
	} else if itemDoc, ok := result.Data.(schema.Document); ok {
		fmt.Printf("Item: ID=%v, Name=%v, Quantity=%v\n", itemDoc["id"], itemDoc["item_name"], itemDoc["quantity"])
	}
} else {
	fmt.Println("No items in inventory.")
}
```

#### Update

```go
import "github.com/asaidimu/go-anansi/v6/core/persistence"

// Update the quantity for 'Laptop'
updateData := map[string]any{
    "quantity":     int64(8), // Quantity reduced
    "last_updated": time.Now(),
}
updateFilter := query.NewQueryBuilder().Where("item_name").Eq("Laptop").Build().Filters

updateParams := &persistence.CollectionUpdate{
	Data:   updateData,
	Filter: updateFilter,
}

rowsAffected, err := inventoryCollection.Update(updateParams)
if err != nil {
    logger.Error("Failed to update Laptop quantity", zap.Error(err))
} else {
    logger.Info("Laptop quantity updated", zap.Int("rows_affected", rowsAffected))
}
```

#### Delete

```go
// Delete 'Mouse' from inventory
deleteFilter := query.NewQueryBuilder().Where("item_name").Eq("Mouse").Build().Filters

// By default, DELETE requires a filter for safety (unsafe=false).
rowsAffected, err := inventoryCollection.Delete(deleteFilter, false)
if err != nil {
    logger.Error("Failed to delete Mouse", zap.Error(err))
} else {
    logger.Info("Mouse deleted", zap.Int("rows_affected", rowsAffected))
}

// To delete an entire collection (table) from the persistence service:
// Note: This operation is generally irreversible.
// deleted, err := persistenceSvc.Delete("inventory_items")
// if err != nil {
// 	logger.Fatal("Failed to drop collection: %v", err)
// }
// logger.Info("Collection 'inventory_items' deleted", zap.Bool("deleted", deleted))
```

### Data Validation

Anansi allows you to validate data against a collection's schema constraints at runtime using `collection.Validate()`.

```go
import "github.com/asaidimu/go-anansi/v6/core/schema"

// Example: Inventory item schema requires 'item_name' and 'quantity'
invalidItemData := map[string]any{
    "id": "123-abc",
    // "item_name" is missing (required)
    "description": "Invalid item attempt",
    "quantity":    "not-a-number", // Quantity is required and must be integer
}

// `false` for strict validation (all required fields must be present)
validationResult, err := inventoryCollection.Validate(invalidItemData, false)
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
	"github.com/asaidimu/go-anansi/v6/core/persistence"
)

// Register a subscription to be notified when a document is created successfully
subscriptionId := inventoryCollection.RegisterSubscription(persistence.RegisterSubscriptionOptions{
    Event: persistence.DocumentCreateSuccess,
    Label: persistence.StringPtr("log_new_item"),
    Description: persistence.StringPtr("Logs details of newly created inventory items."),
    Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
        if event.Collection != nil && event.Output != nil {
            fmt.Printf("EVENT: Document created in collection '%s'. Output: %+v\n",
                *event.Collection, event.Output)
        }
        return nil
    },
})

fmt.Printf("Subscribed to DocumentCreateSuccess with ID: %s\n", subscriptionId)

// Later, to unsubscribe:
// inventoryCollection.UnregisterSubscription(subscriptionId)

// To get all active subscriptions for this collection:
// subs, _ := inventoryCollection.Subscriptions()
// for _, sub := range subs {
//     fmt.Printf("Active Subscription: ID=%s, Event=%s, Label=%s\n", *sub.Id, sub.Event, *sub.Label)
// }
```

### Transaction Management

Anansi supports executing multiple operations within a single database transaction using `persistence.Transact()`.

```go
import (
	"context"
	"fmt"
	"time"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

_, err = persistenceSvc.Transact(func(tx persistence.PersistenceTransactionInterface) (any, error) {
    // Get a collection instance operating within this transaction
    txCollection, err := tx.Collection("inventory_items")
    if err != nil {
        return nil, fmt.Errorf("failed to get transactional collection: %w", err)
    }

    // Perform operations within the transaction.
    // If any operation returns an error, the entire transaction will be rolled back.
    _, err = txCollection.Create(map[string]any{
        "id":           uuid.New().String(),
        "item_name":    "Charger",
        "description":  "USB-C Laptop Charger",
        "quantity":     int64(5),
        "last_updated": time.Now(),
    })
    if err != nil {
        return nil, fmt.Errorf("tx create 'Charger' failed: %w", err)
    }

    // Attempt an operation that might fail (e.g., due to unique constraint violation if "Keyboard" exists)
    _, err = txCollection.Create(map[string]any{
        "id":           uuid.New().String(),
        "item_name":    "Keyboard", // This might already exist from basic example, causing a unique constraint error
        "description":  "Another mechanical keyboard",
        "quantity":     int64(1),
        "last_updated": time.Now(),
    })
    if err != nil {
        // Return error to trigger rollback
        return nil, fmt.Errorf("tx create 'Keyboard' failed (expected if already exists): %w", err)
    }

    logger.Info("Transaction operations completed, preparing to commit.")
    return nil, nil // Return nil, nil for success, or an error to rollback
})

if err != nil {
    logger.Error("Transaction failed and was rolled back", zap.Error(err))
} else {
    logger.Info("Transaction committed successfully.")
}
```

### Advanced Querying with QueryDSL

The `query.QueryBuilder` provides a rich API for constructing declarative queries:

```go
import "github.com/asaidimu/go-anansi/v6/core/query"

// Example: Get items with quantity less than 20, ordered by quantity ascending, and specific fields.
lowStockQuery := query.NewQueryBuilder().
    Where("quantity").Lt(int64(20)). // Filter: quantity < 20
    OrderByAsc("quantity").          // Sort by quantity ascending
    Limit(5).Offset(0).              // Paginate: 5 results, from start
    Select().
        Include("item_name", "quantity"). // Project: only item_name and quantity
    End().
    Build()

result, err := inventoryCollection.Read(&lowStockQuery) // Read takes a pointer to QueryDSL
if err != nil {
    logger.Error("Failed to read data with advanced query", zap.Error(err))
} else {
    fmt.Println("\n--- Advanced Query Results ---")
    if result.Count > 0 {
        if itemDocs, ok := result.Data.([]schema.Document); ok {
            for _, r := range itemDocs {
                fmt.Printf("Item Name: %v, Quantity: %v\n", r["item_name"], r["quantity"])
            }
        } else if itemDoc, ok := result.Data.(schema.Document); ok { // Handle single result
            fmt.Printf("Item Name: %v, Quantity: %v\n", itemDoc["item_name"], itemDoc["quantity"])
        }
    } else {
        fmt.Println("No items matching the advanced query.")
    }
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

	"github.com/asaidimu/go-anansi/v6/core/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/sqlite"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Setup logger
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.TimeKey = "timestamp"
	logger, err := config.Build()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	dbFileName := "go_functions.db"
	if err := os.Remove(dbFileName); err != nil && !os.IsNotExist(err) {
		logger.Fatal("Failed to remove existing database file", zap.String("file", dbFileName), zap.Error(err))
	}
	db, err := sql.Open("sqlite3", dbFileName)
	if err != nil {
		logger.Fatal("Failed to open database", zap.Error(err))
	}
	defer db.Close()

	// 1. Define a schema with a JSON 'properties' field
	schemaJSON := `{
		"name": "products",
		"version": "1.0.0",
		"fields": {
			"id": {"name": "id", "type": "string", "required": true, "unique": true},
			"product_name": {"name": "product_name", "type": "string", "required": true},
			"properties": {"name": "properties", "type": "object"}
		},
		"indexes": [{"name": "pk_product_id", "fields": ["id"], "type": "primary"}]
	}`
	var productSchema schema.SchemaDefinition
	if err := json.Unmarshal([]byte(schemaJSON), &productSchema); err != nil {
		logger.Fatal("Failed to unmarshal product schema", zap.Error(err))
	}

	// 2. Prepare the FunctionMap with your custom Go functions
	customFunctions := schema.FunctionMap{
		// A computed field function to combine product name and a property
		"full_product_name": query.ComputeFunction(func(row schema.Document, args query.FilterValue) (any, error) {
			pName, ok := row["product_name"].(string)
			if !ok {
				return nil, fmt.Errorf("product_name is not a string")
			}
			// Access nested JSON field 'properties.color'
			if props, ok := row["properties"].(map[string]any); ok {
				if color, ok := props["color"].(string); ok {
					return fmt.Sprintf("%s (%s)", pName, color), nil
				}
			}
			return pName, nil // Fallback if no color property
		}),
		// A custom filter function to check a nested property value
		"is_premium": query.PredicateFunction(func(doc schema.Document, field string, args query.FilterValue) (bool, error) {
			// This custom filter checks if 'properties.tier' is "premium"
			if props, ok := doc["properties"].(map[string]any); ok {
				if tier, ok := props["tier"].(string); ok {
					return tier == "premium", nil
				}
			}
			return false, nil // Not premium or no tier defined
		}),
	}

	// 3. Initialize Persistence with the custom functions
	interactor := sqlite.NewSQLiteInteractor(db, logger, sqlite.DefaultInteractorOptions(), nil)
	persistenceService, err := persistence.NewPersistence(interactor, customFunctions)
	if err != nil {
		logger.Fatal("Failed to initialize persistence", zap.Error(err))
	}

	collection, err := persistenceService.Create(productSchema)
	if err != nil {
		logger.Fatal("Failed to create products collection", zap.Error(err))
	}

	// Insert some data with nested JSON properties
	collection.Create(map[string]any{"id": "p001", "product_name": "Smartphone X", "properties": map[string]any{"color": "black", "storage_gb": 128, "tier": "standard"}})
	collection.Create(map[string]any{"id": "p002", "product_name": "Smartwatch Y", "properties": map[string]any{"color": "silver", "tier": "premium"}})
	collection.Create(map[string]any{"id": "p003", "product_name": "Laptop Z", "properties": map[string]any{"color": "space gray", "tier": "premium", "weight_kg": 1.5}})

	// Query using the registered Compute Function
	logger.Info("Querying with Go computed field:")
	qWithGoFuncs := query.NewQueryBuilder().
		Where("id").Gt(""). // Base filter to retrieve all data
		Select().
			Include("id", "product_name", "properties"). // Ensure required fields are selected from DB
			AddComputed("full_display_name", "full_product_name"). // Use registered compute function
		End().
		Build()

	result, err := collection.Read(&qWithGoFuncs)
	if err != nil {
		logger.Fatal("Failed to query with computed field", zap.Error(err))
	}
	fmt.Println("--- Results with Computed Field ---")
	for _, r := range result.Data.([]schema.Document) {
		fmt.Printf("ID: %v, Product Name: %v, Display Name: %v, Properties: %v\n", r["id"], r["product_name"], r["full_display_name"], r["properties"])
	}

	// Query using the custom Go filter
	logger.Info("Querying with custom Go filter:")
	qWithGoFilter := query.NewQueryBuilder().
		Where("id").Custom("is_premium", true). // Use registered Go filter function
		Select().
			Include("id", "product_name", "properties"). // Ensure properties are fetched for the Go filter to work
		End().
		Build()

	resultFilter, err := collection.Read(&qWithGoFilter)
	if err != nil {
		logger.Fatal("Failed to query with custom filter", zap.Error(err))
	}
	fmt.Println("\n--- Results with Custom Filter (is_premium) ---")
	for _, r := range resultFilter.Data.([]schema.Document) {
		fmt.Printf("ID: %v, Product Name: %v, Properties: %v\n", r["id"], r["product_name"], r["properties"])
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
7.  Retrieved rows are read from `*sql.Rows` and converted into a generic `schema.Document` (map[string]any) slice, performing schema-aware type conversions (e.g., SQLite `INTEGER` to Go `bool` for `FieldTypeBoolean`, JSON strings to Go `any` for object/array types).
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
        ID       string `json:"id" anansi:"primary_key" db:"id"`
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
