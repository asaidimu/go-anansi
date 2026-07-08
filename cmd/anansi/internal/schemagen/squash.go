package schemagen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/google/uuid"
)

func RunSquash(cfg *Config, collection string, dryRun bool) error {
	lock, err := ReadLockfile(cfg.Schema.Lockfile)
	if err != nil {
		return fmt.Errorf("read lockfile: %w", err)
	}

	ref, ok := lock.Schemas[collection]
	if !ok {
		return fmt.Errorf("collection %q not found in lockfile", collection)
	}

	if len(ref.History) < 1 {
		return fmt.Errorf("collection %q has no history to squash (only one version exists)", collection)
	}

	from := ref.History[0]
	toVer := ref.Version
	safeName := SafeIdent(collection)
	fromVer := from.Version

	diff, err := definition.Diff(from.Schema, ref.Schema)
	if err != nil {
		return fmt.Errorf("compute consolidated diff: %w", err)
	}

	bump := definition.VersionImpact(diff)
	phase := DetectPhase(diff)

	// Build list of sub-migration function references.
	var subFuncs []string
	for i, h := range ref.History {
		subFrom := h.Version
		var subTo string
		if i+1 < len(ref.History) {
			subTo = ref.History[i+1].Version
		} else {
			subTo = ref.Version
		}
		subFuncs = append(subFuncs, fmt.Sprintf("%s_%s_to_%s", safeName,
			strings.ReplaceAll(subFrom, ".", "_"),
			strings.ReplaceAll(subTo, ".", "_")))
	}

	// Collect sub-migration filenames for the lockfile.
	var subFiles []string
	for _, h := range ref.History {
		if h.MigrationFile != "" {
			subFiles = append(subFiles, h.MigrationFile)
		}
	}
	if ref.MigrationFile != "" {
		subFiles = append(subFiles, ref.MigrationFile)
	}

	if dryRun {
		fmt.Printf("would squash %s: %s -> %s\n", collection, fromVer, toVer)
		fmt.Printf("  would generate: squash migration file\n")
		fmt.Printf("  would update lockfile and regenerate registry\n")
		return nil
	}

	fileName := fmt.Sprintf("%s_squash_%s_%s_to_%s.go", uuid.Must(uuid.NewV7()).String(), safeName,
		strings.ReplaceAll(fromVer, ".", "_"),
		strings.ReplaceAll(toVer, ".", "_"))
	migrationPath := filepath.Join(cfg.Schema.MigrationsDir, fileName)

	raw := ref.Schema.ToJSON()
	sf := SchemaFile{
		Path:   ref.Path,
		Name:   collection,
		Schema: ref.Schema,
		Raw:    raw,
	}

	code := BuildSquashCode(sf, safeName, fromVer, toVer, phase, bump, subFuncs)

	if err := os.WriteFile(migrationPath, []byte(code), 0644); err != nil {
		return fmt.Errorf("write squashed migration: %w", err)
	}
	fmt.Printf("  generated: %s\n", migrationPath)

	ref.History = []*HistoryEntry{
		{Version: fromVer},
	}
	ref.MigrationFile = fileName
	ref.SubMigrations = subFiles

	if err := WriteLockfile(cfg.Schema.Lockfile, lock); err != nil {
		return fmt.Errorf("update lockfile: %w", err)
	}

	if err := GenerateRegistry(lock, cfg.Schema.MigrationsDir); err != nil {
		return fmt.Errorf("regenerate registry: %w", err)
	}

	fmt.Printf("squashed %s: %d intermediate migrations consolidated into %s\n", collection, len(subFuncs), fileName)
	return nil
}

func BuildSquashCode(f SchemaFile, safeName, fromVer, toVer, phase string, bump definition.VersionBump, subFuncs []string) string {
	return renderSquashTemplate(f, safeName, fromVer, toVer, phase, bump, subFuncs)
}
