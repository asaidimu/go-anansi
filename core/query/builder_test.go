package query

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
)

func TestNewQueryBuilder(t *testing.T) {
	qb := NewQueryBuilder()
	assert.NotNil(t, qb)
	assert.NotNil(t, qb.query)
	assert.Nil(t, qb.query.Filters)
	assert.Empty(t, qb.query.Sort)
	assert.Nil(t, qb.query.Pagination)
	assert.Nil(t, qb.query.Projection)
	assert.Empty(t, qb.query.Joins)
	assert.Empty(t, qb.query.Aggregations)
	assert.Empty(t, qb.query.Hints)
}

func TestQueryBuilder_Build(t *testing.T) {
	qb := NewQueryBuilder()
	dsl := qb.Build()
	assert.Equal(t, QueryDSL{}, dsl)

	qb.Limit(10)
	dsl = qb.Build()
	assert.NotNil(t, dsl.Pagination)
	assert.Equal(t, 10, dsl.Pagination.Limit)
}

func TestQueryBuilder_Clone(t *testing.T) {
	qb := NewQueryBuilder().Limit(10).OrderByAsc("name")
	clonedQb := qb.Clone()

	assert.NotNil(t, clonedQb)
	assert.Equal(t, qb.query, clonedQb.query)

	// Modify clonedQb and ensure original qb is not affected
	clonedQb.Limit(20)
	assert.Equal(t, 10, qb.query.Pagination.Limit)
	assert.Equal(t, 20, clonedQb.query.Pagination.Limit)
}

func TestQueryBuilder_Reset(t *testing.T) {
	qb := NewQueryBuilder().Limit(10).OrderByAsc("name")
	assert.NotNil(t, qb.query.Pagination)
	assert.NotEmpty(t, qb.query.Sort)

	qb.Reset()
	assert.Nil(t, qb.query.Filters)
	assert.Empty(t, qb.query.Sort)
	assert.Nil(t, qb.query.Pagination)
	assert.Nil(t, qb.query.Projection)
	assert.Empty(t, qb.query.Joins)
	assert.Empty(t, qb.query.Aggregations)
	assert.Empty(t, qb.query.Hints)
}

func TestQueryBuilder_Where(t *testing.T) {
	tests := []struct {
		name     string
		buildFn  func(*QueryBuilder) *QueryBuilder
		expected QueryFilter
	}{
		{
			name: "Eq condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").Eq("value1")
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorEq,
					Value:    "value1",
				},
			},
		},
		{
			name: "Neq condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").Neq("value1")
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorNeq,
					Value:    "value1",
				},
			},
		},
		{
			name: "Lt condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").Lt(10)
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorLt,
					Value:    10,
				},
			},
		},
		{
			name: "Lte condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").Lte(10)
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorLte,
					Value:    10,
				},
			},
		},
		{
			name: "Gt condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").Gt(10)
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorGt,
					Value:    10,
				},
			},
		},
		{
			name: "Gte condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").Gte(10)
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorGte,
					Value:    10,
				},
			},
		},
		{
			name: "In condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").In("value1", "value2")
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorIn,
					Value:    []FilterValue{"value1", "value2"},
				},
			},
		},
		{
			name: "Nin condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").Nin("value1", "value2")
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorNin,
					Value:    []FilterValue{"value1", "value2"},
				},
			},
		},
		{
			name: "Contains condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").Contains("substring")
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorContains,
					Value:    "substring",
				},
			},
		},
		{
			name: "NotContains condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").NotContains("substring")
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorNotContains,
					Value:    "substring",
				},
			},
		},
		{
			name: "StartsWith condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").StartsWith("prefix")
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorStartsWith,
					Value:    "prefix",
				},
			},
		},
		{
			name: "EndsWith condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").EndsWith("suffix")
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorEndsWith,
					Value:    "suffix",
				},
			},
		},
		{
			name: "Exists condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").Exists()
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorExists,
					Value:    true,
				},
			},
		},
		{
			name: "NotExists condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").NotExists()
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: ComparisonOperatorNotExists,
					Value:    true,
				},
			},
		},
		{
			name: "Custom condition",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.Where("field1").Custom("CUSTOM_OP", "custom_value")
			},
			expected: QueryFilter{
				Condition: &FilterCondition{
					Field:    "field1",
					Operator: "CUSTOM_OP",
					Value:    "custom_value",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			resultQb := tt.buildFn(qb)
			assert.Same(t, qb, resultQb, "Should return the same QueryBuilder instance")
			assert.NotNil(t, qb.query.Filters)
			assert.Equal(t, tt.expected, *qb.query.Filters)
		})
	}
}

