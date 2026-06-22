package collection

import "github.com/asaidimu/go-anansi/v7/core/common"

// Pre-defined errors for the collection package.
var (
	ErrCollectionInitializationFailed = common.NewSystemError("ERR_COLLECTION_INITIALIZATION_FAILED", "failed to initialize collection")
	ErrRecordNotFound                 = common.NewSystemError("ERR_COLLECTION_RECORD_NOT_FOUND").WithMessage("record not found")
	ErrNoRecordsAffected              = common.NewSystemError("ERR_COLLECTION_NO_RECORDS_AFFECTED").WithMessage("no records were affected by the operation")
)
