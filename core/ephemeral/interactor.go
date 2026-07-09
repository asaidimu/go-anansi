package ephemeral

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/utils"
	store "github.com/asaidimu/go-store/v3"
	"github.com/google/uuid"
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
func (i *EphemeralDatabaseInteractor) SelectDocuments(ctx context.Context, schemaDef *definition.Schema, dsl *query.Query) ([]map[string]any, int64, error) {
	if dsl.Target == nil {
		dsl.Target = &query.QueryTarget{
			Name: schemaDef.Name,
		}
	}

	c, err := i.store.getCollection(schemaDef.Name)
	if err != nil {
		return nil, 0, err
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
		return nil, 0, err
	}

	var allDocs []map[string]any
	stream := c.data.Stream(0)
	defer stream.Close()

	for {
		docResult, err := stream.Next()
		if err != nil {
			if err == store.ErrStreamClosed {
				break
			}
			return nil, 0, err
		}
		record := map[string]any(docResult.Data)
		allDocs = append(allDocs, record)
	}

	// Filter documents
	filteredDocs, err := queryHelper.Filter(allDocs)
	if err != nil {
		return nil, 0, err
	}

	// Handle Joins
	if len(dsl.Joins) > 0 {
		currentDocs := filteredDocs
		for _, join := range dsl.Joins {
			rightCollection, err := i.store.getCollection(join.Target.Name)
			if err != nil {
				return nil, 0, err
			}

			var rightDocs []map[string]any
			rightStream := rightCollection.data.Stream(0)
			for {
				docResult, err := rightStream.Next()
				if err != nil {
					if err == store.ErrStreamClosed {
						break
					}
					rightStream.Close()
					return nil, 0, err
				}
				rightDocs = append(rightDocs, map[string]any(docResult.Data))
			}

			rightStream.Close()

			joinedDocs, err := queryHelper.Join(currentDocs, rightDocs, &join)
			if err != nil {
				return nil, 0, err
			}

			currentDocs = joinedDocs
		}
		filteredDocs = currentDocs
	}

	// Handle Aggregations
	if len(dsl.Aggregations) > 0 {
		aggregationResults, err := queryHelper.ApplyAggregations(filteredDocs)
		if err != nil {
			return nil, 0, err
		}
		return []map[string]any{aggregationResults}, 0, nil
	}

	// Apply Sorting
	sortedDocs, err := queryHelper.Sort(filteredDocs)
	if err != nil {
		return nil, 0, err
	}

	// Apply Pagination
	paginatedDocs, paginationResult, err := queryHelper.Paginate(sortedDocs)
	if err != nil {
		return nil, 0, err
	}

	totalCount := int64(len(sortedDocs))
	if paginationResult != nil && paginationResult.Total != nil {
		totalCount = int64(*paginationResult.Total)
	}

	// Apply Projection
	projectedDocs, err := queryHelper.Project(paginatedDocs)
	if err != nil {
		return nil, 0, err
	}

	return projectedDocs, totalCount, nil
}

// SelectStream streams documents from the in-memory store.
func (i *EphemeralDatabaseInteractor) SelectStream(ctx context.Context, sc *definition.Schema, dsl *query.Query) (<-chan map[string]any, <-chan error, error) {
	c, err := i.store.getCollection(sc.Name)
	if err != nil {
		return nil, nil, err
	}

	docCh := make(chan map[string]any)
	errCh := make(chan error, 1)

	go func() {
		defer close(docCh)
		defer close(errCh)

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

				doc := map[string]any(docResult.Data)
				docCh <- doc
			}
		}
	}()

	return docCh, errCh, nil
}

