// Package query provides a fluent API for building complex database queries
// using a structured QueryDSL. This builder ensures that queries are constructed
// in a type-safe and readable manner.
package query

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v5/core/schema"
)

// QueryBuilder provides a fluent and intuitive API for building QueryDSL structures.
// It allows for the step-by-step construction of a query, including filters, sorting,
// pagination, and more, culminating in a final QueryDSL object.
type QueryBuilder struct {
	query QueryDSL
}

// NewQueryBuilder creates a new, empty query builder instance.
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		query: QueryDSL{},
	}
}

// Build returns the constructed QueryDSL object.
func (qb *QueryBuilder) Build() QueryDSL {
	return qb.query
}

// Clone creates a deep copy of the current query builder, allowing for the creation
// of new queries based on an existing one without modifying the original.
func (qb *QueryBuilder) Clone() *QueryBuilder {
	newBuilder := &QueryBuilder{}
	// Note: This is a shallow copy. For a true deep copy, each field would need to be cloned.
	newBuilder.query = qb.query
	return newBuilder
}

// Reset clears all configurations from the query builder, returning it to its initial state.
func (qb *QueryBuilder) Reset() *QueryBuilder {
	qb.query = QueryDSL{}
	return qb
}

// FilterBuilder is a helper struct for building filter conditions.
// It is not intended to be used directly but is part of the fluent API.
type FilterBuilder struct {
	parent *QueryBuilder
	filter *QueryFilter
}

// Where begins the construction of a filter condition for a specific field.
func (qb *QueryBuilder) Where(field string) *FilterConditionBuilder {
	fb := &FilterBuilder{parent: qb}
	return &FilterConditionBuilder{
		filterBuilder: fb,
		field:         field,
	}
}

// WhereGroup begins the construction of a group of filter conditions, combined
// with a logical operator (AND or OR).
func (qb *QueryBuilder) WhereGroup(operator schema.LogicalOperator) *FilterGroupBuilder {
	fb := &FilterBuilder{parent: qb}
	return &FilterGroupBuilder{
		filterBuilder: fb,
		operator:      operator,
		conditions:    []QueryFilter{},
	}
}

// FilterConditionBuilder is used to build a single filter condition (e.g., field = value).
type FilterConditionBuilder struct {
	filterBuilder *FilterBuilder
	field         string
}

// Eq adds an equality condition to the query.
func (fcb *FilterConditionBuilder) Eq(value FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorEq, value)
}

// Neq adds a not-equal condition to the query.
func (fcb *FilterConditionBuilder) Neq(value FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorNeq, value)
}

// Lt adds a less-than condition to the query.
func (fcb *FilterConditionBuilder) Lt(value FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorLt, value)
}

// Lte adds a less-than-or-equal condition to the query.
func (fcb *FilterConditionBuilder) Lte(value FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorLte, value)
}

// Gt adds a greater-than condition to the query.
func (fcb *FilterConditionBuilder) Gt(value FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorGt, value)
}

// Gte adds a greater-than-or-equal condition to the query.
func (fcb *FilterConditionBuilder) Gte(value FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorGte, value)
}

// In adds an "in" condition, checking if a field's value is within a set of values.
func (fcb *FilterConditionBuilder) In(values ...FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorIn, values)
}

// Nin adds a "not in" condition, checking if a field's value is not within a set of values.
func (fcb *FilterConditionBuilder) Nin(values ...FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorNin, values)
}

// Contains adds a condition to check if a string field contains a substring.
func (fcb *FilterConditionBuilder) Contains(value FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorContains, value)
}

// NotContains adds a condition to check if a string field does not contain a substring.
func (fcb *FilterConditionBuilder) NotContains(value FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorNotContains, value)
}

// StartsWith adds a condition to check if a string field starts with a specific prefix.
func (fcb *FilterConditionBuilder) StartsWith(value FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorStartsWith, value)
}

// EndsWith adds a condition to check if a string field ends with a specific suffix.
func (fcb *FilterConditionBuilder) EndsWith(value FilterValue) *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorEndsWith, value)
}

// Exists adds a condition to check if a field exists and is not null.
func (fcb *FilterConditionBuilder) Exists() *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorExists, true)
}

