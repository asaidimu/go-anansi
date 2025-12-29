package schema

import (
	"embed"
	"fmt"
	"io/fs"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/schema"
)

//go:embed models/*.json
var schemasFS embed.FS

var (
	schemas     []schema.SchemaDefinition
	schemasOnce sync.Once
	schemasErr  error
)

// GetSchemas returns all schema definitions, loading them from the embedded filesystem on the first call.
func GetSchemas() ([]schema.SchemaDefinition, error) {
	schemasOnce.Do(func() {
		loadedSchemas := []schema.SchemaDefinition{}
		err := walkSchemas(func(def schema.SchemaDefinition) error {
			loadedSchemas = append(loadedSchemas, def)
			return nil
		})

		if err != nil {
			schemasErr = err
			return
		}
		schemas = loadedSchemas
	})

	return schemas, schemasErr
}

func walkSchemas(callback func(schema.SchemaDefinition) error) error {
	dir, err := schemasFS.ReadDir("models")
	if err != nil {
		return err
	}

	for _, entry := range dir {
		if !entry.Type().IsRegular() {
			continue
		}
		name := fmt.Sprintf("models/%s", entry.Name())
		var schemaDef schema.SchemaDefinition
		bytes, err := fs.ReadFile(schemasFS, name)

		if err != nil {
			return fmt.Errorf("failed to read %s: %w", name, err)
		}
		if err := schemaDef.From(bytes); err != nil {
			return fmt.Errorf("failed to unmarshal schema in %s: %w", name, err)
		}

		def := &schemaDef
		if err := def.Validate(); err != nil {
			return fmt.Errorf("failed to validate schema in %s: %w", name, err)
		}
		if err := callback(schemaDef); err != nil {
			return err
		}
	}

	return nil
}
