package collection

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
)

// ============================================================================
// Predefined Errors
// ============================================================================

var (
	// ErrRecordNotFound indicates the requested record was not found
	ErrRecordNotFound = common.NewSystemError("ERR_COLLECTION_RECORD_NOT_FOUND").
				WithMessage("record not found")

	// ErrNoRecordsAffected indicates an update/delete operation affected no records
	ErrNoRecordsAffected = common.NewSystemError("ERR_COLLECTION_NO_RECORDS_AFFECTED").
				WithMessage("no records were affected by the operation")
)

// ============================================================================
// Model Collection Implementation
// ============================================================================

// modelCollection provides type-safe operations on a raw collection.
// It automatically handles struct ↔ document conversions.
type modelCollection[T any] struct {
	raw            base.Collection
	collectionName string
}

// NewModelCollection creates a type-safe wrapper around a raw collection.
func NewModelCollection[T any](raw base.Collection) base.ModelCollection[T] {
	metadata, _ := raw.Metadata(context.Background(), nil, false)
	return &modelCollection[T]{raw: raw, collectionName: metadata.Name}
}

// ============================================================================
// Create Operations
// ============================================================================

// New creates a new model instance with auto-generated ID and metadata.
// This is useful when you want to generate an ID before actually persisting.
func (mc *modelCollection[T]) New(doc T, ctx ...context.Context) (T, error) {
	var zero T

	var ictx context.Context = context.Background()
	if len(ctx) > 0 && ctx[0] != nil {
		ictx = ctx[0]
	}
	ictx = context.WithValue(ictx, common.CollectionNameContextKey, mc.collectionName)
	// Convert struct → document with full metadata
	d, err := data.NewDocumentFromStruct(doc, ictx)
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.New").
			WithMessage("failed to create document from struct")
	}

	// Bind back to model to get ID and metadata
	err = d.BindToWithContext(ictx, &zero)
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.New").
			WithMessage("failed to bind document back to model")
	}

	return zero, nil
}

// Create inserts a single model into the collection.
func (mc *modelCollection[T]) Create(ctx context.Context, doc T) (T, error) {
	var zero T

	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	d, err := data.NewDocumentFromStruct(doc, ctx)
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Create").
			WithMessage("failed to convert model to document")
	}

	res, err := mc.raw.CreateOne(ctx, d)
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Create")
	}

	err = res.Data.BindToWithContext(ctx, &zero)
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Create").
			WithMessage("failed to bind result document to model")
	}

	return zero, nil
}

// CreateMany inserts multiple models into the collection.
func (mc *modelCollection[T]) CreateMany(ctx context.Context, docs []T) ([]T, error) {
	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	if len(docs) == 0 {
		return []T{}, nil
	}

	// Convert all structs to documents
	input := make([]*data.Document, len(docs))
	for i, doc := range docs {
		d, err := data.NewDocumentFromStruct(doc, ctx)
		if err != nil {
			return nil, common.SystemErrorFrom(err).
				WithOperation("ModelCollection.CreateMany").
				WithPath(fmt.Sprintf("docs[%d]", i)).
				WithMessagef("failed to convert model at index %d to document", i)
		}
		input[i] = d
	}

	// Create all documents
	results, err := mc.raw.CreateMany(ctx, input)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.CreateMany")
	}

	// Bind all results back to models
	output := make([]T, len(results))
	for i, res := range results {
		var model T
		err := res.Data.BindToWithContext(ctx, &model)
		if err != nil {
			return nil, common.SystemErrorFrom(err).
				WithOperation("ModelCollection.CreateMany").
				WithPath(fmt.Sprintf("results[%d]", i)).
				WithMessagef("failed to bind result at index %d to model", i)
		}
		output[i] = model
	}

	return output, nil
}

// ============================================================================
// Read Operations
// ============================================================================

// FindByID retrieves a single model by its ID.
func (mc *modelCollection[T]) FindByID(ctx context.Context, id string) (T, error) {
	var zero T

	q := query.NewQueryBuilder().
		Where("id").Eq(id).
		Limit(1).
		Build()

	results, err := mc.Read(ctx, &q)
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.FindByID").
			WithPath(id)
	}

	if len(results) == 0 {
		return zero, ErrRecordNotFound.
			WithOperation("ModelCollection.FindByID").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	return results[0], nil
}

// Read retrieves multiple models matching the query.
func (mc *modelCollection[T]) Read(ctx context.Context, q *query.Query) ([]T, error) {
	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	res, err := mc.raw.Read(ctx, q)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Read")
	}

	if len(res.Data) == 0 {
		return []T{}, nil
	}

	// Bind all documents to models
	output := make([]T, len(res.Data))
	for i, doc := range res.Data {
		var model T
		err := doc.BindToWithContext(ctx, &model)
		if err != nil {
			return nil, common.SystemErrorFrom(err).
				WithOperation("ModelCollection.Read").
				WithPath(fmt.Sprintf("results[%d]", i)).
				WithMessagef("failed to bind document at index %d to model", i)
		}
		output[i] = model
	}

	return output, nil
}

// ============================================================================
// Update Operations
// ============================================================================

