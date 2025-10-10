package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	sqlite "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDALTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Create users table
	_, err = db.Exec(`
		CREATE TABLE users_1_0_0 (
			id TEXT PRIMARY KEY,
			first_name TEXT,
			last_name TEXT,
			email TEXT,
			age INTEGER,
			status TEXT,
			region TEXT
		)
	`)
	require.NoError(t, err)

	// Create orders table
	_, err = db.Exec(`
		CREATE TABLE orders_1_0_0 (
			id TEXT PRIMARY KEY,
			customer_id TEXT,
			order_date TEXT,
			total_amount REAL
		)
	`)
	require.NoError(t, err)

	// Create sales table
	_, err = db.Exec(`
		CREATE TABLE sales (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			region TEXT,
			amount REAL
		)
	`)
	require.NoError(t, err)

	// Insert sample data
	_, err = db.Exec(`
		INSERT INTO users_1_0_0 (id, first_name, last_name, email, age, status, region) VALUES
		('user-1', 'John', 'Doe', 'john.doe@example.com', 30, 'active', 'West'),
		('user-2', 'Jane', 'Smith', 'jane.smith@example.com', 28, 'inactive', 'East')
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO orders_1_0_0 (id, customer_id, order_date, total_amount) VALUES
		('order-1', 'user-1', '2024-01-15', 150.50),
		('order-2', 'user-1', '2024-02-20', 75.00),
		('order-3', 'user-2', '2024-03-10', 200.00)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO sales (region, amount) VALUES
		('North', 100.0),
		('North', 150.0),
		('South', 200.0),
		('South', 250.0),
		('East', 50.0),
		('West', 300.0)
	`)
	require.NoError(t, err)

	return db
}

func TestInsert_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	builder := sqlite.NewSQLiteFactory()

	usersSchema := &schema.SchemaDefinition{
		Name: "users_1_0_0",
		Fields: map[string]*schema.FieldDefinition{
			"id":         {Name: "id", Type: schema.FieldTypeString},
			"first_name": {Name: "first_name", Type: schema.FieldTypeString},
			"last_name":  {Name: "last_name", Type: schema.FieldTypeString},
			"email":      {Name: "email", Type: schema.FieldTypeString},
			"age":        {Name: "age", Type: schema.FieldTypeInteger},
			"status":     {Name: "status", Type: schema.FieldTypeString},
			"region":     {Name: "region", Type: schema.FieldTypeString},
		},
	}

	qb := query.NewQueryBuilder().From("users_1_0_0").Alias("users").Schema(usersSchema)
	q := qb.Build()

	data := data.Document{
		"id":         "user-3",
		"first_name": "Peter",
		"last_name":  "Jones",
		"age":        45,
	}

	nq, err := builder.Build(&q, native.StmtInsert, data)
	require.NoError(t, err)

	_, err = db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	// Verify insertion
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users_1_0_0 WHERE id = 'user-3'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestUpdate_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	builder := sqlite.NewSQLiteFactory()
	usersSchema := &schema.SchemaDefinition{
		Name: "users_1_0_0",
		Fields: map[string]*schema.FieldDefinition{
			"id":         {Name: "id", Type: schema.FieldTypeString},
			"first_name": {Name: "first_name", Type: schema.FieldTypeString},
			"last_name":  {Name: "last_name", Type: schema.FieldTypeString},
			"email":      {Name: "email", Type: schema.FieldTypeString},
			"age":        {Name: "age", Type: schema.FieldTypeInteger},
			"status":     {Name: "status", Type: schema.FieldTypeString},
			"region":     {Name: "region", Type: schema.FieldTypeString},
		},
	}
	qb := query.NewQueryBuilder().From("users_1_0_0").Alias("users").Schema(usersSchema).Where("id").Eq("user-1")
	q := qb.Build()

	data := data.Document{"age": 31}

	nq, err := builder.Build(&q, native.StmtUpdate, map[string]any{ "set": data })
	require.NoError(t, err)

	res, err := db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify update
	var age int
	err = db.QueryRow("SELECT age FROM users_1_0_0 WHERE id = 'user-1'").Scan(&age)
	require.NoError(t, err)
	assert.Equal(t, 31, age)
}

