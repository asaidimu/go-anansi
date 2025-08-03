package ephemeral

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	store "github.com/asaidimu/go-store/v3"
)

var (
	ErrCollectionNotFound = errors.New("collection not found")
	ErrNotTransaction     = errors.New("not a transaction")
)

// EphemeralDatabaseInteractor provides an in-memory implementation of the
// persistence.DatabaseInteractor interface. This is useful for testing or for
// applications that do not require persistent storage.
type EphemeralDatabaseInteractor struct {
	store  *ephemeralStore
	parent *EphemeralDatabaseInteractor // if non-nil, this is a transaction
}

var _ query.DatabaseInteractor = (*EphemeralDatabaseInteractor)(nil)

// SelectDocuments retrieves documents from the in-memory store.
func (i *EphemeralDatabaseInteractor) SelectDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, dsl *query.Query) ([]common.Document, error) {
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

	var allDocs []common.Document
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
		record := common.Document(docResult.Data)
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
			rightCollection, err := i.store.getCollection(join.Target)
			if err != nil {
				return nil, err
			}

			var rightDocs []common.Document
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
				rightDocs = append(rightDocs, common.Document(docResult.Data))
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
		return []common.Document{aggregationResults}, nil
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
func (i *EphemeralDatabaseInteractor) SelectStream(ctx context.Context, sc *schema.SchemaDefinition, dsl *query.Query) (<-chan common.Document, <-chan error, error) {
	c, err := i.store.getCollection(sc.Name)
	if err != nil {
		return nil, nil, err
	}

	docCh := make(chan common.Document)
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

				doc := common.Document(docResult.Data)
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

		doc := common.Document(docResult.Data)
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
func (i *EphemeralDatabaseInteractor) InsertDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, records []common.Document) ([]common.Document, error) {
	c, err := i.store.getCollection(schemaDef.Name)
	if err != nil {
		return nil, err
	}

	var insertedDocs []common.Document
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

		insertedDoc := common.Document(retrieved.Data)
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

		doc := common.Document(docResult.Data)
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

// StartTransaction begins a new in-memory transaction.
func (i *EphemeralDatabaseInteractor) StartTransaction(ctx context.Context) (query.TransactionalDatabaseInteractor, error) {
	i.store.mu.Lock()
	defer i.store.mu.Unlock()

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
