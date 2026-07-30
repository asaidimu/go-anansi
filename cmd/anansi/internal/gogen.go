package schemagen

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/asaidimu/go-anansi/v8/codegen/golang"
	"github.com/bmatcuk/doublestar/v4"
)

// TODO: extract hardcoded strings into configuration
// For example the extension name on generated files and
// the default package name

func RunGoGen(cfg *Config, dryRun bool) error {
	matches, err := doublestar.FilepathGlob(cfg.Schema.Glob)
	if err != nil {
		return fmt.Errorf("glob pattern %q: %w", cfg.Schema.Glob, err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("no schema files matching %q", cfg.Schema.Glob)
	}

	// Default tags if none specified in cfg.GoGen
	if len(cfg.GoGen.Tags) == 0 {
		cfg.GoGen.Tags = golang.TagConfigFromMap(map[string]string{
			"json":   "name",
			"anansi": "name",
		}, "json")
	}

	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			return fmt.Errorf("read %s: %w", m, err)
		}

		// 1. Determine Go package name from the file's directory
		dir := filepath.Dir(m)
		pkgName := derivePackageName(dir)

		// 2. Output file path alongside schema file (e.g., user.json ->
		// user.model.go)
		ext := filepath.Ext(m)
		outPath := strings.TrimSuffix(m, ext) + ".model.go"

		var sb strings.Builder
		fmt.Fprintf(&sb, "package %s \n", pkgName)

		// 3. Generate Go source code
		gen := golang.NewGoGenerator(&golang.GeneratorConfig{
			TagConfig: cfg.GoGen.Tags,
			ScopedPackages: cfg.GoGen.ScopedPackages,
			NameRules: cfg.GoGen.NameRules,
		})

		result, err := gen.Generate(raw)
		if err != nil {
			return fmt.Errorf("generate %s: %w", m, err)
		}

		fmt.Fprintf(&sb, "%s", result)
		rawCode := sb.String()

		// 4. Format generated Go code using standard go/format
		formattedCode, err := format.Source([]byte(rawCode))
		if err != nil {
			// Fall back to unformatted code if formatting fails to allow syntax debugging
			formattedCode = []byte(rawCode)
		}

		if dryRun {
			fmt.Printf("would generate %s (package %s)\n", outPath, pkgName)
			continue
		}

		if err := os.WriteFile(outPath, formattedCode, 0644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}

		fmt.Printf("generated %s (package %s)\n", outPath, pkgName)
	}

	return nil
}

// DerivePackageName extracts and cleans the parent directory name into a valid Go package identifier.
func derivePackageName(dirPath string) string {
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		absPath = dirPath
	}
	base := filepath.Base(absPath)

	// Clean base string: convert to lowercase and keep only alphanumeric runes
	var buf strings.Builder
	for _, r := range strings.ToLower(base) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			buf.WriteRune(r)
		}
	}

	res := buf.String()
	// Fallback to "main" or "models" if directory name contains no valid identifiers
	if res == "" || res == "." || unicode.IsDigit([]rune(res)[0]) {
		return "model"
	}

	return res
}
