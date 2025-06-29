// Package persistence provides the interfaces and types for database operations.
// It defines a structured and extensible framework for data storage, retrieval,
// management, and observability within the system. This package establishes the
// core contracts that any underlying database driver must implement, ensuring a
// consistent API for data manipulation regardless of the storage technology used.
package persistence

import (
	"context"
	"encoding/json"

	"github.com/asaidimu/go-anansi/v2/core/query"
	"github.com/asaidimu/go-anansi/v2/core/schema"
)

// PersistenceEventType defines the set of possible event types that can be emitted
// by the persistence layer. These events are crucial for observability, allowing other
// parts of the system to react to data changes, trigger workflows, or collect metrics.
// Each event corresponds to a specific stage of a persistence operation.
type PersistenceEventType string

const (
	// DocumentCreateStart is an event triggered just before a document creation attempt.
	DocumentCreateStart PersistenceEventType = "document:create:start"
	// DocumentCreateSuccess is an event triggered after a document has been successfully created.
	DocumentCreateSuccess PersistenceEventType = "document:create:success"
	// DocumentCreateFailed is an event triggered when a document creation operation fails.
	DocumentCreateFailed PersistenceEventType = "document:create:failed"
	// DocumentReadStart is an event triggered just before a document read operation begins.
	DocumentReadStart PersistenceEventType = "document:read:start"
	// DocumentReadSuccess is an event triggered after a document has been successfully read.
	DocumentReadSuccess PersistenceEventType = "document:read:success"
	// DocumentReadFailed is an event triggered when a document read operation fails.
	DocumentReadFailed PersistenceEventType = "document:read:failed"
	// DocumentUpdateStart is an event triggered just before a document update operation begins.
	DocumentUpdateStart PersistenceEventType = "document:update:start"
	// DocumentUpdateSuccess is an event triggered after a document has been successfully updated.
	DocumentUpdateSuccess PersistenceEventType = "document:update:success"
	// DocumentUpdateFailed is an event triggered when a document update operation fails.
	DocumentUpdateFailed PersistenceEventType = "document:update:failed"
	// DocumentDeleteStart is an event triggered just before a document deletion operation begins.
	DocumentDeleteStart PersistenceEventType = "document:delete:start"
	// DocumentDeleteSuccess is an event triggered after a document has been successfully deleted.
	DocumentDeleteSuccess PersistenceEventType = "document:delete:success"
	// DocumentDeleteFailed is an event triggered when a document deletion operation fails.
	DocumentDeleteFailed PersistenceEventType = "document:delete:failed"
	// MigrateStart is an event triggered before a schema migration is applied.
	MigrateStart PersistenceEventType = "migrate:start"
	// MigrateSuccess is an event triggered after a schema migration has been successfully applied.
	MigrateSuccess PersistenceEventType = "migrate:success"
	// MigrateFailed is an event triggered when a schema migration fails.
	MigrateFailed PersistenceEventType = "migrate:failed"
	// RollbackStart is an event triggered before a schema rollback begins.
	RollbackStart PersistenceEventType = "rollback:start"
	// RollbackSuccess is an event triggered after a schema rollback has been successfully completed.
	RollbackSuccess PersistenceEventType = "rollback:success"
	// RollbackFailed is an event triggered when a schema rollback fails.
	RollbackFailed PersistenceEventType = "rollback:failed"
	// TransactionStart is an event triggered when a database transaction begins.
	TransactionStart PersistenceEventType = "transaction:start"
	// TransactionSuccess is an event triggered when a database transaction is successfully committed.
	TransactionSuccess PersistenceEventType = "transaction:success"
	// TransactionFailed is an event triggered when a database transaction fails and is rolled back.
	TransactionFailed PersistenceEventType = "transaction:failed"
	// Telemetry is a generic event type for publishing telemetry data.
	Telemetry PersistenceEventType = "telemetry"
	// CollectionCreateStart is an event triggered before a new collection is created.
	CollectionCreateStart PersistenceEventType = "collection:create:start"
	// CollectionCreateSuccess is an event triggered after a new collection has been successfully created.
	CollectionCreateSuccess PersistenceEventType = "collection:create:success"
	// CollectionCreateFailed is an event triggered when a collection creation operation fails.
	CollectionCreateFailed PersistenceEventType = "collection:create:failed"
	// CollectionDeleteStart is an event triggered before a collection is deleted.
	CollectionDeleteStart PersistenceEventType = "collection:delete:start"
	// CollectionDeleteSuccess is an event triggered after a collection has been successfully deleted.
	CollectionDeleteSuccess PersistenceEventType = "collection:delete:success"
	// CollectionDeleteFailed is an event triggered when a collection deletion operation fails.
	CollectionDeleteFailed PersistenceEventType = "collection:delete:failed"
	// SubscriptionRegister is an event triggered when a new event subscription is registered.
	SubscriptionRegister PersistenceEventType = "subscription:register"
	// SubscriptionUnregister is an event triggered when an event subscription is removed.
	SubscriptionUnregister PersistenceEventType = "subscription:unregister"
	// MetadataCalled is an event triggered when a request for metadata is made.
	MetadataCalled PersistenceEventType = "metadata:called"
)

