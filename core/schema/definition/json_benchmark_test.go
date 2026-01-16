package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

var testSchemaForBenchmark = definition.Schema{
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

func BenchmarkSchema_ToJSON(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = testSchemaForBenchmark.ToJSON()
	}
}

func BenchmarkSchema_MarshalJSON(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(testSchemaForBenchmark)
		if err != nil {
			b.Fatal(err)
		}
	}
}
