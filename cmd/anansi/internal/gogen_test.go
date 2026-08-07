package schemagen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v8/codegen/golang"
	"github.com/stretchr/testify/require"
)

func TestGoGenConfig_UnmarshalMode(t *testing.T) {
	var cfg GoGenConfig
	err := json.Unmarshal([]byte(`{"mode": "structs"}`), &cfg)
	require.NoError(t, err)
	require.Equal(t, golang.ModeStructs, cfg.Mode)

	var cfgFull GoGenConfig
	err = json.Unmarshal([]byte(`{"mode": "full"}`), &cfgFull)
	require.NoError(t, err)
	require.Equal(t, golang.ModeFull, cfgFull.Mode)

	var cfgBad GoGenConfig
	err = json.Unmarshal([]byte(`{"mode": "bogus"}`), &cfgBad)
	require.Error(t, err)
	require.Contains(t, err.Error(), "gogen mode")
}

func TestLoadConfig_GoGenMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anansi.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"schema":{"glob":"schemas/**/*.schema.json"},"gogen":{"mode":"model"}}`), 0644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Equal(t, golang.ModeStructs|golang.ModeModel, cfg.GoGen.Mode)
}

func TestRunGoGen_ModeStructs(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"User","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"email","type":"string","required":true}}}`
	schemaPath := filepath.Join(dir, "user.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	cfg := &Config{
		Schema: SchemaConfig{Glob: filepath.Join(dir, "*.schema.json")},
		GoGen:  GoGenConfig{Mode: golang.ModeStructs},
	}

	require.NoError(t, RunGoGen(cfg, false))

	out, err := os.ReadFile(filepath.Join(dir, "user.schema.model.go"))
	require.NoError(t, err)
	content := string(out)
	require.NotContains(t, content, "DocumentModel", "structs mode must not embed DocumentModel")
	require.NotContains(t, content, "ModelCollection", "structs mode must not emit a collection")
	require.Contains(t, content, "type User struct", "structs mode must emit the root struct")
}

func TestRunGoGen_ModeDefault(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"User","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"email","type":"string","required":true}}}`
	schemaPath := filepath.Join(dir, "user.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schemaContent), 0644))

	cfg := &Config{
		Schema: SchemaConfig{Glob: filepath.Join(dir, "*.schema.json")},
	}

	// Zero mode defaults to ModeFull.
	require.NoError(t, RunGoGen(cfg, false))

	out, err := os.ReadFile(filepath.Join(dir, "user.schema.model.go"))
	require.NoError(t, err)
	content := string(out)
	require.Contains(t, content, "document.DocumentModel")
	require.Contains(t, content, "ModelCollection")
	require.Contains(t, content, "func InitUsersModel")
	require.Contains(t, content, "func DangerouslyResetUsersModel()", "full mode must emit the dangerous singleton reset")
	require.True(t, strings.Contains(content, "const UsersCollectionName = \"User\""), "collection name constant should use the raw schema name")
}

func TestGoGenConfig_UnmarshalEmptyTags(t *testing.T) {
	var cfg GoGenConfig
	err := json.Unmarshal([]byte(`{"tags": []}`), &cfg)
	require.NoError(t, err)
	require.True(t, cfg.TagsSet, "explicit empty tags must be marked as set")
	require.Empty(t, cfg.Tags)

	var absent GoGenConfig
	err = json.Unmarshal([]byte(`{"mode": "full"}`), &absent)
	require.NoError(t, err)
	require.False(t, absent.TagsSet, "missing tags key must not be marked as set")

	var custom GoGenConfig
	err = json.Unmarshal([]byte(`{"tags": [{"Key": "json", "Property": "name", "OmitEmpty": true}]}`), &custom)
	require.NoError(t, err)
	require.True(t, custom.TagsSet)
	require.Len(t, custom.Tags, 1)
	require.Equal(t, "json", custom.Tags[0].Key)
}

func TestLoadConfig_ExplicitEmptyTags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anansi.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"schema":{"glob":"schemas/**/*.schema.json"},"gogen":{"tags":[]}}`), 0644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.True(t, cfg.GoGen.TagsSet)
	require.Empty(t, cfg.GoGen.Tags)
}

func TestLoadConfig_DefaultTags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anansi.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"schema":{"glob":"schemas/**/*.schema.json"}}`), 0644))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.False(t, cfg.GoGen.TagsSet, "absent gogen section must leave Tags unset")
	require.Nil(t, cfg.GoGen.Tags)
}

func TestRunGoGen_NoTags(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"User","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"email","type":"string","required":true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "user.schema.json"), []byte(schemaContent), 0644))

	cfg := &Config{
		Schema: SchemaConfig{Glob: filepath.Join(dir, "*.schema.json")},
		GoGen:  GoGenConfig{Mode: golang.ModeStructs, TagsSet: true},
	}

	require.NoError(t, RunGoGen(cfg, false))

	out, err := os.ReadFile(filepath.Join(dir, "user.schema.model.go"))
	require.NoError(t, err)
	content := string(out)
	require.NotContains(t, content, "anansi:", "no-tags must omit anansi tags")
	require.NotContains(t, content, "json:", "no-tags must omit json tags")
	require.Contains(t, content, "Email string", "field must be emitted without any struct tag")
}

