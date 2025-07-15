package persistence

import "errors"

// Pre-defined errors for the persistence package.
var (
	ErrInvalidDataType              = errors.New("Invalid data type: expected map[string]any or []map[string]any")
	ErrValidationFailed             = errors.New("Provided data does not conform to the collection's schema")
	ErrInsertFailed                 = errors.New("Failed to insert data into collection")
	ErrReadFailed                   = errors.New("Failed to read data from collection")
	ErrUpdateFailed                 = errors.New("Failed to update data in collection")
	ErrDeleteFailed                 = errors.New("Failed to delete data from collection")
	ErrConflict                     = errors.New("Optimistic lock conflict: record version mismatch")
	ErrMetadataNotImplemented       = errors.New("Collection metadata method not implemented")
	ErrDataConversionFailed         = errors.New("Failed to convert data to a map for validation")
	ErrCollectionAlreadyExists      = errors.New("Collection with a similar name already exists")
	ErrSchemaNotFound               = errors.New("Schema does not exist")
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
)
