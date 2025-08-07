package persistence

import (
	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
)


type Document common.Document
type Collection base.Collection
type Persistence base.Persistence

type PersistenceEventType base.PersistenceEventType

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
)
