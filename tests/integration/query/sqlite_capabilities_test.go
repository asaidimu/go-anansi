package query_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	sqlite "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteCapabilities_LogicalOperators(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()
	schema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"name": {Name: "name", Type: schema.FieldTypeString},
			"age":  {Name: "age", Type: schema.FieldTypeInteger},
		},
	}

	// Test AND operator
	qbAnd := query.NewQueryBuilder().From("users").Schema(schema).Where("age").Gt(30).AndFilter(query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "name",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{StringVal: utils.StringPtr("John")},
		},
	})
	qAnd := qbAnd.Build()
	nqAnd, errAnd := builder.Build(&qAnd, native.StmtSelect, nil)
	require.NoError(t, errAnd)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name' FROM users WHERE ("age" > $1 AND "name" = $2)`, nqAnd.Raw().SQL)

	// Test OR operator
	qbOr := query.NewQueryBuilder().From("users").Schema(schema).Where("age").Gt(30).OrFilter(query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "name",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{StringVal: utils.StringPtr("John")},
		},
	})
	qOr := qbOr.Build()
	nqOr, errOr := builder.Build(&qOr, native.StmtSelect, nil)
	require.NoError(t, errOr)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name' FROM users WHERE ("age" > $1 OR "name" = $2)`, nqOr.Raw().SQL)
}