func TestDelete_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	builder := sqlite.NewSQLiteFactory()
	usersSchema := &schema.SchemaDefinition{
		Name: "users_1_0_0",
		Fields: map[string]*schema.FieldDefinition{
			"id":         {Name: "id", Type: schema.FieldTypeString},
			"first_name": {Name: "first_name", Type: schema.FieldTypeString},
			"last_name":  {Name: "last_name", Type: schema.FieldTypeString},
			"email":      {Name: "email", Type: schema.FieldTypeString},
			"age":        {Name: "age", Type: schema.FieldTypeInteger},
			"status":     {Name: "status", Type: schema.FieldTypeString},
			"region":     {Name: "region", Type: schema.FieldTypeString},
		},
	}
	qb := query.NewQueryBuilder().From("users_1_0_0").Alias("users").Schema(usersSchema).Where("id").Eq("user-2")
	q := qb.Build()

	nq, err := builder.Build(&q, native.StmtDelete, nil)
	require.NoError(t, err)

	res, err := db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify deletion
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users_1_0_0 WHERE id = 'user-2'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestComplexTypes_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		CREATE TABLE complex_docs_01 (
			id TEXT PRIMARY KEY,
			tags TEXT,
			metadata TEXT
		)
	`)
	require.NoError(t, err)

	builder := sqlite.NewSQLiteFactory()

	complexDocsSchema := &schema.SchemaDefinition{
		Name: "complex_docs_01",
		Fields: map[string]*schema.FieldDefinition{
			"id":       {Name: "id", Type: schema.FieldTypeString},
			"tags":     {Name: "tags", Type: schema.FieldTypeArray},
			"metadata": {Name: "metadata", Type: schema.FieldTypeObject},
		},
	}

	// Insert
	qb := query.NewQueryBuilder().From("complex_docs_01").Alias("complex_docs").Schema(complexDocsSchema)
	q := qb.Build()

	tags := []string{"go", "sqlite", "testing"}
	metadata := data.Document{"author": "Augustine", "version": 2}
	insertData := data.Document{"id": "doc-1", "tags": tags, "metadata": metadata}

	nq, err := builder.Build(&q, native.StmtInsert, insertData)
	require.NoError(t, err)

	_, err = db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	// Verify Insert
	var id, rawTags, rawMetadata string
	err = db.QueryRow("SELECT id, tags, metadata FROM complex_docs_01 WHERE id = 'doc-1'").Scan(&id, &rawTags, &rawMetadata)
	require.NoError(t, err)
	assert.Equal(t, "doc-1", id)
	assert.JSONEq(t, `["go", "sqlite", "testing"]`, rawTags)
	assert.JSONEq(t, `{"author": "Augustine", "version": 2}`, rawMetadata)

	// Update
	updatedTags := []string{"go", "testing", "updated"}
	updateData := data.Document{"tags": updatedTags}
	q = query.NewQueryBuilder().From("complex_docs_01").Alias("complex_docs").Schema(complexDocsSchema).Where("id").Eq("doc-1").Build()

	nq, err = builder.Build(&q, native.StmtUpdate,  map[string]any{ "set": updateData })
	require.NoError(t, err)

	_, err = db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	// Verify Update
	err = db.QueryRow("SELECT tags FROM complex_docs_01 WHERE id = 'doc-1'").Scan(&rawTags)
	require.NoError(t, err)
	assert.JSONEq(t, `["go", "testing", "updated"]`, rawTags)

	// Select with nested field
	q = query.NewQueryBuilder().From("complex_docs_01").Alias("complex_docs").Schema(complexDocsSchema).Where("complex_docs.metadata.version").Eq(2).Build()
	nq, err = builder.Build(&q, native.StmtSelect, nil)
	require.NoError(t, err)

	rows, err := db.Query(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err, "SQL query failed: %s", nq.Raw().SQL)
	defer rows.Close()

	var count int
	for rows.Next() {
		count++
	}
	assert.Equal(t, 1, count)
}

func TestSelectComplex_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	builder := sqlite.NewSQLiteFactory()

	ordersSchema := &schema.SchemaDefinition{
		Name: "orders_1_0_0",
		Fields: map[string]*schema.FieldDefinition{
			"id":           {Name: "id", Type: schema.FieldTypeString},
			"customer_id":  {Name: "customer_id", Type: schema.FieldTypeString},
			"order_date":   {Name: "order_date", Type: schema.FieldTypeString},
			"total_amount": {Name: "total_amount", Type: schema.FieldTypeNumber},
		},
	}

	usersSchema := &schema.SchemaDefinition{
		Name: "users_1_0_0",
		Fields: map[string]*schema.FieldDefinition{
			"id":         {Name: "id", Type: schema.FieldTypeString},
			"first_name": {Name: "first_name", Type: schema.FieldTypeString},
			"last_name":  {Name: "last_name", Type: schema.FieldTypeString},
			"email":      {Name: "email", Type: schema.FieldTypeString},
			"age":        {Name: "age", Type: schema.FieldTypeInteger},
			"status":     {Name: "status", Type: schema.FieldTypeString},
			"region":     {Name: "region", Type: schema.FieldTypeString},
		},
	}

	qb := query.NewQueryBuilder().From("orders_1_0_0").Alias("orders").Schema(ordersSchema).
		Select().
		Include("orders.id", "total_amount").
		End().
		InnerJoin("users_1_0_0").Alias("users").Schema(usersSchema).
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "orders.customer_id",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "users.id"}},
			},
		}).
		End().
		WhereGroup(common.LogicalAnd).
		Where("users.region").Eq("West").
		Where("total_amount").Gte(100.0).
		End().
		OrderByDesc("total_amount")

	q := qb.Build()

	nq, err := builder.Build(&q, native.StmtSelect, nil)
	require.NoError(t, err)

	rows, err := db.Query(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)
	defer rows.Close()

	var results []data.Document
	for rows.Next() {
		var id string
		var totalAmount float64
		err := rows.Scan(&id, &totalAmount)
		require.NoError(t, err)
		results = append(results, data.Document{"id": id, "total_amount": totalAmount})
	}

	require.NoError(t, rows.Err())
	assert.Len(t, results, 1)
	assert.Equal(t, "order-1", results[0]["id"])
	assert.Equal(t, 150.50, results[0]["total_amount"])
}

func TestSelectWithNestedFieldInJoin_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	// Add a profile column to the users table and insert data
	_, err := db.Exec(`ALTER TABLE users_1_0_0 ADD COLUMN profile TEXT;`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE users_1_0_0 SET profile = ? WHERE id = ?`, `{"preferences": {"theme": "dark"}, "level": 5}`, "user-1")
	require.NoError(t, err)

	builder := sqlite.NewSQLiteFactory()

	// Define the schema for the users table, marking 'profile' as a complex object
	userSchema := &schema.SchemaDefinition{
		Name: "users_1_0_0",
		Fields: map[string]*schema.FieldDefinition{
			"id":      {Name: "id", Type: schema.FieldTypeString},
			"profile": {Name: "profile", Type: schema.FieldTypeRecord},
		},
	}

	// Define the schema for the users table, marking 'profile' as a complex object
	ordersSchema := &schema.SchemaDefinition{
		Name: "orders_1_0_0",
		Fields: map[string]*schema.FieldDefinition{
			"id":           {Name: "id", Type: schema.FieldTypeString},
			"customer_id":  {Name: "customer_id", Type: schema.FieldTypeString},
			"order_date":   {Name: "order_date", Type: schema.FieldTypeString},
			"total_amount": {Name: "total_amount", Type: schema.FieldTypeNumber},
		},
	}

	// Build the query with a JOIN and a filter on a nested, aliased field
	qb := query.NewQueryBuilder().
		From("orders_1_0_0").Alias("o").
		InnerJoin("users_1_0_0").Alias("u").
		Schema(userSchema).
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "o.customer_id",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "u.id"}},
			},
		}).
		End().
		Where("u.profile.preferences.theme").Eq("dark")

	q := qb.Build()

	q.Target.Schema = ordersSchema
	nq, err := builder.Build(&q, native.StmtSelect, nil)
	require.NoError(t, err)

	rows, err := db.Query(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err, "SQL query failed: %s", nq.Raw().SQL)
	defer rows.Close()

	var count int
	for rows.Next() {
		count++
	}
	assert.Equal(t, 2, count, "Should find two orders for the user with the 'dark' theme preference")
}

