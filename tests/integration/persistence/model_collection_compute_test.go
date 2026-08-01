package persistence_test

import (
	"context"
	"errors"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v8/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// modelComputeProduct is a model bound to the schema created by newTestSchema.
// Age maps to the integer "age" field and is the target of computed increments.
type modelComputeProduct struct {
	data.DocumentModel
	Name string `anansi:"name"`
	Age  int64  `anansi:"age"`
}

func newModelComputeCollection(t *testing.T) (*collection.ModelCollection[*modelComputeProduct], func()) {
	t.Helper()
	interactor, cleanup := createNativeInteractor(t)
	logger := zap.NewNop()
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	raw, err := p.CreateCollection(context.Background(), newTestSchema("model_compute"))
	require.NoError(t, err)

	mc, err := collection.NewModelCollection[*modelComputeProduct](raw, logger)
	require.NoError(t, err)
	return mc, cleanup
}

func TestModelCollection_UpdateFrom_WithCompute(t *testing.T) {
	mc, cleanup := newModelComputeCollection(t)
	defer cleanup()

	created, err := mc.Create(context.Background(), &modelComputeProduct{Name: "Widget", Age: 10})
	require.NoError(t, err)
	id := created.Model().ID

	update := base.NewCollectionUpdate().
		WithComputedField("age", query.NewQueryBuilder().Increment("age", 1).End().Build()).
		WithReturnDocument(true)
	updated, err := mc.UpdateFrom[*modelComputeProduct, *modelComputeProduct](context.Background(), id,
		&modelComputeProduct{Name: "Widget Pro"},
		*update)
	require.NoError(t, err)
	assert.Equal(t, "Widget Pro", updated.Name)
	assert.Equal(t, int64(11), updated.Age)

	fresh, err := mc.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, int64(11), fresh.Age)
}

func TestModelCollection_UpdateFrom_SkipBinding(t *testing.T) {
	mc, cleanup := newModelComputeCollection(t)
	defer cleanup()

	created, err := mc.Create(context.Background(), &modelComputeProduct{Name: "Widget", Age: 5})
	require.NoError(t, err)
	id := created.Model().ID

	update := base.NewCollectionUpdate().WithReturnDocument(false)
	zero, err := mc.UpdateFrom[*modelComputeProduct, *modelComputeProduct](context.Background(), id,
		&modelComputeProduct{Name: "Renamed"},
		*update)
	require.NoError(t, err)
	assert.Nil(t, zero)

	fresh, err := mc.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", fresh.Name)
	assert.Equal(t, int64(5), fresh.Age)
}

func TestModelCollection_UpdateMany_WithCompute(t *testing.T) {
	mc, cleanup := newModelComputeCollection(t)
	defer cleanup()

	for _, name := range []string{"a", "b", "c"} {
		_, err := mc.Create(context.Background(), &modelComputeProduct{Name: name, Age: 1})
		require.NoError(t, err)
	}

	filter := query.NewQueryBuilder().Where("name").In("a", "b").Build().Filters
	update := base.NewCollectionUpdate().
		WithComputedField("age", query.NewQueryBuilder().Increment("age", 100).End().Build()).
		WithReturnDocument(false)
	count, err := mc.UpdateMany(context.Background(), filter, &modelComputeProduct{}, *update)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	q := query.NewQueryBuilder().OrderByAsc("name").Build()
	results, err := mc.Read(context.Background(), &q)
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, int64(101), results[0].Age) // a
	assert.Equal(t, int64(101), results[1].Age) // b
	assert.Equal(t, int64(1), results[2].Age)   // c untouched
}

