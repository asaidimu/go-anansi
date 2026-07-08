package migration

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/google/uuid"
)

// Transformer is a function that takes a document conforming to an old schema
// and transforms it into a document conforming to a new schema.
type Transformer func(ctx context.Context, sourceDoc data.Document) (data.Document, error)

// DataMigrator defines the interface for a service that handles the migration
// of data from a collection of one version to another.
type DataMigrator interface {
	Migrate(
		ctx context.Context,
		collectionName, sourceVersion, destVersion string,
		transformer Transformer,
	) (string, error)
}

// DefaultDataMigrator reads documents from the source schema version,
// applies the Transformer, and writes into the destination schema version.
type DefaultDataMigrator struct {
	interactor query.DatabaseInteractor
	registry   base.CollectionRegistry
}

func NewDefaultDataMigrator(interactor query.DatabaseInteractor, registry base.CollectionRegistry) *DefaultDataMigrator {
	return &DefaultDataMigrator{
		interactor: interactor,
		registry:   registry,
	}
}

// Migrate copies every document from the source version, transforms it, and
// inserts it into the destination version. It returns a job ID on success.
func (m *DefaultDataMigrator) Migrate(
	ctx context.Context,
	collectionName, sourceVersion, destVersion string,
	transformer Transformer,
) (string, error) {
	if transformer == nil {
		return "", fmt.Errorf("transformer is required for data migration")
	}

	srcSchema, err := m.registry.GetSchema(ctx, collectionName, sourceVersion)
	if err != nil {
		return "", fmt.Errorf("get source schema: %w", err)
	}

	srcPhysical, err := m.registry.ResolvePhysicalName(ctx, collectionName, sourceVersion)
	if err != nil {
		return "", fmt.Errorf("get source physical name: %w", err)
	}

	dstSchema, err := m.registry.GetSchema(ctx, collectionName, destVersion)
	if err != nil {
		return "", fmt.Errorf("get dest schema: %w", err)
	}

	jobID := uuid.New().String()

	readQuery := &query.Query{
		Target: &query.QueryTarget{
			Name:   srcPhysical,
			Schema: srcSchema,
		},
	}
	rows, _, err := m.interactor.SelectDocuments(ctx, srcSchema, readQuery)
	if err != nil {
		return "", fmt.Errorf("read source documents: %w", err)
	}

	insertBatch := make([]map[string]any, 0, len(rows))

	for _, row := range rows {
		doc, ok := data.DocumentFrom(row)
		if !ok {
			return jobID, fmt.Errorf("failed to convert row to document")
		}

		transformed, tErr := func() (transformed_ data.Document, tErr_ error) {
			defer func() {
				if r := recover(); r != nil {
					tErr_ = fmt.Errorf("transformer panic: %v", r)
				}
			}()
			transformed_, tErr_ = transformer(ctx, *doc)
			return
		}()
		if tErr != nil {
			return jobID, fmt.Errorf("transform error: %w", tErr)
		}

		insertBatch = append(insertBatch, transformed.ToMap())
	}

	if len(insertBatch) == 0 {
		return jobID, nil
	}

	if _, err := m.interactor.InsertDocuments(ctx, dstSchema, insertBatch); err != nil {
		return "", fmt.Errorf("write to destination: %w", err)
	}

	return jobID, nil
}
