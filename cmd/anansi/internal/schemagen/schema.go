package schemagen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/schema"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/schema/meta"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/uuid"
)

type SchemaFile struct {
	Path   string
	Name   string
	Schema *definition.Schema
	Raw    []byte
	Hash   string
}

func RunGen(cfg *Config, check, dryRun bool) error {
	lock, err := ReadLockfile(cfg.Schema.Lockfile)
	if err != nil {
		return fmt.Errorf("read lockfile: %w", err)
	}

	matches, err := doublestar.FilepathGlob(cfg.Schema.Glob)
	if err != nil {
		return fmt.Errorf("glob pattern %q: %w", cfg.Schema.Glob, err)
	}

	if len(matches) == 0 {
		// No schema files found — clean up stale lockfile entries if any.
		lockChanged := false
		for name := range lock.Schemas {
			fmt.Fprintf(os.Stderr, "  warning: schema %q removed from disk, cleaning up lockfile entry\n", name)
			delete(lock.Schemas, name)
			lockChanged = true
		}
		if lockChanged {
			if err := backupFile(cfg.Schema.Lockfile); err != nil {
				return fmt.Errorf("backup lockfile: %w", err)
			}
			if err := WriteLockfile(cfg.Schema.Lockfile, lock); err != nil {
				return fmt.Errorf("update lockfile: %w", err)
			}
			fmt.Println("all schemas are up to date")
			return nil
		}
		return fmt.Errorf("no schema files matching %q", cfg.Schema.Glob)
	}

	var files []SchemaFile
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			return fmt.Errorf("read %s: %w", m, err)
		}

		s, err := definition.FromJSON(raw)
		if err != nil {
			return fmt.Errorf("parse %s: %w", m, err)
		}

		if s.Version == nil {
			s.Version = common.MustNewVersion("1.0.0")
		}

		if meta.NormalizeSchema(s) {
			normalized := s.ToJSON()
			if dryRun {
				fmt.Printf("  would normalize: %s\n", m)
			} else {
				if err := os.WriteFile(m, normalized, 0644); err != nil {
					return fmt.Errorf("write normalized %s: %w", m, err)
				}
				fmt.Printf("  normalized: %s\n", m)
			}
			raw = normalized
		}

		if issues, ok := schema.SchemaValidator().Validate(s.AsMap()); !ok {
			if dryRun {
				fmt.Printf("Schema %s has the following issues:\n %v\n", s.Name, issues)
			} else {
				return fmt.Errorf("schema %s failed validation:\n %v", m, issues)
			}
		}

		files = append(files, SchemaFile{
			Path:   m,
			Name:   s.Name,
			Schema: s,
			Raw:    s.ToJSON(),
			Hash:   ContentHash(raw),
		})
	}

	lockChanged := false
	for name := range lock.Schemas {
		found := false
		for _, f := range files {
			if f.Name == name {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "  warning: schema %q removed from disk, cleaning up lockfile entry\n", name)
			delete(lock.Schemas, name)
			lockChanged = true
		}
	}

	var pending []SchemaFile
	for _, f := range files {
		prev, exists := lock.Schemas[f.Name]
		if !exists || prev.Hash != f.Hash {
			pending = append(pending, f)
		}
	}

	if len(pending) == 0 {
		if lockChanged {
			if err := backupFile(cfg.Schema.Lockfile); err != nil {
				return fmt.Errorf("backup lockfile: %w", err)
			}
			if err := WriteLockfile(cfg.Schema.Lockfile, lock); err != nil {
				return fmt.Errorf("update lockfile: %w", err)
			}
		}
		fmt.Println("all schemas are up to date")
		return nil
	}

	if check {
		return fmt.Errorf("migrations needed; run without --check to generate them")
	}

	if dryRun {
		fmt.Println("would generate migrations for:")
		for _, f := range pending {
			prev := lock.Schemas[f.Name]
			fromVer := "0.0.0"
			if prev != nil && prev.Version != "" {
				fromVer = prev.Version
			}
			fmt.Printf("  %s (%s -> auto)\n", f.Name, fromVer)
		}
		return nil
	}

	for _, f := range pending {
		prev := lock.Schemas[f.Name]
		migFile, newVer, err := GenerateMigration(f, prev, cfg.Schema.MigrationsDir)
		if err != nil {
			return fmt.Errorf("generate migration for %s: %w", f.Name, err)
		}

		ref := &SchemaRef{
			Path:          f.Path,
			Hash:          f.Hash,
			Version:       newVer,
			Schema:        f.Schema,
			MigrationFile: migFile,
		}

		if prev != nil {
			ref.History = append(prev.History, &HistoryEntry{
				Version:       prev.Version,
				Schema:        prev.Schema,
				MigrationFile: prev.MigrationFile,
			})
		}

		lock.Schemas[f.Name] = ref
	}

	if err := backupFile(cfg.Schema.Lockfile); err != nil {
		return fmt.Errorf("backup lockfile: %w", err)
	}

	if err := WriteLockfile(cfg.Schema.Lockfile, lock); err != nil {
		return fmt.Errorf("update lockfile: %w", err)
	}

	if err := GenerateRegistry(lock, cfg.Schema.MigrationsDir); err != nil {
		return fmt.Errorf("generate registry: %w", err)
	}

	fmt.Println("migrations generated successfully")
	return nil
}

