package ephemeral

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	store "github.com/asaidimu/go-store/v3"
)

var (
	ErrCollectionNotFound = errors.New("collection not found")
	ErrNotTransaction     = errors.New("not a transaction")
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
	parent      *EphemeralDatabaseInteractor // if non-nil, this is a transaction
}

var _ query.DatabaseInteractor = (*EphemeralDatabaseInteractor)(nil)

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
func (i *EphemeralDatabaseInteractor) SelectDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, dsl *query.Query) ([]schema.Document, error) {
	c, err := i.getCollection(schemaDef.Name)
	if err != nil {
		return nil, err
	}

	// Register aggregate functions with the query helper
	aggregateFuncs := &query.AggregationFunctionsMap{
		query.AggregationTypeCount: query.AggregateFunction(countAggregate),
		query.AggregationTypeSum:   query.AggregateFunction(sumAggregate),
		query.AggregationTypeAvg:   query.AggregateFunction(avgAggregate),
		query.AggregationTypeMin:   query.AggregateFunction(minAggregate),
		query.AggregationTypeMax:   query.AggregateFunction(maxAggregate),
	}

	queryHelper, err := query.NewQueryHelper(dsl, nil, aggregateFuncs, nil)
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
		allDocs = append(allDocs, record)
	}

	// Filter documents
	filteredDocs, err := queryHelper.Filter(allDocs)
	if err != nil {
		return nil, err
	}

	// Handle Joins
	if len(dsl.Joins) > 0 {
		currentDocs := filteredDocs
		for _, join := range dsl.Joins {
			rightCollection, err := i.getCollection(join.Target)
			if err != nil {
				return nil, err
			}

			var rightDocs []schema.Document
			rightStream := rightCollection.data.Stream(0)
			for {
				docResult, err := rightStream.Next()
				if err != nil {
					if err == store.ErrStreamClosed {
						break
					}
					rightStream.Close()
					return nil, err
				}
				rightDocs = append(rightDocs, schema.Document(docResult.Data))
			}
			rightStream.Close()

			joinedDocs, err := queryHelper.Join(schemaDef.Name, currentDocs, rightDocs, &join)
			if err != nil {
				return nil, err
			}
			currentDocs = joinedDocs
		}
		filteredDocs = currentDocs
	}

	// Handle Aggregations
	if len(dsl.Aggregations) > 0 {
		aggregationResults, err := queryHelper.ApplyAggregations(filteredDocs)
		if err != nil {
			return nil, err
		}
		return []schema.Document{aggregationResults}, nil
	}

	// Apply projection, sorting, and pagination
	projectedDocs, err := queryHelper.Project(filteredDocs)
	if err != nil {
		return nil, err
	}

	sortedDocs, err := queryHelper.Sort(projectedDocs)
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
func (i *EphemeralDatabaseInteractor) SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *query.Query) (<-chan schema.Document, <-chan error, error) {
	c, err := i.getCollection(sc.Name)
	if err != nil {
		return nil, nil, err
	}

	docCh := make(chan schema.Document)
	errCh := make(chan error, 1)

	go func() {
		defer close(docCh)
		defer close(errCh)

		// This is not a truly streaming implementation of all query features.
		// For a simple stream of all documents, this works.
		// For a fully featured stream, we would need to implement filtering, sorting, etc. in a streaming manner.
		stream := c.data.Stream(100)
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

	queryHelper, err := query.NewQueryHelper(&query.Query{Filters: filters}, nil, nil, nil)
	if err != nil {
		return 0, err
	}

	var updatedCount int64
	var idsToUpdate []string
	stream := c.data.Stream(0)
	for {
		docResult, err := stream.Next()
		if err != nil {
			if err == store.ErrStreamClosed {
				break
			}
			stream.Close()
			return 0, err
		}

		doc := schema.Document(docResult.Data)
		matches, err := queryHelper.Match(doc)
		if err != nil {
			stream.Close()
			return 0, err
		}

		if matches {
			idsToUpdate = append(idsToUpdate, docResult.ID)
		}
	}
	stream.Close()

	for _, id := range idsToUpdate {
		doc, err := c.data.Get(id)
		if err != nil {
			return 0, err
		}
		updatedDoc := make(map[string]any)
		maps.Copy(updatedDoc, doc.Data)
		maps.Copy(updatedDoc, updates)

		if err := c.data.Update(id, updatedDoc); err != nil {
			return 0, err
		}
		updatedCount++
	}

	return updatedCount, nil
}

// InsertDocuments inserts documents into the in-memory store.
func (i *EphemeralDatabaseInteractor) InsertDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, records []schema.Document) ([]schema.Document, error) {
	c, err := i.getCollection(schemaDef.Name)
	if err != nil {
		return nil, err
	}

	var insertedDocs []schema.Document
	for _, doc := range records {
		// Ensure nested maps are also of type map[string]any
		utils.ConvertMaps(doc)

		// Manually enforce unique constraints based on schema definition
		for _, field := range schemaDef.Fields {
			if field.Unique != nil && *field.Unique {
				if val, ok := doc[field.Name]; ok {
					// Iterate through existing documents to check for uniqueness
					stream := c.data.Stream(0)
					for {
						existingDocResult, err := stream.Next()
						if err != nil {
							if err == store.ErrStreamClosed {
								break
							}
							stream.Close()
							return nil, fmt.Errorf("failed to read existing documents for unique check: %w", err)
						}
						if existingDocResult.Data[field.Name] == val {
							stream.Close()
							return nil, fmt.Errorf("unique constraint violation: field '%s' with value '%v' already exists", field.Name, val)
						}
					}
					stream.Close()
				}
			}
		}

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

	queryHelper, err := query.NewQueryHelper(&query.Query{Filters: filters}, nil, nil, nil)
	if err != nil {
		return 0, err
	}

	var deletedCount int64
	var idsToDelete []string
	stream := c.data.Stream(0)
	for {
		docResult, err := stream.Next()
		if err != nil {
			if err == store.ErrStreamClosed {
				break
			}
			stream.Close()
			return 0, err
		}

		doc := schema.Document(docResult.Data)
		matches, err := queryHelper.Match(doc)
		if err != nil {
			stream.Close()
			return 0, err
		}

		if matches {
			idsToDelete = append(idsToDelete, docResult.ID)
		}
	}
	stream.Close()

	for _, id := range idsToDelete { // this should be handled in a transaction.
		if err := c.data.Delete(id); err != nil {
			return 0, err
		}
		deletedCount++
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

	// Create indexes based on schema definition
	for _, field := range schemaDef.Fields {
		if field.Unique != nil && *field.Unique {
			if err := newStore.CreateIndex(field.Name, []string{field.Name}); err != nil {
				return fmt.Errorf("failed to create unique index for field %s: %w", field.Name, err)
			}
		}
	}
	for _, index := range schemaDef.Indexes {
		if err := newStore.CreateIndex(index.Name, index.Fields); err != nil {
			return fmt.Errorf("failed to create index %s: %w", index.Name, err)
		}
	}

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
func (i *EphemeralDatabaseInteractor) StartTransaction(ctx context.Context) (query.DatabaseInteractor, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	/* // Create a deep copy of the collections for the transaction
	txCollections := make(map[string]*collection)
	for name, c := range i.collections {
		txStore := store.NewStore()
		stream := c.data.Stream(0)
		for {
			item, err := stream.Next()
			if err != nil {
				if err == store.ErrStreamClosed {
					break
				}
				stream.Close()
				return nil, err
			}
			// We need to copy the data to avoid modifying the original
			dataCopy := make(map[string]any)
			maps.Copy(dataCopy, item.Data)
			if _, err := txStore.InsertWithID(item.ID, dataCopy); err != nil {
				stream.Close()
				return nil, err
			}
		}
		stream.Close()

		// Copy indexes
		for _, indexName := range c.data.ListIndexes() {
			fields, _ := c.data.GetIndex(indexName)
			if err := txStore.CreateIndex(indexName, fields); err != nil {
				return nil, err
			}
		}

		txCollections[name] = &collection{
			Name:   c.Name,
			schema: c.schema,
			data:   txStore,
		}
	}

	txInteractor := &EphemeralDatabaseInteractor{
		collections: txCollections,
		parent:      i,
	} */

	return nil, nil
}

// Commit commits the in-memory transaction.
func (i *EphemeralDatabaseInteractor) Commit(ctx context.Context) error {
	if i.parent == nil {
		return ErrNotTransaction
	}

	i.parent.mu.Lock()
	defer i.parent.mu.Unlock()
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Replace parent's collections with the transactional ones
	i.parent.collections = i.collections

	return nil
}

// Rollback rolls back the in-memory transaction.
func (i *EphemeralDatabaseInteractor) Rollback(ctx context.Context) error {
	if i.parent == nil {
		return ErrNotTransaction
	}
	// Just discard the transactional interactor, no changes are applied to the parent.
	return nil
}

// Capabilities returns the capabilities of the ephemeral database interactor.
func (i *EphemeralDatabaseInteractor) Capabilities() query.Capabilities {
	return query.Capabilities{
		SupportedLogicalOperators: map[query.LogicalOperator]struct{}{
			query.LogicalOperatorAnd: {},
			query.LogicalOperatorOr:  {},
			query.LogicalOperatorNot: {},
			query.LogicalOperatorNor: {},
			query.LogicalOperatorXor: {},
		},
		SupportedComparisonOperators: map[query.ComparisonOperator]struct{}{
			query.ComparisonOperatorEq:        {},
			query.ComparisonOperatorNeq:       {},
			query.ComparisonOperatorLt:        {},
			query.ComparisonOperatorLte:       {},
			query.ComparisonOperatorGt:        {},
			query.ComparisonOperatorGte:       {},
			query.ComparisonOperatorIn:        {},
			query.ComparisonOperatorNin:       {},
			query.ComparisonOperatorContains:  {},
			query.ComparisonOperatorNotContains: {},
			query.ComparisonOperatorExists:    {},
			query.ComparisonOperatorNotExists: {},
		},
		SupportedAggregationFunctions: map[query.AggregationType]struct{}{
			query.AggregationTypeCount: {},
			query.AggregationTypeSum:   {},
			query.AggregationTypeAvg:   {},
			query.AggregationTypeMin:   {},
			query.AggregationTypeMax:   {},
		},
		SupportedJoinTypes: map[query.JoinType]struct{}{
			query.JoinTypeInner: {},
			query.JoinTypeLeft:  {},
			query.JoinTypeRight: {},
			query.JoinTypeFull:  {},
		},
		SupportedPaginationTypes: map[query.PaginationType]struct{}{
			query.PaginationTypeOffset: {},
		},
		SupportsGroupBy:      true,
		SupportsDistinct:     true,
		SupportsNestedFields: true,
		// MaxWhereConditions: 0, // 0 means no limit
		// MaxJoinClauses:     0, // 0 means no limit
	}
}
