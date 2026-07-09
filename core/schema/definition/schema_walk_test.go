package definition_test

import (
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchema_Walk(t *testing.T) {
	s := &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "test_schema",
			Fields: map[definition.FieldId]definition.Field{
				"field1": {
					Name: "field one",
					FieldProperties: definition.FieldProperties{
						Type:    definition.FieldTypeString,
						Default: definition.MustNewLiteralValue("default_val"),
					},
				},
				"field2": {
					Name: "field two",
					FieldProperties: definition.FieldProperties{
						Type:   definition.FieldTypeInteger,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: "nested1"}),
					},
				},
			},
			Constraints: map[definition.ConstraintId]definition.Constraint{
				"root_constraint": {
					Name: "root constraint",
					ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintGroup{
						Operator: common.LogicalAnd,
						Rules: []definition.ConstraintUnion{
							definition.NewConstrainUnion(&definition.ConstraintRule{
								Predicate:  "min",
								Parameters: definition.MustNewLiteralValue(int64(1)),
							}),
						},
					}),
				},
			},
			Indexes: map[definition.IndexID]definition.Index{
				"index1": {
					Name:   "index one",
					Type:   definition.IndexTypeUnique,
					Fields: []definition.FieldName{"field1"},
					Condition: definition.NewIndexConditionUnion(&definition.IndexCondition{
						Field:    "field2",
						Operator: common.Equal,
						Value:    definition.MustNewLiteralValue(int64(10)),
					}),
				},
				"index2": {
					Name:   "index two",
					Type:   definition.IndexTypeNormal,
					Fields: []definition.FieldName{"field2"},
					Condition: definition.NewIndexConditionUnion(&definition.IndexConditionGroup{
						Operator: common.LogicalOr,
						Conditions: []definition.IndexConditionUnion{
							definition.NewIndexConditionUnion(&definition.IndexCondition{
								Field:    "field1",
								Operator: common.NotEqual,
								Value:    definition.MustNewLiteralValue("abc"),
							}),
						},
					}),
				},
			},
			Metadata: map[string]any{
				"meta1": "value1",
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			"nested1": {
				BaseSchema: definition.BaseSchema{
					Name: "nested schema one",
					Fields: map[definition.FieldId]definition.Field{
						"nested_field1": {
							Name: "nested field one",
							FieldProperties: definition.FieldProperties{
								Type: definition.FieldTypeBoolean,
							},
						},
					},
					Constraints: map[definition.ConstraintId]definition.Constraint{
						"nested_constraint": {
							Name: "nested constraint",
							ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
								Predicate: "present",
								Fields:    []definition.FieldName{"nested_field1"},
							}),
						},
					},
				},
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeObject,
				},
			},
			"enum_schema": {
				BaseSchema: definition.BaseSchema{
					Name: "enum schema",
				},
				FieldProperties: definition.FieldProperties{Type: definition.FieldTypeEnum},
				Values: []definition.LiteralValue{
					definition.MustNewLiteralValue("val1"),
					definition.MustNewLiteralValue("val2"),
				},
			},
		},
	}

	var visitedPaths []string
	walker := func(acc any, ctx *definition.NodeContext) (any, error) {
		path := strings.Join(ctx.GetPath(), ".")
		visitedPaths = append(visitedPaths, path)
		return acc, nil
	}

	_, err := s.Walk(nil, walker)
	require.NoError(t, err)

	expectedPaths := []string{
		"schema",
		"schema.fields",
		"schema.fields.field1",
		"schema.fields.field1.default",
		"schema.fields.field2",
		"schema.fields.field2.schema",
		"schema.schemas",
		"schema.schemas.nested1",
		"schema.schemas.nested1.fields",
		"schema.schemas.nested1.fields.nested_field1",
		"schema.schemas.nested1.constraints",
		"schema.schemas.nested1.constraints.nested_constraint",
		"schema.schemas.nested1.constraints.nested_constraint.rule",
		"schema.schemas.nested1.constraints.nested_constraint.rule.fields",
		"schema.schemas.enum_schema",
		"schema.schemas.enum_schema.values",
		"schema.constraints",
		"schema.constraints.root_constraint",
		"schema.constraints.root_constraint.group",
		"schema.constraints.root_constraint.group.rule",
		"schema.constraints.root_constraint.group.rule.parameters",
		"schema.indexes",
		"schema.indexes.index1",
		"schema.indexes.index1.fields",
		"schema.indexes.index1.condition",
		"schema.indexes.index1.condition.value",
		"schema.indexes.index2",
		"schema.indexes.index2.fields",
		"schema.indexes.index2.conditionGroup",
		"schema.indexes.index2.conditionGroup.condition",
		"schema.indexes.index2.conditionGroup.condition.value",
		"schema.metadata",
	}

	assert.ElementsMatch(t, expectedPaths, visitedPaths)
}
