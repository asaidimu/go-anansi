package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v2/core/persistence"
	"github.com/asaidimu/go-anansi/v2/core/query"
	"github.com/asaidimu/go-anansi/v2/core/schema"
	"github.com/asaidimu/go-anansi/v2/sqlite"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"github.com/google/uuid"
	"go.uber.org/zap"         // For logging, as recommended by Anansi docs
	"go.uber.org/zap/zapcore" // Import for logging levels
)

// Define the schema for inventory items as a JSON string
const inventorySchemaJSON = `{
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
}`

// Define the schema for order items as a JSON string
const orderItemSchemaJSON = `{
  "name": "order_items",
  "version": "1.0.0",
  "description": "Schema for individual items within an order",
  "fields": {
    "id": { "name": "id", "type": "string", "required": true, "unique": true },
    "item_id": { "name": "item_id", "type": "string", "required": true },
    "ordered_quantity": { "name": "ordered_quantity", "type": "integer", "required": true },
    "order_id": { "name": "order_id", "type": "string", "required": true },
    "created_at": { "name": "created_at", "type": "datetime", "required": true }
  },
  "indexes": [
    { "fields": ["order_id"] },
    { "fields": ["item_id"] }
  ]
}`

func main() {
	// Initialize logger to display info messages only
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
	db, err := sql.Open("sqlite3", "./inventory_with_triggers.db")
	if err != nil {
		logger.Fatal("Failed to open database", zap.Error(err))
	}
	defer db.Close()

	// 2. Initialize SQLite Interactor
	interactorOptions := sqlite.DefaultInteractorOptions()
	interactorOptions.DropIfExists = true // !!! Use with caution in production !!!
	interactor := sqlite.NewSQLiteInteractor(db, logger, interactorOptions, nil)

	// 3. Initialize the Anansi Persistence service
	persistenceSvc, err := persistence.NewPersistence(interactor, schema.FunctionMap{})
	if err != nil {
		logger.Fatal("Failed to initialize persistence service", zap.Error(err))
	}
	logger.Info("Anansi persistence service initialized for inventory tracking with triggers.")

	// --- Create Collections (Tables) ---
	var inventorySchema schema.SchemaDefinition
	if err := json.Unmarshal([]byte(inventorySchemaJSON), &inventorySchema); err != nil {
		logger.Fatal("Failed to unmarshal inventory schema", zap.Error(err))
	}
	inventoryCollection, err := persistenceSvc.Create(inventorySchema)
	if err != nil {
		logger.Fatal("Failed to create 'inventory_items' collection", zap.Error(err))
	}
	logger.Info("'inventory_items' collection created successfully.")

	var orderItemSchema schema.SchemaDefinition
	if err := json.Unmarshal([]byte(orderItemSchemaJSON), &orderItemSchema); err != nil {
		logger.Fatal("Failed to unmarshal order_item schema", zap.Error(err))
	}
	orderItemCollection, err := persistenceSvc.Create(orderItemSchema)
	if err != nil {
		logger.Fatal("Failed to create 'order_items' collection", zap.Error(err))
	}
	logger.Info("'order_items' collection created successfully.")


	// Helper function to print item details
	printItemDetails := func(logger *zap.Logger, itemDoc schema.Document) {
		logger.Info("Item",
			zap.String("ID", itemDoc["id"].(string)),
			zap.String("Name", itemDoc["item_name"].(string)),
			zap.Int64("Quantity", itemDoc["quantity"].(int64)),
			zap.Any("Last Updated", itemDoc["last_updated"]),
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
			if itemDoc, ok := result.Data.(schema.Document); ok {
				logger.Info("Found 1 item:")
				printItemDetails(logger, itemDoc)
			} else {
				logger.Error("Expected single document but got unexpected type", zap.Any("data", result.Data))
			}
		} else { // result.Count > 1
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

	// --- Implement the "Trigger" using a Subscription ---
	// This subscription will listen for successful creations in the 'order_items' collection
	// and update the 'inventory_items' collection accordingly.
	subscriptionID := orderItemCollection.RegisterSubscription(persistence.RegisterSubscriptionOptions{
		Event: persistence.DocumentCreateSuccess, // Trigger on successful document creation
		Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
			logger.Info("Subscription triggered: DocumentCreateSuccess on 'order_items' collection.")

			// Ensure the event is for a document creation and the collection matches
			if event.Output == nil || event.Collection == nil || *event.Collection != "order_items" {
				logger.Warn("Received unexpected event or collection name in subscription callback",
					zap.Any("event_output", event.Output),
					zap.Any("collection", event.Collection),
				)
				return nil
			}

			// Extract the newly created order item document
			var newOrderItemDoc schema.Document
			if result, ok := event.Output.(*query.QueryResult); ok && result.Count == 1 {
				if doc, ok := result.Data.(schema.Document); ok {
					newOrderItemDoc = doc
				}
			}
			if newOrderItemDoc == nil {
				logger.Error("Failed to extract new order item document from event output.")
				return fmt.Errorf("failed to extract order item document")
			}

			itemID, ok := newOrderItemDoc["item_id"].(string)
			if !ok {
				logger.Error("Order item 'item_id' is missing or not a string", zap.Any("order_item", newOrderItemDoc))
				return fmt.Errorf("missing or invalid item_id in order item")
			}
			orderedQuantity, ok := newOrderItemDoc["ordered_quantity"].(int64)
			if !ok {
				logger.Error("Order item 'ordered_quantity' is missing or not an int64", zap.Any("order_item", newOrderItemDoc))
				return fmt.Errorf("missing or invalid ordered_quantity in order item")
			}

			logger.Info("Processing order item",
				zap.String("item_id", itemID),
				zap.Int64("ordered_quantity", orderedQuantity),
			)

			// Get the inventory item
			inventoryReadQuery := query.NewQueryBuilder().Where("id").Eq(itemID).Build()
			inventoryResult, err := inventoryCollection.Read(&inventoryReadQuery)
			if err != nil {
				logger.Error("Failed to read inventory item for update", zap.String("item_id", itemID), zap.Error(err))
				return err
			}

			if inventoryResult.Count == 0 {
				logger.Error("Inventory item not found for order", zap.String("item_id", itemID))
				return fmt.Errorf("inventory item %s not found", itemID)
			}
			if inventoryResult.Count > 1 {
				// This should ideally not happen if 'id' is unique, but good to check
				logger.Warn("Multiple inventory items found for ID, updating first one", zap.String("item_id", itemID))
			}

			inventoryItemDoc, ok := inventoryResult.Data.(schema.Document)
			if !ok {
				if docs, isSlice := inventoryResult.Data.([]schema.Document); isSlice && len(docs) > 0 {
					inventoryItemDoc = docs[0] // Take the first one if it's a slice
				} else {
					logger.Error("Unexpected type for inventory item document", zap.Any("data", inventoryResult.Data))
					return fmt.Errorf("unexpected inventory item data type")
				}
			}

			currentQuantity, ok := inventoryItemDoc["quantity"].(int64)
			if !ok {
				logger.Error("Inventory item 'quantity' is missing or not an int64", zap.Any("inventory_item", inventoryItemDoc))
				return fmt.Errorf("missing or invalid quantity in inventory item")
			}

			newQuantity := currentQuantity - orderedQuantity
			if newQuantity < 0 {
				logger.Warn("Insufficient stock for item",
					zap.String("item_id", itemID),
					zap.Int64("current_quantity", currentQuantity),
					zap.Int64("ordered_quantity", orderedQuantity),
				)
				// In a real app, you might update order_item status to 'failed' or 'backordered'
				return fmt.Errorf("insufficient stock for item %s", itemID)
			}

			// Update the inventory item
			updateData := map[string]any{
				"quantity":     newQuantity,
				"last_updated": time.Now(),
			}
			updateFilter := query.NewQueryBuilder().Where("id").Eq(itemID).Build().Filters

			updatedRows, err := inventoryCollection.Update(&persistence.CollectionUpdate{
				Data:   updateData,
				Filter: updateFilter,
			})
			if err != nil {
				logger.Error("Failed to update inventory quantity", zap.String("item_id", itemID), zap.Error(err))
				return err
			}
			logger.Info("Inventory updated successfully", zap.String("item_id", itemID), zap.Int("rows_affected", updatedRows), zap.Int64("new_quantity", newQuantity))

			return nil
		},
	})
	logger.Info("Subscription registered for 'order_items' DocumentCreateSuccess events.", zap.String("subscription_id", subscriptionID))

	// --- Initial Inventory Setup ---
	logger.Info("Setting up initial inventory...")
	laptopID := uuid.New().String()
	mouseID := uuid.New().String()
	keyboardID := uuid.New().String()

	_, err = inventoryCollection.Create(map[string]any{
		"id":           laptopID,
		"item_name":    "Laptop",
		"description":  "High-performance notebook",
		"quantity":     int64(10),
		"last_updated": time.Now(),
	})
	if err != nil { logger.Error("Failed to add Laptop", zap.Error(err)) }

	_, err = inventoryCollection.Create(map[string]any{
		"id":           mouseID,
		"item_name":    "Mouse",
		"description":  "Wireless ergonomic mouse",
		"quantity":     int64(50),
		"last_updated": time.Now(),
	})
	if err != nil { logger.Error("Failed to add Mouse", zap.Error(err)) }

	_, err = inventoryCollection.Create(map[string]any{
		"id":           keyboardID,
		"item_name":    "Keyboard",
		"description":  "Mechanical keyboard",
		"quantity":     int64(25),
		"last_updated": time.Now(),
	})
	if err != nil { logger.Error("Failed to add Keyboard", zap.Error(err)) }

	listAllItems(inventoryCollection)

	// --- Demonstrate Triggers in Action ---

	// Scenario 1: Successful Order, Inventory Reduces
	logger.Info("Scenario 1: Creating an order for Laptops (quantity 2)...")
	order1ID := uuid.New().String()
	orderItem1 := map[string]any{
		"id":               uuid.New().String(),
		"item_id":          laptopID, // Reference to Laptop
		"ordered_quantity": int64(2),
		"order_id":         order1ID,
		"created_at":       time.Now(),
	}
	_, err = orderItemCollection.Create(orderItem1)
	if err != nil {
		logger.Error("Failed to create order item 1", zap.Error(err))
	}
	// Give a small moment for the async subscription callback to run if it were truly async.
	// In this synchronous example, it runs immediately.
	time.Sleep(10 * time.Millisecond)
	listAllItems(inventoryCollection) // Show updated inventory

	// Scenario 2: Another Successful Order, Inventory Reduces Further
	logger.Info("Scenario 2: Creating another order for Keyboards (quantity 5)...")
	order2ID := uuid.New().String()
	orderItem2 := map[string]any{
		"id":               uuid.New().String(),
		"item_id":          keyboardID, // Reference to Keyboard
		"ordered_quantity": int64(5),
		"order_id":         order2ID,
		"created_at":       time.Now(),
	}
	_, err = orderItemCollection.Create(orderItem2)
	if err != nil {
		logger.Error("Failed to create order item 2", zap.Error(err))
	}
	time.Sleep(10 * time.Millisecond)
	listAllItems(inventoryCollection) // Show updated inventory


	// Scenario 3: Order that would lead to insufficient stock
	logger.Info("Scenario 3: Attempting to order Laptops (quantity 10) - expecting insufficient stock warning...")
	order3ID := uuid.New().String()
	orderItem3 := map[string]any{
		"id":               uuid.New().String(),
		"item_id":          laptopID, // Reference to Laptop
		"ordered_quantity": int64(10),
		"order_id":         order3ID,
		"created_at":       time.Now(),
	}
	_, err = orderItemCollection.Create(orderItem3)
	if err != nil {
		// This error is from the *subscription callback* returning an error,
		// not directly from the Create operation itself.
		logger.Error("Attempted order resulted in error (expected insufficient stock):", zap.Error(err))
	}
	time.Sleep(10 * time.Millisecond)
	listAllItems(inventoryCollection) // Inventory should not change for Laptop as the transaction was effectively rolled back within the callback if it returned an error.

	// Scenario 4: Delete an item from inventory to see if the system handles it gracefully
	logger.Info("Scenario 4: Deleting a 'Mouse' from inventory.")
	deleteFilter := query.NewQueryBuilder().Where("item_name").Eq("Mouse").Build().Filters
	_, err = inventoryCollection.Delete(deleteFilter, false)
	if err != nil {
		logger.Error("Failed to delete Mouse", zap.Error(err))
	}
	listAllItems(inventoryCollection)

	// Unregister the subscription (good practice for cleanup, especially in longer-running apps)
	orderItemCollection.UnregisterSubscription(subscriptionID)
	logger.Info("Subscription unregistered.")
}

