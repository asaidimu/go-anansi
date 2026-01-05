package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	pevents "github.com/asaidimu/go-anansi/v6/core/persistence/events"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

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

func TestCollection_Create(t *testing.T) {
	collection, cleanup := setupCollectionTest(t)
	defer cleanup()

	docToCreate := data.MustNewDocument(map[string]any{"name": "test-doc"})
	_, err := collection.CreateOne(context.Background(), docToCreate)
	require.NoError(t, err)

	readQuery := query.NewQueryBuilder().Where("name").Eq("test-doc").Build()
	readResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	assert.Equal(t, 1, readResult.Count)
}

func TestCollection_Read(t *testing.T) {
	collection, cleanup := setupCollectionTest(t)
	defer cleanup()

	docToCreate := data.MustNewDocument(map[string]any{"id": "1", "name": "test-doc"})
	_, err := collection.CreateOne(context.Background(), docToCreate)
	require.NoError(t, err)

	readQuery := query.NewQueryBuilder().Where("name").Eq("test-doc").Build()
	readResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	assert.Equal(t, 1, readResult.Count)
	readDoc := readResult.Data[0]
	assert.Equal(t, "test-doc", readDoc.MustGet("name"))
}

func TestCollection_Update(t *testing.T) {
	collection,  cleanup := setupCollectionTest(t)
	defer cleanup()

	docToCreate := data.MustNewDocument(map[string]any{"name": "test-doc"})
	_, err := collection.CreateOne(context.Background(), docToCreate)
	require.NoError(t, err)

	id := docToCreate.ID()

	readQuery := query.NewQueryBuilder().Where("id").Eq(id).Build()

	readUpdatedResult, err := collection.Read(context.Background(), &readQuery)
	require.NotNil(t, readUpdatedResult)

	require.NoError(t, err)
	assert.Equal(t, 1, readUpdatedResult.Count)

	readUpdatedDoc := readUpdatedResult.Data[0]

	docToUpdate := data.Patch{"name": "updated-doc"}

	updateQuery := query.NewQueryBuilder().Where("id").Eq(id).Build()

	_, err = collection.Update(context.Background(), &base.CollectionUpdate{
		Set:   docToUpdate.Document(),
		Filter: updateQuery.Filters,
	})

	require.NoError(t, err)

	readUpdatedResult, err = collection.Read(context.Background(), &readQuery)
	require.NotNil(t, readUpdatedResult)

	require.NoError(t, err)
	assert.Equal(t, 1, readUpdatedResult.Count) // and this will fail
	readUpdatedDoc = readUpdatedResult.Data[0]
	assert.Equal(t, "updated-doc", readUpdatedDoc.MustGet("name"))
}

func TestCollection_Delete(t *testing.T) {
	collection, cleanup := setupCollectionTest(t)
	defer cleanup()

	docToCreate := data.MustNewDocument(map[string]any{"id": "1", "name": "test-doc"})
	_, err := collection.CreateOne(context.Background(), docToCreate)
	require.NoError(t, err)

	deleteQuery := query.NewQueryBuilder().Where("name").Eq("test-doc").Build()
	deleteResult, err := collection.Delete(context.Background(), deleteQuery.Filters, false)
	require.NoError(t, err)
	assert.Equal(t, 1, deleteResult)

	readQuery := query.NewQueryBuilder().Where("name").Eq("test-doc").Build()
	readDeletedResult, err := collection.Read(context.Background(), &readQuery)
	require.NoError(t, err)
	assert.Equal(t, 0, readDeletedResult.Count)
}