func TestRunGoGen_Scoped(t *testing.T) {
	dir := t.TempDir()
	schemaContent := `{"name":"User","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"email","type":"string","required":true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "user.schema.json"), []byte(schemaContent), 0644))

	cfg := &Config{
		Schema: SchemaConfig{Glob: filepath.Join(dir, "*.schema.json")},
		GoGen:  GoGenConfig{ScopedPackages: true},
	}
	require.NoError(t, RunGoGen(cfg, false))

	out, err := os.ReadFile(filepath.Join(dir, "user.schema.model.go"))
	require.NoError(t, err)
	content := string(out)
	require.Contains(t, content, "func InitModel(", "scoped mode must emit unexported-prefix init")
	require.Contains(t, content, "func DangerouslyResetModel()", "scoped mode must emit the dangerous singleton reset")
	require.NotContains(t, content, "InitUsersModel", "scoped mode must not use the collection-name prefix")
}

func TestRunGoGen_MultiGlob(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "user.schema.json"), []byte(`{"name":"User","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"email","type":"string","required":true}}}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "order.schema.json"), []byte(`{"name":"Order","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d2":{"name":"total","type":"integer","required":true}}}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte(`ignored`), 0644))

	cfg := &Config{
		Schema: SchemaConfig{Glob: filepath.Join(dir, "*.txt")},
		GoGen:  GoGenConfig{Mode: golang.ModeStructs},
	}

	// Positional globs override the config glob entirely.
	require.NoError(t, RunGoGen(cfg, false,
		filepath.Join(dir, "user.schema.json"),
		filepath.Join(dir, "order.schema.json"),
	))

	_, err := os.Stat(filepath.Join(dir, "user.schema.model.go"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "order.schema.model.go"))
	require.NoError(t, err)
}

func TestRunGoGen_DryRunNoTags(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "user.schema.json"), []byte(`{"name":"User","fields":{"019d7775-6563-7c55-a6f3-ac8f087d89d1":{"name":"email","type":"string","required":true}}}`), 0644))

	cfg := &Config{
		Schema: SchemaConfig{Glob: filepath.Join(dir, "*.schema.json")},
		GoGen:  GoGenConfig{TagsSet: true},
	}
	require.NoError(t, RunGoGen(cfg, true))

	_, err := os.Stat(filepath.Join(dir, "user.schema.model.go"))
	require.True(t, os.IsNotExist(err), "dry-run must not write any files")
}

// metadataSchema builds a post-normalization schema (reserved _id_/_metadata_
// fields plus the _metadata_ nested schema) for the given collection name and
// a single user field.
func metadataSchema(name, fieldName string) string {
	return `{
  "name": "` + name + `",
  "fields": {
    "sys_id": {"name": "_id_", "type": "string", "required": true, "unique": true},
    "sys_meta": {"name": "_metadata_", "type": "object", "schema": {"id": "meta"}},
    "uf": {"name": "` + fieldName + `", "type": "string", "required": true}
  },
  "schemas": {
    "meta": {
      "name": "_metadata_",
      "fields": {
        "v": {"name": "version", "type": "number", "required": true},
        "u": {"name": "updated", "type": "string", "required": true},
        "c": {"name": "checksum", "type": "string", "required": true},
        "s": {"name": "signature", "type": "string"},
        "cr": {"name": "created", "type": "string", "required": true}
      }
    }
  }
}`
}

func TestRunGoGen_MetadataTypePerSchema(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.schema.json"), []byte(metadataSchema("Alpha", "email")), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "beta.schema.json"), []byte(metadataSchema("Beta", "sku")), 0644))

	cfg := &Config{
		Schema: SchemaConfig{Glob: filepath.Join(dir, "*.schema.json")},
	}
	require.NoError(t, RunGoGen(cfg, false))

	alpha, err := os.ReadFile(filepath.Join(dir, "alpha.schema.model.go"))
	require.NoError(t, err)
	beta, err := os.ReadFile(filepath.Join(dir, "beta.schema.model.go"))
	require.NoError(t, err)

	a, b := string(alpha), string(beta)
	// Each schema in the package declares its own collection-scoped metadata
	// type, so there is no duplicate-declaration collision and no mangled name.
	require.Contains(t, a, "type AlphaMetadata struct {")
	require.Contains(t, b, "type BetaMetadata struct {")
	require.Contains(t, a, "Metadata *AlphaMetadata `anansi:\"_metadata_,required=false\" json:\"_metadata_,omitempty\"`")
	require.Contains(t, b, "Metadata *BetaMetadata `anansi:\"_metadata_,required=false\" json:\"_metadata_,omitempty\"`")
	require.NotContains(t, a, "type Metadata struct", "metadata type must be scoped to its collection")
	require.NotContains(t, b, "type Metadata struct", "metadata type must be scoped to its collection")
	require.NotContains(t, a, "Metadata_", "no mangled metadata type name")
	require.NotContains(t, b, "Metadata_", "no mangled metadata type name")
}