func GenerateMigration(f SchemaFile, prev *SchemaRef, outDir string) (migFile, newVersion string, err error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", "", fmt.Errorf("create output dir: %w", err)
	}

	oldSchema := &definition.Schema{}
	if prev != nil && prev.Schema != nil {
		oldSchema = prev.Schema
	}

	diff, err := definition.Diff(oldSchema, f.Schema)
	if err != nil {
		return "", "", fmt.Errorf("compute diff: %w", err)
	}

	bump := definition.VersionImpact(diff)
	phase := DetectPhase(diff)

	fromVer := "0.0.0"
	if prev != nil && prev.Version != "" {
		fromVer = prev.Version
	}

	toVer, err := bumpVersion(fromVer, bump)
	if err != nil {
		return "", "", fmt.Errorf("compute target version: %w", err)
	}

	safeName := SafeIdent(f.Name)
	fileName := fmt.Sprintf("%s_%s_%s.go", uuid.Must(uuid.NewV7()).String(), safeName, bump.String())
	migrationPath := filepath.Join(outDir, fileName)

	code := BuildMigrationCode(f, safeName, fromVer, toVer, phase, bump)

	if err := os.WriteFile(migrationPath, []byte(code), 0644); err != nil {
		return "", "", fmt.Errorf("write migration file: %w", err)
	}

	rel, _ := filepath.Rel(outDir, migrationPath)
	fmt.Printf("  generated: %s\n", migrationPath)
	return rel, toVer, nil
}

func bumpVersion(from string, bump definition.VersionBump) (string, error) {
	if from == "0.0.0" && bump == definition.BumpNone {
		return "0.0.0", nil
	}
	v, err := common.NewVersion(from)
	if err != nil {
		return "", fmt.Errorf("parse version %q: %w", from, err)
	}
	bumped := bump.Apply(*v)
	return bumped.String(), nil
}