// Issue represents a detailed validation or operational issue. It is used to provide
// structured, machine-readable feedback about problems encountered during an operation,
// which is particularly useful for form validation or API error responses.
type Issue struct {
	Code        string `json:"code"`                  // Code is a machine-readable identifier for the type of issue (e.g., "validation_error", "not_found").
	Message     string `json:"message"`               // Message is a human-readable description of the issue.
	Path        string `json:"path,omitempty"`        // Path indicates the location of the issue, such as a field name in a JSON document (e.g., "user.address.zipCode").
	Severity    string `json:"severity,omitempty"`    // Severity indicates the seriousness of the issue, typically "error" or "warning".
	Description string `json:"description,omitempty"` // Description provides a more detailed, potentially multi-line explanation of the issue and how to resolve it.
}

// PersistenceEvent is the base struct for all events emitted by the persistence layer.
// It contains a common set of fields that provide context about the event, such as the
// event type, timestamp, and the operation being performed. Specific event types embed
// this struct and add their own relevant fields.
type PersistenceEvent struct {
	Type          PersistenceEventType `json:"type"`                    // Type is the specific type of the event.
	Timestamp     int64                `json:"timestamp"`               // Timestamp is when the event occurred, as a Unix timestamp in milliseconds.
	Operation     string               `json:"operation"`               // Operation is the name of the action being performed (e.g., "create", "update").
	Collection    *string              `json:"collection,omitempty"`    // Collection is the name of the collection on which the operation was performed.
	Input         any                  `json:"input,omitempty"`         // Input is the data that was passed to the operation.
	Output        any                  `json:"output,omitempty"`        // Output is the data that was returned by the operation.
	Error         *string              `json:"error,omitempty"`         // Error is the error message if the operation failed.
	Issues        []Issue              `json:"issues,omitempty"`        // Issues is a list of validation or other issues that occurred.
	Query         any                  `json:"query,omitempty"`         // Query is the query used in the operation, if applicable.
	TransactionID *string              `json:"transactionId,omitempty"` // TransactionID is the identifier for the transaction, if the operation was part of one.
	Duration      *int64               `json:"duration,omitempty"`      // Duration is the time the operation took to complete, in milliseconds.
	Context       map[string]any       `json:"context,omitempty"`       // Context provides additional, arbitrary metadata specific to the operation.
}

// TelemetryEvent is a specific type of PersistenceEvent used for publishing arbitrary
// telemetry data. It embeds the base PersistenceEvent and adds a Data field for the payload.
type TelemetryEvent struct {
	PersistenceEvent
	Data map[string]any `json:"data"` // Data contains the arbitrary telemetry data.
}

// SubscriptionEvent is a specific type of PersistenceEvent related to managing subscriptions.
// It provides details about the subscription being registered or unregistered.
type SubscriptionEvent struct {
	PersistenceEvent
	EventName  string `json:"eventName"` // EventName is the name of the event that was subscribed to or unsubscribed from.
	CallbackID string `json:"callbackId"`// CallbackID is the unique identifier for the subscription's callback function.
}

