package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSerializeJSON_MatchesDumpMarshal verifies the direct serializer produces
// byte-identical JSON to the materialized-map path (json.Marshal(Dump(...)))
// for the demo document and every ERP order size. This pins the two paths to
// the same output shape, float formatting, key ordering, and escaping.
func TestSerializeJSON_MatchesDumpMarshal(t *testing.T) {
	t.Run("demo", func(t *testing.T) {
		cs, err := compileSchema([]byte(schemaJSON))
		require.NoError(t, err)

		doc, err := DecodeJSON(cs, []byte(documentJSON))
		require.NoError(t, err)

		m, err := Dump(cs, doc)
		require.NoError(t, err)
		want, err := json.Marshal(m)
		require.NoError(t, err)
		got, err := SerializeJSON(cs, doc)
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got))
	})

	cs, err := compileSchema([]byte(erpSchemaJSON))
	require.NoError(t, err)

	for name, n := range erpLineCounts {
		t.Run("erp_"+name, func(t *testing.T) {
			doc, err := DecodeJSON(cs, buildERPOrder("ORD-2026-000042", n))
			require.NoError(t, err)

			m, err := Dump(cs, doc)
			require.NoError(t, err)
			want, err := json.Marshal(m)
			require.NoError(t, err)
			got, err := SerializeJSON(cs, doc)
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))
		})
	}
}

// TestSerializeJSON_EmptyObject checks the serializer on a container with only
// defaulted fields and on an empty document.
func TestSerializeJSON_EmptyObject(t *testing.T) {
	cs, err := compileSchema([]byte(schemaJSON))
	require.NoError(t, err)

	doc, err := DecodeJSON(cs, []byte(`{}`))
	require.NoError(t, err)

	m, err := Dump(cs, doc)
	require.NoError(t, err)
	want, err := json.Marshal(m)
	require.NoError(t, err)
	got, err := SerializeJSON(cs, doc)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}
