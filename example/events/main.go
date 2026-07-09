package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v8"
	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/query/native"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	sqliteExecutor "github.com/asaidimu/go-anansi/v8/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v8/sqlite/query"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"go.uber.org/zap"
)

// Product schema definition
func getProductSchema() *definition.Schema {
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "Product",
			Fields: map[definition.FieldId]definition.Field{
				"name":  {Name: "name", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"value": {Name: "value", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"price": {Name: "price", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"stock": {Name: "stock", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
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
	queryFactory := sqliteQuery.NewSQLiteFactory(nil)
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil {
		log.Fatalf("Failed to create native interactor: %v", err)
	}

	// 4. Configuration for the Document layer
	factoryConfig := data.DocumentFactoryConfig{
		GlobalSanitizer: &data.FieldMaskConfig{
			Fields: map[string]data.MaskedFieldPolicy{
				"value":    data.MaskObscure,
				"id":       data.MaskObscure,
				"checksum": data.MaskRedact,
			},
		},
	}

	// 5. Setup Decorators (none for basic example)
	decorators := &utils.Decorators{}
	bus := NewWatermillEventBus[base.PersistenceEvent](logger)
	defer bus.Close()

	// 6. Initialize Anansi Persistence Layer
	cfg := anansi.SetupConfig{
		Interactor:            interactor,
		Logger:                logger,
		DocumentFactoryConfig: factoryConfig,
		Decorators:            decorators,
		EventBus:              bus,
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
	productsCollection, err := p.CreateCollection(ctx, productSchema)
	if err != nil {
		log.Fatalf("Failed to create products collection: %v", err)
	}
	logger.Info("Products collection created.")

	unsub := productsCollection.Subscribe(context.Background(), base.SubscriptionOptions{
		Event: base.DocumentCreateSuccess,
		Callback: func(ctx context.Context, event base.PersistenceEvent) error {
			input, ok := data.DocumentFrom(event.Input)
			if ok {
				pretty, _ := input.ToJSON(true)
				logger.Info(fmt.Sprintf("Product created in '%s' \n %s", *event.Collection, pretty))
			}
			return nil
		},
	})
	defer productsCollection.Unsubscribe(context.Background(), unsub)
	// 8. CRUD Operations

	// Create Products
	logger.Info("Creating products...")
	laptop := data.MustNewDocument(map[string]any{"value":100.00, "name": "Laptop", "price": 1200.00, "stock": 50})
	mouse := data.MustNewDocument(map[string]any{"value":2.00, "name": "Mouse", "price": 25.00, "stock": 200})

	_, err = productsCollection.CreateOne(ctx, laptop)
	if err != nil {
		log.Fatalf("Failed to add Laptop: %v", err)
	}
	logger.Info(fmt.Sprintf("Created product Laptop with id %s\n", laptop.ID()))

	_, err = productsCollection.CreateOne(ctx, mouse)
	if err != nil {
		if e, ok := err.(*common.SystemError); ok {
			log.Fatalf("Failed to add mouse: %v", e.ToIssue())
		}
		log.Fatalf("Failed to add mouse: %v", err)
	}
	logger.Info(fmt.Sprintf("Created product Laptop with id %s\n", mouse.ID()))

	// Read Products
	logger.Info("Reading all products...")
	allProductsQuery := query.NewQueryBuilder().Build()
	readResult, err := productsCollection.Read(ctx, &allProductsQuery)
	if err != nil {
		log.Fatalf("Failed to read all products: %v", err)
	}

	// Apply collection-specific sanitization context
	sanitizationCtx := common.ContextWithCollectionName(ctx, productSchema.Name)

	if readResult.Count > 0 {
		for _, found := range readResult.Data {
			doc, err := found.Sanitize(sanitizationCtx)
			if err != nil {
				logger.Error("Failed to sanitize document", zap.Error(err))
				continue
			}
			logger.Info(fmt.Sprintf("Found product: ID=%s, Name=%s, Price=%.2f, Stock=%d, Value=%s",
				doc.Must().Get("id"),
				doc.Must().Get("name"),
				doc.Must().Get("price"),
				doc.Must().Get("stock"),
				doc.Must().Get("value")))
		}
	} else {
		logger.Info("No products found.")
	}

	// Update Product (Laptop stock)
	logger.Info("Updating Laptop stock...")
	updateProduct1 := data.MustNewDocument(map[string]any{"stock": 45})
	filterP001 := query.NewQueryBuilder().Where("id").Eq(laptop.ID()).Build().Filters
	_, err = productsCollection.Update(ctx, &base.CollectionUpdate{Filter: filterP001, Set: updateProduct1})
	if err != nil {
		log.Fatalf("Failed to update Laptop: %v", err)
	}
	logger.Info("Updated product Laptop stock to 45.")

	// Read updated product
	logger.Info("Reading updated Laptop...")
	readP001Query := query.NewQueryBuilder().Where("id").Eq(laptop.ID()).Build()
	readP001Result, err := productsCollection.Read(ctx, &readP001Query)
	if err != nil {
		log.Fatalf("Failed to read Laptop after update: %v", err)
	}
	if readP001Result.Count > 0 {
		updatedProduct := readP001Result.Data[0]
		sanitizedProduct, err := updatedProduct.Sanitize(sanitizationCtx)
		if err != nil {
			logger.Error("Failed to sanitize updated product", zap.Error(err))
			sanitizedProduct = updatedProduct
		}
		updatedProduct = sanitizedProduct
		logger.Info(fmt.Sprintf("Updated Laptop: ID=%s, Name=%s, Price=%.2f, Stock=%d, Value=%s",
			updatedProduct.ID(),
			updatedProduct.Must().Get("name"),
			updatedProduct.Must().Get("price"),
			updatedProduct.Must().Get("stock"),
			updatedProduct.Must().Get("value")))
	}

	// Delete Product (P002)
	logger.Info("Deleting product mouse...")
	filterP002 := query.NewQueryBuilder().Where("id").Eq(mouse.ID()).Build().Filters
	_, err = productsCollection.Delete(ctx, filterP002, false) // false for not deleting physical data
	if err != nil {
		log.Fatalf("Failed to delete mouse: %v", err)
	}
	logger.Info("Deleted mouse.")

	// Verify deletion
	logger.Info("Verifying product mouse deletion...")
	readP002Query := query.NewQueryBuilder().Where("id").Eq(mouse.ID()).Build()
	readP002Result, err := productsCollection.Read(ctx, &readP002Query)
	if err != nil {
		log.Fatalf("Failed to read mouse after deletion: %v", err)
	}
	if readP002Result.Count == 0 {
		logger.Info("Mouse successfully deleted (not found).")
	} else {
		logger.Warn("Mouse still found after deletion.")
	}

	logger.Info("Basic example finished.")
}
