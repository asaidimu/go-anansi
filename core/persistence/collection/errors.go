package collection

import "github.com/asaidimu/go-anansi/v6/core/common"

// Pre-defined errors for the collection package.
var (
	ErrCollectionInitializationFailed          = common.NewSystemError("ERR_COLLECTION_INITIALIZATION_FAILED", "failed to initialize collection")
)
