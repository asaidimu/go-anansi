package persistence_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/document"
	"github.com/asaidimu/go-anansi/v8/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// shadowProduct mirrors a generated model whose schema lacks system fields:
// it embeds document.DocumentModel and declares shadow ID/Metadata fields
// with the canonical system tags. document.New must keep the shadow ID in
// sync, and the pool must treat the shadowed system fields as a single source
// of truth end-to-end.
type shadowProduct struct {
	document.DocumentModel
	ID       string         `json:"id" anansi:"_id_,required=true"`
	Metadata map[string]any `json:"metadata,omitempty" anansi:"_metadata_,required=false"`
	Name     string         `anansi:"name"`
	Status   string         `anansi:"status"`
}

func TestModelCollection_ShadowedSystemFields(t *testing.T) {
	raw, _, _, _, _, ctx := setupCollection(t)

	mc, err := collection.NewModelCollection[*shadowProduct](raw, zap.NewNop())
	require.NoError(t, err)

	created, err := mc.Create(ctx, document.New(&shadowProduct{Name: "Shadow", Status: "new"}))
	require.NoError(t, err)
	assert.NotEmpty(t, created.GetID())
	assert.Equal(t, created.GetID(), created.ID, "created model's shadow ID must equal the record ID")
	assert.Equal(t, "Shadow", created.Name)
	assert.Equal(t, "new", created.Status)

	id := created.GetID()

	full, err := mc.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Shadow", full.Name)
	assert.Equal(t, id, full.ID)

	q := query.NewQueryBuilder().Where("name").Eq("Shadow").Build()
	results, err := mc.ReadAs[*shadowProduct](ctx, &q)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Shadow", results[0].Name)
	assert.Equal(t, id, results[0].GetID())

	updated, err := mc.Update(ctx, id, &shadowProduct{Name: "Shadow", Status: "done"})
	require.NoError(t, err)
	assert.Equal(t, "done", updated.Status)
	assert.Equal(t, id, updated.GetID())
	assert.Equal(t, id, updated.ID)
}
