package schemagen

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed AGENTS.md
var agentsContent string

func RunAgents(dir string, dryRun bool) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	outputPath := filepath.Join(abs, "AGENTS.md")

	if dryRun {
		fmt.Printf("would generate: %s\n", outputPath)
		return nil
	}

	if err := os.MkdirAll(abs, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", abs, err)
	}

	if err := os.WriteFile(outputPath, []byte(agentsContent), 0644); err != nil {
		return fmt.Errorf("write AGENTS.md: %w", err)
	}

	fmt.Printf("created: %s\n", outputPath)
	return nil
}
