package ephemeral

import (
	"sync"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	store "github.com/asaidimu/go-store/v3"
)

// ephemeralStore holds the in-memory collections and a mutex for thread-safe access.
// It is not exported and is shared between the EphemeralDatabaseInteractor and EphemeralSchemaManager.
type ephemeralStore struct {
	collections map[string]*collection
	mu          sync.RWMutex
}

// collection represents a single in-memory collection, analogous to a table in a relational database.
type collection struct {
	Name   string
	schema *definition.Schema
	data   *store.Store
}

// NewEphemeral creates a new in-memory database interactor and schema manager that share the same underlying data store.
func NewEphemeral() query.DatabaseInteractor {
	store := &ephemeralStore{
		collections: make(map[string]*collection),
	}
	interactor := &EphemeralDatabaseInteractor{store: store}
	return interactor
}

// getCollection safely retrieves a collection by name from the store.
func (s *ephemeralStore) getCollection(name string) (*collection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.collections[name]
	if !ok {
		return nil, common.SystemErrorFrom(ErrCollectionNotFound).WithOperation("ephemeral.getCollection").WithPath(name).WithCause(base.ErrCollectionNotFound)
	}
	return c, nil
}
