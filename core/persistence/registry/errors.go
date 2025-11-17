package registry

import (
	"github.com/asaidimu/go-anansi/v6/core/common"
)

// Pre-defined errors for the registry package.
var (
	ErrRegistryNotInitialized            = common.NewSystemError("ERR_REGISTRY_NOT_INITIALIZED", "registry is not initialized")
	ErrFailedToCheckRegistryExistence    = common.NewSystemError("ERR_REGISTRY_FAILED_TO_CHECK_REGISTRY_EXISTENCE", "failed to check for existence of registry collection")
	ErrFailedToCreateRegistryCollection  = common.NewSystemError("ERR_REGISTRY_FAILED_TO_CREATE_REGISTRY_COLLECTION", "failed to create registry collection")
	ErrFailedToWarmCache                 = common.NewSystemError("ERR_REGISTRY_FAILED_TO_WARM_CACHE", "failed to load entries for cache warming")
	ErrDuplicateSchemaInBatch            = common.NewSystemError("ERR_REGISTRY_DUPLICATE_SCHEMA_IN_BATCH", "duplicate schema in batch")
	ErrPhysicalNameConflictInBatch       = common.NewSystemError("ERR_REGISTRY_PHYSICAL_NAME_CONFLICT_IN_BATCH", "physical name conflict in batch")
	ErrFailedToPersistRegistryEntry      = common.NewSystemError("ERR_REGISTRY_FAILED_TO_PERSIST_REGISTRY_ENTRY", "failed to persist registry entry")
	ErrCannotPruneActiveVersion          = common.NewSystemError("ERR_REGISTRY_CANNOT_PRUNE_ACTIVE_VERSION", "cannot prune active version")
	ErrVersionAlreadyActive              = common.NewSystemError("ERR_REGISTRY_VERSION_ALREADY_ACTIVE", "requested version is already the active version")
	ErrFailedToUpdateRegistryEntry       = common.NewSystemError("ERR_REGISTRY_FAILED_TO_UPDATE_REGISTRY_ENTRY", "failed to update registry entry")
	ErrFailedToDeleteRegistryEntry       = common.NewSystemError("ERR_REGISTRY_FAILED_TO_DELETE_REGISTRY_ENTRY", "failed to delete registry entry")
	ErrSchemaNameEmpty                   = common.NewSystemError("ERR_REGISTRY_SCHEMA_NAME_EMPTY", "schema name cannot be empty")
	ErrSchemaVersionEmpty                = common.NewSystemError("ERR_REGISTRY_SCHEMA_VERSION_EMPTY", "schema version cannot be empty")
	ErrInvalidSemanticVersionFormat      = common.NewSystemError("ERR_REGISTRY_INVALID_SEMANTIC_VERSION_FORMAT", "version must follow semantic versioning format (x.y.z)")
	ErrSchemaNameInvalidCharacters       = common.NewSystemError("ERR_REGISTRY_SCHEMA_NAME_INVALID_CHARACTERS", "schema name contains no valid characters")
	ErrVersionTooLong                    = common.NewSystemError("ERR_REGISTRY_VERSION_TOO_LONG", "version too long")
	ErrGeneratedNameExceedsLimit         = common.NewSystemError("ERR_REGISTRY_GENERATED_NAME_EXCEEDS_LIMIT", "generated name exceeds character limit")
	ErrFailedToDropPhysicalCollection    = common.NewSystemError("ERR_REGISTRY_FAILED_TO_DROP_PHYSICAL_COLLECTION", "failed to drop physical collection")
	ErrFailedToQueryRegistryCollection   = common.NewSystemError("ERR_REGISTRY_FAILED_TO_QUERY_REGISTRY_COLLECTION", "failed to query registry for collection")
	ErrMultipleRegistryEntriesFound      = common.NewSystemError("ERR_REGISTRY_MULTIPLE_REGISTRY_ENTRIES_FOUND", "multiple entries found for collection")
	ErrFailedToUnmarshalRegistryEntry    = common.NewSystemError("ERR_REGISTRY_FAILED_TO_UNMARSHAL_REGISTRY_ENTRY", "failed to unmarshal registry entry")
	ErrFailedToMarshalRegistryEntry      = common.NewSystemError("ERR_REGISTRY_FAILED_TO_MARSHAL_REGISTRY_ENTRY", "failed to marshal registry entry")
	ErrFailedToCreateRegistryEntry       = common.NewSystemError("ERR_REGISTRY_FAILED_TO_CREATE_REGISTRY_ENTRY", "failed to create registry entry")
	ErrFailedToCreateRegistryEntryWithIssues = common.NewSystemError("ERR_REGISTRY_FAILED_TO_CREATE_REGISTRY_ENTRY_WITH_ISSUES", "failed to create registry entry with issues")
	ErrVersionNotFoundForCollection      = common.NewSystemError("ERR_REGISTRY_VERSION_NOT_FOUND_FOR_COLLECTION", "version not found for collection")
	ErrCollectionCreationFailed          = common.NewSystemError("ERR_REGISTRY_COLLECTION_CREATION_FAILED", "failed to create physical collection")
)

// Errors related to physical name generation
var (
	ErrFailedToGeneratePhysicalName = common.NewSystemError("ERR_REGISTRY_FAILED_TO_GENERATE_PHYSICAL_NAME", "failed to generate physical name")
)