func TestQueryBuilder_WhereGroup(t *testing.T) {
	tests := []struct {
		name     string
		buildFn  func(*QueryBuilder) *QueryBuilder
		expected QueryFilter
	}{
		{
			name: "AND group with two conditions",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.WhereGroup(schema.LogicalAnd).
					Where("field1").Eq("value1").
					Where("field2").Gt(10).
					End()
			},
			expected: QueryFilter{
				Group: &FilterGroup{
					Operator: schema.LogicalAnd,
					Conditions: []QueryFilter{
						{Condition: &FilterCondition{Field: "field1", Operator: ComparisonOperatorEq, Value: "value1"}},
						{Condition: &FilterCondition{Field: "field2", Operator: ComparisonOperatorGt, Value: 10}},
					},
				},
			},
		},
		{
			name: "OR group with two conditions",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				return qb.WhereGroup(schema.LogicalOr).
					Where("fieldA").Neq("valueA").
					Where("fieldB").Lte(20).
					End()
			},
			expected: QueryFilter{
				Group: &FilterGroup{
					Operator: schema.LogicalOr,
					Conditions: []QueryFilter{
						{Condition: &FilterCondition{Field: "fieldA", Operator: ComparisonOperatorNeq, Value: "valueA"}},
						{Condition: &FilterCondition{Field: "fieldB", Operator: ComparisonOperatorLte, Value: 20}},
					},
				},
			},
		},
		{
			name: "Nested AND group within OR group",
			buildFn: func(qb *QueryBuilder) *QueryBuilder {
				nestedGroup := NewQueryBuilder().WhereGroup(schema.LogicalAnd).
					Where("nestedField1").Contains("text").
					Where("nestedField2").Exists().
					End().Build().Filters // Build the nested group and get its filter

				return qb.WhereGroup(schema.LogicalOr).
					Where("field1").Eq("value1").
					Group(*nestedGroup). // Add the nested group using the new Group method
					End()
			},
			expected: QueryFilter{
				Group: &FilterGroup{
					Operator: schema.LogicalOr,
					Conditions: []QueryFilter{
						{Condition: &FilterCondition{Field: "field1", Operator: ComparisonOperatorEq, Value: "value1"}},
						{Group: &FilterGroup{
							Operator: schema.LogicalAnd,
							Conditions: []QueryFilter{
								{Condition: &FilterCondition{Field: "nestedField1", Operator: ComparisonOperatorContains, Value: "text"}},
								{Condition: &FilterCondition{Field: "nestedField2", Operator: ComparisonOperatorExists, Value: true}},
							},
						}},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := NewQueryBuilder()
			resultQb := tt.buildFn(qb)
			assert.Same(t, qb, resultQb, "Should return the same QueryBuilder instance")
			assert.NotNil(t, qb.query.Filters)
			assert.Equal(t, tt.expected, *qb.query.Filters)
		})
	}
}

func TestQueryBuilder_OrderBy(t *testing.T) {
	qb := NewQueryBuilder().
		OrderBy("name", SortDirectionAsc).
		OrderByDesc("age")

	expectedSort := []SortConfiguration{
		{Field: "name", Direction: SortDirectionAsc},
		{Field: "age", Direction: SortDirectionDesc},
	}

	assert.Equal(t, expectedSort, qb.query.Sort)
}

func TestQueryBuilder_OrderByAsc(t *testing.T) {
	qb := NewQueryBuilder().OrderByAsc("name")
	expectedSort := []SortConfiguration{
		{Field: "name", Direction: SortDirectionAsc},
	}
	assert.Equal(t, expectedSort, qb.query.Sort)
}

func TestQueryBuilder_OrderByDesc(t *testing.T) {
	qb := NewQueryBuilder().OrderByDesc("age")
	expectedSort := []SortConfiguration{
		{Field: "age", Direction: SortDirectionDesc},
	}
	assert.Equal(t, expectedSort, qb.query.Sort)
}

