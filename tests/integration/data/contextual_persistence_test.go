package data_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)


var factoryOnce sync.Once

// setupPersistenceWithContextualProvider creates a real persistence layer with a
// document factory configured for a specific contextual provider.
func setupPersistenceWithContextualProvider(t *testing.T) (base.Persistence, func()) {
	// 1. Define and configure the context-aware metadata provider
	factoryOnce.Do(func() {
		contextualProvider := data.MetadataProviderConfig{
			Name: "contextual",
			Provider: func(ctx context.Context, _ *data.Document) (map[string]any, error) {
				if traceID, ok := ctx.Value(traceIDKey).(string); ok {
					return map[string]any{"trace_id": traceID}, nil
				}
				return nil, nil
			},
		}
		config := data.DocumentFactoryConfig{
			Providers: []data.MetadataProviderConfig{contextualProvider},
		}
		logger, _ := zap.NewDevelopment()
		err := data.ConfigureDocumentFactory(config, logger)
		require.NoError(t, err, "DocumentFactory should be configurable once")
	})

	// 2. Set up the database and interactor
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite3", dsn)
	require.NoError(t, err)

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	require.NoError(t, err)

	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	require.NoError(t, err)

	// 3. Set up the persistence layer
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	cleanup := func() {
		p.Close(context.Background())
		db.Close()
	}

	return p, cleanup
}

// TestContextualMetadataIsPersisted verifies that metadata injected from the context
// during document creation is correctly persisted to and retrieved from the database.
func TestContextualMetadataIsPersisted(t *testing.T) {
	// 1. Set up persistence with the special contextual provider
	p, cleanup := setupPersistenceWithContextualProvider(t)
	defer cleanup()

	// 2. Create a collection
	collectionName := "context_test_collection"
	testSchema := &schema.SchemaDefinition{
		Name:    collectionName,
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"field": {Name: "field", Type: "string"},
		},
	}
	collection, err := p.CreateCollection(context.Background(), testSchema)
	require.NoError(t, err)

	// 3. Create a context with the traceID
	expectedTraceID := "trace-id-for-persistence-test"
	ctxWithTraceID := context.WithValue(context.Background(), traceIDKey, expectedTraceID)

	// 4. Create a new document using the context-aware NewDocument function
	doc, err := data.NewDocument(map[string]any{"field": "some_value"}, ctxWithTraceID)
	require.NoError(t, err)

	// 5. Persist the document
	createResult, err := collection.CreateOne(context.Background(), doc)
	require.NoError(t, err)
	docID := createResult.Data.ID()

	// 6. Read the document back from the database
	readQuery := query.NewQueryBuilder().Where("id").Eq(docID).Build()
	readResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	require.Equal(t, 1, len(readResult.Data))
	readDoc := readResult.Data[0]

	// 7. Assert that the contextual metadata was persisted and retrieved
	retrievedTraceID, err := readDoc.GetMetadataString("trace_id")
	require.NoError(t, err)
	assert.Equal(t, expectedTraceID, retrievedTraceID, "The trace_id from the context should be persisted and retrieved from the database.")
}
