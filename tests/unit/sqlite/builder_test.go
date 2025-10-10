package sqlite_test

import (
	"os"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	sqlite "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/asaidimu/go-anansi/v6/tests/testutils"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	testutils.ConfigureDocumentFactory()
	os.Exit(m.Run())
}

func TestSelect(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	qb := query.NewQueryBuilder()
	qb.From("users").Schema(&schema.SchemaDefinition{}).
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
	userSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"first_name": {Name: "first_name", Type: schema.FieldTypeString},
			"last_name":  {Name: "last_name", Type: schema.FieldTypeString},
			"age":        {Name: "age", Type: schema.FieldTypeInteger},
		},
	}
	qb.From("users").Schema(userSchema)

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
	// Fields are now in alphabetical order: age, first_name, last_name
	assert.Equal(t, 30, nq.Raw().Params[0])
	assert.Equal(t, "John", nq.Raw().Params[1])
	assert.Equal(t, "Doe", nq.Raw().Params[2])
}

func TestUpdate(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	qb := query.NewQueryBuilder()
	qb.From("users").Schema(&schema.SchemaDefinition{}).
		Where("id").Eq("user-123")

	q := qb.Build()
	data := data.Document{
		"age":   31,
		"email": "john.doe@example.com",
	}

	nq, err := builder.Build(&q, native.StmtUpdate, map[string]any{
		"set": data,
	})
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	expectedSQL := `UPDATE "users" SET "age" = $1, "email" = $2 WHERE "id" = $3`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Equal(t, 3, len(nq.Raw().Params))
	assert.Equal(t, 31, nq.Raw().Params[0])
	assert.Equal(t, "john.doe@example.com", nq.Raw().Params[1])
	assert.Equal(t, "user-123", nq.Raw().Params[2])
}

func TestDelete(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	userSchema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"id":    {Name: "id", Type: schema.FieldTypeString},
			"age":   {Name: "age", Type: schema.FieldTypeInteger},
			"email": {Name: "email", Type: schema.FieldTypeString},
		},
	}

	qb := query.NewQueryBuilder()
	qb.From("users").
		Schema(userSchema).
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

	orderSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"id":           {Name: "id", Type: schema.FieldTypeString},
			"order_date":   {Name: "order_date", Type: schema.FieldTypeString},
			"total_amount": {Name: "total_amount", Type: schema.FieldTypeNumber},
			"customer_id":  {Name: "customer_id", Type: schema.FieldTypeString},
		},
	}

	customerSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"id":     {Name: "id", Type: schema.FieldTypeString},
			"region": {Name: "region", Type: schema.FieldTypeString},
		},
	}

	qb := query.NewQueryBuilder()
	qb.From("orders").Schema(orderSchema).
		Select().
		Include("id", "order_date", "total_amount").
		End().
		InnerJoin("customers").Schema(customerSchema).
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

	userSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"region": {Name: "region", Type: schema.FieldTypeString},
			"status": {Name: "status", Type: schema.FieldTypeString},
		},
	}

	// Test IN operator
	qbIn := query.NewQueryBuilder().From("users").Schema(userSchema).Where("region").In("East", "West")
	qIn := qbIn.Build()
	nqIn, errIn := builder.Build(&qIn, native.StmtSelect, nil)
	assert.NoError(t, errIn)
	assert.NotNil(t, nqIn)
	// Fields in alphabetical order: region, status
	expectedSQLIn := `SELECT users.region AS 'users.region', users.status AS 'users.status' FROM users WHERE "region" IN ($1, $2)`
	assert.Equal(t, expectedSQLIn, nqIn.Raw().SQL)
	assert.ElementsMatch(t, []any{"East", "West"}, nqIn.Raw().Params)

	// Test NOT IN operator
	qbNotIn := query.NewQueryBuilder().From("users").Schema(userSchema).Where("status").Nin("inactive", "pending")
	qNotIn := qbNotIn.Build()
	nqNotIn, errNotIn := builder.Build(&qNotIn, native.StmtSelect, nil)
	assert.NoError(t, errNotIn)
	assert.NotNil(t, nqNotIn)
	// Fields in alphabetical order: region, status
	expectedSQLNotIn := `SELECT users.region AS 'users.region', users.status AS 'users.status' FROM users WHERE "status" NOT IN ($1, $2)`
	assert.Equal(t, expectedSQLNotIn, nqNotIn.Raw().SQL)
	assert.ElementsMatch(t, []any{"inactive", "pending"}, nqNotIn.Raw().Params)
}

