package sqlite_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	sqlite "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/stretchr/testify/assert"
)

func TestSelect(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	qb := query.NewQueryBuilder()
	qb.From("users").
		Select().
		AddComputed("full_name", "concat", &query.FieldReference{Field: "first_name"}, " ", &query.FieldReference{Field: "last_name"}).
		AddCase("status_category").
		When(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "status",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{StringVal: utils.StringPtr("active")},
			},
		}, "positive").
		Else("neutral").
		End().
		End()

	q := qb.Build()

	nq, err := builder.Build(&q, native.StmtSelect, nil)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	expectedSQL := `SELECT concat("first_name", $1, "last_name") AS full_name, CASE WHEN "status" = $2 THEN $3 ELSE $4 END AS status_category FROM users`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Equal(t, 4, len(nq.Raw().Params))
	assert.Equal(t, " ", nq.Raw().Params[0])
	assert.Equal(t, "active", nq.Raw().Params[1])
	assert.Equal(t, "positive", nq.Raw().Params[2])
	assert.Equal(t, "neutral", nq.Raw().Params[3])
}

func TestInsert(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	qb := query.NewQueryBuilder()
	qb.From("users")

	q := qb.Build()
	data := data.Document{
		"first_name": "John",
		"last_name":  "Doe",
		"age":        30,
	}

	nq, err := builder.Build(&q, native.StmtInsert, data)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	expectedSQL := `INSERT INTO users (age, first_name, last_name) VALUES ($1, $2, $3) RETURNING *;`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Equal(t, 3, len(nq.Raw().Params))
	// Note: The order of params depends on the sorted order of fields
	assert.Equal(t, 30, nq.Raw().Params[0])
	assert.Equal(t, "John", nq.Raw().Params[1])
	assert.Equal(t, "Doe", nq.Raw().Params[2])
}

func TestUpdate(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	qb := query.NewQueryBuilder()
	qb.From("users").
		Where("id").Eq("user-123")

	q := qb.Build()
	data := data.Document{
		"age":   31,
		"email": "john.doe@example.com",
	}

	nq, err := builder.Build(&q, native.StmtUpdate, data)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	expectedSQL := `UPDATE users SET "age" = $1, "email" = $2 WHERE "id" = $3`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Equal(t, 3, len(nq.Raw().Params))
	assert.Equal(t, 31, nq.Raw().Params[0])
	assert.Equal(t, "john.doe@example.com", nq.Raw().Params[1])
	assert.Equal(t, "user-123", nq.Raw().Params[2])
}

func TestDelete(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	qb := query.NewQueryBuilder()
	qb.From("users").
		Where("id").Eq("user-123")

	q := qb.Build()

	nq, err := builder.Build(&q, native.StmtDelete, nil)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	expectedSQL := `DELETE FROM users WHERE "id" = $1`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Equal(t, 1, len(nq.Raw().Params))
	assert.Equal(t, "user-123", nq.Raw().Params[0])
}

func TestSelectComplex(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	qb := query.NewQueryBuilder()
	qb.From("orders").
		Select().
		Include("id", "order_date", "total_amount").
		End().
		InnerJoin("customers").
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "orders.customer_id",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "customers.id"}},
			},
		}).
		End().
		WhereGroup(common.LogicalAnd).
		Where("total_amount").Gte(100.0).
		Where("customers.region").Eq("West").
		End().
		OrderByDesc("order_date").
		Limit(10).
		Offset(20)

	q := qb.Build()

	nq, err := builder.Build(&q, native.StmtSelect, nil)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	expectedSQL := `SELECT "id", "order_date", "total_amount" FROM orders INNER JOIN customers ON "orders"."customer_id" = "customers"."id" WHERE ("total_amount" >= $1 AND "customers"."region" = $2) ORDER BY "order_date" DESC LIMIT 10 OFFSET 20`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Equal(t, 2, len(nq.Raw().Params))
	assert.Equal(t, 100.0, nq.Raw().Params[0])
	assert.Equal(t, "West", nq.Raw().Params[1])
}

func TestSelectWithInAndNinOperators(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	// Test IN operator
	qbIn := query.NewQueryBuilder().From("users").Where("region").In("East", "West")
	qIn := qbIn.Build()
	nqIn, errIn := builder.Build(&qIn, native.StmtSelect, nil)
	assert.NoError(t, errIn)
	assert.NotNil(t, nqIn)
	expectedSQLIn := `SELECT * FROM users WHERE "region" IN ($1, $2)`
	assert.Equal(t, expectedSQLIn, nqIn.Raw().SQL)
	assert.ElementsMatch(t, []any{"East", "West"}, nqIn.Raw().Params)

	// Test NOT IN operator
	qbNotIn := query.NewQueryBuilder().From("users").Where("status").Nin("inactive", "pending")
	qNotIn := qbNotIn.Build()
	nqNotIn, errNotIn := builder.Build(&qNotIn, native.StmtSelect, nil)
	assert.NoError(t, errNotIn)
	assert.NotNil(t, nqNotIn)
	expectedSQLNotIn := `SELECT * FROM users WHERE "status" NOT IN ($1, $2)`
	assert.Equal(t, expectedSQLNotIn, nqNotIn.Raw().SQL)
	assert.ElementsMatch(t, []any{"inactive", "pending"}, nqNotIn.Raw().Params)
}

