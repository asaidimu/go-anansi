package definition_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	. "github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiff_BothNil(t *testing.T) {
	diff, err := Diff(nil, nil)
	require.NoError(t, err)
	require.NotNil(t, diff)
	assert.Empty(t, diff.Changes)
}

func TestDiff_OldNil_NewWithFields(t *testing.T) {
	diff, err := Diff(nil, &Schema{
		BaseSchema: BaseSchema{
			Fields: map[FieldId]Field{
				"f1": {Name: "name", FieldProperties: FieldProperties{Type: FieldTypeString}},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, FieldAdded, diff.Changes[0].Kind)
	assert.Equal(t, "name", diff.Changes[0].EntityId)
}

func TestDiff_FieldAdded(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{Fields: map[FieldId]Field{}}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "email", FieldProperties: FieldProperties{Type: FieldTypeString}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	c := diff.Changes[0]
	assert.Equal(t, FieldAdded, c.Kind)
	assert.Equal(t, "email", c.EntityId)
	require.Len(t, c.Forward, 1)
	assert.Equal(t, OpAdd, c.Forward[0].Type)
	require.Len(t, c.Backward, 1)
	assert.Equal(t, OpRemove, c.Backward[0].Type)
}

func TestDiff_FieldRemoved(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "email", FieldProperties: FieldProperties{Type: FieldTypeString}},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{Fields: map[FieldId]Field{}}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	c := diff.Changes[0]
	assert.Equal(t, FieldRemoved, c.Kind)
	assert.Equal(t, "email", c.EntityId)
	require.Len(t, c.Forward, 1)
	assert.Equal(t, OpRemove, c.Forward[0].Type)
	require.Len(t, c.Backward, 1)
	assert.Equal(t, OpAdd, c.Backward[0].Type)
}

func TestDiff_FieldModified_Name(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "old_name", FieldProperties: FieldProperties{Type: FieldTypeString}},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "new_name", FieldProperties: FieldProperties{Type: FieldTypeString}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	c := diff.Changes[0]
	assert.Equal(t, FieldModified, c.Kind)
	hasNameOp := false
	for _, op := range c.Forward {
		if op.Type == OpSet && len(op.Path.Segments) > 1 && op.Path.Segments[1].Type == PathName {
			hasNameOp = true
			assert.Equal(t, "new_name", op.Value)
		}
	}
	assert.True(t, hasNameOp)
}

func TestDiff_FieldModified_Type(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "age", FieldProperties: FieldProperties{Type: FieldTypeInteger}},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "age", FieldProperties: FieldProperties{Type: FieldTypeString}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	c := diff.Changes[0]
	assert.Equal(t, FieldModified, c.Kind)
	hasTypeOp := false
	for _, op := range c.Forward {
		if op.Type == OpSet && len(op.Path.Segments) > 1 && op.Path.Segments[1].Type == PathType {
			hasTypeOp = true
		}
	}
	assert.True(t, hasTypeOp)
}

func TestDiff_FieldModified_Required(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "email", Required: false},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "email", Required: true},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	c := diff.Changes[0]
	assert.Equal(t, FieldModified, c.Kind)
	hasRequiredOp := false
	for _, op := range c.Forward {
		if op.Type == OpSet && len(op.Path.Segments) > 1 && op.Path.Segments[1].Type == PathRequired {
			hasRequiredOp = true
			assert.Equal(t, true, op.Value)
		}
	}
	assert.True(t, hasRequiredOp)
}

func TestDiff_FieldModified_Deprecated(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "old_field", Deprecated: false},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "old_field", Deprecated: true},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	c := diff.Changes[0]
	assert.Equal(t, FieldModified, c.Kind)
}

func TestDiff_FieldModified_Default(t *testing.T) {
	defaultOld, _ := NewLiteralValue("active")
	defaultNew, _ := NewLiteralValue("inactive")
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "status", FieldProperties: FieldProperties{Default: defaultOld}},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "status", FieldProperties: FieldProperties{Default: defaultNew}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	c := diff.Changes[0]
	assert.Equal(t, FieldModified, c.Kind)
}

func TestDiff_FieldModified_MultipleProperties(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "x", Description: "old desc", Required: false, FieldProperties: FieldProperties{Type: FieldTypeInteger}},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "y", Description: "new desc", Required: true, FieldProperties: FieldProperties{Type: FieldTypeString}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	c := diff.Changes[0]
	assert.Equal(t, FieldModified, c.Kind)
	assert.GreaterOrEqual(t, len(c.Forward), 3)
}

