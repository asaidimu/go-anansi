package storage

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

// storage_spec.go
//
// This file defines the core interfaces for the new Anansi Storage Engine architecture.
//
// As discussed (refer to the Gemini CLI chat for full context), this `StorageEngine`
// interface is the new top-level abstraction for all persistence. It unifies
// schema management, collection handling, and transactional operations,
// serving as the single pluggable point for both native (high-performance binary)
// and vendor (adapter to existing databases) implementations.
//
// The goal is to move the application away from direct DatabaseInteractor usage
// and towards a native Document-centric persistence model.

// TransactionalEngine defines the set of operations available within a transaction.
// It is a limited subset of the full StorageEngine API, ensuring that non-transactional
// operations (like creating new collections) cannot be performed within the transaction callback.
type TransactionalEngine interface {
	// Collection returns a handle to a specific collection, scoped to the current transaction.
	// All operations performed on the returned Collection will be part of the transaction.
	Collection(ctx context.Context, name string) (base.Collection, error)
}

// StorageEngine is the definitive, top-level interface for the Anansi persistence layer.
// It serves as the single entry point for all application interactions with storage,
// abstracting away the physical implementation (e.g., Native Engine vs. Vendor Engine).
//
// This interface is responsible for the entire lifecycle of collections, schema management
// (as the schema registry is integral to its function), transactions, and observability.
type StorageEngine interface {
	// Collection returns a handle to a named collection, which is the primary object
	// for performing CRUD operations.
	Collection(ctx context.Context, name string) (base.Collection, error)

	// CreateCollection creates a new collection based on the provided schema. This method
	// also registers the schema with the engine's internal schema registry.
	// It returns a handle to the newly created collection.
	CreateCollection(ctx context.Context, schema *definition.Schema) (base.Collection, error)

	// HasCollection checks if a collection with the given name exists.
	HasCollection(ctx context.Context, name string) (bool, error)

	// ListCollections returns a list of all available collection names.
	ListCollections(ctx context.Context) ([]string, error)

	// DeleteCollection completely removes a collection and its associated data and schemas.
	DeleteCollection(ctx context.Context, name string) error

	// Transact executes a series of operations within a single, atomic transaction.
	// The provided callback function receives a transaction-scoped engine. If the callback
	// returns an error, the transaction is automatically rolled back.
	Transact(ctx context.Context, callback func(ctx context.Context, tx TransactionalEngine) (any, error)) (any, error)

	// Schema retrieves a specific schema definition from the engine's registry by name and optional version.
	Schema(ctx context.Context, name string, version ...string) (*definition.Schema, error)

	// Metadata retrieves observability and diagnostic information about the storage engine.
	Metadata(ctx context.Context, filter *base.MetadataFilter) (base.Metadata, error)

	// Subscribe registers a callback for a specific persistence event, enabling real-time reactions.
	Subscribe(ctx context.Context, options base.SubscriptionOptions) string

	// Unsubscribe removes a previously registered event listener.
	Unsubscribe(ctx context.Context, id string)

	// Close gracefully shuts down the storage engine, closing any open connections or file handles.
	Close(ctx context.Context) error
}