func TestQueryBuilder_Pagination(t *testing.T) {
	t.Run("Limit only", func(t *testing.T) {
		qb := NewQueryBuilder().Limit(10)
		assert.NotNil(t, qb.query.Pagination)
		assert.Equal(t, "offset", qb.query.Pagination.Type)
		assert.Equal(t, 10, qb.query.Pagination.Limit)
		assert.Nil(t, qb.query.Pagination.Offset)
		assert.Nil(t, qb.query.Pagination.Cursor)
	})

	t.Run("Limit and Offset", func(t *testing.T) {
		qb := NewQueryBuilder().Limit(10).Offset(5)
		assert.NotNil(t, qb.query.Pagination)
		assert.Equal(t, "offset", qb.query.Pagination.Type)
		assert.Equal(t, 10, qb.query.Pagination.Limit)
		assert.NotNil(t, qb.query.Pagination.Offset)
		assert.Equal(t, 5, *qb.query.Pagination.Offset)
		assert.Nil(t, qb.query.Pagination.Cursor)
	})

	t.Run("Cursor based pagination", func(t *testing.T) {
		qb := NewQueryBuilder().Limit(10).Cursor("some_cursor_value")
		assert.NotNil(t, qb.query.Pagination)
		assert.Equal(t, "cursor", qb.query.Pagination.Type)
		assert.Equal(t, 10, qb.query.Pagination.Limit)
		assert.Nil(t, qb.query.Pagination.Offset)
		assert.NotNil(t, qb.query.Pagination.Cursor)
		assert.Equal(t, "some_cursor_value", *qb.query.Pagination.Cursor)
	})
}

func TestQueryBuilder_Select(t *testing.T) {
	t.Run("Include fields", func(t *testing.T) {
		qb := NewQueryBuilder().Select().Include("field1", "field2").End()
		assert.NotNil(t, qb.query.Projection)
		expectedInclude := []ProjectionField{
			{Name: "field1"},
			{Name: "field2"},
		}
		assert.Equal(t, expectedInclude, qb.query.Projection.Include)
		assert.Empty(t, qb.query.Projection.Exclude)
	})

	t.Run("Exclude fields", func(t *testing.T) {
		qb := NewQueryBuilder().Select().Exclude("field3", "field4").End()
		assert.NotNil(t, qb.query.Projection)
		expectedExclude := []ProjectionField{
			{Name: "field3"},
			{Name: "field4"},
		}
		assert.Equal(t, expectedExclude, qb.query.Projection.Exclude)
		assert.Empty(t, qb.query.Projection.Include)
	})

	t.Run("Include nested fields", func(t *testing.T) {
		nestedConfig := &ProjectionConfiguration{}
		nestedConfig.AddIncludeFields("nestedField1")

		qb := NewQueryBuilder().Select().IncludeNested("parentField", nestedConfig).End()
		assert.NotNil(t, qb.query.Projection)
		expectedInclude := []ProjectionField{
			{Name: "parentField", Nested: nestedConfig},
		}
		assert.Equal(t, expectedInclude, qb.query.Projection.Include)
	})

	t.Run("Add computed field", func(t *testing.T) {
		qb := NewQueryBuilder().Select().AddComputed("fullName", "CONCAT", "firstName", " ", "lastName").End()
		assert.NotNil(t, qb.query.Projection)
		assert.Len(t, qb.query.Projection.Computed, 1)
		computed := qb.query.Projection.Computed[0]
		assert.NotNil(t, computed.ComputedFieldExpression)
		assert.Equal(t, "fullName", computed.ComputedFieldExpression.Alias)
		assert.Equal(t, "computed", computed.ComputedFieldExpression.Type)
		assert.NotNil(t, computed.ComputedFieldExpression.Expression)
		assert.Equal(t, "CONCAT", computed.ComputedFieldExpression.Expression.Function)
		assert.Equal(t, []FilterValue{"firstName", " ", "lastName"}, computed.ComputedFieldExpression.Expression.Arguments)
	})

	t.Run("Add case expression", func(t *testing.T) {
		qb := NewQueryBuilder().Select().
			AddCase("statusText").
			When(CreateSimpleFilter("status", ComparisonOperatorEq, 1), "Active").
			When(CreateSimpleFilter("status", ComparisonOperatorEq, 0), "Inactive").
			Else("Unknown").
			End(). // End CaseExpressionBuilder
			End() // End ProjectionBuilder

		assert.NotNil(t, qb.query.Projection)
		assert.Len(t, qb.query.Projection.Computed, 1)
		computed := qb.query.Projection.Computed[0]
		assert.NotNil(t, computed.CaseExpression)
		assert.Equal(t, "statusText", computed.CaseExpression.Alias)
		assert.Equal(t, "case", computed.CaseExpression.Type)
		assert.Len(t, computed.CaseExpression.Cases, 2)
		assert.Equal(t, "Active", computed.CaseExpression.Cases[0].Then)
		assert.Equal(t, "Inactive", computed.CaseExpression.Cases[1].Then)
		assert.Equal(t, "Unknown", computed.CaseExpression.Else)
	})
}