func TestDiff_NoChanges(t *testing.T) {
	old := &Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: BaseSchema{
			Name: "test",
			Fields: map[FieldId]Field{
				"f1": {Name: "name", FieldProperties: FieldProperties{Type: FieldTypeString}},
			},
		},
	}
	new := &Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: BaseSchema{
			Name: "test",
			Fields: map[FieldId]Field{
				"f1": {Name: "name", FieldProperties: FieldProperties{Type: FieldTypeString}},
			},
		},
	}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	assert.Empty(t, diff.Changes)
}

func TestDiff_IndexAdded(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields:  map[FieldId]Field{"f1": {Name: "email"}},
		Indexes: map[IndexID]Index{},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{"f1": {Name: "email"}},
		Indexes: map[IndexID]Index{
			"i1": {Name: "idx_email", Fields: []FieldName{"email"}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	c := diff.Changes[0]
	assert.Equal(t, IndexAdded, c.Kind)
	assert.Equal(t, "idx_email", c.EntityId)
}

func TestDiff_IndexRemoved(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{"f1": {Name: "email"}},
		Indexes: map[IndexID]Index{
			"i1": {Name: "idx_email", Fields: []FieldName{"email"}},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields:  map[FieldId]Field{"f1": {Name: "email"}},
		Indexes: map[IndexID]Index{},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	c := diff.Changes[0]
	assert.Equal(t, IndexRemoved, c.Kind)
	assert.Equal(t, "idx_email", c.EntityId)
}

func TestDiff_IndexModified_Name(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Indexes: map[IndexID]Index{
			"i1": {Name: "old_name", Fields: []FieldName{"email"}},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Indexes: map[IndexID]Index{
			"i1": {Name: "new_name", Fields: []FieldName{"email"}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, IndexModified, diff.Changes[0].Kind)
}

func TestDiff_IndexModified_Fields(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Indexes: map[IndexID]Index{
			"i1": {Name: "idx", Fields: []FieldName{"a"}},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Indexes: map[IndexID]Index{
			"i1": {Name: "idx", Fields: []FieldName{"a", "b"}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, IndexModified, diff.Changes[0].Kind)
}

func TestDiff_ConstraintAdded(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Constraints: map[ConstraintId]Constraint{},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Constraints: map[ConstraintId]Constraint{
			"c1": {Name: "ck_age", ConstraintUnion: NewConstrainUnion(&ConstraintRule{Predicate: "age > 0"})},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, ConstraintAdded, diff.Changes[0].Kind)
}

func TestDiff_ConstraintRemoved(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Constraints: map[ConstraintId]Constraint{
			"c1": {Name: "ck_age", ConstraintUnion: NewConstrainUnion(&ConstraintRule{Predicate: "age > 0"})},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Constraints: map[ConstraintId]Constraint{},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, ConstraintRemoved, diff.Changes[0].Kind)
}

func TestDiff_ConstraintModified(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Constraints: map[ConstraintId]Constraint{
			"c1": {Name: "ck", ConstraintUnion: NewConstrainUnion(&ConstraintRule{Predicate: "age > 0"})},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Constraints: map[ConstraintId]Constraint{
			"c1": {Name: "ck", ConstraintUnion: NewConstrainUnion(&ConstraintRule{Predicate: "age >= 18"})},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, ConstraintModified, diff.Changes[0].Kind)
}

func TestDiff_NestedSchemaAdded(t *testing.T) {
	old := &Schema{Schemas: map[SchemaId]NestedSchema{}}
	new := &Schema{
		Schemas: map[SchemaId]NestedSchema{
			"s1": {BaseSchema: BaseSchema{Name: "address"}, FieldProperties: FieldProperties{Type: FieldTypeObject}},
		},
	}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, SchemaAdded, diff.Changes[0].Kind)
}

func TestDiff_NestedSchemaRemoved(t *testing.T) {
	old := &Schema{
		Schemas: map[SchemaId]NestedSchema{
			"s1": {BaseSchema: BaseSchema{Name: "address"}, FieldProperties: FieldProperties{Type: FieldTypeObject}},
		},
	}
	new := &Schema{Schemas: map[SchemaId]NestedSchema{}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, SchemaRemoved, diff.Changes[0].Kind)
}

func TestDiff_NestedSchemaModified(t *testing.T) {
	old := &Schema{
		Schemas: map[SchemaId]NestedSchema{
			"s1": {BaseSchema: BaseSchema{Name: "addr"}, FieldProperties: FieldProperties{Type: FieldTypeObject}},
		},
	}
	new := &Schema{
		Schemas: map[SchemaId]NestedSchema{
			"s1": {BaseSchema: BaseSchema{Name: "address"}, FieldProperties: FieldProperties{Type: FieldTypeObject}},
		},
	}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, SchemaModified, diff.Changes[0].Kind)
}

func TestDiff_MetadataAdded(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{Metadata: map[string]any{}}}
	new := &Schema{BaseSchema: BaseSchema{
		Metadata: map[string]any{"key1": "value1"},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, MetadataAdded, diff.Changes[0].Kind)
	assert.Equal(t, "key1", diff.Changes[0].EntityId)
}

func TestDiff_MetadataRemoved(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Metadata: map[string]any{"key1": "value1"},
	}}
	new := &Schema{BaseSchema: BaseSchema{Metadata: map[string]any{}}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, MetadataRemoved, diff.Changes[0].Kind)
	assert.Equal(t, "key1", diff.Changes[0].EntityId)
}

func TestDiff_MetadataModified(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Metadata: map[string]any{"key1": "old"},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Metadata: map[string]any{"key1": "new"},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, MetadataModified, diff.Changes[0].Kind)
	assert.Equal(t, "key1", diff.Changes[0].EntityId)
}

func TestDiff_RootModified_Name(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{Name: "old_name"}}
	new := &Schema{BaseSchema: BaseSchema{Name: "new_name"}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, RootModified, diff.Changes[0].Kind)
}

func TestDiff_RootModified_Description(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{Description: "old"}}
	new := &Schema{BaseSchema: BaseSchema{Description: "new"}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, RootModified, diff.Changes[0].Kind)
}

func TestDiff_RootModified_Version(t *testing.T) {
	old := &Schema{Version: common.MustNewVersion("1.0.0")}
	new := &Schema{Version: common.MustNewVersion("2.0.0")}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, RootModified, diff.Changes[0].Kind)
}

