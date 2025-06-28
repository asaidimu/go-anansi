package persistence

import (
	"context"
	"encoding/json"

	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-anansi/core/schema"
)

// PersistenceEventType defines the possible event types for persistence operations.
type PersistenceEventType string

const (
	DocumentCreateStart     PersistenceEventType = "document:create:start"
	DocumentCreateSuccess   PersistenceEventType = "document:create:success"
	DocumentCreateFailed    PersistenceEventType = "document:create:failed"
	DocumentReadStart       PersistenceEventType = "document:read:start"
	DocumentReadSuccess     PersistenceEventType = "document:read:success"
	DocumentReadFailed      PersistenceEventType = "document:read:failed"
	DocumentUpdateStart     PersistenceEventType = "document:update:start"
	DocumentUpdateSuccess   PersistenceEventType = "document:update:success"
	DocumentUpdateFailed    PersistenceEventType = "document:update:failed"
	DocumentDeleteStart     PersistenceEventType = "document:delete:start"
	DocumentDeleteSuccess   PersistenceEventType = "document:delete:success"
	DocumentDeleteFailed    PersistenceEventType = "document:delete:failed"
	MigrateStart            PersistenceEventType = "migrate:start"
	MigrateSuccess          PersistenceEventType = "migrate:success"
	MigrateFailed           PersistenceEventType = "migrate:failed"
	RollbackStart           PersistenceEventType = "rollback:start"
	RollbackSuccess         PersistenceEventType = "rollback:success"
	RollbackFailed          PersistenceEventType = "rollback:failed"
	TransactionStart        PersistenceEventType = "transaction:start"
	TransactionSuccess      PersistenceEventType = "transaction:success"
	TransactionFailed       PersistenceEventType = "transaction:failed"
	Telemetry               PersistenceEventType = "telemetry"
	CollectionCreateStart   PersistenceEventType = "collection:create:start"
	CollectionCreateSuccess PersistenceEventType = "collection:create:success"
	CollectionCreateFailed  PersistenceEventType = "collection:create:failed"
	CollectionDeleteStart   PersistenceEventType = "collection:delete:start"
	CollectionDeleteSuccess PersistenceEventType = "collection:delete:success"
	CollectionDeleteFailed  PersistenceEventType = "collection:delete:failed"
	SubscriptionRegister    PersistenceEventType = "subscription:register"
	SubscriptionUnregister  PersistenceEventType = "subscription:unregister"
	MetadataCalled          PersistenceEventType = "metadata:called"
)

// Issue represents a validation or operational issue.
type Issue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Path        string `json:"path,omitempty"`
	Severity    string `json:"severity,omitempty"` // e.g., "error", "warning"
	Description string `json:"description,omitempty"`
}

// PersistenceEvent represents events emitted during persistence operations.
// This is a base struct that specific event types will embed.
// Input, Output, Query, and Results are kept as 'any' as per the specified constraints.
type PersistenceEvent struct {
	Type          PersistenceEventType `json:"type"`                    // The type of event (e.g., 'create:start', 'trigger:execute').
	Timestamp     int64                `json:"timestamp"`               // Timestamp when the event occurred (Unix milliseconds).
	Operation     string               `json:"operation"`               // The operation being performed (e.g., 'create', 'trigger').
	Collection    *string              `json:"collection,omitempty"`    // Name of the collection affected (if applicable).
	Input         any                  `json:"input,omitempty"`         // Data passed to the operation (if applicable).
	Output        any                  `json:"output,omitempty"`        // Data returned by the operation (if applicable).
	Error         *string              `json:"error,omitempty"`         // Error message if the operation failed.
	Issues        []Issue              `json:"issues,omitempty"`        // Issues that caused the operation to fail.
	Query         any                  `json:"query,omitempty"`         // Query used in the operation (if applicable). (Corresponds to QueryDSL)
	TransactionID *string              `json:"transactionId,omitempty"` // Identifier for the transaction (if part of one).
	Duration      *int64               `json:"duration,omitempty"`      // Duration of the operation in milliseconds.
	Context       map[string]any       `json:"context,omitempty"`       // Additional context or metadata specific to the operation.
}

