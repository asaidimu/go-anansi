package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNestedSchemaDefinition_UnmarshalJSON(t *testing.T) {
	t.Run("should unmarshal structured nested schema with map of fields", func(t *testing.T) {
		jsonData := `{
			"name": "address_schema",
			"fields": {
				"street": { "type": "string" },
				"city": { "type": "string" }
			}
		}`

		var nsd schema.NestedSchemaDefinition
		err := json.Unmarshal([]byte(jsonData), &nsd)

		require.NoError(t, err)
		assert.Equal(t, "address_schema", nsd.Name)
		require.NotNil(t, nsd.IsStructured)
		assert.True(t, *nsd.IsStructured)
		require.NotNil(t, nsd.StructuredFieldsMap)
		assert.Len(t, nsd.StructuredFieldsMap, 2)
		assert.Equal(t, schema.FieldTypeString, nsd.StructuredFieldsMap["street"].Type)
	})

	t.Run("should unmarshal structured nested schema with array of fields", func(t *testing.T) {
		jsonData := `{
			"name": "contact_schema",
			"fields": [
				{
					"fields": {
						"email": { "type": "string" }
					},
					"when": {
						"field": "type",
						"value": "email"
					}
				},
				{
					"fields": {
						"phone": { "type": "string" }
					}
				}
			]
		}`

		var nsd schema.NestedSchemaDefinition
		err := json.Unmarshal([]byte(jsonData), &nsd)

		require.NoError(t, err)
		assert.Equal(t, "contact_schema", nsd.Name)
		require.NotNil(t, nsd.IsStructured)
		assert.True(t, *nsd.IsStructured)
		require.NotNil(t, nsd.StructuredFieldsArray)
		assert.Len(t, nsd.StructuredFieldsArray, 2)
	})

	t.Run("should unmarshal literal nested schema", func(t *testing.T) {
		stringType := schema.FieldTypeString
		jsonData := `{
			"name": "tag_schema",
			"type": "string"
		}`

		var nsd schema.NestedSchemaDefinition
		err := json.Unmarshal([]byte(jsonData), &nsd)

		require.NoError(t, err)
		assert.Equal(t, "tag_schema", nsd.Name)
		require.NotNil(t, nsd.IsStructured)
		assert.False(t, *nsd.IsStructured)
		require.NotNil(t, nsd.Type)
		assert.Equal(t, &stringType, nsd.Type)
	})

	t.Run("should return error if both fields and type are present", func(t *testing.T) {
		jsonData := `{
			"name": "invalid_schema",
			"type": "string",
			"fields": {
				"key": { "type": "string" }
			}
		}`

		var nsd schema.NestedSchemaDefinition
		err := json.Unmarshal([]byte(jsonData), &nsd)
		assert.Error(t, err)
	})

	t.Run("should return error if neither fields nor type are present", func(t *testing.T) {
		jsonData := `{
			"name": "invalid_schema"
		}`

		var nsd schema.NestedSchemaDefinition
		err := json.Unmarshal([]byte(jsonData), &nsd)
		assert.Error(t, err)
	})
}

func TestNestedSchemaDefinition_MarshalJSON(t *testing.T) {
	t.Run("should marshal structured nested schema with map of fields", func(t *testing.T) {
		trueBool := true
		nsd := schema.NestedSchemaDefinition{
			Name:         "address_schema",
			IsStructured: &trueBool,
			StructuredFieldsMap: map[string]*schema.FieldDefinition{
				"street": {Type: schema.FieldTypeString},
				"city":   {Type: schema.FieldTypeString},
			},
		}

		data, err := json.Marshal(nsd)
		require.NoError(t, err)

		var unmarshalled map[string]any
		err = json.Unmarshal(data, &unmarshalled)
		require.NoError(t, err)

		assert.Equal(t, "address_schema", unmarshalled["name"])
		assert.NotNil(t, unmarshalled["fields"])
	})

	t.Run("should marshal literal nested schema", func(t *testing.T) {
		falseBool := false
		stringType := schema.FieldTypeString
		nsd := schema.NestedSchemaDefinition{
			Name:         "tag_schema",
			IsStructured: &falseBool,
			Type:         &stringType,
		}

		data, err := json.Marshal(nsd)
		require.NoError(t, err)

		var unmarshalled map[string]any
		err = json.Unmarshal(data, &unmarshalled)
		require.NoError(t, err)

		assert.Equal(t, "tag_schema", unmarshalled["name"])
		assert.Equal(t, "string", unmarshalled["type"])
		assert.Nil(t, unmarshalled["fields"])
	})
}
