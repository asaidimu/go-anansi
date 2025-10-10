package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/asaidimu/go-anansi/v6/tests/testutils"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// setupTestDB creates a unique, in-memory SQLite database for each test.
// The database is automatically cleaned up when the returned function is called.
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	// The DSN `file:%s?mode=memory&cache=shared` creates a unique, named in-memory
	// database. The `cache=shared` part allows multiple connections within the
	// same test to access the same in-memory database. The database is destroyed
	// when the last connection to it is closed.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())

	db, err := sql.Open("sqlite3", dsn)
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

func createNativeInteractor(t *testing.T, db *sql.DB) (query.DatabaseInteractor, error) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	require.NoError(t, err)
	queryFactory := sqliteQuery.NewSQLiteFactory()
	return native.NewNativeInteractor(executor, queryFactory, logger)
}

func TestNativeInteractor_CreateCollection(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString},
			"name": {Name: "name", Type: schema.FieldTypeString},
			"age":  {Name: "age", Type: schema.FieldTypeInteger},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "id_pk", Fields: []string{"id"}, Type: schema.IndexTypePrimary},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	// Verify table exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users';")
	var tableName string
	err = row.Scan(&tableName)
	require.NoError(t, err)
	assert.Equal(t, "users", tableName)

	// Verify columns exist
	rows, err := db.Query("PRAGMA table_info(users);")
	require.NoError(t, err)
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk)
		require.NoError(t, err)
		columns[name] = ctype
	}

	assert.Contains(t, columns, "id")
	assert.Contains(t, columns, "name")
	assert.Contains(t, columns, "age")
	assert.Equal(t, "TEXT", columns["id"])
	assert.Equal(t, "TEXT", columns["name"])
	assert.Equal(t, "INTEGER", columns["age"])
}

func TestNativeInteractor_InsertDocuments(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &schema.SchemaDefinition{
		Name: "products",
		Fields: map[string]*schema.FieldDefinition{
			"product_id": {Name: "product_id", Type: schema.FieldTypeString},
			"name":       {Name: "name", Type: schema.FieldTypeString},
			"price":      {Name: "price", Type: schema.FieldTypeNumber},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "product_id_pk", Fields: []string{"product_id"}, Type: schema.IndexTypePrimary},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	docsToInsert := []data.Document{
		data.MustNewDocument(map[string]any{"product_id": "p1", "name": "Laptop", "price": 1200.50}),
		data.MustNewDocument(map[string]any{"product_id": "p2", "name": "Mouse", "price": 25.00}),
	}

	insertedDocs, err := interactor.InsertDocuments(ctx, testSchema, docsToInsert)
	require.NoError(t, err)
	assert.Len(t, insertedDocs, 2)

	// Verify documents are in the database
	rows, err := db.Query("SELECT product_id, name, price FROM products ORDER BY product_id;")
	require.NoError(t, err)
	defer rows.Close()

	var products []map[string]any
	for rows.Next() {
		var productID, name string
		var price float64
		err := rows.Scan(&productID, &name, &price)
		require.NoError(t, err)
		products = append(products, map[string]any{"product_id": productID, "name": name, "price": price})
	}

	assert.Len(t, products, 2)
	assert.Equal(t, "p1", products[0]["product_id"])
	assert.Equal(t, "Laptop", products[0]["name"])
	assert.InDelta(t, 1200.50, products[0]["price"], 0.001)

	assert.Equal(t, "p2", products[1]["product_id"])
	assert.Equal(t, "Mouse", products[1]["name"])
	assert.InDelta(t, 25.00, products[1]["price"], 0.001)
}

func TestNativeInteractor_SelectDocuments(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &schema.SchemaDefinition{
		Name: "orders",
		Fields: map[string]*schema.FieldDefinition{
			"order_id":    {Name: "order_id", Type: schema.FieldTypeString},
			"customer_id": {Name: "customer_id", Type: schema.FieldTypeString},
			"amount":      {Name: "amount", Type: schema.FieldTypeNumber},
			"status":      {Name: "status", Type: schema.FieldTypeString},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "order_id_pk", Fields: []string{"order_id"}, Type: schema.IndexTypePrimary},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	docsToInsert := []data.Document{
		data.MustNewDocument(map[string]any{"order_id": "o1", "customer_id": "c1", "amount": 100.0, "status": "pending"}),
		data.MustNewDocument(map[string]any{"order_id": "o2", "customer_id": "c2", "amount": 250.50, "status": "completed"}),
		data.MustNewDocument(map[string]any{"order_id": "o3", "customer_id": "c1", "amount": 50.0, "status": "completed"}),
	}
	_, err = interactor.InsertDocuments(ctx, testSchema, docsToInsert)
	require.NoError(t, err)

	// Test 1: Select all documents
	allDocs, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})
	require.NoError(t, err)
	assert.Len(t, allDocs, 3)

	// Test 2: Select with filter (customer_id = 'c1')
	filterC1 := &query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "customer_id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{StringVal: utils.StringPtr("c1")},
		},
	}
	docsC1, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target:  &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
		Filters: filterC1,
	})
	require.NoError(t, err)
	assert.Len(t, docsC1, 2)
	for _, doc := range docsC1 {
		val, err := doc.Get("customer_id")
		require.NoError(t, err)
		assert.Equal(t, "c1", val)
	}

	// Test 3: Select with multiple filters (customer_id = 'c1' AND status = 'completed')
	filterC1Completed := &query.QueryFilter{
		Group: &query.FilterGroup{
			Operator: common.LogicalAnd,
			Conditions: []query.QueryFilter{
				{
					Condition: &query.FilterCondition{
						Field:    "customer_id",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{StringVal: utils.StringPtr("c1")},
					},
				},
				{
					Condition: &query.FilterCondition{
						Field:    "status",
						Operator: query.ComparisonOperatorEq,
						Value:    query.FilterValue{StringVal: utils.StringPtr("completed")},
					},
				},
			},
		},
	}
	docsC1Completed, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target:  &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
		Filters: filterC1Completed,
	})
	require.NoError(t, err)
	assert.Len(t, docsC1Completed, 1)
	val, err := docsC1Completed[0].Get("order_id")
	require.NoError(t, err)
	assert.Equal(t, "o3", val)
}

