package document

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
)

type ScratchModel struct {
	DocumentModel
	Name string `anansi:"name"`
}

func TestScratchPipeline(t *testing.T) {
	schemaBytes, err := data.ExtractDTOSchemaDirect(ScratchModel{})
	if err != nil {
		t.Fatalf("ExtractDTOSchemaDirect: %v", err)
	}
	t.Logf("schema: %s", schemaBytes)

	col, err := NewDocumentPoolFromJSON(schemaBytes)
	if err != nil {
		t.Fatalf("NewDocumentPoolFromJSON: %v", err)
	}

	m := ScratchModel{Name: "widget"}
	doc, err := col.FromStruct(&m)
	if err != nil {
		t.Fatalf("FromStruct: %v", err)
	}
	if doc.ID() == "" {
		t.Fatalf("no id")
	}
	if doc.Len() == 0 {
		t.Fatalf("no data")
	}
}
