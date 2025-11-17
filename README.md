# `go-anansi`: A Resilient Go Persistence Platform 🕷️

[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/asaidimu/go-anansi/v6)
[![Go Report Card](https://goreportcard.com/badge/github.com/asaidimu/go-anansi/v6)](https://goreportcard.com/report/github.com/asaidimu/go-anansi/v6)
[![License](https://img.shields.io/badge/License-Proprietary-red.svg)](./LICENSE.md)
[![Current Version](https://img.shields.io/badge/Version-v6-blue.svg)](https://github.com/asaidimu/go-anansi/releases)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg?style=flat)](https://github.com/asaidimu/go-anansi/actions)

## 🔗 Quick Links

*   [Overview & Features](#-overview--features)
*   [Installation & Setup](#-installation--setup)
*   [Usage Documentation](#-usage-documentation)
    *   [Basic CRUD with Playground](#basic-crud-with-playground)
    *   [Production Setup](#production-setup)
    *   [Event-Driven Persistence](#event-driven-persistence)
    *   [Application-Level Row-Level Security](#application-level-row-level-security)
    *   [Complex Queries & Joins](#complex-queries--joins)
    *   [API Usage Example](#api-usage-example)
*   [Project Architecture](#-project-architecture)
*   [Development & Contributing](#-development--contributing)
*   [Additional Information](#-additional-information)

---

## 🕷️ Overview & Features

`go-anansi` is a powerful, flexible, and schema-driven persistence platform for Go applications. Inspired by the resilience and intricate design of a spider's web, Anansi provides a robust abstraction layer over various data stores, enabling developers to define their data models declaratively, perform complex queries, and manage data lifecycle with ease and confidence.

Designed for modern Go applications, `go-anansi` simplifies data interaction by offering a unified API for CRUD operations, transactional guarantees, and an event-driven architecture for real-time observability and extensibility. It emphasizes strong data consistency through schema validation and versioning, while its pluggable design allows seamless integration with different database technologies, starting with a robust SQLite implementation.

### ✨ Key Features

*   **Flexible Data Modeling:**
    *   `Document` type for schema-aware, flexible data structures with comprehensive JSON serialization/deserialization.
    *   Path-based access to nested data.
    *   Data transformation, diffing, merging, and robust type coercion utilities.
*   **Robust Persistence Layer:**
    *   Abstracted `Persistence` interface supporting various data stores.
    *   Centralized `Collection Registry` for managing schema definitions and versions.
    *   Automatic bootstrapping and management of internal schema metadata.
    *   Transactional operations for atomic data and schema changes.
    *   Event-driven architecture for persistence operations, offering hooks for auditing, logging, and custom logic.
*   **Powerful Query Engine:**
    *   Domain-Specific Language (DSL) for expressive data querying.
    *   Support for filtering, sorting, pagination (offset and cursor-based), and field projection.
    *   Aggregation functions (e.g., count, sum) and distinct operations.
    *   In-memory query helper for testing and pre-processing.
    *   Comprehensive join capabilities (Inner, Left, Right, Full).
*   **Schema Management & Validation:**
    *   Declarative schema definition including field types, constraints, and indexes.
    *   Automatic data validation against defined schemas.
    *   Mechanisms for tracking and applying schema changes (migrations).
*   **SQLite Database Integration:**
    *   Concrete, production-ready implementation of the persistence layer for SQLite databases.
    *   Automatic SQL generation for CRUD operations based on schema definitions.
*   **Developer Experience & Utilities:**
    *   Extensive unit, integration, and end-to-end test suites.
    *   Generic utility functions for common tasks (type coercion, JSON handling, pointer helpers).

### 🚀 Upcoming Features (Proposals)

We are actively working on enhancing `go-anansi` with powerful new capabilities:

*   **Persistence Layer Health Check (`persistence.HealthCheck`):** A method to proactively verify the integrity and consistency of the persistence layer, including database connectivity and physical collection existence, upon application startup or on demand.
*   **Enforcing Document Schema at Creation and Runtime Validation (`collection.Document()`):** A `Collection.Document()` factory method that returns a new `data.Document` instance pre-populated with schema-defined defaults and structured to reflect the schema. This `SchemaAwareDocument` will also enforce schema types and constraints at runtime during `Set` and `SetNested` operations, providing immediate feedback.
*   **Pluggable Full-Text Search Functionality:** A comprehensive, pluggable search solution leveraging a coordinated dual-cursor architecture for storage efficiency and performance. This includes both consistent search-as-a-filter via `Read()` and real-time streaming search via a new `Search()` method. It will support pluggable backends (e.g., Bleve) and event-driven indexing.

---

## 🛠️ Installation & Setup

### Prerequisites

*   **Go**: Version `1.24.4` or higher (from `go.mod`).
*   **SQLite3**: The `github.com/mattn/go-sqlite3` driver is used for SQLite integration. Ensure necessary build tools for CGo are available if compiling from source.

### Installation Steps

To integrate `go-anansi` into your Go project, use `go get`:

```bash
go get github.com/asaidimu/go-anansi/v6
```

### Configuration

`go-anansi` offers two primary ways to initialize the persistence layer:

1.  **`anansi.Setup`**: The **production path**, providing granular control over every component.
2.  **`anansi.Playground`**: A **development-only helper** for quick setup of an in-memory or file-based SQLite environment. It's not intended for production.

#### `anansi.SetupConfig` (Production)

This configuration struct allows you to inject custom implementations for various core components.

```go
type SetupConfig struct {
	Interactor    query.DatabaseInteractor        // Concrete database implementation (e.g., SQLite, PostgreSQL)
	Logger        *zap.Logger                     // Structured logger for framework operations
	EventBus      events.EventBus[base.PersistenceEvent] // Pub/sub backbone for events
	FactoryConfig data.DocumentFactoryConfig      // Configures Document factory (hashing, metadata)
	Decorators    *utils.Decorators               // Inject cross-cutting concerns (security, audit)
	Schemas       []schema.SchemaDefinition       // Schemas to auto-create on first start
}
```

#### `anansi.PlaygroundConfig` (Development)

A simplified configuration for development environments.

```go
type PlaygroundConfig struct {
	DBPath        string                    // SQLite DSN (e.g., ":memory:", "file.db")
	EnableLogging bool                      // Enable zap.NewDevelopment() logger
	EnableEvents  bool                      // Enable WatermillEventBus
	Schemas       []schema.SchemaDefinition // Schemas to auto-create
}
```

### Verification

To verify your installation and get a quick feel for `go-anansi`, run the basic example:

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

func getProductSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
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

	// Initialize with Playground for development
	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		DBPath:        ":memory:", // Use in-memory SQLite for quick testing
		EnableLogging: true,
		EnableEvents:  false,
		Schemas:       []schema.SchemaDefinition{*productSchema},
	})
	if err != nil {
		log.Fatalf("Failed to start playground: %v", err)
	}
	defer cleanup() // IMPORTANT: Defer cleanup for Playground environments!

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	products, err := p.Collection(ctx, productSchema.Name)
	if err != nil {
		log.Fatalf("Failed to get products collection: %v", err)
	}
	logger.Info("Products collection ready.")

	// Perform a simple Create operation
	newProduct := data.MustNewDocument(map[string]any{"name": "Test Gadget", "price": 99.99, "stock": 100})
	createResult, err := products.CreateOne(ctx, newProduct)
	if err != nil {
		log.Fatalf("Failed to create product: %v", err)
	}
	logger.Info("Product created:", zap.Any("product", createResult.Data))
}
```

---

## 📖 Usage Documentation

### Basic CRUD with Playground

The `anansi.Playground` function provides a convenient way to quickly spin up a fully functional `go-anansi` environment for development and testing. It handles database initialization, logger setup, and schema loading.

```go
// From example/basic/main.go
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
func getProductSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
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
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	productSchema := getProductSchema()

	// 3. Start Playground with full dev features (using a persistent file for example)
	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		DBPath:        "anansi.db", // This will create anansi.db in your current directory
		EnableLogging: true,
		EnableEvents:  true,
		Schemas:       []schema.SchemaDefinition{*productSchema},
	})
	if err != nil {
		log.Fatalf("Failed to start playground: %v", err)
	}
	defer cleanup() // !!! IMPORTANT: Always defer cleanup() in Playground environments

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	products, err := p.Collection(ctx, productSchema.Name)
	if err != nil {
		log.Fatalf("Failed to get products collection: %v", err)
	}
	logger.Info("Products collection ready.")

	// --- CRUD Operations ---

	// Create
	p1 := data.MustNewDocument(map[string]any{"name": "Laptop", "price": 1200.00, "stock": 50})
	p2 := data.MustNewDocument(map[string]any{"name": "Mouse", "price": 25.00, "stock": 200})

	if _, err = products.CreateOne(ctx, p1); err != nil {
		log.Fatalf("Failed to create Laptop: %v", err)
	}
	if _, err = products.CreateOne(ctx, p2); err != nil {
		log.Fatalf("Failed to create Mouse: %v", err)
	}

	// Read all products
	q := query.NewQueryBuilder().Build()
	result, err := products.Read(ctx, &q)
	if err != nil {
		log.Fatalf("Read failed: %v", err)
	}

	if result.Count > 0 {
		for _, doc := range result.Data.([]data.Document) {
			logger.Info("Found",
				zap.String("id", doc.ID()),
				zap.String("name", doc["name"].(string)),
				zap.Float64("price", doc["price"].(float64)),
				zap.Int64("stock", doc["stock"].(int64)),
			)
		}
	} else {
		logger.Info("No products found.")
	}

	// Update (reduce stock of Laptop)
	update := data.MustNewDocument(map[string]any{"stock": 45})
	filter := query.NewQueryBuilder().Where("id").Eq(p1.ID()).Build().Filters

	if _, err = products.Update(ctx, &base.CollectionUpdate{Filter: filter, Set: update}); err != nil {
		log.Fatalf("Update failed: %v", err)
	}
	logger.Info("Updated Laptop stock.")

	// Read updated Laptop
	q = query.NewQueryBuilder().Where("id").Eq(p1.ID()).Build()
	if result, err = products.Read(ctx, &q); err != nil {
		log.Fatalf("Read updated failed: %v", err)
	}
	if result.Count > 0 {
		doc := result.Data.(data.Document)
		logger.Info("After update",
			zap.String("id", doc.ID()),
			zap.Int64("stock", doc.Must().GetInt64("stock")), // Using Must() for brevity
		)
	}

	// Delete Mouse
	delFilter := query.NewQueryBuilder().Where("id").Eq(p2.ID()).Build().Filters
	if _, err = products.Delete(ctx, delFilter, false); err != nil {
		log.Fatalf("Delete failed: %v", err)
	}
	logger.Info("Deleted Mouse")

	// Verify deletion
	q = query.NewQueryBuilder().Where("id").Eq(p2.ID()).Build()
	if result, err = products.Read(ctx, &q); err != nil {
		log.Fatalf("Verify failed: %v", err)
	}
	if result.Count != 0 {
		logger.Error("Mouse still exists after delete!")
	}
}
```

### Production Setup

For production environments, you'll use `anansi.Setup` to explicitly configure each component.

```go
package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// Example production setup for an in-memory SQLite database.
func main() {
	logger, err := zap.NewProduction() // Use production logger
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// 1. Setup Database Interactor (e.g., SQLite)
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		logger.Fatal("Failed to open database", zap.Error(err))
	}
	defer db.Close()

	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil {
		logger.Fatal("Failed to create SQLite executor", zap.Error(err))
	}
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil {
		logger.Fatal("Failed to create native interactor", zap.Error(err))
	}

	// 2. Setup Event Bus (e.g., Watermill in-memory for simplicity)
	eventBus := utils.NewWatermillEventBus[base.PersistenceEvent](logger)
	defer eventBus.Close()

	// 3. Define Schemas
	userSchema := schema.SchemaDefinition{
		Name:    "User",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"username": {Name: "username", Type: "string", Required: utils.BoolPtr(true), Unique: utils.BoolPtr(true)},
			"email":    {Name: "email", Type: "string", Required: utils.BoolPtr(true)},
		},
	}

	// 4. Configure Anansi Setup
	setupConfig := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		EventBus:      eventBus,
		FactoryConfig: data.DocumentFactoryConfig{}, // Default config
		Decorators:    &utils.Decorators{},         // No custom decorators for now
		Schemas:       []schema.SchemaDefinition{userSchema},
	}

	// 5. Initialize Anansi
	p, err := anansi.Setup(setupConfig)
	if err != nil {
		logger.Fatal("Failed to setup Anansi", zap.Error(err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get collection and perform operations
	usersCollection, err := p.Collection(ctx, "User")
	if err != nil {
		logger.Fatal("Failed to get User collection", zap.Error(err))
	}

	newUser := data.MustNewDocument(map[string]any{"username": "johndoe", "email": "john@example.com"})
	_, err = usersCollection.CreateOne(ctx, newUser)
	if err != nil {
		logger.Error("Failed to create user", zap.Error(err))
	} else {
		logger.Info("User created successfully!")
	}
}
```

### Event-Driven Persistence

`go-anansi` is built with an event-driven core, allowing you to react to persistence operations in real-time.

```go
// From example/events/main.go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/events"
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

// Product schema definition
func getProductSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
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

	db, _ := sql.Open("sqlite3", "file::memory:?cache=shared")
	defer db.Close()

	executor, _ := sqliteExecutor.NewSQLiteExecutor(db, logger)
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, _ := native.NewNativeInteractor(executor, queryFactory, logger)

	factoryConfig := data.DocumentFactoryConfig{}
	decorators := &utils.Decorators{}
	bus := utils.NewWatermillEventBus[base.PersistenceEvent](logger) // Use Watermill for event bus
	defer bus.Close()

	cfg := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		FactoryConfig: factoryConfig,
		Decorators:    decorators,
		EventBus:      bus, // Inject the event bus
	}
	p, err := anansi.Setup(cfg)
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	productSchema := getProductSchema()
	productsCollection, err := p.CreateCollection(ctx, *productSchema)
	if err != nil {
		log.Fatalf("Failed to create products collection: %v", err)
	}
	logger.Info("Products collection created.")

	// Subscribe to DocumentCreateSuccess events specifically for the "Product" collection
	unsub := productsCollection.Subscribe(context.Background(), base.SubscriptionOptions{
		Event: base.DocumentCreateSuccess,
		Callback: func(ctx context.Context, event base.PersistenceEvent) error {
			logger.Info(fmt.Sprintf("Event received: Product created in '%s' \n %v", *event.Collection, event.Input))
			// You can add custom logic here, e.g., send a notification, update a cache, etc.
			return nil
		},
		Label:       coreutils.StringPtr("ProductCreationNotifier"),
		Description: coreutils.StringPtr("Logs details of newly created products."),
	})
	defer productsCollection.Unsubscribe(context.Background(), unsub)

	// Create a product - this will trigger the subscribed event
	product1 := data.MustNewDocument(map[string]any{"id": "P001", "name": "Laptop", "price": 1200.00, "stock": 50})
	_, err = productsCollection.CreateOne(ctx, product1)
	if err != nil {
		log.Fatalf("Failed to create product P001: %v", err)
	}
	logger.Info("Created product P001: Laptop")

	// The event callback will now be triggered and log the creation.
	// Add a small delay to ensure event processing has a chance to occur in non-transactional contexts.
	time.Sleep(100 * time.Millisecond)

	logger.Info("Example finished.")
}
```

### Application-Level Row-Level Security

Leverage `CollectionDecorators` to implement cross-cutting concerns like row-level security directly within your application logic.

```go
// From example/complex/main.go
package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

//go:embed schemas/*.json
var schemasFS embed.FS

const (
	userCtxKey = "userID" // Key for user ID in context
)

// SecurityDecorator enforces document-level security based on ownerId.
func SecurityDecorator(logger *zap.Logger) utils.CollectionDecorator {
	return func(next base.Collection) base.Collection {
		return &securityDecorator{
			next:   next,
			logger: logger,
		}
	}
}

type securityDecorator struct {
	next   base.Collection
	logger *zap.Logger
}

var _ base.Collection = (*securityDecorator)(nil)

// getUserIDFromContext extracts the user ID from the context.
func (d *securityDecorator) getUserIDFromContext(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(userCtxKey).(string)
	if !ok || userID == "" {
		return "", fmt.Errorf("unauthorized: user ID not found in context")
	}
	return userID, nil
}

// Read method for security decorator.
func (d *securityDecorator) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	userID, err := d.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err // Unauthorized
	}

	// Add a filter to ensure only documents owned by the user are returned
	ownerFilter := query.NewQueryBuilder().Where("ownerId").Eq(userID).Build().Filters

	// Combine with existing filters
	if q.Filters == nil {
		q.Filters = ownerFilter
	} else {
		// Use AND to combine the original filter with the ownerId filter
		q.Filters = query.NewQueryBuilder().AndFilter(*q.Filters).AndFilter(*ownerFilter).Build().Filters
	}

	return d.next.Read(ctx, q)
}

// (Other CRUD methods omitted for brevity, see example/complex/main.go for full implementation)

func (d *securityDecorator) CreateOne(ctx context.Context, doc data.Document) (base.CreateResult, error) {
	if ownerID, ok := doc["ownerId"].(string); ok && ownerID != "" {
		userID, err := d.getUserIDFromContext(ctx)
		if err != nil {
			return base.CreateResult{}, err
		}
		if userID != ownerID {
			return base.CreateResult{}, fmt.Errorf("forbidden: cannot create document with ownerId %s as user %s", ownerID, userID)
		}
	}
	return d.next.CreateOne(ctx, doc)
}

// (Other base.Collection interface methods would delegate to d.next)
func (d *securityDecorator) CreateMany(ctx context.Context, docs []data.Document) ([]base.CreateResult, error) { /* ... */ return d.next.CreateMany(ctx, docs) }
func (d *securityDecorator) Update(ctx context.Context, update *base.CollectionUpdate) (int, error) { /* ... */ return d.next.Update(ctx, update) }
func (d *securityDecorator) Delete(ctx context.Context, qf *query.QueryFilter, unsafe bool) (int, error) { /* ... */ return d.next.Delete(ctx, qf, unsafe) }
func (d *securityDecorator) Validate(ctx context.Context, data data.Document, loose bool) (*schema.ValidationResult, error) { return d.next.Validate(ctx, data, loose) }
func (d *securityDecorator) Metadata(ctx context.Context, filter *base.MetadataFilter, forceRefresh bool) (*base.CollectionMetadata, error) { return d.next.Metadata(ctx, filter, forceRefresh) }
func (d *securityDecorator) Subscribe(ctx context.Context, options base.SubscriptionOptions) string { return d.next.Subscribe(ctx, options) }
func (d *securityDecorator) Unsubscribe(ctx context.Context, id string) { d.next.Unsubscribe(ctx, id) }
func (d *securityDecorator) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) { return d.next.Subscriptions(ctx) }
func (d *securityDecorator) Capabilities(ctx context.Context) *query.Capabilities { return d.next.Capabilities(ctx) }


// authenticateMiddleware is a simple middleware to simulate authentication
func authenticateMiddleware(next http.Handler, logger *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			logger.Warn("Unauthorized access attempt: X-User-ID header missing")
			http.Error(w, "Unauthorized: X-User-ID header required", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// writeJSONResponse writes a JSON response to the http.ResponseWriter
func writeJSONResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error writing JSON response: %v", err)
	}
}

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	db, _ := sql.Open("sqlite3", "file::memory:?cache=shared") // In-memory DB for example
	defer db.Close()

	executor, _ := sqliteExecutor.NewSQLiteExecutor(db, logger)
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, _ := native.NewNativeInteractor(executor, queryFactory, logger)

	factoryConfig := data.DocumentFactoryConfig{}

	// Apply the SecurityDecorator
	decorators := &utils.Decorators{
		CollectionDecorators: []utils.DecoratorFunc[base.Collection]{
			(utils.DecoratorFunc[base.Collection])(SecurityDecorator(logger)),
		},
	}

	userSchemaBytes, _ := fs.ReadFile(schemasFS, "schemas/user.json")
	var userSchemaDef schema.SchemaDefinition
	userSchemaDef.From(userSchemaBytes)

	documentSchemaBytes, _ := fs.ReadFile(schemasFS, "schemas/document.json")
	var documentSchemaDef schema.SchemaDefinition
	documentSchemaDef.From(documentSchemaBytes)

	cfg := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		FactoryConfig: factoryConfig,
		Decorators:    decorators, // Decorators applied here
		Schemas: []schema.SchemaDefinition{
			userSchemaDef,
			documentSchemaDef,
		},
	}
	p, err := anansi.Setup(cfg)
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	usersCollection, _ := p.Collection(ctx, userSchemaDef.Name)
	documentsCollection, _ := p.Collection(ctx, documentSchemaDef.Name)

	// Populate some initial data without authentication (system context for setup)
	// User1
	user1 := data.MustNewDocument(map[string]any{"id": "user1", "username": "alice", "passwordHash": "...", "role": "user"})
	usersCollection.CreateOne(ctx, user1)
	// User2
	user2 := data.MustNewDocument(map[string]any{"id": "user2", "username": "bob", "passwordHash": "...", "role": "user"})
	usersCollection.CreateOne(ctx, user2)

	// Documents owned by User1
	doc1 := data.MustNewDocument(map[string]any{"id": "doc1", "ownerId": "user1", "content": "Alice's secret", "accessLevel": "private"})
	documentsCollection.CreateOne(ctx, doc1)
	doc2 := data.MustNewDocument(map[string]any{"id": "doc2", "ownerId": "user1", "content": "Alice's public note", "accessLevel": "public"})
	documentsCollection.CreateOne(ctx, doc2)
	// Document owned by User2
	doc3 := data.MustNewDocument(map[string]any{"id": "doc3", "ownerId": "user2", "content": "Bob's report", "accessLevel": "confidential"})
	documentsCollection.CreateOne(ctx, doc3)


	mux := http.NewServeMux()

	// Handler to read documents - will be filtered by the decorator
	mux.HandleFunc("/documents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			q := query.NewQueryBuilder().Build() // User doesn't need to add ownerId filter
			result, err := documentsCollection.Read(r.Context(), &q)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSONResponse(w, http.StatusOK, result.Data)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Apply authentication middleware to protect API routes
	authenticatedMux := authenticateMiddleware(mux, logger)

	port := ":8080"
	logger.Info(fmt.Sprintf("Server starting on port %s", port))
	// Example usage:
	// To access as user1: `curl -H "X-User-ID: user1" http://localhost:8080/documents` (will only see doc1, doc2)
	// To access as user2: `curl -H "X-User-ID: user2" http://localhost:8080/documents` (will only see doc3)
	log.Fatal(http.ListenAndServe(port, authenticatedMux))
}
```

### Complex Queries & Joins

The `QueryBuilder` and DSL enable powerful, database-agnostic queries, including joins.

```go
// From example/advanced/main.go
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
	"github.com/asaidimu/go-anansi/v6/core/common"
)

// Define schemas for User, Account, and LedgerTransaction (renamed from Transaction to avoid Go keyword conflict)
func getUserSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:    "User",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":    {Name: "id", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
			"name":  {Name: "name", Type: "string", Required: coreutils.BoolPtr(true)},
			"email": {Name: "email", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
		},
	}
}

func getAccountSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:    "Account",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"id":      {Name: "id", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
			"userId":  {Name: "userId", Type: "string", Required: coreutils.BoolPtr(true)}, // Foreign key to User
			"balance": {Name: "balance", Type: "number", Required: coreutils.BoolPtr(true)},
		},
	}
}

func getLedgerTransactionSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:    "LedgerTransaction",
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

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	db, _ := sql.Open("sqlite3", "file::memory:?cache=shared")
	defer db.Close()

	executor, _ := sqliteExecutor.NewSQLiteExecutor(db, logger)
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, _ := native.NewNativeInteractor(executor, queryFactory, logger)

	factoryConfig := data.DocumentFactoryConfig{}
	decorators := &utils.Decorators{} // No custom decorators for this example
	
	cfg := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		FactoryConfig: factoryConfig,
		Decorators:    decorators,
	}
	p, err := anansi.Setup(cfg)
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create Collections
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
		Condition: &query.FilterCondition{
			Field:    "LedgerTransaction.accountId",
			Operator: query.ComparisonOperatorEq,
			Value: query.FilterValue{
				FieldRefVal: &query.FieldReference{
					Type:  "field",
					Field: "Account.id",
				},
			},
		},
	}).End().
		LeftJoin("User").On(query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "Account.userId",
			Operator: query.ComparisonOperatorEq,
			Value: query.FilterValue{
				FieldRefVal: &query.FieldReference{
					Type:  "field",
					Field: "User.id",
				},
			},
		},
	}).End().
		Where("User.id").Eq("U001").
		Build()

	txResult, err := transactionsCollection.Read(ctx, &aliceTransactionsQuery)
	if err != nil {
		log.Fatalf("Failed to query transactions for Alice: %v", err)
	}

	logger.Info(fmt.Sprintf("Found %d transactions for Alice:", txResult.Count))
	var aliceDocs []data.Document
	if txResult.Count == 1 {
		aliceDocs = []data.Document{txResult.Data.(data.Document)}
	} else if txResult.Count > 1 {
		aliceDocs = txResult.Data.([]data.Document)
	}

	for _, doc := range aliceDocs {
		ledgerTx := doc["LedgerTransaction"].(map[string]any)
		user := doc["User"].(map[string]any)
		logger.Info(fmt.Sprintf("  Transaction ID: %s, Amount: %.2f, Type: %s, Account ID: %s, User Name: %s",
			ledgerTx["id"], ledgerTx["amount"], ledgerTx["type"], ledgerTx["accountId"], user["name"]))
	}

	logger.Info("Advanced example finished.")
}
```

### API Usage Example

The `example/api` directory provides a comprehensive example of building a RESTful API using `go-anansi` as the persistence layer. You can find detailed API specifications in [`example/api/spec.md`](./example/api/spec.md).

Here's a snippet showing how API endpoints interact with the `go-anansi` persistence layer:

```go
// From example/api/internal/api/server.go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"go.uber.org/zap"

	"github.com/asaidimu/go-anansi/v6/example/api/internal/app"
	"github.com/asaidimu/go-anansi/v6/example/api/internal/response"
)