// PersistenceOperationEvent is a specific type of PersistenceEvent for document-level
// operations (create, read, update, delete). It includes details like the document ID
// and the number of documents affected.
type PersistenceOperationEvent struct {
	PersistenceEvent
	DocumentID  *string `json:"documentId,omitempty"`  // DocumentID is the unique identifier of the document that was affected.
	ChangeCount *int64  `json:"changeCount,omitempty"` // ChangeCount is the number of documents that were changed by the operation.
}

// MigrationEvent is a specific type of PersistenceEvent for schema migration operations.
// It includes the target schema version and a description of the migration.
type MigrationEvent struct {
	PersistenceEvent
	SchemaVersion string `json:"schemaVersion"` // SchemaVersion is the version of the schema being migrated to.
	Description   string `json:"description"`   // Description is a human-readable summary of the migration.
}

// RollbackEvent is a specific type of PersistenceEvent for schema rollback operations.
// It includes the version to which the schema is being rolled back.
type RollbackEvent struct {
	PersistenceEvent
	TargetVersion string `json:"targetVersion"` // TargetVersion is the schema version that is the target of the rollback.
}

// TransactionEvent is a specific type of PersistenceEvent for database transaction operations.
// It includes the unique identifier for the transaction.
type TransactionEvent struct {
	PersistenceEvent
	TransactionID string `json:"transactionId"` // TransactionID is the unique identifier for the transaction.
}

// EventCallbackFunction defines the function signature for event listeners.
// Callbacks receive the execution context and the event that was triggered.
type EventCallbackFunction func(ctx context.Context, event PersistenceEvent) error

// SubscriptionInfo describes a registered subscription. It holds the necessary information
// to identify, describe, and manage the lifecycle of a subscription, including a function
// to unsubscribe.
type SubscriptionInfo struct {
	Id          *string              `json:"id"`                      // Id is the unique identifier for the subscription.
	Event       PersistenceEventType `json:"event"`                   // Event is the type of event that this subscription listens for.
	Label       *string              `json:"label,omitempty"`         // Label is an optional, human-readable identifier for the subscription.
	Description *string              `json:"description,omitempty"`   // Description provides more detail about what the subscription does.
	Unsubscribe func()                                                // Unsubscribe is a function that, when called, will unregister the subscription.
}

// MetadataFilter provides criteria for filtering metadata queries. This allows clients
// to request a specific subset of metadata, for example, by filtering on subscription
// labels or schema IDs.
type MetadataFilter struct {
	Subscriptions *struct {
		Event *json.RawMessage `json:"event,omitempty"` // Event filters subscriptions by event type. Can be a single event or an array of events.
		Label *string          `json:"label,omitempty"` // Label filters subscriptions by their assigned label.
	} `json:"subscriptions,omitempty"`
	Schemas *struct {
		ID *string `json:"id,omitempty"` // ID filters schemas by their unique identifier.
	} `json:"schemas,omitempty"`
}

// MigrationMetadata describes the metadata of a single schema migration. It provides a
// complete history of a migration's lifecycle, including its status, timestamps, and
// any errors that occurred.
type MigrationMetadata struct {
	ID             string  `json:"id"`                  // ID is the unique identifier for the migration.
	SchemaVersion  string  `json:"schemaVersion"`       // SchemaVersion is the version of the schema after this migration is applied.
	Description    string  `json:"description"`         // Description is a human-readable summary of the changes in this migration.
	Status         string  `json:"status"`              // Status indicates the current state of the migration (e.g., "pending", "applied", "failed", "rolledback").
	Checksum       string  `json:"checksum"`            // Checksum is a hash of the migration script, used to verify its integrity.
	CreatedAt      int64   `json:"createdAt"`           // CreatedAt is the timestamp when the migration was created (Unix milliseconds).
	LastModifiedAt int64   `json:"lastModifiedAt"`      // LastModifiedAt is the timestamp when the migration was last modified (Unix milliseconds).
	StartedAt      *int64  `json:"startedAt,omitempty"` // StartedAt is the timestamp when the migration process began (Unix milliseconds).
	CompletedAt    *int64  `json:"completedAt,omitempty"`// CompletedAt is the timestamp when the migration process finished (Unix milliseconds).
	Error          *string `json:"error,omitempty"`     // Error contains the error message if the migration failed.
}

