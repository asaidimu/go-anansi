package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/core"
	"github.com/asaidimu/go-anansi/core/query"
	"go.uber.org/zap"
)

// Persistence implements the core.Persistence interface.
type Persistence struct {
	interactor DatabaseInteractor
	collection core.PersistenceCollectionInterface
	schema     *core.SchemaDefinition
	executor   *Executor
	fmap       core.FunctionMap
	logger     *zap.Logger
}

// NewPersistence creates a new instance of Persistence.
func NewPersistence(interactor DatabaseInteractor, fmap core.FunctionMap) (*Persistence, error) {
	var schema core.SchemaDefinition
	err := json.Unmarshal(schemasCollectionSchema, &schema)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling JSON: %v\n", err)
	}

	exists, err := interactor.CollectionExists(schema.Name)
	if err != nil {
		return nil, fmt.Errorf("Error looking up schema collection: %v\n", err)
	}

	if !exists {
		if err := interactor.CreateCollection(schema); err != nil {
			return nil, fmt.Errorf("Failed to create table for collections %s: %w", schema.Name, err)
		}
	}

	executor := NewExecutor(interactor, nil)
	collection, err := NewCollection(&schema, executor, fmap)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize schemas collection collections %w", err)
	}

	return &Persistence{
		interactor: interactor,
		executor:   executor,
		fmap:       fmap,
		collection: collection,
		schema:     &schema,
		logger:     zap.NewNop(),
	}, nil
}

// Collection returns a PersistenceCollectionInterface for the given name.
func (pi *Persistence) Collection(name string) (core.PersistenceCollectionInterface, error) {
	schema, err := pi.Schema(name)
	if err != nil {
		return nil, err
	}

	collection, err := NewCollection(schema, pi.executor, pi.fmap)
	if err != nil {
		return nil, err
	}
	return collection, nil
}

func (pi *Persistence) Transact(callback func(tx core.PersistenceTransactionInterface) (any, error)) (any, error) {
	tx, err := pi.interactor.StartTransaction(context.Background())
	if err != nil {
		return nil, err
	}
	transactionCtx, err := NewPersistence(tx, pi.fmap)
	if err != nil {
		tx.Rollback(context.Background())
		return nil, err
	}
	result, err := callback(transactionCtx)
	if err != nil {
		tx.Rollback(context.Background())
		return result, err
	}

	tx.Commit(context.Background())
	return result, err
}

// Collections returns a list of collection names.
func (pi *Persistence) Collections() ([]string, error) {
	return []string{}, nil
}

// Create creates a new collection based on the schema.
func (pi *Persistence) Create(schema core.SchemaDefinition) (core.PersistenceCollectionInterface, error) {
	exists, err := pi.interactor.CollectionExists(schema.Name)
	if err != nil {
		return nil, fmt.Errorf("Error accessing database: %v\n", err)
	}

	if exists {
		return nil, fmt.Errorf("A collection with a similar name exists\n")
	}

	if err := pi.interactor.CreateCollection(schema); err != nil {
		return nil, fmt.Errorf("Failed to create collection %s: %w", schema.Name, err)
	}

	schemaJson, err := schemaToRawJson(&schema)
	if err != nil {
		return nil, err
	}

	record := SchemaRecord{
		Name:        schema.Name,
		Description: *schema.Description,
		Version:     schema.Version,
		Schema:      schemaJson,
	}

	data, err := schemaRecordToMap(&record)
	if err != nil {
		return nil, err
	}
	_, err = pi.collection.Create(data)
	if err != nil {
		return nil, err
	}

	collection, err := NewCollection(&schema, pi.executor, pi.fmap)
	if err != nil {
		return nil, err
	}
	return collection, nil
}

func (pi *Persistence) schemaCollection(tx DatabaseInteractor) (core.PersistenceCollectionInterface, error) {
	var executor *Executor
	if tx != nil {
		executor = NewExecutor(tx, nil)
	} else {
		executor = NewExecutor(pi.interactor, nil)
	}
	collection, err := NewCollection(pi.schema, executor, pi.fmap)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize schemas collection collections %w", err)
	}
	return collection, nil
}