// Update updates a single model by ID and returns the updated model.
// Only non-zero fields in the update model are applied (partial update).
func (mc *modelCollection[T]) Update(ctx context.Context, id string, update T) (T, error) {
	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	var zero T

	// Create partial document (only non-zero fields)
	d, err := data.NewPartialDocumentFromStruct(update, ctx)
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Update").
			WithPath(id).
			WithMessage("failed to convert update model to partial document")
	}

	filter := query.NewQueryBuilder().
		Where("id").Eq(id).
		Build().Filters

	result, err := mc.raw.Update(ctx, &base.CollectionUpdate{
		Filter: filter,
		Set:    d,
	})
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Update").
			WithPath(id)
	}

	if result.Count == 0 {
		return zero, ErrRecordNotFound.
			WithOperation("ModelCollection.Update").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	// Fetch and return the updated model
	return mc.FindByID(ctx, id)
}

// UpdateMany updates multiple models matching the filter.
// Returns the count of updated records.
func (mc *modelCollection[T]) UpdateMany(ctx context.Context, f *query.QueryFilter, update T) (int, error) {
	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	// Create partial document (only non-zero fields)
	d, err := data.NewPartialDocumentFromStruct(update, ctx)
	if err != nil {
		return 0, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.UpdateMany").
			WithMessage("failed to convert update model to partial document")
	}

	result, err := mc.raw.Update(ctx, &base.CollectionUpdate{
		Filter: f,
		Set:    d,
	})
	if err != nil {
		return 0, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.UpdateMany")
	}

	return result.Count, nil
}

// Replace replaces an entire model by ID (all fields, not partial).
// Use Update for partial updates.
func (mc *modelCollection[T]) Replace(ctx context.Context, id string, replacement T) (T, error) {
	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	var zero T

	// Create full document
	d, err := data.NewDocumentFromStruct(replacement, ctx)
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Replace").
			WithPath(id).
			WithMessage("failed to convert replacement model to document")
	}

	filter := query.NewQueryBuilder().
		Where("id").Eq(id).
		Build().Filters

	result, err := mc.raw.Update(ctx, &base.CollectionUpdate{
		Filter: filter,
		Set:    d,
	})
	if err != nil {
		return zero, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Replace").
			WithPath(id)
	}

	if result.Count == 0 {
		return zero, ErrRecordNotFound.
			WithOperation("ModelCollection.Replace").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	// Fetch and return the replaced model
	return mc.FindByID(ctx, id)
}

// ============================================================================
// Delete Operations
// ============================================================================

// DeleteByID deletes a single model by ID.
func (mc *modelCollection[T]) DeleteByID(ctx context.Context, id string) error {
	filter := query.NewQueryBuilder().
		Where("id").Eq(id).
		Build().Filters

	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	count, err := mc.raw.Delete(ctx, filter, false)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.DeleteByID").
			WithPath(id)
	}

	if count == 0 {
		return ErrRecordNotFound.
			WithOperation("ModelCollection.DeleteByID").
			WithPath(id).
			WithMessagef("record with id '%s' not found", id)
	}

	return nil
}

// DeleteMany deletes multiple models matching the filter.
// Returns the count of deleted records.
// Set unsafe=true to allow deleting without filters (deletes all).
func (mc *modelCollection[T]) DeleteMany(ctx context.Context, f *query.QueryFilter, unsafe bool) (int, error) {
	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	count, err := mc.raw.Delete(ctx, f, unsafe)
	if err != nil {
		return 0, common.SystemErrorFrom(err).
			WithOperation("ModelCollection.DeleteMany")
	}
	return count, nil
}

// ============================================================================
// Validation Operations
// ============================================================================

// Validate validates a model against the collection's schema.
// Set loose=true for partial validation (allows missing optional fields).
func (mc *modelCollection[T]) Validate(ctx context.Context, doc T, loose bool) error {
	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	d, err := data.NewDocumentFromStruct(doc, ctx)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Validate").
			WithMessage("failed to convert model to document for validation")
	}

	_, err = mc.raw.Validate(ctx, d, loose)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.Validate")
	}

	return nil
}

// ValidatePartial validates a partial model (only non-zero fields).
// Useful for validating updates before applying them.
func (mc *modelCollection[T]) ValidatePartial(ctx context.Context, doc T) error {
	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	d, err := data.NewPartialDocumentFromStruct(doc, ctx)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.ValidatePartial").
			WithMessage("failed to convert partial model to document for validation")
	}

	_, err = mc.raw.Validate(ctx, d, true) // Always use loose=true for partials
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("ModelCollection.ValidatePartial")
	}

	return nil
}

// ============================================================================
// Subscription Operations
// ============================================================================

// Subscribe creates a subscription for real-time updates.
// Returns a subscription ID that can be used to unsubscribe.
func (mc *modelCollection[T]) Subscribe(ctx context.Context, opt base.SubscriptionOptions) string {
	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	return mc.raw.Subscribe(ctx, opt)
}

// Unsubscribe removes a subscription by ID.
func (mc *modelCollection[T]) Unsubscribe(ctx context.Context, id string) {
	ctx = context.WithValue(ctx, common.CollectionNameContextKey, mc.collectionName)
	mc.raw.Unsubscribe(ctx, id)
}

// ============================================================================
// Raw Access
// ============================================================================

// Raw returns the underlying raw collection for advanced operations.
// Use this when you need direct access to document-level operations.
func (mc *modelCollection[T]) Raw() base.Collection {
	return mc.raw
}
