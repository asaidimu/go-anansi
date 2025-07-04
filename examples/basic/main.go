package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/sqlite"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"go.uber.org/zap"               // For logging, as recommended by Anansi docs
	"go.uber.org/zap/zapcore"
)

// Define the schema for inventory items as a JSON string
const inventorySchemaJSON = `{
  "name": "inventory_items",
  "version": "1.0.0",
  "description": "Schema for tracking inventory items",
  "fields": {
    "id_field_name_can_be_uuid": { "name": "id", "type": "string", "required": true, "unique": true },
    "item_name": { "name": "item_name", "type": "string", "required": true, "unique": true },
    "description": { "name": "description", "type": "string", "required": false },
    "quantity": { "name": "quantity", "type": "integer", "required": true },
    "last_updated": { "name": "last_updated", "type": "datetime", "required": true }
  },
  "indexes": [
    { "fields": ["item_name"], "unique": true },
    { "fields": ["quantity"] }
  ]
}`

func main() {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)       // Set minimum logging level to Info
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // Human-readable timestamps
	config.EncoderConfig.TimeKey = "timestamp"                   // Key for the timestamp field

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

	// 2. Initialize SQLite Interactor
	// Setting DropIfExists to true for easy testing. In a real application, manage migrations carefully.
	interactorOptions := sqlite.DefaultInteractorOptions()
	interactor := sqlite.NewSQLiteInteractor(db, logger, interactorOptions, nil)

	// 3. Initialize the Anansi Persistence service
	persistenceSvc, err := persistence.NewPersistence(interactor, schema.FunctionMap{})
	if err != nil {
		logger.Fatal("Failed to initialize persistence service", zap.Error(err))
	}
	logger.Info("Anansi persistence service initialized for inventory tracking.")

	// --- Create Collection (Table) for Inventory Items ---
	// Use a context for database operations
	var inventorySchema schema.SchemaDefinition
	if err := json.Unmarshal([]byte(inventorySchemaJSON), &inventorySchema); err != nil {
		logger.Fatal("Failed to unmarshal inventory schema", zap.Error(err))
	}
	var inventoryCollection persistence.PersistenceCollectionInterface
	if inventoryCollection, err = persistenceSvc.Collection(inventorySchema.Name); err != nil {
		inventoryCollection, err = persistenceSvc.Create(inventorySchema)
		if err != nil {
			logger.Fatal("Failed to create 'inventory_items' collection", zap.Error(err))
		}
	}

	logger.Info("'inventory_items' collection created successfully.")

	// Helper function to print item details
	printItemDetails := func(logger *zap.Logger, itemDoc schema.Document) {
		logger.Info("Item",
			zap.String("ID", itemDoc["id"].(string)),
			zap.String("Name", itemDoc["item_name"].(string)),
			zap.Int64("Quantity", itemDoc["quantity"].(int64)), // Anansi might return integer as int64
			zap.Any("Last Updated", itemDoc["last_updated"]),   // time.Time or string depending on exact Anansi version/db mapping
		)
	}

	// Helper function to list all items with improved data handling
	listAllItems := func(col persistence.PersistenceCollectionInterface) {
		logger.Info("--- Current Inventory ---")
		readQuery := query.NewQueryBuilder().OrderBy("item_name", query.SortDirectionAsc).Build()
		result, err := col.Read(&readQuery)
		if err != nil {
			logger.Error("Failed to read inventory items", zap.Error(err))
			return
		}

		if result.Count == 0 {
			logger.Info("No items in inventory.")
			return
		} else if result.Count == 1 {
			// If count is 1, expect a single schema.Document
			if itemDoc, ok := result.Data.(schema.Document); ok {
				logger.Info("Found 1 item:")
				printItemDetails(logger, itemDoc)
			} else {
				logger.Error("Expected single document but got unexpected type", zap.Any("data", result.Data))
			}
		} else { // result.Count > 1
			// If count is > 1, expect a slice of schema.Document
			if itemDocs, ok := result.Data.([]schema.Document); ok {
				logger.Info("Found multiple items:")
				for _, itemDoc := range itemDocs {
					printItemDetails(logger, itemDoc)
				}
			} else {
				logger.Error("Expected slice of documents but got unexpected type", zap.Any("data", result.Data))
			}
		}
		logger.Info("-------------------------")
	}

	// --- Inventory Operations ---

	// 1. Add New Items (Create)
	logger.Info("Adding new items to inventory...")
	item1ID := uuid.New().String()
	item1 := map[string]any{
		"id":           item1ID,
		"item_name":    "Laptop",
		"description":  "High-performance notebook",
		"quantity":     int64(10), // Use int64 for integer types in Anansi documents
		"last_updated": time.Now(),
	}
	_, err = inventoryCollection.Create(item1)
	if err != nil {
		logger.Error("Failed to add Laptop", zap.Error(err))
	}

	item2ID := uuid.New().String()
	item2 := map[string]any{
		"id":           item2ID,
		"item_name":    "Mouse",
		"description":  "Wireless ergonomic mouse",
		"quantity":     int64(50),
		"last_updated": time.Now(),
	}
	_, err = inventoryCollection.Create(item2)
	if err != nil {
		logger.Error("Failed to add Mouse", zap.Error(err))
	}

	item3ID := uuid.New().String()
	item3 := map[string]any{
		"id":           item3ID,
		"item_name":    "Keyboard",
		"description":  "Mechanical keyboard",
		"quantity":     int64(25),
		"last_updated": time.Now(),
	}
	_, err = inventoryCollection.Create(item3)
	if err != nil {
		logger.Error("Failed to add Keyboard", zap.Error(err))
	}

	listAllItems(inventoryCollection)

	// 2. Update Item Quantity (Update)
	logger.Info("Updating quantity for 'Laptop'...")
	updateData := map[string]any{
		"quantity":     int64(8), // Quantity reduced
		"last_updated": time.Now(),
	}
	updateFilter := query.NewQueryBuilder().Where("item_name").Eq("Laptop").Build().Filters

	updatedRows, err := inventoryCollection.Update(&persistence.CollectionUpdate{
		Data:   updateData,
		Filter: updateFilter,
	})
	if err != nil {
		logger.Error("Failed to update Laptop quantity", zap.Error(err))
	} else {
		logger.Info("Laptop quantity updated", zap.Int("rows_affected", updatedRows))
	}

	listAllItems(inventoryCollection)

	// 3. Delete an Item (Delete)
	logger.Info("Deleting 'Mouse' from inventory...")
	deleteFilter := query.NewQueryBuilder().Where("item_name").Eq("Mouse").Build().Filters

	deletedRows, err := inventoryCollection.Delete(deleteFilter, false) // false means filter is mandatory
	if err != nil {
		logger.Error("Failed to delete Mouse", zap.Error(err))
	} else {
		logger.Info("Mouse deleted", zap.Int("rows_affected", deletedRows))
	}

	listAllItems(inventoryCollection)

	// 4. Read (Query) items with quantity less than a certain value
	logger.Info("Reading items with quantity less than 20:")
	lowStockQuery := query.NewQueryBuilder().
		Where("quantity").Lt(int64(20)).
		Build()

	lowStockResult, err := inventoryCollection.Read(&lowStockQuery)
	if err != nil {
		logger.Error("Failed to read low stock items", zap.Error(err))
	} else {
		if lowStockResult.Count == 0 {
			logger.Info("No items with quantity less than 20.")
		} else if lowStockResult.Count == 1 {
			if itemDoc, ok := lowStockResult.Data.(schema.Document); ok {
				logger.Info("Found 1 low stock item:")
				printItemDetails(logger, itemDoc)
			} else {
				logger.Error("Expected single low stock document but got unexpected type", zap.Any("data", lowStockResult.Data))
			}
		} else { // lowStockResult.Count > 1
			if itemDocs, ok := lowStockResult.Data.([]schema.Document); ok {
				logger.Info("Found multiple low stock items:")
				for _, itemDoc := range itemDocs {
					printItemDetails(logger, itemDoc)
				}
			} else {
				logger.Error("Expected slice of low stock documents but got unexpected type", zap.Any("data", lowStockResult.Data))
			}
		}
	}
}
