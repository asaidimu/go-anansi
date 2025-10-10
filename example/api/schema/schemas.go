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
	Schemas []schema.SchemaDefinition
}

// NewSchemaLoader loads schema definitions from embedded JSON files.
func NewSchemaLoader() (*SchemaLoader, error) {
	var userSchemaDef schema.SchemaDefinition
	userSchemaBytes, err := fs.ReadFile(schemasFS, "schemas/user.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read user.json: %w", err)
	}
	if err := userSchemaDef.From(userSchemaBytes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user.json: %w", err)
	}

	var documentSchemaDef schema.SchemaDefinition
	documentSchemaBytes, err := fs.ReadFile(schemasFS, "schemas/document.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read document.json: %w", err)
	}
	if err := documentSchemaDef.From(documentSchemaBytes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal document.json: %w", err)
	}

	return &SchemaLoader{
		Schemas: []schema.SchemaDefinition{
			userSchemaDef,
			documentSchemaDef,
		},
	}, nil
}
