package registry

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// generatePhysicalName creates a database-safe identifier from schema name and version
// with a maximum length of 24 characters, suitable for SQL tables and NoSQL collections
func generatePhysicalName(s *schema.SchemaDefinition) (string, error) {
	// Validate inputs
	if s.Name == "" {
		return "", ErrSchemaNameEmpty
	}
	if s.Version == "" {
		return "", ErrSchemaVersionEmpty
	}

	// Validate semantic version format (basic check)
	semVerPattern := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	if !semVerPattern.MatchString(s.Version) {
		return "", ErrInvalidSemanticVersionFormat
	}

	// Sanitize name: keep only alphanumeric and convert to lowercase
	sanitizedName := sanitizeForDatabase(s.Name)
	if sanitizedName == "" {
		return "", ErrSchemaNameInvalidCharacters
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
		return "", common.NewSystemError("ERR_REGISTRY_VERSION_TOO_LONG", fmt.Sprintf("version too long to fit in %d character limit", maxLength))
	}

	// Truncate name if necessary
	if len(sanitizedName) > maxNameLength {
		sanitizedName = sanitizedName[:maxNameLength]
	}

	// Combine name and version
	physicalName := fmt.Sprintf("%s%s%s", sanitizedName, separator, sanitizedVersion)

	// Final validation
	if len(physicalName) > maxLength {
		return "", common.NewSystemError("ERR_REGISTRY_GENERATED_NAME_EXCEEDS_LIMIT", fmt.Sprintf("generated name exceeds %d character limit", maxLength))
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

func unmarshalEntry(doc *data.Document) (*base.RegistryEntry, error) {
	return utils.MapToStruct[*RegistryEntry](doc.ToMap())
}

// EnrichSchema adds system fields (id, metadata) to a schema
// This example shows how to use the new utility methods
func EnrichSchema(sc *schema.SchemaDefinition) (*schema.SchemaDefinition, error) {
	if sc == nil {
		return nil, nil
	}

	// --- Add ID Field ---
	idField := &schema.FieldDefinition{
		Name:     data.DocumentIDField,
		Type:     schema.FieldTypeString,
		Required: utils.BoolPtr(true),
		Unique:   utils.BoolPtr(true),
	}

	// Ensure the ID field exists with exact properties
	// This will add it if missing, or replace it if it exists with different properties
	sc, _, _, err := sc.WithFieldEnsured(idField, nil)
	if err != nil {
		return nil, err
	}

	// --- Remove any user-defined indexes on 'id' field ---
	sc, _, err = sc.WithoutIndexesReferencingField(data.DocumentIDField)
	if err != nil {
		return nil, err
	}

	// --- Add primary key index ---
	pkIndex := &schema.IndexDefinition{
		Name:   "pk_id",
		Fields: []string{data.DocumentIDField},
		Type:   schema.IndexTypePrimary,
		Unique: utils.BoolPtr(true),
	}

	// Ensure the primary key index exists (replaces if already there)
	sc, pkModified, err := sc.WithIndexEnsured(pkIndex)
	if err != nil {
		return nil, err
	}

	_ = pkModified // We know if it was added or replaced

	// --- Add Metadata Field ---
	msd, deps := data.GetMetadataSchema()
	metadataField := &schema.FieldDefinition{
		Name:   data.MetadataField,
		Type:   schema.FieldTypeObject,
		Schema: schema.NestedSchemaReference{ID: *msd.ID},
	}

	// Provider function for nested schemas
	provider := func(sc *schema.SchemaDefinition) (*schema.NestedSchemaDefinition, []*schema.NestedSchemaDefinition) {
		return msd, deps
	}

	// Ensure metadata field exists with exact properties
	result, _, _, err := sc.WithFieldEnsured(metadataField, provider)
	if err != nil {
		return nil, err
	}

	// Validate the final schema
	validationErrors := result.ValidateAll()
	if len(validationErrors) > 0 {
		fmt.Printf("Issues: %v\n", validationErrors)
		return nil, common.NewSystemError("INVALID_SCHEMA").WithIssues(validationErrors)
	}

	return result, nil
}


// TODO: implement a non panicky enrich schema
func MustEnrichSchema(sc *schema.SchemaDefinition) *schema.SchemaDefinition {
	result, err := EnrichSchema(sc)
	if err != nil {
		panic(err)
	}
	return  result
}
