package collection

import (
	"maps"
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

	for _, doc := range docs {
		validationResult, err := c.Validate(ctx, doc, false)

		if err != nil {
			return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_VALIDATION_SYSTEM_ERROR")
		}

		if !validationResult.Valid {
			results = append(results, base.CreateResult{Status: base.StatusFailedValidation, Data: doc, Issues: validationResult.Issues})
		} else {
			results = append(results, base.CreateResult{Status: base.StatusCreated, Data: doc})
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

	results, err := c.wrapped.CreateMany(ctx, docs)
	if err != nil {
		sanitizedErr := c.sanitizeError(ctx, err, nil)
		for i := range results {
			if results[i].Error != nil {
				results[i].Error = c.sanitizeError(ctx, results[i].Error, nil).(*common.SystemError)
			}
		}
		return results, sanitizedErr
	}
	return results, nil
}

// Read fetches documents and enriches them with the metadata block for transport.
func (c *managedCollection) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	var fq *query.Query = q
	var allTranslations map[string]string

	if fq.Raw != nil && c.rawQueryProcessor != nil {
		rawQuery := fq.Raw
		resolvedTemplate, err := c.rawQueryProcessor.ProcessRawQueryTemplate(ctx, rawQuery.Template, rawQuery.Collections)
		if err != nil {
			return nil, err
		}

		fq.Raw = &query.RawQuery{
			Template:    resolvedTemplate,
			Options:     rawQuery.Options,
			Collections: rawQuery.Collections,
			Parameters:  rawQuery.Parameters,
		}
	} else {
		// Clone the query first to avoid mutating the original
		cloned, err := q.Clone()
		if err != nil {
			return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_CLONE_QUERY_FAILED")
		}

		// Prepare the query (resolve all join targets and subqueries)
		prepared, translations, err := c.prepareQuery(ctx, cloned)
		if err != nil {
			return nil, err
		}
		fq = prepared
		allTranslations = translations
	}

	// Set the main target (the collection itself)
	fq.Target = &query.QueryTarget{
		Name:   c.physicalName,
		Alias:  &c.logicalName,
		Schema: c.schema,
	}

	// Add main collection translation
	if allTranslations == nil {
		allTranslations = make(map[string]string)
	}
	allTranslations[c.physicalName] = c.logicalName

	fq = ensureMetadataProjection(fq)

	result, err := c.wrapped.Read(ctx, fq)

	if err != nil || result.Count == 0 {
		return result, c.sanitizeError(ctx, err, allTranslations)
	}

	return result, nil
}

// prepareQuery recursively processes a query to resolve all physical names,
// including those in subqueries and joins at any depth.
func (c *managedCollection) prepareQuery(ctx context.Context, q *query.Query) (*query.Query, map[string]string, error) {
	if q == nil {
		return nil, nil, nil
	}

	// Note: Query is already cloned by Read method
	translations := make(map[string]string)

	// Resolve joins recursively
	for i := range q.Joins {
		// CRITICAL: Store the original logical name BEFORE resolution
		originalLogicalName := q.Joins[i].Target.Name

		physicalName, joinSchema, trans, err := c.resolveTargetWithTranslations(ctx, &q.Joins[i].Target)
		if err != nil {
			return nil, nil, err
		}

		q.Joins[i].Target.Name = physicalName
		q.Joins[i].Target.Schema = joinSchema

		// Ensure alias is set to the original logical name if not provided
		// This is critical: field references like "profiles.user" must work
		if q.Joins[i].Target.Alias == nil {
			q.Joins[i].Target.Alias = &originalLogicalName
		}

		maps.Copy(translations, trans)

		// Recursively process subqueries in join conditions
		if q.Joins[i].On != nil {
			joinFilter, trans, err := c.prepareFilter(ctx, q.Joins[i].On)
			if err != nil {
				return nil, nil, err
			}
			q.Joins[i].On = joinFilter
			maps.Copy(translations, trans)
		}
	}

	// Recursively process subqueries in filters
	if q.Filters != nil {
		filter, trans, err := c.prepareFilter(ctx, q.Filters)
		if err != nil {
			return nil, nil, err
		}
		q.Filters = filter
		maps.Copy(translations, trans)
	}

	// Recursively process subqueries in aggregation filters
	for i := range q.Aggregations {
		if q.Aggregations[i].Filter != nil {
			aggFilter, trans, err := c.prepareFilter(ctx, q.Aggregations[i].Filter)
			if err != nil {
				return nil, nil, err
			}
			q.Aggregations[i].Filter = aggFilter
			maps.Copy(translations, trans)
		}
	}

	// Recursively process union queries
	if q.Union != nil {
		for i := range q.Union.Queries {
			unionQuery, trans, err := c.prepareQuery(ctx, &q.Union.Queries[i])
			if err != nil {
				return nil, nil, err
			}
			q.Union.Queries[i] = *unionQuery
			maps.Copy(translations, trans)
		}
	}

	return q, translations, nil
}

// prepareFilter recursively processes filters to resolve physical names in subqueries.
func (c *managedCollection) prepareFilter(ctx context.Context, filter *query.QueryFilter) (*query.QueryFilter, map[string]string, error) {
	if filter == nil {
		return nil, nil, nil
	}

	translations := make(map[string]string)
	prepared := &query.QueryFilter{}

	if filter.Condition != nil {
		prepared.Condition = &query.FilterCondition{
			Field:    filter.Condition.Field,
			Operator: filter.Condition.Operator,
		}

		value, trans, err := c.prepareFilterValue(ctx, &filter.Condition.Value)
		if err != nil {
			return nil, nil, err
		}
		prepared.Condition.Value = *value
		maps.Copy(translations, trans)
	}

	if filter.Group != nil {
		prepared.Group = &query.FilterGroup{
			Operator:   filter.Group.Operator,
			Conditions: make([]query.QueryFilter, 0, len(filter.Group.Conditions)),
		}

		for _, subFilter := range filter.Group.Conditions {
			preparedSubFilter, trans, err := c.prepareFilter(ctx, &subFilter)
			if err != nil {
				return nil, nil, err
			}
			prepared.Group.Conditions = append(prepared.Group.Conditions, *preparedSubFilter)
			maps.Copy(translations, trans)
		}
	}

	if filter.TextSearchQuery != nil {
		prepared.TextSearchQuery = filter.TextSearchQuery
	}

	return prepared, translations, nil
}

// prepareFilterValue recursively processes filter values to resolve subqueries.
func (c *managedCollection) prepareFilterValue(ctx context.Context, value *query.FilterValue) (*query.FilterValue, map[string]string, error) {
	if value == nil {
		return nil, nil, nil
	}

	translations := make(map[string]string)
	prepared := &query.FilterValue{
		StringVal: value.StringVal,
		NumberVal: value.NumberVal,
		BoolVal:   value.BoolVal,
		ObjectVal: value.ObjectVal,
	}

	if value.ArrayVal != nil {
		prepared.ArrayVal = make([]query.FilterValue, 0, len(value.ArrayVal))
		for _, item := range value.ArrayVal {
			preparedItem, trans, err := c.prepareFilterValue(ctx, &item)
			if err != nil {
				return nil, nil, err
			}
			prepared.ArrayVal = append(prepared.ArrayVal, *preparedItem)
			maps.Copy(translations, trans)
		}
	}

	if value.FieldRefVal != nil {
		prepared.FieldRefVal = value.FieldRefVal
	}

	if value.FunctionCallVal != nil {
		prepared.FunctionCallVal = &query.FunctionCall{
			Function:  value.FunctionCallVal.Function,
			Arguments: make([]query.FilterValue, 0, len(value.FunctionCallVal.Arguments)),
		}

		for _, arg := range value.FunctionCallVal.Arguments {
			preparedArg, trans, err := c.prepareFilterValue(ctx, &arg)
			if err != nil {
				return nil, nil, err
			}
			prepared.FunctionCallVal.Arguments = append(prepared.FunctionCallVal.Arguments, *preparedArg)
			maps.Copy(translations, trans)
		}
	}

	if value.SubqueryVal != nil {
		preparedSubquery, trans, err := c.prepareQuery(ctx, &value.SubqueryVal.Query)
		if err != nil {
			return nil, nil, err
		}

		prepared.SubqueryVal = &query.SubqueryValue{
			Type:  value.SubqueryVal.Type,
			Query: *preparedSubquery,
		}

		maps.Copy(translations, trans)
	}

	return prepared, translations, nil
}

// resolveTargetWithTranslations resolves a query target and returns translation mappings.
func (c *managedCollection) resolveTargetWithTranslations(ctx context.Context, target *query.QueryTarget) (string, *schema.SchemaDefinition, map[string]string, error) {
	if c.resolveSchema == nil {
		return "", nil, nil, schema.ErrPhysicalNameResolverNotSet
	}

	logicalName := target.Name
	physicalName, targetSchema, err := c.resolveSchema(ctx, logicalName)
	if err != nil {
		return "", nil, nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_RESOLVE_PHYSICAL_NAME_FAILED",
			fmt.Sprintf("failed to resolve physical name for target '%s'", logicalName))
	}

	translations := map[string]string{
		physicalName: logicalName,
	}

	// If an alias is provided, use that for translations
	if target.Alias != nil {
		translations[physicalName] = *target.Alias
	}

	return physicalName, targetSchema, translations, nil
}

