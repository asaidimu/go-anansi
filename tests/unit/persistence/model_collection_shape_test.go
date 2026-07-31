package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// shapeProduct is the full model bound to the collection. Fields match the
// schema created by setupCollection (name, status).
type shapeProduct struct {
	data.DocumentModel
	Name   string `anansi:"name"`
	Status string `anansi:"status"`
}

// shapeProductSummary is a projection of shapeProduct exposing a subset of
// fields. It still embeds data.DocumentModel so it satisfies
// data.DocumentModelProvider.
type shapeProductSummary struct {
	data.DocumentModel
	Name string `anansi:"name"`
}

func newShapeModelCollection(t *testing.T) (*collection.ModelCollection[*shapeProduct], context.Context) {
	t.Helper()
	raw, _, _, _, _, ctx := setupCollection(t)
	mc, err := collection.NewModelCollection[*shapeProduct](raw, zap.NewNop())
	require.NoError(t, err)
	return mc, ctx
}

func TestModelCollection_Shape_CreateFindUpdateRead(t *testing.T) {
	mc, ctx := newShapeModelCollection(t)

	created, err := mc.CreateFrom[*shapeProductSummary](ctx, &shapeProductSummary{Name: "Widget"})
	require.NoError(t, err)
	assert.NotEmpty(t, created.Model().ID)
	assert.Equal(t, "Widget", created.Name)

	id := created.Model().ID

	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build()
	got, err := mc.ReadAs[*shapeProductSummary](ctx, &q)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Widget", got[0].Name)
	assert.Equal(t, id, got[0].Model().ID)

	updated, err := mc.UpdateFrom[*shapeProductSummary](ctx, id, &shapeProductSummary{Name: "Widget Pro"})
	require.NoError(t, err)
	assert.Equal(t, "Widget Pro", updated.Name)
	assert.Equal(t, id, updated.Model().ID)

	full, err := mc.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Widget Pro", full.Name)

	q = query.NewQueryBuilder().Where("name").Eq("Widget Pro").Build()
	results, err := mc.ReadAs[*shapeProductSummary](ctx, &q)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Widget Pro", results[0].Name)
	assert.Equal(t, id, results[0].Model().ID)
}

func TestModelCollection_Shape_InteropsWithFullModel(t *testing.T) {
	mc, ctx := newShapeModelCollection(t)

	_, err := mc.Create(ctx, &shapeProduct{Name: "Full", Status: "active"})
	require.NoError(t, err)

	q := query.NewQueryBuilder().Where("name").Eq("Full").Build()
	results, err := mc.ReadAs[*shapeProductSummary](ctx, &q)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Full", results[0].Name)

	id := results[0].Model().ID
	full, err := mc.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "active", full.Status)
}

func TestModelCollection_Shape_ReadAs_NotFound(t *testing.T) {
	mc, ctx := newShapeModelCollection(t)

	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq("does-not-exist").Build()
	results, err := mc.ReadAs[*shapeProductSummary](ctx, &q)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestModelCollection_Shape_UpdateFrom_NotFound(t *testing.T) {
	mc, ctx := newShapeModelCollection(t)

	_, err := mc.UpdateFrom[*shapeProductSummary](ctx, "does-not-exist", &shapeProductSummary{Name: "Nope"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestModelCollection_Shape_ReadAs_Empty(t *testing.T) {
	mc, ctx := newShapeModelCollection(t)

	q := query.NewQueryBuilder().Build()
	results, err := mc.ReadAs[*shapeProductSummary](ctx, &q)
	require.NoError(t, err)
	assert.Empty(t, results)
}
