package events

import (
	"context"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/transaction"
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
		Input:         f.sanitizeEventData(ctx, input),
		Output:        f.sanitizeEventData(ctx, output),
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

// sanitizeEventData automatically sanitizes input and output data that contains documents.
// This is called by WithEventEmission to ensure all event data is sanitized before emission.
//
// The sanitization is context-aware:
// - If context contains collection name, applies collection-specific rules
// - Otherwise applies global rules
// - Handles Document, []Document, map[string]any, ReadResult, and other types
func (f *PersistenceEventFactory) sanitizeEventData(ctx context.Context, value any) any {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	// Special case: ReadResult contains documents
	case *base.ReadResult:
		if v == nil {
			return nil
		}
		d, err := data.SanitizeValue(ctx, v.Data)
		if err != nil {
			return err
		}
		res, ok := d.(data.DocumentSet)
		if !ok {
			return nil
		}
		sanitized := &base.ReadResult{
			Count: v.Count,
			Data:  res,
		}
		return sanitized

	// Special case: CreateResult contains a document
	case base.CreateResult:
		d, err := data.SanitizeValue(ctx, v.Data)
		if err != nil {
			return err
		}
		res, ok := d.(*data.Document)
		if !ok {
			return nil
		}
		sanitized := base.CreateResult{
			Status: v.Status,
			Data:   res,
			Issues: v.Issues,
			Error:  v.Error,
		}
		return sanitized

	case []base.CreateResult:
		sanitized := make([]base.CreateResult, len(v))
		for _, result := range v {
			d, err := data.SanitizeValue(ctx, result.Data)
			if err != nil {
				return err
			}
			res, ok := d.(*data.Document)
			if !ok {
				return nil
			}

			sanitized = append(sanitized,
				base.CreateResult{
					Status: result.Status,
					Issues: result.Issues,
					Data:   res,
					Error:  result.Error,
				})
		}
		return sanitized

	default:
		// Scalar or unknown type - preserve as-is
		// This includes query filters, schema definitions, etc.
		d, err := data.SanitizeValue(ctx, v)
		if err != nil {
			return err
		}
		return d
	}
}