func TestSelectWithMultipleJoins(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	qb := query.NewQueryBuilder().
		From("orders").
		InnerJoin("users").On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "orders.customer_id",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "users.id"}},
			},
		}).End().
		LeftJoin("products").On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "orders.product_id",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "products.id"}},
			},
		}).End()

	q := qb.Build()
	nq, err := builder.Build(&q, native.StmtSelect, nil)

	assert.NoError(t, err)
	assert.NotNil(t, nq)

	expectedSQL := `SELECT * FROM orders INNER JOIN users ON "orders"."customer_id" = "users"."id" LEFT JOIN products ON "orders"."product_id" = "products"."id"`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Empty(t, nq.Raw().Params)
}

func TestSelectWithCaseStatement(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	qb := query.NewQueryBuilder().
		From("users").
		Select().
		Include("first_name", "last_name").
		AddCase("status_category").
		When(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "status",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{StringVal: stringp("active")},
			},
		}, "Active User").
		When(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "status",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{StringVal: stringp("inactive")},
			},
		}, "Inactive User").
		Else("Unknown Status").
		End().
		End()

	q := qb.Build()
	nq, err := builder.Build(&q, native.StmtSelect, nil)

	assert.NoError(t, err)
	assert.NotNil(t, nq)

	expectedSQL := `SELECT "first_name", "last_name", CASE WHEN "status" = $1 THEN $2 WHEN "status" = $3 THEN $4 ELSE $5 END AS status_category FROM users`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Equal(t, []any{"active", "Active User", "inactive", "Inactive User", "Unknown Status"}, nq.Raw().Params)
}

func TestSelectWithDifferentDataTypesInWhere(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	// Test with integer
	qbInt := query.NewQueryBuilder().From("users").Where("age").Gt(30)
	qInt := qbInt.Build()
	nqInt, errInt := builder.Build(&qInt, native.StmtSelect, nil)
	assert.NoError(t, errInt)
	assert.NotNil(t, nqInt)
	expectedSQLInt := `SELECT * FROM users WHERE "age" > $1`
	assert.Equal(t, expectedSQLInt, nqInt.Raw().SQL)
	assert.Equal(t, []any{float64(30)}, nqInt.Raw().Params)

	// Test with float
	qbFloat := query.NewQueryBuilder().From("products").Where("price").Lte(9.99)
	qFloat := qbFloat.Build()
	nqFloat, errFloat := builder.Build(&qFloat, native.StmtSelect, nil)
	assert.NoError(t, errFloat)
	assert.NotNil(t, nqFloat)
	expectedSQLFloat := `SELECT * FROM products WHERE "price" <= $1`
	assert.Equal(t, expectedSQLFloat, nqFloat.Raw().SQL)
	assert.Equal(t, []any{9.99}, nqFloat.Raw().Params)

	// Test with boolean
	qbBool := query.NewQueryBuilder().From("products").Where("in_stock").Eq(true)
	qBool := qbBool.Build()
	nqBool, errBool := builder.Build(&qBool, native.StmtSelect, nil)
	assert.NoError(t, errBool)
	assert.NotNil(t, nqBool)
	expectedSQLBool := `SELECT * FROM products WHERE "in_stock" = $1`
	assert.Equal(t, expectedSQLBool, nqBool.Raw().SQL)
	assert.Equal(t, []any{true}, nqBool.Raw().Params)
}

func stringp(s string) *string {
	return &s
}

func float64p(f float64) *float64 {
	return &f
}

func TestSelectWithDistinct(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	// Test DISTINCT on all fields
	qbAll := query.NewQueryBuilder().From("users").Distinct()
	qAll := qbAll.Build()
	nqAll, errAll := builder.Build(&qAll, native.StmtSelect, nil)
	assert.NoError(t, errAll)
	assert.NotNil(t, nqAll)
	expectedSQLAll := "SELECT DISTINCT * FROM users"
	assert.Equal(t, expectedSQLAll, nqAll.Raw().SQL)

	// Test DISTINCT on specific fields
	qbFields := query.NewQueryBuilder().From("users").Select().Include("country", "city").End().Distinct()
	qFields := qbFields.Build()
	nqFields, errFields := builder.Build(&qFields, native.StmtSelect, nil)
	assert.NoError(t, errFields)
	assert.NotNil(t, nqFields)
	expectedSQLFields := `SELECT DISTINCT "country", "city" FROM users`
	assert.Equal(t, expectedSQLFields, nqFields.Raw().SQL)
}

func TestSelectWithAggregations(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	qb := query.NewQueryBuilder().
		From("sales").
		Count("*", "total_sales").
		Sum("amount", "total_revenue").
		Avg("amount", "avg_sale").
		Min("amount", "min_sale").
		Max("amount", "max_sale").
		GroupBy("region").
		WithFilter(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "total_revenue", // This should refer to an alias
				Operator: query.ComparisonOperatorGt,
				Value:    query.FilterValue{NumberVal: float64p(1000)},
			},
		}).
		End()

	q := qb.Build()
	nq, err := builder.Build(&q, native.StmtSelect, nil)

	assert.NoError(t, err)
	assert.NotNil(t, nq)

	expectedSQL := `SELECT COUNT(*) AS total_sales, SUM("amount") AS total_revenue, AVG("amount") AS avg_sale, MIN("amount") AS min_sale, MAX("amount") AS max_sale FROM sales GROUP BY "region" HAVING "total_revenue" > $1`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Equal(t, []any{1000.0}, nq.Raw().Params)
}
