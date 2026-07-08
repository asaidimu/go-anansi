package schemagen

import (
	"fmt"
	"os"

	"github.com/asaidimu/go-anansi/v7/codegen/typescript"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/bmatcuk/doublestar/v4"
)

func RunTSGen(cfg *Config, dryRun bool) error {
	matches, err := doublestar.FilepathGlob(cfg.Schema.Glob)
	if err != nil {
		return fmt.Errorf("glob pattern %q: %w", cfg.Schema.Glob, err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("no schema files matching %q", cfg.Schema.Glob)
	}

	var schemas []*definition.Schema
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			return fmt.Errorf("read %s: %w", m, err)
		}
		s, err := definition.FromJSON(raw)
		if err != nil {
			return fmt.Errorf("parse %s: %w", m, err)
		}
		schemas = append(schemas, s)
	}

	ts := typescript.GenerateCombined(schemas)

	if dryRun {
		fmt.Printf("would generate %s (%d types from %d schemas)\n", cfg.TSGen.Out, len(schemas), len(schemas))
		return nil
	}

	if err := os.WriteFile(cfg.TSGen.Out, []byte(ts), 0644); err != nil {
		return fmt.Errorf("write %s: %w", cfg.TSGen.Out, err)
	}

	fmt.Printf("generated %s\n", cfg.TSGen.Out)
	return nil
}
