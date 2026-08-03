package schemagen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/schema/meta"
)

func RunNormalize(cfg *Config, path string, dryRun bool) error {
	md, changed, err := LoadMetadata(cfg.Metadata.SchemaPath)
	if err != nil {
		return err
	}
	if changed && !dryRun {
		if err := writeMetadataFile(cfg.Metadata.SchemaPath, md); err != nil {
			return err
		}
	}

	_, _, err = normalizeSchemaFile(path, md, dryRun)
	return err
}

// normalizeSchemaFile canonicalizes a schema file's IDs, enriches it with the
// merged user-defined metadata and platform system fields, and writes the
// result back to disk when it differs from the on-disk bytes. It returns the
// enriched schema and its canonical JSON. EnrichSchema is idempotent, so
// re-normalizing an already-enriched schema is a no-op.
func normalizeSchemaFile(path string, md *Metadata, dryRun bool) (*definition.Schema, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	s, err := definition.FromJSON(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if s.Version == nil {
		s.Version = common.MustNewVersion("1.0.0")
	}

	meta.NormalizeSchema(s)
	enriched, err := data.EnrichSchema(s, md.MergedSchema(), md.Dependencies())
	if err != nil {
		return nil, nil, fmt.Errorf("enrich %s: %w", path, err)
	}

	out, err := json.MarshalIndent(enriched.AsMap(), "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal normalized schema: %w", err)
	}
	out = append(out, '\n')

	if bytes.Equal(out, raw) {
		return enriched, out, nil
	}

	if dryRun {
		fmt.Printf("  would normalize: %s\n", path)
		return enriched, out, nil
	}

	if err := backupFile(path); err != nil {
		return nil, nil, fmt.Errorf("backup %s: %w", path, err)
	}

	if err := os.WriteFile(path, out, 0644); err != nil {
		return nil, nil, fmt.Errorf("write %s: %w", path, err)
	}

	fmt.Printf("  normalized: %s\n", path)
	return enriched, out, nil
}