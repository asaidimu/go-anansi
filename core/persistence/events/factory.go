package events

import (
	"context"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/transaction"
	"go.uber.org/zap"
)

// PersistenceEventFactory creates persistence events.
type PersistenceEventFactory struct {
	collectionName string
	logger         *zap.Logger
}

// NewPersistenceEventFactory creates a new persistence event factory.
func NewPersistenceEventFactory(collectionName string, logger *zap.Logger) *PersistenceEventFactory {
	return &PersistenceEventFactory{
		collectionName: collectionName,
		logger:         logger,
	}
}

// CreateEvent constructs a complete PersistenceEvent with all fields properly populated.
//
// Input and output are placed on the bus unchanged (full fidelity). The event bus is
// internal data movement between trusted components, not a rendering surface, so
// masking policies must not be applied here — consumers that render events (logs,
// audit output, client responses) sanitize at their own egress edge via
// document.Sanitize(ctx)/document.SafeString(ctx).
func (f *PersistenceEventFactory) CreateEvent(
	ctx context.Context,
	eventType string,
	operation string,
	input any,
	output any,
	errorMsg *string,
	startTime time.Time,
	duration *int64,
	extra map[string]any,
) base.PersistenceEvent {
	transactionID := f.extractTransactionID(ctx)

	var collectionName *string
	if name, ok := common.CollectionNameFromContext(ctx); ok {
		collectionName = &name
	}

	// Create the complete event
	event := base.PersistenceEvent{
		Type:          base.PersistenceEventType(eventType),
		Timestamp:     startTime.UnixMilli(),
		Operation:     operation,
		Collection:    collectionName,
		Input:         input,
		Output:        output,
		Error:         errorMsg,
		TransactionID: transactionID,
		Duration:      duration,
	}

	return event
}

// extractTransactionID tries to get transaction ID from operation context or Go context
func (f *PersistenceEventFactory) extractTransactionID(ctx context.Context) *string {
	if tx, ok := transaction.GetCurrentTransaction(ctx); ok {
		id := tx.ID()
		return &id
	}

	return nil
}