package container_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollection_AppendLenAt(t *testing.T) {
	c := container.NewCollection(nil)
	require.Equal(t, 0, c.Len())

	d1 := container.NewDataContainer()
	d2 := container.NewDataContainer()
	require.NoError(t, c.Append(d1))
	require.NoError(t, c.Append(d2))

	assert.Equal(t, 2, c.Len())
	assert.Same(t, d1, c.At(0))
	assert.Same(t, d2, c.At(1))
	assert.Panics(t, func() { c.At(2) })
}

func TestCollection_AppendNil(t *testing.T) {
	c := container.NewCollection(nil)
	assert.Error(t, c.Append(nil))
}

func TestCollection_EachStopsEarly(t *testing.T) {
	c := container.NewCollection(nil)
	for i := 0; i < 3; i++ {
		require.NoError(t, c.Append(container.NewDataContainer()))
	}

	count := 0
	c.Each(func(i int, doc *container.DataContainer) bool {
		count++
		return false
	})
	assert.Equal(t, 1, count)
}

func TestCollection_FilterIsView(t *testing.T) {
	pool := container.NewPool()
	src := container.NewCollection(pool)

	d1 := pool.Get()
	require.NoError(t, d1.SetInt(key(t, container.TypeInt, 1), 10))
	d2 := pool.Get()
	require.NoError(t, d2.SetInt(key(t, container.TypeInt, 2), 20))
	require.NoError(t, src.Append(d1))
	require.NoError(t, src.Append(d2))

	view := src.Filter(func(d *container.DataContainer) bool {
		v, ok, _ := d.GetInt(key(t, container.TypeInt, 1))
		return ok && v == 10
	})
	require.Equal(t, 1, view.Len())
	assert.Same(t, d1, view.At(0))

	// Releasing the view must not touch the source's documents.
	view.Release()
	assert.Equal(t, 2, src.Len())

	// Releasing the source returns documents to the pool (view becomes stale).
	src.Release()
	assert.Equal(t, 0, src.Len())
}

func TestCollection_FilterCopy_DeepCopiesScalar(t *testing.T) {
	pool := container.NewPool()
	src := container.NewCollection(pool)

	d := pool.Get()
	require.NoError(t, d.SetInt(key(t, container.TypeInt, 1), 42))
	require.NoError(t, d.SetString(key(t, container.TypeString, 2), "abc"))
	require.NoError(t, src.Append(d))

	copy, err := src.FilterCopy(func(*container.DataContainer) bool { return true })
	require.NoError(t, err)
	require.Equal(t, 1, copy.Len())
	defer copy.Release()

	copied := copy.At(0)
	assert.NotSame(t, d, copied)

	v, ok, err := copied.GetInt(key(t, container.TypeInt, 1))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int64(42), v)

	s, ok, err := copied.GetString(key(t, container.TypeString, 2))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "abc", s)

	src.Release()
}

func TestCollection_FilterCopy_ClonesRecordMap(t *testing.T) {
	pool := container.NewPool()
	src := container.NewCollection(pool)

	parent := pool.Get()
	rkey := key(t, container.TypeRecord, 1)
	require.NoError(t, parent.SetRecord(rkey, map[string]any{"name": "original"}))
	require.NoError(t, src.Append(parent))

	copy, err := src.FilterCopy(func(*container.DataContainer) bool { return true })
	require.NoError(t, err)
	require.Equal(t, 1, copy.Len())
	defer copy.Release()

	origMap, _, err := parent.GetRecord(rkey)
	require.NoError(t, err)
	copiedMap, _, err := copy.At(0).GetRecord(rkey)
	require.NoError(t, err)

	// Mutating the source map must not leak into the copy (the copy is cloned).
	origMap["name"] = "mutated"
	assert.Equal(t, "original", copiedMap["name"])

	src.Release()
}

