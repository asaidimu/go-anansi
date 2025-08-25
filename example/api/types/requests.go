package types

import (
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// CollectionCreateRequest represents the request body for creating documents
type CollectionCreateRequest struct {
	Documents []data.Document `json:"documents"`
}

// CollectionReadRequest represents the request body for reading documents
type CollectionReadRequest struct {
	Query *query.Query `json:"query,omitempty"`
}

// CollectionUpdateRequest represents the request body for updating documents
type CollectionUpdateRequest struct {
	Data    data.Document      `json:"data"`
	Filter  *query.QueryFilter `json:"filter"`
	Version *int               `json:"version,omitempty"`
	Recover bool               `json:"recover,omitempty"`
}

// CollectionDeleteRequest represents the request body for deleting documents
type CollectionDeleteRequest struct {
	Filter *query.QueryFilter `json:"filter"`
	Unsafe bool               `json:"unsafe,omitempty"`
}

// CollectionCreateCollectionRequest represents the request for creating a new collection
type CollectionCreateCollectionRequest struct {
	Schema schema.SchemaDefinition `json:"schema"`
}

// CollectionSchemaRequest represents the request for getting a collection schema
type CollectionSchemaRequest struct {
	Name    string  `json:"name"`
	Version *string `json:"version,omitempty"`
}

// CollectionDeleteCollectionRequest represents the request for deleting a collection
type CollectionDeleteCollectionRequest struct {
	Name string `json:"name"`
}

// CollectionValidateRequest represents the request for validating a document
type CollectionValidateRequest struct {
	Data  data.Document `json:"data"`
	Loose bool          `json:"loose,omitempty"`
}

// CollectionMetadataRequest represents the request for getting collection metadata
type CollectionMetadataRequest struct {
	Filter       *base.MetadataFilter `json:"filter,omitempty"`
	ForceRefresh bool                 `json:"forceRefresh,omitempty"`
}

// TransactionExecuteRequest represents the request for executing transactions
type TransactionExecuteRequest struct {
	Operations []TransactionOperation `json:"operations"`
}

// TransactionOperation represents a single operation within a transaction
type TransactionOperation struct {
	Type       string                 `json:"type"` // "create", "read", "update", "delete"
	Collection string                 `json:"collection"`
	Data       map[string]any `json:"data,omitempty"`
	Query      *query.Query           `json:"query,omitempty"`
	Filter     *query.QueryFilter     `json:"filter,omitempty"`
	Options    map[string]any `json:"options,omitempty"`
}

// SubscriptionRegisterRequest represents the request for registering subscriptions
type SubscriptionRegisterRequest struct {
	Event       base.PersistenceEventType `json:"event"`
	Label       *string                   `json:"label,omitempty"`
	Description *string                   `json:"description,omitempty"`
	Collection  *string                   `json:"collection,omitempty"` // For collection-specific subscriptions
}

// SubscriptionUnregisterRequest represents the request for unregistering subscriptions
type SubscriptionUnregisterRequest struct {
	ID         string  `json:"id"`
	Collection *string `json:"collection,omitempty"` // For collection-specific subscriptions
}

// MigrateRequest represents the request for schema migrations
type MigrateRequest struct {
	Name      string           `json:"name"`
	Migration schema.Migration `json:"migration"`
	DryRun    *bool            `json:"dryRun,omitempty"`
}

// RollbackRequest represents the request for schema rollbacks
type RollbackRequest struct {
	Name    string  `json:"name"`
	Version *string `json:"version,omitempty"`
	DryRun  *bool   `json:"dryRun,omitempty"`
}
