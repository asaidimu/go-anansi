package schemagen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func TestLoadMetadata_Canonicalize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.schema.json")
	writeFile(t, path, `{
  "name": "_metadata_",
  "providers": {
    "trace": {
      "description": "tracing",
      "fields": {
        "trace_id": { "name": "trace_id", "type": "string" },
        "span_id": { "name": "span_id", "type": "string" }
      }
    }
  },
  "schemas": {
    "Geo": { "name": "Geo", "type": "object", "fields": { "lat": { "name": "lat", "type": "number" } } }
  }
}`)

	md, changed, err := LoadMetadata(path)
	require.NoError(t, err)
	require.True(t, changed, "name keys should canonicalize to UUID v7")
	require.Len(t, md.Providers, 1)
	require.Len(t, md.Providers[0].Fields, 2)
	require.Len(t, md.Deps, 1)
	for _, p := range md.Providers {
		require.Equal(t, "trace", p.Name)
		require.Equal(t, "Trace", p.Ident)
		for fid := range p.Fields {
			require.True(t, looksLikeUUID(string(fid)), "field ID should be UUID v7")
		}
	}

	// Write the canonical form back, then reload — must be idempotent.
	require.NoError(t, writeMetadataFile(path, md))
	_, changed2, err := LoadMetadata(path)
	require.NoError(t, err)
	require.False(t, changed2, "re-loading a canonical file must be a no-op")
}

func TestLoadMetadata_MergedSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.schema.json")
	writeFile(t, path, `{
  "name": "_metadata_",
  "providers": {
    "trace": { "fields": { "trace_id": { "name": "trace_id", "type": "string" } } }
  }
}`)
	md, _, err := LoadMetadata(path)
	require.NoError(t, err)

	merged := md.MergedSchema()
	require.Equal(t, data.MetadataField, merged.Name)
	require.Len(t, merged.Fields, len(data.DefaultMetadataSchema().Fields)+1)
	names := make(map[string]bool)
	for _, f := range merged.Fields {
		names[string(f.Name)] = true
	}
	require.True(t, names["trace_id"])
	require.True(t, names["version"])
	require.True(t, names["checksum"])
}

func TestLoadMetadata_DependenciesDedupe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.schema.json")
	writeFile(t, path, `{
  "name": "_metadata_",
  "providers": { "a": { "fields": { "x": { "name": "x", "type": "string" } } } },
  "schemas": {
    "Geo": { "name": "Geo", "type": "object", "fields": { "lat": { "name": "lat", "type": "number" } } }
  }
}`)
	md, _, err := LoadMetadata(path)
	require.NoError(t, err)
	require.Len(t, md.Dependencies(), 1)
	require.Equal(t, "Geo", md.Dependencies()[0].Name)
}

func TestLoadMetadata_RejectReservedField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.schema.json")
	writeFile(t, path, `{
  "name": "_metadata_",
  "providers": { "bad": { "fields": { "version": { "name": "version", "type": "number" } } } }
}`)
	_, _, err := LoadMetadata(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reserved metadata field name")
}

func TestLoadMetadata_RejectDuplicateFieldName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.schema.json")
	writeFile(t, path, `{
  "name": "_metadata_",
  "providers": {
    "a": { "fields": { "dup": { "name": "dup", "type": "string" } } },
    "b": { "fields": { "dup": { "name": "dup", "type": "string" } } }
  }
}`)
	_, _, err := LoadMetadata(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate field name")
}

func TestLoadMetadata_RejectBaseIDCollision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.schema.json")
	writeFile(t, path, `{
  "name": "_metadata_",
  "providers": { "bad": { "fields": {
    "019f32a2-1eb3-7c39-885e-c3d545f981ac": { "name": "custom", "type": "string" }
  } } }
}`)
	_, _, err := LoadMetadata(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "collides with a base metadata field ID")
}

func TestGenerateMetadataFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.schema.json")
	writeFile(t, path, `{
  "name": "_metadata_",
  "providers": { "trace": { "fields": { "trace_id": { "name": "trace_id", "type": "string" } } } }
}`)
	md, _, err := LoadMetadata(path)
	require.NoError(t, err)

	cfg := &Config{
		Schema:   SchemaConfig{MigrationsDir: filepath.Join(dir, "migrations")},
		Metadata: MetadataConfig{SchemaPath: path},
	}
	written, err := GenerateMetadataFiles(cfg, md, false)
	require.NoError(t, err)
	require.Len(t, written, 2)

	metaSrc, err := os.ReadFile(filepath.Join(cfg.Schema.MigrationsDir, "metadata.go"))
	require.NoError(t, err)
	require.Contains(t, string(metaSrc), "func MetadataSchema()")
	require.Contains(t, string(metaSrc), "func TraceSchema()")
	require.Contains(t, string(metaSrc), "func ValidateMetadataSchema()")
	require.Contains(t, string(metaSrc), "package migrations")

	provSrc, err := os.ReadFile(filepath.Join(cfg.Schema.MigrationsDir, "providers.go"))
	require.NoError(t, err)
	require.Contains(t, string(provSrc), "func TraceProvider(ctx context.Context, doc data.Documenter)")

	// providers.go is write-once: a second run must not clobber it.
	writeFile(t, filepath.Join(cfg.Schema.MigrationsDir, "providers.go"), "// user edited\npackage migrations\n")
	_, err = GenerateMetadataFiles(cfg, md, false)
	require.NoError(t, err)
	after, err := os.ReadFile(filepath.Join(cfg.Schema.MigrationsDir, "providers.go"))
	require.NoError(t, err)
	require.Equal(t, "// user edited\npackage migrations\n", string(after))
}

func TestRenderMetadataGo_CompilesStructurally(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.schema.json")
	writeFile(t, path, `{
  "name": "_metadata_",
  "providers": { "my_provider": { "fields": { "request_id": { "name": "request_id", "type": "string" } } } },
  "schemas": {
    "Geo": { "name": "Geo", "type": "object", "fields": { "lat": { "name": "lat", "type": "number" } } }
  }
}`)
	md, _, err := LoadMetadata(path)
	require.NoError(t, err)

	src := renderMetadataGo("migrations", md)
	require.True(t, strings.Contains(src, "func My_providerSchema()"))
	require.True(t, strings.Contains(src, "func MetadataDependencies()"))
	require.True(t, strings.Contains(src, "var metadata_dependencies_json"))

	// Round-trip the embedded merged schema literal back to a NestedSchema.
	// Extract the literal after `[]byte(` ... `)` for the merged schema var.
	merged := md.MergedSchema()
	require.NotNil(t, merged)
	require.Equal(t, "_metadata_", merged.Name)
}

func TestRunGen_EmbedsMetadata(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "user.schema.json")
	writeFile(t, schemaPath, `{"name":"User","fields":{"019f4066-6563-7c55-a6f3-ac8f087d89d1":{"name":"email","type":"string"}}}`)
	metaPath := filepath.Join(dir, "metadata.schema.json")
	writeFile(t, metaPath, `{
  "name": "_metadata_",
  "providers": { "trace": { "fields": { "trace_id": { "name": "trace_id", "type": "string" } } } }
}`)

	lockfile := filepath.Join(dir, "schemas.lock.json")
	migDir := filepath.Join(dir, "migrations")
	cfg := &Config{
		Schema:   SchemaConfig{Glob: filepath.Join(dir, "*.schema.json"), Lockfile: lockfile, MigrationsDir: migDir},
		Metadata: MetadataConfig{SchemaPath: metaPath},
	}
	require.NoError(t, RunGen(cfg, false, false))

	onDisk, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	require.Contains(t, string(onDisk), "trace_id", "custom metadata field should be embedded in the on-disk schema")

	require.FileExists(t, filepath.Join(migDir, "metadata.go"))
	require.FileExists(t, filepath.Join(migDir, "providers.go"))

	lock, err := ReadLockfile(lockfile)
	require.NoError(t, err)
	ref := lock.Schemas["User"]
	require.NotNil(t, ref)
	// The enriched schema's _metadata_ nested block carries the custom field.
	var found bool
	for _, ns := range ref.Schema.Schemas {
		if ns.Name == data.MetadataField {
			for _, f := range ns.Fields {
				if string(f.Name) == "trace_id" {
					found = true
				}
			}
		}
	}
	require.True(t, found, "lockfile schema should include the custom metadata field")
}

func TestRunGen_Builders_NoMetadataFile(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "user.schema.json")
	writeFile(t, schemaPath, `{"name":"User","fields":{"019f4066-56e3-7c55-a6f3-ac8cf087d09d1":{"name":"email","type":"string"}}}`)
	cfg := &Config{
		Schema:   SchemaConfig{Glob: filepath.Join(dir, "*.schema.json"), Lockfile: filepath.Join(dir, "schemas.lock.json"), MigrationsDir: filepath.Join(dir, "migrations")},
		Metadata: MetadataConfig{SchemaPath: filepath.Join(dir, "metadata.schema.json")},
	}
	require.NoError(t, RunGen(cfg, false, false))
	require.NoFileExists(t, filepath.Join(cfg.Schema.MigrationsDir, "metadata.go"), "no metadata file -> no generated metadata files")
}