package schemagen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/require"
)

func TestRunScaffold(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new-project")
	err := RunScaffold(ScaffoldOptions{Dir: dir, AnansiVersion: "v0.0.0-test"})
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(dir, "schemas"))
	require.DirExists(t, filepath.Join(dir, "migrations"))
	require.FileExists(t, filepath.Join(dir, "anansi.json"))
	require.FileExists(t, filepath.Join(dir, "schemas", "example.schema.json"))
	require.FileExists(t, filepath.Join(dir, "go.mod"))
	require.FileExists(t, filepath.Join(dir, "main.go"))

	// Scaffold ships the bundled skill and a real (non-empty) AGENTS.md.
	skillMd, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "anansi", "SKILL.md"))
	require.NoError(t, err)
	require.NotEmpty(t, skillMd)
	agentsMd, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	require.NotEmpty(t, agentsMd)
}

func TestRunScaffold_DryRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new-project")
	err := RunScaffold(ScaffoldOptions{Dir: dir, DryRun: true, AnansiVersion: "v0.0.0-test"})
	require.NoError(t, err)
	// Nothing should be created in dry-run mode
	require.NoDirExists(t, filepath.Join(dir, "schemas"))
	require.NoDirExists(t, filepath.Join(dir, "migrations"))
	require.NoFileExists(t, filepath.Join(dir, "anansi.json"))
}

func TestRunScaffold_Library(t *testing.T) {
	// Simulate an existing Go module: non-empty dir with its own go.mod/main.go.
	dir := t.TempDir()
	gomod := "module github.com/acme/backend\n\ngo 1.23\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("// existing app\n"), 0644))

	err := RunScaffold(ScaffoldOptions{Dir: dir, Library: true, AnansiVersion: "v0.0.0-test"})
	require.NoError(t, err)

	// Existing files are untouched.
	gotMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)
	require.Equal(t, gomod, string(gotMod), "library mode must not overwrite the existing go.mod")
	gotMain, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)
	require.Equal(t, "// existing app\n", string(gotMain), "library mode must not overwrite main.go")

	// Anansi pieces are added without a new go.mod/main.go.
	require.FileExists(t, filepath.Join(dir, "anansi.json"))
	require.FileExists(t, filepath.Join(dir, "schemas", "example.schema.json"))
	require.FileExists(t, filepath.Join(dir, "migrations", "registry.go"))
	require.FileExists(t, filepath.Join(dir, ".agents", "skills", "anansi", "SKILL.md"))
}

func TestRunScaffold_Library_RefusesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "anansi.json"), []byte(`{}`), 0644))

	err := RunScaffold(ScaffoldOptions{Dir: dir, Library: true, AnansiVersion: "v0.0.0-test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "anansi.json already exists")
}

func TestRunScaffold_RefusesNonEmptyStandalone(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0644))

	err := RunScaffold(ScaffoldOptions{Dir: dir, AnansiVersion: "v0.0.0-test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-empty directory")
}

func TestRunScaffold_Library_RefusesMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	err := RunScaffold(ScaffoldOptions{Dir: dir, Library: true, AnansiVersion: "v0.0.0-test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "existing directory")
}

func TestRunScaffold_LayoutOverrides(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")

	err := RunScaffold(ScaffoldOptions{
		Dir:           dir,
		AnansiVersion: "v0.0.0-test",
		SchemasDir:    "internal/store/schemas",
		MigrationsDir: "internal/store/migrations",
		Lockfile:      "config/anansi.lock.json",
	})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dir, "internal", "store", "schemas", "example.schema.json"))
	require.FileExists(t, filepath.Join(dir, "internal", "store", "migrations", "registry.go"))
	require.FileExists(t, filepath.Join(dir, "config", "anansi.lock.json"))

	// The on-disk config reflects the chosen layout (relative paths).
	var cfg Config
	cfgData, err := os.ReadFile(filepath.Join(dir, "anansi.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(cfgData, &cfg))
	require.Equal(t, "internal/store/schemas/**/*.schema.json", cfg.Schema.Glob)
	require.Equal(t, "internal/store/migrations/", cfg.Schema.MigrationsDir)
	require.Equal(t, "config/anansi.lock.json", cfg.Schema.Lockfile)
}

func TestRunScaffold_DryRunLayout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	err := RunScaffold(ScaffoldOptions{
		Dir:           dir,
		DryRun:        true,
		AnansiVersion: "v0.0.0-test",
		SchemasDir:    "defs",
	})
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(dir, "defs"))
	require.NoDirExists(t, filepath.Join(dir, "schemas"))
}

