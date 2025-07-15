package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-anansi/v6/sqlite"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

func setupTestDB(t *testing.T) (*sql.DB, *sqlite.SQLiteInteractor) {
	db, err := sql.Open("sqlite3", ":memory:")
	assert.NoError(t, err)

	interactor := sqlite.NewSQLiteInteractor(db, zap.NewNop(), nil, nil).(*sqlite.SQLiteInteractor)
	return db, interactor
}

func TestSQLiteInteractor_SelectDocuments_NestedFields(t *testing.T) {
	db, interactor := setupTestDB(t)
	defer db.Close()

	schemaDef := schema.SchemaDefinition{
		Name: "test_users",
		Fields: map[string]*schema.FieldDefinition{
			"id":      {Name: "id", Type: schema.FieldTypeString, Required: utils.BoolPtr(true), Unique: utils.BoolPtr(true)},
			"name":    {Name: "name", Type: schema.FieldTypeString},
			"address": {
				Name: "address",
				Type: schema.FieldTypeObject,
				Schema: &schema.FieldSchema{
					ID: "address_schema",
				},
			},
		},
		NestedSchemas: map[string]*schema.NestedSchemaDefinition{
			"address_schema": {
				Name: "address_schema",
				IsStructured: utils.BoolPtr(true),
				StructuredFieldsMap: map[string]*schema.FieldDefinition{
					"street": {Name: "street", Type: schema.FieldTypeString},
					"city":   {Name: "city", Type: schema.FieldTypeString},
				},
			},
		},
	}

	// Create the table
	err := interactor.CreateCollection(schemaDef)
	assert.NoError(t, err)

	// Insert data with nested JSON
	dataToInsert := []map[string]any{
		{
			"id":   "user1",
			"name": "Alice",
			"address": map[string]any{
				"street": "123 Main St",
				"city":   "Anytown",
			},
		},
		{
			"id":   "user2",
			"name": "Bob",
			"address": map[string]any{
				"street": "456 Oak Ave",
				"city":   "Otherville",
			},
		},
	}

	_, err = interactor.InsertDocuments(context.Background(), &schemaDef, dataToInsert)
	assert.NoError(t, err)

	// Test selecting by nested field
	dsl := query.NewQueryBuilder().Where("address.city").Eq("Anytown").Build()
	results, err := interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "user1", results[0]["id"])

	// Test selecting by another nested field
	dsl = query.NewQueryBuilder().Where("address.street").Eq("456 Oak Ave").Build()
	results, err = interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "user2", results[0]["id"])

	// Test selecting with no match
	dsl = query.NewQueryBuilder().Where("address.city").Eq("NonExistentCity").Build()
	results, err = interactor.SelectDocuments(context.Background(), &schemaDef, &dsl)
	assert.NoError(t, err)
	assert.Len(t, results, 0)
}
