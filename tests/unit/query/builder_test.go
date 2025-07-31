package query_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/asaidimu/go-anansi/v6/core/logical"
	"github.com/asaidimu/go-anansi/v6/core/query"
)


func TestNewQueryBuilder(t *testing.T) {
	qb := query.NewQueryBuilder()
	assert.NotNil(t, qb, "NewQueryBuilder should return a non-nil QueryBuilder")
	assert.Equal(t, query.Query{}, qb.Build(), "New QueryBuilder should have an empty query.Query")
}

func TestQueryBuilder_FilterCondition(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.Where("age").Eq(30)

	expected := query.Query{
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "age",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{NumberVal: floatPtr(30)},
			},
		},
	}

	assert.Equal(t, expected, qb.Build(), "Filter condition should be correctly set")
}

func TestQueryBuilder_FilterGroup(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.WhereGroup(logical.LogicalAnd).
		Where("age").Gte(18).
		Where("status").Eq("active").
		End()

	expected := query.Query{
		Filters: &query.QueryFilter{
			Group: &query.FilterGroup{
				Operator: logical.LogicalAnd,
				Conditions: []query.QueryFilter{
					{
						Condition: &query.FilterCondition{
							Field:    "age",
							Operator: query.ComparisonOperatorGte,
							Value:    query.FilterValue{NumberVal: floatPtr(18)},
						},
					},
					{
						Condition: &query.FilterCondition{
							Field:    "status",
							Operator: query.ComparisonOperatorEq,
							Value:    query.FilterValue{StringVal: stringPtr("active")},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, expected, qb.Build(), "Filter group should be correctly set")
}

func TestQueryBuilder_TextSearch(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.TextSearch("name").Contains("John")

	expected := query.Query{
		Filters: &query.QueryFilter{
			TextSearchQuery: &query.TextSearchQuery{
				Query:  "John",
				Fields: []string{"name"},
				Type:   query.TextSearchTypeContains,
			},
		},
	}

	assert.Equal(t, expected, qb.Build(), "Text search query should be correctly set")
}

func TestQueryBuilder_Sorting(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.OrderByAsc("name").ThenSortByDesc("age")

	expected := query.Query{
		Sort: []query.SortConfiguration{
			{Field: "name", Direction: query.SortDirectionAsc},
			{Field: "age", Direction: query.SortDirectionDesc},
		},
	}

	assert.Equal(t, expected, qb.Build(), "Sort configurations should be correctly set")
}

func TestQueryBuilder_Pagination(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.Limit(10).Offset(20)

	expected := query.Query{
		Pagination: &query.PaginationOptions{
			Type:   "offset",
			Limit:  10,
			Offset: intPtr(20),
		},
	}

	assert.Equal(t, expected, qb.Build(), "Pagination options should be correctly set")
}

func TestQueryBuilder_Projection(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.Select().Include("name", "age").Exclude("password").End()

	expected := query.Query{
		Projection: &query.ProjectionConfiguration{
			Include: []query.ProjectionField{
				{Name: "name"},
				{Name: "age"},
			},
			Exclude: []query.ProjectionField{
				{Name: "password"},
			},
		},
	}

	assert.Equal(t, expected, qb.Build(), "Projection configuration should be correctly set")
}

func TestQueryBuilder_ProjectionWithComputed(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.Select().
		AddComputed("full_name", "concat", "first_name", " ", "last_name").
		AddCase("status_category").
		When(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "status",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{StringVal: stringPtr("active")},
			},
		}, "positive").
		Else("neutral").
		End().
		End()

	expected := query.Query{
		Projection: &query.ProjectionConfiguration{
			Computed: []query.ProjectionComputedItem{
				{
					ComputedFieldExpression: &query.ComputedFieldExpression{
						Type: "computed_field",
						Expression: &query.FunctionCall{
							Function: "concat",
							Arguments: []query.FilterValue{
								{StringVal: stringPtr("first_name")},
								{StringVal: stringPtr(" ")},
								{StringVal: stringPtr("last_name")},
							},
						},
						Alias: "full_name",
					},
				},
				{
					CaseExpression: &query.CaseExpression{
						Type: "case",
						Conditions: []query.CaseCondition{
							{
								When: query.QueryFilter{
									Condition: &query.FilterCondition{
										Field:    "status",
										Operator: query.ComparisonOperatorEq,
										Value:    query.FilterValue{StringVal: stringPtr("active")},
									},
								},
								Then: query.FilterValue{StringVal: stringPtr("positive")},
							},
						},
						Else:  query.FilterValue{StringVal: stringPtr("neutral")},
						Alias: "status_category",
					},
				},
			},
		},
	}

	assert.Equal(t, expected, qb.Build(), "Projection with computed fields and case expressions should be correctly set")
}

func TestQueryBuilder_Join(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.InnerJoin("orders").
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "users.id",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Type: "field", Field: "orders.user_id"}},
			},
		}).
		Alias("o").
		End()

	expected := query.Query{
		Joins: []query.JoinConfiguration{
			{
				Type: query.JoinTypeInner,
				Target: "orders",
				On: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "users.id",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Type: "field", Field: "orders.user_id"}},
					},
				},
				Alias: stringPtr("o"),
			},
		},
	}

	assert.Equal(t, expected, qb.Build(), "Join configuration should be correctly set")
}

