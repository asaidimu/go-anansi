package persistence_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTransactionWithGoroutine(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	p, err := persistence.NewPersistence(interactor,nil, logger, nil)
	require.NoError(t, err)

	schema := newTestSchema("goroutine_test")
	collection, err := p.CreateCollection(context.Background(), schema)
	require.NoError(t, err)

	t.Run("Successful transaction with goroutine", func(t *testing.T) {
		_, err := p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
			tx.Async(tctx, func(ctx context.Context) (any, error) {
				coll, err := tx.Collection(ctx, "goroutine_test")
				if err != nil {
					return nil, err
				}

				_, err = coll.CreateOne(ctx, data.MustNewDocument(map[string]any{"id": "1", "name": "test"}))
				return nil, err
			})

			return nil, nil
		})

		require.NoError(t, err)

		// Verify the data was created.
		q := query.NewQueryBuilder().Where("name").Eq("test").Build()
		result, err := collection.Read(context.Background(), &q)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Count)
	})

	t.Run("Failing transaction with goroutine", func(t *testing.T) {
		// Clean up any data from the previous test
		_, err := collection.Delete(context.Background(), query.NewQueryBuilder().Where("name").Eq("test").Build().Filters, false)
		require.NoError(t, err)

		_, err = p.Transact(context.Background(), func(tctx context.Context, tx base.BasePersistence) (any, error) {
			tx.Async(tctx, func(ctx context.Context) (any, error) {
				return nil, fmt.Errorf("goroutine failed")
			})

			return nil, nil
		})

		require.Error(t, err)
		var sysErr *common.SystemError
		require.ErrorAs(t, err, &sysErr)
		assert.Equal(t, base.ErrTransactionAsyncOperationFailed.Code, sysErr.Code)

		// Verify the data was not created.
		q := query.NewQueryBuilder().Where("name").Eq("test").Build()
		result, err := collection.Read(context.Background(), &q)
		require.NoError(t, err)
		assert.Equal(t, 0, result.Count)
	})
}

func TestTransactionWithNestedGoroutine(t *testing.T) {
	interactor, cleanup := createNativeInteractor(t)
	defer cleanup()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	p, err := persistence.NewPersistence(interactor,nil, logger, nil)
	require.NoError(t, err)

	schema := newTestSchema("nested_goroutine_test")
	_, err = p.CreateCollection(context.Background(), schema)
	require.NoError(t, err)

	_, err = p.Transact(context.Background(), func(tctx context.Context, tx1 base.BasePersistence) (any, error) {
		// First level of transaction
		_, err := tx1.Collection(tctx, "nested_goroutine_test")
		if err != nil {
			return nil, err
		}

		// Nested transaction
		_, err = p.Transact(tctx, func(tctx2 context.Context, tx2 base.BasePersistence) (any, error) {
			tx2.Async(tctx2, func(ctx context.Context) (any, error) {
				coll, err := tx2.Collection(ctx, "nested_goroutine_test")
				if err != nil {
					return nil, err
				}

				_, err = coll.CreateOne(ctx, data.MustNewDocument(map[string]any{"id": "nested", "name": "testa"}))
				return nil, err
			})

			return nil, nil
		})

		return nil, err
	})

	require.NoError(t, err)

	// Verify the data was created.
	collection, err := p.Collection(context.Background(), "nested_goroutine_test")
	require.NoError(t, err)
	q := query.NewQueryBuilder().Where("name").Eq("testa").Build()
	result, err := collection.Read(context.Background(), &q)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Count)
}

