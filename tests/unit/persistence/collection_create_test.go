package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/ephemeral"
	persistence "github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// setupEphemeralManager creates a new ephemeral interactor and returns its schema manager.
func setupEphemeralManager(_ *testing.T) (query.DatabaseInteractor, *zap.Logger, context.Context) {
	ephemeralInteractor := ephemeral.NewEphemeral()
	logger := zap.NewNop()
	ctx := context.Background()
	return ephemeralInteractor, logger, ctx
}

func TestEphemeralManager_CreateMultipleCollectionsWithSameNameFails(t *testing.T) {
	ephemeralInteractor, _, ctx := setupEphemeralManager(t)
	manager := ephemeralInteractor.SchemaManager()

	testSchemaDef := testSchema("my_unique_collection")

	// First creation attempt - should succeed
	err := manager.CreateCollection(ctx, *testSchemaDef)
	assert.NoError(t, err, "First collection creation should succeed")

	// Second creation attempt with the same name - should fail
	err = manager.CreateCollection(ctx, *testSchemaDef)
	assert.Error(t, err, "Second collection creation with the same name should fail")
	assert.ErrorIs(t, err, persistence.ErrCollectionAlreadyExists, "Error should indicate collection already exists")
}
