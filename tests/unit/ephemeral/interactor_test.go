package ephemeral_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/tests/testutils"
	"github.com/stretchr/testify/assert"
)

const userSchemaJSON = `{
	"name": "users",
	"version": "1.0.0",
	"description": "A collection of users",
	"fields": {
		"id": {
			"name": "id",
			"type": "integer"
		},
		"name": {
			"name": "name",
			"type": "string",
			"required": true
		},
		"age": {
			"name": "age",
			"type": "integer",
			"required": true
		},
		"status": {
			"name": "status",
			"type": "string"
		},
		"address": {
			"name": "address",
			"type": "object",
			"schema": {
				"fields": {
					"city": {
						"name": "city",
						"type": "string"
					},
					"zip": {
						"name": "zip",
						"type": "integer"
					}
				}
			}
		}
	},
	"indexes": [
		{
			"name": "name_index",
			"fields": ["name"],
			"type": "unique"
		}
	]
}`


func getUserSchema(t *testing.T) schema.SchemaDefinition {
	var schemaDef schema.SchemaDefinition
	err := json.Unmarshal([]byte(userSchemaJSON), &schemaDef)
	assert.NoError(t, err)
	return schemaDef
}

func TestMain(m *testing.M) {
	testutils.ConfigureDocumentFactory()
	os.Exit(m.Run())
}

func TestEphemeralDatabaseInteractor_InsertAndSelectDocuments(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	manager := interactor.SchemaManager()
	schemaDef := getUserSchema(t)
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	docsToInsert := []map[string]any{
		{"name": "Alice", "age": 30},
		{"name": "Bob", "age": 25},
	}

	inserted, err := interactor.InsertDocuments(context.Background(), &schemaDef, docsToInsert)
	assert.NoError(t, err)
	assert.Len(t, inserted, 2)

	dsl := query.NewQueryBuilder().Build()
	selected, err := interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, selected, 2)
}

func TestEphemeralDatabaseInteractor_SelectDocuments_WithFilter(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	manager := interactor.SchemaManager()
	schemaDef := getUserSchema(t)
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	docsToInsert := []map[string]any{
		{"name": "Alice", "age": 30},
		{"name": "Bob", "age": 25},
		{"name": "Charlie", "age": 30},
	}
	_, err = interactor.InsertDocuments(context.Background(), &schemaDef, docsToInsert)
	assert.NoError(t, err)

	dsl := query.NewQueryBuilder().Where("age").Eq(30).Build()
	selected, err := interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, selected, 2)
}

func TestEphemeralDatabaseInteractor_UpdateDocuments(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	manager := interactor.SchemaManager()
	schemaDef := getUserSchema(t)
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	docsToInsert := []map[string]any{
		{"name": "Alice", "age": 30, "status": "active"},
		{"name": "Bob", "age": 25, "status": "active"},
	}
	_, err = interactor.InsertDocuments(context.Background(), &schemaDef, docsToInsert)
	assert.NoError(t, err)

	filters := query.NewQueryBuilder().Where("name").Eq("Bob").Build().Filters
	updates := map[string]any{"status": "inactive"}

	updatedCount, err := interactor.UpdateDocuments(context.Background(), &schemaDef, updates, nil, filters)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), updatedCount)

	dsl := query.NewQueryBuilder().Where("status").Eq("inactive").Build()
	selected, err := interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, selected, 1)
	assert.Equal(t, "Bob", selected[0]["name"])
}

func TestEphemeralDatabaseInteractor_DeleteDocuments(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	manager := interactor.SchemaManager()
	schemaDef := getUserSchema(t)
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	docsToInsert := []map[string]any{
		{"name": "Alice", "age": 30},
		{"name": "Bob", "age": 25},
	}
	_, err = interactor.InsertDocuments(context.Background(), &schemaDef, docsToInsert)
	assert.NoError(t, err)

	filters := query.NewQueryBuilder().Where("age").Gt(28).Build().Filters
	deletedCount, err := interactor.DeleteDocuments(context.Background(), &schemaDef, filters, false)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), deletedCount)

	dsl := query.NewQueryBuilder().Build()
	selected, err := interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, selected, 1)
}

func TestEphemeralDatabaseInteractor_SelectDocuments_WithNestedProjection(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	manager := interactor.SchemaManager()
	schemaDef := getUserSchema(t)
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	docsToInsert := []map[string]any{
		{"name": "Alice", "age": 30, "address": map[string]any{"city": "New York", "zip": 10001}},
	}
	_, err = interactor.InsertDocuments(context.Background(), &schemaDef, docsToInsert)
	assert.NoError(t, err)

	dsl := query.NewQueryBuilder().Select().Include("name", "address.city").End().Build()
	selected, err := interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, selected, 1)
	assert.Equal(t, "Alice", selected[0]["name"])
	assert.Nil(t, selected[0]["age"])
	address, ok := selected[0]["address"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "New York", address["city"])
	assert.Nil(t, address["zip"])
}

