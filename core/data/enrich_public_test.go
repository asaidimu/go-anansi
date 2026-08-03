package data_test

import (
	"reflect"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/document"
	"github.com/asaidimu/go-anansi/v8/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const convergenceSchemaJSON = `{
  "version": "1.0.0",
  "name": "convtest",
  "fields": {
    "019f4066-1000-7000-8000-000000000001": { "name": "name", "type": "string", "required": true },
    "019f4066-1000-7000-8000-000000000002": { "name": "age",  "type": "integer" }
  }
}`

// TestEnrichConvergence pins the shared-enrichment contract: the
// container-backed document layer and the persistence registry must produce the
// same field IDs, field order, and metadata schema as data.EnrichSchema.
func TestEnrichConvergence(t *testing.T) {
	s, err := definition.FromJSON([]byte(convergenceSchemaJSON))
	if err != nil {
		t.Fatal(err)
	}

	meta, deps := data.GetMetadataSchema()
	dataOut, err := data.EnrichSchema(s, meta, deps)
	if err != nil {
		t.Fatal(err)
	}

	regOut, err := registry.EnrichSchema(s)
	if err != nil {
		t.Fatal(err)
	}
	// Normalize both sides through JSON so representational differences (nil vs
	// empty maps introduced by the registry's index deep copies) are ignored;
	// only the declared content must converge.
	norm := func(sc *definition.Schema) *definition.Schema {
		r, _ := definition.FromJSON(sc.ToJSON())
		return r
	}
	if !reflect.DeepEqual(norm(dataOut).Fields, norm(regOut).Fields) {
		t.Fatalf("field maps differ:\ndata=%+v\nreg=%+v", norm(dataOut).Fields, norm(regOut).Fields)
	}
	if !reflect.DeepEqual(norm(dataOut).Schemas, norm(regOut).Schemas) {
		t.Fatalf("schema maps differ:\ndata=%+v\nreg=%+v", norm(dataOut).Schemas, norm(regOut).Schemas)
	}

	col, err := document.NewDocumentPool(s)
	if err != nil {
		t.Fatal(err)
	}
	docMeta := col.CompiledSchema().FieldsMeta
	if len(docMeta) < 2 || docMeta[0].Name != data.DocumentIDField || docMeta[1].Name != data.MetadataField {
		t.Fatalf("document path: system fields must lead, got %+v", docMeta)
	}

	// The linked schema's FieldsMeta is a flat list — nested metadata fields
	// appear with their own relative Path/Parts, indistinguishable from root
	// fields by name alone. Compare only fields whose ID is a root field of the
	// enriched schema (the metadata schema's IDs never appear at the root).
	want := make(map[string]string)
	for id, f := range dataOut.Fields {
		want[string(id)] = string(f.Name)
	}
	got := make(map[string]string)
	for _, fm := range col.CompiledSchema().FieldsMeta {
		if _, isRoot := want[fm.ID]; !isRoot {
			continue
		}
		got[fm.ID] = fm.Name
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("field id/name sets differ:\nwant=%v\ngot=%v", want, got)
	}
}
