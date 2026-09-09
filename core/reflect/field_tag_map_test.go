package reflect

import (
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldTagMap_SetAndGet(t *testing.T) {
	type A struct{ X int }
	type B struct{ Y string }

	typA := reflect.TypeOf(A{})
	typB := reflect.TypeOf(B{})

	tm := newFieldTagMap[int]()

	tm.Set(typA, "X", 42)
	tm.Set(typB, "Y", 99)

	t.Run("get existing entry", func(t *testing.T) {
		v, ok := tm.Get(typA, "X")
		require.True(t, ok)
		assert.Equal(t, 42, v)
	})

	t.Run("get different type same field name", func(t *testing.T) {
		v, ok := tm.Get(typB, "Y")
		require.True(t, ok)
		assert.Equal(t, 99, v)
	})

	t.Run("get non-existent field", func(t *testing.T) {
		_, ok := tm.Get(typA, "NonExistent")
		assert.False(t, ok)
	})

	t.Run("get non-existent type", func(t *testing.T) {
		type C struct{ Z float64 }
		_, ok := tm.Get(reflect.TypeOf(C{}), "Z")
		assert.False(t, ok)
	})
}

func TestFieldTagMap_SetOverwrite(t *testing.T) {
	type S struct{ F string }
	typ := reflect.TypeOf(S{})

	tm := newFieldTagMap[string]()
	tm.Set(typ, "F", "first")
	tm.Set(typ, "F", "second")

	v, ok := tm.Get(typ, "F")
	require.True(t, ok)
	assert.Equal(t, "second", v)
	assert.Equal(t, 1, tm.Len())
}

func TestFieldTagMap_Has(t *testing.T) {
	type S struct{ A, B int }
	typ := reflect.TypeOf(S{})

	tm := newFieldTagMap[bool]()
	tm.Set(typ, "A", true)

	assert.True(t, tm.Has(typ, "A"))
	assert.False(t, tm.Has(typ, "B"))
}

func TestFieldTagMap_Remove(t *testing.T) {
	type S struct{ A, B, C int }
	typ := reflect.TypeOf(S{})

	tm := newFieldTagMap[int]()
	tm.Set(typ, "A", 1)
	tm.Set(typ, "B", 2)
	tm.Set(typ, "C", 3)

	t.Run("remove middle entry", func(t *testing.T) {
		tm.Remove(typ, "B")
		assert.False(t, tm.Has(typ, "B"))
		assert.Equal(t, 2, tm.Len())
		assert.True(t, tm.Has(typ, "A"))
		assert.True(t, tm.Has(typ, "C"))
	})

	t.Run("remove last entry in bucket deletes bucket", func(t *testing.T) {
		tm.Remove(typ, "A")
		tm.Remove(typ, "C")
		assert.Equal(t, 0, tm.Len())
	})

	t.Run("remove non-existent entry is no-op", func(t *testing.T) {
		tm.Remove(typ, "NonExistent")
		assert.Equal(t, 0, tm.Len())
	})
}

func TestFieldTagMap_RemoveLastInBucket(t *testing.T) {
	type S struct{ F int }
	typ := reflect.TypeOf(S{})

	tm := newFieldTagMap[string]()
	tm.Set(typ, "F", "hello")
	require.Equal(t, 1, tm.Len())

	tm.Remove(typ, "F")
	assert.Equal(t, 0, tm.Len())
	_, ok := tm.Get(typ, "F")
	assert.False(t, ok)
}

func TestFieldTagMap_Len(t *testing.T) {
	type S struct{ A, B, C, D, E int }
	typ := reflect.TypeOf(S{})

	tm := newFieldTagMap[int]()
	assert.Equal(t, 0, tm.Len())

	for _, f := range []string{"A", "B", "C", "D", "E"} {
		tm.Set(typ, f, 1)
	}
	assert.Equal(t, 5, tm.Len())
}

func TestFieldTagMap_Clear(t *testing.T) {
	type S struct{ A, B int }
	typ := reflect.TypeOf(S{})

	tm := newFieldTagMap[int]()
	tm.Set(typ, "A", 1)
	tm.Set(typ, "B", 2)
	require.Equal(t, 2, tm.Len())

	tm.Clear()
	assert.Equal(t, 0, tm.Len())
	_, ok := tm.Get(typ, "A")
	assert.False(t, ok)
}

func TestFieldTagMap_NilBuckets(t *testing.T) {
	tm := &fieldTagMap[int]{}

	_, ok := tm.Get(reflect.TypeOf(0), "X")
	assert.False(t, ok)

	assert.False(t, tm.Has(reflect.TypeOf(0), "X"))

	tm.Remove(reflect.TypeOf(0), "X")

	assert.Equal(t, 0, tm.Len())
}

func TestFieldTagMap_MultipleFieldsSameType(t *testing.T) {
	type S struct{ A, B, C int }
	typ := reflect.TypeOf(S{})

	tm := newFieldTagMap[string]()
	tm.Set(typ, "A", "alpha")
	tm.Set(typ, "B", "bravo")
	tm.Set(typ, "C", "charlie")

	for _, tt := range []struct {
		field string
		want  string
	}{
		{"A", "alpha"},
		{"B", "bravo"},
		{"C", "charlie"},
	} {
		v, ok := tm.Get(typ, tt.field)
		require.True(t, ok, "field %s", tt.field)
		assert.Equal(t, tt.want, v)
	}
}

func TestFieldTagMap_MultipleTypesSameFieldName(t *testing.T) {
	type X struct{ Name string }
	type Y struct{ Name string }

	typX := reflect.TypeOf(X{})
	typY := reflect.TypeOf(Y{})

	tm := newFieldTagMap[int]()
	tm.Set(typX, "Name", 1)
	tm.Set(typY, "Name", 2)

	vx, ok := tm.Get(typX, "Name")
	require.True(t, ok)
	assert.Equal(t, 1, vx)

	vy, ok := tm.Get(typY, "Name")
	require.True(t, ok)
	assert.Equal(t, 2, vy)
}

func TestFieldTagMap_ConcurrentAccess(t *testing.T) {
	type S struct{ F int }
	typ := reflect.TypeOf(S{})

	tm := newFieldTagMap[int]()
	const goroutines = 100
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(g int) {
			defer wg.Done()
			for i := range opsPerGoroutine {
				tm.Set(typ, "F", g*opsPerGoroutine+i)
				tm.Get(typ, "F")
				tm.Has(typ, "F")
				tm.Len()
			}
		}(g)
	}
	wg.Wait()

	_, ok := tm.Get(typ, "F")
	assert.True(t, ok)
}

