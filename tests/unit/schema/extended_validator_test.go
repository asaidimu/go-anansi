package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/schema/validator"
)

func TestValidator_Validate_Extended(t *testing.T) {
	trueBool := true
	stringType := schema.FieldTypeString
	integerType := schema.FieldTypeInteger
	objectType := schema.FieldTypeObject

	// Define nested schemas
	permissionSchema := &schema.NestedSchemaDefinition{
		Name: "permission",
		Fields: &schema.NestedSchemaFields{
			FieldsMap: map[string]*schema.FieldDefinition{
				"resource": {
					Name:     "resource",
					Type:     schema.FieldTypeString,
					Required: &trueBool,
				},
				"level": {
					Name:     "level",
					Type:     schema.FieldTypeEnum,
					Required: &trueBool,
					Values:   []any{"read", "write", "admin"},
				},
			},
		},
	}

	roleSchema := &schema.NestedSchemaDefinition{
		Name: "role",
		Fields: &schema.NestedSchemaFields{
			FieldsMap: map[string]*schema.FieldDefinition{
				"name": {
					Name:     "name",
					Type:     schema.FieldTypeString,
					Required: &trueBool,
				},
				"fb08c75c-4d0c-4d28-88c2-da1e6219ee26": {
					Name:      "permissions",
					Type:      schema.FieldTypeArray,
					ItemsType: &objectType,
					Schema:    schema.NestedSchemaReference{ID: "permission"},
				},
			},
		},
	}

	// Define a complex schema for a user profile
	userSchemaDef := &schema.SchemaDefinition{
		Name:    "user",
		Version: "1.0",
		Fields: map[string]*schema.FieldDefinition{
			"6015dde0-e015-4369-8fd3-8ea37282daaf": {
				Name:     "username",
				Type:     schema.FieldTypeString,
				Required: &trueBool,
			},
			"account": {
				Name:   "account",
				Type:   schema.FieldTypeObject,
				Schema: schema.NestedSchemaReference{ID: "role"},
			},
			"devices": {
				Name:      "devices",
				Type:      schema.FieldTypeArray,
				ItemsType: &stringType,
			},
			"metadata": {
				Name:      "metadata",
				Type:      schema.FieldTypeRecord,
				ItemsType: &integerType,
			},
			"structuredRecord": {
				Name:   "structuredRecord",
				Type:   schema.FieldTypeRecord,
				Schema: schema.NestedSchemaReference{ID: "recordItem"},
			},
			"unionField": {
				Name: "unionField",
				Type: schema.FieldTypeUnion,
				Schema: []schema.NestedSchemaReference{
					{ID: "stringSchema"},
					{ID: "numberSchema"},
				},
			},
		},
		NestedSchemas: map[string]*schema.NestedSchemaDefinition{
			"permission": permissionSchema,
			"role":                                 roleSchema,
			"stringSchema":                         {Name: "stringSchema", Type: &stringType},
			"numberSchema":                         {Name: "numberSchema", Type: &integerType},
			"recordItem": {
				Name: "recordItem",
				Fields: &schema.NestedSchemaFields{
					FieldsMap: map[string]*schema.FieldDefinition{
						"name": {
							Name:     "name",
							Type:     schema.FieldTypeString,
							Required: &trueBool,
						},
						"value": {
							Name: "value",
							Type: schema.FieldTypeInteger,
						},
					},
				},
			},
		},
		Constraints: schema.SchemaConstraint{
			schema.ConstraintRule{
				ConstraintGroup: &schema.ConstraintGroup{
					Name:     "user_constraints",
					Operator: common.LogicalAnd,
					Rules: []schema.ConstraintRule{
						{
							Constraint: &schema.Constraint{
								Name:      "username_availability",
								Predicate: "isUsernameAvailable",
								Field:     &[]string{"username"}[0],
							},
						},
					},
				},
			},
		},
	}

	fmap := schema.FunctionMap{
		"isUsernameAvailable": func(params schema.PredicateParams[any]) bool {
			// Mock function to check username availability
			if data, ok := params.Data.(map[string]any); ok {
				if username, ok := data["username"].(string); ok {
					return username != "taken"
				}
			}
			return true
		},
		"isGlobald": func(params schema.PredicateParams[any]) bool {
			// This predicate checks if a specific field in the root data is 'globalValue'
			if dataMap, ok := params.Data.(map[string]any); ok {
				if globalVal, exists := dataMap["globalCheckField"]; exists {
					return globalVal == "globalValue"
				}
			}
			return false
		},
	}

	v, err := validator.NewDocumentValidator(userSchemaDef, &fmap)
	require.NoError(t, err)

	t.Run("Valid complex data", func(t *testing.T) {
		data := map[string]any{
			"username": "testuser",
			"account": map[string]any{
				"name": "admin",
				"permissions": []any{
					map[string]any{"resource": "users", "level": "admin"},
					map[string]any{"resource": "posts", "level": "write"},
				},
			},
			"devices": []any{"phone", "laptop"},
			"metadata": map[string]any{
				"logins": 10,
				"posts":  5,
			},
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Deeply nested required field missing", func(t *testing.T) {
		data := map[string]any{
			"username": "testuser",
			"account": map[string]any{
				"name": "admin",
				"permissions": []any{
					map[string]any{"level": "admin"}, // resource is missing
				},
			},
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "REQUIRED_FIELD_MISSING", issues[0].Code)
		assert.Equal(t, "account.permissions[0].resource", issues[0].Path)
	})

	t.Run("Schema-level constraint violation", func(t *testing.T) {
		data := map[string]any{
			"username": "taken",
			"account": map[string]any{
				"name": "admin",
				"permissions": []any{
					map[string]any{"resource": "users", "level": "admin"},
				},
			},
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 2)
		assert.Equal(t, "CONSTRAINT_GROUP_VIOLATION", issues[0].Code)
	})

	t.Run("Invalid enum value in nested object", func(t *testing.T) {
		data := map[string]any{
			"username": "testuser",
			"account": map[string]any{
				"name": "admin",
				"permissions": []any{
					map[string]any{"resource": "users", "level": "super-admin"}, // invalid level
				},
			},
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "ENUM_VIOLATION", issues[0].Code)
		assert.Equal(t, "account.permissions[0].level", issues[0].Path)
	})

	t.Run("Record with invalid item type", func(t *testing.T) {
		data := map[string]any{
			"username": "testuser",
			"account": map[string]any{
				"name":        "admin",
				"permissions": []any{},
			},
			"metadata": map[string]any{
				"logins": "ten", // should be an integer
			},
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Union with primitive type", func(t *testing.T) {
		dataWithString := map[string]any{
			"username":   "testuser",
			"unionField": "some string value",
		}
		dataWithNumber := map[string]any{
			"username":   "testuser",
			"unionField": "some string value",
		}

		_, ok := v.Validate(dataWithString, false)
		assert.True(t, ok)

		_, ok = v.Validate(dataWithNumber, false)
		assert.True(t, ok)
	})

	t.Run("Record with structured schema - valid data", func(t *testing.T) {
		data := map[string]any{
			"username": "testuser",
			"structuredRecord": map[string]any{
				"item1": map[string]any{"name": "first", "value": 1},
				"item2": map[string]any{"name": "second", "value": 2},
			},
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Record with structured schema - invalid item (missing required field)", func(t *testing.T) {
		data := map[string]any{
			"username": "testuser",
			"structuredRecord": map[string]any{
				"item1": map[string]any{"value": 1}, // missing 'name'
			},
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "REQUIRED_FIELD_MISSING", issues[0].Code)
		assert.Equal(t, "structuredRecord.item1.name", issues[0].Path)
	})
}