// Delete deletes a collection by its ID (name).
func (pi *Persistence) Delete(name string) (bool, error) {
	tx, err := pi.interactor.StartTransaction(context.Background())
	if err != nil {
		return false, err
	}
	collection, err := pi.schemaCollection(tx)
	if err != nil {
		return false, err
	}
	q := query.NewQueryBuilder().Where("name").Eq(name).Build()
	_, err = collection.Delete(q.Filters, false)
	if err != nil {
		tx.Rollback(context.Background())
		return false, err
	}

	err = tx.DropCollection(name)
	if err != nil {
		tx.Rollback(context.Background())
		return false, err
	}

	err = tx.Commit(context.Background())

	if err != nil {
		tx.Rollback(context.Background())
		return false, err
	}
	return true, nil
}

// Schema returns the schema definition for a given ID.
func (pi *Persistence) Schema(name string) (*core.SchemaDefinition, error) {
	exists, err := pi.interactor.CollectionExists(name)
	if err != nil {
		return nil, fmt.Errorf("Error accessing database: %v, %t\n", err, exists)
	}

	if !exists {
		return nil, fmt.Errorf("Collection %s does not exist: %v\n", name, err)
	}

	q := query.NewQueryBuilder().Where("name").Eq(name).Build()

	out, err := pi.collection.Read(q)
	if err != nil {
		return nil, fmt.Errorf("Error reading schema collection: %v\n", err)
	}

	result, _ := out.(*query.QueryResult)


	if result.Count != 1 {
		return nil, fmt.Errorf("Unexpected count for schema name: %v\n", result.Count)
	}

	record, err := mapToSchemaRecord(result.Data.(query.Document))
	if err != nil {
		return nil, fmt.Errorf("Error converting map to SchemaRecord: %v\n", err)
	}

	var schema core.SchemaDefinition
	err = json.Unmarshal(record.Schema, &schema)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling JSON: %v\n", err)
	}
	return &schema, nil
}

// Metadata retrieves metadata based on the filter.
func (pi *Persistence) Metadata(
	filter *core.MetadataFilter,
	includeCollections bool,
	includeSchemas bool,
	forceRefresh bool,
) (core.Metadata, error) {
	return core.Metadata{}, fmt.Errorf("Metadata method stub") // Stub: not implemented
}

// RegisterSubscription registers a new subscription.
func (pi *Persistence) RegisterSubscription(options core.RegisterSubscriptionOptions) (core.SubscriptionInfo, error) {
	return core.SubscriptionInfo{}, fmt.Errorf("RegisterSubscription method stub") // Stub: not implemented
}

// UnregisterSubscription unregisters an existing subscription.
func (pi *Persistence) UnregisterSubscription(callback string) error {
	return fmt.Errorf("UnregisterSubscription method stub") // Stub: not implemented
}

// RegisterTrigger registers a new trigger.
func (pi *Persistence) RegisterTrigger(options core.RegisterTriggerOptions) (core.TriggerInfo, error) {
	return core.TriggerInfo{}, fmt.Errorf("RegisterTrigger method stub") // Stub: not implemented
}

// UnregisterTrigger unregisters an existing trigger.
func (pi *Persistence) UnregisterTrigger(options core.UnregisterTriggerOptions) error {
	return fmt.Errorf("UnregisterTrigger method stub") // Stub: not implemented
}

// RegisterTask registers a new task.
func (pi *Persistence) RegisterTask(options core.RegisterTaskOptions) (core.TaskInfo, error) {
	return core.TaskInfo{}, fmt.Errorf("RegisterTask method stub") // Stub: not implemented
}

// UnregisterTask unregisters an existing task.
func (pi *Persistence) UnregisterTask(options core.UnregisterTaskOptions) error {
	return fmt.Errorf("UnregisterTask method stub") // Stub: not implemented
}

// Subscriptions returns all registered subscriptions.
func (pi *Persistence) Subscriptions() ([]core.SubscriptionInfo, error) {
	return []core.SubscriptionInfo{}, nil // Stub: return empty slice
}

// Triggers returns all registered triggers.
func (pi *Persistence) Triggers() ([]core.TriggerInfo, error) {
	return []core.TriggerInfo{}, nil // Stub: return empty slice
}

// Tasks returns all registered tasks.
func (pi *Persistence) Tasks() ([]core.TaskInfo, error) {
	return []core.TaskInfo{}, nil // Stub: return empty slice
}
