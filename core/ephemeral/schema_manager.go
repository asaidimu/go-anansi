package ephemeral

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	store "github.com/asaidimu/go-store/v3"
)

// EphemeralSchemaManager provides an in-memory implementation of the query.SchemaManager interface.
// It handles DDL operations for the ephemeral storage.

type EphemeralSchemaManager struct {
	store *ephemeralStore
}

var _ query.SchemaManager = (*EphemeralSchemaManager)(nil)

// CreateCollection creates a new collection in the in-memory store.
func (m *EphemeralSchemaManager) CreateCollection(schemaDef schema.SchemaDefinition) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	if _, exists := m.store.collections[schemaDef.Name]; exists {
		return fmt.Errorf("collection '%s' already exists", schemaDef.Name)
	}

	newStore := store.NewStore()

	// Create indexes based on schema definition
	for _, field := range schemaDef.Fields {
		if field.Unique != nil && *field.Unique {
			if err := newStore.CreateIndex(field.Name, []string{field.Name}); err != nil {
				return fmt.Errorf("failed to create unique index for field %s: %w", field.Name, err)
			}
		}
	}
	for _, index := range schemaDef.Indexes {
		if err := newStore.CreateIndex(index.Name, index.Fields); err != nil {
			return fmt.Errorf("failed to create index %s: %w", index.Name, err)
		}
	}

	newCollection := &collection{
		Name:   schemaDef.Name,
		schema: &schemaDef,
		data:   newStore,
	}

	m.store.collections[schemaDef.Name] = newCollection
	return nil
}

// CreateIndex creates an index in the in-memory store.
func (m *EphemeralSchemaManager) CreateIndex(name string, index schema.IndexDefinition) error {
	c, err := m.store.getCollection(name)
	if err != nil {
		return err
	}

	return c.data.CreateIndex(index.Name, index.Fields)
}

// DropCollection removes a collection from the in-memory store.
func (m *EphemeralSchemaManager) DropCollection(name string) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	c, ok := m.store.collections[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrCollectionNotFound, name)
	}

	c.data.Close()
	delete(m.store.collections, name)
	return nil
}

// CollectionExists checks if a collection exists in the in-memory store.
func (m *EphemeralSchemaManager) CollectionExists(name string) (bool, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	_, exists := m.store.collections[name]
	return exists, nil
}
