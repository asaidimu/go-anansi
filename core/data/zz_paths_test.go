package data_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/document"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

func TestDebugPaths(t *testing.T) {
	s, _ := definition.FromJSON([]byte(convergenceSchemaJSON))
	col, err := document.NewDocumentPool(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, fm := range col.CompiledSchema().FieldsMeta {
		t.Logf("id=%s name=%s path=%q parts=%v", fm.ID, fm.Name, fm.Path, fm.Parts)
	}
	_, _ = data.GetMetadataSchema()
}