func TestQueryBuilder_Join(t *testing.T) {
	t.Run("Inner Join", func(t *testing.T) {
		qb := NewQueryBuilder().
			InnerJoin("orders").
			On(CreateSimpleFilter("users.id", ComparisonOperatorEq, "orders.user_id")).
			Alias("o").
			End()

		assert.Len(t, qb.query.Joins, 1)
		join := qb.query.Joins[0]
		assert.Equal(t, JoinTypeInner, join.Type)
		assert.Equal(t, "orders", join.TargetTable)
		assert.Equal(t, "o", join.Alias)
		assert.NotNil(t, join.On)
		assert.Equal(t, "users.id", join.On.Condition.Field)
	})

	t.Run("Left Join with Projection", func(t *testing.T) {
		projConfig := CreateProjectionConfig().AddIncludeFields("item_name", "quantity")
		qb := NewQueryBuilder().
			LeftJoin("items").
			On(CreateSimpleFilter("orders.item_id", ComparisonOperatorEq, "items.id")).
			WithProjection(projConfig).
			End()

		assert.Len(t, qb.query.Joins, 1)
		join := qb.query.Joins[0]
		assert.Equal(t, JoinTypeLeft, join.Type)
		assert.Equal(t, "items", join.TargetTable)
		assert.NotNil(t, join.Projection)
		assert.Len(t, join.Projection.Include, 2)
	})

	t.Run("Right Join", func(t *testing.T) {
		qb := NewQueryBuilder().RightJoin("products").On(CreateSimpleFilter("p.id", ComparisonOperatorEq, "o.product_id")).End()
		assert.Len(t, qb.query.Joins, 1)
		assert.Equal(t, JoinTypeRight, qb.query.Joins[0].Type)
	})

	t.Run("Full Join", func(t *testing.T) {
		qb := NewQueryBuilder().FullJoin("categories").On(CreateSimpleFilter("c.id", ComparisonOperatorEq, "p.category_id")).End()
		assert.Len(t, qb.query.Joins, 1)
		assert.Equal(t, JoinTypeFull, qb.query.Joins[0].Type)
	})
}

func TestQueryBuilder_Aggregate(t *testing.T) {
	t.Run("Count aggregation", func(t *testing.T) {
		qb := NewQueryBuilder().Count("id", "totalUsers")
		assert.Len(t, qb.query.Aggregations, 1)
		agg := qb.query.Aggregations[0]
		assert.Equal(t, AggregationTypeCount, agg.Type)
		assert.Equal(t, "id", agg.Field)
		assert.Equal(t, "totalUsers", agg.Alias)
	})

	t.Run("Sum aggregation", func(t *testing.T) {
		qb := NewQueryBuilder().Sum("price", "totalRevenue")
		assert.Len(t, qb.query.Aggregations, 1)
		agg := qb.query.Aggregations[0]
		assert.Equal(t, AggregationTypeSum, agg.Type)
		assert.Equal(t, "price", agg.Field)
		assert.Equal(t, "totalRevenue", agg.Alias)
	})

	t.Run("Avg aggregation", func(t *testing.T) {
		qb := NewQueryBuilder().Avg("rating", "avgRating")
		assert.Len(t, qb.query.Aggregations, 1)
		agg := qb.query.Aggregations[0]
		assert.Equal(t, AggregationTypeAvg, agg.Type)
		assert.Equal(t, "rating", agg.Field)
		assert.Equal(t, "avgRating", agg.Alias)
	})

	t.Run("Min aggregation", func(t *testing.T) {
		qb := NewQueryBuilder().Min("date", "minDate")
		assert.Len(t, qb.query.Aggregations, 1)
		agg := qb.query.Aggregations[0]
		assert.Equal(t, AggregationTypeMin, agg.Type)
		assert.Equal(t, "date", agg.Field)
		assert.Equal(t, "minDate", agg.Alias)
	})

	t.Run("Max aggregation", func(t *testing.T) {
		qb := NewQueryBuilder().Max("date", "maxDate")
		assert.Len(t, qb.query.Aggregations, 1)
		agg := qb.query.Aggregations[0]
		assert.Equal(t, AggregationTypeMax, agg.Type)
		assert.Equal(t, "date", agg.Field)
		assert.Equal(t, "maxDate", agg.Alias)
	})
}

