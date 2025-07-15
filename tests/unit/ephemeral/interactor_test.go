package ephemeral_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
)

const userSchemaJSON = `{
	"name": "users",
	"version": "1.0.0",
	"description": "A collection of users",
	"fields": {
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

func TestEphemeralDatabaseInteractor_CreateCollection(t *testing.T) {
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)

	err := interactor.CreateCollection(schemaDef)
	assert.NoError(t, err)

	exists, err := interactor.CollectionExists("users")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestEphemeralDatabaseInteractor_CreateCollection_AlreadyExists(t *testing.T) {
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)

	err := interactor.CreateCollection(schemaDef)
	assert.NoError(t, err)

	err = interactor.CreateCollection(schemaDef)
	assert.Error(t, err)
}

func TestEphemeralDatabaseInteractor_DropCollection(t *testing.T) {
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)

	err := interactor.CreateCollection(schemaDef)
	assert.NoError(t, err)

	err = interactor.DropCollection("users")
	assert.NoError(t, err)

	exists, err := interactor.CollectionExists("users")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestEphemeralDatabaseInteractor_InsertAndSelectDocuments(t *testing.T) {
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)
	err := interactor.CreateCollection(schemaDef)
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
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)
	err := interactor.CreateCollection(schemaDef)
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
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)
	err := interactor.CreateCollection(schemaDef)
	assert.NoError(t, err)

	docsToInsert := []map[string]any{
		{"name": "Alice", "age": 30, "status": "active"},
		{"name": "Bob", "age": 25, "status": "active"},
	}
	_, err = interactor.InsertDocuments(context.Background(), &schemaDef, docsToInsert)
	assert.NoError(t, err)

	filters := query.NewQueryBuilder().Where("name").Eq("Bob").Build().Filters
	updates := map[string]any{"status": "inactive"}

	updatedCount, err := interactor.UpdateDocuments(context.Background(), &schemaDef, updates, filters)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), updatedCount)

	dsl := query.NewQueryBuilder().Where("status").Eq("inactive").Build()
	selected, err := interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, selected, 1)
	assert.Equal(t, "Bob", selected[0]["name"])
}

func TestEphemeralDatabaseInteractor_DeleteDocuments(t *testing.T) {
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)
	err := interactor.CreateCollection(schemaDef)
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

func TestEphemeralDatabaseInteractor_CreateIndex(t *testing.T) {
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)
	err := interactor.CreateCollection(schemaDef)
	assert.NoError(t, err)

	index := schema.IndexDefinition{
		Name:   "age_index",
		Fields: []string{"age"},
	}
	err = interactor.CreateIndex("users", index)
	assert.NoError(t, err)
}

func TestEphemeralDatabaseInteractor_SelectDocuments_WithNestedProjection(t *testing.T) {
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)
	err := interactor.CreateCollection(schemaDef)
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
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)
	err := interactor.CreateCollection(schemaDef)
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
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)
	err := interactor.CreateCollection(schemaDef)
	assert.NoError(t, err)

	dsl := query.NewQueryBuilder().Where("name").Eq("non-existent").Build()
	selected, err := interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, selected, 0)
}

func TestEphemeralDatabaseInteractor_UpdateDocuments_NoMatch(t *testing.T) {
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)
	err := interactor.CreateCollection(schemaDef)
	assert.NoError(t, err)

	filters := query.NewQueryBuilder().Where("name").Eq("non-existent").Build().Filters
	updates := map[string]any{"status": "inactive"}

	updatedCount, err := interactor.UpdateDocuments(context.Background(), &schemaDef, updates, filters)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), updatedCount)
}

func TestEphemeralDatabaseInteractor_DeleteDocuments_NoMatch(t *testing.T) {
	interactor := ephemeral.NewEphemeralDatabaseInteractor()
	schemaDef := getUserSchema(t)
	err := interactor.CreateCollection(schemaDef)
	assert.NoError(t, err)

	filters := query.NewQueryBuilder().Where("name").Eq("non-existent").Build().Filters
	deletedCount, err := interactor.DeleteDocuments(context.Background(), &schemaDef, filters, false)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), deletedCount)
}
