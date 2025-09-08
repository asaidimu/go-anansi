package collection

import (
	"context"
	"errors"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// managedCollection is a decorator that wraps a base.PersistenceCollectionInterface to provide
// transparent metadata management, versioning, and optimistic locking.
type managedCollection struct {
	physicalName  string
	logicalName   string
	wrapped       base.Collection
	schema        *schema.SchemaDefinition
	resolveSchema func(ctx context.Context, name string) (string, *schema.SchemaDefinition, error)
}

// newManagedCollection creates a new ManagedCollection decorator.
func newManagedCollection(
	schema *schema.SchemaDefinition,
	logicalName string,
	physicalName string,
	wrapped base.Collection,
	resolveSchema func(ctx context.Context, name string) (string, *schema.SchemaDefinition, error),
) (*managedCollection, error) {

	return &managedCollection{
		schema:        schema,
		physicalName:  physicalName,
		logicalName:   logicalName,
		wrapped:       wrapped,
		resolveSchema: resolveSchema,
	}, nil
}

// --- Core Method Overrides ---

// CreateOne handles the creation of a single document.
func (c *managedCollection) CreateOne(ctx context.Context, doc data.Document) (base.CreateResult, error) {
	results, err := c.CreateMany(ctx, []data.Document{doc})
	result := base.CreateResult{}

	if len(results) > 0 {
		result = results[0]
	}

	if err != nil {
		return result, err
	}

	return result, nil
}

// CreateMany handles the creation of multiple documents, providing a rich result for each.
func (c *managedCollection) CreateMany(ctx context.Context, docs []data.Document) ([]base.CreateResult, error) {
	results := make([]base.CreateResult, 0)
	validCount := 0

	for _, d := range docs {
		doc := data.MustNewDocument(d)
		doc.MustVerifyHash()
		validationResult, err := c.Validate(ctx, doc, false)

		if err != nil {
			// If Validate itself returns an error, it's a system error, not a validation failure.
			// We should return this error immediately.
			return nil, &CollectionError{
				Operation: "CreateMany",
				Message:   "system error during validation",
				Cause:     errors.Join(data.ErrSystemErrorDuringValidation, err),
			}
		}

		if !validationResult.Valid {
			// Document is invalid, append the result with issues
			results = append(results, base.CreateResult{Status: base.StatusFailedValidation, Data: doc, Issues: validationResult.Issues})
		} else {
			// Document is valid
			results = append(results, base.CreateResult{Status: base.StatusCreated, Data: doc}) // Assuming it will be created
			validCount++
		}
	}

	if validCount != len(docs) {
		// Some documents failed validation, return the results with details
		return results, &CollectionError{
			Operation: "CreateMany",
			Message:   fmt.Sprintf("for %d documents", len(docs)-validCount),
			Cause:     base.ErrValidationFailed,
		}
	}

	// All documents are valid, proceed with actual creation
	return c.wrapped.CreateMany(ctx, docs)
}

// Read fetches documents and enriches them with the metadata block for transport.
func (c *managedCollection) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	var fq *query.Query = q

	if q.Joins != nil {
		modifiedQuery, err := q.Clone()
		if err != nil {
			return nil, base.NewPersistenceError("failed to clone query for join resolution", err)
		}
		// Translate logical join targets to physical names
		for i, join := range modifiedQuery.Joins {
			name := join.Target
			if c.resolveSchema == nil {
				return nil, base.NewPersistenceError(data.ErrPhysicalNameResolverNotSet.Error(), nil)
			}
			physicalName, schema, err := c.resolveSchema(ctx, name.Name)

			if err != nil {
				return nil, base.NewPersistenceError(fmt.Sprintf("%s for join target '%s': %v", data.ErrFailedToResolvePhysicalName.Error(), join.Target.Name, err), err)
			}

			modifiedQuery.Joins[i].Target.Name = physicalName
			modifiedQuery.Joins[i].Target.Schema = schema

			if join.Target.Alias == nil {
				modifiedQuery.Joins[i].Target.Alias = &name.Name
			}
		}

		fq = modifiedQuery
	}

	fq.Target = &query.QueryTarget{
		Name:   c.physicalName,
		Alias:  &c.logicalName,
		Schema: c.schema,
	}

	fq = ensureMetadataProjection(fq)

	result, err := c.wrapped.Read(ctx, fq)

	if err != nil {
		return nil, err
	}

	return result, nil
}