// TransformationMetadata describes the metadata of a single data transformation,
// which is typically part of a schema migration. It details the change from one
// schema version to another.
type TransformationMetadata struct {
	ID                string  `json:"id"`                  // ID is the unique identifier for the transformation.
	Name              string  `json:"name"`                // Name is a human-readable name for the transformation.
	FromSchemaVersion string  `json:"fromSchemaVersion"`   // FromSchemaVersion is the schema version before the transformation.
	ToSchemaVersion   string  `json:"toSchemaVersion"`     // ToSchemaVersion is the schema version after the transformation.
	Description       string  `json:"description"`         // Description is a summary of the transformation's purpose.
	CreatedAt         int64   `json:"createdAt"`           // CreatedAt is the timestamp when the transformation was created (Unix milliseconds).
	LastModifiedAt    int64   `json:"lastModifiedAt"`      // LastModifiedAt is the timestamp when the transformation was last modified (Unix milliseconds).
	Status            string  `json:"status"`              // Status indicates the current state of the transformation (e.g., "pending", "applied", "failed").
	Checksum          string  `json:"checksum"`            // Checksum is a hash of the transformation script to ensure its integrity.
	Error             *string `json:"error,omitempty"`     // Error contains the error message if the transformation failed.
}

// CollectionMetadata provides comprehensive metadata for a single collection.
// It includes descriptive information, schema details, usage statistics, and
// associated operational data like migrations and subscriptions.
type CollectionMetadata struct {
	ID               string                   `json:"id"`                         // ID is the unique identifier for the collection.
	SchemaVersion    string                   `json:"schemaVersion"`              // SchemaVersion is the version of the schema currently used by the collection.
	Name             string                   `json:"name"`                       // Name is the logical name of the collection.
	CollectionName   string                   `json:"collectionName"`             // CollectionName is the physical name of the collection in the database.
	Description      string                   `json:"description"`                // Description is a human-readable summary of the collection's purpose.
	Status           string                   `json:"status"`                     // Status indicates the current state of the collection (e.g., "active", "archived").
	CreatedAt        string                   `json:"createdAt"`                  // CreatedAt is the timestamp when the collection was created.
	CreatedBy        string                   `json:"createdBy"`                  // CreatedBy identifies the user or process that created the collection.
	RecordCount      int64                    `json:"recordCount"`                // RecordCount is the number of records currently in the collection.
	DataSizeBytes    int64                    `json:"dataSizeBytes"`              // DataSizeBytes is the total size of the data in the collection, in bytes.
	Schema           schema.SchemaDefinition  `json:"schema"`                     // Schema is the schema definition associated with this collection.
	LastModified     int64                    `json:"lastModified"`               // LastModified is the timestamp of the last operation on the collection (Unix milliseconds).
	ConnectionStatus *string                  `json:"connectionStatus,omitempty"` // ConnectionStatus indicates the health of the connection to the collection (e.g., "connected", "disconnected", "error").
	ConnectionError  *string                  `json:"connectionError,omitempty"`  // ConnectionError contains an error message if the connection is in an error state.
	Labels           []string                 `json:"labels,omitempty"`           // Labels are tags associated with the collection for organization and filtering.
	Migrations       []MigrationMetadata      `json:"migrations,omitempty"`       // Migrations is a list of all schema migrations that have been applied to this collection.
	Transformations  []TransformationMetadata `json:"transformations,omitempty"`  // Transformations is a list of all data transformations that have been applied to this collection.
	Subscriptions    []SubscriptionInfo       `json:"subscriptions"`              // Subscriptions is a list of all active event subscriptions for this collection.
}

