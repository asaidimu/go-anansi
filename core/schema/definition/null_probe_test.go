package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNullProbe(t *testing.T) {
	src := `{
		"name": "labels",
		"version": "1.0.0",
		"fields": {
			"f1": {"name": "nickname", "type": "string", "default": null}
		},
		"schemas": {},
		"indexes": {}
	}`
	s, err := definition.FromJSON([]byte(src))
	require.NoError(t, err)

	f := s.Fields["f1"]
	t.Logf("default: isZero=%v isNull=%v", f.Default.IsZero(), f.Default.IsNull())
	assert.True(t, f.Default.IsNull(), "default must decode as explicit null")

	out := s.ToJSON()
	t.Logf("ToJSON: %s", out)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	fields := m["fields"].(map[string]any)
	_, hasDefault := fields["f1"].(map[string]any)["default"]
	t.Logf("ToJSON emitted 'default' key: %v", hasDefault)
}

func TestNullProbe_Metadata(t *testing.T) {
	src := `{
		"name": "labels",
		"version": "1.0.0",
		"metadata": {"note": null},
		"fields": {
			"f1": {"name": "nickname", "type": "string", "metadata": {"x": null}}
		},
		"schemas": {},
		"indexes": {}
	}`
	s, err := definition.FromJSON([]byte(src))
	require.NoError(t, err)

	v, ok := s.Metadata["note"]
	require.True(t, ok)
	assert.Nil(t, v)
	fv, ok := s.Fields["f1"].Metadata["x"]
	require.True(t, ok)
	assert.Nil(t, fv)

	out := s.ToJSON()
	t.Logf("ToJSON: %s", out)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	note, ok := m["metadata"].(map[string]any)["note"]
	require.True(t, ok, "schema-level metadata note must survive ToJSON")
	assert.Nil(t, note)
	x, ok := m["fields"].(map[string]any)["f1"].(map[string]any)["metadata"].(map[string]any)["x"]
	require.True(t, ok, "field-level metadata x must survive ToJSON")
	assert.Nil(t, x)

	asMap := s.AsMap()
	_, ok = asMap["metadata"].(map[string]any)["note"]
	t.Logf("AsMap schema metadata note present: %v", ok)
	_, ok = asMap["fields"].(map[string]any)["f1"].(map[string]any)["metadata"].(map[string]any)["x"]
	t.Logf("AsMap field metadata x present: %v", ok)
}

func TestNullProbe_CompileLinkSerialize(t *testing.T) {
	src := `{
		"name": "labels",
		"version": "1.0.0",
		"fields": {
			"f1": {"name": "nickname", "type": "string", "default": null, "nullable": true}
		},
		"schemas": {},
		"indexes": {}
	}`
	s, err := definition.FromJSON([]byte(src))
	require.NoError(t, err)
	rs, err := definition.Compile(s)
	require.NoError(t, err)
	cs, err := definition.Link(rs)
	require.NoError(t, err)
	for i, fm := range cs.FieldsMeta {
		if fm.Name == "nickname" {
			t.Logf("descriptor HasDefault=%v FieldsMeta.Default isNull=%v isZero=%v",
				cs.Descriptors[i].HasDefault(), cs.FieldsMeta[i].Default.IsNull(), cs.FieldsMeta[i].Default.IsZero())
			t.Logf("Defaults container length=%d", cs.Defaults.Length())
		}
	}
	data, err := definition.SerializeCompiledSchema(cs)
	require.NoError(t, err)
	got, err := definition.DeserializeCompiledSchema(data)
	require.NoError(t, err)
	require.NotNil(t, got)
}