func TestSQLiteCapabilities_ComparisonOperators(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()
	schema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"name":    {Name: "name", Type: schema.FieldTypeString},
			"age":     {Name: "age", Type: schema.FieldTypeInteger},
			"status":  {Name: "status", Type: schema.FieldTypeString},
			"profile": {Name: "profile", Type: schema.FieldTypeString},
		},
	}

	// Test Eq
	qEq := query.NewQueryBuilder().From("users").Schema(schema).Where("name").Eq("John").Build()
	nqEq, _ := builder.Build(&qEq, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "name" = $1`, nqEq.Raw().SQL)

	// Test Neq
	qNeq := query.NewQueryBuilder().From("users").Schema(schema).Where("name").Neq("John").Build()
	nqNeq, _ := builder.Build(&qNeq, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "name" != $1`, nqNeq.Raw().SQL)

	// Test Lt
	qLt := query.NewQueryBuilder().From("users").Schema(schema).Where("age").Lt(30).Build()
	nqLt, _ := builder.Build(&qLt, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "age" < $1`, nqLt.Raw().SQL)

	// Test Lte
	qLte := query.NewQueryBuilder().From("users").Schema(schema).Where("age").Lte(30).Build()
	nqLte, _ := builder.Build(&qLte, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "age" <= $1`, nqLte.Raw().SQL)

	// Test Gt
	qGt := query.NewQueryBuilder().From("users").Schema(schema).Where("age").Gt(30).Build()
	nqGt, _ := builder.Build(&qGt, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "age" > $1`, nqGt.Raw().SQL)

	// Test Gte
	qGte := query.NewQueryBuilder().From("users").Schema(schema).Where("age").Gte(30).Build()
	nqGte, _ := builder.Build(&qGte, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "age" >= $1`, nqGte.Raw().SQL)

	// Test In
	qIn := query.NewQueryBuilder().From("users").Schema(schema).Where("status").In("active", "pending").Build()
	nqIn, _ := builder.Build(&qIn, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "status" IN ($1, $2)`, nqIn.Raw().SQL)

	// Test Nin
	qNin := query.NewQueryBuilder().From("users").Schema(schema).Where("status").Nin("inactive", "archived").Build()
	nqNin, _ := builder.Build(&qNin, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "status" NOT IN ($1, $2)`, nqNin.Raw().SQL)

	// Test Contains
	qContains := query.NewQueryBuilder().From("users").Schema(schema).Where("name").Contains("John").Build()
	nqContains, _ := builder.Build(&qContains, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "name" LIKE '%' || $1 || '%'`, nqContains.Raw().SQL)

	// Test NotContains
	qNotContains := query.NewQueryBuilder().From("users").Schema(schema).Where("name").NotContains("John").Build()
	nqNotContains, _ := builder.Build(&qNotContains, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "name" NOT LIKE '%' || $1 || '%'`, nqNotContains.Raw().SQL)

	// Test Exists
	qExists := query.NewQueryBuilder().From("users").Schema(schema).Where("profile").Exists().Build()
	nqExists, _ := builder.Build(&qExists, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "profile" IS NOT NULL`, nqExists.Raw().SQL)

	// Test NotExists
	qNotExists := query.NewQueryBuilder().From("users").Schema(schema).Where("profile").NotExists().Build()
	nqNotExists, _ := builder.Build(&qNotExists, native.StmtSelect, nil)
	assert.Equal(t, `SELECT users.age AS 'users.age', users.name AS 'users.name', users.profile AS 'users.profile', users.status AS 'users.status' FROM users WHERE "profile" IS NULL`, nqNotExists.Raw().SQL)
}

func TestSQLiteCapabilities_ExpressionOperators(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()
	schema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"age":      {Name: "age", Type: schema.FieldTypeInteger},
			"salary":   {Name: "salary", Type: schema.FieldTypeInteger},
			"bonus":    {Name: "bonus", Type: schema.FieldTypeInteger},
			"expenses": {Name: "expenses", Type: schema.FieldTypeInteger},
		},
	}

	// Test ADD
	qAdd := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("total_income", "ADD", &query.FieldReference{Field: "salary"}, &query.FieldReference{Field: "bonus"}).End().Build()
	nqAdd, _ := builder.Build(&qAdd, native.StmtSelect, nil)
	assert.Equal(t, `SELECT ADD("salary", "bonus") AS total_income FROM users`, nqAdd.Raw().SQL)

	// Test SUBTRACT
	qSubtract := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("net_income", "SUBTRACT", &query.FieldReference{Field: "salary"}, &query.FieldReference{Field: "expenses"}).End().Build()
	nqSubtract, _ := builder.Build(&qSubtract, native.StmtSelect, nil)
	assert.Equal(t, `SELECT SUBTRACT("salary", "expenses") AS net_income FROM users`, nqSubtract.Raw().SQL)

	// Test MULTIPLY
	qMultiply := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("gross_salary", "MULTIPLY", &query.FieldReference{Field: "salary"}, 1.2).End().Build()
	nqMultiply, _ := builder.Build(&qMultiply, native.StmtSelect, nil)
	assert.Equal(t, `SELECT MULTIPLY("salary", $1) AS gross_salary FROM users`, nqMultiply.Raw().SQL)

	// Test DIVIDE
	qDivide := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("monthly_salary", "DIVIDE", &query.FieldReference{Field: "salary"}, 12).End().Build()
	nqDivide, _ := builder.Build(&qDivide, native.StmtSelect, nil)
	assert.Equal(t, `SELECT DIVIDE("salary", $1) AS monthly_salary FROM users`, nqDivide.Raw().SQL)
}

func TestSQLiteCapabilities_Functions(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()
	schema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"name":    {Name: "name", Type: schema.FieldTypeString},
			"profile": {Name: "profile", Type: schema.FieldTypeObject},
			"age":     {Name: "age", Type: schema.FieldTypeInteger},
		},
	}

	// Test json_extract
	qJSONExtract := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("city", "json_extract", &query.FieldReference{Field: "profile"}, "$.address.city").End().Build()
	nqJSONExtract, err := builder.Build(&qJSONExtract, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT json_extract("profile", $1) AS city FROM users`, nqJSONExtract.Raw().SQL)

	// Test json_valid
	qJSONValid := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("is_valid_profile", "json_valid", &query.FieldReference{Field: "profile"}).End().Build()
	nqJSONValid, err := builder.Build(&qJSONValid, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT json_valid("profile") AS is_valid_profile FROM users`, nqJSONValid.Raw().SQL)

	// Test UPPER
	qUpper := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("upper_name", "UPPER", &query.FieldReference{Field: "name"}).End().Build()
	nqUpper, err := builder.Build(&qUpper, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT UPPER("name") AS upper_name FROM users`, nqUpper.Raw().SQL)

	// Test LOWER
	qLower := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("lower_name", "LOWER", &query.FieldReference{Field: "name"}).End().Build()
	nqLower, err := builder.Build(&qLower, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT LOWER("name") AS lower_name FROM users`, nqLower.Raw().SQL)

	// Test LENGTH
	qLength := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("name_length", "LENGTH", &query.FieldReference{Field: "name"}).End().Build()
	nqLength, err := builder.Build(&qLength, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT LENGTH("name") AS name_length FROM users`, nqLength.Raw().SQL)

	// Test SUBSTR
	qSubstr := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("name_substr", "SUBSTR", &query.FieldReference{Field: "name"}, 1, 3).End().Build()
	nqSubstr, err := builder.Build(&qSubstr, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT SUBSTR("name", $1, $2) AS name_substr FROM users`, nqSubstr.Raw().SQL)

	// Test ABS
	qAbs := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("abs_age", "ABS", &query.FieldReference{Field: "age"}).End().Build()
	nqAbs, err := builder.Build(&qAbs, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT ABS("age") AS abs_age FROM users`, nqAbs.Raw().SQL)

	// Test ROUND
	qRound := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("round_age", "ROUND", &query.FieldReference{Field: "age"}).End().Build()
	nqRound, err := builder.Build(&qRound, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT ROUND("age") AS round_age FROM users`, nqRound.Raw().SQL)

	// Test datetime
	qDatetime := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("now", "datetime", "now").End().Build()
	nqDatetime, err := builder.Build(&qDatetime, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT datetime($1) AS now FROM users`, nqDatetime.Raw().SQL)

	// Test date
	qDate := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("today", "date", "now").End().Build()
	nqDate, err := builder.Build(&qDate, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT date($1) AS today FROM users`, nqDate.Raw().SQL)
}

func TestSQLiteCapabilities_JoinTypes(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()
	userSchema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString},
			"name": {Name: "name", Type: schema.FieldTypeString},
		},
	}
	profileSchema := &schema.SchemaDefinition{
		Name: "profiles",
		Fields: map[string]*schema.FieldDefinition{
			"user_id": {Name: "user_id", Type: schema.FieldTypeString},
			"bio":     {Name: "bio", Type: schema.FieldTypeString},
		},
	}

	// Test INNER JOIN
	qInnerJoin := query.NewQueryBuilder().From("users").Schema(userSchema).InnerJoin("profiles").Schema(profileSchema).On(query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "users.id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "profiles.user_id"}},
		},
	}).End().Build()
	nqInnerJoin, err := builder.Build(&qInnerJoin, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT profiles.bio AS 'profiles.bio', profiles.user_id AS 'profiles.user_id', users.id AS 'users.id', users.name AS 'users.name' FROM users INNER JOIN profiles ON "users"."id" = "profiles"."user_id"`, nqInnerJoin.Raw().SQL)

	// Test LEFT JOIN
	qLeftJoin := query.NewQueryBuilder().From("users").Schema(userSchema).LeftJoin("profiles").Schema(profileSchema).On(query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "users.id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "profiles.user_id"}},
		},
	}).End().Build()
	nqLeftJoin, err := builder.Build(&qLeftJoin, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT profiles.bio AS 'profiles.bio', profiles.user_id AS 'profiles.user_id', users.id AS 'users.id', users.name AS 'users.name' FROM users LEFT JOIN profiles ON "users"."id" = "profiles"."user_id"`, nqLeftJoin.Raw().SQL)
}

func TestSQLiteCapabilities_AggregationFunctions(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()
	schema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"age":    {Name: "age", Type: schema.FieldTypeInteger},
			"salary": {Name: "salary", Type: schema.FieldTypeNumber},
		},
	}

	// Test COUNT
	qCount := query.NewQueryBuilder().From("users").Schema(schema).Aggregate(query.AggregationTypeCount, "*", "total_users").Build()
	nqCount, err := builder.Build(&qCount, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT COUNT(*) AS total_users FROM users`, nqCount.Raw().SQL)

	// Test SUM
	qSum := query.NewQueryBuilder().From("users").Schema(schema).Aggregate(query.AggregationTypeSum, "salary", "total_salary").Build()
	nqSum, err := builder.Build(&qSum, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT SUM("salary") AS total_salary FROM users`, nqSum.Raw().SQL)

	// Test AVG
	qAvg := query.NewQueryBuilder().From("users").Schema(schema).Aggregate(query.AggregationTypeAvg, "age", "avg_age").Build()
	nqAvg, err := builder.Build(&qAvg, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT AVG("age") AS avg_age FROM users`, nqAvg.Raw().SQL)

	// Test MIN
	qMin := query.NewQueryBuilder().From("users").Schema(schema).Aggregate(query.AggregationTypeMin, "age", "min_age").Build()
	nqMin, err := builder.Build(&qMin, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT MIN("age") AS min_age FROM users`, nqMin.Raw().SQL)

	// Test MAX
	qMax := query.NewQueryBuilder().From("users").Schema(schema).Aggregate(query.AggregationTypeMax, "age", "max_age").Build()
	nqMax, err := builder.Build(&qMax, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT MAX("age") AS max_age FROM users`, nqMax.Raw().SQL)
}