// TelemetryEvent specific fields
type TelemetryEvent struct {
	PersistenceEvent
	Data map[string]any `json:"data"` // Arbitrary telemetry data
}

// SubscriptionEvent specific fields
type SubscriptionEvent struct {
	PersistenceEvent
	EventName  string `json:"eventName"` // The event name registered to
	CallbackID string `json:"callbackId"`
}

// CollectionEvent specific fields
type CollectionEvent struct {
	PersistenceEvent
	CollectionName string           `json:"collectionName"`
	Schema         schema.SchemaDefinition `json:"schema"`           // Assuming schema.schema.schema.SchemaDefinition is correctly defined elsewhere
	Exists         *bool            `json:"exists,omitempty"` // For create/delete success/failed
}

// PersistenceOperationEvent specific fields (for document:*:* events)
type PersistenceOperationEvent struct {
	PersistenceEvent
	DocumentID  *string `json:"documentId,omitempty"`
	ChangeCount *int64  `json:"changeCount,omitempty"`
}

// MigrationEvent specific fields
type MigrationEvent struct {
	PersistenceEvent
	SchemaVersion string `json:"schemaVersion"`
	Description   string `json:"description"`
}

// RollbackEvent specific fields
type RollbackEvent struct {
	PersistenceEvent
	TargetVersion string `json:"targetVersion"`
}

// TransactionEvent specific fields
type TransactionEvent struct {
	PersistenceEvent
	TransactionID string `json:"transactionId"`
}

type EventCallbackFunction func(ctx context.Context, event PersistenceEvent) error

// SubscriptionInfo describes a subscription configuration.
type SubscriptionInfo struct {
	Event       PersistenceEventType `json:"event"`                 // The event subscribed to.
	Label       *string              `json:"label,omitempty"`       // Optional short identifier.
	Description *string              `json:"description,omitempty"` // Optional description.
	Unsubscribe func()
}

// MetadataFilter filter criteria for metadata queries.
// Matches the nested structure of TypeScript's MetadataFilter.
type MetadataFilter struct {
	Subscriptions *struct {
		Event *json.RawMessage `json:"event,omitempty"` // PersistenceEventType or []PersistenceEventType
		Label *string          `json:"label,omitempty"`
	} `json:"subscriptions,omitempty"`
	Schemas *struct {
		ID *string `json:"id,omitempty"`
	} `json:"schemas,omitempty"`
}

// MigrationMetadata describes the metadata of a single schema migration.
type MigrationMetadata struct {
	ID             string  `json:"id"`
	SchemaVersion  string  `json:"schemaVersion"`
	Description    string  `json:"description"`
	Status         string  `json:"status"` // "pending" | "applied" | "failed" | "rolledback"
	Checksum       string  `json:"checksum"`
	CreatedAt      int64   `json:"createdAt"`      // Unix milliseconds
	LastModifiedAt int64   `json:"lastModifiedAt"` // Unix milliseconds
	StartedAt      *int64  `json:"startedAt,omitempty"`
	CompletedAt    *int64  `json:"completedAt,omitempty"`
	Error          *string `json:"error,omitempty"`
}

// TransformationMetadata describes the metadata of a single data transformation.
type TransformationMetadata struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	FromSchemaVersion string  `json:"fromSchemaVersion"`
	ToSchemaVersion   string  `json:"toSchemaVersion"`
	Description       string  `json:"description"`
	CreatedAt         int64   `json:"createdAt"`
	LastModifiedAt    int64   `json:"lastModifiedAt"`
	Status            string  `json:"status"` // "pending" | "applied" | "failed"
	Checksum          string  `json:"checksum"`
	Error             *string `json:"error,omitempty"`
}

