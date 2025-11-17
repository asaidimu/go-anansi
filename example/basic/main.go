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
	coreutils "github.com/asaidimu/go-anansi/v6/core/utils" // For BoolPtr
	_ "github.com/mattn/go-sqlite3"                         // SQLite driver
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
	// 1. Create logger (optional but recommended for dev)
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// 2. Define schema
	productSchema := getProductSchema()

	// 3. Start Playground with full dev features
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

	// 4. Get collection (auto-created via Schemas above)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	products, err := p.Collection(ctx, productSchema.Name)
	if err != nil {
		log.Fatalf("Failed to get products collection: %v", err)
	}
	logger.Info("Products collection ready.")

	unsub := products.Subscribe(ctx, base.SubscriptionOptions{
		Event: base.DocumentCreateStart,
		Callback: func(ctx context.Context, event base.PersistenceEvent) error {
			logger.Info("Event",
				zap.String("type", string(event.Type)),
				zap.String("collection", *event.Collection),
				zap.Any("input", event.Input),
			)
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

	if _, err = products.CreateOne(ctx, p2); err != nil {
		log.Fatalf("Failed to create Mouse: %v", err)
	}

	// Read
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

	// Update
	update := data.MustNewDocument(map[string]any{"stock": 45})
	filter := query.NewQueryBuilder().Where("id").Eq(p1.ID()).Build().Filters

	if _, err = products.Update(ctx, &base.CollectionUpdate{Filter: filter, Set: update}); err != nil {
		log.Fatalf("Update failed: %v", err)
	}

	// Read updated
	q = query.NewQueryBuilder().Where("id").Eq(p1.ID()).Build()

	if result, err = products.Read(ctx, &q); err != nil {
		log.Fatalf("Read updated failed: %v", err)
	}
	if result.Count > 0 {
		doc := result.Data.(data.Document)
		logger.Info("After update",
			zap.String("id", doc.ID()),
			zap.Int("stock", doc["stock"].(int)),
		)
	}

	// Delete
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