// NotExists adds a condition to check if a field does not exist or is null.
func (fcb *FilterConditionBuilder) NotExists() *QueryBuilder {
	return fcb.addCondition(ComparisonOperatorNotExists, true)
}

// Custom allows for the use of a custom comparison operator.
func (fcb *FilterConditionBuilder) Custom(operator ComparisonOperator, value FilterValue) *QueryBuilder {
	return fcb.addCondition(operator, value)
}

// addCondition is an internal helper to add a filter condition to the query.
func (fcb *FilterConditionBuilder) addCondition(operator ComparisonOperator, value FilterValue) *QueryBuilder {
	condition := &FilterCondition{
		Field:    fcb.field,
		Operator: operator,
		Value:    value,
	}

	filter := QueryFilter{Condition: condition}
	fcb.filterBuilder.parent.query.Filters = &filter
	return fcb.filterBuilder.parent
}

// FilterGroupBuilder is used to build a group of filter conditions.
type FilterGroupBuilder struct {
	filterBuilder *FilterBuilder
	operator      schema.LogicalOperator
	conditions    []QueryFilter
}

// Where adds a new condition to the current filter group.
func (fgb *FilterGroupBuilder) Where(field string) *FilterConditionBuilderInGroup {
	return &FilterConditionBuilderInGroup{
		groupBuilder: fgb,
		field:        field,
	}
}

// WhereGroup adds a nested group of filters to the current group.
func (fgb *FilterGroupBuilder) WhereGroup(operator schema.LogicalOperator) *FilterGroupBuilder {
	return &FilterGroupBuilder{
		filterBuilder: fgb.filterBuilder,
		operator:      operator,
		conditions:    []QueryFilter{},
	}
}

// End finalizes the current filter group and returns to the main query builder.
func (fgb *FilterGroupBuilder) End() *QueryBuilder {
	group := &FilterGroup{
		Operator:   fgb.operator,
		Conditions: fgb.conditions,
	}

	filter := QueryFilter{Group: group}
	fgb.filterBuilder.parent.query.Filters = &filter
	return fgb.filterBuilder.parent
}

// FilterConditionBuilderInGroup is used to build a filter condition within a group.
type FilterConditionBuilderInGroup struct {
	groupBuilder *FilterGroupBuilder
	field        string
}

// Eq adds an equality condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) Eq(value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorEq, value)
}

// Neq adds a not-equal condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) Neq(value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorNeq, value)
}

// Lt adds a less-than condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) Lt(value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorLt, value)
}

// Lte adds a less-than-or-equal condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) Lte(value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorLte, value)
}

// Gt adds a greater-than condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) Gt(value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorGt, value)
}

// Gte adds a greater-than-or-equal condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) Gte(value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorGte, value)
}

// In adds an "in" condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) In(values ...FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorIn, values)
}

// Nin adds a "not in" condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) Nin(values ...FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorNin, values)
}

// Contains adds a contains condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) Contains(value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorContains, value)
}

// NotContains adds a not-contains condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) NotContains(value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorNotContains, value)
}

// StartsWith adds a starts-with condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) StartsWith(value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorStartsWith, value)
}

// EndsWith adds an ends-with condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) EndsWith(value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorEndsWith, value)
}

// Exists adds an exists condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) Exists() *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorExists, true)
}

// NotExists adds a not-exists condition to the current filter group.
func (fcbg *FilterConditionBuilderInGroup) NotExists() *FilterGroupBuilder {
	return fcbg.addConditionToGroup(ComparisonOperatorNotExists, true)
}

// Custom allows for custom comparison operators within a filter group.
func (fcbg *FilterConditionBuilderInGroup) Custom(operator ComparisonOperator, value FilterValue) *FilterGroupBuilder {
	return fcbg.addConditionToGroup(operator, value)
}

// addConditionToGroup is an internal helper to add a condition to a filter group.
func (fcbg *FilterConditionBuilderInGroup) addConditionToGroup(operator ComparisonOperator, value FilterValue) *FilterGroupBuilder {
	condition := &FilterCondition{
		Field:    fcbg.field,
		Operator: operator,
		Value:    value,
	}

	filter := QueryFilter{Condition: condition}
	fcbg.groupBuilder.conditions = append(fcbg.groupBuilder.conditions, filter)
	return fcbg.groupBuilder
}

