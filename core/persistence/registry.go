package persistence

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SchemaRegistry manages the lifecycle of schema definitions within the persistence layer.
type SchemaRegistry struct {
	schema     *schema.SchemaDefinition
	collection PersistenceCollectionInterface
	interactor DatabaseInteractor
	logger     *zap.Logger
	fmap       schema.FunctionMap
	names      map[string]string
	bus        *events.TypedEventBus[PersistenceEvent]
}

func NewSchemaRegistry(interactor DatabaseInteractor, executor *Executor, fmap schema.FunctionMap, logger *zap.Logger) (*SchemaRegistry, error) {
	var registrySchema schema.SchemaDefinition
	err := registrySchema.From(RegistryCollectionSchemaJson)
	if err != nil {
		return nil, err
	}

	exists, err := interactor.CollectionExists(registrySchema.Name)
	if err != nil {
		return nil, fmt.Errorf("Error looking up schema collection: %w", err)
	}

	if !exists {
		tx, err := interactor.StartTransaction(context.Background())
		if err != nil {
			return nil, fmt.Errorf("Failed to create schema registry collection: %w", err)
		}
		if err := tx.CreateCollection(registrySchema); err != nil {
			tx.Rollback(context.Background())
			return nil, fmt.Errorf("Failed to create schema registry collection: %w", err)
		}
		tx.Commit(context.Background())
	}

	bus, err := events.NewTypedEventBus[PersistenceEvent](events.DefaultConfig())
	collection, err := NewCollection(bus, registrySchema.Name, &registrySchema, executor, fmap)

	if err != nil {
		return nil, fmt.Errorf("Failed to initialize schema registry: %w", err)
	}

	registry := &SchemaRegistry{
		collection: collection,
		interactor: interactor,
		logger:     logger,
		fmap:       fmap,
		schema:     &registrySchema,
		names:      make(map[string]string),
		bus:        bus,
	}
	// Populate names map using the existing RefreshNames method
	if err := registry.RefreshNames(); err != nil {
		return nil, fmt.Errorf("failed to refresh names during registry initialization: %w", err)
	}

	return registry, nil
}

func (r *SchemaRegistry) SchemaCollection(tx DatabaseInteractor) (PersistenceCollectionInterface, error) {
	var executor *Executor
	if tx != nil {
		executor = NewExecutor(tx, nil)
	} else {
		executor = NewExecutor(r.interactor, nil)
	}
	collection, err := NewCollection(r.bus, r.schema.Name, r.schema, executor, r.fmap)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSchemaCollectionInit, err)
	}
	return collection, nil
}

func (r *SchemaRegistry) GetPhysicalName(logicalName string) (string, bool) {
	record, err := r.Lookup(logicalName)
	if err != nil {
		return "", false
	}
	if activeVersion, ok := record.Versions[record.ActiveVersion]; ok {
		return activeVersion.Physical, true
	}
	return "", false
}

// Lookup retrieves a SchemaRecord by its logical name from the internal _schemas_ collection.
func (r *SchemaRegistry) Lookup(name string) (*SchemaRecord, error) {
	q := query.NewQueryBuilder().Where("name").Eq(name).Build()

	result, err := r.collection.Read(&q)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSchemaRead, err)
	}

	if result.Count == 0 {
		return nil, ErrSchemaNotFound
	}

	if result.Count > 1 {
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedSchemaCount, result.Count)
	}

	record, err := DocumentToStruct[SchemaRecord](result.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDocumentToStructConversion, err)
	}

	return &record, nil
}

// RegisterSchema registers a new schema definition in the internal _schemas_ collection.
// It generates a unique physical name for the collection and stores the mapping.
func (r *SchemaRegistry) RegisterSchema(ctx context.Context, tx DatabaseInteractor, s schema.SchemaDefinition) error {
	physicalName := uuid.New().String()

	// Initialize Versions map and add the initial schema
	versionsMap := make(map[string]CollectionVersionRecord)
	versionsMap[s.Version] = CollectionVersionRecord{
		Version:  s.Version,
		Physical: physicalName,
		Schema:   s,
	}

	record := SchemaRecord{
		Name: s.Name,
		Description:   *s.Description,
		ActiveVersion: s.Version,
		Versions:      versionsMap,
	}

	recordData, err := utils.StructToMap(&record)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStructToMapConversion, err)
	}

	collection, err := r.SchemaCollection(tx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSchemaCollectionInit, err)
	}

	_, err = collection.Create(recordData)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCollectionCreation, err)
	}

	return nil
}

