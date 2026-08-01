package container_test

import (
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClear_ResetsState(t *testing.T) {
	doc := container.NewDataContainer()

	k := key(t, container.TypeInt, 1)
	require.NoError(t, doc.SetInt(k, 5))
	doc.SetNull(key(t, container.TypeString, 2))

	assert.Equal(t, 2, doc.Length())

	doc.Clear()

	assert.Equal(t, 0, doc.Length())
	assert.False(t, doc.IsSet(k))
	_, ok, err := doc.GetInt(k)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestClear_DataContainerIsReusable(t *testing.T) {
	doc := container.NewDataContainer()

	k := key(t, container.TypeInt, 1)
	require.NoError(t, doc.SetInt(k, 1))

	doc.Clear()
	require.NoError(t, doc.SetInt(k, 2))

	v, ok, err := doc.GetInt(k)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(2), v)
	assert.Equal(t, 1, doc.Length())
}

func TestClear_ResetsSliceLengths(t *testing.T) {
	doc := container.NewDataContainer()
	require.NoError(t, doc.SetInt(key(t, container.TypeInt, 1), 1))
	require.NoError(t, doc.SetString(key(t, container.TypeString, 1), "a"))

	doc.Clear()

	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		assert.Len(t, *(*[]int64)(slot(container.TypeInt)), 0)
		assert.Len(t, *(*[]string)(slot(container.TypeString)), 0)
		assert.Len(t, positions, 0)
		return nil, nil
	})
	require.NoError(t, err)
}

func TestClear_EmptyDataContainer(t *testing.T) {
	doc := container.NewDataContainer()
	doc.Clear()
	assert.Equal(t, 0, doc.Length())
}