// OrderBy adds a sorting configuration to the query.
func (qb *QueryBuilder) OrderBy(field string, direction SortDirection) *QueryBuilder {
	sort := SortConfiguration{
		Field:     field,
		Direction: direction,
	}
	qb.query.Sort = append(qb.query.Sort, sort)
	return qb
}

// OrderByAsc adds an ascending sort order for a specific field.
func (qb *QueryBuilder) OrderByAsc(field string) *QueryBuilder {
	return qb.OrderBy(field, SortDirectionAsc)
}

// OrderByDesc adds a descending sort order for a specific field.
func (qb *QueryBuilder) OrderByDesc(field string) *QueryBuilder {
	return qb.OrderBy(field, SortDirectionDesc)
}

// Limit sets the maximum number of records to be returned by the query.
func (qb *QueryBuilder) Limit(limit int) *QueryBuilder {
	if qb.query.Pagination == nil {
		qb.query.Pagination = &PaginationOptions{
			Type: "offset",
		}
	}
	qb.query.Pagination.Limit = limit
	return qb
}

// Offset sets the starting point for the result set, used for offset-based pagination.
func (qb *QueryBuilder) Offset(offset int) *QueryBuilder {
	if qb.query.Pagination == nil {
		qb.query.Pagination = &PaginationOptions{
			Type: "offset",
		}
	}
	qb.query.Pagination.Offset = &offset
	return qb
}

// Cursor sets the starting point for the result set using a cursor, for cursor-based pagination.
func (qb *QueryBuilder) Cursor(cursor string) *QueryBuilder {
	if qb.query.Pagination == nil {
		qb.query.Pagination = &PaginationOptions{
			Type: "cursor",
		}
	}
	qb.query.Pagination.Type = "cursor"
	qb.query.Pagination.Cursor = &cursor
	return qb
}

// ProjectionBuilder is used to build the projection part of a query, which defines
// which fields should be returned.
type ProjectionBuilder struct {
	parent *QueryBuilder
	config *ProjectionConfiguration
}

// Select begins the construction of the projection for the query.
func (qb *QueryBuilder) Select() *ProjectionBuilder {
	if qb.query.Projection == nil {
		qb.query.Projection = &ProjectionConfiguration{}
	}
	return &ProjectionBuilder{
		parent: qb,
		config: qb.query.Projection,
	}
}

// Include specifies which fields should be included in the result set.
func (pb *ProjectionBuilder) Include(fields ...string) *ProjectionBuilder {
	for _, field := range fields {
		pb.config.Include = append(pb.config.Include, ProjectionField{Name: field})
	}
	return pb
}

// IncludeNested allows for specifying projections on nested fields.
func (pb *ProjectionBuilder) IncludeNested(field string, nestedConfig *ProjectionConfiguration) *ProjectionBuilder {
	pb.config.Include = append(pb.config.Include, ProjectionField{
		Name:   field,
		Nested: nestedConfig,
	})
	return pb
}

// Exclude specifies which fields should be excluded from the result set.
func (pb *ProjectionBuilder) Exclude(fields ...string) *ProjectionBuilder {
	for _, field := range fields {
		pb.config.Exclude = append(pb.config.Exclude, ProjectionField{Name: field})
	}
	return pb
}

// AddComputed adds a computed field to the projection, which is calculated at query time.
func (pb *ProjectionBuilder) AddComputed(alias string, function FilterValue, args ...FilterValue) *ProjectionBuilder {
	computed := ProjectionComputedItem{
		ComputedFieldExpression: &ComputedFieldExpression{
			Type: "computed",
			Expression: &FunctionCall{
				Function:  function,
				Arguments: args,
			},
			Alias: alias,
		},
	}
	pb.config.Computed = append(pb.config.Computed, computed)
	return pb
}

// AddCase adds a case expression to the projection, allowing for conditional logic.
func (pb *ProjectionBuilder) AddCase(alias string) *CaseExpressionBuilder {
	return &CaseExpressionBuilder{
		projectionBuilder: pb,
		alias:             alias,
		cases:             []CaseCondition{},
	}
}

// End finalizes the projection and returns to the main query builder.
func (pb *ProjectionBuilder) End() *QueryBuilder {
	return pb.parent
}