// APIServer encapsulates the HTTP server and its dependencies.
type APIServer struct { /* ... */ }

// handleCollectionDocuments handles operations on /api/v1/collections/{collection}/documents
func (s *APIServer) handleCollectionDocuments(w http.ResponseWriter, r *http.Request) {
	collectionName := r.PathValue("collection")
	// ... error handling for missing collectionName ...

	collection, err := s.Persistence.Collection(r.Context(), collectionName)
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "COLLECTION_ERROR",
			Message: fmt.Sprintf("Failed to get collection %s: %v", collectionName, err),
		}, r)
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.createDocuments(w, r, collection)
	case http.MethodGet:
		s.readDocuments(w, r, collection)
	default:
		s.Response.WriteError(w, http.StatusMethodNotAllowed, response.APIError{
			Code:    "METHOD_NOT_ALLOWED",
			Message: "Method not allowed",
		}, r)
	}
}

// createDocuments handles POST /api/v1/collections/{collection}/documents
func (s *APIServer) createDocuments(w http.ResponseWriter, r *http.Request, collection base.Collection) {
	var reqBody struct {
		Documents []data.Document `json:"documents"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		s.Response.WriteError(w, http.StatusBadRequest, response.APIError{
			Code:    "BAD_REQUEST",
			Message: "Invalid request body",
			Details: err.Error(),
		}, r)
		return
	}

	results, err := collection.CreateMany(r.Context(), reqBody.Documents) // Delegate to Anansi's CreateMany
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, err, r)
		return
	}

	createdDocs := make([]data.Document, len(results))
	for i, res := range results {
		createdDocs[i] = res.Data
	}

	s.Response.WriteJSON(w, http.StatusCreated, map[string]any{
		"documents": createdDocs,
	}, r)
}

// readDocuments handles GET /api/v1/collections/{collection}/documents
func (s *APIServer) readDocuments(w http.ResponseWriter, r *http.Request, collection base.Collection) {
	// ... (Query parameter parsing for filter, sort, limit, offset, fields would go here)
	q := query.NewQueryBuilder().Build() // Simple query for all documents for now
	result, err := collection.Read(r.Context(), &q) // Delegate to Anansi's Read
	if err != nil {
		s.Response.WriteError(w, http.StatusInternalServerError, response.APIError{
			Code:    "READ_ERROR",
			Message: "Failed to read documents",
			Details: err.Error(),
		}, r)
		return
	}
	// ... (Handle result.Data being single document or slice)

	s.Response.WriteJSON(w, http.StatusOK, map[string]any{
		"documents": result.Data,
	}, r)
}

// Full API specification: [`example/api/spec.md`](./example/api/spec.md)
```

---

## 🏗️ Project Architecture

`go-anansi` is designed with a layered, pluggable architecture, promoting separation of concerns and extensibility.

*   **`anansi` Package**: The top-level entry point (`anansi.go`) providing simplified `Playground` for development and highly configurable `Setup` for production environments. It orchestrates the initialization of the entire persistence stack.
*   **`core/data`**: Manages the core `Document` data type, which is the primary unit of data interaction. It handles flexible, schema-aware data structures, JSON marshaling/unmarshaling, path-based access, and data manipulation utilities (diffing, merging, cloning).
*   **`core/schema`**: Defines the declarative schema language, including `SchemaDefinition`, `FieldDefinition`, `IndexDefinition`, `Migration`, and `ValidationNode`. It provides a robust `DocumentValidator` capable of building and traversing complex validation graphs for incoming data. It also includes code generation utilities (`codegen`) to produce Go structs from schemas.
*   **`core/query`**: Encapsulates the Domain-Specific Language (DSL) for querying, represented by structs (`dsl.go`) and a fluent `QueryBuilder`. The `QueryEngine` acts as the central query processing unit, utilizing a `QueryPartitioner` to optimize query execution across database and in-memory layers based on `DatabaseInteractor` capabilities.
*   **`core/persistence`**: The high-level, user-facing API for interacting with the data store. This package defines the `Persistence` and `Collection` interfaces, manages `Collection Registry` for schema versioning, handles transactional operations (`transaction`), and orchestrates an `EventEmitter` for persistence-related events.
*   **`core/events`**: Provides a generic event bus interface (`events.EventBus`) and an `EventEmitter` for pub/sub messaging. It integrates with external messaging systems like Watermill (`utils/events.go`).
*   **`core/ephemeral`**: An in-memory implementation of the `DatabaseInteractor` and `SchemaManager`, ideal for rapid prototyping, unit testing, and scenarios not requiring persistent storage.
*   **`sqlite`**: A concrete, production-ready implementation of the `core/query.DatabaseInteractor` interface for SQLite databases. It includes an `executor` for interacting with `database/sql` and a `query/builder` for translating the generic `core/query` DSL into native SQLite SQL.
*   **`core/common`**: Defines common types and error structures (`SystemError`, `Issue`), providing a consistent and extensible error handling mechanism across the entire framework.
*   **`core/utils`**: A collection of generic helper functions for type coercion, JSON manipulation, map operations, and context management.

### Data Flow for `collection.Read()`

The `collection.Read()` operation, a cornerstone of data retrieval, follows a sophisticated, multi-stage pipeline:

1.  **Request Initiation**: A `collection.Read(ctx, query)` call is made with a `Query` object (defined by the DSL).
2.  **Interactor Resolution**: The `collection` first resolves the correct `query.DatabaseInteractor` for the current context (e.g., a transactional interactor if within an active transaction).
3.  **Query Engine Dispatch**: The `Query` object is passed to the `core/query.QueryEngine`.
4.  **Query Partitioning**: The `QueryEngine` utilizes a `QueryPartitioner`. This component intelligently analyzes the incoming `Query` and compares its operations (filters, sorts, aggregations, etc.) against the `Capabilities` declared by the currently configured `DatabaseInteractor` (e.g., SQLite). It then splits the `Query` into two parts:
    *   **Database Query**: Operations that the `DatabaseInteractor` can efficiently execute natively (e.g., indexed filtering, simple sorting).
    *   **Residual Query**: Operations that are either not supported by the database interactor or are more efficiently handled in-memory by Go code (e.g., custom Go functions, complex nested projections).
5.  **Database Execution**: The `QueryEngine` sends the **Database Query** to the `DatabaseInteractor`. For SQLite, the `sqlite/query/builder` translates this into optimized SQL, which is then executed by `sqlite/executor` against the database. An initial, partially filtered dataset of `data.Document`s is returned.
6.  **In-Memory Refinement**: The `QueryEngine` then takes this initial result set and employs a `QueryHelper` to apply all **Residual Query** operations in-memory (e.g., additional filtering, joining, aggregation, sorting, and pagination).
7.  **Result Return**: The final, fully processed and shaped result set of `data.Document`s is returned to the caller.

This two-phase approach (`database-first filtering` followed by `in-memory refinement`) ensures optimal performance and flexibility, leveraging database capabilities where strong, and falling back to robust Go-based processing for complex or unsupported operations.

### Extension Points

`go-anansi` is built with extensibility as a core principle:

*   **`query.DatabaseInteractor`**: The most significant extension point. Developers can implement this interface to integrate `go-anansi` with any SQL or NoSQL database (e.g., PostgreSQL, MongoDB).
*   **`events.EventBus`**: The `EventBus` interface allows plugging in different message brokers (e.g., Kafka, RabbitMQ) by providing a custom implementation. The current `WatermillEventBus` is an example.
*   **`utils.CollectionDecorator` / `utils.PersistenceDecorator`**: Decorators provide a powerful way to inject cross-cutting concerns (e.g., logging, caching, auditing, row-level security, encryption) without modifying the core logic of collections or the persistence layer.
*   **`core/query.QueryEngine` Custom Functions**: The query engine can be extended with custom `ComputeFunction` and `FilterFunction` implementations to enable application-specific logic directly within queries.
*   **`core/persistence/migration.DataMigrator`**: For complex data migrations, custom `Transformer` functions can be provided to transform documents between schema versions.
*   **`query.IndexManager`**: As part of the proposed full-text search, this interface will allow plugging in different search engines (e.g., Bleve, Elasticsearch) for search indexing and querying.

---

## 👨‍💻 Development & Contributing

We welcome contributions to `go-anansi`!

### Development Setup

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/asaidimu/go-anansi.git
    cd go-anansi
    ```
2.  **Ensure Go modules are tidy:**
    ```bash
    go mod tidy
    ```
3.  **Install dependencies:**
    This should be handled by `go mod tidy` and subsequent `go build` or `go test` commands.

### Scripts

The project uses a `Makefile` for common development tasks:

*   **`make build`**: Compiles all Go packages in the project.
*   **`make test`**: Runs all unit and integration tests, clearing the test cache first.

Additionally, the `bin/bump.sh` script is available for automating Go module major version updates, handling changes in `go.mod` and import paths. Use it with `--dry-run` first!

### Testing

To run the comprehensive test suite:

```bash
make test
```

This will execute unit, integration, and end-to-end tests across the codebase. We strive for high test coverage and detailed test cases, as tracked internally in `test-gap.md`.

### Contributing Guidelines

We appreciate your contributions! Please follow these general guidelines:

1.  **Fork the repository** and create your branch from `main`.
2.  **Ensure your code adheres to Go best practices** and `go fmt`.
3.  **Write clear, concise commit messages** following conventional commits.
4.  **Add unit and integration tests** for new features or bug fixes.
5.  **Update documentation** (including the `README.md`) as appropriate.
6.  **Submit a Pull Request** against the `main` branch.

### Issue Reporting

Encounter a bug or have a feature request? Please report it on our [GitHub Issues](https://github.com/asaidimu/go-anansi/issues) page.

---

## 📚 Additional Information

### Troubleshooting

For common issues and system error codes, refer to the internal [`system-errors.md`](./system-errors.md) document which describes standard error codes like `ERR_SCHEMA_VALIDATOR_SCHEMA_VALIDATION_FAILED`, `ERR_SCHEMA_VALIDATOR_CIRCULAR_DEPENDENCY`, and `ERR_SCHEMA_VALIDATOR_CREATION_FAILED`.

### FAQ

*   **How does `go-anansi` compare to traditional ORMs?**
    `go-anansi` is less of a traditional ORM and more of a flexible persistence framework. It doesn't aim to map every database table to a Go struct 1:1. Instead, it uses a schema-driven `Document` model, offering more flexibility for evolving data schemas and supporting diverse storage backends with a unified query language, focusing on patterns like event-sourcing, CQRS, and microservices data layers.

*   **Can I use `go-anansi` with a database other than SQLite?**
    Yes! The core `query.DatabaseInteractor` interface is designed for pluggability. While SQLite is currently the primary concrete implementation, you can implement this interface for other databases (e.g., PostgreSQL, MySQL, MongoDB).

*   **How does schema evolution work?**
    `go-anansi` supports declarative schema definitions and `Migration` objects. It provides mechanisms to track schema versions and apply changes, often involving data transformation functions to migrate data from an old schema version to a new one.

### Changelog / Roadmap

For detailed changes between versions, please refer to the project's [release history on GitHub](https://github.com/asaidimu/go-anansi/releases).
Our roadmap includes the exciting [upcoming features](#-upcoming-features-proposals) mentioned above, focusing on enhanced reliability, developer experience, and powerful search capabilities.

### License

`go-anansi` is licensed under the **Anansi Platform Proprietary License**.

**IMPORTANT**: This is a source-available license. It is **NOT** an open-source license as defined by the Open Source Initiative. A separate commercial license is required for any form of commercial use. For details, please see the full [LICENSE.md](./LICENSE.md) file.

### Acknowledgments

`go-anansi` is a product of [CyberSync Printers & Stationers](https://github.com/asaidimu).

---