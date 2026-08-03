package document

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureProviderFactory(t *testing.T) {
	t.Helper()
	data.ResetFactoryForTesting()
	require.NoError(t, data.ConfigureDocumentFactory(data.DocumentFactoryConfig{
		Providers: []data.MetadataProviderConfig{
			{
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
			},
		},
	}, nil))
	t.Cleanup(data.ResetFactoryForTesting)
}

// typedModel mirrors the test schema's id/age fields via anansi tags.
type typedModel struct {
	ID  string `anansi:"_id_"`
	ID2 string `anansi:"id"`
	Age int    `anansi:"age"`
}

func TestFromStructFinalizesMetadata(t *testing.T) {
	configureProviderFactory(t)

	d, err := newTestCollection(t).FromStruct(&typedModel{ID2: "user-1", Age: 31})
	require.NoError(t, err)

	// Provider metadata is injected.
	val, err := d.GetMetadataValue("custom_field")
	require.NoError(t, err)
	assert.Equal(t, "custom_value", val)

	// Checksum is computed at construction.
	checksum, err := d.Checksum()
	require.NoError(t, err)
	assert.NotEmpty(t, checksum)
	ok, err := d.VerifyHash()
	require.NoError(t, err)
	assert.True(t, ok)

	// User data landed in the container directly.
	age, err := d.GetInt("age")
	require.NoError(t, err)
	assert.Equal(t, 31, age)
}

func TestFromMapFinalizesMetadata(t *testing.T) {
	configureProviderFactory(t)

	d, err := newTestCollection(t).FromMap(map[string]any{"id": "user-1", "age": int64(31)})
	require.NoError(t, err)

	val, err := d.GetMetadataValue("custom_field")
	require.NoError(t, err)
	assert.Equal(t, "custom_value", val)

	checksum, err := d.Checksum()
	require.NoError(t, err)
	assert.NotEmpty(t, checksum)
}

func TestFromPartialStructDoesNotFinalize(t *testing.T) {
	configureProviderFactory(t)

	d, err := newTestCollection(t).FromPartialStruct(&typedModel{Age: 31})
	require.NoError(t, err)

	// Partial payloads are patches: no generated ID, no provider metadata, no
	// metadata defaults and no checksum.
	assert.Empty(t, d.ID())
	if m := d.Metadata(); len(m) != 0 {
		t.Fatalf("partial struct injected metadata defaults: %v", m)
	}
	_, err = d.GetMetadataValue("custom_field")
	require.Error(t, err)
	checksum, err := d.Checksum()
	require.Error(t, err)
	assert.Empty(t, checksum)
}

func TestFromStructNoProviderConfigured(t *testing.T) {
	data.ResetFactoryForTesting()
	t.Cleanup(data.ResetFactoryForTesting)

	d, err := newTestCollection(t).FromStruct(&typedModel{ID2: "user-1", Age: 31})
	require.NoError(t, err)

	// Without providers the checksum is still computed.
	checksum, err := d.Checksum()
	require.NoError(t, err)
	assert.NotEmpty(t, checksum)
}

func TestProviderCannotSetReservedField(t *testing.T) {
	data.ResetFactoryForTesting()
	require.NoError(t, data.ConfigureDocumentFactory(data.DocumentFactoryConfig{
		Providers: []data.MetadataProviderConfig{
			{
				Name: "rogue",
				Provider: func(_ context.Context, _ data.Documenter) (map[string]any, error) {
					return map[string]any{"version": 99}, nil
				},
			},
		},
	}, nil))
	t.Cleanup(data.ResetFactoryForTesting)

	_, err := newTestCollection(t).FromStruct(&typedModel{ID2: "user-1"})
	require.Error(t, err)
}

func TestFactoryMetadataSchemaMerge(t *testing.T) {
	configureProviderFactory(t)

	// The single shared factory must declare provider fields, so the merged
	// metadata schema can store them.
	msd, _ := data.GetMetadataSchema()
	_, hasField := msd.Fields[definition.FieldId("019f3d5c-847c-7618-a2ea-ac43462b96f7")]
	assert.True(t, hasField)
}
