package core

import (
	"context"
	"encoding/json"
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
	TriggerRegister         PersistenceEventType = "trigger:register"
	TriggerUnregister       PersistenceEventType = "trigger:unregister"
	TriggerExecute          PersistenceEventType = "trigger:execute"
	TriggerFailed           PersistenceEventType = "trigger:failed"
	TaskRegister            PersistenceEventType = "task:register"
	TaskUnregister          PersistenceEventType = "task:unregister"
	TaskStart               PersistenceEventType = "task:start"
	TaskSuccess             PersistenceEventType = "task:success"
	TaskFailed              PersistenceEventType = "task:failed"
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
	Schema         SchemaDefinition `json:"schema"`           // Assuming SchemaDefinition is correctly defined elsewhere
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

// TriggerLifecycleEvent specific fields
type TriggerLifecycleEvent struct {
	PersistenceEvent
	TriggerID string `json:"triggerId"`
	Label     string `json:"label"`
}

// TaskLifecycleEvent specific fields
type TaskLifecycleEvent struct {
	PersistenceEvent
	TaskID    string `json:"taskId"`
	Label     string `json:"label"`
	Timestamp int64  `json:"timestamp"` // Time the task was started/completed
}

type CallbackFunction func(ctx context.Context, event PersistenceEvent) error

// SubscriptionInfo describes a subscription configuration.
type SubscriptionInfo struct {
	Event       PersistenceEventType `json:"event"`                 // The event subscribed to.
	Label       *string              `json:"label,omitempty"`       // Optional short identifier.
	Description *string              `json:"description,omitempty"` // Optional description.
	Unsubscribe func()
}

// TriggerInfo describes a trigger configuration.
type TriggerInfo struct {
	Event       json.RawMessage `json:"event"`               // The event(s) or pattern triggering the callback (can be string or array of strings)
	Condition   any             `json:"condition,omitempty"` // Optional condition for the trigger (Corresponds to QueryFilter).
	CallbackID  string          `json:"callbackId"`          // Unique identifier for the callback.
	IsSync      bool            `json:"isSync"`              // Whether the trigger executes synchronously.
	Label       string          `json:"label"`               // Short identifier.
	Description string          `json:"description"`         // Description of the trigger's purpose.
}

// UnmarshalJSON for TriggerInfo to handle `Event` as string or array of strings.
func (ti *TriggerInfo) UnmarshalJSON(data []byte) error {
	type Alias TriggerInfo // Create an alias to avoid infinite recursion
	aux := struct {
		Event json.RawMessage `json:"event"`
		*Alias
	}{
		Alias: (*Alias)(ti),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	ti.Event = aux.Event

	return nil
}

// TaskInfo describes a scheduled task configuration.
type TaskInfo struct {
	ID          string         `json:"id"`                 // Unique identifier for the task.
	Schedule    TaskSchedule   `json:"schedule"`           // Schedule for task execution.
	CallbackID  string         `json:"callbackId"`         // Unique identifier for the callback.
	IsSync      bool           `json:"isSync"`             // Whether the task executes synchronously.
	Metadata    map[string]any `json:"metadata,omitempty"` // Optional metadata.
	Label       string         `json:"label"`              // Short identifier.
	Description string         `json:"description"`        // Description of the task's purpose.
}

// TaskSchedule defines a schedule for a task. This uses omitempty to handle the union.
type TaskSchedule struct {
	Cron     *string `json:"cron,omitempty"`     // e.g., "0 0 * * *"
	At       *string `json:"at,omitempty"`       // ISO 8601
	Interval *int64  `json:"interval,omitempty"` // Milliseconds
}

// MetadataFilter filter criteria for metadata queries.
// Matches the nested structure of TypeScript's MetadataFilter.
type MetadataFilter struct {
	Subscriptions *struct {
		Event *json.RawMessage `json:"event,omitempty"` // PersistenceEventType or []PersistenceEventType
		Label *string          `json:"label,omitempty"`
	} `json:"subscriptions,omitempty"`
	Triggers *struct {
		Event *json.RawMessage `json:"event,omitempty"` // PersistenceEventType or []PersistenceEventType or `${string}:*`
		Label *string          `json:"label,omitempty"`
	} `json:"triggers,omitempty"`
	Tasks *struct {
		ID       *string        `json:"id,omitempty"`
		Metadata map[string]any `json:"metadata,omitempty"`
		Label    *string        `json:"label,omitempty"`
	} `json:"tasks,omitempty"`
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
	Schema           SchemaDefinition         `json:"schema"`                     // Schema definition (reference existing SchemaDefinition).
	LastModified     int64                    `json:"lastModified"`               // Timestamp of last operation (Unix milliseconds).
	ConnectionStatus *string                  `json:"connectionStatus,omitempty"` // "connected" | "disconnected" | "error"
	ConnectionError  *string                  `json:"connectionError,omitempty"`
	Labels           []string                 `json:"labels,omitempty"`
	Migrations       []MigrationMetadata      `json:"migrations,omitempty"`      // Now using defined MigrationMetadata
	Transformations  []TransformationMetadata `json:"transformations,omitempty"` // Now using defined TransformationMetadata
	Subscriptions    []SubscriptionInfo       `json:"subscriptions"`             // Active subscriptions.
	Triggers         []TriggerInfo            `json:"triggers"`                  // Active triggers.
	Tasks            []TaskInfo               `json:"tasks"`                     // Scheduled tasks.
}

// Metadata represents the overall persistence metadata (global or for a specific collection).
// This corresponds to the comprehensive Metadata type in TypeScript, which can also include
// fields relevant to a single collection.
type Metadata struct {
	CollectionCount   *int64               `json:"collectionCount,omitempty"`
	StorageUsageBytes *int64               `json:"storageUsageBytes,omitempty"`
	ConnectionStatus  *string              `json:"connectionStatus,omitempty"`
	ConnectionError   *string              `json:"connectionError,omitempty"`
	Schemas           []SchemaDefinition   `json:"schemas,omitempty"`
	Collections       []CollectionMetadata `json:"collections,omitempty"`
	Subscriptions     []SubscriptionInfo   `json:"subscriptions"`
	Triggers          []TriggerInfo        `json:"triggers"`
	Tasks             []TaskInfo           `json:"tasks"`
	// These fields are optionally present if this Metadata instance also represents a single collection's metadata (union in TS)
	RecordCount   *int64            `json:"recordCount,omitempty"`
	DataSizeBytes *int64            `json:"dataSizeBytes,omitempty"`
	Schema        *SchemaDefinition `json:"schema,omitempty"` // Note: Pointer, as it's optional for global metadata.
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
	Schema      SchemaDefinition `json:"schema"` // SchemaDefinition[T, FunctionMap]
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
	Callback    CallbackFunction
}

// RegisterTriggerOptions defines options for registering a trigger.
type RegisterTriggerOptions struct {
	Event       json.RawMessage `json:"event"`               // PersistenceEventType or []PersistenceEventType
	Filter      any             `json:"condition,omitempty"` // QueryFilter (kept as any)
	IsSync      bool            `json:"isSync"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
	Callback    CallbackFunction
}

// UnregisterTriggerOptions defines options for unregistering a trigger.
type UnregisterTriggerOptions struct {
	CallbackID string `json:"callbackId"`
}

// RegisterTaskOptions defines options for registering a task.
// Note: In TypeScript, 'schedule' returns a cancel function. In Go interfaces, we define the 'Register'
// and 'Unregister' methods explicitly.
type RegisterTaskOptions struct {
	Schedule    TaskSchedule   `json:"schedule"`
	CallbackID  string         `json:"callbackId"`
	IsSync      bool           `json:"isSync"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
}

// UnregisterTaskOptions defines options for unregistering a task.
type UnregisterTaskOptions struct {
	CallbackID string `json:"callbackId"`
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
	Create(schema SchemaDefinition) (PersistenceCollectionInterface, error) // Returns PersistenceCollection<T, FunctionMap>
	Delete(id string) (bool, error)
	Schema(id string) (*SchemaDefinition, error)
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
	RegisterTrigger(options RegisterTriggerOptions) (TriggerInfo, error)
	UnregisterTrigger(options UnregisterTriggerOptions) error
	RegisterTask(options RegisterTaskOptions) (TaskInfo, error)
	UnregisterTask(options UnregisterTaskOptions) error

	// Getter methods for registered entities
	Subscriptions() ([]SubscriptionInfo, error)
	Triggers() ([]TriggerInfo, error)
	Tasks() ([]TaskInfo, error)
}

// PersistenceTransaction interface, omitting subscribe, trigger, schedule, and transact methods.
// In Go, this is an interface defining a subset of Persistence methods.
type PersistenceTransactionInterface interface {
	// Include all methods of Persistence except those explicitly Omitted in TS
	Collections() ([]string, error)
	Create(schema SchemaDefinition) (PersistenceCollectionInterface, error)
	Delete(id string) (bool, error)
	Schema(id string) (*SchemaDefinition, error)
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
	Filter any            `json:"filter"`         // QueryFilter<T, FunctionMap>
}
type ValidationResult struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues"`
}

// PersistenceCollection defines the interface for operations on a specific collection.
// It extends ObservabilityInterface and EventTaskInterface implicitly.
// T generic from TypeScript is represented by 'any' in method signatures for data.
type PersistenceCollectionInterface interface {
	Create(data any) (any, error) // T | T[]
	Read(query any) (any, error)
	Update(params *CollectionUpdate) (int, error) // Array<T>
	Delete(query any, unsafe bool) (int, error)
	Validate(data any, loose bool) (*ValidationResult, error)
	Rollback(
		version *string,
		dryRun *bool,
	) (struct {
		Schema  SchemaDefinition `json:"schema"`
		Preview any              `json:"preview"`
	}, error)
	Migrate(
		description string,
		cb func(h SchemaMigrationHelper) (DataTransform[any, any], error),
		dryRun *bool,
	) (struct {
		Schema  SchemaDefinition `json:"schema"`
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
	RegisterTrigger(options RegisterTriggerOptions) (TriggerInfo, error)
	UnregisterTrigger(options UnregisterTriggerOptions) error
	RegisterTask(options RegisterTaskOptions) (TaskInfo, error)
	UnregisterTask(options UnregisterTaskOptions) error

	// Getter methods for registered entities (collection-scoped)
	Subscriptions() ([]SubscriptionInfo, error)
	Triggers() ([]TriggerInfo, error)
	Tasks() ([]TaskInfo, error)
}

// CollectionTriggerContext context provided to collection-specific trigger callbacks.
// T and FunctionMap are replaced by 'any' or 'map[string]any'.
type CollectionTriggerContext struct {
	Event       PersistenceEvent               `json:"event"`              // The event that triggered the callback
	Persistence PersistenceInterface           `json:"persistence"`        // The Persistence interface
	Collection  PersistenceCollectionInterface `json:"collection"`         // The PersistenceCollection interface
	Params      any                            `json:"params"`             // Parameters passed to the trigger (union type simplified to any)
	Results     any                            `json:"results"`            // Results from the operation that triggered this (union type simplified to any)
	Metadata    map[string]any                 `json:"metadata,omitempty"` // Optional metadata for the trigger itself
	Label       string                         `json:"label"`              // Label of the trigger
	Description string                         `json:"description"`        // Description of the trigger
}

// PersistenceTriggerContext context provided to global trigger callbacks.
// FunctionMap is replaced by 'map[string]any'.
type PersistenceTriggerContext struct {
	Event       PersistenceEvent                `json:"event"`                // The event that triggered the callback
	Persistence PersistenceInterface            `json:"persistence"`          // The Persistence interface
	Collection  *PersistenceCollectionInterface `json:"collection,omitempty"` // Optional: The PersistenceCollection interface if event is collection-related
	Params      any                             `json:"params"`               // Parameters passed to the trigger
	Results     any                             `json:"results"`              // Results from the operation that triggered this
}

// TriggerContext defines a union of trigger contexts.
// For now, we'll represent it as an 'any' which can hold either struct.
type TriggerContext any // This will be either CollectionTriggerContext or PersistenceTriggerContext

// TaskContext context provided to task callbacks.
// T and FunctionMap are replaced by 'any' or 'map[string]any'.
// This is a union type in TypeScript. In Go, we'll use a common struct with optional fields
// or an interface if different behaviors are needed. For now, a common struct.
type TaskContext struct {
	ID          string                          `json:"id"`
	Time        int64                           `json:"time"`
	Persistence PersistenceInterface            `json:"persistence"`          // core.Persistence
	Collection  *PersistenceCollectionInterface `json:"collection,omitempty"` // *core.PersistenceCollection
	Metadata    map[string]any                  `json:"metadata,omitempty"`
	Label       string                          `json:"label"`
	Description string                          `json:"description"`
}
