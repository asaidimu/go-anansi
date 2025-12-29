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

	// --- Demonstrate Users and Carts ---
	logger.Info("--- Testing Users and Carts ---")

	users, err := app.UsersModel()
	if err != nil {
		log.Fatalf("Failed to get users collection: %v", err)
	}
	logger.Info("Typed Users collection ready.")

	// Create single user
	u1 := User{
		Username: "john.doe",
		Email:    "john.doe@example.com",
	}
	u1, err = users.CreateUser(ctx, u1)
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}
	logger.Info("Created user", zap.String("id", u1.ID), zap.String("username", u1.Username))

	// Create multiple users
	newUsers := []User{
		{Username: "jane.doe", Email: "jane.doe@example.com"},
		{Username: "peter.jones", Email: "peter.jones@example.com"},
	}
	createdUsers, err := users.CreateUsers(ctx, newUsers)
	if err != nil {
		log.Fatalf("Failed to create users: %v", err)
	}
	logger.Info("Created multiple users", zap.Int("count", len(createdUsers)))

	// Read all users
	allUsers, err := users.ListAllUsers(ctx)
	if err != nil {
		log.Fatalf("Read all users failed: %v", err)
	}
	logger.Info("All users", zap.Int("count", len(allUsers)))

	// Get single user by ID
	foundUser, err := users.GetUser(ctx, u1.ID)
	if err != nil {
		log.Fatalf("Failed to get user: %v", err)
	}
	logger.Info("Found user by ID", zap.String("id", foundUser.ID), zap.String("username", foundUser.Username))

	// Update user
	updatedUser, err := users.UpdateUser(ctx, u1.ID, User{Username: "john.doe.updated"})
	if err != nil {
		log.Fatalf("Update user failed: %v", err)
	}
	logger.Info("Updated user", zap.String("id", updatedUser.ID), zap.String("username", updatedUser.Username))

	// Delete user
	if err = users.DeleteUser(ctx, u1.ID); err != nil {
		log.Fatalf("Delete user failed: %v", err)
	}
	logger.Info("Deleted user", zap.String("id", u1.ID))

	carts, err := app.CartsModel()
	if err != nil {
		log.Fatalf("Failed to get carts collection: %v", err)
	}
	logger.Info("Typed Carts collection ready.")

	// Create single cart
	c1 := Cart{
		UserID:     createdUsers[0].ID, // Associate with one of the created users
		ProductIDs: []string{finalProducts[0].ID, finalProducts[1].ID},
		Quantity:   1,
	}
	c1, err = carts.Create(ctx, c1)
	if err != nil {
		log.Fatalf("Failed to create cart: %v", err)
	}
	logger.Info("Created cart", zap.String("id", c1.ID), zap.String("user_id", c1.UserID))

	// Update cart
	updatedCart, err := carts.Update(ctx, c1.ID, Cart{Quantity: 2})
	if err != nil {
		log.Fatalf("Update cart failed: %v", err)
	}
	logger.Info("Updated cart", zap.String("id", updatedCart.ID), zap.Int("quantity", updatedCart.Quantity))

	// Get single cart by ID
	foundCart, err := carts.FindByID(ctx, c1.ID)
	if err != nil {
		log.Fatalf("Failed to get cart: %v", err)
	}
	logger.Info("Found cart by ID", zap.String("id", foundCart.ID), zap.String("user_id", foundCart.UserID))

	// Delete cart
	if err = carts.DeleteByID(ctx, c1.ID); err != nil {
		log.Fatalf("Delete cart failed: %v", err)
	}
	logger.Info("Deleted cart", zap.String("id", c1.ID))
}
