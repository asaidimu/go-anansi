package e2e_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	anansi "github.com/asaidimu/go-anansi/v7"
	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/query/native"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	sqliteExecutor "github.com/asaidimu/go-anansi/v7/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v7/sqlite/query"
	_ "github.com/mattn/go-sqlite3" // Import for SQLite driver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// setupTestDB creates a unique, in-memory SQLite database for each test.
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite3", dsn)
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
	}
	return db, cleanup
}

// createNativeInteractor creates a query.DatabaseInteractor for SQLite.
func createNativeInteractor(t *testing.T) (query.DatabaseInteractor, func()) {
	db, cleanupDB := setupTestDB(t)
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	require.NoError(t, err)
	queryFactory := sqliteQuery.NewSQLiteFactory(logger)

	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	require.NoError(t, err)

	return interactor, cleanupDB
}

func newTestSchema(name string) *definition.Schema {

	version, _ := common.NewVersion("8.0.0")
	return &definition.Schema{
		Version: version,
		BaseSchema: definition.BaseSchema{
			Name:        name,
			Description: "test collection",
			Fields: map[definition.FieldId]definition.Field{
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}
}

func TestAnansiSetupAndBasicOperation(t *testing.T) {
	// 1. Setup Database Interactor
	interactor, cleanupInteractor := createNativeInteractor(t)
	defer cleanupInteractor()

	// 2. Setup Logger
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	// 3. Setup Document Factory Config
	factoryConfig := data.DocumentFactoryConfig{
		Providers: []data.MetadataProviderConfig{},
	}

	// 4. Setup Decorators (none for this basic test)
	decorators := &utils.Decorators{}

	// 5. Call anansi.Setup
	cfg := anansi.SetupConfig{
		Interactor:            interactor,
		Logger:                logger,
		DocumentFactoryConfig: factoryConfig,
		Decorators:            decorators,
	}
	p, err := anansi.Setup(cfg)

	// Assert setup was successful
	require.NoError(t, err)
	assert.NotNil(t, p)

	// 6. Perform a basic operation to verify persistence is functional
	testSchema := newTestSchema("e2e_collection")
	collection, err := p.CreateCollection(context.Background(), testSchema)
	require.NoError(t, err)
	assert.NotNil(t, collection)

	// Verify the collection can be retrieved
	retrievedCollection, err := p.Collection(context.Background(), "e2e_collection")
	require.NoError(t, err)
	assert.NotNil(t, retrievedCollection)
	assert.Equal(t, collection, retrievedCollection)
}

func TestAnansiSetupCalledTwice(t *testing.T) {
	// This test verifies that Setup can only be called once.
	// The first call will succeed, subsequent calls should return the same instance and no error.

	// 1. Setup Database Interactor (for first call)
	interactor1, cleanupInteractor1 := createNativeInteractor(t)
	defer cleanupInteractor1()

	// 2. Setup Logger
	logger1, err := zap.NewDevelopment()
	require.NoError(t, err)

	// 3. Setup Document Factory Config
	factoryConfig1 := data.DocumentFactoryConfig{}

	// 4. Setup Decorators
	decorators1 := &utils.Decorators{}

	// First call to Setup
	cfg1 := anansi.SetupConfig{
		Interactor:            interactor1,
		Logger:                logger1,
		DocumentFactoryConfig: factoryConfig1,
		Decorators:            decorators1,
	}
	p1, err1 := anansi.Setup(cfg1)
	require.NoError(t, err1)
	assert.NotNil(t, p1)

	// Second call to Setup with different parameters
	// These parameters should be ignored because Setup should only run once.
	interactor2, cleanupInteractor2 := createNativeInteractor(t)
	defer cleanupInteractor2()
	logger2, err := zap.NewDevelopment()
	require.NoError(t, err)
	factoryConfig2 := data.DocumentFactoryConfig{}
	decorators2 := &utils.Decorators{}

	cfg2 := anansi.SetupConfig{
		Interactor:            interactor2,
		Logger:                logger2,
		DocumentFactoryConfig: factoryConfig2,
		Decorators:            decorators2,
	}
	p2, err2 := anansi.Setup(cfg2)

	// Assert that the second call returns the same instance and no error
	require.NoError(t, err2)
	assert.NotNil(t, p2)
	assert.Equal(t, p1, p2) // Should return the same persistence instance
}
