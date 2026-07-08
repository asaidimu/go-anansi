package base_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSchemaOnlyMigration(t *testing.T) {
	target := &definition.Schema{BaseSchema: definition.BaseSchema{Name: "test"}}
	plan := base.NewSchemaOnlyMigration(target, "add optional field")
	require.NotNil(t, plan)
	assert.Equal(t, "add optional field", plan.Description)
	assert.Equal(t, target, plan.Target)
	assert.Equal(t, base.PhaseSchemaOnly, plan.Phase)
	assert.Nil(t, plan.Transformer)
	assert.Nil(t, plan.Diff)
}

func TestNewFullMigration(t *testing.T) {
	target := &definition.Schema{BaseSchema: definition.BaseSchema{Name: "test"}}
	transformer := func(_ context.Context, doc data.Document) (data.Document, error) { return doc, nil }
	plan := base.NewFullMigration(target, "full copy", transformer)
	require.NotNil(t, plan)
	assert.Equal(t, "full copy", plan.Description)
	assert.Equal(t, target, plan.Target)
	assert.Equal(t, base.PhaseFull, plan.Phase)
	assert.NotNil(t, plan.Transformer)
	assert.Nil(t, plan.Diff)
}

func TestMigrationPlan_TargetVersion(t *testing.T) {
	current := common.MustNewVersion("1.2.3")

	tests := []struct {
		name     string
		bump     definition.VersionBump
		expected string
	}{
		{"major", definition.BumpMajor, "2.0.0"},
		{"minor", definition.BumpMinor, "1.3.0"},
		{"patch", definition.BumpPatch, "1.2.4"},
		{"none", definition.BumpNone, "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &base.MigrationPlan{VersionBump: tt.bump}
			got := plan.TargetVersion(current)
			require.NotNil(t, got)
			assert.Equal(t, tt.expected, got.String())
		})
	}
}

func TestMigrationPlan_ComputeDiff_WithTarget(t *testing.T) {
	current := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "test",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}
	target := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "test",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"f2": {Name: "email", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	plan := &base.MigrationPlan{Target: target}
	err := plan.ComputeDiff(current)
	require.NoError(t, err)
	require.NotNil(t, plan.Diff)
	assert.Len(t, plan.Diff.Changes, 1)
	assert.Equal(t, definition.FieldAdded, plan.Diff.Changes[0].Kind)
	assert.Equal(t, "email", plan.Diff.Changes[0].EntityId)
}

func TestMigrationPlan_ComputeDiff_NilTarget(t *testing.T) {
	plan := &base.MigrationPlan{Target: nil}
	err := plan.ComputeDiff(&definition.Schema{})
	require.NoError(t, err)
	assert.Nil(t, plan.Diff)
}

func TestMigrationPlan_ComputeDiff_VersionBumpSet(t *testing.T) {
	current := &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "test",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}
	target := &definition.Schema{
		Version: common.MustNewVersion("2.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "test",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	plan := &base.MigrationPlan{
		Target:      target,
		VersionBump: definition.BumpNone,
	}
	err := plan.ComputeDiff(current)
	require.NoError(t, err)
	assert.NotEqual(t, definition.BumpNone, plan.VersionBump)
}

func TestMigrationPlan_NeedsDataMigration(t *testing.T) {
	transformer := func(_ context.Context, doc data.Document) (data.Document, error) { return doc, nil }

	tests := []struct {
		name     string
		phase    base.MigrationPhase
		xfm      base.TransformerFunc
		expected bool
	}{
		{"PhaseFull with transformer", base.PhaseFull, transformer, true},
		{"PhaseFull nil transformer", base.PhaseFull, nil, false},
		{"PhaseSchemaOnly with transformer", base.PhaseSchemaOnly, transformer, false},
		{"PhaseSchemaOnly nil transformer", base.PhaseSchemaOnly, nil, false},
		{"PhaseDDL with transformer", base.PhaseDDL, transformer, false},
		{"PhaseDDL nil transformer", base.PhaseDDL, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &base.MigrationPlan{Phase: tt.phase, Transformer: tt.xfm}
			assert.Equal(t, tt.expected, plan.NeedsDataMigration())
		})
	}
}

func TestMigrationPlan_ComputeDiff_VersionImpact(t *testing.T) {
	current := &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: "test",
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	t.Run("major change (field removed)", func(t *testing.T) {
		target := &definition.Schema{
			Version: common.MustNewVersion("2.0.0"),
			BaseSchema: definition.BaseSchema{
				Name:   "test",
				Fields: map[definition.FieldId]definition.Field{},
			},
		}
		plan := &base.MigrationPlan{Target: target}
		err := plan.ComputeDiff(current)
		require.NoError(t, err)
		assert.Equal(t, definition.BumpMajor, plan.VersionBump)
		got := plan.TargetVersion(current.Version)
		assert.Equal(t, "2.0.0", got.String())
	})
}
