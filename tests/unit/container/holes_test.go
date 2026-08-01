package container_test

import (
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// intPositions returns the backing int64 slice of a document via Walk.
func intPositions(t *testing.T, doc *container.DataContainer) []int64 {
	t.Helper()
	var ints []int64
	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		ints = *(*[]int64)(slot(container.TypeInt))
		return nil, nil
	})
	require.NoError(t, err)
	return ints
}

func TestHoles_LIFOClaim(t *testing.T) {
	doc := container.NewDataContainer()

	k1 := key(t, container.TypeInt, 1)
	k2 := key(t, container.TypeInt, 2)
	k3 := key(t, container.TypeInt, 3)

	require.NoError(t, doc.SetInt(k1, 1))
	require.NoError(t, doc.SetInt(k2, 2))
	require.NoError(t, doc.SetInt(k3, 3))

	// Free indices 0 and 1, in that order. holes = [k1(0), k2(1)].
	doc.SetNull(k1)
	doc.SetNull(k2)

	// claimHole walks the hole list from the end, so the most recently freed
	// index (1) is reused first.
	k4 := key(t, container.TypeInt, 4)
	require.NoError(t, doc.SetInt(k4, 4))

	var idx4 int32 = -2
	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		idx4 = positions[int64(k4)]
		return nil, nil
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), idx4, "LIFO: most recently freed slot must be reclaimed first")

	// k3 still holds index 2; k4 reused 1. Backing slice length stays at 3.
	assert.Equal(t, 3, len(intPositions(t, doc)))
}

func TestHoles_TypeIsolation(t *testing.T) {
	doc := container.NewDataContainer()

	ki := key(t, container.TypeInt, 1)
	ks := key(t, container.TypeString, 1)

	require.NoError(t, doc.SetInt(ki, 1))
	doc.SetNull(ki)

	// A string key must not reclaim the freed int slot.
	require.NoError(t, doc.SetString(ks, "a"))

	ints := intPositions(t, doc)
	assert.Equal(t, 1, len(ints), "int slice keeps the freed slot; string writes must not touch it")
}

func TestHoles_SurviveUnset(t *testing.T) {
	doc := container.NewDataContainer()

	k1 := key(t, container.TypeInt, 1)
	k2 := key(t, container.TypeInt, 2)

	require.NoError(t, doc.SetInt(k1, 1))
	require.NoError(t, doc.SetInt(k2, 2))

	// Unset (delete) also frees the position for reuse.
	doc.Unset(k1)

	k3 := key(t, container.TypeInt, 3)
	require.NoError(t, doc.SetInt(k3, 3))

	assert.Equal(t, 2, len(intPositions(t, doc)))
}
