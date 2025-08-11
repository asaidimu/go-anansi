package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	sqlite_executor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqlite_query "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/asaidimu/go-anansi/v6/sqlite/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

func setupQueryExecutorTest(t *testing.T) (*sql.DB, native.QueryExecutor[types.SQLitePayload], *schema.SchemaDefinition) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	executor, err := sqlite_executor.NewSQLiteInteractor(db, zap.NewNop())
	require.NoError(t, err)

	userSchema := &schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"id":    {Name: "id", Type: schema.FieldTypeString},
			"name":  {Name: "name", Type: schema.FieldTypeString},
			"age":   {Name: "age", Type: schema.FieldTypeInteger},
			"email": {Name: "email", Type: schema.FieldTypeString},
		},
	}

	// Create the table
	builder := sqlite_query.NewSQLiteFactory()
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

	builder := sqlite_query.NewSQLiteFactory()

	// Insert data
	records := []common.Document{
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

	insertedDocs, err := executor.Query(context.Background(), nqInsert)
	require.NoError(t, err)
	assert.Len(t, insertedDocs, 2)

	// Select data
	selectQuery := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
	}
	nqSelect, err := builder.Build(&selectQuery, native.StmtSelect, nil)
	require.NoError(t, err)

	selectedDocs, err := executor.Query(context.Background(), nqSelect)
	require.NoError(t, err)
	assert.Len(t, selectedDocs, 2)

	// Assert content of selected documents
	assert.Contains(t, selectedDocs, common.Document{"id": "1", "name": "Alice", "age": int64(30), "email": "alice@example.com"})
	assert.Contains(t, selectedDocs, common.Document{"id": "2", "name": "Bob", "age": int64(25), "email": "bob@example.com"})
}

func TestUpdateIntegration(t *testing.T) {
	db, executor, userSchema := setupQueryExecutorTest(t)
	defer db.Close()

	builder := sqlite_query.NewSQLiteFactory()

	// Insert data
	records := []common.Document{
		{"id": "1", "name": "Alice", "age": 30, "email": "alice@example.com"},
	}
	nqInsert, err := builder.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
	}, native.StmtInsert, records)
	require.NoError(t, err)
	_, err = executor.Query(context.Background(), nqInsert)
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
	nqUpdate, err := builder.Build(&updateQuery, native.StmtUpdate, updates)
	require.NoError(t, err)

	rowsAffected, err := executor.Exec(context.Background(), nqUpdate)
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
	nqSelect, err := builder.Build(&selectQuery, native.StmtSelect, nil)
	require.NoError(t, err)

	selectedDocs, err := executor.Query(context.Background(), nqSelect)
	require.NoError(t, err)
	assert.Len(t, selectedDocs, 1)
	assert.Contains(t, selectedDocs, common.Document{"id": "1", "name": "Alice", "age": int64(31), "email": "alice.updated@example.com"})
}

func TestDeleteIntegration(t *testing.T) {
	db, executor, userSchema := setupQueryExecutorTest(t)
	defer db.Close()

	builder := sqlite_query.NewSQLiteFactory()

	// Insert data
	records := []common.Document{
		{"id": "1", "name": "Alice", "age": 30, "email": "alice@example.com"},
		{"id": "2", "name": "Bob", "age": 25, "email": "bob@example.com"},
	}
	nqInsert, err := builder.Build(&query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: userSchema,
		},
	}, native.StmtInsert, records)
	require.NoError(t, err)
	_, err = executor.Query(context.Background(), nqInsert)
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

	rowsAffected, err := executor.Exec(context.Background(), nqDelete)
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

	selectedDocs, err := executor.Query(context.Background(), nqSelect)
	require.NoError(t, err)
	assert.Len(t, selectedDocs, 1)
	assert.Contains(t, selectedDocs, common.Document{"id": "2", "name": "Bob", "age": int64(25), "email": "bob@example.com"})
}
