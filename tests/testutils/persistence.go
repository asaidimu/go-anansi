package testutils

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	pevents "github.com/asaidimu/go-anansi/v7/core/persistence/events"
	"github.com/asaidimu/go-anansi/v7/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/query/native"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	sqliteExecutor "github.com/asaidimu/go-anansi/v7/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v7/sqlite/query"
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

func NewTestSchema(name ...string) *definition.Schema {
	sname := "test_collection"
	if name != nil {
		sname = name[0]
	}
	version, _ := common.NewVersion("1.0.0")
	return &definition.Schema{
		Version: version,
		BaseSchema: definition.BaseSchema{
			Name:        sname,
			Description: "test collection",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
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
	queryFactory := sqliteQuery.NewSQLiteFactory(nil)

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
