package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/query/native"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/utils"
	sqlite_executor "github.com/asaidimu/go-anansi/v8/sqlite/executor"
	sqlite_query "github.com/asaidimu/go-anansi/v8/sqlite/query"
	"github.com/asaidimu/go-anansi/v8/sqlite/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

func setupQueryExecutorTest(t *testing.T) (*sql.DB, native.QueryExecutor[types.SQLitePayload], *definition.Schema) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	executor, err := sqlite_executor.NewSQLiteExecutor(db, zap.NewNop())
	require.NoError(t, err)

	userSchema := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "users",
			Fields: map[definition.FieldId]definition.Field{
				"id":    {Name: "id", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name":  {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":   {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"email": {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	// Create the table
	builder := sqlite_query.NewSQLiteFactory(nil)
	q := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
	}
	nq, err := builder.Build(&q, native.StmtCreateCollection, nil)
	require.NoError(t, err)

	_, err = db.Exec(nq.Raw().SQL)
	require.NoError(t, err)

	return db, executor, userSchema
}

func TestInsertAndSelectIntegration(t *testing.T) {
	db, executor, userSchema := setupQueryExecutorTest(t)
	defer db.Close()

	builder := sqlite_query.NewSQLiteFactory(nil)

	// Insert data
	records := []map[string]any{
		{"id": "1", "name": "Alice", "age": 30, "email": "alice@example.com"},
		{"id": "2", "name": "Bob", "age": 25, "email": "bob@example.com"},
	}

	insertQuery := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
	}

	nqInsert, err := builder.Build(&insertQuery, native.StmtInsert, records)
	require.NoError(t, err)

	resultSchema, err := query.SchemaFromQuery(&insertQuery, nil)
	require.NoError(t, err)

	insertedDocs, _, err := executor.Query(context.Background(), native.NativeQuery[types.SQLitePayload]{
		Query:  nqInsert,
		Schema: resultSchema,
	})

	require.NoError(t, err)
	assert.Len(t, insertedDocs, 2)

	// Select data
	selectQuery := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
	}

	resultSchema, err = query.SchemaFromQuery(&selectQuery, nil)
	require.NoError(t, err)

	nqSelect, err := builder.Build(&selectQuery, native.StmtSelect, nil)
	require.NoError(t, err)

	selectedDocs, _, err := executor.Query(context.Background(), native.NativeQuery[types.SQLitePayload]{
		Query:  nqSelect,
		Schema: resultSchema,
	})
	require.NoError(t, err)
	assert.Len(t, selectedDocs, 2)
}

func TestUpdateIntegration(t *testing.T) {
	db, executor, userSchema := setupQueryExecutorTest(t)
	defer db.Close()

	builder := sqlite_query.NewSQLiteFactory(nil)

	// Insert data
	records := []map[string]any{
		{"id": "1", "name": "Alice", "age": 30, "email": "alice@example.com"},
	}

	insertQuery := &query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
	}
	nqInsert, err := builder.Build(insertQuery, native.StmtInsert, records)
	require.NoError(t, err)

	resultSchema, err := query.SchemaFromQuery(insertQuery, nil)
	require.NoError(t, err)

	_, _, err = executor.Query(context.Background(), native.NativeQuery[types.SQLitePayload]{
		Query:  nqInsert,
		Schema: resultSchema,
	})
	require.NoError(t, err)

	// Update data
	updates := map[string]any{"age": 31, "email": "alice.updated@example.com"}
	filters := &query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{StringVal: utils.StringPtr("1")},
		},
	}
	updateQuery := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
		Filters: filters,
	}

	nqUpdate, err := builder.Build(&updateQuery, native.StmtUpdate, map[string]any{"set": updates})
	require.NoError(t, err)

	rowsAffected, err := executor.Exec(context.Background(), native.NativeQuery[types.SQLitePayload]{
		Query: nqUpdate,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify updated data
	selectQuery := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
		Filters: filters,
	}

	resultSchema, err = query.SchemaFromQuery(&selectQuery, nil)
	require.NoError(t, err)

	nqSelect, err := builder.Build(&selectQuery, native.StmtSelect, nil)
	require.NoError(t, err)

	selectedDocs, _, err := executor.Query(context.Background(), native.NativeQuery[types.SQLitePayload]{
		Query:  nqSelect,
		Schema: resultSchema,
	})

	require.NoError(t, err)
	assert.Len(t, selectedDocs, 1)
}

func TestDeleteIntegration(t *testing.T) {
	db, executor, userSchema := setupQueryExecutorTest(t)
	defer db.Close()

	builder := sqlite_query.NewSQLiteFactory(nil)

	// Insert data
	records := []map[string]any{
		{"id": "1", "name": "Alice", "age": 30, "email": "alice@example.com"},
		{"id": "2", "name": "Bob", "age": 25, "email": "bob@example.com"},
	}

	insertQuery := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
	}

	nqInsert, err := builder.Build(&insertQuery, native.StmtInsert, records)
	require.NoError(t, err)

	resultSchema, err := query.SchemaFromQuery(&insertQuery, nil)
	require.NoError(t, err)

	_, _, err = executor.Query(context.Background(), native.NativeQuery[types.SQLitePayload]{
		Query:  nqInsert,
		Schema: resultSchema,
	})

	require.NoError(t, err)

	// Delete data
	filters := &query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{StringVal: utils.StringPtr("1")},
		},
	}

	deleteQuery := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
		Filters: filters,
	}
	nqDelete, err := builder.Build(&deleteQuery, native.StmtDelete, nil)
	require.NoError(t, err)

	rowsAffected, err := executor.Exec(context.Background(), native.NativeQuery[types.SQLitePayload]{
		Query: nqDelete,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify data is deleted
	selectQuery := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
	}

	nqSelect, err := builder.Build(&selectQuery, native.StmtSelect, nil)
	require.NoError(t, err)

	resultSchema, err = query.SchemaFromQuery(&selectQuery, nil)
	require.NoError(t, err)

	selectedDocs, _, err := executor.Query(context.Background(), native.NativeQuery[types.SQLitePayload]{
		Query:  nqSelect,
		Schema: resultSchema,
	})
	require.NoError(t, err)
	assert.Len(t, selectedDocs, 1)
}

