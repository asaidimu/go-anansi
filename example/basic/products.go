package main

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
)

const ProductsCollectionName = "Products"

// Product represents a product entity with type-safe fields
type Product struct {
	ID    string  `doc:"id,omitempty"`
	Name  string  `doc:"name"`
	Price float64 `doc:"price"`
	Stock int     `doc:"stock"`
}

// Products wraps a base Collection to provide type-safe operations
type Products struct {
	base.ModelCollection[Product]
}

// CreateProduct creates a single product and returns it with auto-generated ID
func (ps *Products) CreateProduct(ctx context.Context, product Product) (Product, error) {
	return ps.Create(ctx, product)
}

// CreateProducts creates multiple products and returns them with auto-generated IDs
func (ps *Products) CreateProducts(ctx context.Context, products []Product) ([]Product, error) {
	results, err := ps.ModelCollection.CreateMany(ctx, products)
	return results, err
}

// GetProduct retrieves a single product by ID
func (ps *Products) GetProduct(ctx context.Context, id string) (Product, error) {
	return ps.FindByID(ctx, id)
}

// FindProducts retrieves products matching the given query
func (ps *Products) FindProducts(ctx context.Context, q *query.Query) ([]Product, error) {
	return ps.Read(ctx, q)
}

// ListAllProducts retrieves all products in the collection
func (ps *Products) ListAllProducts(ctx context.Context) ([]Product, error) {
	q := query.NewQueryBuilder().Build()
	return ps.FindProducts(ctx, &q)
}

// UpdateProduct updates a product by ID with partial updates supported
func (ps *Products) UpdateProduct(ctx context.Context, id string, updates Product) (Product, error) {
	return ps.Update(ctx, id, updates)
}

// UpdateProducts updates multiple products matching the filter
func (ps *Products) UpdateProducts(ctx context.Context, filter *query.QueryFilter, updates Product) (int, error) {
	return ps.UpdateMany(ctx, filter, updates)
}

// DeleteProduct deletes a single product by ID
func (ps *Products) DeleteProduct(ctx context.Context, id string) error {
	return  ps.DeleteByID(ctx, id)
}

// DeleteProducts deletes multiple products matching the filter
func (ps *Products) DeleteProducts(ctx context.Context, filter *query.QueryFilter, unsafe bool) (int, error) {
	return ps.DeleteMany(ctx, filter, unsafe)
}

// ValidateProduct validates a product against the collection's schema
func (ps *Products) ValidateProduct(ctx context.Context, product Product, loose bool) error {
	return ps.Validate(ctx, product, loose)
}
