package native

import (
	"github.com/asaidimu/go-anansi/v7/core/common"
)

// Abstract database errors that can be mapped to by platform-specific executors.
var (
	ErrUniqueConstraintViolation     = common.NewSystemError("ERR_NATIVE_UNIQUE_CONSTRAINT_VIOLATION", "unique constraint violation")
	ErrForeignKeyConstraintViolation = common.NewSystemError("ERR_NATIVE_FOREIGN_KEY_CONSTRAINT_VIOLATION", "foreign key constraint violation")
	ErrConnectionFailed              = common.NewSystemError("ERR_NATIVE_CONNECTION_FAILED", "database connection failed")
	ErrTransactionFailed             = common.NewSystemError("ERR_NATIVE_TRANSACTION_FAILED", "transaction failed")
	ErrOperationFailed               = common.NewSystemError("ERR_NATIVE_OPERATION_FAILED", "database operation failed")
	ErrFailedToReadRows              = common.NewSystemError("ERR_NATIVE_FAILED_TO_READ_ROWS", "failed to read rows from result set")
	ErrCouldNotBuildQuery            = common.NewSystemError("ERR_NATIVE_COULD_NOT_BUILD_QUERY", "could not build query")
	ErrCouldNotDeleteWithoutFilters  = common.NewSystemError("ERR_NATIVE_COULD_NOT_DELETE_WITHOUT_FILTERS", "could not delete without filters")
	ErrFailedToBeginTransaction      = common.NewSystemError("ERR_NATIVE_FAILED_TO_BEGIN_TRANSACTION", "failed to begin transaction")
	ErrCannotNestTransactions        = common.NewSystemError("ERR_NATIVE_CANNOT_NEST_TRANSACTIONS", "cannot nest transactions")
	ErrQueryExecutorNil              = common.NewSystemError("ERR_NATIVE_QUERY_EXECUTOR_NIL", "query executor cannot be nil")
	ErrQueryFactoryNil               = common.NewSystemError("ERR_NATIVE_QUERY_FACTORY_NIL", "query factory cannot be nil")
	ErrCouldNotGetResultSchema       = common.NewSystemError("ERR_NATIVE_COULD_NOT_GET_RESULT_SCHEMA", "could not determine result schema")
	ErrCouldNotCheckCollection       = common.NewSystemError("ERR_NATIVE_COULD_NOT_CHECK_COLLECTION", "could not check for collection existence")
	ErrCouldNotBuildCreateCollectionQuery = common.NewSystemError("ERR_NATIVE_COULD_NOT_BUILD_CREATE_COLLECTION_QUERY", "could not build query for creating collection")
	ErrCouldNotCreateCollection      = common.NewSystemError("ERR_NATIVE_COULD_NOT_CREATE_COLLECTION", "could not create collection")
	ErrCouldNotBuildCreateIndexQuery = common.NewSystemError("ERR_NATIVE_COULD_NOT_BUILD_CREATE_INDEX_QUERY", "could not build query for creating index")
	ErrCouldNotBuildDropIndexQuery   = common.NewSystemError("ERR_NATIVE_COULD_NOT_BUILD_DROP_INDEX_QUERY", "could not build query for dropping index")
	ErrCouldNotBuildDropCollectionQuery = common.NewSystemError("ERR_NATIVE_COULD_NOT_BUILD_DROP_COLLECTION_QUERY", "could not build query for dropping collection")
	ErrFailedToUpdateDocuments       = common.NewSystemError("ERR_NATIVE_FAILED_TO_UPDATE_DOCUMENTS", "failed to update documents")
	ErrFailedToReadUpdatedDocuments  = common.NewSystemError("ERR_NATIVE_FAILED_TO_READ_UPDATED_DOCUMENTS", "failed to read updated documents after update")
)

