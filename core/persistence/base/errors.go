package base

import (
	"errors"
	"fmt"
)

// PersistenceError represents a custom error type for persistence operations.
type PersistenceError struct {
	Message string
	Err     error
}

// Error returns the string representation of the PersistenceError.
func (e *PersistenceError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error, if any.
func (e *PersistenceError) Unwrap() error {
	return e.Err
}

// NewPersistenceError creates a new PersistenceError.
func NewPersistenceError(message string, err error) error {
	return &PersistenceError{
		Message: message,
		Err:     err,
	}
}

// Pre-defined errors for the persistence package.
var (
	ErrInvalidDataType              = errors.New("Invalid data type: expected map[string]any or []map[string]any")
	ErrValidationFailed             = errors.New("Provided data does not conform to the collection's schema")
	ErrInsertDocuments              = errors.New("Failed to insert data into collection")
	ErrReadDocuments                = errors.New("Failed to read data from collection")
	ErrUpdateDocuments              = errors.New("Failed to update data in collection")
	ErrDeleteDocuments              = errors.New("Failed to delete data from collection")
	ErrInvalidUpdateParams          = errors.New("Invalid update parameters provided")
	ErrDeleteRequiresFilter         = errors.New("Delete operation requires a filter or unsafe flag")
	ErrConflict                     = errors.New("Optimistic lock conflict: record version mismatch")
	ErrMetadataNotImplemented       = errors.New("Collection metadata method not implemented")
	ErrDataConversionFailed         = errors.New("Failed to convert data to a map for validation")
	ErrCollectionAlreadyExists      = errors.New("Collection with a similar name already exists")
	ErrSchemaNotFound               = errors.New("Schema does not exist")
	ErrInvalidSchema                = errors.New("Schema provided is invalid")
	ErrUnexpectedSchemaCount        = errors.New("Unexpected count for schema name")
	ErrFailedToInitializeEventBus   = errors.New("Could not initialize event bus")
	ErrFailedToCreateSchemaRegistry = errors.New("Failed to create schema registry")
	ErrFailedToStartTransaction     = errors.New("Failed to start database transaction")
	ErrFailedToCommitTransaction    = errors.New("Failed to commit database transaction")
	ErrFailedToRollbackTransaction  = errors.New("Failed to rollback database transaction")
	ErrReadingSchemas               = errors.New("Error reading schemas to get collection names")
	ErrMapToStructConversion        = errors.New("Failed to convert map to struct")
	ErrStructToMapConversion        = errors.New("Failed to convert struct to map")
	ErrCollectionCreation           = errors.New("Failed to create collection")
	ErrSchemaCollectionInit         = errors.New("Failed to initialize schemas collection")
	ErrCollectionDeletion           = errors.New("Failed to delete collection")
	ErrDropCollection               = errors.New("Failed to drop collection from database")
	ErrSchemaRead                   = errors.New("Error reading schema collection")
	ErrDocumentToStructConversion   = errors.New("Error converting document to struct")
	ErrCollectionInitialization     = errors.New("Failed to initialize collection")
	ErrFailedToRegisterSchema       = errors.New("Failed to register schema")
	ErrFailedToUnregisterSchema     = errors.New("Failed to unregister schema")
	ErrFailedToRefreshNames         = errors.New("Failed to refresh names")
	ErrFailedToInitializePersistence = errors.New("Failed to initialize persistence layer")
)
