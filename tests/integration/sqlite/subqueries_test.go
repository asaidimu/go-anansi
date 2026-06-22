package sqlite_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/query/native"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	sql "github.com/asaidimu/go-anansi/v7/sqlite/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleScalarSubquery(t *testing.T) {
	// Test: WHERE age > (SELECT AVG(age) FROM users)

	usersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"id":   {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":  {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "users",
			Schema: usersSchema,
		},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "age",
				Operator: query.ComparisonOperatorGt,
				Value: query.FilterValue{
					SubqueryVal: &query.SubqueryValue{
						Type: "subquery",
						Query: query.Query{
							Target: &query.QueryTarget{
								Name:   "users",
								Schema: usersSchema,
							},
							Aggregations: []query.AggregationConfiguration{
								{
									Type:  query.AggregationTypeAvg,
									Field: "age",
								},
							},
						},
					},
				},
			},
		},
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtSelect, nil)
	assert.NoError(t, err)

	expectedSQL := "SELECT users.age AS 'users.age', users.id AS 'users.id', users.name AS 'users.name' FROM users WHERE \"age\" > (SELECT AVG(\"age\") FROM users)"
	assert.Equal(t, expectedSQL, value.Raw().SQL)
	assert.Empty(t, value.Raw().Params)
}

func TestSubqueryWithIN(t *testing.T) {
	// Test: WHERE user_id IN (SELECT id FROM users WHERE department = 'Engineering')
	dept := "Engineering"

	usersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"id":         {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"department": {Name: "department", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	ordersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "orders",
			Fields: map[definition.FieldId]definition.Field{
				"id":      {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"user_id": {Name: "user_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"total":   {Name: "total", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
		},
	}

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "orders",
			Schema: ordersSchema,
		},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "user_id",
				Operator: query.ComparisonOperatorIn,
				Value: query.FilterValue{
					SubqueryVal: &query.SubqueryValue{
						Type: "subquery",
						Query: query.Query{
							Target: &query.QueryTarget{
								Name:   "users",
								Schema: usersSchema,
							},
							Projection: &query.ProjectionConfiguration{
								Include: []query.ProjectionField{
									{Name: "id"},
								},
							},
							Filters: &query.QueryFilter{
								Condition: &query.FilterCondition{
									Field:    "department",
									Operator: query.ComparisonOperatorEq,
									Value: query.FilterValue{
										StringVal: &dept,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtSelect, nil)
	require.NoError(t, err)

	expectedSQL := "SELECT orders.id AS 'orders.id', orders.total AS 'orders.total', orders.user_id AS 'orders.user_id' FROM orders WHERE \"user_id\" IN (SELECT \"id\" FROM users WHERE \"department\" = $1)"
	assert.Equal(t, expectedSQL, value.Raw().SQL)
	assert.Equal(t, []any{"Engineering"}, value.Raw().Params)
}

func TestSubqueryWithJoin(t *testing.T) {
	// Test: Subquery contains a JOIN
	// SELECT * FROM orders WHERE user_id IN (
	//   SELECT u.id FROM users u
	//   INNER JOIN departments d ON u.department_id = d.id
	//   WHERE d.name = 'Engineering'
	// )

	deptName := "Engineering"

	usersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"id":            {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"department_id": {Name: "department_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	departmentsSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "departments",
			Fields: map[definition.FieldId]definition.Field{
				"id":   {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	ordersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "orders",
			Fields: map[definition.FieldId]definition.Field{
				"id":      {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"user_id": {Name: "user_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	userAlias := "u"
	deptAlias := "d"

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "orders",
			Schema: ordersSchema,
		},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "user_id",
				Operator: query.ComparisonOperatorIn,
				Value: query.FilterValue{
					SubqueryVal: &query.SubqueryValue{
						Type: "subquery",
						Query: query.Query{
							Target: &query.QueryTarget{
								Name:   "users",
								Alias:  &userAlias,
								Schema: usersSchema,
							},
							Projection: &query.ProjectionConfiguration{
								Include: []query.ProjectionField{
									{Name: "u.id"},
								},
							},
							Joins: []query.JoinConfiguration{
								{
									Type: query.JoinTypeInner,
									Target: query.QueryTarget{
										Name:   "departments",
										Alias:  &deptAlias,
										Schema: departmentsSchema,
									},
									On: &query.QueryFilter{
										Condition: &query.FilterCondition{
											Field:    "u.department_id",
											Operator: query.ComparisonOperatorEq,
											Value: query.FilterValue{
												FieldRefVal: &query.FieldReference{
													Field: "d.id",
												},
											},
										},
									},
								},
							},
							Filters: &query.QueryFilter{
								Condition: &query.FilterCondition{
									Field:    "d.name",
									Operator: query.ComparisonOperatorEq,
									Value: query.FilterValue{
										StringVal: &deptName,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtSelect, nil)
	require.NoError(t, err)

	expectedSQL := `SELECT orders.id AS 'orders.id', orders.user_id AS 'orders.user_id' FROM orders WHERE "user_id" IN (SELECT "u"."id" FROM users AS u INNER JOIN departments AS d ON "u"."department_id" = "d"."id" WHERE "d"."name" = $1)`
	assert.Equal(t, expectedSQL, value.Raw().SQL)
	assert.Equal(t, []any{"Engineering"}, value.Raw().Params)
}

func TestNestedSubqueries(t *testing.T) {
	// Test: Nested subqueries (subquery within subquery)
	// SELECT * FROM orders WHERE user_id IN (
	//   SELECT id FROM users WHERE department_id IN (
	//     SELECT id FROM departments WHERE region = 'East'
	//   )
	// )

	region := "East"

	departmentsSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "departments",
			Fields: map[definition.FieldId]definition.Field{
				"id":     {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"region": {Name: "region", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	usersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"id":            {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"department_id": {Name: "department_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	ordersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "orders",
			Fields: map[definition.FieldId]definition.Field{
				"id":      {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"user_id": {Name: "user_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "orders",
			Schema: ordersSchema,
		},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "user_id",
				Operator: query.ComparisonOperatorIn,
				Value: query.FilterValue{
					SubqueryVal: &query.SubqueryValue{
						Type: "subquery",
						Query: query.Query{
							Target: &query.QueryTarget{
								Name:   "users",
								Schema: usersSchema,
							},
							Projection: &query.ProjectionConfiguration{
								Include: []query.ProjectionField{
									{Name: "id"},
								},
							},
							Filters: &query.QueryFilter{
								Condition: &query.FilterCondition{
									Field:    "department_id",
									Operator: query.ComparisonOperatorIn,
									Value: query.FilterValue{
										SubqueryVal: &query.SubqueryValue{
											Type: "subquery",
											Query: query.Query{
												Target: &query.QueryTarget{
													Name:   "departments",
													Schema: departmentsSchema,
												},
												Projection: &query.ProjectionConfiguration{
													Include: []query.ProjectionField{
														{Name: "id"},
													},
												},
												Filters: &query.QueryFilter{
													Condition: &query.FilterCondition{
														Field:    "region",
														Operator: query.ComparisonOperatorEq,
														Value: query.FilterValue{
															StringVal: &region,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtSelect, nil)
	require.NoError(t, err)

	expectedSQL := `SELECT orders.id AS 'orders.id', orders.user_id AS 'orders.user_id' FROM orders WHERE "user_id" IN (SELECT "id" FROM users WHERE "department_id" IN (SELECT "id" FROM departments WHERE "region" = $1))`
	assert.Equal(t, expectedSQL, value.Raw().SQL)
	assert.Equal(t, []any{"East"}, value.Raw().Params)
}

func TestCorrelatedSubquery(t *testing.T) {
	// Test: Correlated subquery (references outer query)
	// SELECT * FROM users u WHERE (
	//   SELECT COUNT(*) FROM orders o WHERE o.user_id = u.id
	// ) > 5

	usersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"id":   {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	ordersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "orders",
			Fields: map[definition.FieldId]definition.Field{
				"id":      {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"user_id": {Name: "user_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	userAlias := "u"
	orderAlias := "o"

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "users",
			Alias:  &userAlias,
			Schema: usersSchema,
		},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "dummy", // This would be handled differently in real implementation
				Operator: query.ComparisonOperatorGt,
				Value: query.FilterValue{
					SubqueryVal: &query.SubqueryValue{
						Type: "subquery",
						Query: query.Query{
							Target: &query.QueryTarget{
								Name:   "orders",
								Alias:  &orderAlias,
								Schema: ordersSchema,
							},
							Aggregations: []query.AggregationConfiguration{
								{
									Type:  query.AggregationTypeCount,
									Field: "*",
								},
							},
							Filters: &query.QueryFilter{
								Condition: &query.FilterCondition{
									Field:    "o.user_id",
									Operator: query.ComparisonOperatorEq,
									Value: query.FilterValue{
										FieldRefVal: &query.FieldReference{
											Field: "u.id", // References outer query
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtSelect, nil)
	require.NoError(t, err)

	// The SQL should contain a reference to the outer table
	// Note: Field references are quoted as "table"."column"
	sqlStr := value.Raw().SQL
	assert.Contains(t, sqlStr, "\"u\".\"id\"")
	assert.Contains(t, sqlStr, "\"o\".\"user_id\"")

	// Verify the full expected SQL structure
	expectedSQL := `SELECT u.id AS 'u.id', u.name AS 'u.name' FROM users AS u WHERE "dummy" > (SELECT COUNT(*) FROM orders AS o WHERE "o"."user_id" = "u"."id")`
	assert.Equal(t, expectedSQL, sqlStr)
}

func TestSubqueryDepthLimit(t *testing.T) {
	// Test: Ensure we prevent infinite recursion with depth limit

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "test",
			Fields: map[definition.FieldId]definition.Field{
				"id": {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	// Build a query with excessive nesting (> maxSubqueryDepth)
	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "test",
			Schema: testSchema,
		},
	}

	// Create 12 levels of nesting (maxSubqueryDepth is 10)
	currentQuery := q
	for range 12 {
		val := "test"
		currentQuery.Filters = &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "id",
				Operator: query.ComparisonOperatorIn,
				Value: query.FilterValue{
					SubqueryVal: &query.SubqueryValue{
						Type: "subquery",
						Query: query.Query{
							Target: &query.QueryTarget{
								Name:   "test",
								Schema: testSchema,
							},
							Filters: &query.QueryFilter{
								Condition: &query.FilterCondition{
									Field:    "id",
									Operator: query.ComparisonOperatorEq,
									Value: query.FilterValue{
										StringVal: &val,
									},
								},
							},
						},
					},
				},
			},
		}
		currentQuery = &currentQuery.Filters.Condition.Value.SubqueryVal.Query
	}

	factory := sql.NewSQLiteFactory(nil)
	_, err := factory.Build(q, native.StmtSelect, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum subquery nesting depth")
}

func TestSubqueryInJoinCondition(t *testing.T) {
	// Test: Subquery in JOIN condition
	// SELECT * FROM users u
	// INNER JOIN departments d ON u.department_id = d.id
	//   AND d.budget > (SELECT AVG(budget) FROM departments)

	usersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"id":            {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"department_id": {Name: "department_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	departmentsSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "departments",
			Fields: map[definition.FieldId]definition.Field{
				"id":     {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"budget": {Name: "budget", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
		},
	}

	userAlias := "u"
	deptAlias := "d"

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "users",
			Alias:  &userAlias,
			Schema: usersSchema,
		},
		Joins: []query.JoinConfiguration{
			{
				Type: query.JoinTypeInner,
				Target: query.QueryTarget{
					Name:   "departments",
					Alias:  &deptAlias,
					Schema: departmentsSchema,
				},
				On: &query.QueryFilter{
					Group: &query.FilterGroup{
						Operator: common.LogicalAnd,
						Conditions: []query.QueryFilter{
							{
								Condition: &query.FilterCondition{
									Field:    "u.department_id",
									Operator: query.ComparisonOperatorEq,
									Value: query.FilterValue{
										FieldRefVal: &query.FieldReference{
											Field: "d.id",
										},
									},
								},
							},
							{
								Condition: &query.FilterCondition{
									Field:    "d.budget",
									Operator: query.ComparisonOperatorGt,
									Value: query.FilterValue{
										SubqueryVal: &query.SubqueryValue{
											Type: "subquery",
											Query: query.Query{
												Target: &query.QueryTarget{
													Name:   "departments",
													Schema: departmentsSchema,
												},
												Aggregations: []query.AggregationConfiguration{
													{
														Type:  query.AggregationTypeAvg,
														Field: "budget",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtSelect, nil)
	require.NoError(t, err)

	sqlStr := value.Raw().SQL
	assert.Contains(t, sqlStr, "INNER JOIN departments AS d ON")
	assert.Contains(t, sqlStr, "SELECT AVG(\"budget\") FROM departments")
	assert.Empty(t, value.Raw().Params)
}
