package query

import (
	"errors"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

// QueryPartitioner is responsible for splitting a QueryDSL query into a database-specific query
// and a post-processing query based on the capabilities of the database.
type QueryPartitioner struct {
	capabilities Capabilities
}

// NewQueryPartitioner creates a new QueryPartitioner.
func NewQueryPartitioner(capabilities Capabilities) *QueryPartitioner {
	return &QueryPartitioner{capabilities: capabilities}
}

// Partition splits the given QueryDSL query into two parts: one for the database and one for post-processing.
func (p *QueryPartitioner) Partition(dsl *Query) (*Query, *Query, error) {
	if dsl.Raw != nil {
		return &Query{
			Target: dsl.Target,
			Raw:    dsl.Raw,
		}, &Query{}, nil
	}

	dbQuery := &Query{
		Target: dsl.Target,
	}

	postProcessingQuery := &Query{
		Target: dsl.Target,
	}

	// Partition filters (now handles subqueries recursively)
	dbFilters, postFilters, err := p.partitionFilters(dsl.Filters)
	if err != nil {
		return nil, nil, err
	}
	dbQuery.Filters = dbFilters
	postProcessingQuery.Filters = postFilters

	// Partition joins (now handles subqueries in join conditions)
	dbJoins, postJoins, err := p.partitionJoins(dsl.Joins)
	if err != nil {
		return nil, nil, err
	}
	dbQuery.Joins = dbJoins
	postProcessingQuery.Joins = postJoins

	// Partition aggregations (now handles subqueries in aggregation filters)
	dbAggregations, postAggregations, err := p.partitionAggregations(dsl.Aggregations)
	if err != nil {
		return nil, nil, err
	}
	dbQuery.Aggregations = dbAggregations
	postProcessingQuery.Aggregations = postAggregations

	// Partition sorting
	dbSort, postSort := p.partitionSort(dsl.Sort)
	dbQuery.Sort = dbSort
	postProcessingQuery.Sort = postSort

	// Handle pagination
	if p.supportsPagination(dsl.Pagination) {
		dbQuery.Pagination = dsl.Pagination
	} else {
		postProcessingQuery.Pagination = dsl.Pagination
	}

	// Partition unions (now handles subqueries within union queries)
	if dsl.Union != nil {
		dbUnion, postUnion, err := p.partitionUnion(dsl.Union)
		if err != nil {
			return nil, nil, err
		}
		dbQuery.Union = dbUnion
		postProcessingQuery.Union = postUnion
	}

	// Dependency analysis and projection augmentation
	if err := p.augmentProjection(dsl, dbQuery, postProcessingQuery); err != nil {
		return nil, nil, err
	}

	return dbQuery, postProcessingQuery, nil
}

func (p *QueryPartitioner) partitionFilters(filter *QueryFilter) (*QueryFilter, *QueryFilter, error) {
	if filter == nil {
		return nil, nil, nil
	}

	// Base case: we have a simple condition
	if filter.Condition != nil {
		// Check if the condition value contains a subquery
		if filter.Condition.Value.SubqueryVal != nil {
			dbSubquery, postSubquery, err := p.Partition(&filter.Condition.Value.SubqueryVal.Query)
			if err != nil {
				return nil, nil, err
			}

			// If the subquery requires post-processing, the entire condition must be post-processed
			if !postSubquery.IsEmpty() {
				return nil, filter, nil
			}

			// Otherwise, create an optimized filter with the DB-optimized subquery
			optimizedFilter := &QueryFilter{
				Condition: &FilterCondition{
					Field:    filter.Condition.Field,
					Operator: filter.Condition.Operator,
					Value:    filter.Condition.Value,
				},
			}
			optimizedFilter.Condition.Value.SubqueryVal.Query = *dbSubquery

			if p.isConditionSupported(filter.Condition) {
				return optimizedFilter, nil, nil
			} else {
				return nil, optimizedFilter, nil
			}
		}

		// No subquery, use existing logic
		if p.isConditionSupported(filter.Condition) {
			return filter, nil, nil
		} else {
			return nil, filter, nil
		}
	}

	// Recursive step: we have a group of filters
	if filter.Group != nil {
		var dbConditions, postConditions []QueryFilter

		for _, subFilter := range filter.Group.Conditions {
			dbSubFilter, postSubFilter, err := p.partitionFilters(&subFilter)
			if err != nil {
				return nil, nil, err
			}
			if dbSubFilter != nil {
				dbConditions = append(dbConditions, *dbSubFilter)
			}
			if postSubFilter != nil {
				postConditions = append(postConditions, *postSubFilter)
			}
		}

		// If the logical operator is OR and we have a mix of DB and post-processing filters,
		// the entire group must be handled by post-processing to ensure correct evaluation.
		if filter.Group.Operator == common.LogicalOr && len(dbConditions) > 0 && len(postConditions) > 0 {
			return nil, filter, nil
		}

		var dbFilter, postFilter *QueryFilter
		if len(dbConditions) > 0 {
			dbFilter = &QueryFilter{Group: &FilterGroup{Operator: filter.Group.Operator, Conditions: dbConditions}}
		}
		if len(postConditions) > 0 {
			postFilter = &QueryFilter{Group: &FilterGroup{Operator: filter.Group.Operator, Conditions: postConditions}}
		}

		return dbFilter, postFilter, nil
	}

	// Handle TextSearchQuery if necessary (assuming it's either fully supported or not)
	if filter.TextSearchQuery != nil {
		if _, supported := p.capabilities.SupportedTextSearchTypes[filter.TextSearchQuery.Type]; supported {
			return filter, nil, nil
		} else {
			return nil, filter, nil
		}
	}

	return nil, nil, common.NewSystemError("ERR_QUERY_INVALID_FILTER_STRUCTURE", "invalid filter structure").WithOperation("partitionFilters").WithCause(errors.New("invalid filter structure"))
}

func (p *QueryPartitioner) isConditionSupported(cond *FilterCondition) bool {
	if _, supported := p.capabilities.SupportedComparisonOperators[cond.Operator]; !supported {
		return false
	}

	// Check if the value contains unsupported function calls
	if cond.Value.FunctionCallVal != nil {
		// You could add more sophisticated function support checking here
		return true
	}

	return true
}

func (p *QueryPartitioner) partitionJoins(joins []JoinConfiguration) ([]JoinConfiguration, []JoinConfiguration, error) {
	var dbJoins, postJoins []JoinConfiguration

	for _, join := range joins {
		// Check if the join type is supported
		if _, supported := p.capabilities.SupportedJoinTypes[join.Type]; !supported {
			postJoins = append(postJoins, join)
			continue
		}

		// Check if the join condition contains subqueries
		if join.On != nil {
			dbFilter, postFilter, err := p.partitionFilters(join.On)
			if err != nil {
				return nil, nil, err
			}

			// If the join condition requires post-processing, the entire join must be post-processed
			if postFilter != nil {
				postJoins = append(postJoins, join)
				continue
			}

			// Otherwise, use the optimized DB filter
			optimizedJoin := join
			optimizedJoin.On = dbFilter
			dbJoins = append(dbJoins, optimizedJoin)
		} else {
			dbJoins = append(dbJoins, join)
		}
	}

	return dbJoins, postJoins, nil
}

func (p *QueryPartitioner) partitionAggregations(aggregations []AggregationConfiguration) ([]AggregationConfiguration, []AggregationConfiguration, error) {
	var dbAggregations, postAggregations []AggregationConfiguration

	for _, agg := range aggregations {
		// Check if the aggregation type is supported
		if _, supported := p.capabilities.SupportedAggregationFunctions[agg.Type]; !supported {
			postAggregations = append(postAggregations, agg)
			continue
		}

		// Check if the aggregation filter contains subqueries
		if agg.Filter != nil {
			dbFilter, postFilter, err := p.partitionFilters(agg.Filter)
			if err != nil {
				return nil, nil, err
			}

			// If the filter requires post-processing, the entire aggregation must be post-processed
			if postFilter != nil {
				postAggregations = append(postAggregations, agg)
				continue
			}

			// Otherwise, use the optimized DB filter
			optimizedAgg := agg
			optimizedAgg.Filter = dbFilter
			dbAggregations = append(dbAggregations, optimizedAgg)
		} else {
			dbAggregations = append(dbAggregations, agg)
		}
	}

	return dbAggregations, postAggregations, nil
}

func (p *QueryPartitioner) partitionSort(sorts []SortConfiguration) ([]SortConfiguration, []SortConfiguration) {
	// For simplicity, we assume if the database supports sorting, it supports all of it.
	// A more granular check would be needed for expression-based sorting.
	if p.capabilities.Sorting.SupportsExpression {
		return sorts, nil
	}
	return nil, sorts
}

func (p *QueryPartitioner) supportsPagination(pagination *PaginationOptions) bool {
	if pagination == nil || len(pagination.Type) == 0 {
		return true
	}
	_, supported := p.capabilities.SupportedPaginationTypes[PaginationType(pagination.Type)]
	return supported
}

func (p *QueryPartitioner) partitionUnion(union *QueryUnion) (*QueryUnion, *QueryUnion, error) {
	if union == nil || len(union.Queries) == 0 {
		return nil, nil, nil
	}

	var dbQueries []Query

	for _, subQuery := range union.Queries {
		dbQuery, postQuery, err := p.Partition(&subQuery)
		if err != nil {
			return nil, nil, err
		}

		// If any sub-query requires post-processing, the entire union must be post-processed
		if !postQuery.IsEmpty() {
			// Move all queries to post-processing
			return nil, union, nil
		}

		dbQueries = append(dbQueries, *dbQuery)
	}

	// All queries can be handled by the database
	return &QueryUnion{
		Queries: dbQueries,
		Type:    union.Type,
	}, nil, nil
}

func (p *QueryPartitioner) augmentProjection(originalQuery, dbQuery, postQuery *Query) error {
	dependencies := make(map[string]struct{})

	// Collect dependencies from post-processing query
	collectDependencies(postQuery, dependencies)

	// Collect dependencies from original projection
	if originalQuery.Projection != nil {
		for _, field := range originalQuery.Projection.Include {
			dependencies[field.Name] = struct{}{}
		}
		for _, computed := range originalQuery.Projection.Computed {
			collectDependenciesFromComputedField(computed, dependencies)
		}
	}

	// Augment the dbQuery's projection
	if dbQuery.Projection == nil {
		dbQuery.Projection = &ProjectionConfiguration{}
	}

	// Ensure we don't have conflicting include/exclude
	if len(dbQuery.Projection.Exclude) > 0 {
		return common.NewSystemError("ERR_QUERY_CONFLICTING_PROJECTION", "cannot augment projection with dependencies when an exclude projection is already present").WithOperation("augmentProjection").WithCause(errors.New("conflicting projection"))
	}

	// Add dependencies to the projection
	for field := range dependencies {
		dbQuery.Projection.Include = append(dbQuery.Projection.Include, ProjectionField{Name: field})
	}

	return nil
}

func collectDependencies(q *Query, dependencies map[string]struct{}) {
	if q.Filters != nil {
		collectDependenciesFromFilter(q.Filters, dependencies)
	}
	for _, join := range q.Joins {
		collectDependenciesFromFilter(join.On, dependencies)
	}
	for _, agg := range q.Aggregations {
		dependencies[agg.Field] = struct{}{}
		for _, group := range agg.Groups {
			dependencies[group] = struct{}{}
		}
		if agg.Filter != nil {
			collectDependenciesFromFilter(agg.Filter, dependencies)
		}
	}
	for _, sort := range q.Sort {
		dependencies[sort.Field] = struct{}{}
	}
}

func collectDependenciesFromFilter(filter *QueryFilter, dependencies map[string]struct{}) {
	if filter == nil {
		return
	}
	if filter.Condition != nil {
		dependencies[filter.Condition.Field] = struct{}{}
		collectDependenciesFromFilterValue(&filter.Condition.Value, dependencies)
	}
	if filter.Group != nil {
		for _, f := range filter.Group.Conditions {
			collectDependenciesFromFilter(&f, dependencies)
		}
	}
}

func collectDependenciesFromFilterValue(fv *FilterValue, dependencies map[string]struct{}) {
	if fv.FieldRefVal != nil {
		dependencies[fv.FieldRefVal.Field] = struct{}{}
	}
	if fv.FunctionCallVal != nil {
		for _, arg := range fv.FunctionCallVal.Arguments {
			collectDependenciesFromFilterValue(&arg, dependencies)
		}
	}
	if fv.ArrayVal != nil {
		for _, item := range fv.ArrayVal {
			collectDependenciesFromFilterValue(&item, dependencies)
		}
	}
	// Note: We don't need to collect dependencies from subqueries here
	// because subqueries are self-contained and fetch their own data
}

func collectDependenciesFromComputedField(computed ProjectionComputedItem, dependencies map[string]struct{}) {
	if computed.ComputedFieldExpression != nil && computed.ComputedFieldExpression.Expression != nil {
		for _, arg := range computed.ComputedFieldExpression.Expression.Arguments {
			collectDependenciesFromFilterValue(&arg, dependencies)
		}
	}
	if computed.CaseExpression != nil {
		for _, cond := range computed.CaseExpression.Conditions {
			collectDependenciesFromFilter(&cond.When, dependencies)
			collectDependenciesFromFilterValue(&cond.Then, dependencies)
		}
		collectDependenciesFromFilterValue(&computed.CaseExpression.Else, dependencies)
	}
}
