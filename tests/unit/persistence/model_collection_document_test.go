package persistence_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/document"
	"github.com/asaidimu/go-anansi/v8/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// containerConverter builds a container-backed document.Document from a model
// via a document.DocumentPool — the underlying container is populated
// field-by-field from the struct's anansi tags, with no map[string]any
// intermediate — proving the ModelCollection write path accepts any
// data.Documenter.
func containerConverter(schemaDef *definition.Schema) collection.ModelConverter {
	return func(ctx context.Context, model any) (data.Documenter, error) {
		col, err := document.NewDocumentPool(schemaDef)
		if err != nil {
			return nil, err
		}
		return col.FromStruct(model, document.WithContext(ctx))
	}
}

func TestModelCollection_ContainerBackedDocumenter(t *testing.T) {
	// The collection validates against a schema enriched from the data
	// factory's metadata providers; the container-backed document gets its
	// provider metadata from the same shared factory.
	data.ResetFactoryForTesting()
	testutils.ConfigureDocumentFactory(data.MetadataProviderConfig{
		Name: "custom",
		Schema: &definition.NestedSchema{
			BaseSchema: definition.BaseSchema{
				Name: "custom_meta",
				Fields: map[definition.FieldId]definition.Field{
					"019f3d5c-847c-7618-a2ea-ac43462b96f7": {
						Name: "custom_field", Required: true,
						FieldProperties: definition.FieldProperties{
							Type: definition.FieldTypeString,
						},
					},
				},
			},
		},
		Provider: func(_ context.Context, _ data.Documenter) (map[string]any, error) {
			return map[string]any{"custom_field": "custom_value"}, nil
		},
	})
	t.Cleanup(func() {
		data.ResetFactoryForTesting()
		testutils.ConfigureDocumentFactory()
	})

	raw, _, _, schemaDef, _, ctx := setupCollection(t)

	var conversions atomic.Int64
	conv := func(ctx context.Context, model any) (data.Documenter, error) {
		conversions.Add(1)
		return containerConverter(schemaDef)(ctx, model)
	}

	mc, err := collection.NewModelCollection[*shapeProduct](raw, zap.NewNop(),
		collection.ModelCollectionOptions[*shapeProduct]{ToDocumenter: conv})
	require.NoError(t, err)

	created, err := mc.Create(ctx, &shapeProduct{Name: "Boxed", Status: "active"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), conversions.Load(), "pluggable converter must be used instead of the default")
	assert.NotEmpty(t, created.Model().ID)
	assert.Equal(t, "Boxed", created.Name)
	assert.Equal(t, "active", created.Status)

	id := created.Model().ID

	full, err := mc.FindByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Boxed", full.Name)
	assert.Equal(t, "active", full.Status)

	q := query.NewQueryBuilder().Where("name").Eq("Boxed").Build()
	results, err := mc.ReadAs[*shapeProductSummary](ctx, &q)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Boxed", results[0].Name)

	updated, err := mc.Update(ctx, id, &shapeProduct{Name: "Boxed", Status: "done"})
	require.NoError(t, err)
	assert.Equal(t, "done", updated.Status)
	assert.Equal(t, id, updated.Model().ID)
}
