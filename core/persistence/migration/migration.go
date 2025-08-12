package migration

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/data"
)

// Transformer is a function that takes a document conforming to an old schema
// and transforms it into a document conforming to a new schema.
type Transformer func(ctx context.Context, sourceDoc data.Document) (data.Document, error)

// DataMigrator defines the interface for a service that handles the migration
// of data from a collection of one version to another.
type DataMigrator interface {
	// Migrate runs a data migration process. It reads all documents from the source
	// collection version, applies the transformer function to each, and writes the
	// resulting documents to the destination collection version.
	//
	// This is expected to be a long-running, stateful operation that can be
	// monitored.
	//
	// Parameters:
	//   - ctx: The context for the migration job.
	//   - collectionName: The logical name of the collection being migrated.
	//   - sourceVersion: The version string of the source schema.
	//   - destVersion: The version string of the destination schema.
	//   - transformer: The function that converts a document from source to destination format.
	//
	// Returns:
	//   - A job ID for monitoring the migration's progress.
	//   - An error if the migration could not be started (e.g., versions not found).
	Migrate(
		ctx context.Context,
		collectionName, sourceVersion, destVersion string,
		transformer Transformer,
	) (string, error)
}
