package container_test

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPool_GetReturnsEmptyDataContainer(t *testing.T) {
	p := container.NewPool()
	doc := p.Get()
	assert.NotNil(t, doc)
	assert.Equal(t, 0, doc.Length())
}

func TestPool_ReusesDataContainers(t *testing.T) {
	p := container.NewPool()
	first := p.Get()
	require.NoError(t, first.SetInt(key(t, container.TypeInt, 1), 1))

	p.Put(first)

	again := p.Get()
	assert.Same(t, first, again, "the same document should come back from the pool")
	assert.Equal(t, 0, again.Length(), "reused documents must be cleared")
}

func TestPool_PutRecursesRecordChildren(t *testing.T) {
	p := container.NewPool()

	child := p.Get()
	require.NoError(t, child.SetString(key(t, container.TypeString, 1), "nested"))

	parent := p.Get()
	rkey := key(t, container.TypeRecord, 1)
	require.NoError(t, parent.SetRecord(rkey, child))

	p.Put(parent)

	// The child must have been returned and cleared when the parent was put.
	assert.Equal(t, 0, child.Length())
	assert.Equal(t, 0, parent.Length())
}

func TestPool_PutRecursesArrayObjectChildren(t *testing.T) {
	p := container.NewPool()

	c1 := p.Get()
	require.NoError(t, c1.SetInt(key(t, container.TypeInt, 1), 1))
	c2 := p.Get()
	require.NoError(t, c2.SetInt(key(t, container.TypeInt, 2), 2))

	parent := p.Get()
	akey := key(t, container.TypeArrayObject, 1)
	require.NoError(t, parent.SetArrayObject(akey, []*container.DataContainer{c1, c2}))

	p.Put(parent)

	assert.Equal(t, 0, c1.Length())
	assert.Equal(t, 0, c2.Length())
	assert.Equal(t, 0, parent.Length())
}

func TestPool_PutNilIsSafe(t *testing.T) {
	p := container.NewPool()
	p.Put(nil)
}

func TestPool_AcquireCallsAndReturns(t *testing.T) {
	p := container.NewPool()

	called := false
	err := p.Acquire(func(doc *container.DataContainer) error {
		called = true
		return doc.SetInt(key(t, container.TypeInt, 1), 1)
	})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestPool_AcquirePropagatesError(t *testing.T) {
	p := container.NewPool()
	sentinel := errors.New("boom")
	err := p.Acquire(func(*container.DataContainer) error {
		return sentinel
	})
	assert.ErrorIs(t, err, sentinel)
}

func TestPool_WalkFillsAndReturns(t *testing.T) {
	p := container.NewPool()
	ks := key(t, container.TypeString, 1)

	doc, err := p.Walk(func(d *container.DataContainer, positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) error {
		return d.SetString(ks, "filled")
	})
	require.NoError(t, err)
	require.NotNil(t, doc)
	defer p.Put(doc)

	v, ok, err := doc.GetString(ks)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "filled", v)
}

func TestPool_WalkErrorReturnsNil(t *testing.T) {
	p := container.NewPool()
	sentinel := errors.New("boom")
	doc, err := p.Walk(func(*container.DataContainer, map[int64]int32, func(container.DataType, ...int) unsafe.Pointer) error {
		return sentinel
	})
	assert.Nil(t, doc)
	assert.ErrorIs(t, err, sentinel)
}
