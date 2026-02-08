package definition

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/stretchr/testify/assert"
)

func TestSchema_DeepCopy(t *testing.T) {
	// Create a sample schema
	original := &Schema{
		Version: *common.MustNewVersion("1.0.0"),
		BaseSchema: BaseSchema{
			Name:        "TestSchema",
			Description: "A schema for testing deep copy",
			Fields: map[FieldId]Field{
				"field1": {
					Name:        "fieldOne",
					Description: "This is the first field",
					FieldProperties: FieldProperties{
						Type: FieldTypeString,
					},
				},
			},
			Constraints: map[ConstraintId]Constraint{
				"constraint1": {
					Name: "constraintOne",
					ConstraintUnion: NewConstrainUnion(&ConstraintRule{
						Predicate: "len",
						Parameters: MustNewLiteralValue(map[string]any{
							"min": 1,
							"max": 10,
						}),
						Fields: []FieldName{"field1"},
					}),
				},
			},
			Indexes: map[IndexId]Index{
				"index1": {
					Name:   "indexOne",
					Type:   IndexTypeUnique,
					Fields: []FieldId{"field1"},
				},
			},
			Metadata: map[string]any{
				"meta1": "value1",
				"meta2": map[string]any{"nested": "value2"},
			},
		},
		Schemas: map[SchemaId]NestedSchema{
			"nested1": {
				BaseSchema: BaseSchema{
					Name: "NestedSchema",
				},
			},
		},
	}

	// Create a deep copy
	copied := original.DeepCopy()

	// 1. Check for nil pointers
	assert.NotNil(t, copied)
	assert.NotNil(t, copied.Fields)
	assert.NotNil(t, copied.Constraints)
	assert.NotNil(t, copied.Indexes)
	assert.NotNil(t, copied.Metadata)
	assert.NotNil(t, copied.Schemas)

	// 2. Check for deep equality by comparing JSON output
	originalJSON, err := json.Marshal(original)
	assert.NoError(t, err)
	copiedJSON, err := json.Marshal(copied)
	assert.NoError(t, err)
	assert.JSONEq(t, string(originalJSON), string(copiedJSON), "JSON output should be equivalent")

	// 3. Check that it's a different instance (pointers should not be the same)
	assert.NotSame(t, original, copied)

	// 4. Modify the copy and check if the original is affected
	copied.Name = "ModifiedSchema"
	field1 := copied.Fields["field1"]
	field1.Name = "modifiedField"
	copied.Fields["field1"] = field1
	copied.Metadata["meta1"] = "modifiedValue"

	// Modify slice in constraint
	constraint1 := copied.Constraints["constraint1"]
	rule, _ := ConstraintAs[*ConstraintRule](constraint1.ConstraintUnion)
	rule.Fields[0] = "modifiedField"

	assert.Equal(t, "TestSchema", original.Name)
	assert.Equal(t, FieldName("fieldOne"), original.Fields["field1"].Name)
	assert.Equal(t, "value1", original.Metadata["meta1"])

	originalRule, _ := ConstraintAs[*ConstraintRule](original.Constraints["constraint1"].ConstraintUnion)
	assert.Equal(t, FieldName("field1"), originalRule.Fields[0])
}

func TestSchema_DeepCopy_Complex(t *testing.T) {
	original := &Schema{
		Version: *common.MustNewVersion("2.1.3"),
		BaseSchema: BaseSchema{
			Name: "ComplexSchema",
			Fields: map[FieldId]Field{
				"f1": {
					Name: "fieldOne",
					FieldProperties: FieldProperties{
						Type:    FieldTypeArray,
						Default: MustNewLiteralValue([]any{"a", "b"}),
						Schema: NewSchemaReference(SchemaReference{
							ID: "nestedSchema",
						}),
					},
				},
			},
			Constraints: map[ConstraintId]Constraint{
				"cg1": {
					Name: "constraintGroup",
					ConstraintUnion: NewConstrainUnion(&ConstraintGroup{
						Operator: common.LogicalAnd, // Corrected logical operator
						Rules: []ConstraintUnion{
							NewConstrainUnion(&ConstraintRule{
								Predicate: "dummy",
							}),
						},
					}),
				},
			},
			Indexes: map[IndexId]Index{
				"idx1": {
					Name: "complexIndex",
					Condition: NewIndexConditionUnion(&IndexConditionGroup{
						Operator: common.LogicalOr, // Corrected logical operator
						Conditions: []IndexConditionUnion{ // Corrected type from IndexUnion to IndexConditionUnion
							NewIndexConditionUnion(&IndexCondition{ // Corrected constructor from NewIndexUnion to NewIndexConditionUnion
								Field:    "f1",
								Operator: common.Equal, // Corrected comparison operator
								Value:    MustNewLiteralValue("test"),
							}),
						},
					}),
				},
			},
		},
		Schemas: map[SchemaId]NestedSchema{
			"nestedSchema": {
				BaseSchema: BaseSchema{Name: "Nested"},
			},
		},
	}

	copied := original.DeepCopy()

	// Check for deep equality via JSON marshaling
	originalJSON, err := json.Marshal(original)
	assert.NoError(t, err)
	copiedJSON, err := json.Marshal(copied)
	assert.NoError(t, err)
	assert.JSONEq(t, string(originalJSON), string(copiedJSON))

	// Modify nested parts of the copy and check original
	// 1. Modify slice in default value
	f1 := copied.Fields["f1"]
	def, _ := f1.Default.Value().([]any)
	def[0] = "c"

	origF1 := original.Fields["f1"]
	origDef, _ := origF1.Default.Value().([]any)
	assert.Equal(t, "a", origDef[0], "Original default slice should not be modified")

	// 2. Modify constraint group
	cg1 := copied.Constraints["cg1"]
	cg, _ := ConstraintAs[*ConstraintGroup](cg1.ConstraintUnion)
	cr, _ := ConstraintAs[*ConstraintRule](cg.Rules[0])
	cr.Predicate = "changed"

	origCg1 := original.Constraints["cg1"]
	origCg, _ := ConstraintAs[*ConstraintGroup](origCg1.ConstraintUnion)
	origCr, _ := ConstraintAs[*ConstraintRule](origCg.Rules[0])
	assert.Equal(t, PredicateName("dummy"), origCr.Predicate, "Original constraint group should not be modified")

	// 3. Modify index condition
	idx1 := copied.Indexes["idx1"]
	icg, err := IndexConditionAs[*IndexConditionGroup](idx1.Condition)
	assert.NoError(t, err)
	ic, err := IndexConditionAs[*IndexCondition](icg.Conditions[0])
	assert.NoError(t, err)
	ic.Operator = common.NotEqual // Corrected comparison operator

	origIdx1 := original.Indexes["idx1"]
	origIcg, err := IndexConditionAs[*IndexConditionGroup](origIdx1.Condition)
	assert.NoError(t, err)
	origIc, err := IndexConditionAs[*IndexCondition](origIcg.Conditions[0])
	assert.NoError(t, err)
	assert.Equal(t, common.Equal, origIc.Operator, "Original index condition should not be modified")

	// 4. Modify nested schema reference
	f1SchemaRef, err := FieldSchemaAs[SchemaReference](f1.Schema)
	assert.NoError(t, err)
	f1SchemaRef.ID = "changed"

	origF1SchemaRef, err := FieldSchemaAs[SchemaReference](origF1.Schema)
	assert.NoError(t, err)
	assert.Equal(t, SchemaId("nestedSchema"), origF1SchemaRef.ID, "Original schema reference should not be modified")
}
