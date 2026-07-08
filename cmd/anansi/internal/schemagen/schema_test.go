package schemagen

import (
	"os"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/stretchr/testify/require"
)

func TestSafeIdent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Users", "Users"},
		{"My Collection", "My_Collection"},
		{"123abc", "_123abc"},
		{"user-profile", "user_profile"},
		{"", "_"},
		{"user.name", "user_name"},
	}
	for _, tc := range tests {
		got := SafeIdent(tc.input)
		if got != tc.want {
			t.Errorf("SafeIdent(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDetectPhase(t *testing.T) {
	base := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	// Adding a nullable field → schema_only
	addNullable := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":  {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
		},
	}
	diff, err := definition.Diff(base, addNullable)
	require.NoError(t, err)
	require.Equal(t, "schema_only", DetectPhase(diff))

	// Removing a field → full
	removeField := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{},
		},
	}
	diff, err = definition.Diff(base, removeField)
	require.NoError(t, err)
	require.Equal(t, "full", DetectPhase(diff))

	// Renaming a field → full
	renameField := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"name": {Name: "fullName", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}
	diff, err = definition.Diff(base, renameField)
	require.NoError(t, err)
	require.Equal(t, "full", DetectPhase(diff))

	// Adding a required field → full
	addRequired := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"name": {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"age":  {Name: "age", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
		},
	}
	diff, err = definition.Diff(base, addRequired)
	require.NoError(t, err)
	require.Equal(t, "full", DetectPhase(diff))
}

func TestBuildMigrationCode(t *testing.T) {
	f := SchemaFile{
		Path: "schemas/users.json",
		Name: "Users",
		Raw:  []byte(`{"name":"Users","version":"1.1.0","fields":{}}`),
	}
	code := BuildMigrationCode(f, "Users", "1.0.0", "1.1.0", "schema_only", definition.BumpMinor)
	require.Contains(t, code, "Users_1_0_0_to_1_1_0")
	require.Contains(t, code, "definition.BumpMinor")
	// Phase is now resolved at runtime, not hardcoded
	require.NotContains(t, code, "base.PhaseSchemaOnly")
	require.NotContains(t, code, "base.PhaseFull")
	// Transformer stub is only generated for full-phase migrations
	require.NotContains(t, code, "Transformer")
	require.NotContains(t, code, "context")
	require.NotContains(t, code, "core/data")
	require.NotContains(t, code, "implement transformer")

	codeFull := BuildMigrationCode(f, "Users", "1.0.0", "2.0.0", "full", definition.BumpMajor)
	require.Contains(t, codeFull, "definition.BumpMajor")
	require.Contains(t, codeFull, "Transformer")
	require.Contains(t, codeFull, "context")
	require.Contains(t, codeFull, "core/data")
	require.Contains(t, codeFull, "panic")
}

func TestBuildSquashCode(t *testing.T) {
	f := SchemaFile{
		Path: "schemas/users.json",
		Name: "Users",
		Raw:  []byte(`{"name":"Users","version":"1.2.0","fields":{}}`),
	}
	code := BuildSquashCode(f, "Users", "1.0.0", "1.2.0", "schema_only", definition.BumpMinor, nil)
	require.Contains(t, code, "squashed migration")
	require.Contains(t, code, "Users_1_0_0_to_1_2_0")
	require.Contains(t, code, "target_Users_1_2_0_squash")
	require.NotContains(t, code, "Transformer")

	codeFull := BuildSquashCode(f, "Users", "1.0.0", "2.0.0", "full", definition.BumpMajor,
		[]string{"Users_1_0_0_to_1_1_0", "Users_1_1_0_to_2_0_0"})
	require.Contains(t, codeFull, "Users_1_0_0_to_1_1_0")
	require.Contains(t, codeFull, "Users_1_1_0_to_2_0_0")
	require.Contains(t, codeFull, "Transformer")
	require.Contains(t, codeFull, "fmt")
}

func TestGenerateRegistry(t *testing.T) {
	lock := &Lockfile{
		Version: "1",
		Schemas: map[string]*SchemaRef{
			"Users": {
				Version:       "1.2.0",
				MigrationFile: "20260701_squash_Users_1_0_0_to_1_2_0.go",
				History: []*HistoryEntry{
					{Version: "1.0.0"},
				},
				SubMigrations: []string{
					"20260701_Users_minor.go",
					"20260701_Users_major.go",
				},
			},
		},
	}

	dir := t.TempDir()
	err := GenerateRegistry(lock, dir)
	require.NoError(t, err)

	data, err := os.ReadFile(dir + "/registry.go")
	require.NoError(t, err)
	content := string(data)
	require.Contains(t, content, "Plain")
	require.Contains(t, content, "Squash")
	require.Contains(t, content, "SubMigrations")
	require.Contains(t, content, "20260701_squash_Users_1_0_0_to_1_2_0.go")
}

func TestGenerateFirstMigration(t *testing.T) {
	// New collection, no history → should generate from "0.0.0"
	lock := &Lockfile{
		Version: "1",
		Schemas: map[string]*SchemaRef{
			"Carts": {
				Version:       "1.0.0",
				MigrationFile: "20260701_Carts_major.go",
			},
		},
	}
	dir := t.TempDir()
	err := GenerateRegistry(lock, dir)
	require.NoError(t, err)
}

func TestJsonLiteral(t *testing.T) {
	raw := []byte(`{"name":"test"}`)
	got := jsonLiteral(raw)
	require.Equal(t, "[]byte(`{\"name\":\"test\"}`)", got)
}
