package schemagen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/schema/meta"
)

// starterMetadataJSON is the declarative user-defined metadata template
// scaffolded alongside a new project. It declares a "trace" provider with two
// fields; the schema-level metadata file drives provider stub generation.
const starterMetadataJSON = `{
  "name": "_metadata_",
  "providers": {
    "trace": {
      "description": "Tracing correlation ids",
      "fields": {
        "trace_id": {
          "name": "trace_id",
          "type": "string"
        },
        "span_id": {
          "name": "span_id",
          "type": "string"
        }
      }
    }
  }
}
`

// scaffoldModuleVersion returns a valid v8 module version for the generated
// go.mod. The CLI may run without a release tag (e.g. `go run ./cmd/anansi`),
// in which case anansiVersion is "dev" and unusable in a require directive.
func scaffoldModuleVersion(anansiVersion string) string {
	if semverV8Re.MatchString(anansiVersion) {
		return anansiVersion
	}
	return defaultAnansiVersion
}

const defaultAnansiVersion = "v8.4.7"

var semverV8Re = regexp.MustCompile(`^v8\.\d+\.\d+(-[0-9A-Za-z.\-]+)?(\+[0-9A-Za-z.\-]+)?$`)

// ScaffoldOptions describes what anansi scaffold should create.
type ScaffoldOptions struct {
	Dir           string
	Library       bool
	DryRun        bool
	AnansiVersion string

	// Layout overrides. Empty values fall back to the defaults: schemas dir
	// "schemas", migrations dir "migrations", lockfile "schemas.lock.json".
	// These are written relative into anansi.json so you can organise the
	// project however you like — the CLI defaults are just defaults.
	SchemasDir    string
	MigrationsDir string
	Lockfile      string
}

func (o ScaffoldOptions) schemasDir() string {
	if o.SchemasDir != "" {
		return strings.Trim(filepath.ToSlash(o.SchemasDir), "/")
	}
	return "schemas"
}

func (o ScaffoldOptions) migrationsDir() string {
	if o.MigrationsDir != "" {
		return strings.Trim(filepath.ToSlash(o.MigrationsDir), "/")
	}
	return "migrations"
}

func (o ScaffoldOptions) lockfile() string {
	if o.Lockfile != "" {
		return filepath.ToSlash(o.Lockfile)
	}
	return "schemas.lock.json"
}

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// writeScaffoldFile writes data to path. In library mode it never overwrites an
// existing file, so scaffolding into an existing project leaves the user's
// files untouched. The boolean reports whether the file was written.
func writeScaffoldFile(path string, data []byte, library bool) (bool, error) {
	if library && fileExists(path) {
		return false, nil
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return false, err
	}
	return true, nil
}

