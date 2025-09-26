package events

import (
	"context"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/transaction"
)

// createPersistenceEvent constructs a complete PersistenceEvent with all fields properly populated
func createPersistenceEvent(
	eventType string,
	operation string,
	input any,
	output any,
	query any,
	errorMsg *string,
	transactionID *string,
	startTime time.Time,
	duration *int64,
	contextMap map[string]any,
	collectionName *string,
) base.PersistenceEvent {
	// Extract issues if provided (assuming common.Issue is still relevant)
	var issues []common.Issue

	// Create the complete event
	return base.PersistenceEvent{
		Type:          base.PersistenceEventType(eventType),
		Timestamp:     startTime.UnixMilli(),
		Operation:     operation,
		Collection:    collectionName,
		Input:         input,
		Output:        output,
		Error:         errorMsg,
		Issues:        issues,
		Query:         query,
		TransactionID: transactionID,
		Duration:      duration,
		Context:       contextMap,
	}
}

// extractPersistenceTransactionID tries to get transaction ID from operation context or Go context
func extractPersistenceTransactionID(ctx context.Context) *string {
	if tx, ok := transaction.GetCurrentTransaction(ctx); ok {
		id := tx.ID()
		return &id
	}

	return nil
}
