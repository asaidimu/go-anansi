package query

import (
	"fmt"

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
	dbQuery := &Query{
		Target: dsl.Target,
	}

	postProcessingQuery := &Query{
		Target: dsl.Target,
	}

	// Partition filters
	dbFilters, postFilters, err := p.partitionFilters(dsl.Filters)
	if err != nil {
		return nil, nil, err
	}
	dbQuery.Filters = dbFilters
	postProcessingQuery.Filters = postFilters

	// Partition joins
	dbJoins, postJoins := p.partitionJoins(dsl.Joins)
	dbQuery.Joins = dbJoins
	postProcessingQuery.Joins = postJoins

	// Partition aggregations
	dbAggregations, postAggregations := p.partitionAggregations(dsl.Aggregations)
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
		if p.isConditionSupported(filter.Condition) {
			return filter, nil, nil // DB can handle it
		} else {
			return nil, filter, nil // Must be handled in post-processing
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

	return nil, nil, fmt.Errorf("invalid filter structure")
}

func (p *QueryPartitioner) isConditionSupported(cond *FilterCondition) bool {
	if _, supported := p.capabilities.SupportedComparisonOperators[cond.Operator]; supported {
		// Further checks could be added here, e.g., for function support in filter values
		return true
	}
	return false
}

func (p *QueryPartitioner) partitionJoins(joins []JoinConfiguration) ([]JoinConfiguration, []JoinConfiguration) {
	var dbJoins, postJoins []JoinConfiguration
	for _, join := range joins {
		if _, supported := p.capabilities.SupportedJoinTypes[join.Type]; supported {
			dbJoins = append(dbJoins, join)
		} else {
			postJoins = append(postJoins, join)
		}
	}
	return dbJoins, postJoins
}

func (p *QueryPartitioner) partitionAggregations(aggregations []AggregationConfiguration) ([]AggregationConfiguration, []AggregationConfiguration) {
	var dbAggregations, postAggregations []AggregationConfiguration
	for _, agg := range aggregations {
		if _, supported := p.capabilities.SupportedAggregationFunctions[agg.Type]; supported {
			dbAggregations = append(dbAggregations, agg)
		} else {
			postAggregations = append(postAggregations, agg)
		}
	}
	return dbAggregations, postAggregations
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
	if pagination == nil {
		return true
	}
	_, supported := p.capabilities.SupportedPaginationTypes[PaginationType(pagination.Type)]
	return supported
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
		return fmt.Errorf("cannot augment projection with dependencies when an exclude projection is already present")
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
