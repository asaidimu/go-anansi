package schema_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFieldTypeToDataTypeMapping verifies every FieldType in the JSON schema
// maps to the container.DataType the container will actually use. This pins the
// classification so a change to classifyField surfaces as a test failure.
func TestFieldTypeToDataTypeMapping(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	expected := map[string]container.DataType{
		"f_unknown":        container.TypeUnknown,
		"f_string":         container.TypeString,
		"f_number":         container.TypeFloat,
		"f_integer":        container.TypeInt,
		"f_decimal":        container.TypeString, // canonical decimal string
		"f_boolean":        container.TypeBool,
		"f_bytes":          container.TypeBytes,
		"f_geometry":       container.TypeGeometry,
		"f_enum_str":       container.TypeString, // string-valued enum
		"f_enum_int":       container.TypeInt,    // numeric enum stores the ordinal
		"f_array_string":   container.TypeArrayString,
		"f_array_number":   container.TypeArrayFloat,
		"f_array_decimal":  container.TypeArrayString, // array of canonical decimal strings
		"f_array_integer":  container.TypeArrayInt,
		"f_array_boolean":  container.TypeArrayBool,
		"f_array_bytes":    container.TypeArrayBytes,
		"f_array_geometry": container.TypeArrayGeometry,
		"f_array_unknown":  container.TypeArrayUnknown,
		"f_array_object":   container.TypeArrayObject,
		"f_object":         container.TypeRecord,
		"f_record":         container.TypeArrayObject, // record uses container semantics: a set of records
		"f_union":          container.TypeRecord,
		"f_composite":      container.TypeRecord,
	}

	for name, want := range expected {
		assert.Equal(t, want, rootDataTypeByName(t, cs, name), "field %q", name)
	}
}

func TestFieldTypesParallelToDescriptors(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	require.Equal(t, len(cs.Descriptors), len(cs.FieldTypes))
	require.Equal(t, len(cs.Descriptors), len(cs.FieldsMeta))
	require.Equal(t, len(cs.Descriptors), len(cs.LocalOffsets))

	// The original FieldType must be preserved per descriptor.
	found := false
	for i, meta := range cs.FieldsMeta {
		if meta.Name == "f_decimal" && cs.Descriptors[i].SchemaIdx() == 0 {
			require.Equal(t, definition.FieldTypeDecimal, cs.FieldTypes[i])
			found = true
		}
	}
	assert.True(t, found, "f_decimal descriptor must preserve its FieldType")
}

func TestCompileRejectsUnknownNestedSchema(t *testing.T) {
	schema := `{
	  "version": "1.0.0",
	  "name": "bad_ref",
	  "fields": {
	    "f": { "name": "f", "type": "object", "schema": { "id": "missing" } }
	  }
	}`
	s, err := definition.FromJSON([]byte(schema))
	require.NoError(t, err)
	_, err = definition.Compile(s)
	assert.Error(t, err)
}
