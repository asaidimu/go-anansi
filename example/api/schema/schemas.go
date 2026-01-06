package schema

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/asaidimu/go-anansi/v6/core/schema"
)

//go:embed schemas/*.json
var schemasFS embed.FS

// SchemaLoader handles loading and providing schema definitions.
type SchemaLoader struct {
	Schemas []*schema.SchemaDefinition
}

// NewSchemaLoader loads schema definitions from embedded JSON files.
func NewSchemaLoader() (*SchemaLoader, error) {
	userSchemaBytes, err := fs.ReadFile(schemasFS, "schemas/user.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read user.json: %w", err)
	}
	userSchemaDef, err := schema.From(userSchemaBytes);
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal user.json: %w", err)
	}

	documentSchemaBytes, err := fs.ReadFile(schemasFS, "schemas/document.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read document.json: %w", err)
	}
	 documentSchemaDef, err := schema.From(documentSchemaBytes);
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal document.json: %w", err)
	}

	return &SchemaLoader{
		Schemas: []*schema.SchemaDefinition{
			userSchemaDef,
			documentSchemaDef,
		},
	}, nil
}
