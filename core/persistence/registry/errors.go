package registry

import (
	"errors"
	"strings"
)

// RegistryError represents errors specific to registry operations.
type RegistryError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *RegistryError) Error() string {
	var b strings.Builder
	b.WriteString(e.Operation)
	b.WriteString(" operation failed")

	if e.Key != "" {
		b.WriteString(" for key '")
		b.WriteString(e.Key)
		b.WriteString("': ")
	} else {
		b.WriteString(": ")
	}
	b.WriteString(e.Message)

	if e.Cause != nil {
		b.WriteString(" (caused by: ")
		b.WriteString(e.Cause.Error())
		b.WriteString(")")
	}
	return b.String()
}

func (e *RegistryError) Unwrap() error {
	return e.Cause
}

// Pre-defined errors for the registry package.
var (
	ErrCollectionNotFound                = errors.New("collection not found")
	ErrCollectionAlreadyExists           = errors.New("collection already exists")
	ErrFailedToCreateIndex               = errors.New("failed to create index")
	ErrFailedToReadDocuments             = errors.New("failed to read documents")
	ErrUniqueConstraintViolation         = errors.New("unique constraint violation")
	ErrRegistryNotInitialized            = errors.New("registry is not initialized")
	ErrFailedToListCollections           = errors.New("failed to list collections from registry")
	ErrPersistenceClosed                 = errors.New("persistence instance is closed")
	ErrFailedToCheckRegistryExistence    = errors.New("failed to check for existence of registry collection")
	ErrFailedToCreateRegistryCollection  = errors.New("failed to create registry collection")
	ErrFailedToWarmCache                 = errors.New("failed to load entries for cache warming")
	ErrDuplicateSchemaInBatch            = errors.New("duplicate schema in batch")
	ErrPhysicalNameConflictInBatch       = errors.New("physical name conflict in batch")
	ErrFailedToPersistRegistryEntry      = errors.New("failed to persist registry entry")
	ErrCannotPruneActiveVersion          = errors.New("cannot prune active version")
	ErrMultipleEntriesFound              = errors.New("multiple entries found")
	ErrVersionAlreadyExists              = errors.New("version already exists")
	ErrVersionAlreadyActive              = errors.New("requested version is already the active version")
	ErrVersionNotFound                   = errors.New("version not found")
	ErrFailedToUpdateRegistryEntry       = errors.New("failed to update registry entry")
	ErrFailedToDeleteRegistryEntry       = errors.New("failed to delete registry entry")
	ErrSchemaNameEmpty                   = errors.New("schema name cannot be empty")
	ErrSchemaVersionEmpty                = errors.New("schema version cannot be empty")
	ErrInvalidSemanticVersionFormat      = errors.New("version must follow semantic versioning format (x.y.z)")
	ErrSchemaNameInvalidCharacters       = errors.New("schema name contains no valid characters")
	ErrVersionTooLong                    = errors.New("version too long")
	ErrGeneratedNameExceedsLimit         = errors.New("generated name exceeds character limit")
	ErrFailedToDropPhysicalCollection = errors.New("failed to drop physical collection")
	ErrFailedToQueryRegistryCollection = errors.New("failed to query registry for collection")
	ErrMultipleRegistryEntriesFound = errors.New("multiple entries found for collection")
	ErrFailedToUnmarshalRegistryEntry = errors.New("failed to unmarshal registry entry")
	ErrFailedToMarshalRegistryEntry = errors.New("failed to marshal registry entry")
	ErrFailedToCreateRegistryEntry = errors.New("failed to create registry entry")
	ErrFailedToCreateRegistryEntryWithIssues = errors.New("failed to create registry entry with issues")
	ErrVersionNotFoundForCollection          = errors.New("version not found for collection")
)

// Errors related to physical name generation
var (
	ErrFailedToGeneratePhysicalName = errors.New("failed to generate physical name")
)