func DetectPhase(diff *definition.SchemaDiff) string {
	for _, c := range diff.Changes {
		switch c.Kind {
		case definition.FieldRemoved:
			return "full"
		case definition.FieldModified:
			for _, op := range c.Forward {
				if op.Type != definition.OpSet {
					continue
				}
				seg := lastSegmentType(op.Path)
				switch seg {
				case definition.PathName, definition.PathType,
					definition.PathDefault, definition.PathFieldSchema,
					definition.PathUnique, definition.PathRequired,
					definition.PathNullable, definition.PathDeprecated:
					return "full"
				}
			}
		case definition.ConstraintAdded, definition.ConstraintRemoved, definition.ConstraintModified:
			return "full"
		case definition.SchemaAdded, definition.SchemaRemoved, definition.SchemaModified:
			return "full"
		case definition.IndexModified:
			return "full"
		case definition.FieldAdded:
			for _, op := range c.Forward {
				if op.Type == definition.OpAdd {
					if f, ok := op.Value.(definition.Field); ok && f.Required {
						return "full"
					}
				}
			}
		}
	}
	return "schema_only"
}

func lastSegmentType(p definition.Path) definition.PathSegmentType {
	if len(p.Segments) == 0 {
		return definition.PathUnknown
	}
	return p.Segments[len(p.Segments)-1].Type
}