func TestDiff_MultipleChanges(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Name: "test",
		Fields: map[FieldId]Field{
			"f1": {Name: "keep", FieldProperties: FieldProperties{Type: FieldTypeString}},
			"f2": {Name: "remove", FieldProperties: FieldProperties{Type: FieldTypeInteger}},
		},
		Indexes: map[IndexID]Index{},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Name: "test",
		Fields: map[FieldId]Field{
			"f1": {Name: "keep", FieldProperties: FieldProperties{Type: FieldTypeString}},
			"f3": {Name: "added", FieldProperties: FieldProperties{Type: FieldTypeBoolean}},
		},
		Indexes: map[IndexID]Index{
			"i1": {Name: "idx_added", Fields: []FieldName{"added"}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	assert.Len(t, diff.Changes, 3)
}

func TestDiff_ForwardBackwardSymmetry(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Name: "test",
		Fields: map[FieldId]Field{
			"f1": {Name: "name", FieldProperties: FieldProperties{Type: FieldTypeString}},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Name: "test",
		Fields: map[FieldId]Field{
			"f1": {Name: "name", FieldProperties: FieldProperties{Type: FieldTypeString}},
			"f2": {Name: "email", FieldProperties: FieldProperties{Type: FieldTypeString}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)

	diffBack, err := Diff(new, old)
	require.NoError(t, err)

	require.Len(t, diff.Changes, 1)
	require.Len(t, diffBack.Changes, 1)
	assert.Equal(t, FieldAdded, diff.Changes[0].Kind)
	assert.Equal(t, FieldRemoved, diffBack.Changes[0].Kind)
}

func TestDiff_FieldModified_Nullable(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "nickname", Nullable: false},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "nickname", Nullable: true},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, FieldModified, diff.Changes[0].Kind)
}

func TestDiff_FieldModified_Unique(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "username", Unique: false},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "username", Unique: true},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, FieldModified, diff.Changes[0].Kind)
}

func TestDiff_FieldModified_SchemaRef(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "addr", FieldProperties: FieldProperties{Schema: FieldSchemaReference{}}},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Fields: map[FieldId]Field{
			"f1": {Name: "addr", FieldProperties: FieldProperties{
				Schema: NewSchemaReference(SchemaReference{ID: "s1"}),
			}},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, FieldModified, diff.Changes[0].Kind)
}

func TestDiff_IndexModified_Unique(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Indexes: map[IndexID]Index{
			"i1": {Name: "idx", Fields: []FieldName{"email"}, Unique: false},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Indexes: map[IndexID]Index{
			"i1": {Name: "idx", Fields: []FieldName{"email"}, Unique: true},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, IndexModified, diff.Changes[0].Kind)
}

func TestDiff_IndexModified_Order(t *testing.T) {
	old := &Schema{BaseSchema: BaseSchema{
		Indexes: map[IndexID]Index{
			"i1": {Name: "idx", Fields: []FieldName{"a"}, Order: "asc"},
		},
	}}
	new := &Schema{BaseSchema: BaseSchema{
		Indexes: map[IndexID]Index{
			"i1": {Name: "idx", Fields: []FieldName{"a"}, Order: "desc"},
		},
	}}

	diff, err := Diff(old, new)
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, IndexModified, diff.Changes[0].Kind)
}