// UpdateDocuments updates documents in the in-memory store.
func (i *EphemeralDatabaseInteractor) UpdateDocuments(ctx context.Context, schemaDef *definition.Schema, updates map[string]any, computedUpdates map[string]query.Query, filters *query.QueryFilter, returnDocs bool) ([]map[string]any, int64, error) {
	c, err := i.store.getCollection(schemaDef.Name)
	if err != nil {
		return nil, 0, err
	}

	queryHelper, err := query.NewQueryHelper(&query.Query{
		Target: &query.QueryTarget{
			Name: schemaDef.Name,
		},
		Filters: filters,
	}, nil, nil, nil)
	if err != nil {
		return nil, 0, err
	}

	var updatedCount int64
	var updatedDocuments []map[string]any
	var idsToUpdate []string

	stream := c.data.Stream(0)
	for {
		docResult, err := stream.Next()
		if err != nil {
			if err == store.ErrStreamClosed {
				break
			}
			stream.Close()
			return nil, 0, err
		}

		doc := map[string]any(docResult.Data)
		matches, err := queryHelper.Match(doc)
		if err != nil {
			stream.Close()
			return nil, 0, err
		}

		if matches {
			idsToUpdate = append(idsToUpdate, docResult.ID)
		}
	}
	stream.Close()

	for _, id := range idsToUpdate {
		doc, err := c.data.Get(id)
		if err != nil {
			return nil, 0, err
		}
		updatedDocData := make(map[string]any)
		maps.Copy(updatedDocData, doc.Data)
		maps.Copy(updatedDocData, updates)

		// TODO: Apply computedUpdates logic here

		if err := c.data.Update(id, updatedDocData); err != nil {
			return nil, 0, err
		}
		updatedCount++

		if returnDocs {
			retrievedDoc, err := c.data.Get(id)
			if err != nil {
				return nil, updatedCount, err
			}
			updatedDocuments = append(updatedDocuments, map[string]any(retrievedDoc.Data))
		}
	}

	return updatedDocuments, updatedCount, nil
}