// Metadata represents the overall metadata for the entire persistence layer.
// It can provide a global overview, including aggregate statistics and lists
// of all schemas, collections, and subscriptions across the system.
type Metadata struct {
	CollectionCount   *int64                    `json:"collectionCount,omitempty"`   // CollectionCount is the total number of collections in the system.
	StorageUsageBytes *int64                    `json:"storageUsageBytes,omitempty"` // StorageUsageBytes is the total storage used by all collections, in bytes.
	ConnectionStatus  *string                   `json:"connectionStatus,omitempty"`  // ConnectionStatus indicates the health of the main connection to the persistence layer.
	ConnectionError   *string                   `json:"connectionError,omitempty"`   // ConnectionError contains an error message if the main connection has failed.
	Schemas           []schema.SchemaDefinition `json:"schemas,omitempty"`           // Schemas is a list of all schema definitions available in the system.
	Collections       []CollectionMetadata      `json:"collections,omitempty"`       // Collections is a list of metadata for all collections in the system.
	Subscriptions     []SubscriptionInfo        `json:"subscriptions"`               // Subscriptions is a list of all active subscriptions at the global level.
}

// CreateResult defines the structure of the response for a successful create operation.
type CreateResult struct {
	ID   string `json:"id"`   // ID is the unique identifier of the newly created document.
	Data any    `json:"data"` // Data is the content of the newly created document.
}

// UpdateResult defines the structure of the response for a successful update operation.
type UpdateResult struct {
	ID      string `json:"id"`      // ID is the unique identifier of the updated document.
	Data    any    `json:"data"`    // Data is the content of the document after the update.
	Changed bool   `json:"changed"` // Changed is a boolean flag indicating whether the operation resulted in a change to the document.
}

// DeleteResult defines the structure of the response for a successful delete operation.
type DeleteResult struct {
	Count int64 `json:"count"` // Count is the number of documents that were deleted.
}

// CreateCollectionOptions defines the parameters required to create a new collection.
type CreateCollectionOptions struct {
	Name        string                  `json:"name"`        // Name is the logical name for the new collection.
	Description string                  `json:"description"` // Description is a human-readable summary of the collection's purpose.
	Schema      schema.SchemaDefinition `json:"schema"`      // Schema is the schema definition that documents in this collection must adhere to.
	Labels      []string                `json:"labels,omitempty"` // Labels are optional tags to associate with the collection for organization.
}

// MigrateOptions defines the parameters for a schema migration operation.
type MigrateOptions struct {
	ID string `json:"id"` // ID is the unique identifier of the migration to be applied.
}

// RollbackOptions defines the parameters for a schema rollback operation.
type RollbackOptions struct {
	ID string `json:"id"` // ID is the unique identifier of the migration to be rolled back.
}

// RegisterSubscriptionOptions defines the parameters required to register a new event subscription.
type RegisterSubscriptionOptions struct {
	Event       PersistenceEventType  `json:"event"`                 // Event is the type of event to subscribe to.
	Label       *string               `json:"label,omitempty"`       // Label is an optional, human-readable identifier for the subscription.
	Description *string               `json:"description,omitempty"` // Description provides more detail about what the subscription does.
	Callback    EventCallbackFunction                              // Callback is the function that will be executed when the event is triggered.
}

// UpdateOptions defines the parameters for an update operation.
type UpdateOptions struct {
	Upsert *bool `json:"upsert,omitempty"` // Upsert, if true, creates a new document if no document matches the update query. If false, the update fails if no document is found.
}

// PersistenceInterface defines the core contract for the persistence layer. It provides a
// comprehensive set of methods for managing collections, schemas, transactions, and
// observability features like metadata and event subscriptions.
type PersistenceInterface interface {
	// Collection returns a handle to a specific collection by name, allowing for operations
	// to be performed on that collection.
	Collection(name string) (PersistenceCollectionInterface, error)
	// Collections returns a list of names of all available collections.
	Collections() ([]string, error)
	// Create creates a new collection based on the provided schema definition.
	Create(sc schema.SchemaDefinition) (PersistenceCollectionInterface, error)
	// Delete removes a collection entirely, specified by its ID.
	Delete(id string) (bool, error)
	// Schema retrieves a schema definition by its unique ID.
	Schema(id string) (*schema.SchemaDefinition, error)
	// Transact executes a series of operations within a single atomic transaction.
	// The provided callback function receives a transaction object, and if the callback
	// returns an error, the transaction is rolled back.
	Transact(callback func(tx PersistenceTransactionInterface) (any, error)) (any, error)

	// Metadata retrieves metadata about the persistence layer, optionally filtered
	// by the provided criteria.
	Metadata(
		filter *MetadataFilter,
	) (Metadata, error)

	// RegisterSubscription registers a callback function to be executed when a specific
	// persistence event occurs. It returns a unique ID for the subscription.
	RegisterSubscription(options RegisterSubscriptionOptions) string
	// UnregisterSubscription removes an active subscription, specified by its ID.
	UnregisterSubscription(id string)
	// Subscriptions returns a list of all currently active subscriptions.
	Subscriptions() ([]SubscriptionInfo, error)
}

