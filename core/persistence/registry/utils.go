package registry

import (
	"fmt"
	"hash/fnv"
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

	// @note #physical-name-truncation-collide-496cab49 issue resolved P1 #persistence,#views,#data-corruption : Physical-name truncation collides distinct collections/views onto one table
	// @assignee opencode
	// Fixed: generatePhysicalName now suffixes truncated names with 8 hex chars of FNV-1a over the raw name+version, so distinct logical names can no longer collide on one physical table. Short names keep the legacy name_version form byte-for-byte. Regression tests in core/persistence/registry/utils_test.go.
	//
	// generatePhysicalName caps physical table names at 24 chars, truncating the sanitized logical name to ~18 chars. Distinct collections/views sharing a name prefix + version (e.g. e2e_view_snap-<ts>-<rand>, whose timestamp prefix is stable for days and whose random suffix is truncated off) resolve to the SAME physical table, and CTAS CREATE TABLE IF NOT EXISTS then silently reuses the stale table: reads serve another collection's rows with no error.
	//
	// Reproduced from hestia E2E: materialized snapshots repeatedly served a 3-row table from an earlier run while their base had 2 rows; green only after a server restart wiped :memory:.
	//
	// Fix direction: suffix physical names with a uniqueness component (short hash of the full logical name), and/or fail CreateView/DropCollection loudly when the physical table already belongs to another entry instead of reusing it.
	// Calculate available space for truncation
	const maxLength = 24
	const separator = "_"
	// hashSuffixLength reserves room for a short FNV-1a hash of the full
	// logical identity. Truncating the sanitized name alone lets distinct
	// collections/views sharing a prefix resolve to the SAME physical table
	// (see #physical-name-truncation-collide-496cab49); the hash suffix keeps
	// them distinct while staying within the length limit.
	const hashSuffixLength = 8
	separatorLength := len(separator)
	versionLength := len(sanitizedVersion)

	// Reserve space for version, separators, and the hash suffix
	maxNameLength := maxLength - versionLength - separatorLength - hashSuffixLength - separatorLength

	if maxNameLength < 1 {
		return "", common.NewSystemError("ERR_REGISTRY_VERSION_TOO_LONG", fmt.Sprintf("version too long to fit in %d character limit", maxLength))
	}

	var physicalName string
	if len(sanitizedName) <= maxNameLength {
		// Fits with room to spare: keep the legacy name_version form
		// byte-for-byte so existing physical tables keep resolving.
		physicalName = fmt.Sprintf("%s%s%s", sanitizedName, separator, sanitizedVersion)
	} else {
		// Truncate the name and suffix a hash of the full logical identity
		// (raw name + version, pre-sanitization, so names that sanitize
		// identically still diverge). Deterministic: same input always maps
		// to the same physical table.
		truncated := strings.TrimRight(sanitizedName[:maxNameLength], "_")
		if truncated == "" {
			truncated = "t"
		}
		suffix := physicalNameHash(s.Name, versionStr)
		physicalName = fmt.Sprintf("%s%s%s%s%s", truncated, separator, suffix, separator, sanitizedVersion)
	}

	// Final validation
	if len(physicalName) > maxLength {
		return "", common.NewSystemError("ERR_REGISTRY_GENERATED_NAME_EXCEEDS_LIMIT", fmt.Sprintf("generated name exceeds %d character limit", maxLength))
	}

	return physicalName, nil
}

// physicalNameHash returns 8 lowercase hex chars of FNV-1a over the raw
// logical name and version. FNV is deterministic across runs and platforms,
// unlike salted hashes, so registry lookups stay stable.
func physicalNameHash(name, version string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(version))
	return fmt.Sprintf("%08x", h.Sum32())
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
