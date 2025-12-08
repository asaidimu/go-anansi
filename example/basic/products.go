package main

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
)

// Product represents a product entity with type-safe fields
type Product struct {
	ID    string  `doc:"id,omitempty"`
	Name  string  `doc:"name"`
	Price float64 `doc:"price"`
	Stock int     `doc:"stock"`
}

// Products wraps a base Collection to provide type-safe operations
type Products struct {
	base.Collection
}

// NewProductsCollection creates a new typed Products collection wrapper
func NewProductsCollection(collection base.Collection) *Products {
	return &Products{
		Collection: collection,
	}
}

// CreateProduct creates a single product and returns it with auto-generated ID
func (ps *Products) CreateProduct(ctx context.Context, product Product) (Product, error) {
	var p Product

	doc, err := data.FromStructWithTags(product)
	if err != nil {
		return p, err
	}

	result, err := ps.Collection.CreateOne(ctx, doc)
	if err != nil {
		return p, err
	}

	err = result.Data.Bind().To(&p)
	return p, err
}

// CreateProducts creates multiple products and returns them with auto-generated IDs
func (ps *Products) CreateProducts(ctx context.Context, products []Product) ([]Product, error) {
	docs := make([]data.Document, 0, len(products))
	for _, product := range products {
		doc, err := data.FromStructWithTags(product)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}

	results, err := ps.Collection.CreateMany(ctx, docs)
	if err != nil {
		return nil, err
	}

	createdProducts := make([]Product, 0, len(results))
	for _, result := range results {
		var p Product
		if err := result.Data.Bind().To(&p); err != nil {
			return nil, err
		}
		createdProducts = append(createdProducts, p)
	}

	return createdProducts, nil
}

// GetProduct retrieves a single product by ID
func (ps *Products) GetProduct(ctx context.Context, id string) (Product, error) {
	var p Product

	q := query.NewQueryBuilder().Where("id").Eq(id).Build()
	result, err := ps.Collection.Read(ctx, &q)
	if err != nil {
		return p, err
	}

	if result.Count == 0 {
		return p, fmt.Errorf("product with id %s not found", id)
	}

	// Handle single document result
	switch v := result.Data.(type) {
	case data.Document:
		err = v.Bind().To(&p)
	case []data.Document:
		if len(v) > 0 {
			err = v[0].Bind().To(&p)
		}
	default:
		return p, fmt.Errorf("unexpected result type: %T", result.Data)
	}

	return p, err
}

// FindProducts retrieves products matching the given query
func (ps *Products) FindProducts(ctx context.Context, q *query.Query) ([]Product, error) {
	result, err := ps.Collection.Read(ctx, q)
	if err != nil {
		return nil, err
	}

	if result.Count == 0 {
		return []Product{}, nil
	}

	// Handle both single and multiple document results
	var docs []data.Document
	switch v := result.Data.(type) {
	case data.Document:
		docs = []data.Document{v}
	case []data.Document:
		docs = v
	default:
		return nil, fmt.Errorf("unexpected result type: %T", result.Data)
	}

	products := make([]Product, 0, len(docs))
	for _, doc := range docs {
		var p Product
		if err := doc.Bind().To(&p); err != nil {
			return nil, err
		}
		products = append(products, p)
	}

	return products, nil
}

// ListAllProducts retrieves all products in the collection
func (ps *Products) ListAllProducts(ctx context.Context) ([]Product, error) {
	q := query.NewQueryBuilder().Build()
	return ps.FindProducts(ctx, &q)
}

// UpdateProduct updates a product by ID with partial updates supported
func (ps *Products) UpdateProduct(ctx context.Context, id string, updates Product) (Product, error) {
	updateDoc, err := data.FromStructWithTags(updates, true)
	if err != nil {
		return Product{}, err
	}

	filter := query.NewQueryBuilder().Where("id").Eq(id).Build().Filters

	_, err = ps.Collection.Update(ctx, &base.CollectionUpdate{
		Filter: filter,
		Set:    updateDoc,
	})
	if err != nil {
		return Product{}, err
	}

	// Retrieve and return the updated product
	return ps.GetProduct(ctx, id)
}

// UpdateProducts updates multiple products matching the filter
func (ps *Products) UpdateProducts(ctx context.Context, filter *query.QueryFilter, updates Product) (int, error) {
	updateDoc, err := data.FromStructWithTags(updates, true)
	if err != nil {
		return 0, err
	}

	count, err := ps.Collection.Update(ctx, &base.CollectionUpdate{
		Filter: filter,
		Set:    updateDoc,
	})
	return count, err
}

// DeleteProduct deletes a single product by ID
func (ps *Products) DeleteProduct(ctx context.Context, id string) error {
	filter := query.NewQueryBuilder().Where("id").Eq(id).Build().Filters
	count, err := ps.Collection.Delete(ctx, filter, false)
	if err != nil {
		return err
	}

	if count == 0 {
		return fmt.Errorf("product with id %s not found", id)
	}

	return nil
}

// DeleteProducts deletes multiple products matching the filter
func (ps *Products) DeleteProducts(ctx context.Context, filter *query.QueryFilter, unsafe bool) (int, error) {
	return ps.Collection.Delete(ctx, filter, unsafe)
}

// ValidateProduct validates a product against the collection's schema
func (ps *Products) ValidateProduct(ctx context.Context, product Product, loose bool) error {
	doc, err := data.FromStructWithTags(product)
	if err != nil {
		return err
	}

	result, err := ps.Collection.Validate(ctx, doc, loose)
	if err != nil {
		return err
	}

	if !result.Valid {
		return fmt.Errorf("validation failed: %v", result.Issues)
	}

	return nil
}
