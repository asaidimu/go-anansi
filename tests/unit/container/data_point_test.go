package container_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataType_ValuesAreDistinct(t *testing.T) {
	types := []container.DataType{
		container.TypeUnknown,
		container.TypeInt,
		container.TypeFloat,
		container.TypeString,
		container.TypeBool,
		container.TypeBytes,
		container.TypeGeometry,
		container.TypeRecord,
		container.TypeArrayUnknown,
		container.TypeArrayInt,
		container.TypeArrayFloat,
		container.TypeArrayString,
		container.TypeArrayBool,
		container.TypeArrayBytes,
		container.TypeArrayObject,
		container.TypeArrayGeometry,
	}
	require.Len(t, types, 16)
	seen := make(map[container.DataType]bool)
	for _, typ := range types {
		seen[typ] = true
	}
	assert.Len(t, seen, 16, "all 16 DataType values must be distinct")
}

func TestNewDataPoint_RoundTrip(t *testing.T) {
	for _, typ := range []container.DataType{
		container.TypeInt,
		container.TypeFloat,
		container.TypeString,
		container.TypeBool,
		container.TypeBytes,
		container.TypeGeometry,
		container.TypeRecord,
		container.TypeArrayObject,
	} {
		dp, err := container.NewDataPoint(typ, 42)
		require.NoError(t, err)
		assert.Equal(t, typ, dp.Type())
		assert.Equal(t, int32(42), dp.ID())
	}
}

func TestNewDataPoint_NoIDIsZero(t *testing.T) {
	dp, err := container.NewDataPoint(container.TypeString)
	require.NoError(t, err)
	assert.Equal(t, int32(0), dp.ID())
	assert.Equal(t, container.TypeString, dp.Type())
}

func TestNewDataPoint_IDOutOfBounds(t *testing.T) {
	_, err := container.NewDataPoint(container.TypeInt, -1)
	assert.ErrorIs(t, err, container.ErrIDOutOfBounds)

	_, err = container.NewDataPoint(container.TypeInt, 1<<27)
	assert.ErrorIs(t, err, container.ErrIDOutOfBounds)

	// boundary values are accepted
	_, err = container.NewDataPoint(container.TypeInt, 0)
	assert.NoError(t, err)
	_, err = container.NewDataPoint(container.TypeInt, (1<<27)-1)
	assert.NoError(t, err)
}

func TestDataPoint_WithID_PreservesType(t *testing.T) {
	dp, err := container.NewDataPoint(container.TypeBool, 7)
	require.NoError(t, err)

	updated, err := dp.WithID(99)
	require.NoError(t, err)
	assert.Equal(t, container.TypeBool, updated.Type())
	assert.Equal(t, int32(99), updated.ID())

	_, err = dp.WithID(1 << 27)
	assert.ErrorIs(t, err, container.ErrIDOutOfBounds)
}

func TestDataPoint_IsNull(t *testing.T) {
	dp, err := container.NewDataPoint(container.TypeInt, 3)
	require.NoError(t, err)
	assert.False(t, dp.IsNull())
}

func TestDataContainerKey_RoundTrip(t *testing.T) {
	dp, err := container.NewDataPoint(container.TypeString, 5)
	require.NoError(t, err)

	const descriptor = uint32(0xABCD1234)
	key := container.NewDataContainerKey(dp, descriptor)

	assert.Equal(t, dp, key.DataPoint())
	assert.Equal(t, descriptor, key.Descriptor())
	assert.Equal(t, container.TypeString, key.Type())
	assert.False(t, key.IsNull())
}