func TestVersionBump_Apply(t *testing.T) {
	v := common.MustNewVersion("1.2.3")
	assert.Equal(t, "2.0.0", BumpMajor.Apply(*v).String())
	assert.Equal(t, "1.3.0", BumpMinor.Apply(*v).String())
	assert.Equal(t, "1.2.4", BumpPatch.Apply(*v).String())
	assert.Equal(t, "1.2.3", BumpNone.Apply(*v).String())
}

func TestVersionBump_String(t *testing.T) {
	assert.Equal(t, "major", BumpMajor.String())
	assert.Equal(t, "minor", BumpMinor.String())
	assert.Equal(t, "patch", BumpPatch.String())
	assert.Equal(t, "none", BumpNone.String())
}

func TestVersionImpact_FieldAdded_Required(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind:     FieldAdded,
				EntityId: "email",
				Forward:  []Operation{{Type: OpAdd, Value: Field{Name: "email", Required: true}}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_FieldAdded_Optional(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind:     FieldAdded,
				EntityId: "email",
				Forward:  []Operation{{Type: OpAdd, Value: Field{Name: "email", Required: false}}},
			},
		},
	}
	assert.Equal(t, BumpMinor, VersionImpact(diff))
}

func TestVersionImpact_FieldRemoved(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: FieldRemoved,
				Forward: []Operation{{Type: OpRemove}},
				Backward: []Operation{{Type: OpAdd, Value: Field{Name: "email"}}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_FieldModified_TypeChange(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: FieldModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "f1"}, {Type: PathType}}},
					Value: FieldTypeString,
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_FieldModified_NameChange(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: FieldModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "f1"}, {Type: PathName}}},
					Value: "new_name",
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_FieldModified_RequiredTrue(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: FieldModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "f1"}, {Type: PathRequired}}},
					Value: true,
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_FieldModified_RequiredFalse(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: FieldModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "f1"}, {Type: PathRequired}}},
					Value: false,
				}},
			},
		},
	}
	assert.Equal(t, BumpMinor, VersionImpact(diff))
}

func TestVersionImpact_FieldModified_DefaultChange(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: FieldModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "f1"}, {Type: PathDefault}}},
					Value: "new_default",
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_FieldModified_Deprecated(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: FieldModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "f1"}, {Type: PathDeprecated}}},
					Value: true,
				}},
			},
		},
	}
	assert.Equal(t, BumpMinor, VersionImpact(diff))
}

func TestVersionImpact_FieldModified_UniqueTrue(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: FieldModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "f1"}, {Type: PathUnique}}},
					Value: true,
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_FieldModified_SchemaRef(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: FieldModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "f1"}, {Type: PathFieldSchema}}},
					Value: NewSchemaReference(SchemaReference{ID: "s1"}),
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_IndexAdded_Unique(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: IndexAdded,
				Forward: []Operation{{
					Type: OpAdd,
					Value: Index{Name: "idx", Fields: []FieldName{"email"}, Unique: true},
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_IndexRemoved_Unique(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: IndexRemoved,
				Forward: []Operation{{Type: OpRemove}},
				Backward: []Operation{{
					Type: OpAdd,
					Value: Index{Name: "idx", Fields: []FieldName{"email"}, Unique: true},
				}},
			},
		},
	}
	assert.Equal(t, BumpMinor, VersionImpact(diff))
}

func TestVersionImpact_IndexModified_UniqueTrue(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: IndexModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "i1"}, {Type: PathIndexUnique}}},
					Value: true,
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_IndexModified_FieldsChanged(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: IndexModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "i1"}, {Type: PathFields}}},
					Value: []FieldName{"a", "b"},
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_ConstraintAdded(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{Kind: ConstraintAdded},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_ConstraintModified_Predicate(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: ConstraintModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "c1"}, {Type: PathPredicate}}},
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_SchemaAdded(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{Kind: SchemaAdded},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_SchemaRemoved(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{Kind: SchemaRemoved},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_SchemaModified_Type(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: SchemaModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "s1"}, {Type: PathType}}},
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_SchemaModified_Values(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{
				Kind: SchemaModified,
				Forward: []Operation{{
					Type: OpSet,
					Path: Path{Segments: []PathSegment{{Type: PathEntity, Key: "s1"}, {Type: PathValues}}},
				}},
			},
		},
	}
	assert.Equal(t, BumpMajor, VersionImpact(diff))
}

func TestVersionImpact_NoChanges(t *testing.T) {
	diff := &SchemaDiff{Changes: nil}
	assert.Equal(t, BumpPatch, VersionImpact(diff))
}

func TestVersionImpact_MetadataChange(t *testing.T) {
	diff := &SchemaDiff{
		Changes: []SemanticChange{
			{Kind: MetadataAdded, EntityId: "key1"},
		},
	}
	assert.Equal(t, BumpPatch, VersionImpact(diff))
}