// CollectionMetadata metadata for a single collection.
// This struct now includes all common descriptive and temporal metadata fields
// to align closely with TypeScript's CollectionMetadata.
type CollectionMetadata struct {
	ID               string                   `json:"id"` // Collection identifier.
	SchemaVersion    string                   `json:"schemaVersion"`
	Name             string                   `json:"name"`
	CollectionName   string                   `json:"collectionName"`
	Description      string                   `json:"description"`
	Status           string                   `json:"status"`
	CreatedAt        string                   `json:"createdAt"`
	CreatedBy        string                   `json:"createdBy"`
	LastModifiedAt   string                   `json:"lastModifiedAt"`
	LastModifiedBy   string                   `json:"lastModifiedBy"`
	RecordCount      int64                    `json:"recordCount"`                // Number of records.
	DataSizeBytes    int64                    `json:"dataSizeBytes"`              // Storage used in bytes.
	Schema           schema.SchemaDefinition         `json:"schema"`                     // Schema definition (reference existing schema.schema.schema.SchemaDefinition).
	LastModified     int64                    `json:"lastModified"`               // Timestamp of last operation (Unix milliseconds).
	ConnectionStatus *string                  `json:"connectionStatus,omitempty"` // "connected" | "disconnected" | "error"
	ConnectionError  *string                  `json:"connectionError,omitempty"`
	Labels           []string                 `json:"labels,omitempty"`
	Migrations       []MigrationMetadata      `json:"migrations,omitempty"`      // Now using defined MigrationMetadata
	Transformations  []TransformationMetadata `json:"transformations,omitempty"` // Now using defined TransformationMetadata
	Subscriptions    []SubscriptionInfo       `json:"subscriptions"`             // Active subscriptions.
}

// Metadata represents the overall persistence metadata (global or for a specific collection).
// This corresponds to the comprehensive Metadata type in TypeScript, which can also include
// fields relevant to a single collection.
type Metadata struct {
	CollectionCount   *int64               `json:"collectionCount,omitempty"`
	StorageUsageBytes *int64               `json:"storageUsageBytes,omitempty"`
	ConnectionStatus  *string              `json:"connectionStatus,omitempty"`
	ConnectionError   *string              `json:"connectionError,omitempty"`
	Schemas           []schema.SchemaDefinition   `json:"schemas,omitempty"`
	Collections       []CollectionMetadata `json:"collections,omitempty"`
	Subscriptions     []SubscriptionInfo   `json:"subscriptions"`
	// These fields are optionally present if this Metadata instance also represents a single collection's metadata (union in TS)
	RecordCount   *int64            `json:"recordCount,omitempty"`
	DataSizeBytes *int64            `json:"dataSizeBytes,omitempty"`
	Schema        *schema.SchemaDefinition `json:"schema,omitempty"` // Note: Pointer, as it's optional for global metadata.
	LastModified  *int64            `json:"lastModified,omitempty"`
}

// CreateResult defines the result structure for create operations.
type CreateResult struct {
	ID   string `json:"id"`
	Data any    `json:"data"` // T
}

// UpdateResult defines the result structure for update operations.
type UpdateResult struct {
	ID      string `json:"id"`
	Data    any    `json:"data"` // T
	Changed bool   `json:"changed"`
}

// DeleteResult defines the result structure for delete operations.
type DeleteResult struct {
	Count int64 `json:"count"`
}

// CreateCollectionOptions defines options for creating a new collection.
type CreateCollectionOptions struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Schema      schema.SchemaDefinition `json:"schema"` // schema.schema.schema.SchemaDefinition[T, FunctionMap]
	Labels      []string         `json:"labels,omitempty"`
}

// MigrateOptions defines options for migrating a schema.
type MigrateOptions struct {
	ID string `json:"id"` // Migration ID
}

// RollbackOptions defines options for rolling back a schema migration.
type RollbackOptions struct {
	ID string `json:"id"` // Migration ID
}

// RegisterSubscriptionOptions defines options for registering a subscription.
// Note: In TypeScript, 'subscribe' returns an unsubscribe function. In Go interfaces, we define the 'Register'
// and 'Unregister' methods explicitly. The 'callbackId' here would serve to identify the subscription for unregistration.
type RegisterSubscriptionOptions struct {
	Event       PersistenceEventType `json:"event"`
	Label       *string              `json:"label,omitempty"`
	Description *string              `json:"description,omitempty"`
	Callback    EventCallbackFunction
}

