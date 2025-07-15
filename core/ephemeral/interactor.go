package ephemeral

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	store "github.com/asaidimu/go-store/v3"
)

var (
	ErrCollectionNotFound = errors.New("collection not found")
)

type collection struct {
	Name   string
	schema *schema.SchemaDefinition
	data   *store.Store
}

// EphemeralDatabaseInteractor provides an in-memory implementation of the
// persistence.DatabaseInteractor interface. This is useful for testing or for
// applications that do not require persistent storage.
type EphemeralDatabaseInteractor struct {
	collections map[string]*collection
	mu          sync.RWMutex
}

// NewEphemeralDatabaseInteractor creates a new instance of EphemeralDatabaseInteractor.
func NewEphemeralDatabaseInteractor() *EphemeralDatabaseInteractor {
	return &EphemeralDatabaseInteractor{
		collections: make(map[string]*collection),
	}
}

func (i *EphemeralDatabaseInteractor) getCollection(name string) (*collection, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	c, ok := i.collections[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrCollectionNotFound, name)
	}
	return c, nil
}

// SelectDocuments retrieves documents from the in-memory store.
// This method needs serious optimization, especially around pagination
// We need some sort of caching mechanism here. Now, considering we are in
// memory, we can't be duplicating data all over the place.
// TODO: Joins, Caching for paginations, caching for efficiency.
func (i *EphemeralDatabaseInteractor) SelectDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, dsl *query.QueryDSL) ([]schema.Document, error) {
	c, err := i.getCollection(schemaDef.Name)
	if err != nil {
		return nil, err
	}

	queryHelper, err := query.NewQueryHelper(dsl, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	var allDocs []schema.Document
	stream := c.data.Stream(0)
	defer stream.Close()

	for {
		docResult, err := stream.Next()
		if err != nil {
			if err == store.ErrStreamClosed {
				break
			}
			return nil, err
		}
		record := schema.Document(docResult.Data)
		ok, err := queryHelper.Match(record)
		if err != nil {
			return nil, err
		}

		if !ok {
			continue
		}
		doc, err := queryHelper.ProjectSingle(record)
		if err != nil {
			return nil, err
		}
		allDocs = append(allDocs, doc)
	}

	sortedDocs, err := queryHelper.Sort(allDocs)
	if err != nil {
		return nil, err
	}

	paginatedDocs, _, err := queryHelper.Paginate(sortedDocs)
	if err != nil {
		return nil, err
	}

	return paginatedDocs, nil
}

// SelectStream streams documents from the in-memory store.
func (i *EphemeralDatabaseInteractor) SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *query.QueryDSL) (<-chan schema.Document, <-chan error, error) {
	c, err := i.getCollection(sc.Name)
	if err != nil {
		return nil, nil, err
	}

	docCh := make(chan schema.Document)
	errCh := make(chan error, 1)

	go func() {
		defer close(docCh)
		defer close(errCh)

		stream := c.data.Stream(100) // Using a buffered stream
		defer stream.Close()

		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
				docResult, err := stream.Next()
				if err != nil {
					if err == store.ErrStreamClosed {
						return
					}
					errCh <- err
					return
				}

				doc := schema.Document(docResult.Data)
				docCh <- doc
			}
		}
	}()

	return docCh, errCh, nil
}

// UpdateDocuments updates documents in the in-memory store.
func (i *EphemeralDatabaseInteractor) UpdateDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	c, err := i.getCollection(schemaDef.Name)
	if err != nil {
		return 0, err
	}

	queryHelper, err := query.NewQueryHelper(&query.QueryDSL{Filters: filters}, nil, nil, nil)
 	if err != nil {
		return 0, err
	}

	var updatedCount int64
	stream := c.data.Stream(0)
	defer stream.Close()

	for {
		docResult, err := stream.Next()
		if err != nil {
			if err == store.ErrStreamClosed {
				break
			}
			return 0, err
		}

		doc := schema.Document(docResult.Data)
		matches, err := queryHelper.Match(doc)
		if err != nil {
			return 0, err
		}

		if matches {
			updatedDoc := make(map[string]any)
			maps.Copy(updatedDoc, docResult.Data)
			maps.Copy(updatedDoc, updates)

			if err := c.data.Update(docResult.ID, updatedDoc); err != nil {
				return 0, err
			}
			updatedCount++
		}
	}

	return updatedCount, nil
}

