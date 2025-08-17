package registry

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// generatePhysicalName creates a database-safe identifier from schema name and version
// with a maximum length of 24 characters, suitable for SQL tables and NoSQL collections
func generatePhysicalName(s *schema.SchemaDefinition) (string, error) {
	// Validate inputs
	if s.Name == "" {
		return "", fmt.Errorf("schema name cannot be empty")
	}
	if s.Version == "" {
		return "", fmt.Errorf("schema version cannot be empty")
	}

	// Validate semantic version format (basic check)
	semVerPattern := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	if !semVerPattern.MatchString(s.Version) {
		return "", fmt.Errorf("version must follow semantic versioning format (x.y.z)")
	}

	// Sanitize name: keep only alphanumeric and convert to lowercase
	sanitizedName := sanitizeForDatabase(s.Name)
	if sanitizedName == "" {
		return "", fmt.Errorf("schema name contains no valid characters")
	}

	// Sanitize version: replace dots with underscores
	sanitizedVersion := strings.ReplaceAll(s.Version, ".", "_")

	// Ensure name starts with letter (database requirement)
	if !regexp.MustCompile(`^[a-zA-Z]`).MatchString(sanitizedName) {
		sanitizedName = "t_" + sanitizedName
	}

	// Calculate available space for truncation
	const maxLength = 24
	const separator = "_"
	separatorLength := len(separator)
	versionLength := len(sanitizedVersion)

	// Reserve space for version and separator
	maxNameLength := maxLength - versionLength - separatorLength

	if maxNameLength < 1 {
		return "", fmt.Errorf("version too long to fit in %d character limit", maxLength)
	}

	// Truncate name if necessary
	if len(sanitizedName) > maxNameLength {
		sanitizedName = sanitizedName[:maxNameLength]
	}

	// Combine name and version
	physicalName := fmt.Sprintf("%s%s%s", sanitizedName, separator, sanitizedVersion)

	// Final validation
	if len(physicalName) > maxLength {
		return "", fmt.Errorf("generated name exceeds %d character limit", maxLength)
	}

	return physicalName, nil
}

// sanitizeForDatabase removes invalid characters and converts to lowercase
func sanitizeForDatabase(input string) string {
	// Convert to lowercase
	input = strings.ToLower(input)

	// Keep only alphanumeric characters and underscores
	reg := regexp.MustCompile(`[^a-z0-9_]`)
	sanitized := reg.ReplaceAllString(input, "")

	// Remove consecutive underscores
	reg = regexp.MustCompile(`_+`)
	sanitized = reg.ReplaceAllString(sanitized, "_")

	// Remove leading/trailing underscores
	sanitized = strings.Trim(sanitized, "_")

	return sanitized
}

func unmarshalEntry(doc data.Document) (*base.RegistryEntry, error) {
	return utils.MapToStruct[*RegistryEntry](doc)
}

func EnrichSchema(sc *schema.SchemaDefinition) *schema.SchemaDefinition {
	if sc == nil {
		return nil
	}

	tempSchema := *sc
	// how is it that this sticks
	tempSchema.Fields[data.MetadataFieldName] = &schema.FieldDefinition{
		Name:   data.MetadataFieldName,
		Type:   schema.FieldTypeObject,
		Schema: schema.NestedSchemaReference{ID: data.MetadataFieldName},
	}

	metadata := collection.DefaultMetadataSchema()

	// and this one does not
	if tempSchema.NestedSchemas == nil {
		tempSchema.NestedSchemas = make(map[string]*schema.NestedSchemaDefinition)
	}

	tempSchema.NestedSchemas[data.MetadataFieldName] = metadata
	return &tempSchema
}