func TestQueryBuilder_Aggregation(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.Aggregate(query.AggregationTypeCount, "id", "total_count").
		GroupBy("status").
		AddGroup("category").
		WithFilter(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "age",
				Operator: query.ComparisonOperatorGte,
				Value:    query.FilterValue{NumberVal: floatPtr(18)},
			},
		}).
		End()

	expected := query.Query{
		Aggregations: []query.AggregationConfiguration{
			{
				Type:  query.AggregationTypeCount,
				Field: "id",
				Alias: stringPtr("total_count"),
			},
			{
				Groups: []string{"status", "category"},
				Filter: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "age",
						Operator: query.ComparisonOperatorGte,
						Value:    query.FilterValue{NumberVal: floatPtr(18)},
					},
				},
			},
		},
	}

	assert.Equal(t, expected, qb.Build(), "Aggregation configuration should be correctly set")
}

func TestQueryBuilder_Distinct(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.DistinctBy("name", "email")

	expected := query.Query{
		Distinct: &query.QueryDistinctConfig{
			Fields: []string{"name", "email"},
		},
	}

	assert.Equal(t, expected, qb.Build(), "Distinct configuration should be correctly set")
}

func TestQueryBuilder_Union(t *testing.T) {
	qb1 := query.NewQueryBuilder().Where("age").Gte(18)
	qb2 := query.NewQueryBuilder().Where("status").Eq("active")
	qb1.Union(qb2.Build())

	expected := query.Query{
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "age",
				Operator: query.ComparisonOperatorGte,
				Value:    query.FilterValue{NumberVal: floatPtr(18)},
			},
		},
		Union: &query.QueryUnion{
			Queries: []query.Query{
				{
					Filters: &query.QueryFilter{
						Condition: &query.FilterCondition{
							Field:    "age",
							Operator: query.ComparisonOperatorGte,
							Value:    query.FilterValue{NumberVal: floatPtr(18)},
						},
					},
				},
				{
					Filters: &query.QueryFilter{
						Condition: &query.FilterCondition{
							Field:    "status",
							Operator: query.ComparisonOperatorEq,
							Value:    query.FilterValue{StringVal: stringPtr("active")},
						},
					},
				},
			},
			Type: "union",
		},
	}

	assert.Equal(t, expected, qb1.Build(), "Union configuration should be correctly set")
}

func TestQueryBuilder_Hints(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.UseIndex("idx_name").MaxExecutionTime(30)

	expected := query.Query{
		Hints: []query.QueryHint{
			{
				"type":  "use_index",
				"index": "idx_name",
			},
			{
				"type":    "max_execution_time",
				"seconds": 30,
			},
		},
	}

	assert.Equal(t, expected, qb.Build(), "Query hints should be correctly set")
}

func TestQueryBuilder_Clone(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.Where("age").Eq(30).OrderByAsc("name")
	cloned := qb.Clone()

	// Modify original to ensure clone is independent
	qb.Where("status").Eq("active")

	original := qb.Build()
	clonedQuery := cloned.Build()

	expectedCloned := query.Query{
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "age",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{NumberVal: floatPtr(30)},
			},
		},
		Sort: []query.SortConfiguration{
			{Field: "name", Direction: query.SortDirectionAsc},
		},
	}

	assert.Equal(t, expectedCloned, clonedQuery, "Cloned query should be independent and retain original state")
	assert.NotEqual(t, original, clonedQuery, "Cloned query should not reflect changes to original")
}

func TestQueryBuilder_Validate(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*query.QueryBuilder)
		expected query.QueryValidationResult
	}{
		{
			name: "Valid query",
			setup: func(qb *query.QueryBuilder) {
				qb.Where("age").Gte(18).Limit(10).OrderByAsc("name")
			},
			expected: query.QueryValidationResult{
				Valid:  true,
				Errors: []string(nil),
			},
		},
		{
			name: "Invalid filter field",
			setup: func(qb *query.QueryBuilder) {
				qb.Where("").Eq(30)
			},
			expected: query.QueryValidationResult{
				Valid: false,
				Errors: []string{
					"filter condition field cannot be empty",
				},
			},
		},
		{
			name: "Invalid sort direction",
			setup: func(qb *query.QueryBuilder) {
				qb.OrderBy("name", "invalid")
			},
			expected: query.QueryValidationResult{
				Valid: false,
				Errors: []string{
					"invalid sort direction: invalid",
				},
			},
		},
		{
			name: "Invalid pagination limit",
			setup: func(qb *query.QueryBuilder) {
				qb.Limit(0)
			},
			expected: query.QueryValidationResult{
				Valid: false,
				Errors: []string{
					"pagination limit must be positive",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := query.NewQueryBuilder()
			tt.setup(qb)
			result := qb.Validate()
			assert.Equal(t, tt.expected, result, "Validation result should match expected")
		})
	}
}

func TestQueryBuilder_String(t *testing.T) {
	qb := query.NewQueryBuilder()
	qb.Where("age").Eq(30)

	result := qb.String()
	var jsonResult map[string]any
	err := json.Unmarshal([]byte(result), &jsonResult)
	assert.NoError(t, err, "String output should be valid JSON")
	assert.Contains(t, result, `"field": "age"`, "String output should contain filter field")
	assert.Contains(t, result, `"operator": "eq"`, "String output should contain operator")
	assert.Contains(t, result, `"value": 30`, "String output should contain filter value")
}