func RunScaffold(opts ScaffoldOptions) error {
	abs, err := filepath.Abs(opts.Dir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	modulePath := filepath.Base(abs)

	schemasRel := opts.schemasDir()
	migrationsRel := opts.migrationsDir()
	lockfileRel := opts.lockfile()

	schemasDir := filepath.Join(abs, schemasRel)
	migrationsDir := filepath.Join(abs, migrationsRel)
	exampleRel := filepath.Join(schemasRel, "example.schema.json")
	examplePath := filepath.Join(abs, exampleRel)
	lockfilePath := filepath.Join(abs, lockfileRel)

	printCreated := func(format string, a ...any) {
		fmt.Printf("  created: "+format+"\n", a...)
	}
	printExists := func(format string, a ...any) {
		fmt.Printf("  using existing: "+format+"\n", a...)
	}

	if opts.DryRun {
		fmt.Printf("would scaffold anansi project in %s\n", abs)
		fmt.Printf("  would create: %s/\n", schemasDir)
		fmt.Printf("  would create: %s/\n", migrationsDir)
		fmt.Printf("  would create: %s/anansi.json\n", abs)
		fmt.Printf("  would create: %s/metadata.schema.json\n", abs)
		fmt.Printf("  would create: %s\n", examplePath)
		fmt.Printf("  would create: %s\n", lockfilePath)
		fmt.Printf("  would create: %s/AGENTS.md\n", abs)
		fmt.Printf("  would create: %s/.agents/skills/anansi/ (bundled skill)\n", abs)
		if !opts.Library {
			fmt.Printf("  would create: %s/go.mod\n", abs)
			fmt.Printf("  would create: %s/main.go\n", abs)
		}
		fmt.Println()
		if opts.Library {
			fmt.Println("after scaffolding, run:")
			fmt.Println("  go get github.com/asaidimu/go-anansi/v8")
			fmt.Println("  anansi migrate generate && anansi codegen golang")
		} else {
			fmt.Println("after scaffolding, run:")
			fmt.Printf("  cd %s && go mod tidy && go run .\n", opts.Dir)
		}
		return nil
	}

	// Standalone scaffolding refuses a non-empty directory; library mode is the
	// opposite — it adds anansi to an existing project — so it only refuses a
	// target that is already initialized.
	if opts.Library {
		if fileExists(filepath.Join(abs, "anansi.json")) {
			return fmt.Errorf("refusing to scaffold into %s: anansi.json already exists (project is initialized)", abs)
		}
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return fmt.Errorf("library mode requires an existing directory %s", abs)
		}
	} else if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
		return fmt.Errorf("refusing to scaffold into non-empty directory %s", abs)
	}

	// Create the project root; schemas/migrations dirs are created lazily only
	// when something is actually written into them, so non-destructive library
	// mode never leaves empty directories behind in an existing project.
	if err := os.MkdirAll(abs, 0755); err != nil {
		return fmt.Errorf("create %s: %w", abs, err)
	}

	// go.mod + main.go only for standalone apps.
	if !opts.Library {
		// go.mod
		gomod := fmt.Sprintf(`module %s

go 1.21

require github.com/asaidimu/go-anansi/v8 %s
`, modulePath, scaffoldModuleVersion(opts.AnansiVersion))
		if err := os.WriteFile(filepath.Join(abs, "go.mod"), []byte(gomod), 0644); err != nil {
			return fmt.Errorf("write go.mod: %w", err)
		}

		// main.go
		migrationsPkg := modulePath
		if rel := strings.Trim(filepath.ToSlash(migrationsRel), "/"); rel != "" {
			migrationsPkg += "/" + rel
		}
		tpl := `package main

import (
	"context"
	"fmt"
	"log"
	"time"

	anansi "github.com/asaidimu/go-anansi/v8"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"MPKG"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		DBPath: "data.db",
	})
	if err != nil {
		log.Fatalf("playground: %v", err)
	}
	defer cleanup()

	if err := migrations.Apply(ctx, p); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}
	fmt.Println("migrations applied — database is ready")

	coll, err := p.Collection(ctx, "Example")
	if err != nil {
		log.Fatalf("get collection: %v", err)
	}

	_, err = coll.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"name": "hello world"}),
	})
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	fmt.Println("created a sample document")

	r, err := coll.Read(ctx, &query.Query{})
	if err != nil {
		log.Fatalf("read: %v", err)
	}
	fmt.Printf("collection %q has %d document(s)\n", "Example", len(r.Data))
}`
		mainSrc := strings.ReplaceAll(tpl, "MPKG", migrationsPkg)
		if err := os.WriteFile(filepath.Join(abs, "main.go"), []byte(mainSrc), 0644); err != nil {
			return fmt.Errorf("write main.go: %w", err)
		}
	}

	// anansi.json — relative paths, so the on-disk config always matches the
	// layout the user chose, not the CWD scaffold ran from.
	cfg := DefaultConfig()
	cfg.Schema.Glob = schemasRel + "/**/*.schema.json"
	cfg.Schema.Lockfile = lockfileRel
	cfg.Schema.MigrationsDir = migrationsRel + "/"
	cfgData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(abs, "anansi.json"), cfgData, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	printCreated("%s", filepath.Join(abs, "anansi.json"))

	// metadata.schema.json — declarative user-defined metadata. Only written
	// when absent: in library mode an existing metadata file is reused as-is,
	// never canonicalized or overwritten.
	metadataPath := filepath.Join(abs, cfg.Metadata.SchemaPath)
	mdExists := fileExists(metadataPath)
	if !mdExists {
		if err := os.WriteFile(metadataPath, []byte(starterMetadataJSON), 0644); err != nil {
			return fmt.Errorf("write metadata schema: %w", err)
		}
		printCreated("%s", metadataPath)
	} else {
		printExists("%s", metadataPath)
	}
	md, mdChanged, err := LoadMetadata(metadataPath)
	if err != nil {
		return err
	}
	if mdChanged && !mdExists {
		if err := writeMetadataFile(metadataPath, md); err != nil {
			return err
		}
	}

	// Example schema + migration + lockfile + registry + generated metadata.
	// This whole unit is generated only when the example schema is absent:
	// an existing project owns its schemas, so scaffold leaves them alone and
	// skips generation (run `anansi migrate generate && anansi codegen golang`
	// to produce those from the project's own schemas).
	if !fileExists(examplePath) {
		// Example schema — parse, set version, normalize, then write to disk
		// so the JSON always includes a version field.
		exampleRaw := `{
  "name": "Example",
  "description": "Example schema — replace with your own",
  "fields": {
    "019f4066-6563-7c55-a6f3-ac8f087d89d1": {
      "name": "name",
      "type": "string",
      "required": true
    }
  }
}`
		schema, err := definition.FromJSON([]byte(exampleRaw))
		if err != nil {
			return fmt.Errorf("parse example schema: %w", err)
		}
		if schema.Version == nil {
			schema.Version = common.MustNewVersion("0.1.0")
		}
		meta.NormalizeSchema(schema)
		enriched, err := data.EnrichSchema(schema, md.MergedSchema(), md.Dependencies())
		if err != nil {
			return fmt.Errorf("enrich example schema: %w", err)
		}
		out, err := json.MarshalIndent(enriched.AsMap(), "", "  ")
		if err != nil {
			return fmt.Errorf("marshal example schema: %w", err)
		}
		out = append(out, '\n')
		exampleJSON := out

		if err := os.MkdirAll(schemasDir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", schemasDir, err)
		}
		if _, err := writeScaffoldFile(examplePath, exampleJSON, opts.Library); err != nil {
			return fmt.Errorf("write example schema: %w", err)
		}
		printCreated("%s", examplePath)

		// Generate migration file
		safeName := SafeIdent("Example")
		exampleVer := schema.Version.String()
		verIdent := strings.ReplaceAll(exampleVer, ".", "_")
		funcName := fmt.Sprintf("%s_0_0_0_to_%s", safeName, verIdent)
		targetFunc := fmt.Sprintf("target_%s_%s", safeName, verIdent)
		fileName := fmt.Sprintf("%s_%s_minor.go",
			strings.ReplaceAll(exampleVer, ".", "_"), safeName)

		migSrc := fmt.Sprintf(`// Code generated by anansi schema migrate. DO NOT EDIT.
// Source: %s
package migrations

import (
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// %[2]s returns a migration plan (0.0.0 → %[3]s).
func %[2]s() *base.MigrationPlan {
	m := &base.MigrationPlan{
		Description: "initial schema",
		Target:      %[4]s(),
		VersionBump: definition.BumpMinor,
	}
	return m
}

var %[4]s_json = []byte(%[5]q)

func %[4]s() *definition.Schema {
	s, err := definition.FromJSON(%[4]s_json)
	if err != nil {
		panic("invalid embedded target schema: " + err.Error())
	}
	return s
}
`, exampleRel, funcName, exampleVer, targetFunc, string(exampleJSON))

		if err := os.MkdirAll(migrationsDir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", migrationsDir, err)
		}
		if err := os.WriteFile(filepath.Join(migrationsDir, fileName), []byte(migSrc), 0644); err != nil {
			return fmt.Errorf("write migration file: %w", err)
		}
		printCreated("%s", filepath.Join(migrationsDir, fileName))

		// Build lockfile
		hash := ContentHash(exampleJSON)
		lock := &Lockfile{
			Version: "1",
			Schemas: map[string]*SchemaRef{
				"Example": {
					Path:          exampleRel,
					Hash:          hash,
					Version:       exampleVer,
					Schema:        enriched,
					MigrationFile: fileName,
				},
			},
		}

		if !opts.Library || !fileExists(lockfilePath) {
			if err := os.MkdirAll(filepath.Dir(lockfilePath), 0755); err != nil {
				return fmt.Errorf("create %s: %w", filepath.Dir(lockfilePath), err)
			}
			if err := WriteLockfile(lockfilePath, lock); err != nil {
				return fmt.Errorf("write lockfile: %w", err)
			}
			printCreated("%s", lockfilePath)
		} else {
			printExists("%s", lockfilePath)
		}

		// Registry + generated metadata (metadata.go). In library mode these
		// are write-once: if either exists, skip generation so we never clobber
		// files the project already owns.
		registryPath := filepath.Join(migrationsDir, "registry.go")
		metadataGoPath := filepath.Join(migrationsDir, "metadata.go")
		if !opts.Library || (!fileExists(registryPath) && !fileExists(metadataGoPath)) {
			if err := GenerateRegistry(lock, migrationsDir); err != nil {
				return fmt.Errorf("generate registry: %w", err)
			}
			genCfg := *cfg
			genCfg.Metadata.OutDir = migrationsDir
			if _, err := GenerateMetadataFiles(&genCfg, md, false); err != nil {
				return fmt.Errorf("generate metadata files: %w", err)
			}
		} else {
			if fileExists(registryPath) {
				printExists("%s", registryPath)
			}
			if fileExists(metadataGoPath) {
				printExists("%s", metadataGoPath)
			}
		}
	} else {
		printExists("%s", examplePath)
	}

	// AGENTS.md + bundled agent skill
	agentsPath := filepath.Join(abs, "AGENTS.md")
	if agentsWritten, err := writeScaffoldFile(agentsPath, []byte(scaffoldAgentsMD), opts.Library); err != nil {
		return fmt.Errorf("write AGENTS.md: %w", err)
	} else if agentsWritten {
		printCreated("%s", agentsPath)
	} else {
		printExists("%s", agentsPath)
	}
	if err := InstallSkill(filepath.Join(abs, ".agents", "skills", "anansi"), false); err != nil {
		return fmt.Errorf("install skill: %w", err)
	}

	fmt.Printf("scaffolded anansi project in %s\n", abs)
	if opts.Library {
		fmt.Println("  existing files were left untouched")
	}
	if !opts.Library {
		fmt.Printf("  created: %s/go.mod, %s/main.go\n", abs, abs)
	}
	fmt.Printf("  layout: schemas=%s/, migrations=%s/, lockfile=%s\n",
		schemasRel, migrationsRel, lockfileRel)
	fmt.Printf("  created: %s/.agents/skills/anansi/ (bundled skill)\n", abs)
	fmt.Println()
	fmt.Println("next steps:")
	if opts.Library {
		fmt.Println("  go get github.com/asaidimu/go-anansi/v8")
		fmt.Println("  anansi migrate generate && anansi codegen golang")
		fmt.Println("  import migrations from your own code and apply on startup")
	} else {
		fmt.Printf("  cd %s && go mod tidy && go run .\n", opts.Dir)
	}
	return nil
}

// scaffoldAgentsMD is the project-local agent reference written by scaffold.
const scaffoldAgentsMD = `# Agents

This project is built on Anansi (schema-driven document persistence).

- **Schemas** live in schemas/; edit the .schema.json files, then run
  anansi migrate generate and anansi codegen golang to keep migrations and
  generated models in sync.
- **Skill**: an Anansi skill is installed at .agents/skills/anansi/ — agent
  tools that support project-scoped skills load it automatically.
- **Testing**: run go test ./...
`
