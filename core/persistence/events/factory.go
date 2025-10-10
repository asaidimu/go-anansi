package events

import (
	"context"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/transaction"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	putils "github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"go.uber.org/zap"
)

// PersitenceEventContextData manages the context data for persistence events.
var PersitenceEventContextData = utils.NewContextData("persistence_event_context", nil)

// PersistenceEventFactory creates persistence events.
type PersistenceEventFactory struct {
	collectionName string
	logger         *zap.Logger
	cd             utils.ContextData
}

// NewPersistenceEventFactory creates a new persistence event factory.
func NewPersistenceEventFactory(collectionName string, logger *zap.Logger) *PersistenceEventFactory {
	return &PersistenceEventFactory{
		collectionName: collectionName,
		logger:         logger,
	}
}

// CreateEvent constructs a complete PersistenceEvent with all fields properly populated
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
	contextMap := PersitenceEventContextData.Data(ctx)

	var collectionName *string
	if name, ok := putils.CollectionNameFromContext(ctx); ok {
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
		Context:       contextMap,
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
