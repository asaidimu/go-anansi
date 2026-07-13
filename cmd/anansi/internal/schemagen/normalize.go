package schemagen

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/schema/meta"
)

func RunNormalize(path string, dryRun bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	s, err := definition.FromJSON(raw)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	changed := meta.NormalizeSchema(s)
	if !changed {
		fmt.Printf("  no changes: %s\n", path)
		return nil
	}

	if dryRun {
		fmt.Printf("  would normalize: %s\n", path)
		return nil
	}

	if err := backupFile(path); err != nil {
		return fmt.Errorf("backup %s: %w", path, err)
	}

	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal normalized schema: %w", err)
	}

	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	fmt.Printf("  normalized: %s\n", path)
	return nil
}


