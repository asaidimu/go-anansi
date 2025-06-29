package query

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v5/core/schema"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewDataProcessor(t *testing.T) {
	p := NewDataProcessor(nil)
	assert.NotNil(t, p)
	assert.NotNil(t, p.goComputeFunctions)
	assert.NotNil(t, p.goFilterFunctions)
	assert.NotNil(t, p.logger)

	p = NewDataProcessor(zap.NewNop())
	assert.NotNil(t, p)
}

func TestDataProcessor_RegisterComputeFunction(t *testing.T) {
	p := NewDataProcessor(nil)
	fn := func(row schema.Document, args FilterValue) (any, error) { return nil, nil }
	p.RegisterComputeFunction("testFunc", fn)
	assert.Contains(t, p.goComputeFunctions, "testFunc")
}

func TestDataProcessor_RegisterFilterFunction(t *testing.T) {
	p := NewDataProcessor(nil)
	fn := func(doc schema.Document, field string, args FilterValue) (bool, error) { return true, nil }
	p.RegisterFilterFunction("customOp", fn)
	assert.Contains(t, p.goFilterFunctions, ComparisonOperator("customOp"))
}

func TestDataProcessor_RegisterComputeFunctions(t *testing.T) {
	p := NewDataProcessor(nil)
	funcs := map[string]ComputeFunction{
		"func1": func(row schema.Document, args FilterValue) (any, error) { return nil, nil },
		"func2": func(row schema.Document, args FilterValue) (any, error) { return nil, nil },
	}
	p.RegisterComputeFunctions(funcs)
	assert.Contains(t, p.goComputeFunctions, "func1")
	assert.Contains(t, p.goComputeFunctions, "func2")
}

func TestDataProcessor_RegisterFilterFunctions(t *testing.T) {
	p := NewDataProcessor(nil)
	funcs := map[ComparisonOperator]PredicateFunction{
		"op1": func(doc schema.Document, field string, args FilterValue) (bool, error) { return true, nil },
		"op2": func(doc schema.Document, field string, args FilterValue) (bool, error) { return true, nil },
	}
	p.RegisterFilterFunctions(funcs)
	assert.Contains(t, p.goFilterFunctions, ComparisonOperator("op1"))
	assert.Contains(t, p.goFilterFunctions, ComparisonOperator("op2"))
}

