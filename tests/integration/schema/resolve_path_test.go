package schema_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rootFieldDesc returns the FieldDescriptor of a root-level field by name.
func rootFieldDesc(t *testing.T, cs *definition.CompiledSchema, name string) (definition.FieldDescriptor, int) {
	t.Helper()
	for i, meta := range cs.FieldsMeta {
		if meta.Name != name {
			continue
		}
		if cs.Descriptors[i].SchemaIdx() != 0 {
			continue
		}
		return cs.Descriptors[i], i
	}
	t.Fatalf("root field %q not found", name)
	return 0, -1
}

func TestResolvePath_SingleStep(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	rp, err := cs.ResolvePath("f_string")
	require.NoError(t, err)
	require.Len(t, rp, 1)

	fd, _ := rootFieldDesc(t, cs, "f_string")
	assert.Equal(t, uint8(0), rp[0].SchemaIdx())
	assert.Equal(t, fd.FieldIdx(), rp[0].FieldIdx())

	addr := cs.Address(rp)
	assert.Greater(t, addr, uint32(0))
	assert.Less(t, addr, uint32(definition.SingleStepRegion))
}

func TestResolvePath_MultiStep(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	rp, err := cs.ResolvePath("f_object.nested_str")
	require.NoError(t, err)
	require.Len(t, rp, 2)

	// Manually reconstruct the same path from the compiled schema.
	fd, _ := rootFieldDesc(t, cs, "f_object")
	childIdx := fd.ChildSchemaIdx()
	require.NotEqual(t, definition.FdNoChild, childIdx)

	var nestedFd definition.FieldDescriptor
	found := false
	for i, meta := range cs.FieldsMeta {
		if meta.Name != "nested_str" || cs.Descriptors[i].SchemaIdx() != childIdx {
			continue
		}
		nestedFd = cs.Descriptors[i]
		found = true
		break
	}
	require.True(t, found, "nested_str not found in child slot")

	manual := definition.ResolvedPath{
		definition.NewResolvedStep(0, fd.FieldIdx()),
		definition.NewResolvedStep(childIdx, nestedFd.FieldIdx()),
	}

	assert.Equal(t, manual, rp, "ResolvePath steps must match manual construction")
	assert.Equal(t, cs.Address(manual), cs.Address(rp))

	addr := cs.Address(rp)
	assert.GreaterOrEqual(t, addr, uint32(definition.MultiStepBase))
	assert.Less(t, addr, uint32(1<<definition.AddrBits))
}

func TestResolvePath_ThroughArrayOfObject(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	rp, err := cs.ResolvePath("f_array_object.nested_int")
	require.NoError(t, err)
	require.Len(t, rp, 2)
	assert.GreaterOrEqual(t, cs.Address(rp), uint32(definition.MultiStepBase))
}

func TestResolvePath_ThroughComposite(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	// A composite collapses its parts into one child schema, so it resolves
	// exactly like an object.
	rp, err := cs.ResolvePath("f_composite.a_name")
	require.NoError(t, err)
	require.Len(t, rp, 2)
	assert.GreaterOrEqual(t, cs.Address(rp), uint32(definition.MultiStepBase))

	rp, err = cs.ResolvePath("f_composite.b_name")
	require.NoError(t, err)
	require.Len(t, rp, 2)
	// Collapsed parts share one address block: distinct fields get distinct
	// flat addresses.
	aName, err := cs.ResolvePath("f_composite.a_name")
	require.NoError(t, err)
	assert.NotEqual(t, cs.Address(aName), cs.Address(rp))
}

func TestResolvePath_RecordResolvesAsContainer(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	// Records compile with a child schema slot (reachable via ChildSchemaIdx),
	// so their fields are addressable through multi-step paths. The record field
	// itself is TypeUnknown (any channel); its children carry the flat keys.
	rp, err := cs.ResolvePath("f_record.nested_str")
	require.NoError(t, err)
	require.Len(t, rp, 2)
	assert.GreaterOrEqual(t, cs.Address(rp), uint32(definition.MultiStepBase))
}

func TestResolvePath_Errors(t *testing.T) {
	cs := compileSchema(t, allTypesSchema)

	tests := []struct {
		name     string
		path     string
		wantCode string
	}{
		{name: "empty path", path: "", wantCode: "ERR_SCHEMA_INVALID_SCHEMA"},
		{name: "unknown field", path: "does_not_exist", wantCode: "ERR_SCHEMA_FAILED_TO_RESOLVE_FIELD"},
		{name: "unknown nested field", path: "f_object.nope", wantCode: "ERR_SCHEMA_FAILED_TO_RESOLVE_FIELD"},
		{name: "descend through terminal", path: "f_unknown.nested_str", wantCode: "ERR_SCHEMA_INVALID_SCHEMA"},
		{name: "descend through union", path: "f_union.a_name", wantCode: "ERR_SCHEMA_INVALID_SCHEMA"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cs.ResolvePath(tt.path)
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, common.SystemErrorFrom(err).Code)
		})
	}
}
