package document

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v8/core/data"
)

// shadowProduct models what codegen emits when the schema lacks system
// fields: it embeds DocumentModel and declares shadow ID/Metadata fields with
// the canonical system tags.
type shadowProduct struct {
	DocumentModel
	ID       string         `json:"id" anansi:"_id_,required=true"`
	Metadata map[string]any `json:"metadata,omitempty" anansi:"_metadata_,required=false"`
	Name     string         `anansi:"name"`
	Status   string         `anansi:"status"`
}

// shadowProductPreSet carries a caller-supplied ID before New.
type shadowProductPreSet struct {
	DocumentModel
	ID   string `anansi:"_id_,required=true"`
	Name string `anansi:"name"`
}

func TestShadowModel_ExtractorDedupesSystemFields(t *testing.T) {
	schemaBytes, err := data.ExtractDTOSchemaDirect(&shadowProduct{})
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(schemaBytes, &parsed))
	fields, _ := parsed["fields"].(map[string]any)

	countID, countMeta := 0, 0
	for _, fv := range fields {
		fm := fv.(map[string]any)
		switch fm["name"] {
		case "_id_":
			countID++
		case "_metadata_":
			countMeta++
		}
	}
	assert.Equal(t, 1, countID, "extractor must emit _id_ exactly once")
	assert.Equal(t, 1, countMeta, "extractor must emit _metadata_ exactly once")

	// The deduped schema must still build a valid pool.
	col, err := NewDocumentPoolFromJSON(schemaBytes)
	require.NoError(t, err)
	require.NotNil(t, col)
}

func TestShadowModel_NewSyncsShadowID(t *testing.T) {
	model := New(&shadowProduct{Name: "widget"})
	require.NotNil(t, model)
	assert.NotEmpty(t, model.GetID())
	assert.Equal(t, model.GetID(), model.ID, "shadow ID must mirror the embedded model's _id_")
}

func TestShadowModel_PreSetIDHonored(t *testing.T) {
	model := New(&shadowProductPreSet{ID: "fixed-abc", Name: "widget"})
	require.NotNil(t, model)
	assert.Equal(t, "fixed-abc", model.GetID())
	assert.Equal(t, "fixed-abc", model.ID)
}

func TestShadowModel_PartialHonorsIDSkipsMetadata(t *testing.T) {
	col, err := DocumentPoolFromType[*shadowProduct]()
	require.NoError(t, err)

	model := New(&shadowProduct{Name: "widget", Status: "active"})
	patch, err := col.FromPartialStruct(model)
	require.NoError(t, err)

	// The carried _id_ is honored in a patch.
	assert.Equal(t, model.ID, patch.ID())
	// Metadata is system-managed and must not leak into a patch.
	_, ok := patch.Metadata()[data.MetadataCreated]
	assert.False(t, ok, "partial document must not carry metadata")
	assert.Equal(t, "widget", patch.GetOr("name", nil))
}

func TestShadowModel_FullDocumentBuilds(t *testing.T) {
	col, err := DocumentPoolFromType[*shadowProduct]()
	require.NoError(t, err)

	model := New(&shadowProduct{Name: "widget", Status: "active"})
	doc, err := col.FromStruct(model)
	require.NoError(t, err)
	assert.Equal(t, model.ID, doc.ID())
	assert.Equal(t, "widget", doc.GetOr("name", nil))
}

func TestShadowModel_BindRoundTrip(t *testing.T) {
	data.ResetFactoryForTesting()
	require.NoError(t, data.ConfigureDocumentFactory(data.DocumentFactoryConfig{}, nil))

	model := New(&shadowProduct{Name: "widget", Status: "active"})
	doc, err := model.Document()
	require.NoError(t, err)

	back := new(shadowProduct)
	require.NoError(t, doc.BindTo(back))
	// Both the shadow and the embedded model are populated on read-back.
	assert.Equal(t, model.ID, back.ID)
	assert.Equal(t, model.ID, back.GetID())
	assert.Equal(t, "widget", back.Name)
	assert.Equal(t, "active", back.Status)
}