func TestDataProcessor_DetermineFieldsToSelect(t *testing.T) {
	p := NewDataProcessor(nil)

	t.Run("No projection, no filters", func(t *testing.T) {
		dsl := &QueryDSL{}
		fields := p.DetermineFieldsToSelect(dsl)
		assert.Empty(t, fields)
	})

	t.Run("Include fields in projection", func(t *testing.T) {
		dsl := &QueryDSL{
			Projection: &ProjectionConfiguration{
				Include: []ProjectionField{
					{Name: "field1"},
					{Name: "field2"},
				},
			},
		}
		fields := p.DetermineFieldsToSelect(dsl)
		assert.Len(t, fields, 2)
		assert.Contains(t, fields, ProjectionField{Name: "field1"})
		assert.Contains(t, fields, ProjectionField{Name: "field2"})
	})

	t.Run("Computed fields with string arguments", func(t *testing.T) {
		dsl := &QueryDSL{
			Projection: &ProjectionConfiguration{
				Computed: []ProjectionComputedItem{
					{
						ComputedFieldExpression: &ComputedFieldExpression{
							Expression: &FunctionCall{
								Arguments: []FilterValue{"arg1", 123, "arg2"},
							},
						},
					},
				},
			},
		}
		fields := p.DetermineFieldsToSelect(dsl)
		assert.Len(t, fields, 2)
		assert.Contains(t, fields, ProjectionField{Name: "arg1"})
		assert.Contains(t, fields, ProjectionField{Name: "arg2"})
	})

	t.Run("Non-standard filter functions", func(t *testing.T) {
		dsl := &QueryDSL{
			Filters: &QueryFilter{
				Condition: &FilterCondition{
					Field:    "customField",
					Operator: "custom_op",
					Value:    "value",
				},
			},
		}
		fields := p.DetermineFieldsToSelect(dsl)
		assert.Len(t, fields, 1)
		assert.Contains(t, fields, ProjectionField{Name: "customField"})
	})

	t.Run("Nested filter groups with non-standard operators", func(t *testing.T) {
		dsl := &QueryDSL{
			Filters: &QueryFilter{
				Group: &FilterGroup{
					Operator: schema.LogicalAnd,
					Conditions: []QueryFilter{
						{Condition: &FilterCondition{Field: "fieldA", Operator: ComparisonOperatorEq, Value: "valA"}},
						{Condition: &FilterCondition{Field: "fieldB", Operator: "custom_op_2", Value: 123}},
						{Group: &FilterGroup{
							Operator: schema.LogicalOr,
							Conditions: []QueryFilter{
								{Condition: &FilterCondition{Field: "fieldC", Operator: ComparisonOperatorGt, Value: 10}},
								{Condition: &FilterCondition{Field: "fieldD", Operator: "custom_op_3", Value: true}},
							},
						}},
					},
				},
			},
		}
		fields := p.DetermineFieldsToSelect(dsl)
		assert.Len(t, fields, 2)
		assert.Contains(t, fields, ProjectionField{Name: "fieldB"})
		assert.Contains(t, fields, ProjectionField{Name: "fieldD"})
	})

	t.Run("Combination of projection and filters", func(t *testing.T) {
		dsl := &QueryDSL{
			Filters: &QueryFilter{
				Condition: &FilterCondition{
					Field:    "filterField",
					Operator: "custom_filter_op",
					Value:    "xyz",
				},
			},
			Projection: &ProjectionConfiguration{
				Include: []ProjectionField{
					{Name: "projField1"},
				},
				Computed: []ProjectionComputedItem{
					{
						ComputedFieldExpression: &ComputedFieldExpression{
							Expression: &FunctionCall{
								Arguments: []FilterValue{"computedArg"},
							},
						},
					},
				},
			},
		}
		fields := p.DetermineFieldsToSelect(dsl)
		assert.Len(t, fields, 3)
		assert.Contains(t, fields, ProjectionField{Name: "filterField"})
		assert.Contains(t, fields, ProjectionField{Name: "projField1"})
		assert.Contains(t, fields, ProjectionField{Name: "computedArg"})
	})
}

