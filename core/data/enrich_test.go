package data

import (
	"reflect"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const enrichTestSchemaJSON = `{
  "version": "1.0.0",
  "name": "enrichtest",
  "fields": {
    "name": { "name": "name", "type": "string", "required": true },
    "age":  { "name": "age",  "type": "integer" }
  }
}`

func enrich(s *definition.Schema) (*definition.Schema, error) {
	meta, deps := GetMetadataSchema()
	return EnrichSchema(s, meta, deps)
}

func TestEnrichSchema_SystemFieldsFirst(t *testing.T) {
	s, err := definition.FromJSON([]byte(enrichTestSchemaJSON))
	if err != nil {
		t.Fatal(err)
	}
	out, err := enrich(s)
	if err != nil {
		t.Fatal(err)
	}

	f, ok := out.Fields[definition.FieldId(SystemFieldIDDocumentID)]
	if !ok || string(f.Name) != DocumentIDField {
		t.Fatalf("expected _id_ field, got %+v", f)
	}
	m, ok := out.Fields[definition.FieldId(SystemFieldIDMetadata)]
	if !ok || string(m.Name) != MetadataField {
		t.Fatalf("expected _metadata_ field, got %+v", m)
	}
	if _, ok := out.Schemas[definition.SchemaId(SystemSchemaIDMetadata)]; !ok {
		t.Fatalf("expected metadata schema %q", SystemSchemaIDMetadata)
	}

	// The compiled schema orders fields lexicographically by ID: the system
	// fields must occupy the first two slots.
	compiled, err := definition.Compile(out)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := definition.Link(compiled)
	if err != nil {
		t.Fatal(err)
	}
	meta := linked.FieldsMeta
	if len(meta) < 2 || meta[0].Name != DocumentIDField || meta[1].Name != MetadataField {
		t.Fatalf("system fields must be first, got %+v", meta)
	}
}

func TestEnrichSchema_Idempotent(t *testing.T) {
	once, _ := definition.FromJSON([]byte(enrichTestSchemaJSON))
	o1, err := enrich(once)
	if err != nil {
		t.Fatal(err)
	}

	twice, _ := definition.FromJSON([]byte(enrichTestSchemaJSON))
	if _, err := enrich(twice); err != nil {
		t.Fatal(err)
	}
	o2, err := enrich(twice)
	if err != nil {
		t.Fatal(err)
	}

	r1, _ := definition.FromJSON(o1.ToJSON())
	r2, _ := definition.FromJSON(o2.ToJSON())
	if !reflect.DeepEqual(r1, r2) {
		t.Fatalf("content differs:\n1=%s\n2=%s", r1.ToJSON(), r2.ToJSON())
	}
}

func TestEnrichSchema_DoesNotMutateInput(t *testing.T) {
	s, _ := definition.FromJSON([]byte(enrichTestSchemaJSON))
	beforeFields := len(s.Fields)
	beforeSchemas := len(s.Schemas)
	if _, err := enrich(s); err != nil {
		t.Fatal(err)
	}
	if len(s.Fields) != beforeFields {
		t.Errorf("input schema mutated: fields %d -> %d", beforeFields, len(s.Fields))
	}
	if len(s.Schemas) != beforeSchemas {
		t.Errorf("input schema mutated: schemas %d -> %d", beforeSchemas, len(s.Schemas))
	}
}

func TestEnrichSchema_GuardRejectsFieldBeforeSystemIDs(t *testing.T) {
	s, _ := definition.FromJSON([]byte(enrichTestSchemaJSON))
	// A user field whose ID sorts before the static system IDs would shift every
	// storage address — enrichment must fail loudly.
	s.Fields[definition.FieldId("0")] = definition.Field{
		Name: definition.FieldName("early"),
		FieldProperties: definition.FieldProperties{
			Type: definition.FieldTypeString,
		},
	}
	if _, err := enrich(s); err == nil {
		t.Fatalf("expected error for field ID sorting before system IDs")
	}
}

func TestEnrichSchema_Nil(t *testing.T) {
	if out, err := enrich(nil); out != nil || err != nil {
		t.Fatalf("expected (nil, nil), got (%v, %v)", out, err)
	}
}

func TestEnrichSchema_PartialSystemFields(t *testing.T) {
	s, err := definition.FromJSON([]byte(enrichTestSchemaJSON))
	if err != nil {
		t.Fatal(err)
	}

	// Default: _id_ is required, uniqueness preserved.
	strict, err := enrich(s)
	if err != nil {
		t.Fatal(err)
	}
	strictID := strict.Fields[definition.FieldId(SystemFieldIDDocumentID)]
	if !strictID.Required {
		t.Fatalf("default enrichment must keep _id_ required, got %+v", strictID)
	}
	if !strictID.Unique {
		t.Fatalf("default enrichment must keep _id_ unique, got %+v", strictID)
	}

	// Partial: _id_ is optional but still unique and still first.
	meta, deps := GetMetadataSchema()
	partial, err := EnrichSchema(s, meta, deps, WithPartialSystemFields())
	if err != nil {
		t.Fatal(err)
	}
	partialID := partial.Fields[definition.FieldId(SystemFieldIDDocumentID)]
	if partialID.Required {
		t.Fatalf("partial enrichment must drop _id_ required, got %+v", partialID)
	}
	if !partialID.Unique {
		t.Fatalf("partial enrichment must keep _id_ unique, got %+v", partialID)
	}
}