func TestNativeInteractor_UpdateDocuments(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &schema.SchemaDefinition{
		Name: "tasks",
		Fields: map[string]*schema.FieldDefinition{
			"task_id":   {Name: "task_id", Type: schema.FieldTypeString},
			"description": {Name: "description", Type: schema.FieldTypeString},
			"completed": {Name: "completed", Type: schema.FieldTypeBoolean},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "task_id_pk", Fields: []string{"task_id"}, Type: schema.IndexTypePrimary},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	docsToInsert := []data.Document{
		data.MustNewDocument(map[string]any{"task_id": "t1", "description": "Buy groceries", "completed": false}),
		data.MustNewDocument(map[string]any{"task_id": "t2", "description": "Walk the dog", "completed": false}),
		data.MustNewDocument(map[string]any{"task_id": "t3", "description": "Pay bills", "completed": true}),
	}
	_, err = interactor.InsertDocuments(ctx, testSchema, docsToInsert)
	require.NoError(t, err)

	// Update t1 to completed
	updates := map[string]any{"completed": true}
	filterT1 := &query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "task_id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{StringVal: utils.StringPtr("t1")},
		},
	}
	rowsAffected, err := interactor.UpdateDocuments(ctx, testSchema, updates, nil, filterT1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify update
	updatedDoc, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target:  &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
		Filters: filterT1,
	})
	require.NoError(t, err)
	assert.Len(t, updatedDoc, 1)
	val, err := updatedDoc[0].Get("completed")
	require.NoError(t, err)
	assert.Equal(t, true, val)

	// Update all incomplete tasks
	updatesAll := map[string]any{"completed": true, "description": "DONE"}
	filterIncomplete := &query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "completed",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{BoolVal: utils.BoolPtr(false)},
		},
	}
	rowsAffectedAll, err := interactor.UpdateDocuments(ctx, testSchema, updatesAll, nil, filterIncomplete)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffectedAll) // Only t2 was incomplete

	// Verify all are now completed
	allDocs, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})
	require.NoError(t, err)
	assert.Len(t, allDocs, 3)
	for _, doc := range allDocs {
		val, err := doc.Get("completed")
		require.NoError(t, err)
		assert.Equal(t, true, val)
	}
}