// UnregisterSchema removes a schema definition from the internal _schemas_ collection.
func (r *SchemaRegistry) UnregisterSchema(ctx context.Context, tx DatabaseInteractor, logicalName string) error {
	collection, err := r.SchemaCollection(tx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSchemaCollectionInit, err)
	}

	q := query.NewQueryBuilder().Where("name").Eq(logicalName).Build()
	_, err = collection.Delete(q.Filters, false)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCollectionDeletion, err)
	}

	return nil
}

// SwapCollectionVersion updates the active physical name, schema version, and schema definition
// for a logical collection, and archives the previous version. This is a core operation
// for managing schema migrations.
func (r *SchemaRegistry) SwapCollectionVersion(
	ctx context.Context,
	tx DatabaseInteractor,
	logicalName string,
	newPhysicalName string,
	targetVersion string,
	newSchema schema.SchemaDefinition,
) error {
	record, err := r.Lookup(logicalName)
	if err != nil {
		return err
	}

	if record.Versions == nil {
		record.Versions = make(map[string]CollectionVersionRecord)
	}

	// Archive the current active version before updating
	currentActiveVersion, ok := record.Versions[record.ActiveVersion]
	if !ok {
		// This case should ideally not happen if RegisterSchema correctly initializes
		// the first version. Log a warning or return an error if this is critical.
		r.logger.Warn("Current active version not found in versions map during swap",
			zap.String("logicalName", logicalName),
			zap.String("activeVersion", record.ActiveVersion))
		// Decide: should this be an error or proceed? For now, proceed but log.
	} else {
		// Ensure the archived record has its own version field set
		currentActiveVersion.Version = record.ActiveVersion
		record.Versions[record.ActiveVersion] = currentActiveVersion
	}


	// Update the active version pointer
	record.ActiveVersion = targetVersion

	// Add the new version to the versions map (or update if it already exists)
	record.Versions[targetVersion] = CollectionVersionRecord{
		Version:  targetVersion,
		Physical: newPhysicalName,
		Schema:   newSchema,
	}

	updatedData, err := utils.StructToMap(record)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStructToMapConversion, err)
	}

	filter := query.NewQueryBuilder().Where("name").Eq(logicalName).Build().Filters

	collection, err := r.SchemaCollection(tx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSchemaCollectionInit, err)
	}

	affected, err := collection.Update(&CollectionUpdate{
		Data:   updatedData,
		Filter: filter,
	})

	if err != nil {
		return fmt.Errorf("%w: failed to update schema record for '%s': %v", ErrUpdateFailed, logicalName, err)
	}

	if affected == 0 {
		return fmt.Errorf("no schema record found to update for logical name '%s'", logicalName)
	}
	if affected > 1 {
		r.logger.Warn("Multiple schema records updated for logical name", zap.String("logicalName", logicalName), zap.Int("affectedCount", affected))
	}

	return nil
}

func (r *SchemaRegistry) RefreshNames() error {
	// Select logical name and activeVersion from the _schemas_ collection
	q := query.NewQueryBuilder().Select().Include("name", "activeVersion", "versions").End().Build()
	result, err := r.collection.Read(&q)

	if err != nil {
		return fmt.Errorf("Failed to get load schema names: %w", err)
	}

	names := make(map[string]string)

	var schemaRecords []SchemaRecord
	if result.Data != nil {
		if docs, ok := result.Data.([]schema.Document); ok {
			for _, doc := range docs {
				record, err := DocumentToStruct[SchemaRecord](doc)
				if err != nil {
					r.logger.Warn("Failed to convert document to SchemaRecord during RefreshNames", zap.Error(err))
					continue
				}
				schemaRecords = append(schemaRecords, record)
			}
		} else if doc, ok := result.Data.(schema.Document); ok {
			record, err := DocumentToStruct[SchemaRecord](doc)
			if err != nil {
				return fmt.Errorf("%w for single schema record: %v", ErrDocumentToStructConversion, err)
			}
			schemaRecords = append(schemaRecords, record)
		}
	}

	for _, record := range schemaRecords {
		if activeVersionData, ok := record.Versions[record.ActiveVersion]; ok {
			names[record.Name] = activeVersionData.Physical
		} else {
			r.logger.Warn("Active version not found in versions map for schema record",
				zap.String("logicalName", record.Name),
				zap.String("activeVersion", record.ActiveVersion))
		}
	}

	r.names = names
	return nil
}