func TestFieldTagMap_DifferentPayloadTypes(t *testing.T) {
	type S struct{ F int }
	typ := reflect.TypeOf(S{})

	t.Run("string payload", func(t *testing.T) {
		tm := newFieldTagMap[string]()
		tm.Set(typ, "F", "hello")
		v, ok := tm.Get(typ, "F")
		require.True(t, ok)
		assert.Equal(t, "hello", v)
	})

	t.Run("slice payload", func(t *testing.T) {
		tm := newFieldTagMap[[]int]()
		tm.Set(typ, "F", []int{1, 2, 3})
		v, ok := tm.Get(typ, "F")
		require.True(t, ok)
		assert.Equal(t, []int{1, 2, 3}, v)
	})

	t.Run("struct payload", func(t *testing.T) {
		type payload struct {
			Name string
			Val  int
		}
		tm := newFieldTagMap[payload]()
		tm.Set(typ, "F", payload{Name: "test", Val: 42})
		v, ok := tm.Get(typ, "F")
		require.True(t, ok)
		assert.Equal(t, payload{Name: "test", Val: 42}, v)
	})

	t.Run("bool payload", func(t *testing.T) {
		tm := newFieldTagMap[bool]()
		tm.Set(typ, "F", true)
		v, ok := tm.Get(typ, "F")
		require.True(t, ok)
		assert.True(t, v)
	})
}
