package sqlite_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-anansi/v6/sqlite"
	"github.com/stretchr/testify/assert"
)

func TestSqliteQuery_GenerateSelectSQL_WithNestedFields(t *testing.T) {
	schemaDef := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString},
			"name": {Name: "name", Type: schema.FieldTypeString},
			"profile": {
				Name: "profile",
				Type: schema.FieldTypeObject,
				Schema: &schema.FieldSchema{
					ID: "profile_schema",
				},
			},
		},
		NestedSchemas: map[string]*schema.NestedSchemaDefinition{
			"profile_schema": {
				Name: "profile_schema",
				IsStructured: utils.BoolPtr(true),
				StructuredFieldsMap: map[string]*schema.FieldDefinition{
					"email": {Name: "email", Type: schema.FieldTypeString},
					"age":   {Name: "age", Type: schema.FieldTypeInteger},
				},
			},
		},
	}

	generator, err := sqlite.NewSqliteQuery(schemaDef)
	assert.NoError(t, err)

	dsl := &query.QueryDSL{
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "profile.email",
				Operator: query.ComparisonOperatorEq,
				Value:    "test@example.com",
			},
		},
	}

	expectedSQL := `SELECT * FROM "users" WHERE json_extract("profile", '$.email') = ?;`
	expectedParams := []any{"test@example.com"}

	sql, params, err := generator.GenerateSelectSQL(dsl)

	assert.NoError(t, err)
	assert.Equal(t, expectedSQL, sql)
	assert.Equal(t, expectedParams, params)
}
