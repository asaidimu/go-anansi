package schemagen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/asaidimu/go-anansi/v8/codegen/typescript"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/require"
)

func TestGenerateCombined(t *testing.T) {
	s1 := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "Users",
			Fields: map[definition.FieldId]definition.Field{
				"019d7775-6563-7c55-a6f3-ac8f087d89d1": {Name: "name", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}
	s2 := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "Products",
			Fields: map[definition.FieldId]definition.Field{
				"019d7775-6563-7605-8bfb-c27365b73581": {Name: "price", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeDecimal}},
			},
		},
	}

	ts := typescript.GenerateCombined([]*definition.Schema{s1, s2})
	require.Contains(t, ts, "export interface Users")
	require.Contains(t, ts, "name: string")
	require.Contains(t, ts, "export interface Products")
	require.Contains(t, ts, "price?: number")
}

func TestGenerateCombined_WithNestedSchemas(t *testing.T) {
	s := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "Orders",
			Fields: map[definition.FieldId]definition.Field{
				"019d7775-6563-7c55-a6f3-ac8f087d89d1": {Name: "id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			"019d7775-6565-7c1f-a6f7-aeb5c4eb35e4": {
				BaseSchema: definition.BaseSchema{
					Name: "Item",
					Fields: map[definition.FieldId]definition.Field{
						"019d7775-6565-7545-9e25-fc471f57bcfb": {Name: "sku", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
					},
				},
			},
		},
	}

	ts := typescript.GenerateCombined([]*definition.Schema{s})
	require.Contains(t, ts, "export interface Item")
	require.Contains(t, ts, "sku?: string")
	require.Contains(t, ts, "export interface Orders")
	require.Contains(t, ts, "id: string")
}

func TestGenerateCombined_NestedSchemaRefs(t *testing.T) {
	schemaID := definition.SchemaId("019d7775-6565-7c1f-a6f7-aeb5c4eb35e4")
	s := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "Orders",
			Fields: map[definition.FieldId]definition.Field{
				"019d7775-6563-7c55-a6f3-ac8f087d89d1": {
					Name:     "item",
					Required: true,
					FieldProperties: definition.FieldProperties{
						Type:   definition.FieldTypeObject,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: schemaID}),
					},
				},
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			schemaID: {
				BaseSchema: definition.BaseSchema{
					Name: "Item",
					Fields: map[definition.FieldId]definition.Field{
						"019d7775-6565-7545-9e25-fc471f57bcfb": {Name: "sku", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
					},
				},
			},
		},
	}

	ts := typescript.GenerateCombined([]*definition.Schema{s})
	require.Contains(t, ts, "item: Item")
}

func TestRunTSGen(t *testing.T) {
	// dir := t.TempDir()
	dir := os.TempDir()
	s1 := `{"name":"A","version":"1.0.0","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"x","type":"string"}}}`
	s2 := `{"name":"B","version":"1.0.0","fields":{"019d7775-6563-7605-8bfb-c27365b73581":{"name":"y","type":"integer"}}}`

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.schema.json"), []byte(s1), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.schema.json"), []byte(s2), 0644))

	out := filepath.Join(dir, "types.ts")
	err := RunTSGen(&Config{
		Schema: SchemaConfig{
			Glob: filepath.Join(dir, "*.schema.json"),
		},
		TSGen: TSGenConfig{
			Out: out,
		},
	}, false)
	require.NoError(t, err)
	require.FileExists(t, out)

	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	content := string(raw)
	require.Contains(t, content, "export interface A")
	require.Contains(t, content, "x?: string")
	require.Contains(t, content, "export interface B")
	require.Contains(t, content, "y?: number")
}
