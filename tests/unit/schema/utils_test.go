package schema_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
)

func TestSchemaDefinition_FindNestedSchema(t *testing.T) {
	schemaDef := &schema.SchemaDefinition{
		NestedSchemas: map[string]*schema.NestedSchemaDefinition{
			"profile_schema": {
				Name: "profile_schema",
			},
		},
	}

	t.Run("should find nested schema", func(t *testing.T) {
		nestedSchema, found := schemaDef.FindNestedSchema("profile_schema")
		assert.True(t, found)
		assert.NotNil(t, nestedSchema)
		assert.Equal(t, "profile_schema", nestedSchema.Name)
	})

	t.Run("should not find non-existent nested schema", func(t *testing.T) {
		_, found := schemaDef.FindNestedSchema("nonexistent_schema")
		assert.False(t, found)
	})
}

func TestNestedSchemaDefinition_FindField(t *testing.T) {
	nestedSchemaDef := &schema.NestedSchemaDefinition{
		Name: "profile_schema",
		Fields: &schema.NestedSchemaFields{
			FieldsMap: map[string]*schema.FieldDefinition{
				"email": {Name: "email", Type: schema.FieldTypeString},
				"age":   {Name: "age", Type: schema.FieldTypeInteger},
			},
		},
	}

	t.Run("should find field in structured map", func(t *testing.T) {
		field := nestedSchemaDef.FindField("email")
		assert.NotNil(t, field)
		assert.Equal(t, "email", field.Name)
	})

	t.Run("should not find non-existent field in structured map", func(t *testing.T) {
		field := nestedSchemaDef.FindField("nonexistent")
		assert.Nil(t, field)
	})

	nestedSchemaDefWithArray := &schema.NestedSchemaDefinition{
		Name:         "contact_schema",
		Fields: &schema.NestedSchemaFields{
			FieldsArray: []schema.ConditionalFieldSet{
			{
				Fields: map[string]*schema.FieldDefinition{
					"email": {Name: "email", Type: schema.FieldTypeString},
				},
			},
			},
		},
	}

	t.Run("should find field in structured array", func(t *testing.T) {
		field := nestedSchemaDefWithArray.FindField("email")
		assert.NotNil(t, field)
		assert.Equal(t, "email", field.Name)
	})

	t.Run("should not find non-existent field in structured array", func(t *testing.T) {
		field := nestedSchemaDefWithArray.FindField("nonexistent")
		assert.Nil(t, field)
	})
}

func TestSchemaDefinition_From(t *testing.T) {
	t.Run("should unmarshal valid json schema", func(t *testing.T) {
		jsonData := []byte(`{
			"name": "test_schema",
			"version": "1.0.0",
			"fields": {
				"name": { "type": "string", "name": "example" }
			}
		}`)
		schemaDef, err := schema.From(jsonData)
		assert.NoError(t, err)
		assert.Equal(t, "test_schema", schemaDef.Name)
		assert.Len(t, schemaDef.Fields, 1)
	})

	t.Run("should return error for invalid json", func(t *testing.T) {
		jsonData := []byte(`{
			"name": "test_schema",
			"version": "1.0.0",
			"field": {
				"name": { "type": "string", "name": "example" }
			}
		}`)
		_, err := schema.From(jsonData)
		assert.Error(t, err)
	})
}

func TestSchemaDefinition_FieldNamesStability(t *testing.T) {
	s := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"fieldC": {Name: "fieldC", Type: schema.FieldTypeString},
			"fieldA": {Name: "fieldA", Type: schema.FieldTypeString},
			"fieldB": {Name: "fieldB", Type: schema.FieldTypeString},
		},
	}

	// Call FieldNames multiple times and ensure the order is stable (sorted)
	expectedOrder := []string{"fieldA", "fieldB", "fieldC"}

	for range 10 { // Run multiple times to catch non-determinism
		actualOrder := s.FieldNames()
		assert.Equal(t, expectedOrder, actualOrder, "FieldNames should return a stable, sorted order")
	}
}