func TestRunScaffold_Library_NonDestructive(t *testing.T) {
	// An existing project with its own AGENTS.md, metadata, and schema files.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module acme\n"), 0644))

	userAgents := "# My Project\n\ncustom agent notes\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(userAgents), 0644))

	userMetadata := []byte(`{"name":"_metadata_","providers":{}}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.schema.json"), userMetadata, 0644))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0755))
	userSchema := []byte(`{"name":"Widget","fields":{"019f4066-6563-7c55-a6f3-ac8f087d89d1":{"name":"sku","type":"string"}}}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas", "example.schema.json"), userSchema, 0644))

	err := RunScaffold(ScaffoldOptions{Dir: dir, Library: true, AnansiVersion: "v0.0.0-test"})
	require.NoError(t, err)

	// Existing files are never overwritten, even the ones scaffold would write.
	got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, userAgents, string(got), "must not overwrite an existing AGENTS.md")

	gotMD, err := os.ReadFile(filepath.Join(dir, "metadata.schema.json"))
	require.NoError(t, err)
	require.Equal(t, userMetadata, gotMD, "must not overwrite an existing metadata.schema.json")

	gotSchema, err := os.ReadFile(filepath.Join(dir, "schemas", "example.schema.json"))
	require.NoError(t, err)
	require.Equal(t, userSchema, gotSchema, "must not overwrite an existing example schema")

	// Because the project already owns a schema, scaffold must NOT generate a
	// migration/lockfile/registry from its own example.
	require.NoFileExists(t, filepath.Join(dir, "schemas.lock.json"))
	require.NoDirExists(t, filepath.Join(dir, "migrations"))

	// anansi.json is still added.
	require.FileExists(t, filepath.Join(dir, "anansi.json"))
}

func TestRunGen_BasicFlow(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"User","fields":{"019f4066-6563-7c55-a6f3-ac8f087d89d1":{"name":"email","type":"string","required":true}}}`
	schemaPath := filepath.Join(dir, "user.schema.json")
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
	schemaContent := `{"name":"Product","fields":{"019f4066-6563-7c55-a6f3-ac8f087d89d1":{"name":"title","type":"string"}}}`
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
	modifiedContent := `{"name":"Product","fields":{"019f4066-6563-7c55-a6f3-ac8f087d89d1":{"name":"title","type":"string"},"019f4066-6563-7605-8bfb-c27365b73581":{"name":"price","type":"decimal"}}}`
	require.NoError(t, os.WriteFile(schemaPath, []byte(modifiedContent), 0644))

	// Check mode after change — should fail (migrations needed)
	err = RunGen(cfg, true, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "migrations needed")
}

func TestRunGen_DryRun(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"Article","fields":{"019f4066-6563-7c55-a6f3-ac8f087d89d1":{"name":"body","type":"string"}}}`
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
	schemaContent := `{"name":"TempSchema","fields":{"019f4066-6563-7c55-a6f3-ac8f087d89d1":{"name":"x","type":"string"}}}`
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
	cfg := &Config{
		Schema: SchemaConfig{
			Glob:          filepath.Join(dir, "*.schema.json"),
			Lockfile:      filepath.Join(dir, "schemas.lock.json"),
			MigrationsDir: filepath.Join(dir, "migrations"),
		},
		Metadata: MetadataConfig{SchemaPath: filepath.Join(dir, "metadata.schema.json")},
	}
	err := RunNormalize(cfg, schemaPath, false)
	require.NoError(t, err)

	// Verify file was updated with UUID v7 IDs and system fields
	updated, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	require.Contains(t, string(updated), `"name": "uname"`)
	require.Contains(t, string(updated), `"_id_"`)
	require.Contains(t, string(updated), `"_metadata_"`)
}