// Update verifies the integrity of the metadata block, performs an optimistic lock check,
// and updates the document and its metadata.
func (c *managedCollection) Update(ctx context.Context, params *base.CollectionUpdate) (int, error) {
	meta, ok := params.Data.Metadata()
	if !ok {
		return 0, &CollectionError{
			Operation: "Update",
			Message:   "update operation requires a valid metadata block",
			Cause:     data.ErrNoMetadata,
		}
	}

	var err error
	if params.Recover {
		params.Data.HashMetadata()
	}

	if !params.Data.VerifyHash() {
		return 0, &CollectionError{
			Operation: "Update",
			Message:   "data may be tampered",
			Cause:     data.ErrHashMismatch,
		}
	}

	d := params.Data.StripMetadata()
	result, err := c.Validate(ctx, d, true)

	if err != nil {
		return 0, &CollectionError{
			Operation: "Update",
			Message:   base.ErrUpdateDocuments.Error(),
			Cause:     errors.Join(base.ErrUpdateDocuments, err),
		}
	}

	if !result.Valid {
		return 0, &CollectionError{
			Operation: "Update",
			Message:   fmt.Sprintf("%v", result.Issues),
			Cause:     base.ErrValidationFailed,
		}
	}

	// 2. Prepare for optimistic locking
	version, ok := utils.CoerceToPrimitiveValue[float64](meta["version"])

	if !ok {
		return 0, &CollectionError{
			Operation: "Update",
			Message:   fmt.Sprintf("%v", meta),
			Cause:     data.ErrInvalidOrMissingMetadataVersion,
		}
	}

	// Modify the filter to include the version check
	if params.Filter == nil {
		params.Filter = &query.QueryFilter{}
	}

	versionFilter := query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    fmt.Sprintf("%s.version", data.MetadataFieldName),
			Operator: query.ComparisonOperatorEq,
			Value: query.FilterValue{
				NumberVal: &version,
			},
		},
	}

	qb := query.NewQueryBuilder()
	qb = qb.AndFilter(*params.Filter).AndFilter(versionFilter)
	combined := qb.Build().Filters
	params.Filter = combined

	err = params.Data.TouchMetadata()
	if err != nil {
		return 0, err
	}

	return c.wrapped.Update(ctx, params)
}

// --- Passthrough Methods ---
func (c *managedCollection) Delete(ctx context.Context, q *query.QueryFilter, unsafe bool) (int, error) {
	return c.wrapped.Delete(ctx, q, unsafe)
}

func (c *managedCollection) Validate(ctx context.Context, data data.Document, loose bool) (*schema.ValidationResult, error) {
	return c.wrapped.Validate(ctx, data, loose)
}

func (c *managedCollection) Metadata(ctx context.Context, filter *base.MetadataFilter, forceRefresh bool) (*base.CollectionMetadata, error) {
	return c.wrapped.Metadata(ctx, filter, forceRefresh)
}

func (c *managedCollection) Subscribe(ctx context.Context, options base.SubscriptionOptions) string {
	return c.wrapped.Subscribe(ctx, options)
}

func (c *managedCollection) Unsubscribe(ctx context.Context, id string) {
	c.wrapped.Unsubscribe(ctx, id)
}

func (c *managedCollection) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) {
	return c.wrapped.Subscriptions(ctx)
}

func (c *managedCollection) Capabilities(ctx context.Context) *query.Capabilities {
	return c.wrapped.Capabilities(ctx)
}

func ensureMetadataProjection(q *query.Query) *query.Query {
	if q.Projection == nil {
		return q
	}

	// Users must not manually include metadata
	if q.Projection.HasField(data.MetadataFieldName) {
		// defensive: prevent overriding system metadata
		panic(data.ErrExplicitMetadataProjectionForbidden.Error())
	}

	// Always remove metadata from exclusions
	if q.Projection.IsExcluded(data.MetadataFieldName) {
		q.Projection.RemoveExcludedField(data.MetadataFieldName)
	}

	// Ensure metadata is added to Include
	q.Projection.IncludeField(data.MetadataFieldName, nil, nil)

	return q
}
