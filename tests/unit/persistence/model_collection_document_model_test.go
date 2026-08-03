package persistence_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/document"
	"github.com/asaidimu/go-anansi/v8/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// documentProduct embeds document.DocumentModel — the container-backed model —
// rather than data.DocumentModel. It still satisfies ModelIdentity via the
// promoted GetID, so it works with ModelCollection out of the box.
type documentProduct struct {
	document.DocumentModel
	Name   string `anansi:"name"`
	Status string `anansi:"status"`
}

// documentProductSummary is a projection of documentProduct. Like the full
// model it embeds document.DocumentModel, proving shape operations accept the
// same generalized constraint.
type documentProductSummary struct {
	document.DocumentModel
	Name string `anansi:"name"`
}

func TestModelCollection_DocumentModelDefaultConverters(t *testing.T) {
	raw, _, _, _, _, ctx := setupCollection(t)

	// No ToDocumenter/ToPartialDocumenter overrides: the data-factory default
	// pipeline builds documents from a document.DocumentModel-embedding struct,
	// proving it is a true drop-in for data.DocumentModel.
	mc, err := collection.NewModelCollection[*documentProduct](raw, zap.NewNop())
	require.NoError(t, err)

	created, err := mc.Create(ctx, &documentProduct{Name: "Plain", Status: "new"})
	require.NoError(t, err)
	assert.NotEmpty(t, created.GetID())
	assert.Equal(t, "Plain", created.Name)
	assert.Equal(t, "new", created.Status)

	id := created.GetID()
	full, err := mc.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Plain", full.Name)
}

func TestModelCollection_ReleasesConvertedDocument(t *testing.T) {
	raw, _, _, _, _, ctx := setupCollection(t)

	var produced *document.Document
	toDocumenter := func(_ context.Context, model any) (data.Documenter, error) {
		doc, err := document.New(model.(*documentProduct)).Document()
		produced = doc
		return doc, err
	}

	mc, err := collection.NewModelCollection[*documentProduct](raw, zap.NewNop(),
		collection.ModelCollectionOptions[*documentProduct]{ToDocumenter: toDocumenter})
	require.NoError(t, err)

	_, err = mc.Create(ctx, &documentProduct{Name: "x", Status: "y"})
	require.NoError(t, err)
	require.NotNil(t, produced)

	// The converted document was consumed by persistence and its pooled
	// container returned to the per-type collection's pool. Release nils the
	// container, so any access panics — that is the observable released state.
	require.Panics(t, func() {
		produced.GetOr("name", nil)
	}, "converted document must have been released")
}

func TestModelCollection_DocumentModelFullPipeline(t *testing.T) {
	raw, _, _, _, _, ctx := setupCollection(t)

	toDocumenter := func(_ context.Context, model any) (data.Documenter, error) {
		return document.New(model.(*documentProduct)).Document()
	}
	toPartialDocumenter := func(_ context.Context, model any) (data.Documenter, error) {
		return document.New(model.(*documentProduct)).Patch()
	}

	mc, err := collection.NewModelCollection[*documentProduct](raw, zap.NewNop(),
		collection.ModelCollectionOptions[*documentProduct]{
			ToDocumenter:        toDocumenter,
			ToPartialDocumenter: toPartialDocumenter,
		})
	require.NoError(t, err)

	created, err := mc.Create(ctx, &documentProduct{Name: "Boxed", Status: "active"})
	require.NoError(t, err)
	assert.NotEmpty(t, created.GetID())
	assert.Equal(t, "Boxed", created.Name)
	assert.Equal(t, "active", created.Status)

	id := created.GetID()

	full, err := mc.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Boxed", full.Name)
	assert.Equal(t, "active", full.Status)

	q := query.NewQueryBuilder().Where("name").Eq("Boxed").Build()
	results, err := mc.ReadAs[*documentProductSummary](ctx, &q)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Boxed", results[0].Name)

	updated, err := mc.Update(ctx, id, &documentProduct{Name: "Boxed", Status: "done"})
	require.NoError(t, err)
	assert.Equal(t, "done", updated.Status)
	assert.Equal(t, id, updated.GetID())
}
