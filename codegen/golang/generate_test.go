package golang

import (
	"flag"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update golden files")

const testSchema = `{
  "name": "Order",
  "fields": {
    "f1": {"name": "id", "type": "string", "required": true},
    "f2": {"name": "total", "type": "decimal", "required": false},
    "f3": {"name": "quantity", "type": "integer", "nullable": true},
    "f4": {"name": "status", "type": "enum", "schema": {"type": "string", "values": ["open", "closed"]}},
    "f5": {"name": "shipping", "type": "object", "schema": {"id": "addr"}},
    "f6": {"name": "tags", "type": "array", "schema": {"type": "string"}},
    "f7": {"name": "meta", "type": "record"},
    "f8": {"name": "payload", "type": "union", "schema": [{"id": "cat_a"}, {"id": "cat_b"}]},
    "f9": {"name": "combo", "type": "composite", "schema": [{"id": "cat_a"}, {"id": "cat_b"}]}
  },
  "schemas": {
    "addr": {"name": "Address", "fields": {"s1": {"name": "street", "type": "string"}}},
    "cat_a": {"name": "CategoryA", "fields": {"x1": {"name": "code", "type": "string"}}},
    "cat_b": {"name": "CategoryB", "fields": {"x2": {"name": "label", "type": "string"}}},
    "color": {"name": "Color", "type": "string", "values": ["red", "green", "blue"]},
    "size_list": {"name": "SizeList", "type": "array", "schema": {"id": "addr"}}
  },
  "metadata": {
    "projections": {
      "OrderSummary": { "fields": { "include": ["total", "status"] } },
      "OrderCreate": { "fields": { "include": ["total"], "required": ["total"], "tags": { "total": { "input": "arguments.{name}", "schema": "{type}:{required}" } } } },
      "OrderUpdate": { "fields": { "exclude": ["total"], "optional": ["status"] } }
    }
  }
}`

func generate(t *testing.T, config *GeneratorConfig) string {
	t.Helper()
	gen := NewGoGenerator(config)
	out, err := gen.Generate([]byte(testSchema))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return out
}

// goldenCase runs a golden comparison for a specific mode against testdata/<name>.golden.
// Use -update to (re)generate the golden files.
func goldenCase(t *testing.T, name string, mode GenerationMode) {
	t.Helper()
	out := generate(t, &GeneratorConfig{Mode: mode})

	// The generated source must always be syntactically valid Go. Prepending
	// a package declaration is all that's needed for a parser check, since
	// type references are not resolved at parse time.
	src := "package test\n\n" + out
	if _, err := parser.ParseFile(token.NewFileSet(), "", src, parser.AllErrors); err != nil {
		t.Errorf("generated output for mode %s is not valid Go: %v\n%s", mode, err, out)
	}

	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(out), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to generate)", path, err)
	}
	if string(want) != out {
		t.Errorf("output mismatch for mode %s\n--- want (testdata/%s.golden) ---\n%s\n--- got ---\n%s", mode, name, want, out)
	}
}

func TestGenerate_ModeFull(t *testing.T) {
	goldenCase(t, "full", ModeFull)
}

func TestGenerate_ModeModel(t *testing.T) {
	goldenCase(t, "model", ModeModel)
}

func TestGenerate_ModeStructs(t *testing.T) {
	goldenCase(t, "structs", ModeStructs)
}

func TestGenerate_ZeroModeDefaultsToFull(t *testing.T) {
	out := generate(t, nil)
	if !strings.Contains(out, "data.DocumentModel") {
		t.Error("zero-mode generation should include the DocumentModel embed")
	}
	if !strings.Contains(out, "func InitOrdersModel") {
		t.Error("zero-mode generation should include the collection scaffold")
	}
}

func TestGenerate_ModeLayers(t *testing.T) {
	structs := generate(t, &GeneratorConfig{Mode: ModeStructs})
	model := generate(t, &GeneratorConfig{Mode: ModeModel})
	full := generate(t, &GeneratorConfig{Mode: ModeFull})

	// structs: no model, no collection, no data import.
	if strings.Contains(structs, "DocumentModel") {
		t.Error("structs mode must not embed DocumentModel")
	}
	if strings.Contains(structs, "ModelCollection") {
		t.Error("structs mode must not emit a collection wrapper")
	}
	if strings.Contains(structs, `"github.com/asaidimu/go-anansi/v8/core/data"`) {
		t.Error("structs mode must not import core/data")
	}
	// decimal field is still present (it is a plain field type), so the
	// decimal import must be preserved.
	if !strings.Contains(structs, `"github.com/asaidimu/go-anansi/v8/core/types/decimal"`) {
		t.Error("structs mode must import decimal when a decimal field exists")
	}
	// Root struct is emitted like any other struct.
	if !strings.Contains(structs, "type Order struct") {
		t.Error("structs mode must emit the root struct as a plain struct")
	}

	// model: no collection.
	if !strings.Contains(model, "data.DocumentModel") {
		t.Error("model mode must embed DocumentModel")
	}
	if strings.Contains(model, "ModelCollection") {
		t.Error("model mode must not emit a collection wrapper")
	}

	// full: everything.
	if !strings.Contains(full, "data.DocumentModel") || !strings.Contains(full, "ModelCollection") {
		t.Error("full mode must emit the model and collection layers")
	}
}

func TestGenerate_Deterministic(t *testing.T) {
	a := generate(t, &GeneratorConfig{Mode: ModeFull})
	b := generate(t, &GeneratorConfig{Mode: ModeFull})
	if a != b {
		t.Error("generation is not deterministic across runs")
	}
}

