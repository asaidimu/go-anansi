package main

import (
	"context"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)


func main() {
	app := NewApp()
	cleanup, err := app.Init()
	if err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}
	defer cleanup()

	logger := app.Logger

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	products, err := app.ProductsModel()
	if err != nil {
		log.Fatalf("Failed to get products collection: %v", err)
	}

	// Subscribe to events
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

	logger.Info("Typed Products collection ready.")

	// --- CRUD Operations with Type Safety ---

	// Create single product
	p1 := Product{
		Name:  "Laptop",
		Price: 1200.00,
		Stock: 50,
	}

	p1, err = products.CreateProduct(ctx, p1)
	if err != nil {
		log.Fatalf("Failed to create Laptop: %v", err)
	}
	logger.Info("Created product",
		zap.String("id", p1.ID),
		zap.String("name", p1.Name),
		zap.Float64("price", p1.Price),
		zap.Int("stock", p1.Stock),
	)

	// Create multiple products
	newProducts := []Product{
		{Name: "Mouse", Price: 25.00, Stock: 200},
		{Name: "Keyboard", Price: 75.00, Stock: 150},
		{Name: "Monitor", Price: 300.00, Stock: 75},
	}

	createdProducts, err := products.CreateProducts(ctx, newProducts)
	if err != nil {
		log.Fatalf("Failed to create products: %v", err)
	}
	logger.Info("Created multiple products", zap.Int("count", len(createdProducts)))

	// Read all products
	allProducts, err := products.ListAllProducts(ctx)
	if err != nil {
		log.Fatalf("Read failed: %v", err)
	}

	logger.Info("All products", zap.Int("count", len(allProducts)))
	for _, product := range allProducts {
		logger.Info("Product",
			zap.String("id", product.ID),
			zap.String("name", product.Name),
			zap.Float64("price", product.Price),
			zap.Int("stock", product.Stock),
		)
	}

	// Get single product by ID
	foundProduct, err := products.GetProduct(ctx, p1.ID)
	if err != nil {
		log.Fatalf("Failed to get product: %v", err)
	}
	logger.Info("Found product by ID",
		zap.String("id", foundProduct.ID),
		zap.String("name", foundProduct.Name),
	)

	// Find products with query (e.g., price > 100)
	q := query.NewQueryBuilder().Where("price").Gt(100.0).Build()
	expensiveProducts, err := products.FindProducts(ctx, &q)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	logger.Info("Expensive products", zap.Int("count", len(expensiveProducts)))
	for _, product := range expensiveProducts {
		logger.Info("Expensive product",
			zap.String("name", product.Name),
			zap.Float64("price", product.Price),
		)
	}

	// Update single product (partial update)
	updatedProduct, err := products.UpdateProduct(ctx, p1.ID, Product{
		Stock: 45, // Only update stock
	})
	if err != nil {
		log.Fatalf("Update failed: %v", err)
	}
	logger.Info("After update",
		zap.String("id", updatedProduct.ID),
		zap.String("name", updatedProduct.Name),
		zap.Int("stock", updatedProduct.Stock),
	)

	// Update multiple products matching a filter
	filter := query.NewQueryBuilder().Where("price").Lt(100.0).Build().Filters
	count, err := products.UpdateProducts(ctx, filter, Product{
		Stock: 500, // Increase stock for all cheap items
	})
	if err != nil {
		log.Fatalf("Bulk update failed: %v", err)
	}
	logger.Info("Bulk update completed", zap.Int("affected", count))

	// Delete single product
	mouseID := createdProducts[0].ID
	if err = products.DeleteProduct(ctx, mouseID); err != nil {
		log.Fatalf("Delete failed: %v", err)
	}
	logger.Info("Deleted product", zap.String("id", mouseID))

	// Verify deletion
	_, err = products.GetProduct(ctx, mouseID)
	if err != nil {
		logger.Info("Confirmed deletion - product not found")
	}

	// Delete multiple products matching filter
	deleteFilter := query.NewQueryBuilder().Where("stock").Gt(400).Build().Filters
	deletedCount, err := products.DeleteProducts(ctx, deleteFilter, false)
	if err != nil {
		log.Fatalf("Bulk delete failed: %v", err)
	}
	logger.Info("Bulk delete completed", zap.Int("deleted", deletedCount))

	// Final count
	finalProducts, err := products.ListAllProducts(ctx)
	if err != nil {
		log.Fatalf("Final read failed: %v", err)
	}
	logger.Info("Final product count", zap.Int("remaining", len(finalProducts)))
}