func TestNativeInteractor_DeleteDocuments(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &schema.SchemaDefinition{
		Name: "items",
		Fields: map[string]*schema.FieldDefinition{
			"item_id": {Name: "item_id", Type: schema.FieldTypeString},
			"name":    {Name: "name", Type: schema.FieldTypeString},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "item_id_pk", Fields: []string{"item_id"}, Type: schema.IndexTypePrimary},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	docsToInsert := []data.Document{
		data.MustNewDocument(map[string]any{"item_id": "i1", "name": "Apple"}),
		data.MustNewDocument(map[string]any{"item_id": "i2", "name": "Banana"}),
		data.MustNewDocument(map[string]any{"item_id": "i3", "name": "Orange"}),
	}
	_, err = interactor.InsertDocuments(ctx, testSchema, docsToInsert)
	require.NoError(t, err)

	// Delete i2
	filterI2 := &query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "item_id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{StringVal: utils.StringPtr("i2")},
		},
	}
	rowsAffected, err := interactor.DeleteDocuments(ctx, testSchema, filterI2, false)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify i2 is gone
	remainingDocs, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})
	require.NoError(t, err)
	assert.Len(t, remainingDocs, 2)
	for _, doc := range remainingDocs {
		val, err := doc.Get("item_id")
		require.NoError(t, err)
		assert.NotEqual(t, "i2", val)
	}

	// Delete all remaining (unsafe delete)
	rowsAffectedAll, err := interactor.DeleteDocuments(ctx, testSchema, nil, true)
	require.NoError(t, err)
	assert.Equal(t, int64(2), rowsAffectedAll)

	// Verify no documents left
	finalDocs, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})
	require.NoError(t, err)
	assert.Len(t, finalDocs, 0)
}