// UpdateOptions defines options for update operations.
type UpdateOptions struct {
	Upsert *bool `json:"upsert,omitempty"`
}

// Persistence defines the core persistence layer interface.
// It implicitly includes methods from ObservabilityInterface and EventTaskInterface.
type PersistenceInterface interface {
	Collection(name string) (PersistenceCollectionInterface, error)
	Transaction() error // Corresponds to `transact` but simplified for interface

	// Methods directly from Persistence in TS
	Collections() ([]string, error)
	Create(sc schema.SchemaDefinition) (PersistenceCollectionInterface, error) // Returns PersistenceCollection<T, FunctionMap>
	Delete(id string) (bool, error)
	Schema(id string) (*schema.SchemaDefinition, error)
	Transact(callback func(tx PersistenceTransactionInterface) (any, error)) (any, error) // Simplified callback signature

	// Methods from ObservabilityInterface
	Metadata(
		filter *MetadataFilter,
		includeCollections bool,
		includeSchemas bool,
		forceRefresh bool,
	) (Metadata, error)

	// Methods from EventTaskInterface (Go-idiomatic Register/Unregister pattern)
	RegisterSubscription(options RegisterSubscriptionOptions) string
	UnregisterSubscription(id string)
	// Getter methods for registered entities
	Subscriptions() ([]SubscriptionInfo, error)
}

// PersistenceTransaction interface, omitting subscribe, trigger, schedule, and transact methods.
// In Go, this is an interface defining a subset of Persistence methods.
type PersistenceTransactionInterface interface {
	Collections() ([]string, error)
	Create(schema schema.SchemaDefinition) (PersistenceCollectionInterface, error)
	Delete(id string) (bool, error)
	Schema(id string) (*schema.SchemaDefinition, error)
	Collection(name string) (PersistenceCollectionInterface, error)
	Metadata(
		filter *MetadataFilter,
		includeCollections bool,
		includeSchemas bool,
		forceRefresh bool,
	) (Metadata, error)
}

type CollectionUpdate struct {
	Data   map[string]any `json:"data,omitempty"` // Partial<T>
	Filter *query.QueryFilter    `json:"filter"`         // QueryFilter<T, FunctionMap>
}



// PersistenceCollection defines the interface for operations on a specific collection.
// It extends ObservabilityInterface and EventTaskInterface implicitly.
// T generic from TypeScript is represented by 'any' in method signatures for data.
type PersistenceCollectionInterface interface {
	Create(data any) (any, error) // T | T[]
	Read(query *query.QueryDSL) (*query.QueryResult, error)
	Update(params *CollectionUpdate) (int, error) // Array<T>
	Delete(query *query.QueryFilter, unsafe bool) (int, error)
	Validate(data any, loose bool) (*schema.ValidationResult, error)
	Rollback(
		version *string,
		dryRun *bool,
	) (struct {
		Schema  schema.SchemaDefinition `json:"schema"`
		Preview any              `json:"preview"`
	}, error)
	Migrate(
		description string,
		cb func(h schema.SchemaMigrationHelper) (schema.DataTransform[any, any], error),
		dryRun *bool,
	) (struct {
		Schema  schema.SchemaDefinition `json:"schema"`
		Preview any              `json:"preview"`
	}, error)

	// Methods from ObservabilityInterface (collection-scoped)
	Metadata(
		filter *MetadataFilter,
		forceRefresh bool,
	) (Metadata, error) // Returns collection-specific metadata

	// Methods from EventTaskInterface (Go-idiomatic Register/Unregister pattern, collection-scoped)
	RegisterSubscription(options RegisterSubscriptionOptions) string
	UnregisterSubscription(id string)

	// Getter methods for registered entities (collection-scoped)
	Subscriptions() ([]SubscriptionInfo, error)
}