func TestQueryBuilder_Hints(t *testing.T) {
	t.Run("Add generic hint", func(t *testing.T) {
		qb := NewQueryBuilder().AddHint("NO_CACHE")
		assert.Len(t, qb.query.Hints, 1)
		hint := qb.query.Hints[0]
		assert.Equal(t, "NO_CACHE", hint.Type)
		assert.Empty(t, hint.Index)
		assert.Zero(t, hint.Seconds)
	})

	t.Run("UseIndex hint", func(t *testing.T) {
		qb := NewQueryBuilder().UseIndex("idx_users_email")
		assert.Len(t, qb.query.Hints, 1)
		hint := qb.query.Hints[0]
		assert.Equal(t, "index", hint.Type)
		assert.Equal(t, "idx_users_email", hint.Index)
	})

	t.Run("ForceIndex hint", func(t *testing.T) {
		qb := NewQueryBuilder().ForceIndex("idx_products_price")
		assert.Len(t, qb.query.Hints, 1)
		hint := qb.query.Hints[0]
		assert.Equal(t, "force_index", hint.Type)
		assert.Equal(t, "idx_products_price", hint.Index)
	})

	t.Run("NoIndex hint", func(t *testing.T) {
		qb := NewQueryBuilder().NoIndex("idx_old_data")
		assert.Len(t, qb.query.Hints, 1)
		hint := qb.query.Hints[0]
		assert.Equal(t, "no_index", hint.Type)
		assert.Equal(t, "idx_old_data", hint.Index)
	})

	t.Run("MaxExecutionTime hint", func(t *testing.T) {
		qb := NewQueryBuilder().MaxExecutionTime(30)
		assert.Len(t, qb.query.Hints, 1)
		hint := qb.query.Hints[0]
		assert.Equal(t, "max_execution_time", hint.Type)
		assert.Equal(t, 30, hint.Seconds)
	})
}

func TestQueryBuilder_Validate(t *testing.T) {
	tests := []struct {
		name      string
		buildFn   func() *QueryBuilder
		isValid   bool
		errorMsgs []string
	}{
		{
			name:    "Valid empty query",
			buildFn: NewQueryBuilder,
			isValid: true,
		},
		{
			name: "Valid query with filter and sort",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Where("name").Eq("test").OrderByAsc("id")
			},
			isValid: true,
		},
		{
			name: "Invalid pagination - limit zero",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Limit(0)
			},
			isValid:   false,
			errorMsgs: []string{"limit must be greater than 0"},
		},
		{
			name: "Invalid pagination - negative offset",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Offset(-1).Limit(1) // Add a valid limit to isolate offset error
			},
			isValid:   false,
			errorMsgs: []string{"offset cannot be negative"},
		},
		{
			name: "Invalid pagination - empty cursor for cursor type",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Cursor("").Limit(1) // Add a valid limit to isolate cursor error
			},
			isValid:   false,
			errorMsgs: []string{"cursor cannot be empty for cursor-based pagination"},
		},
		{
			name: "Invalid projection - both include and exclude",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Select().Include("field1").Exclude("field2").End()
			},
			isValid:   false,
			errorMsgs: []string{"cannot have both include and exclude fields"},
		},
		{
			name: "Invalid join - empty target table",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().InnerJoin("").On(CreateSimpleFilter("a", ComparisonOperatorEq, "b")).End()
			},
			isValid:   false,
			errorMsgs: []string{"target table cannot be empty"},
		},
		{
			name: "Invalid aggregation - missing field for non-count",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Sum("", "total")
			},
			isValid:   false,
			errorMsgs: []string{"field is required for non-count aggregations"},
		},
		{
			name: "Invalid aggregation - missing alias",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Sum("amount", "")
			},
			isValid:   false,
			errorMsgs: []string{"alias is required for aggregations"},
		},
		{
			name: "Multiple errors",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Limit(0).
					Select().Include("f1").Exclude("f2").End().
					InnerJoin("").On(CreateSimpleFilter("a", ComparisonOperatorEq, "b")).End()
			},
			isValid:   false,
			errorMsgs: []string{"limit must be greater than 0", "cannot have both include and exclude fields", "target table cannot be empty"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := tt.buildFn()
			result := qb.Validate()
			assert.Equal(t, tt.isValid, result.IsValid)
			if !tt.isValid {
				assert.Len(t, result.Errors, len(tt.errorMsgs))
				for _, errMsg := range tt.errorMsgs {
					// Find the error in the result.Errors slice
					found := false
					for _, err := range result.Errors {
						if err.Message == errMsg {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected error message not found: %s", errMsg)
				}
			} else {
				assert.Empty(t, result.Errors)
			}
		})
	}
}

