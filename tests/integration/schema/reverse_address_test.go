package schema_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReverseAddress_FirstRootField guards against address 0 being a valid
// leaf address: the very first root field occupies abs 0, which previously
// computed address 0 and collided with the "not addressable" sentinel, so it
// silently fell back to internal keys. Addresses are shifted to [1, 2^14).
func TestReverseAddress_FirstRootField(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	// Root fields are sorted by name; the first descriptor is abs 0.
	fd := cs.Descriptors[cs.Schemas[0].FieldStart]
	rp := definition.ResolvedPath{definition.NewResolvedStep(0, fd.FieldIdx())}

	addr := cs.Address(rp)
	require.NotZero(t, addr, "first root field must get a non-zero address")

	got, ok := cs.PathForAddress(addr)
	require.True(t, ok, "first root field's address must reverse-map to its path")
	assert.Equal(t, rp, got)
}

// TestPathForAddress_Empty verifies the reverse cache starts empty: no path has
// been resolved, so no address can map back to a path yet.
func TestPathForAddress_Empty(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	_, ok := cs.PathForAddress(1)
	assert.False(t, ok)
}

// TestPathForAddress_AfterResolution resolves a path, computes its address, and
// verifies the reverse mapping recovers the exact same path and dotted string.
func TestPathForAddress_AfterResolution(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	tests := []struct {
		path string
	}{
		{path: "f_string"},
		{path: "f_object.nested_str"},
		{path: "f_object.nested_int"},
		{path: "f_composite.a_name"},
		{path: "f_array_object.nested_int"},
		{path: "f_record.nested_str"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rp, err := cs.ResolvePath(tt.path)
			require.NoError(t, err)

			addr := cs.Address(rp)
			require.NotZero(t, addr, "addressable path must get a non-zero address")

			got, ok := cs.PathForAddress(addr)
			require.True(t, ok, "reverse mapping must exist for a resolved address")
			assert.Equal(t, rp, got, "recovered path must match the resolved path")
			assert.Equal(t, tt.path, cs.PathString(got))
		})
	}
}

// TestPathForAddress_NonTerminal verifies that a path with address 0 (a path
// that resolves but is not a terminal leaf) records no reverse mapping.
func TestPathForAddress_NonTerminal(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	// An object field is non-terminal; Address returns 0 and no reverse entry.
	rp, err := cs.ResolvePath("f_object")
	require.NoError(t, err)
	assert.Zero(t, cs.Address(rp))

	_, ok := cs.PathForAddress(0)
	assert.False(t, ok)
}

// TestPathForAddress_ReusedSchemaMounts verifies that the same dotted path
// mounted at different schema positions gets distinct addresses, each mapping
// back to its own path and the shared dotted string.
func TestPathForAddress_ReusedSchemaMounts(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	obj, err := cs.ResolvePath("f_object.nested_str")
	require.NoError(t, err)
	arr, err := cs.ResolvePath("f_array_object.nested_str")
	require.NoError(t, err)

	addrObj := cs.Address(obj)
	addrArr := cs.Address(arr)
	assert.NotEqual(t, addrObj, addrArr, "distinct mounts must get distinct addresses")
	assert.Equal(t, "f_object.nested_str", cs.PathString(obj))
	assert.Equal(t, "f_array_object.nested_str", cs.PathString(arr))

	gotObj, ok := cs.PathForAddress(addrObj)
	require.True(t, ok)
	assert.Equal(t, obj, gotObj)

	gotArr, ok := cs.PathForAddress(addrArr)
	require.True(t, ok)
	assert.Equal(t, arr, gotArr)
}

// TestPathString verifies dotted rendering for single-step, multi-step, and
// descriptor-local (FieldPath) paths.
func TestPathString(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	rp, err := cs.ResolvePath("f_object.nested_int")
	require.NoError(t, err)
	assert.Equal(t, "f_object.nested_int", cs.PathString(rp))

	fd, _ := rootFieldDesc(t, cs, "f_string")
	assert.Equal(t, "f_string", cs.FieldPath(fd))
}

// TestReverseCache_AfterRoundtrip verifies the cache is not serialized: a
// deserialized schema starts with an empty reverse cache but rebuilds it
// lazily on the next Address() call, with no format change required.
func TestReverseCache_AfterRoundtrip(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	data, err := definition.SerializeCompiledSchema(cs)
	require.NoError(t, err)

	restored, err := definition.DeserializeCompiledSchema(data)
	require.NoError(t, err)

	_, ok := restored.PathForAddress(1)
	assert.False(t, ok, "reverse cache must not be serialized")

	rp, err := restored.ResolvePath("f_object.nested_str")
	require.NoError(t, err)
	addr := restored.Address(rp)
	require.NotZero(t, addr)

	got, ok := restored.PathForAddress(addr)
	require.True(t, ok, "reverse cache must rebuild lazily on demand")
	assert.Equal(t, rp, got)
}
