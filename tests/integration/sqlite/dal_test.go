package sqlite_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	sqlite "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v6/tests/testutils"
)

func setupDALTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Create users table
	_, err = db.Exec(`
		CREATE TABLE users_1_0_0 (
		 _id_ TEXT PRIMARY KEY,
			first_name TEXT,
			last_name TEXT,
			email TEXT,
			age INTEGER,
			status TEXT,
			region TEXT,
			_metadata_ TEXT
		)
	`)
	require.NoError(t, err)

	// Create orders table
	_, err = db.Exec(`
		CREATE TABLE orders_1_0_0 (
		 _id_ TEXT PRIMARY KEY,
			customer_id TEXT,
			order_date TEXT,
			total_amount REAL,
			_metadata_ TEXT
		)
	`)
	require.NoError(t, err)

	// Create sales table
	_, err = db.Exec(`
		CREATE TABLE sales (
		 _id_ INTEGER PRIMARY KEY AUTOINCREMENT,
			region TEXT,
			amount REAL,
			_metadata_ TEXT
		)
	`)
	require.NoError(t, err)

	// Insert sample data
	_, err = db.Exec(`
		INSERT INTO users_1_0_0 (_id_, first_name, last_name, email, age, status, region) VALUES
		('user-1', 'John', 'Doe', 'john.doe@example.com', 30, 'active', 'West'),
		('user-2', 'Jane', 'Smith', 'jane.smith@example.com', 28, 'inactive', 'East')
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO orders_1_0_0 (_id_, customer_id, order_date, total_amount) VALUES
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

	testutils.ConfigureDocumentFactory()

	return db
}

func TestInsert_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	builder := sqlite.NewSQLiteFactory()

	usersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users_1_0_0",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"first_name": {Name: "first_name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"last_name":  {Name: "last_name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"email":      {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":        {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"status":     {Name: "status", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"region":     {Name: "region", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	qb := query.NewQueryBuilder().From("users_1_0_0").Alias("users").Schema(usersSchema)
	q := qb.Build()

	data := *data.MustNewDocument(map[string]any{
		"first_name": "Peter",
		"last_name":  "Jones",
		"age":        45,
	})

	nq, err := builder.Build(&q, native.StmtInsert, data.ToMap())
	require.NoError(t, err)

	_, err = db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	// Verify insertion
	var count int
	err = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM users_1_0_0 WHERE _id_ = '%s'", data.ID())).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestUpdate_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	builder := sqlite.NewSQLiteFactory()
	usersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users_1_0_0",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"first_name": {Name: "first_name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"last_name":  {Name: "last_name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"email":      {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":        {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"status":     {Name: "status", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"region":     {Name: "region", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"metadata":   {Name: "_metadata_", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeRecord}},
			},
		},
	}

	dt := data.MustNewDocument(map[string]any{"age": 31})

	qb := query.NewQueryBuilder().From("users_1_0_0").Alias("users").Schema(usersSchema).Where(data.DocumentIDField).Eq("user-1")
	q := qb.Build()

	nq, err := builder.Build(&q, native.StmtUpdate, map[string]any{"set": dt.StripMetadata().ToMap()})
	require.NoError(t, err)

	res, err := db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify update
	var age int
	err = db.QueryRow(fmt.Sprintf("SELECT age FROM users_1_0_0 WHERE _id_ = '%s'", dt.ID())).Scan(&age)
	require.NoError(t, err)
	assert.Equal(t, 31, age)
}

func TestDelete_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	builder := sqlite.NewSQLiteFactory()
	usersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users_1_0_0",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"first_name": {Name: "first_name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"last_name":  {Name: "last_name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"email":      {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":        {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"status":     {Name: "status", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"region":     {Name: "region", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}
	qb := query.NewQueryBuilder().From("users_1_0_0").Alias("users").Schema(usersSchema).Where(data.DocumentIDField).Eq("user-2")
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
	err = db.QueryRow("SELECT COUNT(*) FROM users_1_0_0 WHERE _id_ = 'user-2'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestComplexTypes_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		CREATE TABLE complex_docs_01 (
		 _id_ TEXT PRIMARY KEY,
			tags TEXT,
		    _metadata_ TEXT
		)
	`)
	require.NoError(t, err)

	builder := sqlite.NewSQLiteFactory()

	complexDocsSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "complex_docs_01",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"tags":     {Name: "tags", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeArray}},
				"metadata": {Name: "_metadata_", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeObject}},
			},
		},
	}

	// Insert
	qb := query.NewQueryBuilder().From("complex_docs_01").Alias("complex_docs").Schema(complexDocsSchema)
	q := qb.Build()

	tags := []string{"go", "sqlite", "testing"}
	metadata := map[string]any{"author": "Augustine", "version": 2}
	insertData := *data.MustNewDocument(map[string]any{"tags": tags, "_metadata_": metadata})

	nq, err := builder.Build(&q, native.StmtInsert, insertData.ToMap())
	require.NoError(t, err)

	_, err = db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	// Verify Insert
	var id, rawTags string
	err = db.QueryRow(fmt.Sprintf("SELECT _id_, tags FROM complex_docs_01 WHERE _id_ = '%s'", insertData.ID())).Scan(&id, &rawTags)
	require.NoError(t, err)
	assert.Equal(t, insertData.ID(), id)
	assert.JSONEq(t, `["go", "sqlite", "testing"]`, rawTags)

	// Update
	updatedTags := []string{"go", "testing", "updated"}
	updateData := map[string]any{"tags": updatedTags}
	q = query.NewQueryBuilder().From("complex_docs_01").Alias("complex_docs").Schema(complexDocsSchema).Where(data.DocumentIDField).Eq(insertData.ID()).Build()

	nq, err = builder.Build(&q, native.StmtUpdate, map[string]any{"set": updateData})
	require.NoError(t, err)

	_, err = db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	// Verify Update
	err = db.QueryRow(fmt.Sprintf("SELECT tags FROM complex_docs_01 WHERE _id_ = '%s'", insertData.ID())).Scan(&rawTags)
	require.NoError(t, err)
	assert.JSONEq(t, `["go", "testing", "updated"]`, rawTags)

	// Select with nested field
	q = query.NewQueryBuilder().From("complex_docs_01").Alias("complex_docs").Schema(complexDocsSchema).Where("complex_docs._metadata_.version").Eq(2).Build()
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

	ordersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "orders_1_0_0",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"customer_id":  {Name: "customer_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"order_date":   {Name: "order_date", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"total_amount": {Name: "total_amount", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
		},
	}

	usersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users_1_0_0",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"first_name": {Name: "first_name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"last_name":  {Name: "last_name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"email":      {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":        {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"status":     {Name: "status", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"region":     {Name: "region", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	qb := query.NewQueryBuilder().From("orders_1_0_0").Alias("orders").Schema(ordersSchema).
		Select().
		Include("orders._id_", "total_amount").
		End().
		InnerJoin("users_1_0_0").Alias("users").Schema(usersSchema).
		On(query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    "orders.customer_id",
				Operator: query.ComparisonOperatorEq,
				Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "users._id_"}},
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
		results = append(results, *data.MustNewDocument(map[string]any{data.DocumentIDField: id, "total_amount": totalAmount}))
	}

	require.NoError(t, rows.Err())
	assert.Len(t, results, 1)
	assert.Equal(t, 150.50, results[0].MustGet("total_amount"))
}

func TestSelectWithNestedFieldInJoin_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	// Add a profile column to the users table and insert data
	_, err := db.Exec(`ALTER TABLE users_1_0_0 ADD COLUMN profile TEXT;`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE users_1_0_0 SET profile = ? WHERE _id_ = ?`, `{"preferences": {"theme": "dark"}, "level": 5}`, "user-1")
	require.NoError(t, err)

	builder := sqlite.NewSQLiteFactory()

	// Define the schema for the users table, marking 'profile' as a complex object
	userSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users_1_0_0",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"profile": {Name: "profile", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeRecord}},
			},
		},
	}

	// Define the schema for the users table, marking 'profile' as a complex object
	ordersSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "orders_1_0_0",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"customer_id":  {Name: "customer_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"order_date":   {Name: "order_date", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"total_amount": {Name: "total_amount", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
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
				Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "u._id_"}},
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
		 _id_ TEXT PRIMARY KEY,
			_metadata_ TEXT,
			status TEXT
		)
	`)
	require.NoError(t, err)

	// Insert sample data
	_, err = db.Exec(`
		INSERT INTO docs (_id_, _metadata_, status) VALUES
		('doc-1', '{"version": 1, "author": "John"}', 'pending'),
		('doc-2', '{"version": 2, "author": "Jane"}', 'pending')
	`)
	require.NoError(t, err)

	builder := sqlite.NewSQLiteFactory()

	// Define the schema for the docs table
	docSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "docs",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"metadata": {Name: "_metadata_", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeObject}},
				"status":   {Name: "status", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	// Build the update query
	qb := query.NewQueryBuilder().
		From("docs").
		Where("docs._metadata_.version").Eq(2)

	q := qb.Build()
	q.Target.Schema = docSchema
	updateData := map[string]any{"status": "approved"}

	nq, err := builder.Build(&q, native.StmtUpdate, map[string]any{"set": updateData})
	require.NoError(t, err)

	// Execute the query
	res, err := db.Exec(nq.Raw().SQL, nq.Raw().Params...)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify the update
	var status string
	err = db.QueryRow("SELECT status FROM docs WHERE _id_ = 'doc-2'").Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "approved", status)

	// Verify the other document was not updated
	err = db.QueryRow("SELECT status FROM docs WHERE _id_ = 'doc-1'").Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "pending", status)
}

