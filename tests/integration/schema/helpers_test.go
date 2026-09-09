package schema_test

import (
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/require"
)

// allTypesSchema exercises every FieldType and its DataType mapping. Field and
// schema ids are readable strings (no UUIDs required for compilation).
const allTypesSchema = `{
  "version": "1.0.0",
  "name": "all_types",
  "metadata": { "owner": "schema-tests", "team": ["core"] },
  "fields": {
    "f_unknown":       { "name": "f_unknown",       "type": "unknown" },
    "f_string":        { "name": "f_string",        "type": "string",  "default": "hello", "metadata": { "ui": "input", "tags": ["a", "b"] } },
    "f_number":        { "name": "f_number",        "type": "number",  "default": 1.5 },
    "f_integer":       { "name": "f_integer",       "type": "integer", "default": 42 },
    "f_decimal":       { "name": "f_decimal",       "type": "decimal", "default": "123.45" },
    "f_boolean":       { "name": "f_boolean",       "type": "boolean", "default": true },
    "f_bytes":         { "name": "f_bytes",         "type": "bytes" },
    "f_geometry":      { "name": "f_geometry",      "type": "geometry" },
    "f_enum_str":      { "name": "f_enum_str",      "type": "enum",    "schema": { "type": "string",  "values": ["a", "b"] } },
    "f_enum_int":      { "name": "f_enum_int",      "type": "enum",    "schema": { "type": "integer", "values": [1, 2] } },
    "f_array_string":  { "name": "f_array_string",  "type": "array",   "schema": { "type": "string" } },
    "f_array_number":  { "name": "f_array_number",  "type": "array",   "schema": { "type": "number" } },
    "f_array_decimal": { "name": "f_array_decimal", "type": "array",   "schema": { "type": "decimal" } },
    "f_array_integer": { "name": "f_array_integer", "type": "array",   "schema": { "type": "integer" } },
    "f_array_boolean": { "name": "f_array_boolean", "type": "array",   "schema": { "type": "boolean" } },
    "f_array_bytes":   { "name": "f_array_bytes",   "type": "array",   "schema": { "type": "bytes" } },
    "f_array_geometry":{ "name": "f_array_geometry","type": "array",   "schema": { "type": "geometry" } },
    "f_array_unknown": { "name": "f_array_unknown", "type": "array",   "schema": { "type": "unknown" } },
    "f_array_object":  { "name": "f_array_object",  "type": "array",   "schema": { "id": "obj_schema" } },
    "f_object":        { "name": "f_object",        "type": "object",  "schema": { "id": "obj_schema" } },
    "f_record":        { "name": "f_record",        "type": "record",  "schema": { "id": "obj_schema" } },
    "f_union":         { "name": "f_union",         "type": "union",   "schema": [ { "id": "obj_a" }, { "id": "obj_b" } ] },
    "f_composite":     { "name": "f_composite",     "type": "composite","schema": [ { "id": "obj_a" }, { "id": "obj_b" } ] }
  },
  "schemas": {
    "obj_schema": {
      "name": "obj_schema",
      "metadata": { "purpose": "embedded" },
      "fields": {
        "nested_str": { "name": "nested_str", "type": "string" },
        "nested_int": { "name": "nested_int", "type": "integer" }
      }
    },
    "obj_a": {
      "name": "obj_a",
      "fields": {
        "a_name": { "name": "a_name", "type": "string" }
      }
    },
    "obj_b": {
      "name": "obj_b",
      "fields": {
        "b_name": { "name": "b_name", "type": "string" }
      }
    }
  }
}`

// compileSchema parses a JSON schema, compiles it, and links it.
func compileSchema(t *testing.T, jsonSchema string) *definition.CompiledSchema {
	t.Helper()
	s, err := definition.FromJSON([]byte(jsonSchema))
	require.NoError(t, err)
	rs, err := definition.Compile(s)
	require.NoError(t, err)
	cs, err := definition.Link(rs)
	require.NoError(t, err)
	return cs
}

// rootDataTypeByName returns the container.DataType of a root-level field.
func rootDataTypeByName(t *testing.T, cs *definition.CompiledSchema, name string) container.DataType {
	t.Helper()
	for i, meta := range cs.FieldsMeta {
		if meta.Name != name {
			continue
		}
		if cs.Descriptors[i].SchemaIdx() != 0 {
			continue
		}
		return cs.Descriptors[i].DataType()
	}
	t.Fatalf("root field %q not found in compiled schema", name)
	return 0
}

// snapshotDoc flattens a document's positions and values into a comparable map.
func snapshotDoc(t *testing.T, doc *container.DataContainer) map[int64]any {
	t.Helper()
	out := make(map[int64]any, doc.Length())
	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		for k, idx := range positions {
			dk := container.DataContainerKey(k)
			if idx < 0 {
				out[k] = nil
				continue
			}
			switch dk.Type() {
			case container.TypeInt:
				out[k] = (*(*[]int64)(slot(container.TypeInt)))[idx]
			case container.TypeFloat:
				out[k] = (*(*[]float64)(slot(container.TypeFloat)))[idx]
			case container.TypeString:
				out[k] = (*(*[]string)(slot(container.TypeString)))[idx]
			case container.TypeBool:
				out[k] = (*(*[]bool)(slot(container.TypeBool)))[idx]
			case container.TypeBytes:
				out[k] = (*(*[][]byte)(slot(container.TypeBytes)))[idx]
			case container.TypeGeometry:
				out[k] = (*(*[][][]float64)(slot(container.TypeGeometry)))[idx]
			case container.TypeRecord:
				out[k] = (*(*[]map[string]any)(slot(container.TypeRecord)))[idx]
			case container.TypeArrayUnknown:
				out[k] = (*(*[][]any)(slot(container.TypeArrayUnknown)))[idx]
			case container.TypeArrayInt:
				out[k] = (*(*[][]int64)(slot(container.TypeArrayInt)))[idx]
			case container.TypeArrayFloat:
				out[k] = (*(*[][]float64)(slot(container.TypeArrayFloat)))[idx]
			case container.TypeArrayString:
				out[k] = (*(*[][]string)(slot(container.TypeArrayString)))[idx]
			case container.TypeArrayBool:
				out[k] = (*(*[][]bool)(slot(container.TypeArrayBool)))[idx]
			case container.TypeArrayBytes:
				out[k] = (*(*[][][]byte)(slot(container.TypeArrayBytes)))[idx]
			case container.TypeArrayObject:
				group := (*(*[][]*container.DataContainer)(slot(container.TypeArrayObject)))[idx]
				vals := make([]any, len(group))
				for i, child := range group {
					vals[i] = snapshotDoc(t, child)
				}
				out[k] = vals
			case container.TypeArrayGeometry:
				out[k] = (*(*[][][][]float64)(slot(container.TypeArrayGeometry)))[idx]
			case container.TypeUnknown:
				out[k] = (*(*[]any)(slot(container.TypeUnknown)))[idx]
			}
		}
		return nil, nil
	})
	require.NoError(t, err)
	return out
}