// CaseExpressionBuilder is used to build a case expression for a computed field.
type CaseExpressionBuilder struct {
	projectionBuilder *ProjectionBuilder
	alias             string
	cases             []CaseCondition
	elseValue         FilterValue
}

// When adds a condition to the case expression.
func (ceb *CaseExpressionBuilder) When(filter QueryFilter, then FilterValue) *CaseExpressionBuilder {
	ceb.cases = append(ceb.cases, CaseCondition{
		When: filter,
		Then: then,
	})
	return ceb
}

// Else sets the default value for the case expression if no conditions are met.
func (ceb *CaseExpressionBuilder) Else(value FilterValue) *CaseExpressionBuilder {
	ceb.elseValue = value
	return ceb
}

// End finalizes the case expression and adds it to the projection.
func (ceb *CaseExpressionBuilder) End() *ProjectionBuilder {
	computed := ProjectionComputedItem{
		CaseExpression: &CaseExpression{
			Type:  "case",
			Cases: ceb.cases,
			Else:  ceb.elseValue,
			Alias: ceb.alias,
		},
	}
	ceb.projectionBuilder.config.Computed = append(ceb.projectionBuilder.config.Computed, computed)
	return ceb.projectionBuilder
}

// JoinBuilder is used to build a join configuration for the query.
type JoinBuilder struct {
	parent *QueryBuilder
	join   *JoinConfiguration
}

// Join begins the construction of a join with another table.
func (qb *QueryBuilder) Join(joinType JoinType, targetTable string) *JoinBuilder {
	join := &JoinConfiguration{
		Type:        joinType,
		TargetTable: targetTable,
	}
	return &JoinBuilder{
		parent: qb,
		join:   join,
	}
}

// InnerJoin creates an inner join with another table.
func (qb *QueryBuilder) InnerJoin(targetTable string) *JoinBuilder {
	return qb.Join(JoinTypeInner, targetTable)
}

// LeftJoin creates a left join with another table.
func (qb *QueryBuilder) LeftJoin(targetTable string) *JoinBuilder {
	return qb.Join(JoinTypeLeft, targetTable)
}

// RightJoin creates a right join with another table.
func (qb *QueryBuilder) RightJoin(targetTable string) *JoinBuilder {
	return qb.Join(JoinTypeRight, targetTable)
}

// FullJoin creates a full join with another table.
func (qb *QueryBuilder) FullJoin(targetTable string) *JoinBuilder {
	return qb.Join(JoinTypeFull, targetTable)
}

// On sets the join condition.
func (jb *JoinBuilder) On(filter QueryFilter) *JoinBuilder {
	jb.join.On = filter
	return jb
}

// Alias sets an alias for the joined table.
func (jb *JoinBuilder) Alias(alias string) *JoinBuilder {
	jb.join.Alias = alias
	return jb
}

// WithProjection sets the projection for the joined table.
func (jb *JoinBuilder) WithProjection(projection *ProjectionConfiguration) *JoinBuilder {
	jb.join.Projection = projection
	return jb
}

// End finalizes the join and adds it to the query.
func (jb *JoinBuilder) End() *QueryBuilder {
	jb.parent.query.Joins = append(jb.parent.query.Joins, *jb.join)
	return jb.parent
}

// Aggregate adds an aggregation to the query.
func (qb *QueryBuilder) Aggregate(aggType AggregationType, field string, alias string) *QueryBuilder {
	agg := AggregationConfiguration{
		Type:  aggType,
		Field: field,
		Alias: alias,
	}
	qb.query.Aggregations = append(qb.query.Aggregations, agg)
	return qb
}

// Count adds a count aggregation to the query.
func (qb *QueryBuilder) Count(field string, alias string) *QueryBuilder {
	return qb.Aggregate(AggregationTypeCount, field, alias)
}

// Sum adds a sum aggregation to the query.
func (qb *QueryBuilder) Sum(field string, alias string) *QueryBuilder {
	return qb.Aggregate(AggregationTypeSum, field, alias)
}

// Avg adds an average aggregation to the query.
func (qb *QueryBuilder) Avg(field string, alias string) *QueryBuilder {
	return qb.Aggregate(AggregationTypeAvg, field, alias)
}

