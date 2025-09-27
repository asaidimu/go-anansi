package events

import (
	"context"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/transaction"
	"github.com/asaidimu/go-anansi/v6/core/utils"
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
) base.PersistenceEvent {
	transactionID := f.extractTransactionID(ctx)
	contextMap := PersitenceEventContextData.Data(ctx)

	// Handle collection name (could be empty for persistence-level events)
	var collectionName *string
	if f.collectionName != "" {
		collectionName = &f.collectionName
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

	// Log event creation if logger is available
	if f.logger != nil {
		etype := string(eventType)

		logLevel := zap.DebugLevel
		if errorMsg != nil {
			logLevel = zap.ErrorLevel
		} else if etype != "" && strings.Contains(strings.ToLower(etype), "success") {
			logLevel = zap.InfoLevel
		}

		f.logger.Log(logLevel, "Persistence event created",
			zap.String("type", string(eventType)),
			zap.String("operation", operation),
			zap.Any("collection", collectionName),
			zap.Any("transactionID", transactionID),
			zap.Any("duration", duration),
			zap.Bool("hasError", errorMsg != nil),
		)
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
