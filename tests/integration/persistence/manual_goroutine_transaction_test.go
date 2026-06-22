package persistence_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	pevents "github.com/asaidimu/go-anansi/v7/core/persistence/events"
	"github.com/asaidimu/go-anansi/v7/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTransactionInManualGoroutine(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	bus, err := events.NewTypedEventBus[base.PersistenceEvent](events.DefaultConfig())
	require.NoError(t, err)

	p, err := persistence.NewPersistence(interactor, pevents.NewGoEventsBusAdapter(bus), logger, nil)
	require.NoError(t, err)

	schema := newTestSchema("manual_goroutine_test")
	collection, err := p.CreateCollection(context.Background(), schema)
	require.NoError(t, err)

	t.Run("Successful transaction in a manual goroutine", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
				// This inner goroutine's operation will be coordinated by the transaction.
				tx.Async(tctx, func(ctx context.Context) (any, error) {
					_, err := collection.CreateOne(ctx, data.MustNewDocument(map[string]any{"name": "test3"}))
					return nil, err
				})
				return nil, nil
			})
			require.NoError(t, err)
		}()

		wg.Wait()

		// Verify the data was created.
		q := query.NewQueryBuilder().Where("name").Eq("test3").Build()
		result, err := collection.Read(context.Background(), &q)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Count)
	})

	t.Run("Failing transaction in a manual goroutine", func(t *testing.T) {
		// Clean up data from the previous test
		_, err := collection.Delete(context.Background(), query.NewQueryBuilder().Where("name").Eq("test3").Build().Filters, false)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
				// This inner goroutine's error will cause the transaction to roll back.
				tx.Async(tctx, func(ctx context.Context) (any, error) {
					return nil, fmt.Errorf("manual goroutine failed")
				})
				return nil, nil
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed")
		}()

		wg.Wait()

		// Verify the data was not created.
		q := query.NewQueryBuilder().Where("name").Eq("test3").Build()
		result, err := collection.Read(context.Background(), &q)
		require.NoError(t, err)
		assert.Equal(t, 0, result.Count)
	})
}