func TestDataProcessor_ProcessRows(t *testing.T) {
	logger := zap.NewNop()

	t.Run("No filters, no projections", func(t *testing.T) {
		p := NewDataProcessor(logger)
		rows := []schema.Document{{"id": 1}, {"id": 2}}
		dsl := &QueryDSL{}
		processedRows, err := p.ProcessRows(rows, dsl, nil)
		assert.NoError(t, err)
		assert.Equal(t, rows, processedRows)
	})

	t.Run("Standard filters", func(t *testing.T) {
		p := NewDataProcessor(logger)
		rows := []schema.Document{{"id": 1, "age": 25}, {"id": 2, "age": 30}, {"id": 3, "age": 25}}
		dsl := &QueryDSL{
			Filters: &QueryFilter{
				Condition: &FilterCondition{
					Field:    "age",
					Operator: ComparisonOperatorEq,
					Value:    25,
				},
			},
		}
		processedRows, err := p.ProcessRows(rows, dsl, nil)
		assert.NoError(t, err)
		assert.Len(t, processedRows, 2)
		assert.Contains(t, processedRows, schema.Document{"id": 1, "age": 25})
		assert.Contains(t, processedRows, schema.Document{"id": 3, "age": 25})
	})

	t.Run("Custom filter function", func(t *testing.T) {
		p := NewDataProcessor(logger)
		p.RegisterFilterFunction("is_even", func(doc schema.Document, field string, args FilterValue) (bool, error) {
			val, ok := doc[field].(int)
			if !ok { return false, errors.New("not an int") }
			return val%2 == 0, nil
		})
		rows := []schema.Document{{"num": 1}, {"num": 2}, {"num": 3}, {"num": 4}}
		dsl := &QueryDSL{
			Filters: &QueryFilter{
				Condition: &FilterCondition{
					Field:    "num",
					Operator: "is_even",
					Value:    nil,
				},
			},
		}
		processedRows, err := p.ProcessRows(rows, dsl, nil)
		assert.NoError(t, err)
		assert.Len(t, processedRows, 2)
		assert.Contains(t, processedRows, schema.Document{"num": 2})
		assert.Contains(t, processedRows, schema.Document{"num": 4})
	})

	t.Run("Skipped standard operator", func(t *testing.T) {
		p := NewDataProcessor(logger)
		rows := []schema.Document{{"id": 1, "age": 25}, {"id": 2, "age": 30}}
		dsl := &QueryDSL{
			Filters: &QueryFilter{
				Condition: &FilterCondition{
					Field:    "age",
					Operator: ComparisonOperatorEq,
					Value:    25,
				},
			},
		}
		processedRows, err := p.ProcessRows(rows, dsl, []ComparisonOperator{ComparisonOperatorEq})
		assert.NoError(t, err)
		assert.Len(t, processedRows, 2) // No filtering should occur as Eq is skipped
		assert.Contains(t, processedRows, schema.Document{"id": 1, "age": 25})
		assert.Contains(t, processedRows, schema.Document{"id": 2, "age": 30})
	})

	t.Run("Computed fields", func(t *testing.T) {
		p := NewDataProcessor(logger)
		p.RegisterComputeFunction("concat", func(row schema.Document, args FilterValue) (any, error) {
			strArgs, ok := args.([]FilterValue)
			if !ok { return nil, errors.New("args not []FilterValue") }
			var s string
            for _, arg := range strArgs {
                if fieldName, isString := arg.(string); isString {
                    if val, ok := row[fieldName]; ok {
                        if val != nil {
                            s += fmt.Sprintf("%v", val)
                        }
                    } else {
					s += fieldName // It's a literal string, not a field name
                    }
                } else {
                    s += fmt.Sprintf("%v", arg)
                }
            }
            return s, nil
		})
		rows := []schema.Document{{"first": "John", "last": "Doe"}}
		dsl := &QueryDSL{
			Projection: &ProjectionConfiguration{
				Computed: []ProjectionComputedItem{
					{
						ComputedFieldExpression: &ComputedFieldExpression{
							Expression: &FunctionCall{
								Function:  "concat",
								Arguments: []FilterValue{"first", " ", "last"},
							},
							Alias: "fullName",
						},
					},
				},
			},
		}
		processedRows, err := p.ProcessRows(rows, dsl, nil)
		assert.NoError(t, err)
		assert.Len(t, processedRows, 1)
		assert.Equal(t, "John Doe", processedRows[0]["fullName"])
	})

	t.Run("Final projection - Include", func(t *testing.T) {
		p := NewDataProcessor(logger)
		rows := []schema.Document{{"id": 1, "name": "test", "age": 30}}
		dsl := &QueryDSL{
			Projection: &ProjectionConfiguration{
				Include: []ProjectionField{{Name: "id"}, {Name: "name"}},
			},
		}
		processedRows, err := p.ProcessRows(rows, dsl, nil)
		assert.NoError(t, err)
		assert.Len(t, processedRows, 1)
		assert.Equal(t, schema.Document{"id": 1, "name": "test"}, processedRows[0])
	})

	t.Run("Final projection - Exclude", func(t *testing.T) {
		p := NewDataProcessor(logger)
		rows := []schema.Document{{"id": 1, "name": "test", "age": 30}}
		dsl := &QueryDSL{
			Projection: &ProjectionConfiguration{
				Exclude: []ProjectionField{{Name: "age"}},
			},
		}
		processedRows, err := p.ProcessRows(rows, dsl, nil)
		assert.NoError(t, err)
		assert.Len(t, processedRows, 1)
		assert.Equal(t, schema.Document{"id": 1, "name": "test"}, processedRows[0])
	})

	t.Run("Final projection - Include with computed field", func(t *testing.T) {
		p := NewDataProcessor(logger)
		p.RegisterComputeFunction("upper", func(row schema.Document, args FilterValue) (any, error) {
			val, ok := args.([]FilterValue)
			if !ok || len(val) == 0 { return nil, errors.New("args not []FilterValue or empty") }
			fieldName, isString := val[0].(string)
			if !isString { return nil, errors.New("first argument not a string field name") }
			fieldVal, ok := row[fieldName].(string)
			if !ok { return nil, errors.New("field value not a string") }
			return "UPPER_" + fieldVal, nil
		})
		rows := []schema.Document{{"name": "test"}}
		dsl := &QueryDSL{
			Projection: &ProjectionConfiguration{
				Include: []ProjectionField{{Name: "name"}},
				Computed: []ProjectionComputedItem{
					{
						ComputedFieldExpression: &ComputedFieldExpression{
							Expression: &FunctionCall{
								Function:  "upper",
								Arguments: []FilterValue{"name"},
							},
							Alias: "upperName",
						},
					},
				},
			},
		}
		processedRows, err := p.ProcessRows(rows, dsl, nil)
		assert.NoError(t, err)
		assert.Len(t, processedRows, 1)
		assert.Equal(t, schema.Document{"name": "test", "upperName": "UPPER_test"}, processedRows[0])
	})

	t.Run("Final projection - Exclude with computed field", func(t *testing.T) {
		p := NewDataProcessor(logger)
		p.RegisterComputeFunction("upper", func(row schema.Document, args FilterValue) (any, error) {
			val, ok := args.([]FilterValue)
			if !ok || len(val) == 0 { return nil, errors.New("args not []FilterValue or empty") }
			fieldName, isString := val[0].(string)
			if !isString { return nil, errors.New("first argument not a string field name") }
			fieldVal, ok := row[fieldName].(string)
			if !ok { return nil, errors.New("field value not a string") }
			return "UPPER_" + fieldVal, nil
		})
		rows := []schema.Document{{"name": "test", "age": 30}}
		dsl := &QueryDSL{
			Projection: &ProjectionConfiguration{
				Exclude: []ProjectionField{{Name: "age"}},
				Computed: []ProjectionComputedItem{
					{
						ComputedFieldExpression: &ComputedFieldExpression{
							Expression: &FunctionCall{
								Function:  "upper",
								Arguments: []FilterValue{"name"},
							},
							Alias: "upperName",
						},
					},
				},
			},
		}
		processedRows, err := p.ProcessRows(rows, dsl, nil)
		assert.NoError(t, err)
		assert.Len(t, processedRows, 1)
		assert.Equal(t, schema.Document{"name": "test", "upperName": "UPPER_test"}, processedRows[0])
	})

	t.Run("Final projection - Computed field only", func(t *testing.T) {
		p := NewDataProcessor(logger)
		p.RegisterComputeFunction("upper", func(row schema.Document, args FilterValue) (any, error) {
			val, ok := args.([]FilterValue)
			if !ok || len(val) == 0 { return nil, errors.New("args not []FilterValue or empty") }
			fieldName, isString := val[0].(string)
			if !isString { return nil, errors.New("first argument not a string field name") }
			fieldVal, ok := row[fieldName].(string)
			if !ok { return nil, errors.New("field value not a string") }
			return "UPPER_" + fieldVal, nil
		})
		rows := []schema.Document{{"name": "test", "age": 30}}
		dsl := &QueryDSL{
			Projection: &ProjectionConfiguration{
				Computed: []ProjectionComputedItem{
					{
						ComputedFieldExpression: &ComputedFieldExpression{
							Expression: &FunctionCall{
								Function:  "upper",
								Arguments: []FilterValue{"name"},
							},
							Alias: "upperName",
						},
					},
				},
			},
		}
		processedRows, err := p.ProcessRows(rows, dsl, nil)
		assert.NoError(t, err)
		assert.Len(t, processedRows, 1)
		assert.Equal(t, schema.Document{"upperName": "UPPER_test"}, processedRows[0])
	})

	t.Run("Case expression", func(t *testing.T) {
		p := NewDataProcessor(logger)
		rows := []schema.Document{{"status": 1}, {"status": 0}, {"status": 99}}
		dsl := &QueryDSL{
			Projection: &ProjectionConfiguration{
				Computed: []ProjectionComputedItem{
					{
						CaseExpression: &CaseExpression{
							Alias: "statusText",
							Cases: []CaseCondition{
								{When: CreateSimpleFilter("status", ComparisonOperatorEq, 1), Then: "Active"},
								{When: CreateSimpleFilter("status", ComparisonOperatorEq, 0), Then: "Inactive"},
							},
							Else: "Unknown",
						},
					},
				},
			},
		}
		processedRows, err := p.ProcessRows(rows, dsl, nil)
		assert.NoError(t, err)
		assert.Len(t, processedRows, 3)
		assert.Equal(t, "Active", processedRows[0]["statusText"])
		assert.Equal(t, "Inactive", processedRows[1]["statusText"])
		assert.Equal(t, "Unknown", processedRows[2]["statusText"])
	})
}

