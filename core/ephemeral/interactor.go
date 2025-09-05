package ephemeral

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/registry"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	store "github.com/asaidimu/go-store/v3"
)

var (
	ErrNotTransaction     = errors.New("not a transaction")
)

// EphemeralDatabaseInteractor provides an in-memory implementation of the
// persistence.DatabaseInteractor interface. This is useful for testing or for
// applications that do not require persistent storage.
type EphemeralDatabaseInteractor struct {
	store  *ephemeralStore
	parent *EphemeralDatabaseInteractor // if non-nil, this is a transaction
	txMu   sync.Mutex
}

var _ query.DatabaseInteractor = (*EphemeralDatabaseInteractor)(nil)
var _ query.SchemaManager = (*EphemeralDatabaseInteractor)(nil)

// SelectDocuments retrieves documents from the in-memory store.
func (i *EphemeralDatabaseInteractor) SelectDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, dsl *query.Query) ([]data.Document, error) {

	if dsl.Target == nil {
		dsl.Target = &query.QueryTarget{
			Name: schemaDef.Name,
		}
	}

	c, err := i.store.getCollection(schemaDef.Name)
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

	var allDocs []data.Document
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
		record := data.Document(docResult.Data)
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
			rightCollection, err := i.store.getCollection(join.Target.Name)
			if err != nil {
				return nil, err
			}

			var rightDocs []data.Document
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
				rightDocs = append(rightDocs, data.Document(docResult.Data))
			}

			rightStream.Close()

			joinedDocs, err := queryHelper.Join(currentDocs, rightDocs, &join)
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
		return []data.Document{aggregationResults}, nil
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
func (i *EphemeralDatabaseInteractor) SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *query.Query) (<-chan data.Document, <-chan error, error) {
	c, err := i.store.getCollection(sc.Name)
	if err != nil {
		return nil, nil, err
	}

	docCh := make(chan data.Document)
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

				doc := data.Document(docResult.Data)
				docCh <- doc
			}
		}
	}()

	return docCh, errCh, nil
}

// UpdateDocuments updates documents in the in-memory store.
func (i *EphemeralDatabaseInteractor) UpdateDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, updates map[string]any, filters *query.QueryFilter) (int64, error) {
	c, err := i.store.getCollection(schemaDef.Name)
	if err != nil {
		return 0, err
	}
	queryHelper, err := query.NewQueryHelper(&query.Query{
		Target: &query.QueryTarget{
			Name: schemaDef.Name,
		},
		Filters: filters,
	}, nil, nil, nil)
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

		doc := data.Document(docResult.Data)
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
func (i *EphemeralDatabaseInteractor) InsertDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, records []data.Document) ([]data.Document, error) {
	c, err := i.store.getCollection(schemaDef.Name)
	if err != nil {
		return nil, err
	}

	var insertedDocs []data.Document
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
							return nil, &EphemeralError{
								Operation: "InsertDocuments",
								Message:   fmt.Sprintf("for unique check: %v", registry.ErrFailedToReadDocuments),
								Cause:     errors.Join(registry.ErrFailedToReadDocuments, err),
							}
						}
						if existingDocResult.Data[field.Name] == val {
							stream.Close()
															return nil, &EphemeralError{
									Operation: "InsertDocuments",
									Message:   fmt.Sprintf("field '%s' with value '%v' already exists", field.Name, val),
									Cause:     registry.ErrUniqueConstraintViolation,
								}
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

		insertedDoc := data.Document(retrieved.Data)
		insertedDocs = append(insertedDocs, insertedDoc)
	}

	return insertedDocs, nil
}

// DeleteDocuments deletes documents from the in-memory store.
func (i *EphemeralDatabaseInteractor) DeleteDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	c, err := i.store.getCollection(schemaDef.Name)
	if err != nil {
		return 0, err
	}

	queryHelper, err := query.NewQueryHelper(&query.Query{Filters: filters,
		Target: &query.QueryTarget{
			Name: schemaDef.Name,
		},
	}, nil, nil, nil)
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

		doc := data.Document(docResult.Data)
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

func (i *EphemeralDatabaseInteractor) HasTransaction(ctx context.Context) bool {
	return i.parent != nil
}

// StartTransaction begins a new in-memory transaction.
func (i *EphemeralDatabaseInteractor) StartTransaction(ctx context.Context) (query.DatabaseInteractor, error) {
	i.store.mu.Lock()
	defer i.store.mu.Unlock()

	i.txMu.Lock()

	// Create a deep copy of the collections for the transaction
	txCollections := make(map[string]*collection)
	for name, c := range i.store.collections {
		data, err := c.data.Clone()
		if err != nil {
			return nil, err
		}

		txCollections[name] = &collection{
			Name:   c.Name,
			schema: c.schema,
			data:   data,
		}
	}

	txInteractor := &EphemeralDatabaseInteractor{
		store: &ephemeralStore{
			collections: txCollections,
		},
		parent: i,
	}
	return txInteractor, nil
}