func TestOffsetPagination(t *testing.T) {
	db, executor, userSchema := setupQueryExecutorTest(t)
	defer db.Close()

	builder := sqlite_query.NewSQLiteFactory(nil)

	// Insert test data
	records := make([]map[string]any, 11)
	for i := range 11 {
		records[i] = map[string]any{
			"id": i,
			"name":  "Alice",
			"age":   30 + i,
			"email": "alice@example.com",
		}
	}

	insertQuery := query.Query{
		Target: &query.QueryTarget{Name: userSchema.Name, Schema: userSchema},
	}

	nqInsert, err := builder.Build(&insertQuery, native.StmtInsert, records)
	require.NoError(t, err)

	resultSchema, err := query.SchemaFromQuery(&insertQuery, nil)
	require.NoError(t, err)

	_, _, err = executor.Query(context.Background(), native.NativeQuery[types.SQLitePayload]{Query: nqInsert, Schema: resultSchema})
	require.NoError(t, err)

	// Helper to test offset pagination
	testOffset := func(offset int, limit int, direction query.SortDirection, expectedAges []int) {
		selectQuery := query.Query{
			Target: &query.QueryTarget{Name: userSchema.Name, Schema: userSchema},
			Pagination: &query.PaginationOptions{
				Type:   query.PaginationTypeOffset,
				Offset: utils.PrimitivePtr(offset),
				Limit:  limit,
				Order: []query.SortConfiguration{
					{Field: "id", Direction: direction},
				},
			},
		}

		nqSelect, err := builder.Build(&selectQuery, native.StmtSelect, nil)
		require.NoError(t, err)

		resultSchema, err := query.SchemaFromQuery(&selectQuery, nil)
		require.NoError(t, err)

		selectedDocs, _, err := executor.Query(context.Background(), native.NativeQuery[types.SQLitePayload]{Query: nqSelect, Schema: resultSchema})
		require.NoError(t, err)
		assert.Len(t, selectedDocs, len(expectedAges))

		for i, age := range expectedAges {
			gotAge := selectedDocs[i]["age"].(int64)
			assert.Equal(t, int64(age), gotAge)
		}
	}

	// Forward pagination
	testOffset(5, 3, query.SortDirectionAsc, []int{34, 35, 36})
	// Backward pagination
	testOffset(5, 3, query.SortDirectionDesc, []int{34, 33, 32})
}

func TestCursorPagination(t *testing.T) {
	db, executor, userSchema := setupQueryExecutorTest(t)
	defer db.Close()

	builder := sqlite_query.NewSQLiteFactory(nil)

	// Insert test data
	records := make([]map[string]any, 11)
	for i := range 11 {
		records[i] = map[string]any{
			"name":  "Alice",
			"age":   30 + i,
			"email": "alice@example.com",
		}
	}

	insertQuery := query.Query{
		Target: &query.QueryTarget{Name: userSchema.Name, Schema: userSchema},
	}

	nqInsert, err := builder.Build(&insertQuery, native.StmtInsert, records)
	require.NoError(t, err)

	resultSchema, err := query.SchemaFromQuery(&insertQuery, nil)
	require.NoError(t, err)

	_, _, err = executor.Query(context.Background(), native.NativeQuery[types.SQLitePayload]{Query: nqInsert, Schema: resultSchema})
	require.NoError(t, err)

	// Test cursor pagination
	cursorField := "age"

	selectQuery := query.Query{
		Target: &query.QueryTarget{Name: userSchema.Name, Schema: userSchema},
		Pagination: &query.PaginationOptions{
			Type:  query.PaginationTypeCursor,
			Limit: 3,
			Cursor: &query.PaginationCursor{
				Field: &cursorField,
				Cursor: &query.FilterValue{
					NumberVal: utils.PrimitivePtr(float64(34)),
				},
			},
			Order: []query.SortConfiguration{
				{Field: cursorField, Direction: query.SortDirectionAsc},
			},
		},
	}

	nqSelect, err := builder.Build(&selectQuery, native.StmtSelect, nil)
	require.NoError(t, err)

	resultSchema, err = query.SchemaFromQuery(&selectQuery, nil)
	require.NoError(t, err)

	selectedDocs, _, err := executor.Query(context.Background(), native.NativeQuery[types.SQLitePayload]{Query: nqSelect, Schema: resultSchema})
	require.NoError(t, err)
	assert.Len(t, selectedDocs, 3)

	expectedAges := []int{35, 36, 37}
	for i, age := range expectedAges {
		gotAge := selectedDocs[i]["age"].(int64)
		assert.Equal(t, int64(age), gotAge)
	}
}