func TestDataProcessor_Match(t *testing.T) {
	logger := zap.NewNop()
	p := NewDataProcessor(logger)

	t.Run("Nil filters", func(t *testing.T) {
		data := schema.Document{"id": 1}
		match, err := p.Match(context.Background(), nil, data)
		assert.NoError(t, err)
		assert.True(t, match)
	})

	t.Run("Matching condition", func(t *testing.T) {
		filters := &QueryFilter{
			Condition: &FilterCondition{
				Field:    "value",
				Operator: ComparisonOperatorEq,
				Value:    10,
			},
		}
		data := schema.Document{"value": 10}
		match, err := p.Match(context.Background(), filters, data)
		assert.NoError(t, err)
		assert.True(t, match)
	})

	t.Run("Non-matching condition", func(t *testing.T) {
		filters := &QueryFilter{
			Condition: &FilterCondition{
				Field:    "value",
				Operator: ComparisonOperatorEq,
				Value:    10,
			},
		}
		data := schema.Document{"value": 20}
		match, err := p.Match(context.Background(), filters, data)
		assert.NoError(t, err)
		assert.False(t, match)
	})

	t.Run("Matching group - AND", func(t *testing.T) {
		filters := &QueryFilter{
			Group: &FilterGroup{
				Operator: schema.LogicalAnd,
				Conditions: []QueryFilter{
					{Condition: &FilterCondition{Field: "age", Operator: ComparisonOperatorGt, Value: 18}},
					{Condition: &FilterCondition{Field: "city", Operator: ComparisonOperatorEq, Value: "New York"}},
				},
			},
		}
		data := schema.Document{"age": 20, "city": "New York"}
		match, err := p.Match(context.Background(), filters, data)
		assert.NoError(t, err)
		assert.True(t, match)
	})

	t.Run("Non-matching group - AND", func(t *testing.T) {
		filters := &QueryFilter{
			Group: &FilterGroup{
				Operator: schema.LogicalAnd,
				Conditions: []QueryFilter{
					{Condition: &FilterCondition{Field: "age", Operator: ComparisonOperatorGt, Value: 18}},
					{Condition: &FilterCondition{Field: "city", Operator: ComparisonOperatorEq, Value: "New York"}},
				},
			},
		}
		data := schema.Document{"age": 15, "city": "New York"}
		match, err := p.Match(context.Background(), filters, data)
		assert.NoError(t, err)
		assert.False(t, match)
	})

	t.Run("Matching group - OR", func(t *testing.T) {
		filters := &QueryFilter{
			Group: &FilterGroup{
				Operator: schema.LogicalOr,
				Conditions: []QueryFilter{
					{Condition: &FilterCondition{Field: "age", Operator: ComparisonOperatorGt, Value: 18}},
					{Condition: &FilterCondition{Field: "city", Operator: ComparisonOperatorEq, Value: "New York"}},
				},
			},
		}
		data := schema.Document{"age": 15, "city": "New York"}
		match, err := p.Match(context.Background(), filters, data)
		assert.NoError(t, err)
		assert.True(t, match)
	})

	t.Run("Unregistered custom operator", func(t *testing.T) {
		filters := &QueryFilter{
			Condition: &FilterCondition{
				Field:    "field",
				Operator: "unregistered_op",
				Value:    "value",
			},
		}
		data := schema.Document{"field": "value"}
		_, err := p.Match(context.Background(), filters, data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unregistered Go filter function for operator: unregistered_op")
	})
}
