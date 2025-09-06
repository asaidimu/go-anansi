package persistence_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestConcurrentTransactions(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	p, err := persistence.NewPersistence(interactor, logger, nil)
	require.NoError(t, err)

	schema := newTestSchema("concurrent_test")
	collection, err := p.CreateCollection(context.Background(), *schema)
	require.NoError(t, err)

	numConcurrent := 5
	var wg sync.WaitGroup
	wg.Add(numConcurrent)

	for i := range numConcurrent {
		go func(id int) {
			defer wg.Done()

			_, err := p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
				docID := fmt.Sprintf("concurrent-%d", id)
				_, err := collection.CreateOne(tctx, data.Document{"id": docID, "name": "test"})
				return nil, err
			})

			require.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all data was created
	for i := range numConcurrent {
		docID := fmt.Sprintf("concurrent-%d", i)
		q := query.NewQueryBuilder().Where("id").Eq(docID).Build()
		result, err := collection.Read(context.Background(), &q)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Count, "document %s should exist", docID)
	}
}
