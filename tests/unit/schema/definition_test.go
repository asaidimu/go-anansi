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
		assert.True(t, nsd.IsStructured())
		require.NotNil(t, nsd.Fields.FieldsMap)
		assert.Len(t, nsd.Fields.FieldsMap, 2)
		assert.Equal(t, schema.FieldTypeString, nsd.Fields.FieldsMap["street"].Type)
	})

	t.Run("should unmarshal structured nested schema with array of fields", func(t *testing.T) {
		jsonData := `{
			"name": "OrganisationalUnit",
  "version": "1.0.0",
  "description": "Represents a unit within an organizational structure, supporting hierarchical and peer-to-peer groupings.",
  "fields": {
    "name": {
      "name": "name",
      "type": "string",
      "description": "Name of the organizational unit.",
      "required": true
    },
    "parent": {
      "name": "parent",
      "type": "union",
      "description": "Optional parent unit for hierarchy, identified by ID or full OrganisationalUnit object.",
      "required": false,
      "schema": [
        {
          "id": "GenericString"
        },
        {
          "id": "OrganisationalUnit"
        }
      ]
    },
    "members": {
      "name": "members",
      "type": "array",
      "description": "Array of memberships within this organizational unit.",
      "required": true,
      "itemsType": "object",
      "schema": {
        "id": "Membership"
      }
    },
    "data": {
      "name": "data",
      "type": "record",
      "description": "Unit-specific data for the organizational unit.",
      "required": true
    },
    "metadata": {
      "name": "metadata",
      "type": "record",
      "description": "Optional metadata for the organizational unit.",
      "required": false
    }
  },
  "nestedSchemas": {
    "GenericStringSchema": {
      "name": "GenericString",
      "description": "A generic string type used for union memberships.",
      "type": "string",
      "concrete": false
    },
    "MembershipSchema": {
        "name": "Membership",
        "description": "Represents a membership within an organizational unit, linking a person to specific membership-related data.",
        "type": "record"
      },
    "PersonSchema": {
      "name": "Person",
      "description": "Represents a person or entity, such as an employee or firm.",
      "concrete": true,
      "fields": {
        "id": {
          "name": "id",
          "type": "string",
          "description": "Unique identifier (e.g., 'EMP123' for employees, 'FRM456' for firms). Supports alphanumeric IDs or UUIDs.",
          "required": true,
          "unique": true,
          "hint": {
            "input": {
              "type": "text",
              "placeholder": "e.g., EMP123"
            }
          }
        },
        "data": {
          "name": "data",
          "type": "record",
          "description": "Core data specific to the entity, including its type (natural or artificial person).",
          "required": true
        },
        "metadata": {
          "name": "metadata",
          "type": "record",
          "description": "Metadata for additional context (e.g., { hireDate: ISOStringDate }). Must include 'data-role'.",
          "required": true
        }
      }
    }
  }
		}`

		nsd, err := schema.From([]byte(jsonData))
		assert.NoError(t, err)
		assert.Equal(t, "OrganisationalUnit", nsd.Name)
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
		assert.True(t, nsd.IsStructured())
		require.NotNil(t, nsd.Fields.FieldsArray)
		assert.Len(t, nsd.Fields.FieldsArray, 2)
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
		assert.False(t, nsd.IsStructured())
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
		nsd := schema.NestedSchemaDefinition{
			Name: "address_schema",
			Fields: &schema.NestedSchemaFields{
				FieldsMap: map[string]*schema.FieldDefinition{
					"street": {Type: schema.FieldTypeString},
					"city":   {Type: schema.FieldTypeString},
				},
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
		stringType := schema.FieldTypeString
		nsd := schema.NestedSchemaDefinition{
			Name: "tag_schema",
			Type: &stringType,
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
