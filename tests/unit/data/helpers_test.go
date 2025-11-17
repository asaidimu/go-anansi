package data_test

import (
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetGeneric(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{
		"name":  "Alice",
		"age":   30,
		"active": true,
	})

	// Test successful retrieval
	name, err := data.Get[string](doc, "name")
	require.NoError(t, err)
	assert.Equal(t, "Alice", name)

	age, err := data.Get[int](doc, "age")
	require.NoError(t, err)
	assert.Equal(t, 30, age)

	active, err := data.Get[bool](doc, "active")
	require.NoError(t, err)
	assert.Equal(t, true, active)

	// Test key not found
	_, err = data.Get[string](doc, "nonexistent")
	assert.Error(t, err)
	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)
	assert.Equal(t, data.ErrKeyNotFound.Code, sysErr.Code)

	// Test type mismatch
	_, err = data.Get[int](doc, "name")
	assert.Error(t, err)
	require.ErrorAs(t, err, &sysErr)
	assert.Equal(t, data.ErrTypeConversion.Code, sysErr.Code)
}

func TestGetWithCoercion(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{
		"age_str":   "30",
		"price_int": 100,
		"is_admin":  "true",
		"timestamp": "2023-01-01T12:00:00Z",
	})

	// Test string to int coercion
	age, err := data.GetWithCoercion[int](doc, "age_str")
	require.NoError(t, err)
	assert.Equal(t, 30, age)

	// Test int to float64 coercion
	price, err := data.GetWithCoercion[float64](doc, "price_int")
	require.NoError(t, err)
	assert.Equal(t, 100.0, price)

	// Test string to bool coercion
	isAdmin, err := data.GetWithCoercion[bool](doc, "is_admin")
	require.NoError(t, err)
	assert.Equal(t, true, isAdmin)

	// Test string to time.Time coercion
	timestamp, err := data.GetWithCoercion[time.Time](doc, "timestamp")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC), timestamp)

	// Test failed coercion
	_, err = data.GetWithCoercion[int](doc, "is_admin")
	assert.Error(t, err)
	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)
	assert.Equal(t, data.ErrTypeConversion.Code, sysErr.Code)
}

func TestMustHelper(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{}).StripMetadata() // Empty document

	// This should panic with ErrKeyNotFound
	assert.Panics(t, func() {
		doc.Must().Get("nonexistent_key")
	})
}



func TestFluentQuery_Limit(t *testing.T) {
	docs := data.NewDocumentSet(
		data.MustNewDocument(map[string]any{}).StripMetadata(),
		data.MustNewDocument(map[string]any{}).StripMetadata(),
		data.MustNewDocument(map[string]any{}).StripMetadata(),
	)

	limitedDocs := data.Query(docs).Limit(1).Execute()
	assert.Len(t, limitedDocs, 1)
}

func TestStructBinder(t *testing.T) {
	type User struct {
		Name    string `doc:"name"`
		Age     int    `doc:"age"`
		IsAdmin bool   `doc:"is_admin,omitempty"`
		Email   string `doc:"email,omitempty"`
		Address struct {
			Street string `doc:"street"`
			City   string `doc:"city"`
		} `doc:"address"`
		CreatedAt time.Time `doc:"created_at"`
	}

	doc := data.MustNewDocument(map[string]any{
		"name":       "Alice",
		"age":        30,
		"is_admin":   true,
		"address": map[string]any{
			"street": "123 Main St",
			"city":   "Anytown",
		},
		"created_at": "2024-01-01T10:00:00Z",
	})

	var user User
	err := doc.Bind().To(&user)
	require.NoError(t, err)

	assert.Equal(t, "Alice", user.Name)
	assert.Equal(t, 30, user.Age)
	assert.True(t, user.IsAdmin)
	assert.Equal(t, "123 Main St", user.Address.Street)
	assert.Equal(t, "Anytown", user.Address.City)
	assert.Equal(t, time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), user.CreatedAt)
	assert.Empty(t, user.Email) // omitempty field not present

	// Test with missing required field
	docMissing := data.MustNewDocument(map[string]any{
		"name": "Bob",
		// age is missing
	})
	var userMissing User
	err = docMissing.Bind().To(&userMissing)
	assert.Error(t, err)
	sysErr, ok := err.(*common.SystemError)
	assert.True(t,ok)
	assert.Equal(t, sysErr.Code, data.ErrRequiredFieldNotFound.Code)

	// Test BindTo generic helper
	userFromGeneric, err := data.BindTo[User](doc)
	require.NoError(t, err)
	assert.Equal(t, "Alice", userFromGeneric.Name)
}

func TestFromStructWithTags(t *testing.T) {
	type Product struct {
		ID    string  `doc:"product_id"`
		Name  string  `doc:"product_name"`
		Price float64 `doc:"price"`
		Stock int     `doc:"stock,omitempty"`
		Desc  string  `doc:"-"` // Ignored field
	}

	product := Product{
		ID:    "P001",
		Name:  "Laptop",
		Price: 1200.50,
		Stock: 10,
		Desc:  "A great laptop",
	}

	doc, err := data.FromStructWithTags(product)
	require.NoError(t, err)

	assert.Equal(t, "P001", doc.Must().Get("product_id"))
	assert.Equal(t, "Laptop", doc.Must().Get("product_name"))
	assert.Equal(t, 1200.50, doc.Must().Get("price"))
	assert.Equal(t, 10, doc.Must().GetInt("stock"))
	assert.False(t, doc.HasKey("Desc")) // Ignored field

	// Test omitempty
	productNoStock := Product{
		ID:    "P002",
		Name:  "Mouse",
		Price: 25.0,
		Stock: 0, // Zero value, should be omitted
	}
	docNoStock, err := data.FromStructWithTags(productNoStock)
	require.NoError(t, err)
	assert.False(t, docNoStock.HasKey("stock"))
}
