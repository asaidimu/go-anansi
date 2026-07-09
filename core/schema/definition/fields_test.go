package definition_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MustNewLiteralValueStrict is a test helper that creates a LiteralValue or panics.
func MustNewLiteralValueStrict[T definition.LiteralValueType](val T) definition.LiteralValue {
	lv, err := definition.NewLiteralValue(val)
	if err != nil {
		panic(err)
	}
	return lv
}

func TestField_MarshalJSON(t *testing.T) {
	val, err := definition.NewLiteralValue("default_string")
	require.NoError(t, err)

	schemaRef := definition.NewSchemaReference(definition.SchemaReference{ID: "nestedSchema"})

	f := definition.Field{
		Name:        "testField",
		Required:    true,
		Deprecated:  false,
		Description: "A description for testField",
		Unique:      true,
		FieldProperties: definition.FieldProperties{
			Type:    definition.FieldTypeString,
			Default: val,
			Schema:  schemaRef,
		},
	}

	data, err := json.Marshal(f)
	require.NoError(t, err)

	expected := `{
		"name": "testField",
		"required": true,
		"description": "A description for testField",
		"unique": true,
		"type": "string",
		"default": "default_string",
		"schema": {"id": "nestedSchema"}
	}`
	assert.JSONEq(t, expected, string(data))
}

func TestField_MarshalJSON_OptionalFieldsOmitted(t *testing.T) {
	f := definition.Field{
		Name: definition.FieldName("minimalField"),
		FieldProperties: definition.FieldProperties{
			Type: definition.FieldTypeBoolean,
		},
	}

	data, err := json.Marshal(f)
	require.NoError(t, err)

	expected := `{
		"name": "minimalField",
		"type": "boolean"
	}`
	assert.JSONEq(t, expected, string(data))

	// Test with explicit zero values for omitempty fields
	fWithZeroOptionals := definition.Field{
		Name:        definition.FieldName("zeroOptionalField"),
		Required:    false,
		Deprecated:  false,
		Description: "",
		Unique:      false,
		FieldProperties: definition.FieldProperties{
			Type:    definition.FieldTypeNumber,
			Default: definition.NewNullLiteral(),
			Schema:  definition.FieldSchemaReference{}, // Explicitly zero value for Schema
		},
	}
	data, err = json.Marshal(fWithZeroOptionals)
	require.NoError(t, err)
	expected = `{
		"name": "zeroOptionalField",
		"type": "number"
	}`
	assert.JSONEq(t, expected, string(data))
}

func TestField_MarshalUnmarshalJSON_DifferentFieldTypes(t *testing.T) {
	testCases := []struct {
		name      string
		fieldType definition.FieldType
		defaultValue definition.LiteralValue
	}{
		{
			name:      "StringField",
			fieldType: definition.FieldTypeString,
			defaultValue: MustNewLiteralValueStrict("hello"),
		},
		{
			name:      "NumberField",
			fieldType: definition.FieldTypeNumber,
			defaultValue: MustNewLiteralValueStrict(123.45),
		},
		{
			name:      "IntegerField",
			fieldType: definition.FieldTypeInteger,
			defaultValue: MustNewLiteralValueStrict(int64(123)),
		},
		{
			name:      "BooleanField",
			fieldType: definition.FieldTypeBoolean,
			defaultValue: MustNewLiteralValueStrict(true),
		},
		{
			name:      "ArrayField",
			fieldType: definition.FieldTypeArray,
			defaultValue: MustNewLiteralValueStrict([]any{int64(1), "two", true}),
		},
		{
			name:      "ObjectField",
			fieldType: definition.FieldTypeObject,
			defaultValue: MustNewLiteralValueStrict(map[string]any{"key": "value"}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := definition.Field{
				Name: definition.FieldName(tc.name),
				FieldProperties: definition.FieldProperties{
					Type:    tc.fieldType,
					Default: tc.defaultValue,
				},
			}

			data, err := json.Marshal(f)
			require.NoError(t, err)

			var unmarshaledF definition.Field
			err = json.Unmarshal(data, &unmarshaledF)
			require.NoError(t, err)

			assert.Equal(t, f.Name, unmarshaledF.Name)
			assert.Equal(t, f.Type, unmarshaledF.Type)
			require.NotNil(t, unmarshaledF.Default)
			assert.Equal(t, tc.defaultValue.Value(), unmarshaledF.Default.Value())
		})
	}
}

func TestField_UnmarshalJSON_ErrorCases(t *testing.T) {
	// Invalid JSON for the Field struct itself
	t.Run("InvalidFieldJSON", func(t *testing.T) {
		jsonStr := `{ "name": "field", "type": "string", "default": 123` // Missing closing brace
		var f definition.Field
		err := json.Unmarshal([]byte(jsonStr), &f)
		require.Error(t, err)
	})

	// Invalid JSON for the Default LiteralValue (e.g., expecting a string but getting an object)
	t.Run("InvalidDefaultLiteralValue", func(t *testing.T) {
		jsonStr := `{ "name": "field", "type": "string", "default": {"key": "value"} }`
		var f definition.Field
		err := json.Unmarshal([]byte(jsonStr), &f)
		require.NoError(t, err) // Unmarshal itself will not error here, but LiteralValueAs would fail
		require.NotNil(t, f.Default)
		_, err = definition.LiteralValueAs[string](f.Default)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type mismatch")
	})

	// Invalid JSON for the Schema FieldSchemaReference
	t.Run("InvalidSchemaReference", func(t *testing.T) {
		jsonStr := `{ "name": "field", "type": "object", "schema": "not_an_object" }` // Schema expects object/array
		var f definition.Field
		err := json.Unmarshal([]byte(jsonStr), &f)
		require.Error(t, err)
		var sysErr *common.SystemError
		if errors.As(err, &sysErr) {
			assert.NotNil(t, sysErr)
		} else {
			assert.Fail(t, "Error was not a *common.SystemError")
		}
		assert.Equal(t, "ERR_SCHEMA_UNMARSHAL_FAILED", sysErr.Code)
	})
}
