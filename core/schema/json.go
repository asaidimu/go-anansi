package schema

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed definition.json
var schemasFS embed.FS

var reservedFieldNames = map[string]bool{"id": true, "_metadata_": true}

func (s *SchemaDefinition) From(jsonSchema []byte) error {
	// Validate input
	if len(jsonSchema) == 0 {
		return common.NewSystemError(
			"ERR_SCHEMA_EMPTY_INPUT",
			"schema definition JSON cannot be empty",
		).WithOperation("schema.SchemaDefinition.From")
	}

	// Load and compile meta-schema
	metaSchema, err := s.loadAndCompileMetaSchema()
	if err != nil {
		return err // Already wrapped with proper context
	}

	// Parse input JSON
	var data any
	if err := json.Unmarshal(jsonSchema, &data); err != nil {
		return common.NewSystemError(
			"ERR_SCHEMA_INVALID_JSON",
			fmt.Sprintf("failed to parse schema definition as JSON: %v", err),
		).WithOperation("schema.SchemaDefinition.From").
		WithCause(err)
	}

	// Validate against meta-schema
	if err := s.validateAgainstMetaSchema(metaSchema, data); err != nil {
		return err // Already wrapped with proper context
	}

	// Unmarshal into struct
	if err := utils.FromJSON(jsonSchema, s); err != nil {
		return common.SystemErrorFrom(
			err,
			"ERR_SCHEMA_UNMARSHAL_FAILED",
		).WithOperation("schema.SchemaDefinition.From").
			WithMessage("failed to unmarshal validated JSON into SchemaDefinition structure")
	}

	return nil
}

// loadAndCompileMetaSchema loads the embedded meta-schema and compiles it
func (s *SchemaDefinition) loadAndCompileMetaSchema() (*jsonschema.Schema, error) {
	// Load the embedded meta-schema file
	b, err := schemasFS.ReadFile("definition.json")
	if err != nil {
		return nil, common.SystemErrorFrom(
			err,
			"ERR_SCHEMA_META_SCHEMA_READ_FAILED",
		).WithOperation("schema.SchemaDefinition.loadAndCompileMetaSchema").
			WithMessage("failed to read embedded meta-schema file 'definition.json'")
	}

	// Parse meta-schema JSON
	r, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return nil, common.SystemErrorFrom(
			err,
			"ERR_SCHEMA_META_SCHEMA_PARSE_FAILED",
		).WithOperation("schema.SchemaDefinition.loadAndCompileMetaSchema").
			WithMessage("failed to parse meta-schema JSON structure")
	}

	// Create compiler and add meta-schema resource
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", r); err != nil {
		return nil, common.SystemErrorFrom(
			err,
			"ERR_SCHEMA_META_SCHEMA_RESOURCE_FAILED",
		).WithOperation("schema.SchemaDefinition.loadAndCompileMetaSchema").
			WithMessage("failed to register meta-schema resource with compiler")
	}

	// Compile meta-schema
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, common.SystemErrorFrom(
			err,
			"ERR_SCHEMA_META_SCHEMA_COMPILE_FAILED",
		).WithOperation("schema.SchemaDefinition.loadAndCompileMetaSchema").
			WithMessage("failed to compile meta-schema into validator")
	}

	return schema, nil
}

// validateAgainstMetaSchema validates the input data against the compiled meta-schema
func (s *SchemaDefinition) validateAgainstMetaSchema(metaSchema *jsonschema.Schema, data any) error {
	if err := metaSchema.Validate(data); err != nil {
		validationErr, ok := err.(*jsonschema.ValidationError)
		if !ok {
			// Unexpected validation error type
			return common.SystemErrorFrom(
				err,
				"ERR_SCHEMA_VALIDATION_UNEXPECTED_ERROR",
			).WithOperation("schema.SchemaDefinition.validateAgainstMetaSchema").
				WithMessage("schema validation failed with unexpected error type")
		}

		// Process validation errors into structured issues
		issues := s.processValidationErrors(validationErr)

		return common.NewSystemError(
			"ERR_SCHEMA_VALIDATION_FAILED",
			"provided JSON does not conform to the schema definition meta-schema",
		).WithOperation("schema.SchemaDefinition.validateAgainstMetaSchema").
			WithIssues(issues).
			WithCause(err)
	}

	return nil
}

