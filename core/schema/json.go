package schema

import (
	"embed"
	"sync"
	"github.com/asaidimu/go-anansi/v6/core/json"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

//go:embed definition.json
var schemasFS embed.FS

var (
	// Singleton instances for meta-schema validation
	metaCompiler *json.Compiler
	compileOnce  sync.Once
	compileErr   error
)


// getMetaCompiler ensures the meta-schema is loaded and compiled only once
// during the application lifecycle, improving performance significantly.
func getMetaCompiler() (*json.Compiler, error) {
	compileOnce.Do(func() {
		schema, err := schemasFS.ReadFile("definition.json")
		if err != nil {
			compileErr = ErrSchemaMetaSchemaReadFailed.WithCause(err)
			return
		}
		metaCompiler, err = json.NewCompiler(schema)
		if err != nil {
			compileErr = ErrSchemaMetaSchemaCompileFailed.WithCause(err)
		}
	})
	return metaCompiler, compileErr
}

// From validates a raw JSON byte slice against the meta-schema and
// unmarshals it into the SchemaDefinition receiver.
func (s *SchemaDefinition) From(jsonSchema []byte) error {
	op := "schema.SchemaDefinition.From"

	// 1. Basic validation
	if len(jsonSchema) == 0 {
		return ErrSchemaEmptyInput.WithOperation(op)
	}

	// 2. Load compiled meta-schema
	compiler, err := getMetaCompiler()
	if err != nil {
		return ErrSchemaInternalInitFailed.
			WithOperation(op).
			WithCause(err)
	}

	// 3. Validate input against meta-schema rules
	if err := compiler.Validate(jsonSchema); err != nil {
		return ErrSchemaValidationFailed.
			WithOperation(op).
			WithCause(err)
	}

	// 4. Unmarshal into the Go struct
	if err := utils.FromJSON(jsonSchema, s); err != nil {
		return ErrSchemaUnmarshalFailed.
			WithOperation(op).
			WithCause(err)
	}

	return nil
}
