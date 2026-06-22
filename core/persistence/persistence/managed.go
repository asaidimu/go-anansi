package persistence

import (
	"context"

	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
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
		return base.ErrPersistenceClosed
	}
	return nil
}

// Close sets the closed flag and delegates the call to the wrapped persistence.
func (m *managedPersistence) Close(ctx context.Context) {
	m.closed = true
	m.wrapped.Close(ctx)
}

func (m *managedPersistence) Async(ctx context.Context, f func(ctx context.Context) (any, error)) base.Future {
	if err := m.checkClosed(); err != nil {
		return nil
	}
	return m.wrapped.Async(ctx, f)
}

// Collection delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Collection(ctx context.Context, name string) (base.Collection, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Collection(ctx, name)
}

// ListCollections delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) ListCollections(ctx context.Context) ([]string, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.ListCollections(ctx)
}

// CreateCollection delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) CreateCollection(ctx context.Context, sc *definition.Schema) (base.Collection, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.CreateCollection(ctx, sc)
}

func (m *managedPersistence) CreateCollections(ctx context.Context, schemas []*definition.Schema) error {
	if err := m.checkClosed(); err != nil {
		return err
	}
	return m.wrapped.CreateCollections(ctx, schemas)
}

func (m *managedPersistence) HasCollection(ctx context.Context, name string) (bool, error) {
	if err := m.checkClosed(); err != nil {
		return false, err
	}
	return m.wrapped.HasCollection(ctx, name)
}

// Delete delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Delete(ctx context.Context, id string) (bool, error) {
	if err := m.checkClosed(); err != nil {
		return false, err
	}
	return m.wrapped.Delete(ctx, id)
}

// Metadata delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Metadata(ctx context.Context, filter *base.MetadataFilter) (base.Metadata, error) {
	if err := m.checkClosed(); err != nil {
		return base.Metadata{}, err
	}
	return m.wrapped.Metadata(ctx, filter)
}

// Subscribe delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Subscribe(ctx context.Context, options base.SubscriptionOptions) string {
	if err := m.checkClosed(); err != nil {
		return "" // Or handle error appropriately, e.g., panic or log
	}
	return m.wrapped.Subscribe(ctx, options)
}

// Schema delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Schema(ctx context.Context, id string, version ...string) (*definition.Schema, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Schema(ctx, id, version...)
}

// Subscriptions delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Subscriptions(ctx)
}

// Transact delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Transact(ctx context.Context, callback func(ctx context.Context, tx base.BasePersistence) (any, error)) (any, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Transact(ctx, callback)
}

// Unsubscribe delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Unsubscribe(ctx context.Context, id string) {
	if err := m.checkClosed(); err != nil {
		return
	}
	m.wrapped.Unsubscribe(ctx, id)
}

// Rollback delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Rollback(
	ctx context.Context,
	name string,
	version *string,
	dryRun *bool,
) (base.Collection, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Rollback(ctx, name, version, dryRun)
}

// Migrate delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Migrate(
	ctx context.Context,
	name string,
	migration any,
	dryRun *bool,
) (base.Collection, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Migrate(ctx, name, migration, dryRun)
}

// Query delegates the call to the wrapped persistence after checking the closed state.
func (m *managedPersistence) Query(ctx context.Context, rawQuery *query.RawQuery) (*query.RawQueryResult, error) {
	if err := m.checkClosed(); err != nil {
		return nil, err
	}
	return m.wrapped.Query(ctx, rawQuery)
}