func TestSQLiteCapabilities_Pagination(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()
	schema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"name": {Name: "name", Type: schema.FieldTypeString},
			"age":  {Name: "age", Type: schema.FieldTypeInteger},
		},
	}

	// Test Limit
	qLimit := query.NewQueryBuilder().From("users").Schema(schema).Limit(10).Build()
	nqLimit, err := builder.Build(&qLimit, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Contains(t, nqLimit.Raw().SQL, "LIMIT 10")

	// Test Offset
	qOffset := query.NewQueryBuilder().From("users").Schema(schema).Limit(10).Offset(20).Build()
	nqOffset, err := builder.Build(&qOffset, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Contains(t, nqOffset.Raw().SQL, "LIMIT 10 OFFSET 20")
}

func TestSQLiteCapabilities_TextSearch(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()
	schema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"name": {Name: "name", Type: schema.FieldTypeString},
			"bio":  {Name: "bio", Type: schema.FieldTypeString},
		},
	}

	// Test Contains
	qContains := query.NewQueryBuilder().From("users").Schema(schema).TextSearch("name").Contains("John").Build()
	nqContains, err := builder.Build(&qContains, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Equal(t, `SELECT users.bio AS 'users.bio', users.name AS 'users.name' FROM users WHERE (LOWER("name") LIKE $1)`, nqContains.Raw().SQL)
}

