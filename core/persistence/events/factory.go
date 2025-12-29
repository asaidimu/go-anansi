package events

import (
	"context"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/transaction"
	putils "github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"go.uber.org/zap"
)

// PersitenceEventContextData manages the context data for persistence events.
var PersistenceEventContextData = utils.NewContextData("persistence_event_context", nil)

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
	contextMap := PersistenceEventContextData.Data(ctx)

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
		Input:         f.sanitizeEventData(ctx, input),
		Output:         f.sanitizeEventData(ctx, output),
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
	case data.Document:
		// Single document - use context-aware sanitization
		return v.Sanitize(ctx)

	case []data.Document:
		// Array of documents
		return data.SanitizeDocumentArray(ctx, v)

	case map[string]any:
		// Treat as document
		return data.Document(v).Sanitize(ctx)

	case []map[string]any:
		// Array of maps - treat as documents
		sanitized := make([]map[string]any, len(v))
		for i, m := range v {
			sanitized[i] = data.Document(m).Sanitize(ctx)
		}
		return sanitized

	// Special case: ReadResult contains documents
	case *base.ReadResult:
		if v == nil {
			return nil
		}
		sanitized := &base.ReadResult{
			Count: v.Count,
			Data:  data.SanitizeDocumentArray(ctx, v.Data),
		}
		return sanitized

	// Special case: CreateResult contains a document
	case base.CreateResult:
		sanitized := base.CreateResult{
			Status: v.Status,
			Data:   v.Data.Sanitize(ctx),
			Issues: v.Issues,
			Error:  v.Error,
		}
		return sanitized

	case []base.CreateResult:
		sanitized := make([]base.CreateResult, len(v))
		for _, result := range v {
			sanitized = append(sanitized,
				base.CreateResult{
					Status: result.Status,
					Data:   result.Data.Sanitize(ctx),
					Issues: result.Issues,
					Error:  result.Error,
				})
		}
		return sanitized

	case []any:
		// Generic array - recurse on elements
		sanitized := make([]any, len(v))
		for i, item := range v {
			sanitized[i] = f.sanitizeEventData(ctx, item)
		}
		return sanitized

	default:
		// Scalar or unknown type - preserve as-is
		// This includes query filters, schema definitions, etc.
		return value
	}
}