// InsertDocuments inserts documents into the in-memory store.
func (i *EphemeralDatabaseInteractor) InsertDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, records []map[string]any) ([]schema.Document, error) {
	c, err := i.getCollection(schemaDef.Name)
	if err != nil {
		return nil, err
	}

	var insertedDocs []schema.Document
	for _, doc := range records {
		id, err := c.data.Insert(doc)
		if err != nil {
			return nil, err
		}

		retrieved, err := c.data.Get(id)
		if err != nil {
			return nil, err
		}

		insertedDoc := schema.Document(retrieved.Data)
		insertedDocs = append(insertedDocs, insertedDoc)
	}

	return insertedDocs, nil
}

// DeleteDocuments deletes documents from the in-memory store.
func (i *EphemeralDatabaseInteractor) DeleteDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	c, err := i.getCollection(schemaDef.Name)
	if err != nil {
		return 0, err
	}

	queryHelper, err := query.NewQueryHelper(&query.QueryDSL{Filters: filters}, nil, nil, nil)
	if err != nil {
		return 0, err
	}

	var deletedCount int64
	stream := c.data.Stream(0)
	defer stream.Close()

	for {
		docResult, err := stream.Next()
		if err != nil {
			if err == store.ErrStreamClosed {
				break
			}
			return 0, err
		}

		doc := schema.Document(docResult.Data)
		matches, err := queryHelper.Match(doc)
		if err != nil {
			return 0, err
		}

		if matches {
			if err := c.data.Delete(docResult.ID); err != nil {
				return 0, err
			}
			deletedCount++
		}
	}

	return deletedCount, nil
}

// CreateCollection creates a new collection in the in-memory store.
func (i *EphemeralDatabaseInteractor) CreateCollection(schemaDef schema.SchemaDefinition) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if _, exists := i.collections[schemaDef.Name]; exists {
		return fmt.Errorf("collection '%s' already exists", schemaDef.Name)
	}

	newStore := store.NewStore()
	newCollection := &collection{
		Name:   schemaDef.Name,
		schema: &schemaDef,
		data:   newStore,
	}

	i.collections[schemaDef.Name] = newCollection
	return nil
}

// GetColumnType returns a generic column type for the in-memory store.
func (i *EphemeralDatabaseInteractor) GetColumnType(fieldType schema.FieldType, field *schema.FieldDefinition) string {
	return "any"
}

// CreateIndex creates an index in the in-memory store.
func (i *EphemeralDatabaseInteractor) CreateIndex(name string, index schema.IndexDefinition) error {
	c, err := i.getCollection(name)
	if err != nil {
		return err
	}

	return c.data.CreateIndex(index.Name, index.Fields)
}

// DropCollection removes a collection from the in-memory store.
func (i *EphemeralDatabaseInteractor) DropCollection(name string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	c, ok := i.collections[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrCollectionNotFound, name)
	}

	c.data.Close()
	delete(i.collections, name)
	return nil
}

// CollectionExists checks if a collection exists in the in-memory store.
func (i *EphemeralDatabaseInteractor) CollectionExists(name string) (bool, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	_, exists := i.collections[name]
	return exists, nil
}

// StartTransaction begins a new in-memory transaction.
// go-store does not support transactions, so this is a no-op.
func (i *EphemeralDatabaseInteractor) StartTransaction(ctx context.Context) (persistence.DatabaseInteractor, error) {
	return i, nil
}

// Commit commits the in-memory transaction.
// go-store does not support transactions, so this is a no-op.
func (i *EphemeralDatabaseInteractor) Commit(ctx context.Context) error {
	return nil
}

// Rollback rolls back the in-memory transaction.
// go-store does not support transactions, so this is a no-op.
func (i *EphemeralDatabaseInteractor) Rollback(ctx context.Context) error {
	return nil
}
