package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchema_MarshalUnmarshalJSON(t *testing.T) {
	// Example of a complex schema to test round-tripping
	testSchema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name:        "UserProfileSchema",
			Description: "Defines the structure for user profiles",
			Fields: map[definition.FieldId]definition.Field{
				"8ffb9dff-e32a-4d67-8eb3-da9aa7d4941e": { // UUID for "name"
					Name:     "name",
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeString,
					},
				},
				"50f9ff0f-de26-464f-9f3b-8384172d4d07": { // UUID for "age"
					Name: "age",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeInteger,
					},
				},
				"e24d49a9-a904-4a84-8d08-52cac8115098": { // UUID for "address"
					Name: "address",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeObject,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "3cc51bb6-92d1-4dad-bb2f-d7c21db1a0a5"}), // UUID for AddressSchema
					},
				},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"77117b14-13e7-4dcf-91ba-f5f1f36adafb": { // UUID for "idx_name"
					Name: "idx_name",
					Type: definition.IndexTypeNormal,
					Fields: []definition.FieldId{
						"8ffb9dff-e32a-4d67-8eb3-da9aa7d4941e", // UUID for "name" field
					},
				},
			},
			Constraints: map[definition.ConstraintId]definition.Constraint{ // Uncommented
				"33c8b8f5-1ab2-433a-80bc-211000f47ba3": { // UUID for "age_range"
					Name: "age_range",
					ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
						Fields:    []definition.FieldName{"50f9ff0f-de26-464f-9f3b-8384172d4d07"}, // UUID for "age" field
						Predicate: "range",
						Parameters: func() definition.LiteralValue {
							lv, _ := definition.NewLiteralValue(map[string]any{"min": int64(0), "max": int64(150)})
							return lv
						}(),
					}),
				},
			},
			Metadata: map[string]any{
				"author": "Test Author",
			},
		},
		Version: *common.MustNewVersion("1.0.0"), // Version is a field of Schema, not BaseSchema
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			"3cc51bb6-92d1-4dad-bb2f-d7c21db1a0a5": { // UUID for AddressSchema
				BaseSchema: definition.BaseSchema{
					Name: "AddressSchema",
					Fields: map[definition.FieldId]definition.Field{
						"1ebc9a37-d39a-4a59-9756-e671916aec62": { // UUID for "street"
							Name: "street",
							FieldProperties: definition.FieldProperties{
								Type: definition.FieldTypeString,
							},
						},
						"5fe0f9dd-565f-48cd-8939-abd65e2f8553": { // UUID for "zip"
							Name: "zip",
							FieldProperties: definition.FieldProperties{
								Type: definition.FieldTypeString,
							},
							Required: true, // Add a required field to ensure it's marshaled
						},
					},
				},
			},
		},
	}

	// Marshal the schema
	marshaledData, err := json.MarshalIndent(testSchema, "", "  ")
	require.NoError(t, err)

	t.Logf("Marshaled Schema: %s", string(marshaledData))

	// Unmarshal the schema back into a new struct
	var unmarshaledSchema definition.Schema
	err = json.Unmarshal(marshaledData, &unmarshaledSchema)
	require.NoError(t, err)

	// Assert equality
	assert.Equal(t, testSchema.Name, unmarshaledSchema.Name)
	assert.Equal(t, testSchema.Description, unmarshaledSchema.Description)
	assert.Equal(t, testSchema.Version, unmarshaledSchema.Version)
	assert.Equal(t, testSchema.Metadata["author"], unmarshaledSchema.Metadata["author"])

	// Fields comparison - Field IDs are UUIDs generated on the fly, so direct map comparison won't work if they change.
	// We need to compare field properties by their Name, assuming Name is unique within Fields.
	require.Len(t, unmarshaledSchema.Fields, len(testSchema.Fields))
	for _, expectedField := range testSchema.Fields {
		var actualField *definition.Field
		for _, uf := range unmarshaledSchema.Fields {
			if uf.Name == expectedField.Name {
				actualField = &uf
				break
			}
		}
		require.NotNil(t, actualField, "Field %s not found in unmarshaled schema", expectedField.Name)
		assert.Equal(t, expectedField.Name, actualField.Name)
		assert.Equal(t, expectedField.Required, actualField.Required)
		assert.Equal(t, expectedField.FieldProperties.Type, actualField.FieldProperties.Type)
		// More assertions for nested FieldProperties might be needed
	}

	// Indexes comparison
	require.Len(t, unmarshaledSchema.Indexes, len(testSchema.Indexes))
	for _, expectedIndex := range testSchema.Indexes {
		var actualIndex *definition.Index
		for _, ui := range unmarshaledSchema.Indexes {
			if ui.Name == expectedIndex.Name {
				actualIndex = &ui
				break
			}
		}
		require.NotNil(t, actualIndex, "Index %s not found in unmarshaled schema", expectedIndex.Name)
		assert.Equal(t, expectedIndex.Name, actualIndex.Name)
		assert.Equal(t, expectedIndex.Type, actualIndex.Type)
		assert.Equal(t, expectedIndex.Fields, actualIndex.Fields)
	}

	// Constraints comparison (UNCOMMENTED)
	require.Len(t, unmarshaledSchema.Constraints, len(testSchema.Constraints))
	// For "age_range" constraint
	expectedConstraint := testSchema.Constraints["33c8b8f5-1ab2-433a-80bc-211000f47ba3"] // UUID for "age_range"
	actualConstraint := unmarshaledSchema.Constraints["33c8b8f5-1ab2-433a-80bc-211000f47ba3"] // UUID for "age_range"
	assert.Equal(t, expectedConstraint.Name, actualConstraint.Name)
	assert.Equal(t, expectedConstraint.Description, actualConstraint.Description)
	assert.Equal(t, expectedConstraint.ConstraintUnion.Kind(), actualConstraint.ConstraintUnion.Kind())

	expectedRule, err := definition.ConstraintAs[*definition.ConstraintRule](expectedConstraint.ConstraintUnion)
	require.NoError(t, err)
	actualRule, err := definition.ConstraintAs[*definition.ConstraintRule](actualConstraint.ConstraintUnion)
	require.NoError(t, err)
	assert.Equal(t, expectedRule.Fields, actualRule.Fields)
	assert.Equal(t, expectedRule.Predicate, actualRule.Predicate)
	assert.Equal(t, expectedRule.Parameters.Value(), actualRule.Parameters.Value())

	// Nested Schemas comparison
	require.Len(t, unmarshaledSchema.Schemas, len(testSchema.Schemas))
	expectedAddressSchema := testSchema.Schemas["3cc51bb6-92d1-4dad-bb2f-d7c21db1a0a5"] // UUID for AddressSchema
	actualAddressSchema := unmarshaledSchema.Schemas["3cc51bb6-92d1-4dad-bb2f-d7c21db1a0a5"] // UUID for AddressSchema

	assert.Equal(t, expectedAddressSchema.Name, actualAddressSchema.Name)
	require.Len(t, actualAddressSchema.Fields, len(expectedAddressSchema.Fields))
	// Similar to top-level fields, compare by Name for nested fields
	for _, expectedNestedField := range expectedAddressSchema.Fields {
		var actualNestedField *definition.Field
		for _, unf := range actualAddressSchema.Fields {
			if unf.Name == expectedNestedField.Name {
				actualNestedField = &unf
				break
			}
		}
		require.NotNil(t, actualNestedField, "Nested Field %s not found in unmarshaled schema", expectedNestedField.Name)
		assert.Equal(t, expectedNestedField.Name, actualNestedField.Name)
		assert.Equal(t, expectedNestedField.Required, actualNestedField.Required)
		assert.Equal(t, expectedNestedField.FieldProperties.Type, actualNestedField.FieldProperties.Type)
	}
}

