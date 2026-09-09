package container_test

import (
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalk_ExposesPositionsAndValues(t *testing.T) {
	doc := container.NewDataContainer()

	ki := key(t, container.TypeInt, 1)
	ks := key(t, container.TypeString, 2)
	require.NoError(t, doc.SetInt(ki, 7))
	require.NoError(t, doc.SetString(ks, "hello"))
	doc.SetNull(key(t, container.TypeBool, 3))

	seen := make(map[container.DataContainerKey]int64)
	posCount := 0
	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		posCount = len(positions)
		ints := *(*[]int64)(slot(container.TypeInt))
		strings := *(*[]string)(slot(container.TypeString))
		for k, idx := range positions {
			dk := container.DataContainerKey(k)
			switch dk.Type() {
			case container.TypeInt:
				seen[dk] = ints[idx]
			case container.TypeString:
				assert.Equal(t, "hello", strings[idx])
			case container.TypeBool:
				assert.Equal(t, int32(-1), idx, "null fields must map to -1")
			}
		}
		return nil, nil
	})
	require.NoError(t, err)

	assert.Equal(t, int64(7), seen[ki])
	assert.Equal(t, 3, posCount)
}

func TestWalk_CanMutatePositions(t *testing.T) {
	doc := container.NewDataContainer()
	k := key(t, container.TypeInt, 1)
	require.NoError(t, doc.SetInt(k, 1))

	// Simulate in-place deserialization: bump the value through the slot.
	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		idx := positions[int64(k)]
		ints := *(*[]int64)(slot(container.TypeInt))
		ints[idx] = 99
		return nil, nil
	})
	require.NoError(t, err)

	v, ok, err := doc.GetInt(k)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(99), v)
}