func TestSelectWithMultipleJoins(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	orderSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"customer_id": {Name: "customer_id", Type: schema.FieldTypeString},
			"product_id":  {Name: "product_id", Type: schema.FieldTypeString},
		},
	}

	userSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"id": {Name: "id", Type: schema.FieldTypeString},
		},
	}

	productSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"id": {Name: "id", Type: schema.FieldTypeString},
		},
	}

	qb := query.NewQueryBuilder().
		From("orders").Schema(orderSchema).
		InnerJoin("users").Schema(userSchema).On(query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "orders.customer_id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "users.id"}},
		},
	}).End().
		LeftJoin("products").Schema(productSchema).On(query.QueryFilter{
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

	// Fields in alphabetical order by table: orders (customer_id, product_id), products (id), users (id)
	expectedSQL := `SELECT orders.customer_id AS 'orders.customer_id', orders.product_id AS 'orders.product_id', products.id AS 'products.id', users.id AS 'users.id' FROM orders INNER JOIN users ON "orders"."customer_id" = "users"."id" LEFT JOIN products ON "orders"."product_id" = "products"."id"`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Empty(t, nq.Raw().Params)
}

func TestSelectWithCaseStatement(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	userSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"first_name": {Name: "first_name", Type: schema.FieldTypeString},
			"last_name":  {Name: "last_name", Type: schema.FieldTypeString},
			"status":     {Name: "status", Type: schema.FieldTypeString},
		},
	}

	qb := query.NewQueryBuilder().
		From("users").Schema(userSchema).
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

	userSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"age": {Name: "age", Type: schema.FieldTypeInteger},
		},
	}

	productSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"price":    {Name: "price", Type: schema.FieldTypeNumber},
			"in_stock": {Name: "in_stock", Type: schema.FieldTypeBoolean},
		},
	}

	// Test with integer
	qbInt := query.NewQueryBuilder().From("users").Schema(userSchema).Where("age").Gt(30)
	qInt := qbInt.Build()
	nqInt, errInt := builder.Build(&qInt, native.StmtSelect, nil)
	assert.NoError(t, errInt)
	assert.NotNil(t, nqInt)
	expectedSQLInt := `SELECT users.age AS 'users.age' FROM users WHERE "age" > $1`
	assert.Equal(t, expectedSQLInt, nqInt.Raw().SQL)
	assert.Equal(t, []any{float64(30)}, nqInt.Raw().Params)

	// Test with float - fields in alphabetical order: in_stock, price
	qbFloat := query.NewQueryBuilder().From("products").Schema(productSchema).Where("price").Lte(9.99)
	qFloat := qbFloat.Build()
	nqFloat, errFloat := builder.Build(&qFloat, native.StmtSelect, nil)
	assert.NoError(t, errFloat)
	assert.NotNil(t, nqFloat)
	expectedSQLFloat := `SELECT products.in_stock AS 'products.in_stock', products.price AS 'products.price' FROM products WHERE "price" <= $1`
	assert.Equal(t, expectedSQLFloat, nqFloat.Raw().SQL)
	assert.Equal(t, []any{9.99}, nqFloat.Raw().Params)

	// Test with boolean - fields in alphabetical order: in_stock, price
	qbBool := query.NewQueryBuilder().From("products").Schema(productSchema).Where("in_stock").Eq(true)
	qBool := qbBool.Build()
	nqBool, errBool := builder.Build(&qBool, native.StmtSelect, nil)
	assert.NoError(t, errBool)
	assert.NotNil(t, nqBool)
	expectedSQLBool := `SELECT products.in_stock AS 'products.in_stock', products.price AS 'products.price' FROM products WHERE "in_stock" = $1`
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

	userSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"country": {Name: "country", Type: schema.FieldTypeString},
			"city":    {Name: "city", Type: schema.FieldTypeString},
		},
	}

	// Test DISTINCT on all fields - alphabetical order: city, country
	qbAll := query.NewQueryBuilder().From("users").Schema(userSchema).Distinct()
	qAll := qbAll.Build()
	nqAll, errAll := builder.Build(&qAll, native.StmtSelect, nil)
	assert.NoError(t, errAll)
	assert.NotNil(t, nqAll)
	expectedSQLAll := `SELECT DISTINCT users.city AS 'users.city', users.country AS 'users.country' FROM users`
	assert.Equal(t, expectedSQLAll, nqAll.Raw().SQL)

	// Test DISTINCT on specific fields - order as specified: country, city
	qbFields := query.NewQueryBuilder().From("users").Schema(userSchema).Select().Include("country", "city").End().Distinct()
	qFields := qbFields.Build()
	nqFields, errFields := builder.Build(&qFields, native.StmtSelect, nil)
	assert.NoError(t, errFields)
	assert.NotNil(t, nqFields)
	expectedSQLFields := `SELECT DISTINCT "country", "city" FROM users`
	assert.Equal(t, expectedSQLFields, nqFields.Raw().SQL)
}

func TestSelectWithAggregations(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	salesSchema := &schema.SchemaDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"amount": {Name: "amount", Type: schema.FieldTypeNumber},
			"region": {Name: "region", Type: schema.FieldTypeString},
		},
	}

	qb := query.NewQueryBuilder().
		From("sales").Schema(salesSchema).
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

func TestSQLiteFactory_SelectImplicitFields(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	// Define a schema for the collection
	accountsSchema := &schema.SchemaDefinition{
		Name: "accounts",
		Fields: map[string]*schema.FieldDefinition{
			"id":      {Name: "id", Type: schema.FieldTypeString},
			"name":    {Name: "name", Type: schema.FieldTypeString},
			"balance": {Name: "balance", Type: schema.FieldTypeNumber},
		},
	}

	// Create a query without explicit Select fields, but with a Where clause
	qb := query.NewQueryBuilder().
		From("accounts").Schema(accountsSchema).
		Where("id").Eq("A")

	q := qb.Build()

	// Build the native query
	nq, err := builder.Build(&q, native.StmtSelect, nil)
	assert.NoError(t, err)
	assert.NotNil(t, nq)

	// Assert the generated SQL
	// The SELECT clause should implicitly include all fields from the schema in alphabetical order: balance, id, name
	expectedSQL := `SELECT accounts.balance AS 'accounts.balance', accounts.id AS 'accounts.id', accounts.name AS 'accounts.name' FROM accounts WHERE "id" = $1`
	assert.Equal(t, expectedSQL, nq.Raw().SQL)
	assert.Equal(t, 1, len(nq.Raw().Params))
	assert.Equal(t, "A", nq.Raw().Params[0])
}

