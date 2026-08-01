package schema_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompiledSchemaRoundTrip verifies that a compiled schema — including its
// defaults and enum side tables — survives Serialize/Deserialize losslessly.
func TestCompiledSchemaRoundTrip(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	data, err := definition.SerializeCompiledSchema(cs)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	got, err := definition.DeserializeCompiledSchema(data)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, cs.Descriptors, got.Descriptors)
	assert.Equal(t, cs.FieldTypes, got.FieldTypes)
	assert.Equal(t, cs.FieldsMeta, got.FieldsMeta)
	assert.Equal(t, cs.Schemas, got.Schemas)
	assert.Equal(t, cs.SchemasMeta, got.SchemasMeta)
	assert.Equal(t, cs.Variants, got.Variants)
	assert.Equal(t, cs.LocalOffsets, got.LocalOffsets)
	assert.Equal(t, cs.Indexes, got.Indexes)

	assert.Equal(t, snapshotDoc(t, cs.Defaults), snapshotDoc(t, got.Defaults))
	assert.Equal(t, snapshotDoc(t, cs.Enums), snapshotDoc(t, got.Enums))
}

func TestCompiledSchemaRoundTrip_Defaults(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)
	require.NotZero(t, cs.Defaults.Length(), "schema must have default values")

	data, err := definition.SerializeCompiledSchema(cs)
	require.NoError(t, err)
	got, err := definition.DeserializeCompiledSchema(data)
	require.NoError(t, err)

	snapshot := snapshotDoc(t, cs.Defaults)
	restored := snapshotDoc(t, got.Defaults)
	assert.Equal(t, snapshot, restored)
	assert.NotEmpty(t, snapshot)
}

func TestCompiledSchemaRoundTrip_Enums(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)
	require.NotZero(t, cs.Enums.Length(), "enum fields must populate the Enums table")

	data, err := definition.SerializeCompiledSchema(cs)
	require.NoError(t, err)
	got, err := definition.DeserializeCompiledSchema(data)
	require.NoError(t, err)

	assert.Equal(t, snapshotDoc(t, cs.Enums), snapshotDoc(t, got.Enums))
}

func TestCompiledSchemaRoundTrip_Stability(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	first, err := definition.SerializeCompiledSchema(cs)
	require.NoError(t, err)
	second, err := definition.SerializeCompiledSchema(cs)
	require.NoError(t, err)

	assert.Equal(t, first, second, "serialization must be deterministic")
}