func TestCollection_FilterCopy_DeepCopiesArrayObject(t *testing.T) {
	pool := container.NewPool()
	src := container.NewCollection(pool)

	c1 := pool.Get()
	require.NoError(t, c1.SetInt(key(t, container.TypeInt, 1), 1))
	parent := pool.Get()
	akey := key(t, container.TypeArrayObject, 1)
	require.NoError(t, parent.SetArrayObject(akey, []*container.DataContainer{c1}))
	require.NoError(t, src.Append(parent))

	copy, err := src.FilterCopy(func(*container.DataContainer) bool { return true })
	require.NoError(t, err)
	defer copy.Release()

	origGroup, _, err := parent.GetArrayObject(akey)
	require.NoError(t, err)
	copiedGroup, _, err := copy.At(0).GetArrayObject(akey)
	require.NoError(t, err)

	require.Len(t, origGroup, 1)
	require.Len(t, copiedGroup, 1)
	assert.NotSame(t, origGroup[0], copiedGroup[0])

	src.Release()
}

func TestCollection_FilterCopy_CopiesNullState(t *testing.T) {
	pool := container.NewPool()
	src := container.NewCollection(pool)

	d := pool.Get()
	d.SetNull(key(t, container.TypeInt, 1))
	require.NoError(t, src.Append(d))

	copy, err := src.FilterCopy(func(*container.DataContainer) bool { return true })
	require.NoError(t, err)
	defer copy.Release()

	assert.True(t, copy.At(0).IsNull(key(t, container.TypeInt, 1)))
	src.Release()
}

func TestCollection_FilterCopy_NilPool(t *testing.T) {
	src := container.NewCollection(nil)
	_, err := src.FilterCopy(func(*container.DataContainer) bool { return true })
	assert.Error(t, err)
}

func TestCollection_Project_SelectsKeysOnly(t *testing.T) {
	pool := container.NewPool()
	src := container.NewCollection(pool)

	k1 := key(t, container.TypeInt, 1)
	k2 := key(t, container.TypeString, 2)
	d := pool.Get()
	require.NoError(t, d.SetInt(k1, 1))
	require.NoError(t, d.SetString(k2, "keep"))
	require.NoError(t, src.Append(d))

	out, err := src.Project([]container.DataContainerKey{k1})
	require.NoError(t, err)
	require.Equal(t, 1, out.Len())
	defer out.Release()

	proj := out.At(0)
	assert.Equal(t, 1, proj.Length())
	assert.True(t, proj.IsSet(k1))
	assert.False(t, proj.IsSet(k2))

	src.Release()
}

func TestCollection_Project_ClonesRecordMaps(t *testing.T) {
	pool := container.NewPool()
	src := container.NewCollection(pool)

	parent := pool.Get()
	rkey := key(t, container.TypeRecord, 1)
	require.NoError(t, parent.SetRecord(rkey, map[string]any{"name": "nested"}))
	require.NoError(t, src.Append(parent))

	out, err := src.Project([]container.DataContainerKey{rkey})
	require.NoError(t, err)
	defer out.Release()

	origMap, _, err := parent.GetRecord(rkey)
	require.NoError(t, err)
	copiedMap, _, err := out.At(0).GetRecord(rkey)
	require.NoError(t, err)

	origMap["name"] = "changed"
	assert.Equal(t, "nested", copiedMap["name"])

	src.Release()
}

func TestCollection_Project_NilPool(t *testing.T) {
	src := container.NewCollection(nil)
	_, err := src.Project([]container.DataContainerKey{})
	assert.Error(t, err)
}

func TestCollection_Release_ReusesDataContainers(t *testing.T) {
	pool := container.NewPool()
	c := container.NewCollection(pool)

	d := pool.Get()
	require.NoError(t, d.SetInt(key(t, container.TypeInt, 1), 1))
	require.NoError(t, c.Append(d))

	c.Release()
	assert.Equal(t, 0, c.Len())

	reused := pool.Get()
	assert.Equal(t, 0, reused.Length())
}
