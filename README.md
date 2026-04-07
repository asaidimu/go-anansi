# Go-Anansi: A Schema-Driven, Hybrid Persistence Layer for Go

![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=for-the-badge&logo=go)
![License](https://img.shields.io/badge/License-Proprietary-red?style=for-the-badge)
![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen?style=for-the-badge)

Go-Anansi is a sophisticated, schema-driven persistence framework for Go applications, engineered for flexibility, extensibility, and high performance. It provides a robust data access layer that abstracts away underlying database complexities, allowing developers to focus on business logic rather than boilerplate data management.

## 🚀 Quick Links

*   [Overview](#overview--features)
*   [Key Features](#key-features)
*   [Installation & Setup](#installation--setup)
*   [Usage Documentation](#usage-documentation)
*   [Project Architecture](#project-architecture)
*   [Development & Contributing](#development--contributing)
*   [Additional Information](#additional-information)
*   [License](#license)

---

## ✨ Overview & Features

Go-Anansi is designed to revolutionize how Go applications interact with data stores. At its core, it's a highly modular persistence layer that leverages a decorator pattern, enabling flexible composition of functionalities like lifecycle management, event emission, and custom business logic. A standout feature is its self-managing nature, utilizing an internal `CollectionRegistry` to store system metadata, making the system self-describing and simplifying setup.

The framework's advanced query engine intelligently partitions queries, distributing processing between the database backend and in-memory execution based on the database's capabilities. This hybrid approach ensures both performance and unparalleled flexibility in handling complex data access patterns that might not be natively supported by all underlying data stores. While currently providing a robust SQLite implementation, its pluggable `DatabaseInteractor` interface means it can seamlessly integrate with various SQL or NoSQL backends.

Go-Anansi champions a schema-driven development approach. All data interactions are governed by declarative schema definitions, ensuring data integrity and consistency. It intentionally provides a flexible, unopinionated core, allowing users to inject specific functionalities like migration systems as decorators, reinforcing its adaptable philosophy.

### Key Features

*   **Flexible Data Modeling**:
    *   `Document` type (`map[string]any`) for schema-aware, flexible data structures.
    *   Comprehensive JSON serialization/deserialization for `Document` and Go structs.
    *   Support for path-based access to nested data.
    *   Data transformation, diffing, and merging utilities.
    *   Robust type coercion capabilities.
*   **Robust Persistence Layer**:
    *   Abstracted `Persistence` interface supporting various data stores via a `DatabaseInteractor`.
    *   Centralized `CollectionRegistry` for managing schema definitions and versions.
    *   Automatic bootstrapping and management of an internal schema metadata store (`_schemas_`).
    *   Transactional operations for atomic data and schema changes with robust concurrency handling.
    *   Event-driven architecture providing hooks for observability and extensibility during persistence operations.
*   **Powerful Query Engine**:
    *   Domain-Specific Language (DSL) for expressive data querying (filtering, sorting, pagination, projection, joins, aggregations, distinct operations).
    *   Hybrid query execution that intelligently partitions queries between database and in-memory processing based on backend capabilities.
    *   Comprehensive join capabilities (Inner, Left, Right, Full) and aggregation functions.
*   **Schema Management & Validation**:
    *   Declarative schema definition supporting field types, constraints, indexes, and nested schemas.
    *   Graph-based validator for efficient, complex data validation, including conditional logic and circular dependency detection.
    *   Mechanisms for tracking and applying schema changes (migrations), with user-injectable data transformation logic.
    *   Code generation utilities (`codegen`) to create type-safe Go structs from schema definitions.
*   **SQLite Database Integration**:
    *   Concrete `DatabaseInteractor` implementation for SQLite databases.
    *   Automatic SQL generation for CRUD operations based on schema definitions, including advanced JSON functions.
*   **Developer Experience & Utilities**:
    *   `Playground` helper for quick development setup with SQLite.
    *   Extensive unit, integration, and end-to-end test suites (with recognized areas for further expansion).
    *   Generic utility functions for common tasks (e.g., type coercion, JSON handling, pointer helpers).

---

## 💾 Installation & Setup

### Prerequisites

*   **Go**: Version 1.24 or higher.
*   **SQLite**: The SQLite driver is automatically included, but ensure your system has `sqlite3` development libraries if compiling from source in some environments.

### Installation Steps

To integrate Go-Anansi into your Go project, use `go get`:

```bash
go get github.com/asaidimu/go-anansi/v6
```

### Basic Setup

Go-Anansi offers two primary ways to initialize the persistence layer:

1.  **`anansi.Setup`**: The production-grade path, offering full control over every component.
2.  **`anansi.Playground`**: A development-only helper for quick local testing with SQLite.

#### 1. Production Setup (`anansi.Setup`)

For production environments, you configure the `anansi.SetupConfig` with your chosen `DatabaseInteractor`, logger, event bus, custom decorators, and schema definitions.

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
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	u "github.com/asaidimu/go-anansi/v6/utils" // for NewWatermillEventBus
	_ "github.com/mattn/go-sqlite3"           // SQLite driver
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	db, err := sql.Open("sqlite3", "./production.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil {
		log.Fatalf("Failed to create SQLite executor: %v", err)
	}
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil {
		log.Fatalf("Failed to create native interactor: %v", err)
	}

	productSchema := &definition.Schema{
		Name:    "Product",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Name: "name", Type: "string", Required: &[]bool{true}[0]},
			"price": {Name: "price", Type: "number", Required: &[]bool{true}[0]},
			"stock": {Name: "stock", Type: "integer", Required: &[]bool{true}[0]},
		},
	}

	persistence, err := anansi.Setup(anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		EventBus:      u.NewWatermillEventBus[anansi.PersistenceEvent](logger),
		FactoryConfig: data.DocumentFactoryConfig{},
		Decorators:    &utils.Decorators{}, // Add custom decorators here
		Schemas:       []schema.SchemaDefinition{*productSchema},
	})
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}

	// Use the persistence instance...
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	productsCollection, err := persistence.Collection(ctx, "Product")
	if err != nil {
		log.Fatalf("Failed to get product collection: %v", err)
	}

	fmt.Println("Anansi production setup successful, ready to use!")
	_ = productsCollection // suppress unused variable warning
}
```

#### 2. Development Setup (`anansi.Playground`)

For local development and testing, `anansi.Playground` simplifies initialization by spinning up a complete SQLite-based environment. Remember to `defer cleanup()`!

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"go.uber.org/zap"
)

func getProductSchema() *definition.Schema {
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "Product",
			Fields: map[definition.FieldId]definition.Field{
				"name":  {Name: "name", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"price": {Name: "price", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"stock": {Name: "stock", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
		},
	}
}

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	productSchema := getProductSchema()

	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		DBPath:        ":memory:", // Use an in-memory database
		EnableLogging: true,
		EnableEvents:  true,
		Schemas:       []*definition.Schema{productSchema},
	})
	if err != nil {
		log.Fatalf("Failed to start playground: %v", err)
	}
	defer cleanup() // IMPORTANT: Ensure cleanup is deferred

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	products, err := p.Collection(ctx, productSchema.Name)
	if err != nil {
		log.Fatalf("Failed to get products collection: %v", err)
	}

	logger.Info("Anansi Playground initialized and ready for use!")
	_ = products // suppress unused variable warning
}
```

### Verification

After running either `Setup` or `Playground`, if no fatal errors occur, the persistence layer is initialized. You can verify by attempting to retrieve a collection:

```go
// ... (after anansi.Setup or anansi.Playground)

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

collectionNames, err := p.ListCollections(ctx)
if err != nil {
    log.Fatalf("Failed to list collections: %v", err)
}
fmt.Printf("Successfully listed collections: %v\n", collectionNames)
```

---

## 📖 Usage Documentation

Go-Anansi provides a rich API for interacting with your data. The core interfaces are `Persistence` (for global operations like managing collections) and `Collection` (for operations on a single collection).

### Basic CRUD Operations

Here's a basic example demonstrating how to define a schema, initialize the persistence layer (using `Playground` for simplicity), and perform CRUD operations:

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	coreutils "github.com/asaidimu/go-anansi/v6/core/utils"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// getProductSchema returns a minimal, valid Product schema.
func getProductSchema() *definition.Schema {
	return &definition.Schema{
		Name:    "Product",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Name: "name", Type: "string", Required: coreutils.BoolPtr(true)},
			"price": {Name: "price", Type: "number", Required: coreutils.BoolPtr(true)},
			"stock": {Name: "stock", Type: "integer", Required: coreutils.BoolPtr(true)},
		},
	}
}

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	productSchema := getProductSchema()

	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		DBPath:        "anansi.db",
		EnableLogging: false,
		EnableEvents:  true,
		Schemas:       []schema.SchemaDefinition{*productSchema},
	})
	if err != nil {
		log.Fatalf("Failed to start playground: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	products, err := p.Collection(ctx, productSchema.Name)
	if err != nil {
		log.Fatalf("Failed to get products collection: %v", err)
	}
	logger.Info("Products collection ready.")

	// Subscribe to a creation event (optional, demonstrates eventing)
	unsub := products.Subscribe(ctx, base.SubscriptionOptions{
		Event: base.DocumentCreateSuccess,
		Callback: func(ctx context.Context, event base.PersistenceEvent) error {
			logger.Info("Event: Document Created", zap.String("collection", *event.Collection), zap.Any("input", event.Input))
			return nil
		},
	})
	defer products.Unsubscribe(ctx, unsub)

	// --- CRUD Operations ---

	// Create
	p1 := data.MustNewDocument(map[string]any{"name": "Laptop", "price": 1200.00, "stock": 50})
	p2 := data.MustNewDocument(map[string]any{"name": "Mouse", "price": 25.00, "stock": 200})

	if _, err = products.CreateOne(ctx, p1); err != nil {
		log.Fatalf("Failed to create Laptop: %v", err)
	}
	logger.Info("Created Laptop.")

	if _, err = products.CreateOne(ctx, p2); err != nil {
		log.Fatalf("Failed to create Mouse: %v", err)
	}
	logger.Info("Created Mouse.")

	// Read all documents
	qAll := query.NewQueryBuilder().Build()
	resultAll, err := products.Read(ctx, &qAll)
	if err != nil {
		log.Fatalf("Read all failed: %v", err)
	}
	logger.Info("All Products:")
	if resultAll.Count > 0 {
		for _, doc := range resultAll.Data {
			logger.Info("  ", zap.String("id", doc.ID()), zap.Any("data", doc))
		}
	} else {
		logger.Info("No products found.")
	}

	// Update (reduce stock for Laptop)
	updateData := data.MustNewDocument(map[string]any{"stock": 45})
	filterLaptop := query.NewQueryBuilder().Where("id").Eq(p1.ID()).Build().Filters

	if _, err = products.Update(ctx, &base.CollectionUpdate{Filter: filterLaptop, Set: updateData}); err != nil {
		log.Fatalf("Update failed: %v", err)
	}
	logger.Info("Updated Laptop stock.")

	// Read updated Laptop
	qLaptop := query.NewQueryBuilder().Where("id").Eq(p1.ID()).Build()
	if resultLaptop, err := products.Read(ctx, &qLaptop); err != nil {
		log.Fatalf("Read updated Laptop failed: %v", err)
	}
	if resultLaptop.Count > 0 {
		doc := resultLaptop.Data.(data.Document)
		logger.Info("Laptop after update:", zap.Any("data", doc))
	}

	// Delete Mouse
	delFilterMouse := query.NewQueryBuilder().Where("id").Eq(p2.ID()).Build().Filters
	if _, err = products.Delete(ctx, delFilterMouse, false); err != nil {
		log.Fatalf("Delete failed: %v", err)
	}
	logger.Info("Deleted Mouse.")

	// Verify deletion
	qMouse := query.NewQueryBuilder().Where("id").Eq(p2.ID()).Build()
	if resultMouse, err := products.Read(ctx, &qMouse); err != nil {
		log.Fatalf("Verify deletion failed: %v", err)
	}
	if resultMouse.Count != 0 {
		logger.Error("Mouse still exists after delete!")
	} else {
		logger.Info("Mouse successfully deleted.")
	}
}
```

### Advanced Queries and Joins

The `core/query` package provides a powerful, fluent API for constructing complex queries, including joins and aggregations.

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
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	coreutils "github.com/asaidimu/go-anansi/v6/core/utils"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3"
)

// Schemas for User, Account, and LedgerTransaction
func getUserSchema() *definition.Schema {
	return &definition.Schema{
		Name:    "User", Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":    {Name: "id", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
			"name":  {Name: "name", Type: "string", Required: coreutils.BoolPtr(true)},
			"email": {Name: "email", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
		},
	}
}

func getAccountSchema() *definition.Schema {
	return &definition.Schema{
		Name:    "Account", Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":     {Name: "id", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
			"userId": {Name: "userId", Type: "string", Required: coreutils.BoolPtr(true)},
			"balance": {Name: "balance", Type: "number", Required: coreutils.BoolPtr(true)},
		},
	}
}

func getLedgerTransactionSchema() *definition.Schema {
	return &definition.Schema{
		Name:    "LedgerTransaction", Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":        {Name: "id", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
			"accountId": {Name: "accountId", Type: "string", Required: coreutils.BoolPtr(true)},
			"amount":    {Name: "amount", Type: "number", Required: coreutils.BoolPtr(true)},
			"type":      {Name: "type", Type: "string", Required: coreutils.BoolPtr(true)},
			"timestamp": {Name: "timestamp", Type: "integer", Required: coreutils.BoolPtr(true)},
		},
	}
}

func main() {
	// ... (Logger, DB, Interactor setup - similar to Basic Setup)
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil { log.Fatalf("Failed to open database: %v", err) }
	defer db.Close()

	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil { log.Fatalf("Failed to create SQLite interactor: %v", err) }
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil { log.Fatalf("Failed to create native interactor: %v", err) }

	factoryConfig := data.DocumentFactoryConfig{}
	cfg := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		FactoryConfig: factoryConfig,
		Decorators:    &utils.Decorators{}, // No custom decorators for this example, but can add.
	}
	p, err := anansi.Setup(cfg)
	if err != nil { log.Fatalf("Failed to setup Anansi: %v", err) }
	logger.Info("Anansi persistence layer initialized successfully.")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create collections
	usersCollection, _ := p.CreateCollection(ctx, *getUserSchema())
	accountsCollection, _ := p.CreateCollection(ctx, *getAccountSchema())
	transactionsCollection, _ := p.CreateCollection(ctx, *getLedgerTransactionSchema())
	
	// Populate Data
	user1 := data.MustNewDocument(map[string]any{"id": "U001", "name": "Alice", "email": "alice@example.com"})
	user2 := data.MustNewDocument(map[string]any{"id": "U002", "name": "Bob", "email": "bob@example.com"})
	usersCollection.CreateMany(ctx, []data.Document{user1, user2})

	account1 := data.MustNewDocument(map[string]any{"id": "A001", "userId": "U001", "balance": 1000.00})
	account2 := data.MustNewDocument(map[string]any{"id": "A002", "userId": "U002", "balance": 500.00})
	accountsCollection.CreateMany(ctx, []data.Document{account1, account2})

	tx1 := data.MustNewDocument(map[string]any{"id": "T001", "accountId": "A001", "amount": 200.00, "type": "deposit", "timestamp": time.Now().Unix()})
	tx2 := data.MustNewDocument(map[string]any{"id": "T002", "accountId": "A002", "amount": 50.00, "type": "withdrawal", "timestamp": time.Now().Unix()})
	transactionsCollection.CreateMany(ctx, []data.Document{tx1, tx2})

	// --- Complex Queries with Joins ---

	// Get all transactions for Alice (User U001)
	logger.Info("Querying all transactions for Alice (User U001)...")
	aliceTransactionsQuery := query.NewQueryBuilder().
		From("LedgerTransaction").
		LeftJoin("Account").On(query.QueryFilter{
			Condition: &query.FilterCondition{Field: "LedgerTransaction.accountId", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{FieldRefVal: &query.FieldReference{Field: "Account.id"}}}},
		).End().
		LeftJoin("User").On(query.QueryFilter{
			Condition: &query.FilterCondition{Field: "Account.userId", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{FieldRefVal: &query.FieldReference{Field: "User.id"}}}},
		).End().
		Where("User.id").Eq("U001").
		Build()

	txResult, err := transactionsCollection.Read(ctx, &aliceTransactionsQuery)
	if err != nil { log.Fatalf("Failed to query transactions for Alice: %v", err) }

	fmt.Printf("Found %d transactions for Alice:\n", txResult.Count)
	if txResult.Count > 0 {
		docs := txResult.Data
		for _, doc := range docs {
			ledgerTx := doc["LedgerTransaction"].(map[string]any)
			user := doc["User"].(map[string]any)
			fmt.Printf("  Transaction ID: %s, Amount: %.2f, Type: %s, Account ID: %s, User Name: %s\n",
				ledgerTx["id"], ledgerTx["amount"], ledgerTx["type"], ledgerTx["accountId"], user["name"])
		}
	}
}
```

### Event-Driven Architecture

Subscribe to `PersistenceEvent`s to react to data changes across your application:

```go
// ... (Persistence and Collection setup as above)

// Subscribe to a specific event type, e.g., DocumentCreateSuccess
unsubCreateSuccess := products.Subscribe(context.Background(), base.SubscriptionOptions{
	Event: base.DocumentCreateSuccess,
	Callback: func(ctx context.Context, event base.PersistenceEvent) error {
		logger.Info("Caught DocumentCreateSuccess event!",
			zap.String("collection", *event.Collection),
			zap.Any("document_data", event.Input))
		// Here you could trigger external systems, invalidate caches, etc.
		return nil
	},
	Label: coreutils.StringPtr("ProductCreatedLogger"),
})
defer products.Unsubscribe(context.Background(), unsubCreateSuccess) // Don't forget to unsubscribe!

// Subscribe to all events for a collection
unsubAllCollectionEvents := products.Subscribe(context.Background(), base.SubscriptionOptions{
	Event: base.PersistenceEvents, // Wildcard event for collection
	Callback: func(ctx context.Context, event base.PersistenceEvent) error {
		logger.Debug("Caught ALL collection events",
			zap.String("event_type", string(event.Type)),
			zap.String("collection", *event.Collection))
		return nil
	},
})
defer products.Unsubscribe(context.Background(), unsubAllCollectionEvents)

// Perform an operation that triggers the event
p1 := data.MustNewDocument(map[string]any{"name": "Monitor", "price": 300.00, "stock": 100})
_, err = products.CreateOne(ctx, p1)
if err != nil {
	log.Fatalf("Failed to create Monitor: %v", err)
}
```

### Custom Decorators (Extension Points)

The decorator pattern is central to Go-Anansi's extensibility. You can inject custom logic into the persistence pipeline by creating `CollectionDecorator` or `PersistenceDecorator` functions.

Here's an example of a `CollectionDecorator` that enforces a negative amount check for a `LedgerTransaction` collection:

```go
package main

import (
	"context"
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
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// NegativeAmountValidator is a CollectionDecorator that prevents transactions with negative amounts.
func NegativeAmountValidator(logger *zap.Logger) utils.CollectionDecorator {
	return func(next base.Collection) base.Collection {
		return &negativeAmountValidator{
			next:   next,
			logger: logger,
		}
	}
}

type negativeAmountValidator struct {
	next   base.Collection
	logger *zap.Logger
}

var _ base.Collection = (*negativeAmountValidator)(nil)

func (d *negativeAmountValidator) validateAmount(doc data.Document) error {
	if val, ok := doc["amount"]; ok {
		if amount, isFloat := val.(float64); isFloat {
			if amount < 0 {
				d.logger.Warn("Attempted to create/update transaction with negative amount", zap.Float64("amount", amount))
				return fmt.Errorf("transaction amount cannot be negative: %f", amount)
			}
		} else {
			return fmt.Errorf("invalid amount type for document %v, expected float64", doc)
		}
	}
	return nil
}

// CreateOne method overridden to apply custom validation
func (d *negativeAmountValidator) CreateOne(ctx context.Context, doc data.Document) (base.CreateResult, error) {
	if err := d.validateAmount(doc); err != nil {
		return base.CreateResult{Status: base.StatusFailedValidation, Data: doc, Issues: []common.Issue{{Message: err.Error()}}}, err
	}
	return d.next.CreateOne(ctx, doc)
}

// All other Collection interface methods would typically delegate to d.next without modification,
// unless specific interception logic is required. For brevity, only `CreateOne` is shown here.
// In a full implementation, all methods of `base.Collection` need to be implemented,
// typically by calling `d.next.<MethodName>(...)`.

func (d *negativeAmountValidator) CreateMany(ctx context.Context, docs []data.Document) ([]base.CreateResult, error) { /* ... delegate ... */ return d.next.CreateMany(ctx, docs) }
func (d *negativeAmountValidator) Read(ctx context.Context, query *query.Query) (*base.ReadResult, error) { /* ... delegate ... */ return d.next.Read(ctx, query) }
func (d *negativeAmountValidator) Update(ctx context.Context, params *base.CollectionUpdate) (int, error) {
	if err := d.validateAmount(params.Set); err != nil {
		return 0, err
	}
	return d.next.Update(ctx, params)
}
func (d *negativeAmountValidator) Delete(ctx context.Context, queryFilter *query.QueryFilter, unsafe bool) (int, error) { /* ... delegate ... */ return d.next.Delete(ctx, queryFilter, unsafe) }
func (d *negativeAmountValidator) Validate(ctx context.Context, data data.Document, loose bool) (*schema.ValidationResult, error) { /* ... delegate ... */ return d.next.Validate(ctx, data, loose) }
func (d *negativeAmountValidator) Metadata(ctx context.Context, filter *base.MetadataFilter, forceRefresh bool) (*base.CollectionMetadata, error) { /* ... delegate ... */ return d.next.Metadata(ctx, filter, forceRefresh) }
func (d *negativeAmountValidator) Subscribe(ctx context.Context, options base.SubscriptionOptions) string { /* ... delegate ... */ return d.next.Subscribe(ctx, options) }
func (d *negativeAmountValidator) Unsubscribe(ctx context.Context, id string) { d.next.Unsubscribe(ctx, id) }
func (d *negativeAmountValidator) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) { /* ... delegate ... */ return d.next.Subscriptions(ctx) }
func (d *negativeAmountValidator) Capabilities(ctx context.Context) *query.Capabilities { /* ... delegate ... */ return d.next.Capabilities(ctx) }


func main() {
	// ... (Logger, DB, Interactor setup - similar to Basic Setup)
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil { log.Fatalf("Failed to open database: %v", err) }
	defer db.Close()

	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil { log.Fatalf("Failed to create SQLite interactor: %v", err) }
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil { log.Fatalf("Failed to create native interactor: %v", err) }

	// 5. Setup Decorators - Add our custom validator
	decorators := &utils.Decorators{
		CollectionDecorators: []utils.DecoratorFunc[base.Collection]{
			(utils.DecoratorFunc[base.Collection])(NegativeAmountValidator(logger)),
		},
	}
	
	// Initialize Anansi Persistence Layer with custom decorators
	cfg := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		FactoryConfig: data.DocumentFactoryConfig{},
		Decorators:    decorators,
	}
	p, err := anansi.Setup(cfg)
	if err != nil { log.Fatalf("Failed to setup Anansi: %v", err) }
	logger.Info("Anansi persistence layer initialized successfully.")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a transaction schema
	ledgerTransactionSchema := getLedgerTransactionSchema()
	transactionsCollection, _ := p.CreateCollection(ctx, *ledgerTransactionSchema)

	// Attempt to create an invalid transaction (negative amount)
	logger.Info("Attempting to create invalid transaction (negative amount)...")
	invalidTx := data.MustNewDocument(map[string]any{"id": "T003", "accountId": "A001", "amount": -10.00, "type": "withdrawal", "timestamp": time.Now().Unix()})
	_, err = transactionsCollection.CreateOne(ctx, invalidTx)
	if err != nil {
		logger.Info(fmt.Sprintf("Successfully prevented invalid transaction: %v", err))
	} else {
		log.Fatalf("ERROR: Invalid transaction (negative amount) was created!")
	}
}

func getLedgerTransactionSchema() *definition.Schema {
	return &definition.Schema{
		Name:    "LedgerTransaction", // Renamed from Transaction
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":        {Name: "id", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
			"accountId": {Name: "accountId", Type: "string", Required: coreutils.BoolPtr(true)}, // Foreign key to Account
			"amount":    {Name: "amount", Type: "number", Required: coreutils.BoolPtr(true)},
			"type":      {Name: "type", Type: "string", Required: coreutils.BoolPtr(true)}, // e.g., "deposit", "withdrawal"
			"timestamp": {Name: "timestamp", Type: "integer", Required: coreutils.BoolPtr(true)},
		},
	}
}

```

### Query Language (DSL)

Go-Anansi provides a robust internal Domain-Specific Language (DSL) for constructing queries, which is directly translated to the underlying database's query language (e.g., SQL for SQLite). The DSL supports rich filtering, sorting, pagination, projection, joins, and aggregations.

For a comprehensive specification of the query grammar and its JSON mapping, refer to the [QUERYLANG.md](QUERYLANG.md) document in the repository.

### API Server Example

The `example/api` directory provides a more complete example of how to build a RESTful API service on top of Go-Anansi. It demonstrates:

*   Loading schema definitions from JSON files (`example/api/schema`).
*   Initializing the `anansi.Persistence` layer.
*   Implementing HTTP handlers for collection and document CRUD operations, including validation and metadata handling.
*   Consistent API response and error handling.

Refer to `example/api/main.go` and `example/api/internal/api/server.go` for implementation details, and `example/api/spec.md` for the full API documentation.

---

## 🏗️ Project Architecture

Go-Anansi's architecture is a testament to modular design, leveraging interfaces and the decorator pattern to achieve a flexible, extensible, and robust persistence layer.

### Core Components

*   **`Persistence` Interface**: The top-level facade, offering global operations like `CreateCollection`, `ListCollections`, `Transact`, and `Metadata`. It's the primary entry point for users to interact with the system.
*   **`Collection` Interface**: Represents a single data collection (analogous to a database table). It defines CRUD operations (`CreateOne`, `Read`, `Update`, `Delete`), `Validate` against its schema, and collection-specific `Metadata` and `Subscribe` methods.
*   **`CollectionRegistry`**: A self-managing internal component that stores and manages schema definitions and their versions. It handles the mapping of logical collection names to physical database names (e.g., table names) and their associated schema definitions.
*   **`Document` (`core/data`)**: The universal data structure, essentially a `map[string]any` supercharged with fluent APIs for data access, transformations, serialization, and integrity features (hashing, signing).
*   **`SchemaDefinition` (`core/schema`)**: Declaratively defines the structure, types, constraints, and indexes for collections. The `core/schema` package also includes a powerful graph-based validator for ensuring data integrity.
*   **`QueryEngine` (`core/query`)**: The brain of data retrieval. It takes an abstract `Query` DSL object, partitions it based on the `DatabaseInteractor`'s `Capabilities`, and orchestrates execution between the database backend and in-memory processing (`QueryHelper`).
*   **`DatabaseInteractor` (`core/query/native`)**: An interface that abstracts the underlying database technology. It defines the contract for executing native queries (e.g., SQL) for SELECT, INSERT, UPDATE, and DELETE operations, as well as transaction management.
*   **`SQLiteExecutor` (`sqlite/executor`)**: The concrete implementation of `DatabaseInteractor` for SQLite, including SQL generation and execution logic.
*   **`EventEmitter` (`core/events`)**: A generic publish-subscribe system that allows for decoupled observation and reaction to lifecycle events within the persistence layer.

### Data Flow

1.  **`Persistence.Collection(name)`**: Looks up collection metadata in `CollectionRegistry`. If not cached, it fetches the schema definition and its physical name. It then instantiates and caches a `Collection` instance for that name.
2.  **`Collection.Read(query)`**: The `Query` DSL object is passed to the `QueryEngine`.
3.  **`QueryEngine`**: Utilizes a `QueryPartitioner` to compare the `Query`'s requirements against the `DatabaseInteractor`'s declared `Capabilities`. The query is split into a "Database Query" (what the database can handle natively) and a "Residual Query" (what must be processed in-memory by Go).
4.  **`DatabaseInteractor.SelectDocuments(dbQuery)`**: The database-specific implementation (e.g., `SQLiteExecutor`) translates the "Database Query" into native commands (e.g., SQL) and executes them, returning a preliminary set of `Document`s.
5.  **`QueryHelper.ApplyResidualQuery(result, residualQuery)`**: The `QueryEngine` then uses an in-memory `QueryHelper` to apply any remaining filtering, sorting, or projections from the "Residual Query" on the documents fetched from the database.
6.  **Results**: The final, fully processed `Document`s are returned to the user, enriched with system metadata (`_metadata_`).
7.  **Eventing**: Throughout CRUD and lifecycle operations, `eventsPersistence` and `eventsCollection` decorators publish `PersistenceEvent`s to the `EventEmitter`, allowing registered subscribers to react.

### Extension Points

Go-Anansi is highly extensible through several mechanisms:

*   **Decorator Pattern**: The `Persistence` and `Collection` interfaces are designed to be wrapped by custom decorators (`utils.DecoratorFunc`). This allows users to inject cross-cutting concerns (e.g., authentication, auditing, caching, custom validation, encryption, multi-tenancy) without altering the core logic. (See `example/complex` and `example/advanced` for examples).
*   **Pluggable `DatabaseInteractor`**: The core `query.DatabaseInteractor` interface allows you to swap out the underlying database backend. Implement this interface to connect Go-Anansi to PostgreSQL, MySQL, MongoDB, or any other data store.
*   **Custom `MetadataProvider`**: The `data.DocumentFactoryConfig` allows you to inject custom providers for document metadata (e.g., custom ID generation, custom timestamp formats).
*   **Schema Predicates & Functions**: The schema validation system supports custom `Predicate` functions that can be registered to extend the validation rules.
*   **Code Generation**: The `core/schema/codegen` package allows generating type-safe Go structs directly from your `SchemaDefinition`s, bridging the gap between dynamic data and static Go types. This is a powerful tool for enhancing developer experience and compile-time safety.

---

## 🧑‍💻 Development & Contributing

We welcome contributions! Please follow these guidelines to ensure a smooth development process.

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
3.  **Build the project:**
    ```bash
    go build -v ./...
    ```

### Scripts

The project includes a `Makefile` for common development tasks:

*   `make build`: Compiles the entire project.
*   `make test`: Runs all unit and integration tests. It also cleans the test cache.

The `bin/bump.sh` script is provided for safely upgrading the Go module's major version across the codebase. Always use `--dry-run` first:

```bash
./bin/bump.sh <OLD_VERSION_NUMBER> <NEW_VERSION_NUMBER> --dry-run
./bin/bump.sh <OLD_VERSION_NUMBER> <NEW_VERSION_NUMBER> # To apply changes
go mod tidy
```

### Testing

Go-Anansi has a comprehensive test suite. To run tests:

```bash
make test
```

The `test-gap.md` document outlines areas needing further test coverage, and contributions improving test coverage are highly appreciated. Focus areas include:

*   `core/schema/validator.go` and `core/query/helper.go` require extensive, robust test suites for edge cases, deep nesting, and complex logic.
*   `core/data/document.go` needs more tests for complex `DeepMerge` scenarios, deep copy verification in `Clone`, and `Equals` method behavior.
*   `core/persistence/registry/registry.go` could benefit from more detailed transactional rollback tests for `CreateCollections`, `DropCollection`, `PruneVersion`, and `AddSchemaVersion`.
*   `sqlite/executor/executor.go` needs more tests for `QueryStream` context cancellation and various error conditions.
*   `sqlite/query/builder.go` requires comprehensive DSL coverage for all `buildXTree` methods, especially SQL injection prevention and schema mapping.

### Contributing Guidelines

1.  **Fork** the repository.
2.  **Create a new branch** for your feature or bug fix.
3.  **Write clear, concise code** that adheres to Go best practices and existing coding style.
4.  **Write unit and integration tests** for your changes.
5.  **Ensure all tests pass** (`make test`).
6.  **Update documentation** as needed.
7.  **Submit a Pull Request** with a detailed description of your changes.

### Issue Reporting

If you encounter any bugs, have feature requests, or need assistance, please [open an issue](https://github.com/asaidimu/go-anansi/issues). Provide as much detail as possible, including steps to reproduce, expected vs. actual behavior, and your environment.

---

## 📚 Additional Information

### Troubleshooting

*   **`ERR_REGISTRY_FAILED_TO_CREATE_REGISTRY_COLLECTION`**: This usually indicates an issue with the underlying database connection or permissions during initial setup of the internal `_schemas_` collection. Ensure your database is accessible and writable.
*   **`ERR_PERSISTENCE_VALIDATION_FAILED`**: Your document data does not conform to the defined `SchemaDefinition`. Check the `Issues` field in the error for detailed validation messages, including the exact path and reason for failure.
*   **Performance with `bind.go` or `Clone()`**: Operations involving reflection (like `data.Document.Bind()`) or deep cloning (`data.Document.Clone()`) can have performance implications. For high-performance scenarios, consider optimizing your data structures or avoiding excessive cloning.

### FAQ

**Q: Does Go-Anansi have a built-in migration system?**
A: Go-Anansi provides the *mechanisms* for tracking and applying schema changes (as described in `schema.Migration`), but the actual data transformation logic (`Transformer` functions) is designed to be injected by the user as a decorator. This offers maximum flexibility for custom migration strategies. API endpoints for `Migrate` and `Rollback` are defined, but their full implementation for data transformation requires user-defined logic.

**Q: Can I use Go-Anansi with databases other than SQLite?**
A: Yes! Go-Anansi is designed with a pluggable `DatabaseInteractor` interface. While SQLite is the reference implementation, you can implement this interface for any other SQL or NoSQL database (e.g., PostgreSQL, MySQL, MongoDB).

**Q: How does Go-Anansi handle concurrency?**
A: The framework is built with concurrency safety in mind. It uses mutexes for shared resources and a robust `Transaction` manager (within `core/persistence/transaction`) that correctly handles nesting and concurrent operations via `sync.WaitGroup` to ensure atomicity. Optimistic locking is also transparently implemented for updates.

### Roadmap

Future enhancements and areas of active development include:

*   **Enhanced Migration System**: Further development of data transformation capabilities and tools for `Migrate` and `Rollback` operations.
*   **Pluggable Full-Text Search**: Integration of a high-performance, pluggable full-text search capability leveraging a coordinated dual-cursor architecture, as detailed in the [search.md](search.md) proposal. This will enable both consistent filter-based searches and real-time streaming search experiences.
*   **More Database Interactors**: Community contributions for PostgreSQL, MySQL, or other database backends.
*   **Advanced Query Optimizations**: Further intelligence in the `QueryEngine` for database-specific optimizations and query plan generation.
*   **Code Generation Enhancements**: Expanding the `codegen` package to support more schema types (e.g., unions, enums) and automatically manage Go imports.

---

## 📜 License

This project is licensed under the **Anansi Platform Proprietary License**.

**IMPORTANT: This is a source-available license. It is NOT an open-source license as defined by the Open Source Initiative. A separate commercial license is required for any form of commercial use.**

For the full text of the license, please see the [LICENSE.md](LICENSE.md) file.

To obtain a commercial license or discuss consulting engagements, please contact the Licensor.

---

## 🙏 Acknowledgments

Go-Anansi is a product of CyberSync Printers & Stationers - BN-P7SABM5J. All rights reserved.