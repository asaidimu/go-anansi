package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
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

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"id":   {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":  {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"pk": {Name: "id_pk", Fields: []definition.FieldId{"id"}, Type: definition.IndexTypePrimary},
			},
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

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "products",
			Fields: map[definition.FieldId]definition.Field{
				"product_id": {Name: "product_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name":       {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"price":      {Name: "price", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"pk": {Name: "product_id_pk", Fields: []definition.FieldId{"product_id"}, Type: definition.IndexTypePrimary},
			},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	docsToInsert := []map[string]any{
		{"product_id": "p1", "name": "Laptop", "price": 1200.50},
		{"product_id": "p2", "name": "Mouse", "price": 25.00},
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

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "orders",
			Fields: map[definition.FieldId]definition.Field{
				"order_id":    {Name: "order_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"customer_id": {Name: "customer_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"amount":      {Name: "amount", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"status":      {Name: "status", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"pk": {Name: "order_id_pk", Fields: []definition.FieldId{"order_id"}, Type: definition.IndexTypePrimary},
			},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	docsToInsert := []map[string]any{
		{"order_id": "o1", "customer_id": "c1", "amount": 100.0, "status": "pending"},
		{"order_id": "o2", "customer_id": "c2", "amount": 250.50, "status": "completed"},
		{"order_id": "o3", "customer_id": "c1", "amount": 50.0, "status": "completed"},
	}
	_, err = interactor.InsertDocuments(ctx, testSchema, docsToInsert)
	require.NoError(t, err)

	// Test 1: Select all documents
	allDocs, _, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
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
	docsC1, _, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target:  &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
		Filters: filterC1,
	})
	require.NoError(t, err)
	assert.Len(t, docsC1, 2)
	for _, doc := range docsC1 {
		assert.Equal(t, "c1", doc["customer_id"])
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
	docsC1Completed, _, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target:  &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
		Filters: filterC1Completed,
	})
	require.NoError(t, err)
	assert.Len(t, docsC1Completed, 1)
	assert.Equal(t, "o3", docsC1Completed[0]["order_id"])
}

func TestNativeInteractor_UpdateDocuments(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "tasks",
			Fields: map[definition.FieldId]definition.Field{
				"task_id":     {Name: "task_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"description": {Name: "description", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"completed":   {Name: "completed", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeBoolean}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"pk": {Name: "task_id_pk", Fields: []definition.FieldId{"task_id"}, Type: definition.IndexTypePrimary},
			},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	docsToInsert := []map[string]any{
		{"task_id": "t1", "description": "Buy groceries", "completed": false},
		{"task_id": "t2", "description": "Walk the dog", "completed": false},
		{"task_id": "t3", "description": "Pay bills", "completed": true},
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
	_, rowsAffected, err := interactor.UpdateDocuments(ctx, testSchema, updates, nil, filterT1, false)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify update
	updatedDoc, _, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target:  &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
		Filters: filterT1,
	})
	require.NoError(t, err)
	assert.Len(t, updatedDoc, 1)
	assert.Equal(t, true, updatedDoc[0]["completed"])

	// Update all incomplete tasks
	updatesAll := map[string]any{"completed": true, "description": "DONE"}
	filterIncomplete := &query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "completed",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{BoolVal: utils.BoolPtr(false)},
		},
	}
	_, rowsAffectedAll, err := interactor.UpdateDocuments(ctx, testSchema, updatesAll, nil, filterIncomplete, false)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffectedAll) // Only t2 was incomplete

	// Verify all are now completed
	allDocs, _, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})
	require.NoError(t, err)
	assert.Len(t, allDocs, 3)
	for _, doc := range allDocs {
		assert.Equal(t, true, doc["completed"])
	}
}