func TestQueryBuilder_String(t *testing.T) {
	tests := []struct {
		name     string
		buildFn  func() *QueryBuilder
		expected string
	}{
		{
			name:     "Empty query",
			buildFn:  NewQueryBuilder,
			expected: "EMPTY QUERY",
		},
		{
			name: "Query with filter",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Where("name").Eq("test")
			},
			expected: "FILTERS: present",
		},
		{
			name: "Query with sort",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().OrderByAsc("name").OrderByDesc("age")
			},
			expected: "ORDER BY: name asc, age desc",
		},
		{
			name: "Query with limit and offset",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Limit(10).Offset(5)
			},
			expected: "LIMIT: 10 | OFFSET: 5",
		},
		{
			name: "Query with cursor pagination",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Limit(20).Cursor("abc")
			},
			expected: "CURSOR LIMIT: 20",
		},
		{
			name: "Query with include projection",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Select().Include("id", "name").End()
			},
			expected: "SELECT: id, name",
		},
		{
			name: "Query with exclude projection",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Select().Exclude("password", "ssn").End()
			},
			expected: "EXCLUDE: password, ssn",
		},
		{
			name: "Query with join",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().InnerJoin("orders").On(CreateSimpleFilter("u.id", ComparisonOperatorEq, "o.user_id")).End()
			},
			expected: "JOINS: 1",
		},
		{
			name: "Query with aggregation",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Count("id", "total")
			},
			expected: "AGGREGATIONS: 1",
		},
		{
			name: "Query with hint",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().UseIndex("idx_name")
			},
			expected: "HINTS: 1",
		},
		{
			name: "Complex query string",
			buildFn: func() *QueryBuilder {
				return NewQueryBuilder().Where("status").Eq("active").
					OrderByAsc("createdAt").
					Limit(5).Offset(0).
					Select().Include("id", "name").End().
					InnerJoin("details").On(CreateSimpleFilter("u.id", ComparisonOperatorEq, "d.user_id")).End().
					Count("id", "total").
					MaxExecutionTime(60)
			},
			expected: "FILTERS: present | ORDER BY: createdAt asc | LIMIT: 5 | OFFSET: 0 | SELECT: id, name | JOINS: 1 | AGGREGATIONS: 1 | HINTS: 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := tt.buildFn()
			assert.Equal(t, tt.expected, qb.String())
		})
	}
}

func TestCreateSimpleFilter(t *testing.T) {
	filter := CreateSimpleFilter("field", ComparisonOperatorEq, "value")
	assert.NotNil(t, filter.Condition)
	assert.Equal(t, "field", filter.Condition.Field)
	assert.Equal(t, ComparisonOperatorEq, filter.Condition.Operator)
	assert.Equal(t, "value", filter.Condition.Value)
	assert.Nil(t, filter.Group)
}

func TestCreateFilterGroup(t *testing.T) {
	group := CreateFilterGroup(schema.LogicalAnd,
		CreateSimpleFilter("field1", ComparisonOperatorEq, "value1"),
		CreateSimpleFilter("field2", ComparisonOperatorGt, 10),
	)
	assert.NotNil(t, group.Group)
	assert.Equal(t, schema.LogicalAnd, group.Group.Operator)
	assert.Len(t, group.Group.Conditions, 2)
	assert.NotNil(t, group.Group.Conditions[0].Condition)
	assert.NotNil(t, group.Group.Conditions[1].Condition)
	assert.Nil(t, group.Condition)
}

func TestCreateProjectionConfig(t *testing.T) {
	pc := CreateProjectionConfig()
	assert.NotNil(t, pc)
	assert.Empty(t, pc.Include)
	assert.Empty(t, pc.Exclude)
	assert.Empty(t, pc.Computed)
}

func TestProjectionConfiguration_AddIncludeFields(t *testing.T) {
	pc := CreateProjectionConfig()
	pc.AddIncludeFields("field1", "field2")
	expected := []ProjectionField{{Name: "field1"}, {Name: "field2"}}
	assert.Equal(t, expected, pc.Include)
}

func TestProjectionConfiguration_AddExcludeFields(t *testing.T) {
	pc := CreateProjectionConfig()
	pc.AddExcludeFields("field3", "field4")
	expected := []ProjectionField{{Name: "field3"}, {Name: "field4"}}
	assert.Equal(t, expected, pc.Exclude)
}
