package data_test

import (
	"context"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/stretchr/testify/require"
)

func TestStructBinder_To(t *testing.T) {
	type Address struct {
		Street string `doc:"street"`
		City   string `doc:"city"`
	}

	type User struct {
		ID        int       `doc:"user_id"`
		Name      string    `doc:"full_name,omitempty"`
		Active    bool      `doc:"is_active"`
		CreatedAt time.Time `doc:"created_at"`
		Address   Address   `doc:"address"`
		Tags      []string  `doc:"tags"`
	}

	doc, err := data.NewDocument(map[string]any{
		"user_id":    123,
		"full_name":  "John Doe",
		"is_active":  true,
		"created_at": "2023-10-27T10:00:00Z",
		"address": map[string]any{
			"street": "123 Main St",
			"city":   "Anytown",
		},
		"tags": []any{"go", "developer"},
	})
	require.NoError(t, err)

	var user User
	err = doc.BindTo(&user)
	require.NoError(t, err)

	require.Equal(t, 123, user.ID)
	require.Equal(t, "John Doe", user.Name)
	require.Equal(t, true, user.Active)
	require.Equal(t, "123 Main St", user.Address.Street)
	require.Equal(t, "Anytown", user.Address.City)
	require.Equal(t, []string{"go", "developer"}, user.Tags)

	// Test with context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = doc.BindToWithContext(ctx, &user)
	require.Error(t, err)
}

func TestBindTo_Generic(t *testing.T) {
	type Product struct {
		SKU   string  `doc:"sku"`
		Price float64 `doc:"price"`
	}

	doc, err := data.NewDocument(map[string]any{
		"sku":   "ABC-123",
		"price": 99.99,
	})
	require.NoError(t, err)

	var product Product
	err = doc.BindTo(&product)
	require.NoError(t, err)
	require.Equal(t, "ABC-123", product.SKU)
	require.Equal(t, 99.99, product.Price)
}