func TestNativeInteractor_DeleteDocuments(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "items",
			Fields: map[definition.FieldId]definition.Field{
				"item_id": {Name: "item_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name":    {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"pk": {Name: "item_id_pk", Fields: []definition.FieldId{"item_id"}, Type: definition.IndexTypePrimary},
			},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	docsToInsert := []map[string]any{
		{"item_id": "i1", "name": "Apple"},
		{"item_id": "i2", "name": "Banana"},
		{"item_id": "i3", "name": "Orange"},
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
	remainingDocs, _, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})
	require.NoError(t, err)
	assert.Len(t, remainingDocs, 2)
	for _, doc := range remainingDocs {
		assert.NotEqual(t, "i2", doc["item_id"])
	}

	// Delete all remaining (unsafe delete)
	rowsAffectedAll, err := interactor.DeleteDocuments(ctx, testSchema, nil, true)
	require.NoError(t, err)
	assert.Equal(t, int64(2), rowsAffectedAll)

	// Verify no documents left
	finalDocs, _, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
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

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "temp_collection",
			Fields: map[definition.FieldId]definition.Field{
				"id": {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"pk": {Name: "id_pk", Fields: []definition.FieldId{"id"}, Type: definition.IndexTypePrimary},
			},
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

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "indexed_data",
			Fields: map[definition.FieldId]definition.Field{
				"id":    {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"value": {Name: "value", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"pk": {Name: "id_pk", Fields: []definition.FieldId{"id"}, Type: definition.IndexTypePrimary},
				"idx": {Name: "idx_value", Fields: []definition.FieldId{"value"}, Type: definition.IndexTypeUnique},
			},
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
	docsToInsert := []map[string]any{
		{"id": "d1", "value": "unique_val"},
	}
	_, err = interactor.InsertDocuments(ctx, testSchema, docsToInsert)
	require.NoError(t, err)

	duplicateDoc := []map[string]any{
		{"id": "d2", "value": "unique_val"},
	}
	_, err = interactor.InsertDocuments(ctx, testSchema, duplicateDoc)
	assert.Error(t, err) // Expect an error due to unique constraint
}

func TestNativeInteractor_DropIndex(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "data_with_index",
			Fields: map[definition.FieldId]definition.Field{
				"id":         {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"data_value": {Name: "data_value", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"pk": {Name: "id_pk", Fields: []definition.FieldId{"id"}, Type: definition.IndexTypePrimary},
				"idx": {Name: "idx_data_value", Fields: []definition.FieldId{"data_value"}, Type: definition.IndexTypeNormal},
			},
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
	indexToDrop := definition.Index{Name: "idx_data_value", Fields: []definition.FieldId{"data_value"}}
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

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "accounts",
			Fields: map[definition.FieldId]definition.Field{
				"account_id": {Name: "account_id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"balance":    {Name: "balance", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"pk": {Name: "account_id_pk", Fields: []definition.FieldId{"account_id"}, Type: definition.IndexTypePrimary},
			},
		},
	}

	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	// Initial insert
	_, err = interactor.InsertDocuments(ctx, testSchema, []map[string]any{
		{"account_id": "acc1", "balance": 100.0},
		{"account_id": "acc2", "balance": 50.0},
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
	_, _, err = txInteractor.UpdateDocuments(ctx, testSchema, updates, nil, filterAcc1, false)
	require.NoError(t, err)

	// Insert within transaction
	_, err = txInteractor.InsertDocuments(ctx, testSchema, []map[string]any{
		{"account_id": "acc3", "balance": 200.0},
	})

	require.NoError(t, err)

	// Verify changes are not visible outside transaction yet
	_, _, err = interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})

	assert.Error(t, err)

	// Commit the transaction
	err = txInteractor.Commit(ctx)
	require.NoError(t, err)

	// Verify changes are visible after commit
	docsAfterCommit, _, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})
	require.NoError(t, err)
	assert.Len(t, docsAfterCommit, 3) // Should now be 3 documents
	acc1Doc := findDoc(docsAfterCommit, "account_id", "acc1")
	require.NotNil(t, acc1Doc)
	assert.InDelta(t, 90.0, acc1Doc["balance"], 0.001)
	acc3Doc := findDoc(docsAfterCommit, "account_id", "acc3")
	require.NotNil(t, acc3Doc)
	assert.InDelta(t, 200.0, acc3Doc["balance"], 0.001)

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
	_, _, err = txInteractor2.UpdateDocuments(ctx, testSchema, updates2, nil, filterAcc1_2, false)
	require.NoError(t, err)
	_, err = txInteractor2.InsertDocuments(ctx, testSchema, []map[string]any{
		{"account_id": "acc4", "balance": 300.0},
	})
	require.NoError(t, err)

	// Rollback the transaction
	err = txInteractor2.Rollback(ctx)
	require.NoError(t, err)

	// Verify changes are not visible after rollback
	docsAfterRollback, _, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target: &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
	})
	require.NoError(t, err)
	assert.Len(t, docsAfterRollback, 3) // Should still be 3 documents
	acc1DocAfterRollback := findDoc(docsAfterRollback, "account_id", "acc1")
	require.NotNil(t, acc1DocAfterRollback)
	assert.InDelta(t, 90.0, acc1DocAfterRollback["balance"], 0.001) // Should be original committed value
	acc4Doc := findDoc(docsAfterRollback, "account_id", "acc4")
	assert.Nil(t, acc4Doc) // Should not exist
/**/
}

func findDoc(docs []map[string]any, key string, value any) map[string]any {
	for _, doc := range docs {
		if val, ok := doc[key]; ok {
			if val == value {
				return doc
			}
		}
	}
	return nil
}

func TestNativeInteractor_CheckCollection(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "test_collection",
			Fields: map[definition.FieldId]definition.Field{
				"id": {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
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

func TestNativeInteractor_RawQuery(t *testing.T) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	defer cleanup()

	interactor, err := createNativeInteractor(t, db)
	require.NoError(t, err)

	// 1. Create a collection using DSL
	testSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "raw_users",
			Fields: map[definition.FieldId]definition.Field{
				"id":    {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name":  {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"email": {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":   {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
			Indexes: map[definition.IndexId]definition.Index{
				"pk": {Name: "id_pk", Fields: []definition.FieldId{"id"}, Type: definition.IndexTypePrimary},
			},
		},
	}
	ctx := context.Background()
	err = interactor.SchemaManager().CreateCollection(ctx, *testSchema)
	require.NoError(t, err)

	// 2. Insert some data using DSL
	docsToInsert := []map[string]any{
		{"id": 1, "name": "Alice", "email": "alice@example.com", "age": 30},
		{"id": 2, "name": "Bob", "email": "bob@example.com", "age": 25},
		{"id": 3, "name": "Charlie", "email": "charlie@example.com", "age": 35},
	}
	_, err = interactor.InsertDocuments(ctx, testSchema, docsToInsert)
	require.NoError(t, err)

	// 3. Execute a raw SELECT query
	rawSelectQuery := &query.RawQuery{
		Template:   "SELECT id, name, age FROM raw_users WHERE age > ? ORDER BY age ASC",
		Parameters: []any{28},
	}
	selectResult, err := interactor.Query(ctx, &query.Query{Raw: rawSelectQuery})
	require.NoError(t, err)
	assert.True(t, selectResult.Success)
	assert.Equal(t, 2, selectResult.Count)
	assert.Len(t, selectResult.Data, 2)

	// Verify selected data
	selectedDocs := selectResult.Data.([]map[string]any)
	assert.Equal(t, "Alice", selectedDocs[0]["name"])
	assert.Equal(t, int64(30), selectedDocs[0]["age"])

	assert.Equal(t, "Charlie", selectedDocs[1]["name"])
	assert.Equal(t, int64(35), selectedDocs[1]["age"])

	// 4. Execute a raw UPDATE query
	rawUpdateQuery := &query.RawQuery{
		Template:   "UPDATE raw_users SET age = ? WHERE id = ?",
		Parameters: []any{31, selectedDocs[0]["id"]},
	}
	updateResult, err := interactor.Query(ctx, &query.Query{Raw: rawUpdateQuery})
	require.NoError(t, err)
	assert.True(t, updateResult.Success)
	assert.Equal(t, int64(1), updateResult.AffectedRows)

	// Verify the update using DSL select
	updatedUser, _, err := interactor.SelectDocuments(ctx, testSchema, &query.Query{
		Target:  &query.QueryTarget{Name: testSchema.Name, Schema: testSchema},
		Filters: &query.QueryFilter{Condition: &query.FilterCondition{Field: "id", Operator: query.ComparisonOperatorEq, Value: query.FilterValue{StringVal: utils.StringPtr(
			selectedDocs[0]["id"].(string),
		)}}},
	})
	require.NoError(t, err)
	assert.Len(t, updatedUser, 1)
	assert.Equal(t, int64(31), updatedUser[0]["age"])

	// 5. Execute a raw DDL query (CREATE INDEX)
	rawCreateIndexQuery := &query.RawQuery{
		Template: "CREATE INDEX idx_raw_users_email ON raw_users (email)",
	}
	createIndexResult, err := interactor.Query(ctx, &query.Query{Raw: rawCreateIndexQuery})
	require.NoError(t, err)
	assert.True(t, createIndexResult.Success)
	assert.Equal(t, int64(1), createIndexResult.AffectedRows) // SQLite reports 1 affected row for CREATE INDEX

	// Verify index exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_raw_users_email';")
	var indexName string
	err = row.Scan(&indexName)
	require.NoError(t, err)
	assert.Equal(t, "idx_raw_users_email", indexName)
}
