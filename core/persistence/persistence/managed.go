package persistence

import (
	"errors"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// managedPersistence is a decorator that wraps a base.Persistence
// to manage its closed state and prevent operations after closure.
type managedPersistence struct {
	wrapped base.Persistence
	closed  bool
}


var _ base.Persistence = (*managedPersistence)(nil)

// NewManagedPersistence creates a new managedPersistence decorator.
func newManagedPersistence(wrapped base.Persistence) base.Persistence {
	return &managedPersistence{
		wrapped: wrapped,
		closed:  false,
	}
}

// checkClosed returns an error if the persistence instance is closed.
func (m *managedPersistence) checkClosed() error {
	if m.closed {
		return errors.New("persistence instance is closed")
	}
	return nil
}

// Close sets the closed flag and delegates the call to the wrapped persistence.
func (m *managedPersistence) Close() {
	m.closed = true
	m.wrapped.Close()
}

// Collection delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Collection(name string) (base.Collection, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Collection(name)
}

// Collections delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Collections() ([]string, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Collections()
}

// Create delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Create(sc schema.SchemaDefinition) (base.Collection, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err	}
	return m.wrapped.Create(sc)
}

// Delete delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Delete(id string) (bool, error) {
	if err := m.checkClosed(); err != nil {
		return false, err
	}
	return m.wrapped.Delete(id)
}

// Metadata delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Metadata(filter *base.MetadataFilter) (base.Metadata, error) {
	if err := m.checkClosed(); err != nil {
		return base.Metadata{}, err
	}
	return m.wrapped.Metadata(filter)
}

// RegisterSubscription delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) RegisterSubscription(options base.RegisterSubscriptionOptions) string {
	if err := m.checkClosed(); err != nil {
		return "" // Or handle error appropriately, e.g., panic or log
	}
	return m.wrapped.RegisterSubscription(options)
}

// Schema delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Schema(id string, version ...string) (*schema.SchemaDefinition, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Schema(id, version...)
}

// Subscriptions delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Subscriptions() ([]base.SubscriptionInfo, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Subscriptions()
}

// Transact delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Transact(callback func(tx base.BasePersistence) (any, error)) (any, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Transact(callback)
}

// UnregisterSubscription delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) UnregisterSubscription(id string) {
	if err := m.checkClosed(); err != nil {
		return
	}
	m.wrapped.UnregisterSubscription(id)
}

// Rollback delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Rollback(
	name string,
	version *string,
	dryRun *bool,
) (base.Collection, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Rollback(name, version, dryRun)
}

// Migrate delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Migrate(
	name string,
	migration schema.Migration,
	dryRun *bool,
) (base.Collection, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Migrate(name, migration, dryRun)
}
