package schemagen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asaidimu/go-anansi/v7/codegen/faker"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

func RunFaker(seed int64, count int, pretty bool, dir string, files []string) error {
	if dir == "" && len(files) == 0 {
		return fmt.Errorf("either --dir or schema file paths are required")
	}

	var schemaFiles []string
	if dir != "" {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(info.Name(), ".schema.json") {
				schemaFiles = append(schemaFiles, path)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("walk dir %s: %w", dir, err)
		}
	} else {
		schemaFiles = files
	}

	if len(schemaFiles) == 0 {
		return fmt.Errorf("no schema files found")
	}

	for _, path := range schemaFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		schema, err := definition.FromJSON(data)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		gen := faker.NewFakeGenerator(schema, seed)
		results := make([]map[string]any, count)
		for i := 0; i < count; i++ {
			results[i] = gen.Generate()
		}

		var out []byte
		if pretty {
			out, err = json.MarshalIndent(results, "", "  ")
		} else {
			out, err = json.Marshal(results)
		}
		if err != nil {
			return fmt.Errorf("marshal %s: %w", path, err)
		}

		if len(schemaFiles) == 1 {
			fmt.Println(string(out))
		} else {
			base := strings.TrimSuffix(filepath.Base(path), ".schema.json")
			fmt.Printf("--- %s ---\n%s\n", base, string(out))
		}
	}

	return nil
}