func TestUpdateWithNestedField_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	// Create a table with a JSON column
	_, err := db.Exec(`
		CREATE TABLE docs (
			id TEXT PRIMARY KEY,
			metadata TEXT,
			status TEXT
		)
	`)
	require.NoError(t, err)

	// Insert sample data
	_, err = db.Exec(`
		INSERT INTO docs (id, metadata, status) VALUES
		('doc-1', '{"version": 1, "author": "John"}', 'pending'),
		('doc-2', '{"version": 2, "author": "Jane"}', 'pending')
	`)
	require.NoError(t, err)

	builder := sqlite.NewSQLiteFactory()

	// Define the schema for the docs table
	docSchema := &schema.SchemaDefinition{
		Name: "docs",
		Fields: map[string]*schema.FieldDefinition{
			"id":       {Name: "id", Type: schema.FieldTypeString},
			"metadata": {Name: "metadata", Type: schema.FieldTypeObject},
			"status":   {Name: "status", Type: schema.FieldTypeString},
		},
	}

	// Build the update query
	qb := query.NewQueryBuilder().
		From("docs").
		Where("docs.metadata.version").Eq(2)

	q := qb.Build()
	q.Target.Schema = docSchema
	updateData := data.Document{"status": "approved"}

	nq, err := builder.Build(&q, native.StmtUpdate, map[string]any{ "set": updateData })
	require.NoError(t, err)

	// Execute the query
	res, err := db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify the update
	var status string
	err = db.QueryRow("SELECT status FROM docs WHERE id = 'doc-2'").Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "approved", status)

	// Verify the other document was not updated
	err = db.QueryRow("SELECT status FROM docs WHERE id = 'doc-1'").Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "pending", status)
}