func TestSelectWithAggregations_Integration(t *testing.T) {
	db := setupDALTestDB(t)
	defer db.Close()

	builder := sqlite.NewSQLiteFactory()

	salesSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "sales",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"region": {Name: "region", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"amount": {Name: "amount", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
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
		results = append(results, *data.MustNewDocument(map[string]any{
			"region":        region,
			"sale_count":    saleCount,
			"total_revenue": totalRevenue,
			"avg_sale":      avgSale,
			"min_sale":      minSale,
			"max_sale":      maxSale,
		}))
	}

	require.NoError(t, rows.Err())
	assert.Len(t, results, 1)
	assert.Equal(t, "South", results[0].MustGet("region"))
	assert.Equal(t, 2, results[0].MustGet("sale_count"))
	assert.Equal(t, 450.0, results[0].MustGet("total_revenue"))
	assert.Equal(t, 225.0, results[0].MustGet("avg_sale"))
	assert.Equal(t, 200.0, results[0].MustGet("min_sale"))
	assert.Equal(t, 250.0, results[0].MustGet("max_sale"))
}

func TestComputedFieldTranslation(t *testing.T) {
	builder := sqlite.NewSQLiteFactory()

	docSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "docs",
			Fields: map[definition.FieldId]definition.Field{
				definition.FieldId(data.DocumentIDField): {Name: definition.FieldName(data.DocumentIDField), FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"metadata": {Name: "metadata", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeObject}},
			},
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
			Name:   docSchema.Name,
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