// processValidationErrors converts jsonschema validation errors into structured issues
func (s *SchemaDefinition) processValidationErrors(validationErr *jsonschema.ValidationError) []common.Issue {
	issues := make([]common.Issue, 0)
	seen := make(map[string]bool) // For deduplication

	// Process BasicOutput errors (flat list)
	for _, ve := range validationErr.BasicOutput().Errors {
		// Skip root summary entries
		if ve.KeywordLocation == "" && ve.Error == nil {
			continue
		}

		// Extract error details
		path := normalizePath(ve.InstanceLocation)
		message := ve.Error.String()
		keywordLocation := ve.KeywordLocation

		// Enhance message for better user experience
		message = improveValidationMessage(message)

		// Special handling for propertyNames errors (empty InstanceLocation)
		if ve.InstanceLocation == "" && strings.Contains(strings.ToLower(message), "propertyname") {
			path = extractPropertyNamesPath(ve.KeywordLocation, message)
		}

		// Handle 'not' failures on reserved field names
		if strings.Contains(message, "'not' failed") && strings.Contains(ve.KeywordLocation, "/name/not") {
			if path != "" {
				path += ".name"
			}
		}

		// Create unique key for deduplication
		key := fmt.Sprintf("%s|%s", path, message)
		if seen[key] {
			continue
		}
		seen[key] = true

		// Build issue with rich context
		issue := common.Issue{
			Code:     "SCHEMA_VALIDATION_ERROR",
			Message:  message,
			Path:     path,
			Severity: "error",
			Description: fmt.Sprintf(
				"JSON Schema validation failed at '%s' for keyword '%s'",
				path,
				keywordLocation,
			),
		}

		// Add additional context for specific error types
		if strings.Contains(message, "reserved") {
			issue.Description = "Field name conflicts with reserved system fields (id, _metadata_)"
		} else if strings.Contains(message, "missing required") {
			issue.Description = "A required property is missing from the schema definition"
		} else if strings.Contains(message, "unknown property") {
			issue.Description = "Schema contains properties not defined in the meta-schema"
		}

		issues = append(issues, issue)
	}

	return issues
}

// normalizePath converts JSON pointer to dot notation
func normalizePath(pointer string) string {
	if pointer == "" || pointer == "/" {
		return ""
	}
	path := strings.ReplaceAll(pointer[1:], "/", ".") // remove leading /
	return path
}

// improveValidationMessage enhances common raw messages
func improveValidationMessage(msg string) string {
	lowerMsg := strings.ToLower(msg)

	if strings.Contains(lowerMsg, "propertyname") {
		re := regexp.MustCompile(`'([^']+)'`)
		matches := re.FindStringSubmatch(msg)
		if len(matches) > 1 {
			badName := matches[1]
			return fmt.Sprintf("reserved field name '%s' not allowed (id and _metadata_ are reserved at root level)", badName)
		}
		return "invalid field name (reserved names: id, _metadata_)"
	}

	if strings.Contains(lowerMsg, "'not' failed") {
		return "field name is reserved (cannot be 'id' or '_metadata_' at root level)"
	}

	if strings.Contains(lowerMsg, "missing property") {
		re := regexp.MustCompile(`'([^']+)'`)
		matches := re.FindStringSubmatch(msg)
		if len(matches) > 1 {
			return fmt.Sprintf("missing required property '%s'", matches[1])
		}
	}

	if strings.Contains(lowerMsg, "additional properties") {
		re := regexp.MustCompile(`'([^']+)'`)
		matches := re.FindStringSubmatch(msg)
		if len(matches) > 1 && strings.ToLower(matches[1]) == "fieds" {
			return "unknown property 'fieds' (did you mean 'fields'?)"
		}
		if len(matches) > 1 {
			return fmt.Sprintf("unknown property '%s' not allowed", matches[1])
		}
	}

	return msg
}

// extractPropertyNamesPath infers path for propertyNames errors
func extractPropertyNamesPath(keywordLocation, msg string) string {
	// keywordLocation e.g., "/properties/fields/propertyNames/not/enum"
	parts := strings.Split(keywordLocation, "/")
	if len(parts) >= 3 {
		objectProp := parts[len(parts)-3] // e.g., "fields"
		path := objectProp

		// Extract bad name from message
		re := regexp.MustCompile(`'([^']+)'`)
		matches := re.FindStringSubmatch(msg)
		if len(matches) > 1 {
			path += "." + matches[1]
		}
		return path
	}
	return "fields"
}