func TestSchema_EmptyFieldsOmitted(t *testing.T) {
	schema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name:    "EmptyTest",
		},
		Version: *common.MustNewVersion("1.0.0"), // Version is directly on Schema
		Schemas: make(map[definition.SchemaId]definition.NestedSchema), // Explicitly initialize empty map
	}

	marshaled, err := json.Marshal(schema)
	require.NoError(t, err)

	expectedJSON := `{"name":"EmptyTest","version":"1.0.0"}` // Corrected expectedJSON
	assert.JSONEq(t, expectedJSON, string(marshaled))
}

func TestNestedSchema_UnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"name": "NestedObject",
		"type": "object",
		"fields": {
			"nestedField1": {
				"name": "nestedField1",
				"type": "string"
			}
		}
	}`

	var ns definition.NestedSchema
	err := json.Unmarshal([]byte(jsonStr), &ns)
	require.NoError(t, err)

	assert.Equal(t, "NestedObject", ns.Name)
	assert.Equal(t, definition.FieldTypeObject, ns.FieldProperties.Type)
	require.NotNil(t, ns.Fields)
	assert.Equal(t, definition.FieldName("nestedField1"), ns.Fields["nestedField1"].Name)
	assert.Equal(t, definition.FieldTypeString, ns.Fields["nestedField1"].FieldProperties.Type)
}

func TestNestedSchema_UnmarshalJSON_ElementType(t *testing.T) {
	jsonStr := `{
		"type": "string",
		"values": ["a", "b"]
	}`

	var ns definition.NestedSchema
	err := json.Unmarshal([]byte(jsonStr), &ns)
	require.NoError(t, err)

	assert.Equal(t, definition.FieldTypeString, ns.FieldProperties.Type)
	require.NotNil(t, ns.Values)
	assert.Len(t, ns.Values, 2)
	assert.Equal(t, "a", ns.Values[0].Value())
	assert.Equal(t, "b", ns.Values[1].Value())
}