func TestNativeInteractor_DropCollection(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &schema.SchemaDefinition{
		Name: "temp_collection",
		Fields: map[string]*schema.FieldDefinition{
			"id": {Name: "id", Type: schema.FieldTypeString},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "id_pk", Fields: []string{"id"}, Type: schema.IndexTypePrimary},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	// Verify table exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='temp_collection';")
	var tableName string
	err = row.Scan(&tableName)
	require.NoError(t, err)
	assert.Equal(t, "temp_collection", tableName)

	err = interactor.SchemaManager().DropCollection(ctx, testSchema.Name)
	require.NoError(t, err)

	// Verify table does not exist
	row = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='temp_collection';")
	err = row.Scan(&tableName)
	assert.Equal(t, sql.ErrNoRows, err)
}

func TestNativeInteractor_CreateIndex(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &schema.SchemaDefinition{
		Name: "indexed_data",
		Fields: map[string]*schema.FieldDefinition{
			"id":    {Name: "id", Type: schema.FieldTypeString},
			"value": {Name: "value", Type: schema.FieldTypeString},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "id_pk", Fields: []string{"id"}, Type: schema.IndexTypePrimary},
			{Name: "idx_value", Fields: []string{"value"}, Type: schema.IndexTypeUnique},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	// Verify index exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_value';")
	var indexName string
	err = row.Scan(&indexName)
	require.NoError(t, err)
	assert.Equal(t, "idx_value", indexName)

	// Test inserting duplicate value for unique index
	docsToInsert := []data.Document{
		data.MustNewDocument(map[string]any{"id": "d1", "value": "unique_val"}),
	}
	_, err = interactor.InsertDocuments(ctx, testSchema, docsToInsert)
	require.NoError(t, err)

	duplicateDoc := []data.Document{
		data.MustNewDocument(map[string]any{"id": "d2", "value": "unique_val"}),
	}
	_, err = interactor.InsertDocuments(ctx, testSchema, duplicateDoc)
	assert.Error(t, err) // Expect an error due to unique constraint
	assert.Contains(t, err.Error(), "UNIQUE constraint failed")
}

func TestNativeInteractor_DropIndex(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &schema.SchemaDefinition{
		Name: "data_with_index",
		Fields: map[string]*schema.FieldDefinition{
			"id":         {Name: "id", Type: schema.FieldTypeString},
			"data_value": {Name: "data_value", Type: schema.FieldTypeString},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "id_pk", Fields: []string{"id"}, Type: schema.IndexTypePrimary},
			{Name: "idx_data_value", Fields: []string{"data_value"}, Type: schema.IndexTypeNormal},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	// Verify index exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_data_value';")
	var indexName string
	err = row.Scan(&indexName)
	require.NoError(t, err)
	assert.Equal(t, "idx_data_value", indexName)

	// Drop the index
	indexToDrop := schema.IndexDefinition{Name: "idx_data_value", Fields: []string{"data_value"}}
	err = interactor.SchemaManager().DropIndex(ctx, testSchema.Name, indexToDrop)
	require.NoError(t, err)

	// Verify index does not exist
	row = db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_data_value';")
	err = row.Scan(&indexName)
	assert.Equal(t, sql.ErrNoRows, err)
}

func TestNativeInteractor_Transactions(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &schema.SchemaDefinition{
		Name: "accounts",
		Fields: map[string]*schema.FieldDefinition{
			"account_id": {Name: "account_id", Type: schema.FieldTypeString},
			"balance":    {Name: "balance", Type: schema.FieldTypeNumber},
		},
		Indexes: []schema.IndexDefinition{
			{Name: "account_id_pk", Fields: []string{"account_id"}, Type: schema.IndexTypePrimary},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	// Initial insert
	_, err = interactor.InsertDocuments(ctx, testSchema, []data.Document{
		data.MustNewDocument(map[string]any{"account_id": "acc1", "balance": 100.0}),
		data.MustNewDocument(map[string]any{"account_id": "acc2", "balance": 50.0}),
	})
	require.NoError(t, err)

	// Start a transaction
	txInteractor, err := interactor.StartTransaction(ctx)
	require.NoError(t, err)
	require.NotNil(t, txInteractor)
	assert.True(t, txInteractor.HasTransaction(ctx))

	// Update within transaction
	updates := map[string]any{"balance": 90.0}
	filterAcc1 := &query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "account_id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{StringVal: utils.StringPtr("acc1")},
		},
	}
	_, err = txInteractor.UpdateDocuments(ctx, testSchema, updates, nil, filterAcc1)
	require.NoError(t, err)

	// Insert within transaction
	_, err = txInteractor.InsertDocuments(ctx, testSchema, []data.Document{
		data.MustNewDocument(map[string]any{"account_id": "acc3", "balance": 200.0}),
	})

	require.NoError(t, err)

	// Verify changes are not visible outside transaction yet
	_, err = interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})

	assert.Error(t, err)

	// Commit the transaction
	err = txInteractor.Commit(ctx)
	require.NoError(t, err)

	// Verify changes are visible after commit
	docsAfterCommit, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})
	require.NoError(t, err)
	assert.Len(t, docsAfterCommit, 3) // Should now be 3 documents
	acc1Doc := findDoc(docsAfterCommit, "account_id", "acc1")
	require.NotNil(t, acc1Doc)
	val, err := acc1Doc.Get("balance")
	require.NoError(t, err)
	assert.InDelta(t, 90.0, val, 0.001)
	acc3Doc := findDoc(docsAfterCommit, "account_id", "acc3")
	require.NotNil(t, acc3Doc)
	val, err = acc3Doc.Get("balance")
	require.NoError(t, err)
	assert.InDelta(t, 200.0, val, 0.001)

	// Test Rollback
	txInteractor2, err := interactor.StartTransaction(ctx)
	require.NoError(t, err)
	updates2 := map[string]any{"balance": 10.0}
	filterAcc1_2 := &query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "account_id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{StringVal: utils.StringPtr("acc1")},
		},
	}
	_, err = txInteractor2.UpdateDocuments(ctx, testSchema, updates2, nil, filterAcc1_2)
	require.NoError(t, err)
	_, err = txInteractor2.InsertDocuments(ctx, testSchema, []data.Document{
		data.MustNewDocument(map[string]any{"account_id": "acc4", "balance": 300.0}),
	})
	require.NoError(t, err)

	// Rollback the transaction
	err = txInteractor2.Rollback(ctx)
	require.NoError(t, err)

	// Verify changes are not visible after rollback
	docsAfterRollback, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})
	require.NoError(t, err)
	assert.Len(t, docsAfterRollback, 3) // Should still be 3 documents
	acc1DocAfterRollback := findDoc(docsAfterRollback, "account_id", "acc1")
	require.NotNil(t, acc1DocAfterRollback)
	val, err = acc1DocAfterRollback.Get("balance")
	require.NoError(t, err)
	assert.InDelta(t, 90.0, val, 0.001) // Should be original committed value
	acc4Doc := findDoc(docsAfterRollback, "account_id", "acc4")
	assert.Nil(t, acc4Doc) // Should not exist
/**/
}

func findDoc(docs []data.Document, key string, value any) data.Document {
	for _, doc := range docs {
		val, err := doc.Get(key)
		if err != nil {
			continue
		}
		if val == value {
			return doc
		}
	}
	return nil
}

func TestNativeInteractor_CheckCollection(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &schema.SchemaDefinition{
		Name: "test_collection",
		Fields: map[string]*schema.FieldDefinition{
			"id": {Name: "id", Type: schema.FieldTypeString},
		},
	}

	ctx := context.Background()

	// 1. Check for a non-existent collection
	exists, err := interactor.SchemaManager().CollectionExists(ctx, "non_existent_collection")
	require.NoError(t, err)
	require.False(t, exists)

	// 2. Create a collection
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	// 3. Check that the collection now exists
	exists, err = interactor.SchemaManager().CollectionExists(ctx, "test_collection")
	require.NoError(t, err)
	require.True(t, exists)
}
