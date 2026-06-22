package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchema_ToJSONVsEncodingJSON(t *testing.T) {
	// Reusing a complex test schema from definition_test.go
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
						Type:   definition.FieldTypeObject,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "3cc51bb6-92d1-4dad-bb2f-d7c21db1a0a5"}), // UUID for AddressSchema
					},
				},
				"7d8a65b2-0274-47b8-8496-4447c26ec7ec": { // UUID for "address"
					Name: "another",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeObject,
						Schema: definition.NewSchemaReference([]definition.SchemaReference{
							{ID: "3cc51bb6-92d1-4dad-bb2f-d7c21db1a0a5"},
							{ID: "695fc841-1e4f-4835-843c-8be0c5a8bb08"},
						}), // UUID for AddressSchema
					},
				},
			},
			Indexes: map[definition.IndexId]definition.Index{
				definition.IndexId("77117b14-13e7-4dcf-91ba-f5f1f36adafb"): { // UUID for "idx_name"
					Name: "idx_name",
					Type: definition.IndexTypeNormal,
					Fields: []definition.FieldId{
						"8ffb9dff-e32a-4d67-8eb3-da9aa7d4941e", // UUID for "name" field
					},
				},
			},
			Constraints: map[definition.ConstraintId]definition.Constraint{
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
		Version: common.MustNewVersion("1.0.0"), // Version is a field of Schema, not BaseSchema
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			"695fc841-1e4f-4835-843c-8be0c5a8bb08": {
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
			"IndexOrderEnum": {
				BaseSchema: definition.BaseSchema{Name: "IndexOrderEnum"},
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
				Values: []definition.LiteralValue{
					definition.MustNewLiteralValue("asc"),
					definition.MustNewLiteralValue("desc"),
				},
			},
			"3cc51bb6-92d1-4dad-bb2f-d7c21db1a0a5": { // UUID for AddressSchema
				BaseSchema: definition.BaseSchema{
					Name: "AddressSchema",
					Fields: map[definition.FieldId]definition.Field{
						"87a1d771-db5e-4ea9-8374-dbabbe0ce946": { // UUID for "street"
							Name: "street",
							FieldProperties: definition.FieldProperties{
								Type: definition.FieldTypeString,
							},
						},
						"a5e1be82-c8f3-443f-8332-592c310aecab": { // UUID for "zip"
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

	// Determine max nesting depth of the schema for optimizing contextStack capacity
	maxDepth := 0
	_, _ = testSchema.Walk(nil, func(acc any, ctx *definition.NodeContext) (any, error) {
		if ctx.Depth > maxDepth {
			maxDepth = ctx.Depth
		}
		return acc, nil
	})
	t.Logf("Max schema nesting depth: %d", maxDepth)

	// 1. Generate JSON using the custom ToJSON method
	customJSON := testSchema.ToJSON()
	t.Logf("Custom JSON:\n%s", string(customJSON))

	// 2. Generate JSON using encoding/json.Marshal
	stdJSON, err := json.Marshal(testSchema)
	require.NoError(t, err)
	t.Logf("Standard JSON:\n%s", string(stdJSON))

	// 3. Compare outputs
	assert.JSONEq(t, string(stdJSON), string(customJSON), "Custom ToJSON output should match standard encoding/json output")
}

