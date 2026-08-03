package registry

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/schema"
	"github.com/asaidimu/go-anansi/v8/core/utils"
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

func unmarshalEntry(doc data.Documenter) (*base.RegistryEntry, error) {
	return utils.MapToStruct[*RegistryEntry](doc.ToMap())
}

// EnrichSchema adds system fields (id, metadata) to a schema.
// Uses static UUIDs for all injected entities so the result is idempotent —
// the same input schema always produces the identical output.
func EnrichSchema(sc *schema.Schema) (*schema.Schema, error) {
	if sc == nil {
		return nil, nil
	}

	// Inject the system fields (_id_, _metadata_) and the metadata schema via
	// the shared enrichment utility, so this path and the container-backed
	// document layer always agree on the injection mechanics. The metadata
	// schema comes from the data factory, as before. The persistence-only
	// extras (index handling, validation) stay here.
	meta, deps := data.GetMetadataSchema()
	enriched, err := data.EnrichSchema(sc, meta, deps)
	if err != nil {
		return nil, err
	}
	sc = enriched

	// --- Remove any user-defined indexes on 'id' field ---
	sc, _, err = sc.WithoutIndexesReferencingField(schema.FieldName(data.DocumentIDField))
	if err != nil {
		return nil, err
	}

	// --- Add primary key index ---
	pkIndex := &schema.Index{
		Name:   "pk_id",
		Fields: []schema.FieldName{schema.FieldName(data.DocumentIDField)},
		Type:   schema.IndexTypePrimary,
		Unique: true,
	}
	sc, _, err = sc.WithIndexEnsured(pkIndex)
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
		e := common.NewSystemError(fmt.Sprintf("Enrichment failed for schema %s", sc.Name)).WithCause(err)
		panic(e)
	}
	return result
}