// InsertDocuments inserts documents into the in-memory store.
func (i *EphemeralDatabaseInteractor) InsertDocuments(ctx context.Context, schemaDef *definition.Schema, records []map[string]any) ([]map[string]any, error) {
	c, err := i.store.getCollection(schemaDef.Name)
	if err != nil {
		return nil, err
	}

	var insertedDocs []map[string]any
	for _, doc := range records {
		utils.ConvertMaps(doc)

		// Manually enforce unique constraints based on schema definition
		for _, field := range schemaDef.Fields {
			if field.Unique {
				fieldName := string(field.Name)
				if val, ok := doc[fieldName]; ok {
					stream := c.data.Stream(0)
					for {
						existingDocResult, err := stream.Next()
						if err != nil {
							if err == store.ErrStreamClosed {
								break
							}
							stream.Close()
							return nil, common.SystemErrorFrom(ErrUniqueCheckFailed).WithOperation("ephemeral.InsertDocuments").WithCause(err)
						}
						if existingDocResult.Data[fieldName] == val {
							stream.Close()
							return nil, common.SystemErrorFrom(ErrUniqueConstraintViolation).WithOperation("ephemeral.InsertDocuments").WithPath(fieldName).WithMessage(fmt.Sprintf("field '%s' with value '%v' already exists", fieldName, val)).WithCause(base.ErrUniqueConstraintViolation)
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

		insertedDoc := map[string]any(retrieved.Data)
		insertedDocs = append(insertedDocs, insertedDoc)
	}

	return insertedDocs, nil
}

// DeleteDocuments deletes documents from the in-memory store.
func (i *EphemeralDatabaseInteractor) DeleteDocuments(ctx context.Context, schemaDef *definition.Schema, filters *query.QueryFilter, unsafeDelete bool) (int64, error) {
	c, err := i.store.getCollection(schemaDef.Name)
	if err != nil {
		return 0, err
	}

	queryHelper, err := query.NewQueryHelper(&query.Query{
		Filters: filters,
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

		doc := map[string]any(docResult.Data)
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

	for _, id := range idsToDelete {
		if err := c.data.Delete(id); err != nil {
			return 0, err
		}
		deletedCount++
	}

	return deletedCount, nil
}

func (i *EphemeralDatabaseInteractor) Query(ctx context.Context, rawQuery *query.Query) (*query.RawQueryResult, error) {
	return &query.RawQueryResult{
		Success: false,
		Message: "Raw queries are not fully supported in the ephemeral database interactor.",
	}, ErrRawQueriesNotSupported
}

func (i *EphemeralDatabaseInteractor) HasTransaction(ctx context.Context) bool {
	return i.parent != nil
}

func (i *EphemeralDatabaseInteractor) StartTransaction(ctx context.Context) (query.DatabaseInteractor, error) {
	i.store.mu.Lock()
	defer i.store.mu.Unlock()

	i.txMu.Lock()

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

func (i *EphemeralDatabaseInteractor) Commit(ctx context.Context) error {
	if i.parent == nil {
		return ErrNotTransaction
	}

	i.parent.store.mu.Lock()
	defer i.parent.store.mu.Unlock()
	i.store.mu.RLock()
	defer i.store.mu.RUnlock()

	i.parent.store.collections = i.store.collections
	i.parent.txMu.Unlock()
	return nil
}

func (i *EphemeralDatabaseInteractor) Rollback(ctx context.Context) error {
	if i.parent == nil {
		return ErrNotTransaction
	}
	i.parent.txMu.Unlock()
	return nil
}

func (i *EphemeralDatabaseInteractor) SchemaManager() query.SchemaManager {
	return i
}

func (i *EphemeralDatabaseInteractor) Capabilities() query.Capabilities {
	return query.Capabilities{
		RequiresTransactionSerialization: true,
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
		SchemaEvolution: query.SchemaEvolution{
			AddColumn:        true,
			DropColumn:       true,
			RenameColumn:     true,
			AlterColumnType:  true,
			AddConstraint:    true,
			DropConstraint:   true,
		},
		SupportsGroupBy:      true,
		SupportsDistinct:     true,
		SupportsNestedFields: true,
		MaxWhereConditions:   0,
		MaxJoinClauses:       0,
	}
}

func (m *EphemeralDatabaseInteractor) CreateCollection(ctx context.Context, schemaDef definition.Schema) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	if _, exists := m.store.collections[schemaDef.Name]; exists {
		return common.SystemErrorFrom(ErrCollectionAlreadyExists).WithOperation("ephemeral.CreateCollection").WithPath(schemaDef.Name).WithCause(base.ErrCollectionAlreadyExists)
	}

	newStore := store.NewStore()

	for _, field := range schemaDef.Fields {
		if field.Unique {
			fieldName := string(field.Name)
			if err := newStore.CreateIndex(fieldName, []string{fieldName}); err != nil {
				return common.SystemErrorFrom(ErrCreateIndexFailed).WithOperation("ephemeral.CreateCollection").WithPath(fieldName).WithCause(err)
			}
		}
	}
	for _, index := range schemaDef.Indexes {
		fields := make([]string, len(index.Fields))
		for j, f := range index.Fields {
			fields[j] = string(f)
		}
		if err := newStore.CreateIndex(index.Name, fields); err != nil {
			return common.SystemErrorFrom(ErrCreateIndexFailed).WithOperation("ephemeral.CreateCollection").WithPath(index.Name).WithCause(err)
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

func (m *EphemeralDatabaseInteractor) CreateIndex(ctx context.Context, collection string, index definition.Index) error {
	c, err := m.store.getCollection(collection)
	if err != nil {
		return err
	}

	fields := make([]string, len(index.Fields))
	for j, f := range index.Fields {
		fields[j] = string(f)
	}
	err = c.data.CreateIndex(index.Name, fields)
	if err != nil {
		return common.SystemErrorFrom(err).WithOperation("ephemeral.CreateIndex").WithCause(err)
	}
	return nil
}

func (m *EphemeralDatabaseInteractor) DropIndex(ctx context.Context, collection string, index definition.Index) error {
	c, err := m.store.getCollection(collection)
	if err != nil {
		return err
	}

	err = c.data.DropIndex(index.Name)
	if err != nil {
		return common.SystemErrorFrom(err).WithOperation("ephemeral.DropIndex").WithCause(err)
	}
	return nil
}

func (m *EphemeralDatabaseInteractor) DropCollection(ctx context.Context, name string) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	c, ok := m.store.collections[name]
	if !ok {
		return common.SystemErrorFrom(ErrCollectionNotFound).WithOperation("ephemeral.DropCollection").WithPath(name).WithCause(base.ErrCollectionNotFound)
	}

	c.data.Close()
	delete(m.store.collections, name)
	return nil
}

func (m *EphemeralDatabaseInteractor) CollectionExists(ctx context.Context, name string) (bool, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	_, exists := m.store.collections[name]
	return exists, nil
}

func (m *EphemeralDatabaseInteractor) AddColumn(ctx context.Context, collection string, field definition.Field) error {
	c, err := m.store.getCollection(collection)
	if err != nil {
		return err
	}
	if c.schema.Fields == nil {
		c.schema.Fields = make(map[definition.FieldId]definition.Field)
	}
	newID := definition.FieldId(uuid.Must(uuid.NewV7()).String())
	c.schema.Fields[newID] = field
	return nil
}

func (m *EphemeralDatabaseInteractor) DropColumn(ctx context.Context, collection string, fieldName string) error {
	c, err := m.store.getCollection(collection)
	if err != nil {
		return err
	}
	for id, f := range c.schema.Fields {
		if string(f.Name) == fieldName {
			delete(c.schema.Fields, id)
			break
		}
	}
	return nil
}

func (m *EphemeralDatabaseInteractor) RenameColumn(ctx context.Context, collection string, oldName, newName string) error {
	c, err := m.store.getCollection(collection)
	if err != nil {
		return err
	}
	for id, f := range c.schema.Fields {
		if string(f.Name) == oldName {
			f.Name = definition.FieldName(newName)
			c.schema.Fields[id] = f
			break
		}
	}
	return nil
}
