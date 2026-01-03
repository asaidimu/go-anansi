package collection

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// managedCollection is a decorator that wraps a base.PersistenceCollectionInterface to provide
// transparent metadata management, versioning, and optimistic locking.
type managedCollection struct {
	physicalName      string
	logicalName       string
	wrapped           base.Collection
	schema            *schema.SchemaDefinition
	rawQueryProcessor base.RawQueryProcessor
	resolveSchema     func(ctx context.Context, name string) (string, *schema.SchemaDefinition, error)
}

// newManagedCollection creates a new ManagedCollection decorator.
func newManagedCollection(
	schema *schema.SchemaDefinition,
	logicalName string,
	physicalName string,
	wrapped base.Collection,
	resolveSchema func(ctx context.Context, name string) (string, *schema.SchemaDefinition, error),
	processor base.RawQueryProcessor,
) (*managedCollection, error) {

	if wrapped == nil {
		return nil, ErrCollectionInitializationFailed
	}

	return &managedCollection{
		schema:            schema,
		physicalName:      physicalName,
		logicalName:       logicalName,
		wrapped:           wrapped,
		resolveSchema:     resolveSchema,
		rawQueryProcessor: processor,
	}, nil
}

// --- Core Method Overrides ---

// CreateOne handles the creation of a single document.
func (c *managedCollection) CreateOne(ctx context.Context, doc *data.Document) (base.CreateResult, error) {
	results, err := c.CreateMany(ctx, []*data.Document{doc})
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
func (c *managedCollection) CreateMany(ctx context.Context, docs []*data.Document) ([]base.CreateResult, error) {
	results := make([]base.CreateResult, 0)
	validCount := 0

	for _, d := range docs {
		doc := data.MustNewDocument(d)
		validationResult, err := c.Validate(ctx, doc, false)

		if err != nil {
			// If Validate itself returns an error, it's a system error, not a validation failure.
			// We should return this error immediately.
			return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_VALIDATION_SYSTEM_ERROR")
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
		var allIssues []common.Issue
		for _, res := range results {
			if res.Status == base.StatusFailedValidation && len(res.Issues) > 0 {
				allIssues = append(allIssues, res.Issues...)
			}
		}

		return results, base.ErrValidationFailed.WithIssues(allIssues).WithMessage(fmt.Sprintf("validation failed for %d documents", len(docs)-validCount))
	}

	// All documents are valid, proceed with actual creation
	results, err := c.wrapped.CreateMany(ctx, docs)
	if err != nil {
		sanitizedErr := c.sanitize(ctx, err, nil)
		// We must also sanitize errors inside the individual result objects
		for i := range results {
			if results[i].Error != nil {
				results[i].Error = c.sanitize(ctx, results[i].Error, nil).(*common.SystemError)
			}
		}
		return results, sanitizedErr
	}
	return results, nil
}

// Read fetches documents and enriches them with the metadata block for transport.
func (c *managedCollection) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	var fq *query.Query = q

	if fq.Raw != nil && c.rawQueryProcessor != nil {
		rawQuery := fq.Raw
		// Resolve collection placeholders in the template
		resolvedTemplate, err := c.rawQueryProcessor.ProcessRawQueryTemplate(ctx, rawQuery.Template, rawQuery.Collections)
		if err != nil {
			return nil, err
		}

		// Create a new RawQuery with the resolved template
		fq.Raw = &query.RawQuery{
			Template:    resolvedTemplate,
			Options:     rawQuery.Options,
			Collections: rawQuery.Collections,
			Parameters:  rawQuery.Parameters,
		}
	}

	if q.Joins != nil {
		modifiedQuery, err := q.Clone()
		if err != nil {
			return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_CLONE_QUERY_FAILED")
		}
		// Translate logical join targets to physical names
		for i, join := range modifiedQuery.Joins {
			name := join.Target
			if c.resolveSchema == nil {
				return nil, schema.ErrPhysicalNameResolverNotSet
			}
			physicalName, schema, err := c.resolveSchema(ctx, name.Name)

			if err != nil {
				return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_RESOLVE_PHYSICAL_NAME_FAILED", fmt.Sprintf("%s for join target '%s'", base.ErrFailedToResolvePhysicalName.Error(), join.Target.Name))
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

	if err != nil || result.Count == 0 {
		return result, c.sanitize(ctx, err, fq)
	}

	docs := result.Data

	// Ensure all docs have metadata and are re-hashed
	for i, doc := range docs {
		if _, ok := doc.Metadata(); ok {
			if err := docs[i].Hash(); err != nil {
				return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_HASH_DOCUMENT_FAILED")
			}
		} else {
			docs[i] = data.MustNewDocument(doc)
		}
	}

	result.Data = docs

	return result, nil
}

// Update verifies the integrity of the metadata block, performs an optimistic lock check,
// and updates the document and its metadata.
func (c *managedCollection) Update(ctx context.Context, params *base.CollectionUpdate) (int, error) {
	// --- helper for validation & wrapping errors ---
	validate := func() error {
		result, err := c.Validate(ctx, params.Set, true)
		if err != nil {
			return common.SystemErrorFrom(err, "ERR_PERSISTENCE_VALIDATION_SYSTEM_ERROR")
		}
		if !result.Valid {
			return base.ErrValidationFailed.WithIssues(result.Issues)
		}
		return nil
	}

	// --- perform validation once ---
	if err := validate(); err != nil {
		return 0, err
	}

	// --- setup version computation ---
	if params.Compute == nil {
		params.Compute = map[string]query.Query{}
	}

	versionField := data.MetadataFieldPath(data.MetadataVersion)
	params.Compute[versionField] = query.NewQueryBuilder().
		Select().
		AddComputed(versionField, "ADD", &query.FieldReference{Field: versionField}, 1).
		End().
		Build()

	// --- optimistic lock: if Version provided, modify filter ---
	if params.Version != nil {
		version := float64(*params.Version)

		if params.Filter == nil {
			return 0, base.ErrDangerousUpdate
		}

		versionFilter := query.QueryFilter{
			Condition: &query.FilterCondition{
				Field:    versionField,
				Operator: query.ComparisonOperatorEq,
				Value: query.FilterValue{
					NumberVal: &version,
				},
			},
		}

		qb := query.NewQueryBuilder().
			AndFilter(*params.Filter).
			AndFilter(versionFilter)

		params.Filter = qb.Build().Filters
	}

	// update the updated timestamp
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	updatedField := data.MetadataFieldPath(data.MetadataUpdated)

	params.Set.Set(updatedField, now)
	// --- delegate actual update ---
	count, err := c.wrapped.Update(ctx, params)
	if err != nil {
		// We use nil for query here unless you want to pass joined params
		return count, c.sanitize(ctx, err, nil)
	}
	return count, nil
}

// --- Passthrough Methods ---
func (c *managedCollection) Delete(ctx context.Context, q *query.QueryFilter, unsafe bool) (int, error) {
	if q == nil && !unsafe {
		return 0, base.ErrDangerousDelete
	}

	count, err := c.wrapped.Delete(ctx, q, unsafe)
	if err != nil {
		return count, c.sanitize(ctx, err, nil)
	}

	return count, nil
}

func (c *managedCollection) Validate(ctx context.Context, data *data.Document, loose bool) (*schema.ValidationResult, error) {
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
	if q.Projection.HasField(data.MetadataField) {
		// defensive: prevent overriding system metadata
		panic(base.ErrExplicitMetadataProjectionForbidden.Error())
	}

	// Always remove metadata from exclusions
	if q.Projection.IsExcluded(data.MetadataField) {
		q.Projection.RemoveExcludedField(data.MetadataField)
	}

	// Ensure metadata is added to Include
	q.Projection.IncludeField(data.MetadataField, nil, nil)

	return q
}
func (c *managedCollection) sanitize(_ context.Context, err error, q *query.Query) error {
	if err == nil {
		return nil
	}

	// 1. Build the translation registry
	// Start with the primary collection
	translations := map[string]string{
		c.physicalName: c.logicalName,
	}

	// Add any joins involved in the current query
	if q != nil && q.Joins != nil {
		for _, join := range q.Joins {
			if join.Target.Alias != nil {
				translations[join.Target.Name] = *join.Target.Alias
			}
		}
	}

	// 2. Define the transformation logic
	tf := func(input string) string {
		if input == "" {
			return ""
		}
		output := input
		for phys, log := range translations {
			output = fmt.Sprintf("%s", output)
			output = (func(s string) string {
				return (func(str string) string {
					return strings.ReplaceAll(str, phys, log)
				})(output)
			})(output)
		}
		return output
	}

	// 3. Perform Deep Sanitization
	// We use SystemErrorFrom to ensure we have a SystemError, then call the
	// recursive Sanitize method we discussed earlier.
	return common.SystemErrorFrom(err).Sanitize(tf)
}
