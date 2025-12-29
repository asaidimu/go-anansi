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
	coreutils "github.com/asaidimu/go-anansi/v6/core/utils" // For BoolPtr
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
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
	// 1. Setup Logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync() // Flush any buffered log entries

	// 2. Setup In-Memory SQLite Database
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// 3. Create Database Interactor for SQLite
	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil {
		log.Fatalf("Failed to create SQLite interactor: %v", err)
	}
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil {
		log.Fatalf("Failed to create native interactor: %v", err)
	}

	// 4. Setup Document Factory Config
	factoryConfig := data.DocumentFactoryConfig{}

	// 5. Setup Decorators (none for basic example)
	decorators := &utils.Decorators{}
	bus := NewWatermillEventBus[base.PersistenceEvent](logger)
	defer bus.Close()

	// 6. Initialize Anansi Persistence Layer
	cfg := anansi.SetupConfig{
		Interactor:    interactor,
		Logger:        logger,
		FactoryConfig: factoryConfig,
		Decorators:    decorators,
		EventBus:     bus,
	}
	p, err := anansi.Setup(cfg)
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 7. Create "products" collection
	productSchema := getProductSchema()
	productsCollection, err := p.CreateCollection(ctx, *productSchema)
	if err != nil {
		log.Fatalf("Failed to create products collection: %v", err)
	}
	logger.Info("Products collection created.")

	unsub := productsCollection.Subscribe(context.Background(), base.SubscriptionOptions{
		Event: base.DocumentCreateSuccess,
		Callback: func(ctx context.Context, event base.PersistenceEvent) error {
			logger.Info(fmt.Sprintf("Product created in '%s' \n %v", *event.Collection, event.Input))
			return nil
		},
	})
	defer productsCollection.Unsubscribe(context.Background(), unsub)
	// 8. CRUD Operations

	// Create Products
	logger.Info("Creating products...")
	product1 := data.MustNewDocument(map[string]any{"id": "P001", "name": "Laptop", "price": 1200.00, "stock": 50})
	product2 := data.MustNewDocument(map[string]any{"id": "P002", "name": "Mouse", "price": 25.00, "stock": 200})

	_, err = productsCollection.CreateOne(ctx, product1)
	if err != nil {
		log.Fatalf("Failed to create product P001: %v", err)
	}
	logger.Info("Created product P001: Laptop")


	_, err = productsCollection.CreateOne(ctx, product2)
	if err != nil {
		log.Fatalf("Failed to create product P002: %v", err)
	}

	// Read Products
	logger.Info("Reading all products...")
	allProductsQuery := query.NewQueryBuilder().Build()
	readResult, err := productsCollection.Read(ctx, &allProductsQuery)
	if err != nil {
		log.Fatalf("Failed to read all products: %v", err)
	}

	if readResult.Count > 0 {
		for _, doc := range readResult.Data {
			logger.Info(fmt.Sprintf("Found product: ID=%s, Name=%s, Price=%.2f, Stock=%d",
				doc["id"], doc["name"], doc["price"], doc["stock"]))
		}
	} else {
		logger.Info("No products found.")
	}

	// Update Product (P001 stock)
	logger.Info("Updating product P001 stock...")
	updateProduct1 := data.MustNewDocument(map[string]any{"id": "P001", "stock": 45})
	filterP001 := query.NewQueryBuilder().Where("id").Eq("P001").Build().Filters
	_, err = productsCollection.Update(ctx, &base.CollectionUpdate{Filter: filterP001, Set: updateProduct1})
	if err != nil {
		log.Fatalf("Failed to update product P001: %v", err)
	}
	logger.Info("Updated product P001 stock to 45.")

	// Read updated product
	logger.Info("Reading updated product P001...")
	readP001Query := query.NewQueryBuilder().Where("id").Eq("P001").Build()
	readP001Result, err := productsCollection.Read(ctx, &readP001Query)
	if err != nil {
		log.Fatalf("Failed to read product P001 after update: %v", err)
	}
	if readP001Result.Count > 0 {
		updatedProduct := readP001Result.Data[0]
		logger.Info(fmt.Sprintf("Updated product P001: ID=%s, Name=%s, Price=%.2f, Stock=%d",
			updatedProduct["id"], updatedProduct["name"], updatedProduct["price"], updatedProduct["stock"]))
	}

	// Delete Product (P002)
	logger.Info("Deleting product P002...")
	filterP002 := query.NewQueryBuilder().Where("id").Eq("P002").Build().Filters
	_, err = productsCollection.Delete(ctx, filterP002, false) // false for not deleting physical data
	if err != nil {
		log.Fatalf("Failed to delete product P002: %v", err)
	}
	logger.Info("Deleted product P002.")

	// Verify deletion
	logger.Info("Verifying product P002 deletion...")
	readP002Query := query.NewQueryBuilder().Where("id").Eq("P002").Build()
	readP002Result, err := productsCollection.Read(ctx, &readP002Query)
	if err != nil {
		log.Fatalf("Failed to read product P002 after deletion: %v", err)
	}
	if readP002Result.Count == 0 {
		logger.Info("Product P002 successfully deleted (not found).")
	} else {
		logger.Warn("Product P002 still found after deletion.")
	}

	logger.Info("Basic example finished.")
}
