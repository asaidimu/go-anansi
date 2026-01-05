package data_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	pevents "github.com/asaidimu/go-anansi/v6/core/persistence/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	"github.com/asaidimu/go-anansi/v6/tests/testutils"
	"github.com/asaidimu/go-events"
	_ "github.com/mattn/go-sqlite3"
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

	var version string
	err = db.QueryRow("SELECT sqlite_version()").Scan(&version)
	require.NoError(t, err)

	return db, cleanup
}

func newTestSchema(name ...string) *schema.SchemaDefinition {
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

func createNativeInteractor(t *testing.T) (query.DatabaseInteractor, func()) {
	testutils.ConfigureDocumentFactory()
	db, cleanup := setupTestDB(t)
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	require.NoError(t, err)
	queryFactory := sqliteQuery.NewSQLiteFactory()

	i, err := native.NewNativeInteractor(executor, queryFactory, logger)
	require.NoError(t, err)
	return i, cleanup
}

func setupCollectionTest(t *testing.T) (base.Collection, func()) {
	interactor, cleanup := createNativeInteractor(t)

	logger, _ := zap.NewDevelopment()

	bus, err := events.NewTypedEventBus[base.PersistenceEvent](events.DefaultConfig())
	require.NoError(t, err)

	p, err := persistence.NewPersistence(interactor, pevents.NewGoEventsBusAdapter(bus), logger, nil)
	require.NoError(t, err)

	schema := newTestSchema("crud_collection")
	collection, err := p.CreateCollection(context.Background(), *schema)
	require.NoError(t, err)

	return collection, cleanup
}

func TestDocumentHashingIntegrity(t *testing.T) {
	col, cleanup := setupCollectionTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Create a document
	initialDoc, err := data.NewDocument(map[string]any{"name": "test_doc", "age": 30})
	require.NoError(t, err)
	require.NotNil(t, initialDoc)

	// 2. Insert the document
	results, err := col.CreateMany(ctx, []*data.Document{initialDoc})
	require.NoError(t, err)

	resultSet := base.CreateResultSet(results)
	require.Len(t, resultSet.Documents(), 1)
	createdDoc := resultSet.Documents()[0]

	// 3. Retrieve the document from the database
	q := query.NewQueryBuilder().Where("id").Eq(createdDoc.ID()).Build()
	readResultSet, err := col.Read(ctx, &q)
	require.NoError(t, err)
	require.Len(t, readResultSet.Data, 1)
	retrievedDoc := readResultSet.Data[0]

	// 4. Verify the hash of the retrieved document
	ok, err := retrievedDoc.VerifyHash()
	require.NoError(t, err, "Error verifying hash for retrieved document")
	require.True(t, ok, "Retrieved document hash should be valid")

	// 5. Ensure the content and metadata are consistent
	require.Equal(t, createdDoc.ID(), retrievedDoc.ID())
	require.Equal(t, createdDoc.MustGet("name"), retrievedDoc.MustGet("name"))
	require.Equal(t, createdDoc.MustGet("age"), retrievedDoc.MustGet("age"))

	// Verify metadata fields that should be preserved and updated
	initialMeta, _ := createdDoc.Metadata()
	retrievedMeta, _ := retrievedDoc.Metadata()

	require.Equal(t, initialMeta[data.MetadataChecksum], retrievedMeta[data.MetadataChecksum])
	require.Equal(t, initialMeta[data.MetadataVersion], retrievedMeta[data.MetadataVersion])
	require.Equal(t, initialMeta[data.MetadataCreated], retrievedMeta[data.MetadataCreated])
}

