package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/schema/validator"
)

func TestValidator_Validate(t *testing.T) {
	trueBool := true
	schemaDef := &schema.SchemaDefinition{
		Name: "test",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"4d160d45-9aee-4d71-96dd-3fe83e9d6e6c": { // the key of the map is supposed to be a uuid
				Name:     "name", // and this is the actual name of the field
				Type:     schema.FieldTypeString,
				Required: &trueBool,
			},
			"age": {
				Name: "age",
				Type: schema.FieldTypeInteger,
			},
			"email": {
				Name: "email",
				Type: schema.FieldTypeString,
				Constraints: []schema.ConstraintRule{
					{
						Constraint: &schema.Constraint{
							Predicate: "isEmail",
						},
					},
				},
			},
		},
	}

	fmap := schema.FunctionMap{
		"isEmail": func(params schema.PredicateParams[any]) bool {
			// A very basic email validation for testing purposes
			if email, ok := params.Data.(string); ok {
				return len(email) > 5 && email[len(email)-4:] == ".com"
			}
			return false
		},
	}

	v, err := validator.NewDocumentValidator(schemaDef, &fmap)
	require.NoError(t, err)

	t.Run("Valid data", func(t *testing.T) {
		data := map[string]any{
			"name":  "John Doe",
			"age":   30,
			"email": "john.doe@example.com",
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Missing required field", func(t *testing.T) {
		data := map[string]any{
			"age": 30,
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "REQUIRED_FIELD_MISSING", issues[0].Code)
		assert.Equal(t, "name", issues[0].Path)
	})

	t.Run("Missing required field but loose is true", func(t *testing.T) {
		data := map[string]any{
			"age": 30,
		}
		issues, ok := v.Validate(data, true)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Type mismatch", func(t *testing.T) {
		data := map[string]any{
			"name": "Jane Doe",
			"age":  "thirty", // incorrect type
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "TYPE_MISMATCH", issues[0].Code)
		assert.Equal(t, "age", issues[0].Path)
	})

	t.Run("Unexpected field", func(t *testing.T) {
		data := map[string]any{
			"name":    "Jane Doe",
			"address": "123 Main St", // unexpected field
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "UNEXPECTED_FIELD", issues[0].Code)
		assert.Equal(t, "address", issues[0].Path)
	})

	t.Run("Constraint violation", func(t *testing.T) {
		data := map[string]any{
			"name":  "Jane Doe",
			"email": "not-an-email",
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "CONSTRAINT_VIOLATION", issues[0].Code)
		assert.Equal(t, "email", issues[0].Path)
	})

	t.Run("Coercion from string to integer", func(t *testing.T) {
		data := map[string]any{
			"name": "Test User",
			"age":  "42",
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Coercion from string to boolean", func(t *testing.T) {
		schemaDefWithBool := &schema.SchemaDefinition{
		Name: "test",
		Version: "1.0.0",
			Fields: map[string]*schema.FieldDefinition{
				"isActive": {
					Name: "isActive",
					Type: schema.FieldTypeBoolean,
				},
			},
		}
		vWithBool, err := validator.NewDocumentValidator(schemaDefWithBool, &fmap)
		require.NoError(t, err)
		data := map[string]any{
			"isActive": "true",
		}
		issues, ok := vWithBool.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})
}

func TestValidator_Validate_Advanced(t *testing.T) {
	trueBool := true
	stringType := schema.FieldTypeString
	nestedSchema := &schema.NestedSchemaDefinition{
		Name:         "address",
		Fields: &schema.NestedSchemaFields{
			FieldsMap: map[string]*schema.FieldDefinition{
			"street": {
				Name:     "street",
				Type:     schema.FieldTypeString,
				Required: &trueBool,
			},
			"city": {
				Name: "city",
				Type: schema.FieldTypeString,
			},
		},
		},
	}

	contactSchema := &schema.NestedSchemaDefinition{
		Name:         "contact",
		Fields: &schema.NestedSchemaFields{
			FieldsMap: map[string]*schema.FieldDefinition{
			"email": {
				Name: "email",
				Type: schema.FieldTypeString,
			},
			"phone": {
				Name: "phone",
				Type: schema.FieldTypeString,
			},
		},
		},
	}

	schemaDef := &schema.SchemaDefinition{
		Name: "test",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"profile": {
				Name:   "profile",
				Type:   schema.FieldTypeObject,
				Schema: schema.NestedSchemaReference{ID: "address"},
			},
			"tags": {
				Name:      "tags",
				Type:      schema.FieldTypeArray,
				ItemsType: &stringType,
			},
			"contacts": {
				Name:      "contacts",
				Type:      schema.FieldTypeSet,
				ItemsType: &stringType,
			},
			"primaryContact": {
				Name: "primaryContact",
				Type: schema.FieldTypeUnion,
				Schema: []schema.NestedSchemaReference{
					{ID: "address"},
					{ID: "contact"},
				},
			},
			"nullableField": {
				Name: "nullableField",
				Type: schema.FieldTypeString,
			},
		},
		NestedSchemas: map[string]*schema.NestedSchemaDefinition{
			"address": nestedSchema,
			"contact": contactSchema,
		},
	}

	v, err := validator.NewDocumentValidator(schemaDef, nil)
	require.NoError(t, err)

	t.Run("Nested object validation success", func(t *testing.T) {
		data := map[string]any{
			"profile": map[string]any{
				"street": "123 Main St",
				"city":   "Anytown",
			},
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Nested object validation failure", func(t *testing.T) {
		data := map[string]any{
			"profile": map[string]any{
				"city": "Anytown", // street is missing
			},
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "REQUIRED_FIELD_MISSING", issues[0].Code)
		assert.Equal(t, "profile.street", issues[0].Path)
	})

	t.Run("Array of primitives validation success", func(t *testing.T) {
		data := map[string]any{
			"tags": []any{"go", "testing"},
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Array of primitives validation failure", func(t *testing.T) {
		data := map[string]any{
			"tags": []any{"go", 123}, // 123 is not a string
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "TYPE_MISMATCH", issues[0].Code)
		assert.Equal(t, "tags[1]", issues[0].Path)
	})

	t.Run("Set validation success", func(t *testing.T) {
		data := map[string]any{
			"contacts": []any{"one", "two"},
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Set validation failure due to duplicates", func(t *testing.T) {
		data := map[string]any{
			"contacts": []any{"one", "one"},
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "SET_DUPLICATE", issues[0].Code)
		assert.Equal(t, "contacts", issues[0].Path)
	})

	t.Run("Union validation success - first type", func(t *testing.T) {
		data := map[string]any{
			"primaryContact": map[string]any{
				"street": "456 Oak Ave",
				"city":   "Otherville",
			},
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Union validation success - second type", func(t *testing.T) {
		data := map[string]any{
			"primaryContact": map[string]any{
				"email": "test@example.com",
				"phone": "555-1234",
			},
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})

	t.Run("Union validation failure - no match", func(t *testing.T) {
		data := map[string]any{
			"primaryContact": map[string]any{
				"name": "Just a name",
			},
		}
		issues, ok := v.Validate(data, false)
		assert.False(t, ok)
		require.Len(t, issues, 1)
		assert.Equal(t, "UNION_NO_MATCH", issues[0].Code)
		assert.Equal(t, "primaryContact", issues[0].Path)
	})

	t.Run("Coercion from 'null' string", func(t *testing.T) {
		data := map[string]any{
			"nullableField": "null",
		}
		issues, ok := v.Validate(data, false)
		assert.True(t, ok)
		assert.Empty(t, issues)
	})
}