func TestRunNormalize_DryRun(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"Test2","fields":{"userName":{"name":"uname","type":"string"}}}`
	schemaPath := filepath.Join(dir, "test2.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	// Dry run — should not modify file
	cfg := &Config{
		Schema: SchemaConfig{
			Glob:          filepath.Join(dir, "*.schema.json"),
			Lockfile:      filepath.Join(dir, "schemas.lock.json"),
			MigrationsDir: filepath.Join(dir, "migrations"),
		},
		Metadata: MetadataConfig{SchemaPath: filepath.Join(dir, "metadata.schema.json")},
	}
	err := RunNormalize(cfg, schemaPath, true)
	require.NoError(t, err)

	updated, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	require.Equal(t, schemaContent, string(updated), "file should be unmodified in dry-run mode")
}

func TestRunNormalize_AlreadyNormalized(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"Test3","fields":{"019f4066-0000-7000-8000-000000000000":{"name":"uname","type":"string"}}}`
	schemaPath := filepath.Join(dir, "test3.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	// First run rewrites the schema to inject the system fields.
	cfg := &Config{
		Schema: SchemaConfig{
			Lockfile:      filepath.Join(dir, "schemas.lock.json"),
			MigrationsDir: filepath.Join(dir, "migrations"),
		},
		Metadata: MetadataConfig{SchemaPath: filepath.Join(dir, "metadata.schema.json")},
	}
	err := RunNormalize(cfg, schemaPath, false)
	require.NoError(t, err)

	enriched, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	require.Contains(t, string(enriched), `"_id_"`)
	require.Contains(t, string(enriched), `"_metadata_"`)

	// A second run is a no-op (idempotent), so no new .bak is created.
	require.NoError(t, os.Remove(schemaPath+".bak"))
	err = RunNormalize(cfg, schemaPath, false)
	require.NoError(t, err)
	after, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	require.Equal(t, enriched, after, "second normalize should be a no-op")
	require.NoFileExists(t, schemaPath+".bak")
}

func TestRunGen_SystemFieldsOnlyTransition(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"EnrichTransition","fields":{"019f4066-0000-7000-8000-000000000000":{"name":"email","type":"string","required":true}}}`
	schemaPath := filepath.Join(dir, "enrich_transition.schema.json")
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

	// Migrate runs the normalize/enrich step first: the on-disk schema is
	// rewritten to include the injected system fields, and the lockfile stores
	// that enriched form.
	require.NoError(t, RunGen(cfg, false, false))

	enriched, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	require.Contains(t, string(enriched), `"_id_"`, "migrate runs normalize first and enriches the on-disk schema")

	lock, err := ReadLockfile(lockfilePath)
	require.NoError(t, err)
	prev := lock.Schemas["EnrichTransition"]
	require.NotNil(t, prev)
	oldHash := prev.Hash
	oldVersion := prev.Version
	migFiles, err := filepath.Glob(filepath.Join(migrationsDir, "*.go"))
	require.NoError(t, err)
	require.Len(t, migFiles, 2, "initial migration + registry.go")
	require.Contains(t, prev.Schema.Fields, definition.FieldId(data.SystemFieldIDDocumentID), "lockfile stores the enriched schema after migrate")

	// A second migrate run is a no-op: enrichment is idempotent, so no new
	// migration is emitted and the version does not change.
	require.NoError(t, RunGen(cfg, false, false))

	migFiles2, err := filepath.Glob(filepath.Join(migrationsDir, "*.go"))
	require.NoError(t, err)
	require.Len(t, migFiles2, 2, "idempotent enrichment must not emit a migration")

	lock2, err := ReadLockfile(lockfilePath)
	require.NoError(t, err)
	ref := lock2.Schemas["EnrichTransition"]
	require.NotNil(t, ref)
	require.Equal(t, oldHash, ref.Hash, "lockfile hash tracked by the enriched on-disk bytes")
	require.Equal(t, oldVersion, ref.Version, "version must not bump for system fields only")
	require.Contains(t, ref.Schema.Fields, definition.FieldId(data.SystemFieldIDDocumentID), "lockfile should store the enriched schema")

	// A third run is fully up to date.
	require.NoError(t, RunGen(cfg, true, false))
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
	schemaContent := `{"name":"SquashTest","fields":{"019f4066-6563-7c55-a6f3-ac8f087d89d1":{"name":"a","type":"string"}}}`
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
	modifiedContent := `{"name":"SquashTest","fields":{"019f4066-6563-7c55-a6f3-ac8f087d89d1":{"name":"a","type":"string"},"019f4066-6563-7605-8bfb-c27365b73581":{"name":"b","type":"integer"}}}`
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
	schemaContent := `{"name":"Versionless","fields":{"019f4066-6563-7c55-a6f3-ac8f087d89d1":{"name":"x","type":"string"}}}`
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
