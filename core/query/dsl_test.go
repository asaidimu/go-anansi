package query

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
)

func TestComparisonOperator_IsStandard(t *testing.T) {
	tests := []struct {
		operator ComparisonOperator
		expected bool
	}{
		{ComparisonOperatorEq, true},
		{ComparisonOperatorNeq, true},
		{ComparisonOperatorLt, true},
		{ComparisonOperatorLte, true},
		{ComparisonOperatorGt, true},
		{ComparisonOperatorGte, true},
		{ComparisonOperatorIn, true},
		{ComparisonOperatorNin, true},
		{ComparisonOperatorContains, true},
		{ComparisonOperatorNotContains, true},
		{ComparisonOperatorStartsWith, true},
		{ComparisonOperatorEndsWith, true},
		{ComparisonOperatorExists, true},
		{ComparisonOperatorNotExists, true},
		{"custom_op", false},
		{"another_custom", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.operator), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.operator.IsStandard())
		})
	}
}

func TestGetStandardComparisonOperators(t *testing.T) {
	operators := GetStandardComparisonOperators()
	assert.NotNil(t, operators)
	assert.NotEmpty(t, operators)

	expectedOperators := []ComparisonOperator{
		ComparisonOperatorEq,
		ComparisonOperatorNeq,
		ComparisonOperatorLt,
		ComparisonOperatorLte,
		ComparisonOperatorGt,
		ComparisonOperatorGte,
		ComparisonOperatorIn,
		ComparisonOperatorNin,
		ComparisonOperatorContains,
		ComparisonOperatorNotContains,
		ComparisonOperatorStartsWith,
		ComparisonOperatorEndsWith,
		ComparisonOperatorExists,
		ComparisonOperatorNotExists,
	}

	assert.Len(t, operators, len(expectedOperators))
	for _, op := range expectedOperators {
		_, ok := operators[op]
		assert.True(t, ok, "Expected operator %s not found in map", op)
	}
}

func TestQueryFilter_Condition(t *testing.T) {
	filter := QueryFilter{
		Condition: &FilterCondition{
			Field:    "name",
			Operator: ComparisonOperatorEq,
			Value:    "test",
		},
	}
	assert.NotNil(t, filter.Condition)
	assert.Nil(t, filter.Group)
	assert.Equal(t, "name", filter.Condition.Field)
}

func TestQueryFilter_Group(t *testing.T) {
	filter := QueryFilter{
		Group: &FilterGroup{
			Operator: schema.LogicalAnd,
			Conditions: []QueryFilter{
				{Condition: &FilterCondition{Field: "age", Operator: ComparisonOperatorGt, Value: 18}},
			},
		},
	}
	assert.NotNil(t, filter.Group)
	assert.Nil(t, filter.Condition)
	assert.Equal(t, schema.LogicalAnd, filter.Group.Operator)
}

func TestProjectionComputedItem_ComputedFieldExpression(t *testing.T) {
	item := ProjectionComputedItem{
		ComputedFieldExpression: &ComputedFieldExpression{
			Type:  "computed",
			Alias: "full_name",
			Expression: &FunctionCall{
				Function:  "CONCAT",
				Arguments: []FilterValue{"first", " ", "last"},
			},
		},
	}
	assert.NotNil(t, item.ComputedFieldExpression)
	assert.Nil(t, item.CaseExpression)
	assert.Equal(t, "full_name", item.ComputedFieldExpression.Alias)
}

func TestProjectionComputedItem_CaseExpression(t *testing.T) {
	item := ProjectionComputedItem{
		CaseExpression: &CaseExpression{
			Type:  "case",
			Alias: "status_text",
			Cases: []CaseCondition{
				{When: QueryFilter{Condition: &FilterCondition{Field: "status", Operator: ComparisonOperatorEq, Value: 1}}, Then: "Active"},
			},
			Else: "Unknown",
		},
	}
	assert.NotNil(t, item.CaseExpression)
	assert.Nil(t, item.ComputedFieldExpression)
	assert.Equal(t, "status_text", item.CaseExpression.Alias)
}
