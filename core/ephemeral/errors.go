package ephemeral

import (

	"github.com/asaidimu/go-anansi/v6/core/common"

)



// Pre-defined errors for the ephemeral package.

var (

	ErrNotTransaction         = common.NewSystemError("ERR_EPHEMERAL_NOT_TRANSACTION", "not a transaction")

	ErrRawQueriesNotSupported = common.NewSystemError("ERR_EPHEMERAL_RAW_QUERIES_NOT_SUPPORTED", "raw queries not supported")

	ErrNoNumericValuesForAggregation = common.NewSystemError("ERR_EPHEMERAL_NO_NUMERIC_VALUES_FOR_AGGREGATION", "no numeric values found for aggregation")

	ErrUniqueCheckFailed      = common.NewSystemError("ERR_EPHEMERAL_UNIQUE_CHECK_FAILED", "unique check failed")

	ErrUniqueConstraintViolation = common.NewSystemError("ERR_EPHEMERAL_UNIQUE_CONSTRAINT_VIOLATION", "unique constraint violation")

	ErrCollectionAlreadyExists = common.NewSystemError("ERR_EPHEMERAL_COLLECTION_ALREADY_EXISTS", "collection already exists")

	ErrCreateIndexFailed      = common.NewSystemError("ERR_EPHEMERAL_CREATE_INDEX_FAILED", "failed to create index")

	ErrCollectionNotFound     = common.NewSystemError("ERR_EPHEMERAL_COLLECTION_NOT_FOUND", "collection not found")

)
