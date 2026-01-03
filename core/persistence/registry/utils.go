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
	"github.com/google/uuid"
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
	return utils.MapToStruct[*RegistryEntry](doc.AsMap())
}

func EnrichSchema(sc *schema.SchemaDefinition) *schema.SchemaDefinition {
	if sc == nil {
		return nil
	}

	// --- Add ID Field ---
	idField := &schema.FieldDefinition{
		Name:     data.DocumentID,
		Type:     schema.FieldTypeString,
		Required: utils.BoolPtr(true),
		Unique:   utils.BoolPtr(true),
	}
	id_id := uuid.Must(uuid.NewV7()).String()
	sc = sc.MustAddField(id_id, idField, nil)

	// --- Enforce ID Index ---
	var filteredIndexes []schema.IndexOrReference
	for _, index := range sc.Indexes {
		if len(index.Index.Fields) == 1 && index.Index.Fields[0] == data.DocumentID {
			continue // Skip user-defined index on 'id'.
		}
		filteredIndexes = append(filteredIndexes, index)
	}
	sc.Indexes = filteredIndexes

	sc = sc.MustAddIndex(schema.IndexDefinition{
		Name:   "pk_id",
		Fields: []string{data.DocumentID},
		Type:   schema.IndexTypePrimary,
		Unique: utils.BoolPtr(true),
	})

	metadata_id := uuid.Must(uuid.NewV7()).String()

	msd, deps := data.GetMetadataSchema()

	// --- Add Metadata Field ---
	metadataField := &schema.FieldDefinition{
		Name:   data.MetadataField,
		Type:   schema.FieldTypeObject,
		Schema: schema.NestedSchemaReference{ID: *msd.ID},
	}

	provider := func(sc *schema.SchemaDefinition) (*schema.NestedSchemaDefinition, []*schema.NestedSchemaDefinition) {
		return msd, deps
	}

	result := sc.MustAddField(metadata_id,metadataField, provider)
	if err := result.Validate(); err != nil {
		panic(err)
	}
	return  result
}
