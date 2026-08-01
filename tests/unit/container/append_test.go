package container_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppend_MultipleKeysGetDistinctSlots(t *testing.T) {
	doc := container.NewDataContainer()

	for i := int32(0); i < 5; i++ {
		k := key(t, container.TypeInt, 100+i)
		require.NoError(t, doc.AppendInt(k, int64(i)))
	}

	for i := int32(0); i < 5; i++ {
		k := key(t, container.TypeInt, 100+i)
		v, ok, err := doc.GetInt(k)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, int64(i), v)
	}
	assert.Equal(t, 5, doc.Length())
	assert.Equal(t, 5, len(intPositions(t, doc)))
}

func TestAppend_ThenSetUpdatesInPlace(t *testing.T) {
	doc := container.NewDataContainer()
	k := key(t, container.TypeInt, 1)

	require.NoError(t, doc.AppendInt(k, 1))
	require.NoError(t, doc.SetInt(k, 2))

	v, ok, err := doc.GetInt(k)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(2), v)
	assert.Equal(t, 1, len(intPositions(t, doc)), "append+set must not grow the slice")
}

func TestAppend_RejectsWrongType(t *testing.T) {
	doc := container.NewDataContainer()
	k := key(t, container.TypeString, 1)
	assert.ErrorIs(t, doc.AppendInt(k, 1), container.ErrTypeMismatch)
}

func TestAppend_ArrayString(t *testing.T) {
	doc := container.NewDataContainer()
	k1 := key(t, container.TypeArrayString, 1)
	k2 := key(t, container.TypeArrayString, 2)

	require.NoError(t, doc.AppendArrayString(k1, []string{"a", "b"}))
	require.NoError(t, doc.AppendArrayString(k2, []string{"c"}))

	v, ok, err := doc.GetArrayString(k1)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []string{"a", "b"}, v)

	v2, ok, err := doc.GetArrayString(k2)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []string{"c"}, v2)
}
