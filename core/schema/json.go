package schema

import (
	"embed"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
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
			compileErr = common.SystemErrorFrom(err, "ERR_SCHEMA_META_SCHEMA_READ_FAILED").
				WithMessage("failed to read embedded meta-schema file 'definition.json'")
			return
		}

		metaCompiler, err = json.NewCompiler(schema)
		if err != nil {
			compileErr = common.SystemErrorFrom(err, "ERR_SCHEMA_META_SCHEMA_COMPILE_FAILED").
				WithMessage("failed to create compiler for meta-schema")
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
		return common.NewSystemError(
			"ERR_SCHEMA_EMPTY_INPUT",
			"schema definition JSON cannot be empty",
		).WithOperation(op)
	}

	// 2. Load compiled meta-schema
	compiler, err := getMetaCompiler()
	if err != nil {
		// Wrap the cached error with the current operation context
		return common.SystemErrorFrom(err, "ERR_SCHEMA_INTERNAL_INIT_FAILED").
			WithOperation(op)
	}

	// 3. Validate input against meta-schema rules
	if err := compiler.Validate(jsonSchema); err != nil {
		return common.SystemErrorFrom(err, "ERR_SCHEMA_VALIDATION_FAILED").
			WithOperation(op).
			WithMessage("failed to validate input against the required schema structure")
	}

	// 4. Unmarshal into the Go struct
	if err := utils.FromJSON(jsonSchema, s); err != nil {
		return common.SystemErrorFrom(err, "ERR_SCHEMA_UNMARSHAL_FAILED").
			WithOperation(op).
			WithMessage("failed to map JSON to SchemaDefinition structure")
	}

	return nil
}