// Min adds a minimum aggregation to the query.
func (qb *QueryBuilder) Min(field string, alias string) *QueryBuilder {
	return qb.Aggregate(AggregationTypeMin, field, alias)
}

// Max adds a maximum aggregation to the query.
func (qb *QueryBuilder) Max(field string, alias string) *QueryBuilder {
	return qb.Aggregate(AggregationTypeMax, field, alias)
}

// AddHint adds a query hint to the query, which can be used to influence the query execution plan.
func (qb *QueryBuilder) AddHint(hintType string) *QueryBuilder {
	hint := QueryHint{Type: hintType}
	qb.query.Hints = append(qb.query.Hints, hint)
	return qb
}

// UseIndex adds a hint to use a specific index for the query.
func (qb *QueryBuilder) UseIndex(index string) *QueryBuilder {
	hint := QueryHint{
		Type:  "index",
		Index: index,
	}
	qb.query.Hints = append(qb.query.Hints, hint)
	return qb
}

// ForceIndex adds a hint to force the use of a specific index.
func (qb *QueryBuilder) ForceIndex(index string) *QueryBuilder {
	hint := QueryHint{
		Type:  "force_index",
		Index: index,
	}
	qb.query.Hints = append(qb.query.Hints, hint)
	return qb
}

// NoIndex adds a hint to avoid using a specific index.
func (qb *QueryBuilder) NoIndex(index string) *QueryBuilder {
	hint := QueryHint{
		Type:  "no_index",
		Index: index,
	}
	qb.query.Hints = append(qb.query.Hints, hint)
	return qb
}

// MaxExecutionTime sets a hint for the maximum execution time of the query.
func (qb *QueryBuilder) MaxExecutionTime(seconds int) *QueryBuilder {
	hint := QueryHint{
		Type:    "max_execution_time",
		Seconds: seconds,
	}
	qb.query.Hints = append(qb.query.Hints, hint)
	return qb
}

// QueryValidationError represents an error found during query validation.
type QueryValidationError struct {
	Field   string
	Message string
}

// Error returns the error message for a QueryValidationError.
func (ve QueryValidationError) Error() string {
	return fmt.Sprintf("validation error in %s: %s", ve.Field, ve.Message)
}

// QueryValidationResult contains the results of a query validation.
type QueryValidationResult struct {
	IsValid bool
	Errors  []QueryValidationError
}

// Validate performs a comprehensive validation of the built query, checking for
// common errors such as invalid pagination options or conflicting projections.
func (qb *QueryBuilder) Validate() QueryValidationResult {
	var errors []QueryValidationError

	// Validate pagination
	if qb.query.Pagination != nil {
		if qb.query.Pagination.Limit <= 0 {
			errors = append(errors, QueryValidationError{
				Field:   "pagination.limit",
				Message: "limit must be greater than 0",
			})
		}

		if qb.query.Pagination.Type == "offset" && qb.query.Pagination.Offset != nil && *qb.query.Pagination.Offset < 0 {
			errors = append(errors, QueryValidationError{
				Field:   "pagination.offset",
				Message: "offset cannot be negative",
			})
		}

		if qb.query.Pagination.Type == "cursor" && (qb.query.Pagination.Cursor == nil || *qb.query.Pagination.Cursor == "") {
			errors = append(errors, QueryValidationError{
				Field:   "pagination.cursor",
				Message: "cursor cannot be empty for cursor-based pagination",
			})
		}
	}

	// Validate projections
	if qb.query.Projection != nil {
		if len(qb.query.Projection.Include) > 0 && len(qb.query.Projection.Exclude) > 0 {
			errors = append(errors, QueryValidationError{
				Field:   "projection",
				Message: "cannot have both include and exclude fields",
			})
		}
	}

	// Validate joins
	for i, join := range qb.query.Joins {
		if join.TargetTable == "" {
			errors = append(errors, QueryValidationError{
				Field:   fmt.Sprintf("joins[%d].target_table", i),
				Message: "target table cannot be empty",
			})
		}
	}

	// Validate aggregations
	for i, agg := range qb.query.Aggregations {
		if agg.Field == "" && agg.Type != AggregationTypeCount {
			errors = append(errors, QueryValidationError{
				Field:   fmt.Sprintf("aggregations[%d].field", i),
				Message: "field is required for non-count aggregations",
			})
		}
		if agg.Alias == "" {
			errors = append(errors, QueryValidationError{
				Field:   fmt.Sprintf("aggregations[%d].alias", i),
				Message: "alias is required for aggregations",
			})
		}
	}

	return QueryValidationResult{
		IsValid: len(errors) == 0,
		Errors:  errors,
	}
}