func TestEphemeralDatabaseInteractor_SelectDocuments_WithNestedFilter(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	manager := interactor.SchemaManager()
	schemaDef := getUserSchema(t)
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	docsToInsert := []map[string]any{
		{"name": "Alice", "age": 30, "address": map[string]any{"city": "New York"}},
		{"name": "Bob", "age": 25, "address": map[string]any{"city": "London"}},
	}
	_, err = interactor.InsertDocuments(context.Background(), &schemaDef, docsToInsert)
	assert.NoError(t, err)

	dsl := query.NewQueryBuilder().Where("address.city").Eq("London").Build()
	selected, err := interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, selected, 1)
	assert.Equal(t, "Bob", selected[0]["name"])
}

func TestEphemeralDatabaseInteractor_SelectDocuments_EmptyResult(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	manager := interactor.SchemaManager()
	schemaDef := getUserSchema(t)
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	dsl := query.NewQueryBuilder().Where("name").Eq("non-existent").Build()
	selected, err := interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, selected, 0)
}

func TestEphemeralDatabaseInteractor_UpdateDocuments_NoMatch(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	manager := interactor.SchemaManager()
	schemaDef := getUserSchema(t)
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	filters := query.NewQueryBuilder().Where("name").Eq("non-existent").Build().Filters
	updates := map[string]any{"status": "inactive"}

	updatedCount, err := interactor.UpdateDocuments(context.Background(), &schemaDef, updates, nil, filters)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), updatedCount)
}

func TestEphemeralDatabaseInteractor_DeleteDocuments_NoMatch(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	manager := interactor.SchemaManager()
	schemaDef := getUserSchema(t)
	err := manager.CreateCollection(context.Background(),schemaDef)
	assert.NoError(t, err)

	filters := query.NewQueryBuilder().Where("name").Eq("non-existent").Build().Filters
	deletedCount, err := interactor.DeleteDocuments(context.Background(), &schemaDef, filters, false)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), deletedCount)
}

func TestEphemeralDatabaseInteractor_SelectDocuments_WithJoin(t *testing.T) {
	interactor := ephemeral.NewEphemeral()
	manager := interactor.SchemaManager()

	// Create users collection
	userSchema := getUserSchema(t)
	err := manager.CreateCollection(context.Background(),userSchema)
	assert.NoError(t, err)

	// Create orders collection
	orderSchemaJSON := `{
		"name": "orders",
		"fields": {
			"order_id": {"type": "integer"},
			"user_id": {"type": "integer"},
			"amount": {"type": "float"}
		}
	}`
	var orderSchema schema.SchemaDefinition
	err = json.Unmarshal([]byte(orderSchemaJSON), &orderSchema)
	assert.NoError(t, err)
	err = manager.CreateCollection(context.Background(),orderSchema)
	assert.NoError(t, err)

	// Insert data
	users := []map[string]any{
		{"id": 1, "name": "Alice", "age": 30},
		{"id": 2, "name": "Bob", "age": 25},
	}
	_, err = interactor.InsertDocuments(context.Background(), &userSchema, users)
	assert.NoError(t, err)

	orders := []map[string]any{
		{"order_id": 101, "user_id": 1, "amount": 150.0},
		{"order_id": 102, "user_id": 2, "amount": 200.0},
		{"order_id": 103, "user_id": 1, "amount": 50.0},
	}
	_, err = interactor.InsertDocuments(context.Background(), &orderSchema, orders)
	assert.NoError(t, err)

	// Build join query
	dsl := query.NewQueryBuilder().
		InnerJoin("orders").
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "users.id",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Type: "field", Field: "orders.user_id"}},
			},
		}).
		End().
		Build()

	// Execute query
	results, err := interactor.SelectDocuments(context.Background(), &userSchema, &dsl)
	assert.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify results
	for _, r := range results {
		user, ok := r["users"].(map[string]any)
		assert.True(t, ok, "Expected 'users' to be a map[string]any")
		order, ok := r["orders"].(map[string]any)
		assert.True(t, ok, "Expected 'orders' to be a map[string]any")
		assert.Equal(t, user["id"], order["user_id"], "User ID and Order User ID should match")
	}
}
