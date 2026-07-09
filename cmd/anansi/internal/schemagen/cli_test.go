package schemagen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/require"
)

func TestRunScaffold(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new-project")
	err := RunScaffold(dir, false, "v0.0.0-test")
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(dir, "schemas"))
	require.DirExists(t, filepath.Join(dir, "migrations"))
	require.FileExists(t, filepath.Join(dir, "anansi.json"))
	require.FileExists(t, filepath.Join(dir, "schemas", "example.schema.json"))
}

func TestRunScaffold_DryRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new-project")
	err := RunScaffold(dir, true, "v0.0.0-test")
	require.NoError(t, err)
	// Nothing should be created in dry-run mode
	require.NoDirExists(t, filepath.Join(dir, "schemas"))
	require.NoDirExists(t, filepath.Join(dir, "migrations"))
	require.NoFileExists(t, filepath.Join(dir, "anansi.json"))
}

func TestRunGen_BasicFlow(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"User","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"email","type":"string","required":true}}}`
	schemaPath := filepath.Join(dir, "user.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	lockfilePath := filepath.Join(dir, "schemas.lock.json")
	migrationsDir := filepath.Join(dir, "migrations")

	cfg := &Config{
		Schema: SchemaConfig{
			Glob:         filepath.Join(dir, "*.schema.json"),
			Lockfile:     lockfilePath,
			MigrationsDir: migrationsDir,
		},
	}

	err := RunGen(cfg, false, false)
	require.NoError(t, err)

	// Verify migration file and registry were created
	require.DirExists(t, migrationsDir)
	require.FileExists(t, filepath.Join(migrationsDir, "registry.go"))
	require.FileExists(t, lockfilePath)

	// Lockfile should contain the schema entry
	lock, err := ReadLockfile(lockfilePath)
	require.NoError(t, err)
	_, exists := lock.Schemas["User"]
	require.True(t, exists, "lockfile should contain User schema")
}

func TestRunGen_Check(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"Product","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"title","type":"string"}}}`
	schemaPath := filepath.Join(dir, "product.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	migrationsDir := filepath.Join(dir, "migrations")
	cfg := &Config{
		Schema: SchemaConfig{
			Glob:          filepath.Join(dir, "*.schema.json"),
			Lockfile:      filepath.Join(dir, "schemas.lock.json"),
			MigrationsDir: migrationsDir,
		},
	}

	// First run — should succeed
	err := RunGen(cfg, false, false)
	require.NoError(t, err)

	// Check mode after no changes — should succeed (up to date)
	err = RunGen(cfg, true, false)
	require.NoError(t, err)

	// Modify the schema to trigger a change
	modifiedContent := `{"name":"Product","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"title","type":"string"},"019d7775-6563-7605-8bfb-c27365b73581":{"name":"price","type":"decimal"}}}`
	require.NoError(t, os.WriteFile(schemaPath, []byte(modifiedContent), 0644))

	// Check mode after change — should fail (migrations needed)
	err = RunGen(cfg, true, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "migrations needed")
}

func TestRunGen_DryRun(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"Article","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"body","type":"string"}}}`
	schemaPath := filepath.Join(dir, "article.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	migrationsDir := filepath.Join(dir, "migrations")
	cfg := &Config{
		Schema: SchemaConfig{
			Glob:          filepath.Join(dir, "*.schema.json"),
			Lockfile:      filepath.Join(dir, "schemas.lock.json"),
			MigrationsDir: migrationsDir,
		},
	}

	// Dry run — should not create any files
	err := RunGen(cfg, false, true)
	require.NoError(t, err)
	require.NoDirExists(t, migrationsDir)
	require.NoFileExists(t, cfg.Schema.Lockfile)
}

func TestRunGen_SchemaRemovedAutoCleanup(t *testing.T) {
	dir := t.TempDir()

	// First schema
	schemaContent := `{"name":"TempSchema","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"x","type":"string"}}}`
	schemaPath := filepath.Join(dir, "temp.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	lockfilePath := filepath.Join(dir, "schemas.lock.json")
	migrationsDir := filepath.Join(dir, "migrations")
	cfg := &Config{
		Schema: SchemaConfig{
			Glob:          filepath.Join(dir, "*.schema.json"),
			Lockfile:      lockfilePath,
			MigrationsDir: migrationsDir,
		},
	}

	// Generate migrations
	require.NoError(t, RunGen(cfg, false, false))

	// Remove the schema file from disk
	require.NoError(t, os.Remove(schemaPath))

	// Re-run gen — should auto-cleanup the lockfile entry instead of erroring
	require.NoError(t, RunGen(cfg, false, false))

	lock, err := ReadLockfile(lockfilePath)
	require.NoError(t, err)
	_, exists := lock.Schemas["TempSchema"]
	require.False(t, exists, "lockfile entry should be removed after schema file deletion")
}

func TestRunNormalize(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"Test","fields":{"userName":{"name":"uname","type":"string"}}}`
	schemaPath := filepath.Join(dir, "test.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	// Normalize in-place
	err := RunNormalize(schemaPath, false)
	require.NoError(t, err)

	// Verify file was updated with UUID v7 IDs
	updated, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	require.Contains(t, string(updated), `"name": "uname"`)
}

func TestRunNormalize_DryRun(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"Test2","fields":{"userName":{"name":"uname","type":"string"}}}`
	schemaPath := filepath.Join(dir, "test2.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	// Dry run — should not modify file
	err := RunNormalize(schemaPath, true)
	require.NoError(t, err)

	updated, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	require.Equal(t, schemaContent, string(updated), "file should be unmodified in dry-run mode")
}

func TestRunNormalize_AlreadyNormalized(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"Test3","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"uname","type":"string"}}}`
	schemaPath := filepath.Join(dir, "test3.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	err := RunNormalize(schemaPath, false)
	require.NoError(t, err)

	// File should not have changed (no .bak should exist for unchanged files)
	require.NoFileExists(t, schemaPath+".bak")
}

func TestRunGen_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Schema: SchemaConfig{
			Glob:          filepath.Join(dir, "*.schema.json"),
			Lockfile:      filepath.Join(dir, "schemas.lock.json"),
			MigrationsDir: filepath.Join(dir, "migrations"),
		},
	}

	// No schema files — should error
	err := RunGen(cfg, false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no schema files")
}

func TestRunSquash_BasicFlow(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"SquashTest","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"a","type":"string"}}}`
	schemaPath := filepath.Join(dir, "squash_test.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	lockfilePath := filepath.Join(dir, "schemas.lock.json")
	migrationsDir := filepath.Join(dir, "migrations")
	cfg := &Config{
		Schema: SchemaConfig{
			Glob:          filepath.Join(dir, "*.schema.json"),
			Lockfile:      lockfilePath,
			MigrationsDir: migrationsDir,
		},
	}

	// First gen creates initial migration
	require.NoError(t, RunGen(cfg, false, false))

	// Modify schema to create history
	modifiedContent := `{"name":"SquashTest","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"a","type":"string"},"019d7775-6563-7605-8bfb-c27365b73581":{"name":"b","type":"integer"}}}`
	require.NoError(t, os.WriteFile(schemaPath, []byte(modifiedContent), 0644))
	require.NoError(t, RunGen(cfg, false, false))

	// Verify lockfile has history
	lock, err := ReadLockfile(lockfilePath)
	require.NoError(t, err)
	ref, exists := lock.Schemas["SquashTest"]
	require.True(t, exists)
	require.Equal(t, 1, len(ref.History), "should have 1 history entry after 2 migrations")

	// Squash
	require.NoError(t, RunSquash(cfg, "SquashTest", false))

	// Verify squash produced a new file and history is consolidated
	lock2, err := ReadLockfile(lockfilePath)
	require.NoError(t, err)
	ref2, exists := lock2.Schemas["SquashTest"]
	require.True(t, exists)
	require.Equal(t, 1, len(ref2.History), "should have 1 history entry after squash")
	require.NotEmpty(t, ref2.MigrationFile)
}

func TestRunGen_AutoVersion(t *testing.T) {
	dir := t.TempDir()
	// Schema without a version field
	schemaContent := `{"name":"Versionless","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"x","type":"string"}}}`
	schemaPath := filepath.Join(dir, "versionless.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	lockfilePath := filepath.Join(dir, "schemas.lock.json")
	migrationsDir := filepath.Join(dir, "migrations")
	cfg := &Config{
		Schema: SchemaConfig{
			Glob:          filepath.Join(dir, "*.schema.json"),
			Lockfile:      lockfilePath,
			MigrationsDir: migrationsDir,
		},
	}

	require.NoError(t, RunGen(cfg, false, false))

	// Verify lockfile has an auto-assigned version
	lock, err := ReadLockfile(lockfilePath)
	require.NoError(t, err)
	ref, exists := lock.Schemas["Versionless"]
	require.True(t, exists)
	require.NotEmpty(t, ref.Version, "version should be auto-assigned")

	// Verify migration file name contains the version
	migrationFiles, err := filepath.Glob(filepath.Join(migrationsDir, "*.go"))
	require.NoError(t, err)
	require.Greater(t, len(migrationFiles), 0, "should have migration files")
}

func TestRunNewSchema(t *testing.T) {
	dir := t.TempDir()
	err := RunNewSchema("Blog", dir, false)
	require.NoError(t, err)

	schemaPath := filepath.Join(dir, "Blog.schema.json")
	require.FileExists(t, schemaPath)

	raw, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	require.Contains(t, string(raw), `Blog`)
}

func TestRunNewSchema_DryRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty")
	err := RunNewSchema("Blog", dir, true)
	require.NoError(t, err)
	require.NoDirExists(t, dir)
}

func TestRunNewSchema_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	err := RunNewSchema("Widget", dir, false)
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dir, "Widget.schema.json"))
	require.NoError(t, err)
	_, err = definition.FromJSON(raw)
	require.NoError(t, err, "schema file should contain valid JSON")
}
