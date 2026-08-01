package container_test

import (
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func key(t *testing.T, typ container.DataType, id int32) container.DataContainerKey {
	t.Helper()
	dp, err := container.NewDataPoint(typ, id)
	require.NoError(t, err)
	return container.NewDataContainerKey(dp, 0)
}

func TestDataContainer_SetGet_RoundTrip(t *testing.T) {
	doc := container.NewDataContainer()

	ki := key(t, container.TypeInt, 1)
	require.NoError(t, doc.SetInt(ki, 41))
	v, ok, err := doc.GetInt(ki)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(41), v)

	kf := key(t, container.TypeFloat, 2)
	require.NoError(t, doc.SetFloat(kf, 1.5))
	f, ok, err := doc.GetFloat(kf)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 1.5, f)

	ks := key(t, container.TypeString, 3)
	require.NoError(t, doc.SetString(ks, "hello"))
	s, ok, err := doc.GetString(ks)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "hello", s)

	kb := key(t, container.TypeBool, 4)
	require.NoError(t, doc.SetBool(kb, true))
	b, ok, err := doc.GetBool(kb)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, b)

	kx := key(t, container.TypeBytes, 5)
	require.NoError(t, doc.SetBytes(kx, []byte{1, 2, 3}))
	by, ok, err := doc.GetBytes(kx)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []byte{1, 2, 3}, by)
}

func TestDataContainer_SetOverwritesInPlace(t *testing.T) {
	doc := container.NewDataContainer()
	k := key(t, container.TypeInt, 1)

	require.NoError(t, doc.SetInt(k, 1))
	require.NoError(t, doc.SetInt(k, 2))
	require.NoError(t, doc.SetInt(k, 3))

	v, ok, err := doc.GetInt(k)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(3), v)
	assert.Equal(t, 1, doc.Length(), "overwriting a key must not add positions")
}

func TestDataContainer_GetMissing(t *testing.T) {
	doc := container.NewDataContainer()
	k := key(t, container.TypeInt, 1)
	_, ok, err := doc.GetInt(k)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestDataContainer_GetWrongType(t *testing.T) {
	doc := container.NewDataContainer()
	ki := key(t, container.TypeInt, 1)
	require.NoError(t, doc.SetInt(ki, 7))

	_, _, err := doc.GetString(ki)
	assert.ErrorIs(t, err, container.ErrTypeMismatch)
}

func TestDataContainer_SetWrongType(t *testing.T) {
	doc := container.NewDataContainer()
	k := key(t, container.TypeString, 1)
	assert.ErrorIs(t, doc.SetInt(k, 1), container.ErrTypeMismatch)
}

func TestDataContainer_SetNullAndHasValue(t *testing.T) {
	doc := container.NewDataContainer()
	k := key(t, container.TypeInt, 1)

	require.NoError(t, doc.SetInt(k, 5))
	doc.SetNull(k)

	assert.True(t, doc.IsSet(k))
	assert.True(t, doc.IsNull(k))
	assert.False(t, doc.HasValue(k))
	assert.Equal(t, 1, doc.Length())

	v, ok, err := doc.GetInt(k)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(0), v)
}

func TestDataContainer_Unset(t *testing.T) {
	doc := container.NewDataContainer()
	k := key(t, container.TypeInt, 1)

	require.NoError(t, doc.SetInt(k, 5))
	doc.Unset(k)

	assert.False(t, doc.IsSet(k))
	assert.Equal(t, 0, doc.Length())

	_, ok, err := doc.GetInt(k)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestDataContainer_ReusesFreedPosition(t *testing.T) {
	doc := container.NewDataContainer()

	k1 := key(t, container.TypeInt, 1)
	k2 := key(t, container.TypeInt, 2)

	require.NoError(t, doc.SetInt(k1, 1))
	require.NoError(t, doc.SetInt(k2, 2))
	doc.SetNull(k1)

	k3 := key(t, container.TypeInt, 3)
	require.NoError(t, doc.SetInt(k3, 3))

	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		ints := *(*[]int64)(slot(container.TypeInt))
		// Two values were ever live at once, so the backing slice never grows
		// past 2 — the freed slot from k1 is reused by k3.
		assert.LessOrEqual(t, len(ints), 2)
		return nil, nil
	})
	require.NoError(t, err)
}
