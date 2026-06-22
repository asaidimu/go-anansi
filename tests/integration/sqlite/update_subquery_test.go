package sqlite_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/query/native"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	sql "github.com/asaidimu/go-anansi/v7/sqlite/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateWithScalarSubquery(t *testing.T) {
	// Test: UPDATE employees SET salary = (SELECT AVG(salary) FROM employees WHERE department = 'Engineering')
	// WHERE id = 'emp1'

	employeesSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "employees",
			Fields: map[definition.FieldId]definition.Field{
				"id":         {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"salary":     {Name: "salary", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"department": {Name: "department", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	empID := "emp1"
	dept := "Engineering"

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "employees",
			Schema: employeesSchema,
		},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					StringVal: &empID,
				},
			},
		},
	}

	updatePayload := map[string]any{
		"compute": map[string]query.Query{
			"salary": {
				Target: &query.QueryTarget{
					Name:   "employees",
					Schema: employeesSchema,
				},
				Aggregations: []query.AggregationConfiguration{
					{
						Type:  query.AggregationTypeAvg,
						Field: "salary",
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
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtUpdate, updatePayload)
	require.NoError(t, err)

	expectedSQL := `UPDATE "employees" SET "salary" = (SELECT AVG("salary") FROM employees WHERE "department" = $1) WHERE "id" = $2`
	assert.Equal(t, expectedSQL, value.Raw().SQL)
	assert.Equal(t, []any{"Engineering", "emp1"}, value.Raw().Params)
}

func TestUpdateWithSubqueryInWHERE(t *testing.T) {
	// Test: UPDATE employees SET bonus = 1000
	// WHERE department_id IN (SELECT id FROM departments WHERE budget > 100000)

	employeesSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "employees",
			Fields: map[definition.FieldId]definition.Field{
				"id":            {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"bonus":         {Name: "bonus", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
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

	budget := 100000.0
	bonus := 1000.0

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "employees",
			Schema: employeesSchema,
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
									Field:    "budget",
									Operator: query.ComparisonOperatorGt,
									Value: query.FilterValue{
										NumberVal: &budget,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	updatePayload := map[string]any{
		"set": map[string]any{
			"bonus": bonus,
		},
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtUpdate, updatePayload)
	require.NoError(t, err)

	expectedSQL := `UPDATE "employees" SET "bonus" = $1 WHERE "department_id" IN (SELECT "id" FROM departments WHERE "budget" > $2)`
	assert.Equal(t, expectedSQL, value.Raw().SQL)
	assert.Equal(t, []any{1000.0, 100000.0}, value.Raw().Params)
}

func TestUpdateWithSubqueryContainingJoin(t *testing.T) {
	// Test: UPDATE employees SET salary = (
	//   SELECT AVG(e.salary) FROM employees e
	//   INNER JOIN departments d ON e.department_id = d.id
	//   WHERE d.name = 'Engineering'
	// )
	// WHERE id = 'emp1'

	employeesSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "employees",
			Fields: map[definition.FieldId]definition.Field{
				"id":            {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"salary":        {Name: "salary", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
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

	empID := "emp1"
	deptName := "Engineering"
	empAlias := "e"
	deptAlias := "d"

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "employees",
			Schema: employeesSchema,
		},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					StringVal: &empID,
				},
			},
		},
	}

	updatePayload := map[string]any{
		"compute": map[string]query.Query{
			"salary": {
				Target: &query.QueryTarget{
					Name:   "employees",
					Alias:  &empAlias,
					Schema: employeesSchema,
				},
				Aggregations: []query.AggregationConfiguration{
					{
						Type:  query.AggregationTypeAvg,
						Field: "e.salary",
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
								Field:    "e.department_id",
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
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtUpdate, updatePayload)
	require.NoError(t, err)

	sqlStr := value.Raw().SQL
	assert.Contains(t, sqlStr, `UPDATE "employees"`)
	assert.Contains(t, sqlStr, `SET "salary" = (SELECT AVG("e"."salary") FROM employees AS e`)
	assert.Contains(t, sqlStr, `INNER JOIN departments AS d ON "e"."department_id" = "d"."id"`)
	assert.Contains(t, sqlStr, `WHERE "d"."name" = $1)`)
	assert.Contains(t, sqlStr, `WHERE "id" = $2`)
	assert.Equal(t, []any{"Engineering", "emp1"}, value.Raw().Params)
}

func TestUpdateWithNestedSubqueries(t *testing.T) {
	// Test: UPDATE employees SET salary = (
	//   SELECT AVG(salary) FROM employees
	//   WHERE department_id IN (
	//     SELECT id FROM departments WHERE region = 'West'
	//   )
	// )
	// WHERE id = 'emp1'

	employeesSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "employees",
			Fields: map[definition.FieldId]definition.Field{
				"id":            {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"salary":        {Name: "salary", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"department_id": {Name: "department_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	departmentsSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "departments",
			Fields: map[definition.FieldId]definition.Field{
				"id":     {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"region": {Name: "region", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	empID := "emp1"
	region := "West"

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "employees",
			Schema: employeesSchema,
		},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					StringVal: &empID,
				},
			},
		},
	}

	updatePayload := map[string]any{
		"compute": map[string]query.Query{
			"salary": {
				Target: &query.QueryTarget{
					Name:   "employees",
					Schema: employeesSchema,
				},
				Aggregations: []query.AggregationConfiguration{
					{
						Type:  query.AggregationTypeAvg,
						Field: "salary",
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
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtUpdate, updatePayload)
	require.NoError(t, err)

	expectedSQL := `UPDATE "employees" SET "salary" = (SELECT AVG("salary") FROM employees WHERE "department_id" IN (SELECT "id" FROM departments WHERE "region" = $1)) WHERE "id" = $2`
	assert.Equal(t, expectedSQL, value.Raw().SQL)
	assert.Equal(t, []any{"West", "emp1"}, value.Raw().Params)
}

func TestUpdateWithCorrelatedSubquery(t *testing.T) {
	// Test: UPDATE employees e1 SET salary = (
	//   SELECT AVG(e2.salary) FROM employees e2
	//   WHERE e2.department_id = e1.department_id
	// )
	// WHERE e1.id = 'emp1'

	employeesSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "employees",
			Fields: map[definition.FieldId]definition.Field{
				"id":            {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"salary":        {Name: "salary", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"department_id": {Name: "department_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	empID := "emp1"
	e1Alias := "e1"
	e2Alias := "e2"

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "employees",
			Alias:  &e1Alias,
			Schema: employeesSchema,
		},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "e1.id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					StringVal: &empID,
				},
			},
		},
	}

	updatePayload := map[string]any{
		"compute": map[string]query.Query{
			"salary": {
				Target: &query.QueryTarget{
					Name:   "employees",
					Alias:  &e2Alias,
					Schema: employeesSchema,
				},
				Aggregations: []query.AggregationConfiguration{
					{
						Type:  query.AggregationTypeAvg,
						Field: "e2.salary",
					},
				},
				Filters: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "e2.department_id",
						Operator: query.ComparisonOperatorEq,
						Value: query.FilterValue{
							FieldRefVal: &query.FieldReference{
								Field: "e1.department_id", // Correlated reference to outer query
							},
						},
					},
				},
			},
		},
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtUpdate, updatePayload)
	require.NoError(t, err)

	sqlStr := value.Raw().SQL
	// Verify correlated references are present
	assert.Contains(t, sqlStr, `"e1"."department_id"`)
	assert.Contains(t, sqlStr, `"e2"."department_id"`)
	assert.Contains(t, sqlStr, `AVG("e2"."salary")`)
}

func TestUpdateMultipleFieldsWithSubqueries(t *testing.T) {
	// Test: UPDATE employees SET
	//   salary = (SELECT AVG(salary) FROM employees WHERE department = 'Engineering'),
	//   bonus = (SELECT MAX(bonus) FROM employees WHERE department = 'Sales')
	// WHERE id = 'emp1'

	employeesSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "employees",
			Fields: map[definition.FieldId]definition.Field{
				"id":         {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"salary":     {Name: "salary", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"bonus":      {Name: "bonus", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"department": {Name: "department", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	empID := "emp1"
	engDept := "Engineering"
	salesDept := "Sales"

	q := &query.Query{
		Target: &query.QueryTarget{
			Name:   "employees",
			Schema: employeesSchema,
		},
		Filters: &query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "id",
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					StringVal: &empID,
				},
			},
		},
	}

	updatePayload := map[string]any{
		"compute": map[string]query.Query{
			"salary": {
				Target: &query.QueryTarget{
					Name:   "employees",
					Schema: employeesSchema,
				},
				Aggregations: []query.AggregationConfiguration{
					{
						Type:  query.AggregationTypeAvg,
						Field: "salary",
					},
				},
				Filters: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "department",
						Operator: query.ComparisonOperatorEq,
						Value: query.FilterValue{
							StringVal: &engDept,
						},
					},
				},
			},
			"bonus": {
				Target: &query.QueryTarget{
					Name:   "employees",
					Schema: employeesSchema,
				},
				Aggregations: []query.AggregationConfiguration{
					{
						Type:  query.AggregationTypeMax,
						Field: "bonus",
					},
				},
				Filters: &query.QueryFilter{
					Condition: &query.FilterCondition{
						Field:    "department",
						Operator: query.ComparisonOperatorEq,
						Value: query.FilterValue{
							StringVal: &salesDept,
						},
					},
				},
			},
		},
	}

	factory := sql.NewSQLiteFactory(nil)
	value, err := factory.Build(q, native.StmtUpdate, updatePayload)
	require.NoError(t, err)

	sqlStr := value.Raw().SQL
	// Both subqueries should be present
	assert.Contains(t, sqlStr, `"bonus" = (SELECT MAX("bonus") FROM employees WHERE "department" = $1)`)
	assert.Contains(t, sqlStr, `"salary" = (SELECT AVG("salary") FROM employees WHERE "department" = $2)`)
	assert.Contains(t, sqlStr, `WHERE "id" = $3`)

	// Verify all three parameters are present
	assert.Len(t, value.Raw().Params, 3)
}