// Commit commits the in-memory transaction.
func (i *EphemeralDatabaseInteractor) Commit(ctx context.Context) error {
	if i.parent == nil {
		return ErrNotTransaction
	}

	i.parent.store.mu.Lock()
	defer i.parent.store.mu.Unlock()
	i.store.mu.RLock()
	defer i.store.mu.RUnlock()

	// Replace parent's collections with the transactional ones
	i.parent.store.collections = i.store.collections

	i.parent.txMu.Unlock()
	return nil
}

// Rollback rolls back the in-memory transaction.
func (i *EphemeralDatabaseInteractor) Rollback(ctx context.Context) error {
	if i.parent == nil {
		return ErrNotTransaction
	}
	i.parent.txMu.Unlock()
	// Just discard the transactional interactor, no changes are applied to the parent.
	return nil
}

// SchemaManager returns only the methods available to the schema manager
func (i *EphemeralDatabaseInteractor) SchemaManager() query.SchemaManager {
	return i
}

// Capabilities returns the capabilities of the ephemeral database interactor.
func (i *EphemeralDatabaseInteractor) Capabilities() query.Capabilities {
	return query.Capabilities{
		SupportedLogicalOperators: map[common.LogicalOperator]struct{}{
			common.LogicalAnd: {},
			common.LogicalOr:  {},
			common.LogicalNot: {},
			common.LogicalNor: {},
			common.LogicalXor: {},
		},
		SupportedComparisonOperators: map[query.ComparisonOperator]struct{}{
			query.ComparisonOperatorEq:          {},
			query.ComparisonOperatorNeq:         {},
			query.ComparisonOperatorLt:          {},
			query.ComparisonOperatorLte:         {},
			query.ComparisonOperatorGt:          {},
			query.ComparisonOperatorGte:         {},
			query.ComparisonOperatorIn:          {},
			query.ComparisonOperatorNin:         {},
			query.ComparisonOperatorContains:    {},
			query.ComparisonOperatorNotContains: {},
			query.ComparisonOperatorExists:      {},
			query.ComparisonOperatorNotExists:   {},
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
		MaxWhereConditions:   0, // 0 means no limit
		MaxJoinClauses:       0, // 0 means no limit
	}
}

// CreateCollection creates a new collection in the in-memory store.
func (m *EphemeralDatabaseInteractor) CreateCollection(ctx context.Context, schemaDef schema.SchemaDefinition) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	if _, exists := m.store.collections[schemaDef.Name]; exists {
		return &EphemeralError{
			Operation: "CreateCollection",
			Message:   fmt.Sprintf("'%s'", schemaDef.Name),
			Cause:     registry.ErrCollectionAlreadyExists,
		}
	}

	newStore := store.NewStore()

	// Create indexes based on schema definition
	for _, field := range schemaDef.Fields {
		if field.Unique != nil && *field.Unique {
			if err := newStore.CreateIndex(field.Name, []string{field.Name}); err != nil {
				return &EphemeralError{
					Operation: "CreateCollection",
					Message:   fmt.Sprintf("for field %s", field.Name),
					Cause:     errors.Join(registry.ErrFailedToCreateIndex, err),
				}
			}
		}
	}
	for _, index := range schemaDef.Indexes {
		if err := newStore.CreateIndex(index.Name, index.Fields); err != nil {
			return &EphemeralError{
				Operation: "CreateCollection",
				Message:   fmt.Sprintf("'%s'", index.Name),
				Cause:     errors.Join(registry.ErrFailedToCreateIndex, err),
			}
		}
	}

	newCollection := &collection{
		Name:   schemaDef.Name,
		schema: &schemaDef,
		data:   newStore,
	}

	m.store.collections[schemaDef.Name] = newCollection
	return nil
}

// CreateIndex creates an index in the in-memory store.
func (m *EphemeralDatabaseInteractor) CreateIndex(ctx context.Context, collection string, index schema.IndexDefinition) error {
	c, err := m.store.getCollection(collection)
	if err != nil {
		return err
	}

	return c.data.CreateIndex(index.Name, index.Fields)
}

func (m *EphemeralDatabaseInteractor) DropIndex(ctx context.Context, collection string, index schema.IndexDefinition) error {
	c, err := m.store.getCollection(collection)
	if err != nil {
		return err
	}

	return c.data.DropIndex(index.Name)
}

// DropCollection removes a collection from the in-memory store.
func (m *EphemeralDatabaseInteractor) DropCollection(ctx context.Context,name string) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	c, ok := m.store.collections[name]
	if !ok {
		return &EphemeralError{
			Operation: "DropCollection",
			Message:   fmt.Sprintf("'%s'", name),
			Cause:     ErrCollectionNotFound,
		}
	}

	c.data.Close()
	delete(m.store.collections, name)
	return nil
}

// CollectionExists checks if a collection exists in the in-memory store.
func (m *EphemeralDatabaseInteractor) CollectionExists(ctx context.Context,name string) (bool, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	_, exists := m.store.collections[name]
	return exists, nil
}
