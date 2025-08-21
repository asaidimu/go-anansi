package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupCollectionTest(t *testing.T) (base.Collection, func()) {
	interactor, cleanup := createNativeInteractor(t)

	logger, _ := zap.NewDevelopment()

	p, err := persistence.NewPersistence(interactor, logger, nil)
	require.NoError(t, err)

	schema := newTestSchema("crud_collection")
	collection, err := p.Create(context.Background(), *schema)
	require.NoError(t, err)

	return collection, cleanup
}

func TestCollection_Create(t *testing.T) {
	collection, cleanup := setupCollectionTest(t)
	defer cleanup()

	docToCreate := data.Document{"id": "1", "name": "test-doc"}
	_, err := collection.CreateOne(context.Background(), docToCreate)
	require.NoError(t, err)

	readQuery := query.NewQueryBuilder().Where("id").Eq("1").Build()
	readResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	assert.Equal(t, 1, readResult.Count)
}

func TestCollection_Read(t *testing.T) {
	collection, cleanup := setupCollectionTest(t)
	defer cleanup()

	docToCreate := data.Document{"id": "1", "name": "test-doc"}
	_, err := collection.CreateOne(context.Background(), docToCreate)
	require.NoError(t, err)

	readQuery := query.NewQueryBuilder().Where("id").Eq("1").Build()
	readResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	assert.Equal(t, 1, readResult.Count)
	readDoc := readResult.Data.(data.Document)
	assert.Equal(t, "test-doc", readDoc["name"])
}

func TestCollection_Update(t *testing.T) {
	collection, cleanup := setupCollectionTest(t)
	defer cleanup()

	docToCreate := data.Document{"id": "1", "name": "test-doc"}
	_, err := collection.CreateOne(context.Background(), docToCreate)
	require.NoError(t, err)

	readQuery := query.NewQueryBuilder().Where("id").Eq("1").Build()
	readResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	readDoc := readResult.Data.(data.Document)

	docToUpdate := readDoc.Clone()
	docToUpdate["name"] = "updated-doc"

	updateQuery := query.NewQueryBuilder().Where("id").Eq("1").Build()

	_, err = collection.Update(context.Background(), &base.CollectionUpdate{
		Data:   docToUpdate,
		Filter: updateQuery.Filters,
	})

	require.NoError(t, err)

	readUpdatedResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	assert.Equal(t, 1, readUpdatedResult.Count)
	readUpdatedDoc := readUpdatedResult.Data.(data.Document)
	assert.Equal(t, "updated-doc", readUpdatedDoc["name"])
}

func TestCollection_Delete(t *testing.T) {
	collection, cleanup := setupCollectionTest(t)
	defer cleanup()

	docToCreate := data.Document{"id": "1", "name": "test-doc"}
	_, err := collection.CreateOne(context.Background(), docToCreate)
	require.NoError(t, err)

	deleteQuery := query.NewQueryBuilder().Where("id").Eq("1").Build()
	deleteResult, err := collection.Delete(context.Background(), deleteQuery.Filters, false)
	require.NoError(t, err)
	assert.Equal(t, 1, deleteResult)

	readQuery := query.NewQueryBuilder().Where("id").Eq("1").Build()
	readDeletedResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	assert.Equal(t, 0, readDeletedResult.Count)
}