func TestSQLiteCapabilities_Sorting(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()
	schema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"name": {Name: "name", Type: schema.FieldTypeString},
			"age":  {Name: "age", Type: schema.FieldTypeInteger},
		},
	}

	// Test NULLS FIRST
	qNullsFirst := query.NewQueryBuilder().From("users").Schema(schema).OrderBy("age", query.SortDirectionDesc).ThenSortBy("name", query.SortDirectionAsc).Build()
	nqNullsFirst, err := builder.Build(&qNullsFirst, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Contains(t, nqNullsFirst.Raw().SQL, "ORDER BY \"age\" DESC, \"name\" ASC")

	// Test NULLS LAST
	qNullsLast := query.NewQueryBuilder().From("users").Schema(schema).OrderBy("age", query.SortDirectionAsc).ThenSortBy("name", query.SortDirectionDesc).Build()
	nqNullsLast, err := builder.Build(&qNullsLast, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Contains(t, nqNullsLast.Raw().SQL, "ORDER BY \"age\" ASC, \"name\" DESC")

	// Test Sort by Expression
	qSortByExpr := query.NewQueryBuilder().From("users").Schema(schema).Select().AddComputed("age_plus_10", "ADD", &query.FieldReference{Field: "age"}, 10).End().OrderBy("age_plus_10", query.SortDirectionDesc).Build()
	nqSortByExpr, err := builder.Build(&qSortByExpr, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Contains(t, nqSortByExpr.Raw().SQL, "ORDER BY \"age_plus_10\" DESC")
}

func TestSQLiteCapabilities_OtherFeatures(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()
	schema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"name":    {Name: "name", Type: schema.FieldTypeString},
			"age":     {Name: "age", Type: schema.FieldTypeInteger},
			"profile": {Name: "profile", Type: schema.FieldTypeObject},
		},
	}

	// Test GROUP BY
	qGroupBy := query.NewQueryBuilder().From("users").Schema(schema).Aggregate(query.AggregationTypeCount, "*", "total_users").GroupBy("age").End().Build()
	nqGroupBy, err := builder.Build(&qGroupBy, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Contains(t, nqGroupBy.Raw().SQL, "GROUP BY \"age\"")

	// Test DISTINCT
	qDistinct := query.NewQueryBuilder().From("users").Schema(schema).Distinct().Build()
	nqDistinct, err := builder.Build(&qDistinct, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Contains(t, nqDistinct.Raw().SQL, "SELECT DISTINCT")

	// Test Nested Fields
	qNested := query.NewQueryBuilder().From("users").Schema(schema).Where("profile.address.city").Eq("Nairobi").Build()
	nqNested, err := builder.Build(&qNested, native.StmtSelect, nil)
	require.NoError(t, err)
	assert.Contains(t, nqNested.Raw().SQL, "json_extract(\"profile\", '$.address.city') = $1")
}