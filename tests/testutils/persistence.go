package testutils

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	pevents "github.com/asaidimu/go-anansi/v6/core/persistence/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/asaidimu/go-events"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// SetupTestDB creates a unique, in-memory SQLite database for each test.
// The database is automatically cleaned up when the returned function is called.
func SetupTestDB(t *testing.T) (*sql.DB, func()) {
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

	var version string
	err = db.QueryRow("SELECT sqlite_version()").Scan(&version)
	require.NoError(t, err)

	return db, cleanup
}

func NewTestSchema(name ...string) *schema.SchemaDefinition {
	sname := "test_collection"
	if name != nil {
		sname = name[0]
	}
	return &schema.SchemaDefinition{
		Name:        sname,
		Version:     "8.0.0",
		Description: utils.StringPtr("test collection"),
		Fields: map[string]*schema.FieldDefinition{
			"name":      {Name: "name", Type: "string", Required: utils.BoolPtr(true)},
			"age":       {Name: "age", Type: "integer"},
		},
	}
}

func CreateNativeInteractor(t *testing.T) (query.DatabaseInteractor, func()) {
	ConfigureDocumentFactory()
	db, cleanup := SetupTestDB(t)
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	require.NoError(t, err)
	queryFactory := sqliteQuery.NewSQLiteFactory()

	i, err := native.NewNativeInteractor(executor, queryFactory, logger)
	require.NoError(t, err)
	return i, cleanup
}

func SetupCollectionTest(t *testing.T) (base.Collection, func()) {
	interactor, cleanup := CreateNativeInteractor(t)

	logger, _ := zap.NewDevelopment()

	bus, err := events.NewTypedEventBus[base.PersistenceEvent](events.DefaultConfig())
	require.NoError(t, err)

	p, err := persistence.NewPersistence(interactor, pevents.NewGoEventsBusAdapter(bus), logger, nil)
	require.NoError(t, err)

	schema := NewTestSchema("crud_collection")
	collection, err := p.CreateCollection(context.Background(), schema)
	require.NoError(t, err)

	return collection, cleanup
}
