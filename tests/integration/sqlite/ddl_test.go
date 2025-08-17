package sqlite_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	sqlite "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates a unique, in-memory SQLite database for each test.
// The database is automatically cleaned up when the returned function is called.
func setupDDLTestDB(t *testing.T) (*sql.DB, func()) {
	// The DSN `file:%s?mode=memory&cache=shared` creates a unique, named in-memory
	// database. The `cache=shared` part allows multiple connections within the
	// same test to access the same in-memory database. The database is destroyed
	// when the last connection to it is closed.
	dsn := fmt.Sprintf("file:ddl%s?mode=memory&cache=shared", t.Name())

	db, err := sql.Open("sqlite3", dsn)
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

func TestCreateCollectionIntegration(t *testing.T) {
	db, _ := setupDDLTestDB(t)
	defer db.Close()

	userSchema := schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString},
			"name": {Name: "name", Type: schema.FieldTypeString},
			"age":  {Name: "age", Type: schema.FieldTypeInteger},
		},
	}

	builder := sqlite.NewSQLiteFactory()
	q := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: &userSchema,
		},
	}
	nq, err := builder.Build(&q, native.StmtCreateCollection, nil)
	require.NoError(t, err)

	sqlQuery := nq.Raw().SQL
	_, err = db.Exec(sqlQuery)
	require.NoError(t, err)

	// Verify table exists
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name='users';")
	require.NoError(t, err)
	defer rows.Close()

	assert.True(t, rows.Next(), "Table 'users' should exist")

	var tableName string
	err = rows.Scan(&tableName)
	require.NoError(t, err)
	assert.Equal(t, "users", tableName)
}

func TestDropCollectionIntegration(t *testing.T) {
	db, _ := setupDDLTestDB(t)
	defer db.Close()

	userSchema := schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString},
			"name": {Name: "name", Type: schema.FieldTypeString},
			"age":  {Name: "age", Type: schema.FieldTypeInteger},
		},
	}

	builder := sqlite.NewSQLiteFactory()
	q := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: &userSchema,
		},
	}

	// Create the collection first
	nqCreate, err := builder.Build(&q, native.StmtCreateCollection, nil)
	require.NoError(t, err)
	_, err = db.Exec(nqCreate.Raw().SQL)
	require.NoError(t, err)

	// Verify table exists before dropping
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name='users';")
	require.NoError(t, err)
	assert.True(t, rows.Next(), "Table 'users' should exist before dropping")
	rows.Close()

	// Drop the collection
	nqDrop, err := builder.Build(&q, native.StmtDropCollection, nil)
	require.NoError(t, err)
	_, err = db.Exec(nqDrop.Raw().SQL)
	require.NoError(t, err)

	// Verify table does not exist after dropping
	rows, err = db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name='users';")
	require.NoError(t, err)
	assert.False(t, rows.Next(), "Table 'users' should not exist after dropping")
	rows.Close()
}

func TestCreateIndexIntegration(t *testing.T) {
	db, _ := setupDDLTestDB(t)
	defer db.Close()

	userSchema := schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString},
			"name": {Name: "name", Type: schema.FieldTypeString},
			"age":  {Name: "age", Type: schema.FieldTypeInteger},
		},
	}
	indexDef := schema.IndexDefinition{
		Name:   "idx_users_name",
		Fields: []string{"name"},
		Type:   schema.IndexTypeNormal,
	}

	builder := sqlite.NewSQLiteFactory()
	q := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: &userSchema,
		},
	}

	// Create the collection first
	nqCreateCollection, err := builder.Build(&q, native.StmtCreateCollection, nil)
	require.NoError(t, err)
	_, err = db.Exec(nqCreateCollection.Raw().SQL)
	require.NoError(t, err)

	// Create the index
	nqCreateIndex, err := builder.Build(&q, native.StmtCreateIndex, indexDef)
	require.NoError(t, err)
	_, err = db.Exec(nqCreateIndex.Raw().SQL)
	require.NoError(t, err)

	// Verify index exists
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_users_name';")
	require.NoError(t, err)
	defer rows.Close()

	assert.True(t, rows.Next(), "Index 'idx_users_name' should exist")

	var indexName string
	err = rows.Scan(&indexName)
	require.NoError(t, err)
	assert.Equal(t, "idx_users_name", indexName)
}

func TestDropIndexIntegration(t *testing.T) {
	db,_ := setupDDLTestDB(t)
	defer db.Close()

	userSchema := schema.SchemaDefinition{
		Name: "users",
		Fields: map[string]*schema.FieldDefinition{
			"id":   {Name: "id", Type: schema.FieldTypeString},
			"name": {Name: "name", Type: schema.FieldTypeString},
			"age":  {Name: "age", Type: schema.FieldTypeInteger},
		},
	}
	indexDef := schema.IndexDefinition{
		Name:   "idx_users_name",
		Fields: []string{"name"},
		Type:   schema.IndexTypeNormal,
	}

	builder := sqlite.NewSQLiteFactory()
	q := query.Query{
		Target: &query.QueryTarget{
			Name:   userSchema.Name,
			Schema: &userSchema,
		},
	}

	// Create the collection and index first
	nqCreateCollection, err := builder.Build(&q, native.StmtCreateCollection, nil)
	require.NoError(t, err)
	_, err = db.Exec(nqCreateCollection.Raw().SQL)
	require.NoError(t, err)

	nqCreateIndex, err := builder.Build(&q, native.StmtCreateIndex, indexDef)
	require.NoError(t, err)
	_, err = db.Exec(nqCreateIndex.Raw().SQL)
	require.NoError(t, err)

	// Verify index exists before dropping
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_users_name';")
	require.NoError(t, err)
	assert.True(t, rows.Next(), "Index 'idx_users_name' should exist before dropping")
	rows.Close()

	// Drop the index
	nqDropIndex, err := builder.Build(&q, native.StmtDropIndex, indexDef)
	require.NoError(t, err)
	_, err = db.Exec(nqDropIndex.Raw().SQL)
	require.NoError(t, err)

	// Verify index does not exist after dropping
	rows, err = db.Query("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_users_name';")
	require.NoError(t, err)
	assert.False(t, rows.Next(), "Index 'idx_users_name' should not exist after dropping")
	rows.Close()
}