func TestSelectWithAggregations_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	builder := sqlite.NewSQLiteFactory()

	salesSchema := &schema.SchemaDefinition{
		Name: "sales",
		Fields: map[string]*schema.FieldDefinition{
			"id":     {Name: "id", Type: schema.FieldTypeInteger},
			"region": {Name: "region", Type: schema.FieldTypeString},
			"amount": {Name: "amount", Type: schema.FieldTypeNumber},
		},
	}

	qb := query.NewQueryBuilder().
		From("sales").Schema(salesSchema).
		Select().Include("region").End().
		Count("*", "sale_count").
		Sum("amount", "total_revenue").
		Avg("amount", "avg_sale").
		Min("amount", "min_sale").
		Max("amount", "max_sale").
		GroupBy("region").
		WithFilter(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "total_revenue",
				Operator: query.ComparisonOperatorGt,
				Value:    query.FilterValue{NumberVal: utils.PrimitivePtr(float64(300))},
			},
		}).
		End().
		OrderByDesc("total_revenue")

	q := qb.Build()
	nq, err := builder.Build(&q, native.StmtSelect, nil)
	require.NoError(t, err)

	rows, err := db.Query(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)
	defer rows.Close()

	var results []data.Document
	for rows.Next() {
		var region string
		var saleCount int
		var totalRevenue, avgSale, minSale, maxSale float64
		err := rows.Scan(&saleCount, &totalRevenue, &avgSale, &minSale, &maxSale, &region)
		require.NoError(t, err)
		results = append(results, data.Document{
			"region":        region,
			"sale_count":    saleCount,
			"total_revenue": totalRevenue,
			"avg_sale":      avgSale,
			"min_sale":      minSale,
			"max_sale":      maxSale,
		})
	}

	require.NoError(t, rows.Err())
	assert.Len(t, results, 1)
	assert.Equal(t, "South", results[0]["region"])
	assert.Equal(t, 2, results[0]["sale_count"])
	assert.Equal(t, 450.0, results[0]["total_revenue"])
	assert.Equal(t, 225.0, results[0]["avg_sale"])
	assert.Equal(t, 200.0, results[0]["min_sale"])
	assert.Equal(t, 250.0, results[0]["max_sale"])
}

func TestComputedFieldTranslation(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	docSchema := &schema.SchemaDefinition{
		Name: "docs",
		Fields: map[string]*schema.FieldDefinition{
			"id":       {Name: "id", Type: schema.FieldTypeString},
			"metadata": {Name: "metadata", Type: schema.FieldTypeObject},
		},
	}

	t.Run("demonstrate computed field translation for arithmetic", func(t *testing.T) {
		dsl := query.NewQueryBuilder().Select().
		AddComputed("next_version", "ADD",
		&query.FieldReference{
			Field: "metadata.version",
		}, 1).End().
		Build()

		dsl.Target = &query.QueryTarget{
			Name: docSchema.Name,
			Schema: docSchema,
		}

		nq, err := builder.Build(&dsl, native.StmtSelect, nil)
		require.NoError(t, err)

		// This assertion demonstrates the desired SQL output.
		// It is expected to fail with the current implementation, and the failure
		// will show the actual generated SQL, guiding the necessary changes.
		expectedSQL := `SELECT (json_extract("metadata", '$.version') + $1) AS next_version FROM docs`
		assert.Equal(t, expectedSQL, nq.Raw().SQL)
		assert.Equal(t, []any{float64(1)}, nq.Raw().Params)
	})
}