// PersistenceTransactionInterface defines the set of operations that can be performed
// within a database transaction. It is a subset of the PersistenceInterface, ensuring
// that transactional operations are consistent with the main persistence API, but
// excluding non-transactional methods like creating new transactions.
type PersistenceTransactionInterface interface {
	// Collections returns a list of all collection names within the scope of the transaction.
	Collections() ([]string, error)
	// Create creates a new collection within the transaction.
	Create(schema schema.SchemaDefinition) (PersistenceCollectionInterface, error)
	// Delete deletes a collection within the transaction.
	Delete(id string) (bool, error)
	// Schema retrieves a schema definition within the transaction.
	Schema(id string) (*schema.SchemaDefinition, error)
	// Collection returns a handle to a specific collection within the transaction.
	Collection(name string) (PersistenceCollectionInterface, error)
	// Metadata retrieves metadata about the persistence layer within the transaction.
	Metadata(
		filter *MetadataFilter,
	) (Metadata, error)
}

// CollectionUpdate defines the parameters for an update operation on a collection.
// It specifies the data to be updated and a filter to select which documents to update.
type CollectionUpdate struct {
	Data   map[string]any     `json:"data,omitempty"` // Data contains the fields and values to be updated.
	Filter *query.QueryFilter `json:"filter"`         // Filter is a query that selects the documents to be updated.
}

// PersistenceCollectionInterface defines the contract for operations on a specific collection.
// This includes standard CRUD (Create, Read, Update, Delete) operations, as well as methods
// for schema management (migration, rollback), data validation, and collection-scoped
// observability (metadata and subscriptions).
type PersistenceCollectionInterface interface {
	// Create adds one or more new documents to the collection.
	Create(data any) (any, error)
	// Read retrieves documents from the collection that match the given QueryDSL.
	Read(query *query.QueryDSL) (*query.QueryResult, error)
	// Update modifies documents in the collection that match the filter in CollectionUpdate.
	Update(params *CollectionUpdate) (int, error)
	// Delete removes documents from the collection that match the given query filter.
	// The 'unsafe' flag can be used to bypass safety checks.
	Delete(query *query.QueryFilter, unsafe bool) (int, error)
	// Validate checks if the given data conforms to the collection's schema.
	// The 'loose' flag allows for partial validation.
	Validate(data any, loose bool) (*schema.ValidationResult, error)
	// Rollback reverts a schema migration for the collection. A specific version can be
	// targeted, and a dry run can be performed to preview the changes.
	Rollback(
		version *string,
		dryRun *bool,
	) (struct {
		Schema  schema.SchemaDefinition `json:"schema"`
		Preview any                     `json:"preview"`
	}, error)
	// Migrate applies a schema migration to the collection. It takes a description and a
	// callback function that defines the data transformation. A dry run can be performed
	// to preview the changes.
	Migrate(
		description string,
		cb func(h schema.SchemaMigrationHelper) (schema.DataTransform[any, any], error),
		dryRun *bool,
	) (struct {
		Schema  schema.SchemaDefinition `json:"schema"`
		Preview any                     `json:"preview"`
	}, error)

	// Metadata retrieves metadata specifically for this collection, with an option to
	// force a refresh of the data.
	Metadata(
		filter *MetadataFilter,
		forceRefresh bool,
	) (Metadata, error)

	// RegisterSubscription registers a subscription for an event that is specific to this collection.
	RegisterSubscription(options RegisterSubscriptionOptions) string
	// UnregisterSubscription removes a collection-specific subscription.
	UnregisterSubscription(id string)

	// Subscriptions returns a list of all active subscriptions for this collection.
	Subscriptions() ([]SubscriptionInfo, error)
}
