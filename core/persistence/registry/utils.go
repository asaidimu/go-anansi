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
func generatePhysicalName(s *schema.Schema) (string, error) {
	// Validate inputs
	if s.Name == "" {
		return "", ErrSchemaNameEmpty
	}
	if s.Version == nil {
		return "", ErrSchemaVersionEmpty
	}

	versionStr := s.Version.String()

	// Sanitize name: keep only alphanumeric and convert to lowercase
	sanitizedName := sanitizeForDatabase(s.Name)
	if sanitizedName == "" {
		return "", ErrSchemaNameInvalidCharacters
	}

	// Sanitize version: replace dots with underscores
	sanitizedVersion := strings.ReplaceAll(versionStr, ".", "_")

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
func EnrichSchema(sc *schema.Schema) (*schema.Schema, error) {
	if sc == nil {
		return nil, nil
	}

	// --- Add ID Field ---
	idField := &schema.Field{
		Name:     schema.FieldName(data.DocumentIDField),
		Required: true,
		Unique:   true,
		FieldProperties: schema.FieldProperties{
			Type: schema.FieldTypeString,
		},
	}

	// Ensure the ID field exists with exact properties
	var err error
	sc, _, _, err = sc.WithFieldEnsured(idField)
	if err != nil {
		return nil, err
	}

	// --- Remove any user-defined indexes on 'id' field ---
	sc, _, err = sc.WithoutIndexesReferencingField(schema.FieldName(data.DocumentIDField))
	if err != nil {
		return nil, err
	}

	// --- Add primary key index ---
	// First find the FieldId for the ID field
	idFieldId, _, exists := sc.GetFieldByName(schema.FieldName(data.DocumentIDField))
	if !exists {
		return nil, fmt.Errorf("id field not found after being ensured")
	}

	pkIndex := &schema.Index{
		Name:   "pk_id",
		Fields: []schema.FieldId{idFieldId},
		Type:   schema.IndexTypePrimary,
		Unique: true,
	}

	// Ensure the primary key index exists (replaces if already there)
	sc, _, err = sc.WithIndexEnsured(pkIndex)
	if err != nil {
		return nil, err
	}

	// --- Add Metadata Field ---
	msdid := schema.SchemaId(uuid.Must(uuid.NewV7()).String())
	msd, _ := data.GetMetadataSchema()
	metadataField := &schema.Field{
		Name: schema.FieldName(data.MetadataField),
		FieldProperties: schema.FieldProperties{
			Type: schema.FieldTypeObject,
			Schema: schema.NewSchemaReference(schema.SchemaReference{
				ID: msdid,
			}),
		},
	}

	if sc.Schemas == nil {
		sc.Schemas = make(map[schema.SchemaId]schema.NestedSchema)
	}

	sc.Schemas[msdid] = *msd

	// Ensure metadata field exists
	sc, _, _, err = sc.WithFieldEnsured(metadataField)
	if err != nil {
		return nil, err
	}

	if _, err := schema.ValidateSchema(sc); err != nil {
		return nil, err
	}

	return sc, nil
}

func MustEnrichSchema(sc *schema.Schema) *schema.Schema {
	result, err := EnrichSchema(sc)
	if err != nil {
		panic(err)
	}
	return result
}
