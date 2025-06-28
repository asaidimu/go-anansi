package persistence

import (
	"context"
	"fmt"
	"github.com/asaidimu/go-anansi/core"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
	"sync"
)

// CollectionBase implements core.PersistenceCollectionInterface.
type CollectionBase struct {
	schema        *core.SchemaDefinition
	processor     *core.DataProcessor
	executor      *Executor
	validator     *core.Validator
	bus           *events.TypedEventBus[core.PersistenceEvent]
	subscriptions map[string]*core.SubscriptionInfo // To store unsubscribe functions
	subMu         sync.RWMutex                      // Mutex to protect subscriptions map
}

// NewPersistence creates a new instance of Persistence.
func NewCollection(schema *core.SchemaDefinition, executor *Executor, fmap core.FunctionMap) (core.PersistenceCollectionInterface, error) {
	validator := core.NewValidator(schema, fmap)
	bus, err := events.NewTypedEventBus[core.PersistenceEvent](events.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("Could not initialize event bus %v", err)
	}

	collection := NewEventEmittingCollection(&CollectionBase{
		schema:        schema,
		executor:      executor,
		validator:     validator,
		bus:           bus,
		subscriptions: map[string]*core.SubscriptionInfo{},
	})

	return collection, nil
}

func (ci *CollectionBase) Create(data any) (any, error) {
	var records []map[string]any
	switch v := data.(type) {
	case map[string]any:
		records = []map[string]any{v}
	case []map[string]any:
		records = v
	default:
		return nil, fmt.Errorf("invalid data type for Create: expected map[string]any or []map[string]any, got %T", data)
	}

	for _, record := range records {
		validation, err := ci.Validate(record, false)
		if err != nil {
			return nil, fmt.Errorf("An error occured when trying to validate an entry %e", err)
		}

		if !validation.Valid {
			return nil, fmt.Errorf("Provided data does not conform to the collections schema")
		}
	}

	result, err := ci.executor.Insert(context.Background(), ci.schema, records)
	if err != nil {
		return nil, fmt.Errorf("failed to insert data into collection '%s': %w", ci.schema.Name, err)
	}

	return result, nil
}

// Read retrieves data from the collection based on a core.
func (ci *CollectionBase) Read(qr any) (any, error) {
	q := qr.(core.QueryDSL)
	result, err := ci.executor.Query(context.Background(), ci.schema, &q)
	if err != nil {
		return nil, fmt.Errorf("failed to read data from collection '%s': %w", ci.schema.Name, err)
	}

	return result, nil
}

// Update updates data in the collection based on filter.
func (ci *CollectionBase) Update(params *core.CollectionUpdate) (int, error) {
	var filter *core.QueryFilter
	if params != nil {
		f, ok := params.Filter.(*core.QueryFilter)
		if !ok {
			return 0, fmt.Errorf("invalid params type for Update: expected core.QueryFilter, got %T", params.Filter)
		}
		filter = f
	}
	result, err := ci.executor.Update(context.Background(), ci.schema, params.Data, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to read data from collection '%s': %w", ci.schema.Name, err)
	}

	return int(result), nil
}

// Delete deletes data from the collection based on core.
func (ci *CollectionBase) Delete(params any, unsafe bool) (int, error) {
	var filter *core.QueryFilter
	if params != nil {
		f, ok := params.(*core.QueryFilter)
		if !ok {
			return 0, fmt.Errorf("invalid params type for Delete: expected core.QueryFilter, got %T", params)
		}
		filter = f
	}

	ctx := context.Background()
	affected, err := ci.executor.Delete(ctx, ci.schema, filter, false)
	if err != nil {
		return 0, fmt.Errorf("failed to delete data from collection '%s': %w", ci.schema.Name, err)
	}

	// Convert int64 to int
	return int(affected), nil
}

// Validate validates data against the collection's schema.
func (ci *CollectionBase) Validate(data any, loose bool) (*core.ValidationResult, error) {
	values, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Failed to convert data to a map")
	}

	valid, issues := ci.validator.Validate(values, loose)
	return &core.ValidationResult{
		Valid:  valid,
		Issues: issues,
	}, nil
}

// Rollback rolls back the collection's schema.
func (ci *CollectionBase) Rollback(version *string, dryRun *bool) (struct {
	Schema  core.SchemaDefinition `json:"schema"`
	Preview any                   `json:"preview"`
}, error) {
	// TODO: Discuss & Design
	return struct {
		Schema  core.SchemaDefinition `json:"schema"`
		Preview any                   `json:"preview"`
	}{}, fmt.Errorf("Rollback method stub for collection '%s'", ci.schema.Name) // Stub: not implemented
}

// Migrate migrates the collection's schema.
func (ci *CollectionBase) Migrate(
	description string,
	cb func(h core.SchemaMigrationHelper) (core.DataTransform[any, any], error),
	dryRun *bool,
) (struct {
	Schema  core.SchemaDefinition `json:"schema"`
	Preview any                   `json:"preview"`
}, error) {
	// TODO: Discuss & Design
	return struct {
		Schema  core.SchemaDefinition `json:"schema"`
		Preview any                   `json:"preview"`
	}{}, fmt.Errorf("Migrate method stub for collection '%s'", ci.schema.Name) // Stub: not implemented
}

func (ci *CollectionBase) Metadata(
	filter *core.MetadataFilter,
	forceRefresh bool,
) (core.Metadata, error) {
	return core.Metadata{}, fmt.Errorf("Collection metadata method stub for '%s'", ci.schema.Name) // Stub: not implemented
}

// RegisterSubscription registers a collection-scoped subscription.
func (ci *CollectionBase) RegisterSubscription(options core.RegisterSubscriptionOptions) string {
	ci.subMu.Lock()
	defer ci.subMu.Unlock()
	unsubscribe := ci.bus.Subscribe(string(options.Event), options.Callback)
	id := uuid.New()
	callbackID := id.String()

	data := core.SubscriptionInfo{
		Event:       options.Event,
		Unsubscribe: unsubscribe,
		Label:       options.Label,
		Description: options.Description,
	}

	ci.subscriptions[callbackID] = &data
	return callbackID
}

// UnregisterSubscription unregisters a collection-scoped subscription.
func (ci *CollectionBase) UnregisterSubscription(id string) {
	ci.subMu.Lock()
	defer ci.subMu.Unlock()
	info := ci.subscriptions[id]
	if info != nil {
		info.Unsubscribe()
		delete(ci.subscriptions, id)
	}
}

// Subscriptions returns all registered collection-scoped subscriptions.
func (ci *CollectionBase) Subscriptions() ([]core.SubscriptionInfo, error) {
	return []core.SubscriptionInfo{}, nil // Stub: return empty slice
}