func SafeIdent(name string) string {
	var b strings.Builder
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else if r == '-' || r == '.' || r == ' ' {
			b.WriteRune('_')
		} else if i == 0 {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if result == "" {
		return "_"
	}
	if result[0] >= '0' && result[0] <= '9' {
		result = "_" + result
	}
	return result
}

func BuildMigrationCode(f SchemaFile, safeName, fromVer, toVer, phase string, bump definition.VersionBump) string {
	return renderMigrationTemplate(f, safeName, fromVer, toVer, phase, bump)
}

func renderMigrationTemplate(f SchemaFile, safeName, fromVer, toVer, phase string, bump definition.VersionBump) string {
	bumpRef := "definition.BumpNone"
	switch bump {
	case definition.BumpMajor:
		bumpRef = "definition.BumpMajor"
	case definition.BumpMinor:
		bumpRef = "definition.BumpMinor"
	case definition.BumpPatch:
		bumpRef = "definition.BumpPatch"
	}

	funcName := fmt.Sprintf("%s_%s_to_%s", safeName,
		strings.ReplaceAll(fromVer, ".", "_"),
		strings.ReplaceAll(toVer, ".", "_"))

	desc := fmt.Sprintf("auto migration from %s to %s", fromVer, toVer)

	targetIdent := fmt.Sprintf("target_%s_%s", safeName,
		strings.ReplaceAll(toVer, ".", "_"))

	needsTransformer := phase == "full"
	var importBlock string
	var transformerStub string
	if needsTransformer {
		importBlock = `import (
	"context"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)`
		transformerStub = fmt.Sprintf(`
	m.Transformer = func(ctx context.Context, doc data.Document) (data.Document, error) {
		panic("migrations: %s: implement transformer or remove this line")
	}`, funcName)
	} else {
		importBlock = `import (
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)`
		transformerStub = ""
	}

	funcDesc := fromVer + " \u2192 " + toVer

	code := fmt.Sprintf(`// Code generated by anansi schema migrate. DO NOT EDIT.
// Source: %s
// Date: %s

package migrations

%s

// %s returns a migration plan for %s (%s).
func %s() *base.MigrationPlan {
	desc := "%s"
	m := &base.MigrationPlan{
		Description: desc,
		Target:      %s(),
		VersionBump: %s,
	}
	%s
	return m
}

// %s is the target schema serialized from %s.
var %s_json = %s

func %s() *definition.Schema {
	s, err := definition.FromJSON(%s_json)
	if err != nil {
		panic("invalid embedded target schema: " + err.Error())
	}
	return s
}
`, f.Path, time.Now().UTC().Format(time.RFC3339),
		importBlock, funcName, funcName, funcDesc, funcName, desc, targetIdent, bumpRef, transformerStub,
		targetIdent, f.Path, targetIdent, jsonLiteral(f.Raw), targetIdent, targetIdent)

	return code
}

func renderSquashTemplate(f SchemaFile, safeName, fromVer, toVer, phase string, bump definition.VersionBump, subFuncs []string) string {
	bumpRef := "definition.BumpNone"
	switch bump {
	case definition.BumpMajor:
		bumpRef = "definition.BumpMajor"
	case definition.BumpMinor:
		bumpRef = "definition.BumpMinor"
	case definition.BumpPatch:
		bumpRef = "definition.BumpPatch"
	}

	funcName := fmt.Sprintf("%s_%s_to_%s", safeName,
		strings.ReplaceAll(fromVer, ".", "_"),
		strings.ReplaceAll(toVer, ".", "_"))
	targetIdent := fmt.Sprintf("target_%s_%s_squash", safeName,
		strings.ReplaceAll(toVer, ".", "_"))
	funcDesc := fromVer + " \u2192 " + toVer

	// Build the chained transformer code.
	transformerCode := ""
	importBlock := `import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)`

	if phase == "full" {
		var subCalls []string
		for _, sf := range subFuncs {
			subCalls = append(subCalls, fmt.Sprintf("		%s(),", sf))
		}
		transformerCode = fmt.Sprintf(`
	m.Transformer = func(ctx context.Context, doc data.Document) (data.Document, error) {
		subs := []*base.MigrationPlan{
%s
		}
		for _, sub := range subs {
			if sub.Transformer != nil {
				var err error
				doc, err = sub.Transformer(ctx, doc)
				if err != nil {
					return doc, fmt.Errorf("%%s: %%w", sub.Description, err)
				}
			}
		}
		return doc, nil
	}`, strings.Join(subCalls, "\n"))
	} else {
		importBlock = `import (
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)`
		transformerCode = ""
	}

	code := fmt.Sprintf(`// Code generated by anansi schema migrate. DO NOT EDIT.
// Source: %s
// Date: %s
// This is a squashed migration consolidating multiple intermediate steps.

package migrations

%s

// %s returns a migration plan for %s (%s).
func %s() *base.MigrationPlan {
	desc := "squashed migration from %s to %s"
	m := &base.MigrationPlan{
		Description: desc,
		Target:      %s(),
		VersionBump: %s,
	}
	%s
	return m
}

// %s is the target schema serialized from %s.
var %s_json = %s

func %s() *definition.Schema {
	s, err := definition.FromJSON(%s_json)
	if err != nil {
		panic("invalid embedded target schema: " + err.Error())
	}
	return s
}
`, f.Path, time.Now().UTC().Format(time.RFC3339),
		importBlock, funcName, funcName, funcDesc, funcName, fromVer, toVer, targetIdent, bumpRef, transformerCode,
		targetIdent, f.Path, targetIdent, jsonLiteral(f.Raw), targetIdent, targetIdent)

	return code
}

// RunNewSchema creates a blank schema file with the given name and empty fields.
func RunNewSchema(name, dir string, dryRun bool) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve dir: %w", err)
	}

	json := fmt.Sprintf(`{
  "name": %q,
  "description": "",
  "fields": {}
}`, name)

	s, err := definition.FromJSON([]byte(json))
	if err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}
	if s.Version == nil {
		s.Version = common.MustNewVersion("0.1.0")
	}
	meta.NormalizeSchema(s)
	raw := s.ToJSON()

	filename := filepath.Join(abs, SafeIdent(name)+".schema.json")

	if dryRun {
		fmt.Printf("would create: %s\n", filename)
		return nil
	}

	if err := os.MkdirAll(abs, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", abs, err)
	}

	if err := os.WriteFile(filename, raw, 0644); err != nil {
		return fmt.Errorf("write schema: %w", err)
	}

	fmt.Printf("created: %s\n", filename)
	return nil
}

func jsonLiteral(raw []byte) string {
	var b strings.Builder
	b.WriteString("[]byte(")
	b.WriteString("`")
	escaped := strings.ReplaceAll(string(raw), "`", "` + \"`\" + `")
	b.WriteString(escaped)
	b.WriteString("`)")
	return b.String()
}