// String returns a human-readable representation of the built query.
func (qb *QueryBuilder) String() string {
	var parts []string

	if qb.query.Filters != nil {
		parts = append(parts, "FILTERS: present")
	}

	if len(qb.query.Sort) > 0 {
		sortFields := make([]string, len(qb.query.Sort))
		for i, sort := range qb.query.Sort {
			sortFields[i] = fmt.Sprintf("%s %s", sort.Field, sort.Direction)
		}
		parts = append(parts, fmt.Sprintf("ORDER BY: %s", strings.Join(sortFields, ", ")))
	}

	if qb.query.Pagination != nil {
		if qb.query.Pagination.Type == "offset" {
			parts = append(parts, fmt.Sprintf("LIMIT: %d", qb.query.Pagination.Limit))
			if qb.query.Pagination.Offset != nil {
				parts = append(parts, fmt.Sprintf("OFFSET: %d", *qb.query.Pagination.Offset))
			}
		} else {
			parts = append(parts, fmt.Sprintf("CURSOR LIMIT: %d", qb.query.Pagination.Limit))
		}
	}

	if qb.query.Projection != nil {
		if len(qb.query.Projection.Include) > 0 {
			fields := make([]string, len(qb.query.Projection.Include))
			for i, field := range qb.query.Projection.Include {
				fields[i] = field.Name
			}
			parts = append(parts, fmt.Sprintf("SELECT: %s", strings.Join(fields, ", ")))
		}
		if len(qb.query.Projection.Exclude) > 0 {
			fields := make([]string, len(qb.query.Projection.Exclude))
			for i, field := range qb.query.Projection.Exclude {
				fields[i] = field.Name
			}
			parts = append(parts, fmt.Sprintf("EXCLUDE: %s", strings.Join(fields, ", ")))
		}
	}

	if len(qb.query.Joins) > 0 {
		parts = append(parts, fmt.Sprintf("JOINS: %d", len(qb.query.Joins)))
	}

	if len(qb.query.Aggregations) > 0 {
		parts = append(parts, fmt.Sprintf("AGGREGATIONS: %d", len(qb.query.Aggregations)))
	}

	if len(qb.query.Hints) > 0 {
		parts = append(parts, fmt.Sprintf("HINTS: %d", len(qb.query.Hints)))
	}

	if len(parts) == 0 {
		return "EMPTY QUERY"
	}

	return strings.Join(parts, " | ")
}

// CreateSimpleFilter is a helper function to create a simple filter condition.
func CreateSimpleFilter(field string, operator ComparisonOperator, value FilterValue) QueryFilter {
	return QueryFilter{
		Condition: &FilterCondition{
			Field:    field,
			Operator: operator,
			Value:    value,
		},
	}
}

// CreateFilterGroup is a helper function to create a filter group.
func CreateFilterGroup(operator schema.LogicalOperator, conditions ...QueryFilter) QueryFilter {
	return QueryFilter{
		Group: &FilterGroup{
			Operator:   operator,
			Conditions: conditions,
		},
	}
}

// CreateProjectionConfig is a helper function to create a projection configuration.
func CreateProjectionConfig() *ProjectionConfiguration {
	return &ProjectionConfiguration{}
}

// AddIncludeFields adds fields to be included in a projection configuration.
func (pc *ProjectionConfiguration) AddIncludeFields(fields ...string) *ProjectionConfiguration {
	for _, field := range fields {
		pc.Include = append(pc.Include, ProjectionField{Name: field})
	}
	return pc
}

// AddExcludeFields adds fields to be excluded from a projection configuration.
func (pc *ProjectionConfiguration) AddExcludeFields(fields ...string) *ProjectionConfiguration {
	for _, field := range fields {
		pc.Exclude = append(pc.Exclude, ProjectionField{Name: field})
	}
	return pc
}