// schemaWithProjections builds a minimal schema whose metadata declares a
// single projection with the given fields DSL body.
func schemaWithProjections(projName, fieldsBody string) string {
	return `{
  "name": "Order",
  "fields": {"f1": {"name": "total", "type": "decimal"}, "f2": {"name": "status", "type": "string"}},
  "metadata": {"projections": {"` + projName + `": {"fields": ` + fieldsBody + `}}}
}`
}

func TestProjections_ValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		fields  string
		wantErr string
	}{
		{"unknown field", `{"include": ["nope"]}`, "unknown field"},
		{"include and exclude", `{"include": ["total"], "exclude": ["total"]}`, "both included and excluded"},
		{"required and optional", `{"required": ["total"], "optional": ["total"]}`, "both required and optional"},
		{"required excluded", `{"exclude": ["total"], "required": ["total"]}`, "required field"},
		{"required not included", `{"include": ["status"], "required": ["total"]}`, "not part of the projection"},
		{"optional not included", `{"include": ["total"], "optional": ["status"]}`, "not part of the projection"},
		{"fields not an object", `["total"]`, "missing 'fields'"},
		{"include not an array", `{"include": "total"}`, "must be an array"},
		{"tags reference missing field", `{"include": ["total"], "tags": {"status": {"input": "x"}}}`, "not part of the projection"},
		{"tags value not a string", `{"include": ["total"], "tags": {"total": {"input": 1}}}`, "must be a string"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewGoGenerator(nil).Generate([]byte(schemaWithProjections("Proj", c.fields)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
		})
	}

	t.Run("projection conflicts with root type", func(t *testing.T) {
		_, err := NewGoGenerator(nil).Generate([]byte(schemaWithProjections("Order", `{"include": ["total"]}`)))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with the root model type")
	})
}

func TestProjections_EmitRequiredOverride(t *testing.T) {
	gen := NewGoGenerator(&GeneratorConfig{TagConfig: DefaultTagConfig()})
	out, err := gen.Generate([]byte(testSchema))
	require.NoError(t, err)

	// OrderCreate forces total required: value type + required=true tag.
	assert.Contains(t, out, "type OrderCreate struct {\n    data.DocumentModel\n    Total decimal.Decimal `anansi:\"total,required=true\" json:\"total\" input:\"arguments.total\" schema:\"decimal:true\"`\n}")
	assert.Contains(t, out, `anansi:"total,required=true"`)

	// OrderSummary inherits optional: pointer type + required=false tag.
	assert.Contains(t, out, "type OrderSummary struct {\n    data.DocumentModel")
	assert.Contains(t, out, "Total *decimal.Decimal `anansi:\"total,required=false\" json:\"total,omitempty\"`")
	assert.Contains(t, out, `anansi:"total,required=false"`)

	// No projection accessors on the collection wrapper — projections are
	// consumed via the generic shape methods (ReadAs/CreateFrom/UpdateFrom).
	assert.NotContains(t, out, "func (o *Orders) FindOrderSummaryByID")
	assert.NotContains(t, out, "func (o *Orders) ReadOrderSummary")
	assert.NotContains(t, out, "func (o *Orders) CreateOrderSummary")
	assert.NotContains(t, out, "func (o *Orders) UpdateOrderSummary")

	// Structs mode must not emit projections (no DocumentModel layer).
	structs := generate(t, &GeneratorConfig{Mode: ModeStructs})
	assert.NotContains(t, structs, "OrderSummary")
}

func TestProjections_Tags(t *testing.T) {
	gen := NewGoGenerator(&GeneratorConfig{TagConfig: DefaultTagConfig()})
	out, err := gen.Generate([]byte(testSchema))
	require.NoError(t, err)

	// {name}, {type} and {required} placeholders expand from the field.
	assert.Contains(t, out, `Total decimal.Decimal `+"`anansi:\"total,required=true\" json:\"total\" input:\"arguments.total\" schema:\"decimal:true\"`")

	t.Run("unknown property fails fast", func(t *testing.T) {
		schema := schemaWithProjections("Proj", `{"include": ["total"], "tags": {"total": {"input": "arguments.{frobnicate}"}}}`)
		_, err := NewGoGenerator(nil).Generate([]byte(schema))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field property")
	})
}


func TestParseGenerationMode(t *testing.T) {
	cases := []struct {
		in      string
		want    GenerationMode
		wantErr bool
	}{
		{"full", ModeFull, false},
		{"collection", ModeFull, false},
		{"model", ModeStructs | ModeModel, false},
		{"structs", ModeStructs, false},
		{"structs,collection", ModeFull, false},
		{"collection,structs", ModeFull, false},
		{"model,collection", ModeFull, false},
		{" FULL ", ModeFull, false},
		{"", 0, true},
		{"bogus", 0, true},
		{"structs,bogus", 0, true},
	}
	for _, c := range cases {
		got, err := ParseGenerationMode(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseGenerationMode(%q): expected error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseGenerationMode(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseGenerationMode(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestGenerationMode_String(t *testing.T) {
	cases := []struct {
		mode GenerationMode
		want string
	}{
		{ModeStructs, "structs"},
		{ModeStructs | ModeModel, "model"},
		{ModeFull, "full"},
		{0, ""},
	}
	for _, c := range cases {
		if got := c.mode.String(); got != c.want {
			t.Errorf("GenerationMode(%d).String() = %q, want %q", c.mode, got, c.want)
		}
	}
}