// Update verifies the integrity of the metadata block, performs an optimistic lock check,
// and updates the document and its metadata.
func (c *managedCollection) Update(ctx context.Context, params *base.CollectionUpdate) (*base.ReadResult, error) {
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

	if err := validate(); err != nil {
		return nil, err
	}

	// Prepare compute queries to resolve subqueries
	var allTranslations map[string]string
	if params.Compute != nil {
		allTranslations = make(map[string]string)
		preparedCompute := make(map[string]query.Query)

		for fieldPath, computeQuery := range params.Compute {
			// Clone and prepare the compute query to resolve subqueries
			cloned, err := computeQuery.Clone()
			if err != nil {
				return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_CLONE_COMPUTE_QUERY_FAILED")
			}

			prepared, trans, err := c.prepareQuery(ctx, cloned)
			if err != nil {
				return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_PREPARE_COMPUTE_QUERY_FAILED")
			}

			preparedCompute[fieldPath] = *prepared
			maps.Copy(allTranslations, trans)
		}

		params.Compute = preparedCompute
	}

	if params.Compute == nil {
		params.Compute = map[string]query.Query{}
	}

	versionField := data.MetadataFieldPath(data.MetadataVersion)
	params.Compute[versionField] = query.NewQueryBuilder().
		Select().
		AddComputed(versionField, "ADD", &query.FieldReference{Field: versionField}, 1).
		End().
		Build()

	// Prepare the filter to resolve subqueries
	if params.Filter != nil {
		preparedFilter, trans, err := c.prepareFilter(ctx, params.Filter)
		if err != nil {
			return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_PREPARE_UPDATE_FILTER_FAILED")
		}
		params.Filter = preparedFilter

		if allTranslations == nil {
			allTranslations = make(map[string]string)
		}
		maps.Copy(allTranslations, trans)
	}

	if params.Version != nil {
		version := float64(*params.Version)

		if params.Filter == nil {
			return nil, base.ErrDangerousUpdate
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

	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	updatedField := data.MetadataFieldPath(data.MetadataUpdated)
	params.Set.Set(updatedField, now)

	count, err := c.wrapped.Update(ctx, params)
	if err != nil {
		return count, c.sanitizeError(ctx, err, allTranslations)
	}
	return count, nil
}

// --- Passthrough Methods ---
func (c *managedCollection) Delete(ctx context.Context, q *query.QueryFilter, unsafe bool) (int, error) {
	if q == nil && !unsafe {
		return 0, base.ErrDangerousDelete
	}

	var preparedFilter *query.QueryFilter
	var allTranslations map[string]string
	var err error

	// Prepare the filter to resolve subqueries
	if q != nil {
		preparedFilter, allTranslations, err = c.prepareFilter(ctx, q)
		if err != nil {
			return 0, common.SystemErrorFrom(err, "ERR_PERSISTENCE_PREPARE_DELETE_FILTER_FAILED")
		}
	}

	count, err := c.wrapped.Delete(ctx, preparedFilter, unsafe)
	if err != nil {
		return count, c.sanitizeError(ctx, err, allTranslations)
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

	if q.Projection.HasField(data.MetadataField) {
		panic(base.ErrExplicitMetadataProjectionForbidden.Error())
	}

	if q.Projection.IsExcluded(data.MetadataField) {
		q.Projection.RemoveExcludedField(data.MetadataField)
	}

	q.Projection.IncludeField(data.MetadataField, nil, nil)

	return q
}

// sanitizeError applies translations from prepareQuery
func (c *managedCollection) sanitizeError(_ context.Context, err error, translations map[string]string) error {
	if err == nil {
		return nil
	}

	if translations == nil {
		translations = make(map[string]string)
	}
	translations[c.physicalName] = c.logicalName

	tf := func(input string) string {
		if input == "" {
			return ""
		}
		output := input
		for phys, log := range translations {
			output = strings.ReplaceAll(output, phys, log)
		}
		return output
	}

	return common.SystemErrorFrom(err).Sanitize(tf)
}
