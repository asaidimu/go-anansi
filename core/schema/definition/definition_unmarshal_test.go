package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchema_Unmarshal_WithNestedSchema(t *testing.T) {
	jsonStr := `
	{
		"name": "RootSchema",
		"version": "1.0.0",
		"fields": {
			"user": {
				"name": "user",
				"type": "object",
				"schema": {
					"id": "UserSchema"
				}
			}
		},
		"schemas": {
			"UserSchema": {
				"name": "UserSchema",
				"type": "object",
				"fields": {
					"name": {
						"name": "name",
						"type": "string",
						"required": true
					},
					"email": {
						"name": "email",
						"type": "string"
					}
				}
			}
		}
	}`

	var schema definition.Schema
	err := json.Unmarshal([]byte(jsonStr), &schema)
	require.NoError(t, err)

	// Check root schema properties
	assert.Equal(t, "RootSchema", schema.Name)
	assert.Len(t, schema.Fields, 1)

	// Check nested schema properties
	require.Len(t, schema.Schemas, 1)
	nestedSchema, ok := schema.Schemas["UserSchema"]
	require.True(t, ok, "Nested schema 'UserSchema' not found")
	assert.Equal(t, "UserSchema", nestedSchema.Name)
	assert.Equal(t, definition.FieldTypeObject, nestedSchema.Type)

	// Check fields within the nested schema
	require.Len(t, nestedSchema.Fields, 2)

	nameField, ok := nestedSchema.Fields["name"]
	require.True(t, ok, "Field 'name' not found in nested schema")
	assert.Equal(t, definition.FieldName("name"), nameField.Name)
	assert.Equal(t, definition.FieldTypeString, nameField.Type)
	assert.True(t, nameField.Required)

	emailField, ok := nestedSchema.Fields["email"]
	require.True(t, ok, "Field 'email' not found in nested schema")
	assert.Equal(t, definition.FieldName("email"), emailField.Name)
	assert.Equal(t, definition.FieldTypeString, emailField.Type)
	assert.False(t, emailField.Required)
}

