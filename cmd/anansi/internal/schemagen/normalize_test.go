package schemagen

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/schema/meta"
	"github.com/asaidimu/go-anansi/v8/core/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIsUUIDv7(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"019d7775-6563-7c55-a6f3-ac8f087d89d1", true},  // v7
		{"019d7775-6563-7605-8bfb-c27365b73581", true},  // v7
		{"550e8400-e29b-41d4-a716-446655440000", false}, // v4
		{"userName", false},
		{"not-a-uuid", false},
		{"", false},
		{"00000000-0000-7000-8000-000000000000", true}, // v7
		{"xxx", false},
	}
	for _, tc := range tests {
		got := utils.IsUUIDv7(tc.input)
		require.Equal(t, tc.want, got, "utils.IsUUIDv7(%q)", tc.input)
	}
}

func TestNewUUID(t *testing.T) {
	u := uuid.Must(uuid.NewV7()).String()
	_, err := uuid.Parse(u)
	require.NoError(t, err)
	parsed, _ := uuid.Parse(u)
	require.Equal(t, uuid.Version(7), parsed.Version())
}

func TestNormalizeSchema_NoChanges(t *testing.T) {
	s := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"019d7775-6563-7c55-a6f3-ac8f087d89d1": {Name: "name"},
			},
		},
	}
	changed := meta.NormalizeSchema(s)
	require.False(t, changed)
}

func TestNormalizeSchema_FieldID(t *testing.T) {
	s := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"userName": {},
			},
		},
	}
	changed := meta.NormalizeSchema(s)
	require.True(t, changed)

	require.Len(t, s.Fields, 1)
	for id, f := range s.Fields {
		require.True(t, utils.IsUUIDv7(string(id)), "field ID should be UUID v7")
		require.Equal(t, definition.FieldName("userName"), f.Name, "name should come from old key")
	}
}

func TestNormalizeSchema_FieldIDWithExistingName(t *testing.T) {
	s := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"userName": {Name: "username"},
			},
		},
	}
	changed := meta.NormalizeSchema(s)
	require.True(t, changed)

	for _, f := range s.Fields {
		require.Equal(t, definition.FieldName("username"), f.Name, "existing name preserved")
	}
}

func TestNormalizeSchema_IndexID(t *testing.T) {
	s := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Indexes: map[definition.IndexID]definition.Index{
				"by_email": {Name: "", Fields: []definition.FieldName{"email"}},
			},
		},
	}
	changed := meta.NormalizeSchema(s)
	require.True(t, changed)

	for id, idx := range s.Indexes {
		require.True(t, utils.IsUUIDv7(string(id)))
		require.Equal(t, "by_email", idx.Name)
	}
}

func TestNormalizeSchema_ConstraintID(t *testing.T) {
	s := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Constraints: map[definition.ConstraintId]definition.Constraint{
				"unique_email": {Name: "unique email"},
			},
		},
	}
	changed := meta.NormalizeSchema(s)
	require.True(t, changed)

	for id, c := range s.Constraints {
		require.True(t, utils.IsUUIDv7(string(id)))
		require.Equal(t, "unique email", c.Name)
	}
}

func TestNormalizeSchema_SchemaID(t *testing.T) {
	s := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"addr": {
					Name: "addr",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeObject,
						Schema: definition.NewSchemaReference(definition.SchemaReference{
							ID: "address",
						}),
					},
				},
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			"address": {BaseSchema: definition.BaseSchema{Name: "Address"}},
		},
	}
	changed := meta.NormalizeSchema(s)
	require.True(t, changed)

	require.Len(t, s.Schemas, 1)
	for sid := range s.Schemas {
		require.True(t, utils.IsUUIDv7(string(sid)))
	}

	// SchemaReference.ID should be updated to the new UUID
	for _, f := range s.Fields {
		sr, _ := definition.FieldSchemaAs[definition.SchemaReference](f.Schema)
		_, exists := s.Schemas[definition.SchemaId(sr.ID)]
		require.True(t, exists, "SchemaReference.ID should point to a valid schema key")
	}
}

func TestNormalizeSchema_NestedSchemaRecursion(t *testing.T) {
	s := &definition.Schema{
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			"address": {
				BaseSchema: definition.BaseSchema{
					Name: "Address",
					Fields: map[definition.FieldId]definition.Field{
						"street": {},
					},
				},
			},
		},
	}
	changed := meta.NormalizeSchema(s)
	require.True(t, changed)

	for _, ns := range s.Schemas {
		require.Equal(t, "Address", ns.Name)
		for fid, f := range ns.Fields {
			require.True(t, utils.IsUUIDv7(string(fid)))
			require.Equal(t, definition.FieldName("street"), f.Name)
		}
	}
}

func TestNormalizeSchema_AlreadyNormalizedFieldRefs(t *testing.T) {
	// Schema references that already use UUIDs should be untouched
	fieldID := definition.FieldId(newUUID())
	schemaID := definition.SchemaId(newUUID())
	s := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				fieldID: {
					Name: "addr",
					FieldProperties: definition.FieldProperties{
						Type: definition.FieldTypeObject,
						Schema: definition.NewSchemaReference(definition.SchemaReference{
							ID: schemaID,
						}),
					},
				},
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			schemaID: {BaseSchema: definition.BaseSchema{Name: "Address"}},
		},
	}
	changed := meta.NormalizeSchema(s)
	require.False(t, changed, "already normalized schema should not change")
}

func TestNormalizeSchema_RemoveUnusedImports(t *testing.T) {
	s := &definition.Schema{}
	changed := meta.NormalizeSchema(s)
	require.False(t, changed)
}


func newUUID() string {
	return uuid.Must(uuid.NewV7()).String()
}
