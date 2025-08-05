# Anansi (Go Implementation)

[![Go Reference](https://pkg.go.dev/badge/github.com/asaidimu/go-anansi/v6.svg)](https://pkg.go.dev/github.com/asaidimu/go-anansi/v6)
[![Build Status](https://github.com/asaidimu/go-anansi/workflows/Test%20Workflow/badge.svg)](https://github.com/asaidimu/go-anansi/actions)
[![License: Proprietary](https://img.shields.io/badge/License-Proprietary-red.svg)](LICENSE.md)

Anansi is a comprehensive Go toolkit for schema-driven data persistence and flexible querying, supporting dynamic data models, powerful runtime validation, and adaptable storage layers.

---

## 📚 Table of Contents

*   [✨ Overview & Features](#-overview--features)
*   [🚀 Installation & Setup](#-installation--setup)
*   [📖 Usage Documentation](#-usage-documentation)
*   [🏗️ Project Architecture](#%EF%B8%8F-project-architecture)
*   [🤝 Development & Contributing](#-development--contributing)
*   [ℹ️ Additional Information](#%EF%B8%8F-additional-information)

---

## ✨ Overview & Features

Anansi is designed to bring a robust, schema-first approach to data persistence in Go applications. By externalizing data models into declarative JSON schema definitions, it allows for dynamic collection (table) creation, powerful data retrieval, and a clear pathway for future data migrations and versioning. This framework aims to provide a high degree of flexibility and extensibility by abstracting the underlying storage mechanism through a pluggable `DatabaseInteractor` interface.

At its core, Anansi focuses on providing a rich Domain-Specific Language (DSL) for constructing queries, enabling developers to express complex data operations in a concise and intuitive manner. This DSL is designed to be highly expressive, supporting advanced filtering, sophisticated projections, multi-collection joins, and powerful aggregation capabilities, all while maintaining a clear and unambiguous mapping to structured data.

> [!WARNING]
> This project is currently under heavy development. It is **not** ready for production use. APIs and features are subject to change.

### Key Features

*   **Schema-driven Development**:
    *   Define data models using declarative JSON schemas, enabling dynamic collection creation and validation.
    *   Support for a rich set of field types: `string`, `number`, `integer`, `boolean`, `array`, `set`, `enum`, `object`, `record`, and `union`.
    *   Comprehensive schema validation, including required fields, type checking, enum value constraints, and custom predicate-based constraints (field-level, nested schema, and schema-level).
    *   Support for defining indexes (e.g., unique, primary) directly within the schema.
    *   Built-in schema versioning and migration capabilities for evolving data models.

*   **Powerful Query Language (Anansi Query)**:
    *   A natural language-like query grammar for constructing complex data retrieval operations.
    *   **Filtering**: Basic conditions (`==`, `!=`, `<`, `<=`, `>`, `>=`, `IN`, `NOT IN`, `CONTAINS`, `NOT CONTAINS`, `EXISTS`, `NOT EXISTS`), logical combinations (`AND`, `OR`, `XOR`, `NOR`, `NOT`), and function calls as conditions.
    *   **Text Search**: Advanced full-text search types like `MATCH`, `PHRASE`, `PREFIX`, `WILDCARD`, `FUZZY`, and `REGEX`, configurable with options like `FUZZINESS`, `MINIMUM_MATCH`, `BOOST`, `ANALYZER`, and `OPERATOR`.
    *   **Sorting**: Multi-field sorting with ascending (`ASC`) or descending (`DESC`) order.
    *   **Pagination**: Offset-based pagination (`OFFSET`, `LIMIT`) for controlling result set size and navigation.
    *   **Projections**: Select (`INCLUDE`) or exclude (`EXCLUDE`) specific fields, including nested fields. Define `COMPUTE`d fields using function calls or powerful `CASE` expressions.
    *   **Joins**: Combine data from related collections using `INNER`, `LEFT`, `RIGHT`, and `FULL` joins, with configurable `ON` conditions and nested projections for joined data.
    *   **Aggregations**: Perform summary calculations (`COUNT`, `SUM`, `AVG`, `MIN`, `MAX`) on data, with support for `GROUP BY` fields and `HAVING` filters on aggregated results.
    *   **Distinct Operations**: Retrieve unique records, either for the entire result set or based on specific fields.
    *   **Union Operations**: Combine results from multiple queries using `UNION`, `UNION ALL`, `INTERSECT`, and `EXCEPT`.
    *   **Query Hints**: Provide optimization directives to the underlying database, such as `USE INDEX`, `FORCE INDEX`, `NO INDEX`, and `MAX_TIME`.
    *   **Functions & Case Expressions**: Support for a wide range of runtime functions and conditional logic within queries for advanced data manipulation.

*   **Pluggable Persistence Layer**:
    *   Abstract `DatabaseInteractor` interface allows seamless integration with various storage backends.
    *   Includes a robust `ephemeral` (in-memory) implementation for rapid prototyping, testing, and lightweight use cases.
    *   Designed to be extensible for SQL (e.g., SQLite via `mattn/go-sqlite3`), NoSQL, or other custom storage solutions.

*   **Runtime Validation & Coercion**:
    *   Automatic validation of documents against their defined schemas during write operations.
    *   Built-in primitive type coercion (e.g., "123" to `int`, "true" to `bool`).

*   **Event-Driven Observability**:
    *   A comprehensive `PersistenceEvent` system emits events for every lifecycle operation (create, read, update, delete, migration, transaction).
    *   Allows external modules to subscribe to and react to data changes in a decoupled manner, enabling audit logging, caching, and real-time analytics.

*   **Automatic Metadata Management**:
    *   Transparent management of an internal `_metadata_` field on every document.
    *   Includes fields for versioning and optimistic locking, secured by HMAC-SHA256 hashing to ensure data integrity.

*   **Query Optimization & Partitioning**:
    *   A `QueryEngine` intelligently partitions complex queries, offloading native operations to the underlying database and performing unsupported operations (e.g., custom functions, complex joins on non-SQL backends) in-memory.
    *   Leverages an LRU cache for partitioned queries to improve performance for frequently executed complex queries.

---

## 🚀 Installation & Setup

### Prerequisites

*   [Go](https://go.dev/dl/) (version 1.24.4 or higher recommended)
*   Git

### Installation Steps

To get started with Anansi, you can clone the repository and build the project:

```bash
# Clone the repository
git clone https://github.com/asaidimu/go-anansi.git

# Navigate into the project directory
cd go-anansi

# Fetch Go modules and their dependencies
go mod tidy

# Build the project (optional, for local development)
make build
```

Alternatively, if you want to use Anansi as a library in your Go project:

```bash
go get github.com/asaidimu/go-anansi/v6
```

### Configuration

Anansi's configuration is primarily programmatic, defined through the options passed during the initialization of its core components like `Persistence` and `QueryEngine`.

**Basic Setup with Ephemeral (In-Memory) Backend:**

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/persistence"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"go.uber.org/zap"
)

func main() {
	// 1. Initialize Logging and Event Bus
	logger, _ := zap.NewDevelopment() // Or zap.NewProduction()
	eventBus, err := persistence.NewEventBus(persistence.DefaultEventBusConfig())
	if err != nil {
		log.Fatalf("Failed to initialize event bus: %v", err)
	}

	// Optional: Subscribe to events for observability
	eventBus.RegisterSubscription(base.RegisterSubscriptionOptions{
		Event: base.DocumentCreateSuccess,
		Label: utils.StringPtr("document_created_logger"),
		Callback: func(ctx context.Context, event base.PersistenceEvent) error {
			logger.Info("Document Created", zap.Stringp("collection", event.Collection), zap.Any("document", event.Output))
			return nil
		},
	})

	// 2. Setup the underlying database interactor (e.g., Ephemeral for in-memory)
	// You could replace this with a SQL/NoSQL interactor for persistent storage
	ephemeralInteractor, ephemeralSchemaManager := ephemeral.NewEphemeral()

	// 3. Initialize the core Persistence layer
	persistenceService, err := persistence.NewPersistence(
		eventBus,
		ephemeralInteractor,
		ephemeralSchemaManager,
		logger,
		&base.MetadataOptions{
			HmacSecretKey: []byte("your-super-secret-key-that-is-at-least-32-bytes-long"), // CHANGE THIS IN PROD!
		},
	)
	if err != nil {
		log.Fatalf("Failed to initialize persistence service: %v", err)
	}

	// 4. Define a Schema for your data
	userSchema := schema.SchemaDefinition{
		Name:    "users",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString, Required: utils.BoolPtr(true), Unique: utils.BoolPtr(true)},
			"name": {Name: "name", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			"age":  {Name: "age", Type: schema.FieldTypeInteger},
		},
	}

	// 5. Create a Collection based on the schema
	usersCollection, err := persistenceService.Create(userSchema)
	if err != nil {
		log.Fatalf("Failed to create 'users' collection: %v", err)
	}
	fmt.Println("Collection 'users' created successfully.")

	// 6. Create (Insert) Documents
	doc1 := common.Document{"id": "user1", "name": "Alice", "age": 30}
	doc2 := common.Document{"id": "user2", "name": "Bob", "age": 25}

	createResult1, err := usersCollection.CreateOne(doc1)
	if err != nil {
		log.Fatalf("Failed to create document 1: %v", err)
	}
	fmt.Printf("Created document: %+v\n", createResult1.Data)

	createResults, err := usersCollection.CreateMany([]common.Document{doc2})
	if err != nil {
		log.Fatalf("Failed to create document 2: %v", err)
	}
	fmt.Printf("Created documents count: %d\n", len(createResults))

	// 7. Read Documents with Query
	// Example: Find users older than 28, sorted by age descending, limit to 1
	queryBuilder := query.NewQueryBuilder().
		Where("age").Gt(28).
		OrderByDesc("age").
		Limit(1)

	readQuery := queryBuilder.Build()
	results, err := usersCollection.Read(&readQuery)
	if err != nil {
		log.Fatalf("Failed to read documents: %v", err)
	}

	fmt.Printf("\nQuery Results (Count: %d):\n", results.Count)
	for _, doc := range results.Data.([]common.Document) {
		doc.StripMetadata() // Optional: remove internal metadata for clean output
		fmt.Printf("- %+v\n", doc)
	}
}
```

### Verification

To verify your installation and setup, you can run the project's tests:

```bash
make test
```

This command will execute all unit tests and should complete successfully if everything is set up correctly.

---

## 📖 Usage Documentation

Anansi offers a rich API for interacting with its persistence and query layers. This section provides examples and details on how to leverage its core functionalities.

### Defining Schemas

Schemas are the backbone of Anansi, defining the structure and validation rules for your data. They are expressed in a declarative JSON format.

```json
{
  "name": "products",
  "version": "1.0.0",
  "description": "Schema for e-commerce products",
  "fields": {
    "id": {
      "name": "id",
      "type": "string",
      "required": true,
      "unique": true,
      "description": "Unique product identifier"
    },
    "name": {
      "name": "name",
      "type": "string",
      "required": true,
      "constraints": [
        {
          "name": "min_length_5",
          "predicate": "minLength",
          "parameters": { "min": 5 },
          "errorMessage": "Product name must be at least 5 characters long"
        }
      ]
    },
    "price": {
      "name": "price",
      "type": "number",
      "required": true,
      "constraints": [
        {
          "name": "positive_price",
          "predicate": "isPositive"
        }
      ]
    },
    "category": {
      "name": "category",
      "type": "enum",
      "values": ["electronics", "apparel", "books", "home"],
      "required": true
    },
    "details": {
      "name": "details",
      "type": "object",
      "schema": {
        "id": "product_details"
      }
    },
    "tags": {
      "name": "tags",
      "type": "array",
      "itemsType": "string"
    },
    "variants": {
      "name": "variants",
      "type": "record",
      "schema": {
        "id": "product_variant"
      }
    }
  },
  "nestedSchemas": {
    "product_details": {
      "name": "product_details",
      "description": "Details for a product",
      "fields": {
        "weight_kg": { "name": "weight_kg", "type": "number" },
        "dimensions_cm": {
          "name": "dimensions_cm",
          "type": "object",
          "schema": {
            "fields": {
              "length": { "type": "number" },
              "width": { "type": "number" },
              "height": { "type": "number" }
            }
          }
        }
      }
    },
    "product_variant": {
      "name": "product_variant",
      "description": "A product variant schema",
      "type": "object",
      "schema": {
        "fields": {
          "sku": { "name": "sku", "type": "string", "required": true },
          "color": { "name": "color", "type": "string" },
          "size": { "name": "size", "type": "string" }
        }
      }
    }
  },
  "constraints": [
    {
      "name": "expensive_electronics_check",
      "operator": "AND",
      "rules": [
        {
          "predicate": "isExpensive",
          "field": "price"
        },
        {
          "predicate": "isElectronics",
          "field": "category"
        }
      ]
    }
  ],
  "indexes": [
    {
      "name": "category_price_index",
      "fields": ["category", "price"],
      "type": "normal"
    }
  ]
}
```

*Note: Custom predicates like `minLength`, `isPositive`, `isExpensive`, `isElectronics` would need to be registered with the `SchemaValidator` or `QueryHelper` respectively.*

### Querying Data with QueryBuilder

The `QueryBuilder` provides a fluent API to programmatically construct complex queries using the Anansi Query DSL. It's the recommended way to interact with the query layer in Go.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/persistence"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"go.uber.org/zap"
)

func main() {
	logger := zap.NewNop() // Use a no-op logger for example simplicity
	eventBus, _ := persistence.NewEventBus(persistence.DefaultEventBusConfig())
	ephemeralInteractor, ephemeralSchemaManager := ephemeral.NewEphemeral()
	persistenceService, err := persistence.NewPersistence(eventBus, ephemeralInteractor, ephemeralSchemaManager, logger, &base.MetadataOptions{HmacSecretKey: []byte("a-very-secret-key-for-testing-purposes-123")})
	if err != nil {
		log.Fatalf("Failed to init persistence: %v", err)
	}

	productSchema := schema.SchemaDefinition{
		Name:    "products",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":          {Name: "id", Type: schema.FieldTypeString, Required: utils.BoolPtr(true), Unique: utils.BoolPtr(true)},
			"name":        {Name: "name", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			"price":       {Name: "price", Type: schema.FieldTypeNumber, Required: utils.BoolPtr(true)},
			"category":    {Name: "category", Type: schema.FieldTypeString},
			"description": {Name: "description", Type: schema.FieldTypeString},
			"stock":       {Name: "stock", Type: schema.FieldTypeInteger},
			"available":   {Name: "available", Type: schema.FieldTypeBoolean},
			"supplierId":  {Name: "supplierId", Type: schema.FieldTypeString},
		},
	}
	productsCollection, err := persistenceService.Create(productSchema)
	if err != nil {
		log.Fatalf("Failed to create products collection: %v", err)
	}

	// Insert some sample data
	productsCollection.CreateMany([]common.Document{
		{"id": "p001", "name": "Laptop Pro", "price": 1200.00, "category": "electronics", "description": "High performance laptop", "stock": 50, "available": true, "supplierId": "s101"},
		{"id": "p002", "name": "Wireless Mouse", "price": 25.50, "category": "electronics", "description": "Ergonomic wireless mouse", "stock": 200, "available": true, "supplierId": "s102"},
		{"id": "p003", "name": "Desk Chair", "price": 300.00, "category": "home", "description": "Comfortable office chair", "stock": 10, "available": false, "supplierId": "s101"},
		{"id": "p004", "name": "USB-C Hub", "price": 45.00, "category": "electronics", "description": "Multiport adapter", "stock": 150, "available": true, "supplierId": "s103"},
	})

	// Example 1: Basic Filtering and Sorting
	fmt.Println("\n--- Basic Filtering and Sorting ---")
	q1 := query.NewQueryBuilder().
		Where("category").Eq("electronics").
		AndFilter(query.NewQueryBuilder().Where("price").Lte(100.00).Build().Filters).
		OrderByDesc("price").
		Build()
	res1, _ := productsCollection.Read(&q1)
	for _, doc := range res1.Data.([]common.Document) {
		doc.StripMetadata()
		fmt.Printf("ID: %s, Name: %s, Price: %.2f\n", doc["id"], doc["name"], doc["price"])
	}
	// Expected: ID: p004, Name: USB-C Hub, Price: 45.00
	//           ID: p002, Name: Wireless Mouse, Price: 25.50

	// Example 2: Text Search (MATCH-like behavior in ephemeral)
	fmt.Println("\n--- Text Search ---")
	q2 := query.NewQueryBuilder().
		TextSearch("description").Contains("wireless mouse").
		Build()
	res2, _ := productsCollection.Read(&q2)
	for _, doc := range res2.Data.([]common.Document) {
		doc.StripMetadata()
		fmt.Printf("ID: %s, Name: %s, Description: %s\n", doc["id"], doc["name"], doc["description"])
	}
	// Expected: ID: p002, Name: Wireless Mouse, Description: Ergonomic wireless mouse

	// Example 3: Projection (Include, Exclude, Computed)
	fmt.Println("\n--- Projections ---")
	q3 := query.NewQueryBuilder().
		Select().
		Include("id", "name", "price").
		AddComputed("adjusted_price", "MULTIPLY", "price", 0.9). // Assuming MULTIPLY function is registered
		AddCase("stock_status").
		When(query.NewQueryBuilder().Where("stock").Gt(100).Build().Filters, "High Stock").
		When(query.NewQueryBuilder().Where("stock").Gt(0).Build().Filters, "Low Stock").
		Else("Out of Stock").
		End().
		End().
		Build()
	res3, _ := productsCollection.Read(&q3)
	for _, doc := range res3.Data.([]common.Document) {
		doc.StripMetadata()
		fmt.Printf("ID: %s, Name: %s, Price: %.2f, Adjusted Price: %.2f, Stock Status: %s\n",
			doc["id"], doc["name"], doc["price"], doc["adjusted_price"], doc["stock_status"])
	}
	// Expected (approx):
	// ID: p001, Name: Laptop Pro, Price: 1200.00, Adjusted Price: 1080.00, Stock Status: High Stock
	// ID: p002, Name: Wireless Mouse, Price: 25.50, Adjusted Price: 22.95, Stock Status: High Stock
	// ID: p003, Name: Desk Chair, Price: 300.00, Adjusted Price: 270.00, Stock Status: Low Stock
	// ID: p004, Name: USB-C Hub, Price: 45.00, Adjusted Price: 40.50, Stock Status: High Stock

	// To make the computed fields work with `MULTIPLY` in the ephemeral helper,
	// you'd need to register it:
	// queryEngine := query.NewQueryEngine(ephemeralInteractor, logger)
	// queryEngine.RegisterComputeFunction("MULTIPLY", func(row common.Document, args []query.FilterValue) (any, error) {
	// 	if len(args) != 2 { return nil, fmt.Errorf("MULTIPLY expects 2 arguments") }
	// 	val1, err1 := queryEngine.Helper.ResolveFilterValue(row, &args[0]) // simplified, actual helper needed
	// 	val2, err2 := queryEngine.Helper.ResolveFilterValue(row, &args[1])
	// 	// ... perform multiplication and return
	// })

	// Example 4: Aggregations with Group By
	fmt.Println("\n--- Aggregations ---")
	q4 := query.NewQueryBuilder().
		Aggregate(query.AggregationTypeCount, "id", "total_products").
		Aggregate(query.AggregationTypeAvg, "price", "avg_price_per_category").
		GroupBy("category").
		Build()
	res4, _ := productsCollection.Read(&q4)
	for _, doc := range res4.Data.([]common.Document) {
		doc.StripMetadata()
		fmt.Printf("Category: %s, Total Products: %.0f, Average Price: %.2f\n",
			doc["category"], doc["total_products"], doc["avg_price_per_category"])
	}
	// Expected (approx):
	// Category: electronics, Total Products: 3, Average Price: 423.50
	// Category: home, Total Products: 1, Average Price: 300.00

	// Example 5: Joining Collections (Ephemeral currently combines into a single document)
	fmt.Println("\n--- Joins (Conceptual for Ephemeral) ---")
	supplierSchema := schema.SchemaDefinition{
		Name:    "suppliers",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString, Required: utils.BoolPtr(true), Unique: utils.BoolPtr(true)},
			"name": {Name: "name", Type: schema.FieldTypeString, Required: utils.BoolPtr(true)},
			"country": {Name: "country", Type: schema.FieldTypeString},
		},
	}
	suppliersCollection, err := persistenceService.Create(supplierSchema)
	if err != nil {
		log.Fatalf("Failed to create suppliers collection: %v", err)
	}
	suppliersCollection.CreateMany([]common.Document{
		{"id": "s101", "name": "GlobalTech Inc.", "country": "USA"},
		{"id": "s102", "name": "Asian Electronics", "country": "China"},
		{"id": "s103", "name": "EuroParts Ltd.", "country": "Germany"},
	})

	q5 := query.NewQueryBuilder().
		InnerJoin("suppliers").
		On(query.NewQueryBuilder().Where("products.supplierId").Eq(query.NewFieldReference("suppliers.id")).Build().Filters).
		Select().
		Include("products.name", "products.price").
		IncludeNested("suppliers", &query.ProjectionConfiguration{
			Include: []query.ProjectionField{{Name: "name", Alias: utils.StringPtr("supplier_name")}, {Name: "country"}},
		}).
		End().
		Build()

	res5, _ := productsCollection.Read(&q5) // Read from the "left" side of the join
	for _, doc := range res5.Data.([]common.Document) {
		doc.StripMetadata()
		fmt.Printf("Product: %s (%.2f), Supplier: %s, Country: %s\n",
			doc["products"].(common.Document)["name"],
			doc["products"].(common.Document)["price"],
			doc["suppliers"].(common.Document)["supplier_name"],
			doc["suppliers"].(common.Document)["country"],
		)
	}
	// Expected:
	// Product: Laptop Pro (1200.00), Supplier: GlobalTech Inc., Country: USA
	// Product: Wireless Mouse (25.50), Supplier: Asian Electronics, Country: China
	// Product: Desk Chair (300.00), Supplier: GlobalTech Inc., Country: USA
	// Product: USB-C Hub (45.00), Supplier: EuroParts Ltd., Country: Germany

}
```

### Advanced Query Language Specification

For a comprehensive understanding of the natural language-like query grammar and its mapping to the structured JSON Query DSL, please refer to the dedicated specification:

*   [QUERYLANG.md](QUERYLANG.md)

This document details all supported clauses, operators, data types, functions, and provides numerous examples.

---

## 🏗️ Project Architecture

Anansi's architecture is modular and layered, designed for extensibility and separation of concerns.

```mermaid
graph TD
    A[Application Layer] --> B(Persistence Service);
    B --> C{Collection Layer};
    C --> D[Query Engine];
    D --> E[Database Interactor];
    E -- In-Memory --> F(Ephemeral Store);
    E -- SQL --> G(SQL Driver);
    D --> H[Query Helper (In-Memory Processing)];
    C --> I(Schema Validation);

    subgraph Core Components
        subgraph Persistence Module
            C;
        end
        subgraph Query Module
            D;
            H;
        end
        subgraph Schema Module
            I;
        end
        subgraph Storage Backends
            E;
            F;
            G;
        end
    end

    style A fill:#f9f,stroke:#333,stroke-width:2px
    style B fill:#bbf,stroke:#333,stroke-width:2px
    style C fill:#ccf,stroke:#333,stroke-width:2px
    style D fill:#ddf,stroke:#333,stroke-width:2px
    style E fill:#eef,stroke:#333,stroke-width:2px
    style F fill:#efe,stroke:#333,stroke-width:2px
    style G fill:#efe,stroke:#333,stroke-width:2px
    style H fill:#fef,stroke:#333,stroke-width:2px
    style I fill:#fcf,stroke:#333,stroke-width:2px
```

### Core Components

*   **Persistence Module (`core/persistence`)**:
    *   **`Persistence` Service**: The top-level entry point for data operations. It manages collections, schemas, transactions, and global event subscriptions.
    *   **`Collection`**: Represents a single data collection (like a table). It provides CRUD operations, validation, and collection-specific event subscriptions. Decorated with metadata management and event emission.
    *   **Event System**: (`go-events` library wrapper) A robust mechanism for emitting and subscribing to `PersistenceEvent`s across the entire persistence layer, enabling observability and reactive patterns.
    *   **Metadata Management**: Automatically injects and manages `_metadata_` fields (version, timestamps, hash) for optimistic concurrency control and data integrity.

*   **Query Module (`core/query`)**:
    *   **`Query`**: Defines the structured representation of queries (filters, sorts, joins, aggregations, etc.).
    *   **`QueryBuilder`**: A fluent API for programmatically constructing `Query` objects.
    *   **`QueryParser` (`core/query/parser`)**: Responsible for parsing the natural language-like query grammar (`QUERYLANG.md`) into the structured `Query`.
    *   **`QueryEngine`**: The central orchestrator for query execution. It takes a `Query`, partitions it based on backend `Capabilities`, executes the database-native portion, and then performs any necessary in-memory post-processing.
    *   **`QueryPartitioner`**: Analyzes a `Query` and the `DatabaseInteractor`'s `Capabilities` to split the query into parts executable by the database and parts requiring in-memory processing.
    *   **`QueryHelper`**: An in-memory data processor that applies filtering, sorting, pagination, projection, and aggregation logic to Go `map[string]any` slices. This is used for post-processing or for ephemeral storage.
    *   **`DatabaseInteractor`**: An interface defining the contract for low-level database operations (Select, Insert, Update, Delete, Transactions). Concrete implementations abstract SQL dialects or NoSQL APIs.
    *   **`SchemaManager`**: An interface for DDL operations (Create/Drop Collection, Create Index).

*   **Schema Module (`core/schema`)**:
    *   **`SchemaDefinition`**: The core type for defining data schemas, including fields, nested schemas, constraints, and indexes.
    *   **`DocumentValidator`**: Enforces schema rules against incoming data, providing detailed validation `Issue`s.
    *   **`Migration`**: Defines schema changes and data transformations for versioning.

*   **Ephemeral Storage (`core/ephemeral`)**:
    *   An in-memory implementation of the `DatabaseInteractor` and `SchemaManager` interfaces, built on top of `asaidimu/go-store/v3`. Ideal for testing and lightweight data needs.

*   **Common Utilities (`core/common`, `core/utils`, `core/logical`)**:
    *   Shared data structures (`Document`, `Issue`), logical operator evaluation, and helper functions for type coercion, map/struct conversions, and path manipulation.

### Data Flow (Query Execution)

1.  **Query Construction**: An application constructs a query using the `QueryBuilder` (programmatically) or potentially via a text string parsed by the `QueryParser`. This results in a `query.Query` (Query) object.
2.  **Query Partitioning**: The `QueryEngine`'s `QueryPartitioner` inspects the `query.Query` and the connected `DatabaseInteractor`'s `Capabilities`. It splits the complex `Query` into two simplified `query.Query` objects: one for the database (`dbQuery`) and one for in-memory post-processing (`postProcessingQuery`).
3.  **Database Execution**: The `QueryEngine` passes the `dbQuery` to the `DatabaseInteractor` (e.g., `EphemeralDatabaseInteractor` or a future SQL adapter). The interactor executes the query natively against the underlying storage.
4.  **In-Memory Post-processing**: If the `postProcessingQuery` is not empty, the `QueryEngine` retrieves the results from the database. It then uses the `QueryHelper` to apply any remaining filtering, sorting, pagination, or complex aggregations that couldn't be handled natively by the database.
5.  **Result Projection**: Finally, the overall projection (defined in the original `Query`) is applied to the processed data, and the final results are returned to the application.

### Extension Points

Anansi is designed to be extensible:

*   **Database Interactors**: Implement the `query.DatabaseInteractor` and `query.SchemaManager` interfaces to add support for new database systems (e.g., PostgreSQL, MongoDB).
*   **Custom Functions**: Register custom `ComputeFunction` (for computed fields), `PredicateFunction` (for custom filters), or general `FunctionExecutor`s with the `QueryEngine` or `QueryHelper` to extend query capabilities.
*   **Custom Constraints**: Define new predicates for schema validation and register them with the `schema.DocumentValidator`.
*   **Event Subscriptions**: Subscribe to `base.PersistenceEvent`s to trigger custom logic, integrate with external systems, or implement observability features.

---

## 🤝 Development & Contributing

Contributions are welcome, but please note the project is in active development.

### Development Setup

1.  **Fork the repository**: Go to the [Anansi GitHub repository](https://github.com/asaidimu/go-anansi) and click the "Fork" button.
2.  **Clone your fork**:
    ```bash
    git clone https://github.com/YOUR_GITHUB_USERNAME/go-anansi.git
    cd go-anansi
    ```
3.  **Install dependencies**:
    ```bash
    go mod tidy
    ```
4.  **Build**:
    ```bash
    make build
    ```

### Scripts

*   `make build`: Compiles the Go project.
*   `make test`: Runs all unit tests.
*   `./bin/bump.sh <OLD_VERSION_NUM> <NEW_VERSION_NUM> [--dry-run]`: A utility script to update Go module paths for major version bumps. Always use `--dry-run` first!

### Testing

All core functionalities are covered by unit tests. To run them:

```bash
make test
```

Tests use `stretchr/testify` for assertions and cover `persistence`, `schema`, `ephemeral`, and `query` modules extensively.

### Contributing Guidelines

1.  **Fork & Branch**: Fork the repository and create a new branch for your feature or bug fix (`git checkout -b feature/your-feature-name` or `bugfix/issue-description`).
2.  **Code**: Write your code, adhering to Go idioms and maintaining clean, readable code.
3.  **Test**: Add or update tests to cover your changes. Ensure all existing tests pass (`make test`).
4.  **Commit**: Write clear and concise commit messages. Follow a conventional commit style if possible (e.g., `feat: add new feature`, `fix: resolve bug`).
5.  **Pull Request**: Submit a Pull Request to the `main` branch of the original repository. Provide a detailed description of your changes.

Consider checking the `REFACTOR.md` file for known areas of improvement if you're looking for tasks.

### Issue Reporting

Please report any bugs, issues, or feature requests via the [GitHub Issues page](https://github.com/asaidimu/go-anansi/issues).

---

## ℹ️ Additional Information

### Troubleshooting

*   **Go Module Path Errors**: If you encounter issues related to module imports (e.g., "cannot find module .../v6"), ensure your `go.mod` file and import statements correctly reference the `github.com/asaidimu/go-anansi/v6` path. If upgrading from an older major version, use `go get github.com/asaidimu/go-anansi/v6` and run `go mod tidy`. The `bin/bump.sh` script might be helpful if you are directly modifying the codebase for version bumps.
*   **Generic Type Coercion**: The `REFACTOR.md` notes "Extensive Use of `any`" and "Pointer to Primitives." While `any` (interface{}) is sometimes necessary for a generic query engine, it can lead to runtime type assertion panics if not handled carefully. Ensure you check types (e.g., with type assertions `value.(type)`) when processing data retrieved from the `Document` type.
*   **Proprietary License**: Remember that the software is under a proprietary license. Using it for commercial purposes without a separate commercial license is prohibited.

### Changelog & Roadmap

For detailed version history and breaking changes, please refer to the [CHANGELOG.md](CHANGELOG.md) file.
As the project is under heavy development, a formal roadmap is being established. Expect continued enhancements to query capabilities, additional database interactor implementations (e.g., SQLite, PostgreSQL), and further refinement of the schema and persistence layers.

### License

This project is licensed under the **Anansi Platform Proprietary License**. This is a **source-available license** and is **NOT** an open-source license.

**A separate commercial license is required for any form of commercial use.**

For the full license text, please see the [LICENSE.md](LICENSE.md) file.

### Acknowledgments

Anansi is developed by Saidimu, under CyberSync Printers & Stationers.
