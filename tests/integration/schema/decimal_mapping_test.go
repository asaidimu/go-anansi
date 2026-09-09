package schema_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/types/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecimal_IsValidAcceptsStrings(t *testing.T) {
	assert.True(t, decimal.IsValid("0"))
	assert.True(t, decimal.IsValid("123.45"))
	assert.True(t, decimal.IsValid("-42.001"))
	assert.True(t, decimal.IsValid("0.0001"))
}

func TestDecimal_IsValidRejectsNonStrings(t *testing.T) {
	assert.False(t, decimal.IsValid(123.45))
	assert.False(t, decimal.IsValid(int64(42)))
	assert.False(t, decimal.IsValid(nil))
	assert.False(t, decimal.IsValid("not-a-number"))
	assert.False(t, decimal.IsValid("1.2.3"))
}

// TestDecimalStoredAsString pins the invariant: a decimal field is stored in
// the container as a TypeString value, and an array<decimal> as TypeArrayString.
func TestDecimalStoredAsString(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	assert.Equal(t, container.TypeString, rootDataTypeByName(t, cs, "f_decimal"))
	assert.Equal(t, container.TypeArrayString, rootDataTypeByName(t, cs, "f_array_decimal"))
}

// TestDecimalDefaultRoundTrips verifies a decimal default survives compile
// (stored as string) and the compiled-schema codec round trip.
func TestDecimalDefaultRoundTrips(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	require.NotZero(t, cs.Defaults.Length())
	snapshot := snapshotDoc(t, cs.Defaults)

	data, err := definition.SerializeCompiledSchema(cs)
	require.NoError(t, err)
	got, err := definition.DeserializeCompiledSchema(data)
	require.NoError(t, err)

	assert.Equal(t, snapshot, snapshotDoc(t, got.Defaults))
}