func TestModelCollection_Update_RejectsEmptyUpdate(t *testing.T) {
	mc, cleanup := newModelComputeCollection(t)
	defer cleanup()

	created, err := mc.Create(context.Background(), &modelComputeProduct{Name: "Widget", Age: 1})
	require.NoError(t, err)
	id := created.Model().ID

	// An all-zero payload carries no user fields and must be rejected.
	_, err = mc.Update(context.Background(), id, &modelComputeProduct{})
	require.Error(t, err)
	sysErr, ok := err.(*common.SystemError)
	require.True(t, ok)
	assert.Equal(t, base.ErrEmptyUpdate.Code, sysErr.Code)

	// A compute-only update is still allowed even when the payload is empty.
	update := base.NewCollectionUpdate().
		WithComputedField("age", query.NewQueryBuilder().Increment("age", 7).End().Build()).
		WithReturnDocument(true)
	updated, err := mc.Update(context.Background(), id, &modelComputeProduct{}, *update)
	require.NoError(t, err)
	assert.Equal(t, int64(8), updated.Age)

	fresh, err := mc.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, int64(8), fresh.Age)
}

func TestModelCollection_Transact_CommitsOnSuccess(t *testing.T) {
	mc, cleanup := newModelComputeCollection(t)
	defer cleanup()

	created, err := mc.Create(context.Background(), &modelComputeProduct{Name: "Widget", Age: 1})
	require.NoError(t, err)
	id := created.Model().ID

	res, err := mc.Transact(context.Background(), func(ctx context.Context) (any, error) {
		update := base.NewCollectionUpdate().
			WithComputedField("age", query.NewQueryBuilder().Increment("age", 5).End().Build()).
			WithReturnDocument(true)
		return mc.UpdateFrom[*modelComputeProduct, *modelComputeProduct](ctx, id,
			&modelComputeProduct{Name: "Tx"},
			*update)
	})
	require.NoError(t, err)
	updated := res.(*modelComputeProduct)
	assert.Equal(t, "Tx", updated.Name)
	assert.Equal(t, int64(6), updated.Age)

	fresh, err := mc.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "Tx", fresh.Name)
	assert.Equal(t, int64(6), fresh.Age)
}

func TestModelCollection_Transact_RollsBackOnError(t *testing.T) {
	mc, cleanup := newModelComputeCollection(t)
	defer cleanup()

	created, err := mc.Create(context.Background(), &modelComputeProduct{Name: "Widget", Age: 1})
	require.NoError(t, err)
	id := created.Model().ID

	_, err = mc.Transact(context.Background(), func(ctx context.Context) (any, error) {
		update := base.NewCollectionUpdate().
			WithComputedField("age", query.NewQueryBuilder().Increment("age", 100).End().Build())
		if _, err := mc.UpdateFrom[*modelComputeProduct, *modelComputeProduct](ctx, id,
			&modelComputeProduct{Name: "Never"},
			*update); err != nil {
			return nil, err
		}
		return nil, errors.New("boom")
	})
	require.Error(t, err)

	fresh, err := mc.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "Widget", fresh.Name)
	assert.Equal(t, int64(1), fresh.Age)
}

func TestModelCollection_Transact_NestedJoin(t *testing.T) {
	mc, cleanup := newModelComputeCollection(t)
	defer cleanup()

	created, err := mc.Create(context.Background(), &modelComputeProduct{Name: "Widget", Age: 0})
	require.NoError(t, err)
	id := created.Model().ID

	// A nested Transact joins the ambient transaction: the inner one reports
	// success, but the outer one fails, so both operations must roll back.
	_, err = mc.Transact(context.Background(), func(ctx context.Context) (any, error) {
		updateA := base.NewCollectionUpdate().
			WithComputedField("age", query.NewQueryBuilder().Increment("age", 1).End().Build())
		if _, err := mc.UpdateFrom[*modelComputeProduct, *modelComputeProduct](ctx, id,
			&modelComputeProduct{Name: "A"},
			*updateA); err != nil {
			return nil, err
		}
		if _, err := mc.Transact(ctx, func(tctx context.Context) (any, error) {
			updateB := base.NewCollectionUpdate().
				WithComputedField("age", query.NewQueryBuilder().Increment("age", 1).End().Build())
			return mc.UpdateFrom[*modelComputeProduct, *modelComputeProduct](tctx, id,
				&modelComputeProduct{Name: "B"},
				*updateB)
		}); err != nil {
			return nil, err
		}
		return nil, errors.New("outer fails, both roll back")
	})
	require.Error(t, err)

	fresh, err := mc.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "Widget", fresh.Name)
	assert.Equal(t, int64(0), fresh.Age)
}
