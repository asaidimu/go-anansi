package query

import (
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// QueryValidationResult represents the result of query validation
type QueryValidationResult struct {
	Valid  bool
	Errors []string
}

// Main QueryBuilder implementation
type QueryBuilder struct {
	query Query
}

var _ QueryBuilderInterface = (*QueryBuilder)(nil)

// NewQueryBuilder creates a new QueryBuilder instance
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		query: Query{},
	}
}

func (qb *QueryBuilder) Build() Query {
	return qb.query
}

func (qb *QueryBuilder) From(source string) *QueryBuilder {
	qb.query.Target = &QueryTarget{
		Name: source,
	}
	return qb
}

func (qb *QueryBuilder) Alias(alias string) *QueryBuilder {
	if qb.query.Target != nil {
		qb.query.Target.Alias = &alias
	}
	return qb
}

func (qb *QueryBuilder) Schema(schema *definition.Schema) *QueryBuilder {
	if qb.query.Target == nil {
		qb.query.Target = &QueryTarget{}
	}
	qb.query.Target.Schema = schema
	return qb
}

func (qb *QueryBuilder) Clone() *QueryBuilder {
	// Deep clone the query structure
	data, err := json.Marshal(qb.query)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal query for cloning: %v", err))
	}
	var clonedQuery Query
	err = json.Unmarshal(data, &clonedQuery)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal cloned query: %v", err))
	}

	return &QueryBuilder{
		query: clonedQuery,
	}
}

func (qb *QueryBuilder) Reset() *QueryBuilder {
	qb.query = Query{}
	return qb
}

func (qb *QueryBuilder) Validate() QueryValidationResult {
	var errors []string

	// Validate filters
	if qb.query.Filters != nil {
		if err := qb.validateQueryFilter(*qb.query.Filters); err != nil {
			errors = append(errors, err.Error())
		}
	}

	// Validate sort configurations
	for _, sort := range qb.query.Sort {
		if sort.Field == "" {
			errors = append(errors, "sort field cannot be empty")
		}
		if sort.Direction != SortDirectionAsc && sort.Direction != SortDirectionDesc {
			errors = append(errors, fmt.Sprintf("invalid sort direction: %s", sort.Direction))
		}
	}

	// Validate pagination
	if qb.query.Pagination != nil {
		if qb.query.Pagination.Limit <= 0 {
			errors = append(errors, "pagination limit must be positive")
		}
		if qb.query.Pagination.Type == "offset" && qb.query.Pagination.Offset != nil && *qb.query.Pagination.Offset < 0 {
			errors = append(errors, "pagination offset cannot be negative")
		}
	}

	// Validate joins
	for _, join := range qb.query.Joins {
		if join.Target.Name == "" {
			errors = append(errors, "join target name cannot be empty")
		}
		if join.On == nil {
			errors = append(errors, "join condition cannot be nil")
		}
	}

	// Validate aggregations
	for _, agg := range qb.query.Aggregations {
		if agg.Type == "" {
			errors = append(errors, "aggregation type cannot be empty")
		}
		if agg.Field == "" && agg.Type != AggregationTypeCount {
			errors = append(errors, "aggregation field cannot be empty for non-count aggregations")
		}
	}

	return QueryValidationResult{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

func (qb *QueryBuilder) validateQueryFilter(filter QueryFilter) error {
    // Check mutually exclusive constraint: Condition XOR Group
    hasCondition := filter.Condition != nil
    hasGroup := filter.Group != nil
    hasTextSearch := filter.TextSearchQuery != nil

    // Must have at least one field
    if !hasCondition && !hasGroup && !hasTextSearch {
        		return ErrQueryFilterMustHaveOneFieldPopulated.WithOperation("validateQueryFilter")
    }

    // Condition and Group are mutually exclusive
    if hasCondition && hasGroup {
        		return ErrQueryFilterMutuallyExclusive.WithOperation("validateQueryFilter")
    }

    // TextSearch can be combined with either Condition or Group, so no additional constraint needed

    // Existing field-specific validation...
    if filter.Condition != nil {
        if filter.Condition.Field == "" {
            			return ErrFilterConditionFieldEmpty.WithOperation("validateQueryFilter")
        }
        if !filter.Condition.Operator.IsStandard() {
            return common.NewSystemError(
                ErrUnknownComparisonOperator.Code,
                fmt.Sprintf("unknown comparison operator: %s", filter.Condition.Operator),
            ).WithOperation("validateQueryFilter").WithCause(ErrUnknownComparisonOperator)
        }
    }

    if filter.Group != nil {
        for _, condition := range filter.Group.Conditions {
            if err := qb.validateQueryFilter(condition); err != nil {
                return err
            }
        }
    }

    if filter.TextSearchQuery != nil {
        if filter.TextSearchQuery.Query == "" {
            return ErrTextSearchQueryEmpty.WithOperation("validateQueryFilter")
        }
        if len(filter.TextSearchQuery.Fields) == 0 {
            return ErrTextSearchFieldsEmpty.WithOperation("validateQueryFilter")
        }
    }

    return nil
}

func (qb *QueryBuilder) String() string {
	data, err := json.MarshalIndent(qb.query, "", "  ")
	if err != nil {
		// This should ideally not happen for a well-formed Query struct.
		// Log the error if a logger is available, or return an empty string.
		return ""
	}
	return string(data)
}

// Filtering
func (qb *QueryBuilder) Where(field string) FilterConditionBuilderInterface {
	return &FilterConditionBuilder{
		queryBuilder: qb,
		field:        field,
	}
}

func (qb *QueryBuilder) WhereGroup(operator common.LogicalOperator) FilterGroupBuilderInterface {
	return &FilterGroupBuilder{
		queryBuilder: qb,
		operator:     operator,
		conditions:   []QueryFilter{},
	}
}

func (qb *QueryBuilder) TextSearch(field string) TextSearchBuilderInterface {
	return &TextSearchBuilder{
		queryBuilder: qb,
		field:        field,
	}
}

// Filter concatenation
func (qb *QueryBuilder) ApplyFilter(newFilter QueryFilter) *QueryBuilder {
	return qb.AndFilter(newFilter)
}

func (qb *QueryBuilder) AndFilter(newFilter QueryFilter) *QueryBuilder {
	if qb.query.Filters == nil {
		qb.query.Filters = &newFilter
	} else {
		qb.query.Filters = &QueryFilter{
			Group: &FilterGroup{
				Operator:   common.LogicalAnd,
				Conditions: []QueryFilter{*qb.query.Filters, newFilter},
			},
		}
	}
	return qb
}

func (qb *QueryBuilder) OrFilter(newFilter QueryFilter) *QueryBuilder {
	if qb.query.Filters == nil {
		qb.query.Filters = &newFilter
	} else {
		qb.query.Filters = &QueryFilter{
			Group: &FilterGroup{
				Operator:   common.LogicalOr,
				Conditions: []QueryFilter{*qb.query.Filters, newFilter},
			},
		}
	}
	return qb
}

// Sorting
func (qb *QueryBuilder) OrderBy(field string, direction SortDirection) *QueryBuilder {
	qb.query.Sort = []SortConfiguration{
		{Field: field, Direction: direction},
	}
	return qb
}

func (qb *QueryBuilder) OrderByAsc(field string) *QueryBuilder {
	return qb.OrderBy(field, SortDirectionAsc)
}

func (qb *QueryBuilder) OrderByDesc(field string) *QueryBuilder {
	return qb.OrderBy(field, SortDirectionDesc)
}

func (qb *QueryBuilder) ThenSortBy(field string, direction SortDirection) *QueryBuilder {
	qb.query.Sort = append(qb.query.Sort, SortConfiguration{
		Field:     field,
		Direction: direction,
	})
	return qb
}

func (qb *QueryBuilder) ThenSortByAsc(field string) *QueryBuilder {
	return qb.ThenSortBy(field, SortDirectionAsc)
}

func (qb *QueryBuilder) ThenSortByDesc(field string) *QueryBuilder {
	return qb.ThenSortBy(field, SortDirectionDesc)
}

// Pagination
func (qb *QueryBuilder) Limit(limit int) *QueryBuilder {
	if qb.query.Pagination == nil {
		qb.query.Pagination = &PaginationOptions{
			Type: "offset",
		}
	}
	qb.query.Pagination.Limit = limit
	return qb
}

func (qb *QueryBuilder) Offset(offset int) *QueryBuilder {
	if qb.query.Pagination == nil {
		qb.query.Pagination = &PaginationOptions{
			Type: "offset",
		}
	}
	qb.query.Pagination.Offset = &offset
	return qb
}



// Projection
func (qb *QueryBuilder) Select() ProjectionBuilderInterface {
	return &ProjectionBuilder{
		queryBuilder: qb,
		projection:   &ProjectionConfiguration{},
	}
}

// Joins
func (qb *QueryBuilder) Join(joinType JoinType, target string) JoinBuilderInterface {
	return &JoinBuilder{
		queryBuilder: qb,
		joinConfig: &JoinConfiguration{
			Type:   joinType,
			Target: QueryTarget{Name: target},
		},
	}
}

func (qb *QueryBuilder) InnerJoin(target string) JoinBuilderInterface {
	return qb.Join(JoinTypeInner, target)
}

func (qb *QueryBuilder) LeftJoin(target string) JoinBuilderInterface {
	return qb.Join(JoinTypeLeft, target)
}

func (qb *QueryBuilder) RightJoin(target string) JoinBuilderInterface {
	return qb.Join(JoinTypeRight, target)
}

func (qb *QueryBuilder) FullJoin(target string) JoinBuilderInterface {
	return qb.Join(JoinTypeFull, target)
}

// Aggregations
func (qb *QueryBuilder) Aggregate(aggType AggregationType, field string, alias string) *QueryBuilder {
	var aliasPtr *string
	if alias != "" {
		aliasPtr = &alias
	}

	agg := AggregationConfiguration{
		Type:  aggType,
		Field: field,
		Alias: aliasPtr,
	}
	qb.query.Aggregations = append(qb.query.Aggregations, agg)
	return qb
}

func (qb *QueryBuilder) Count(field string, alias string) *QueryBuilder {
	return qb.Aggregate(AggregationTypeCount, field, alias)
}

func (qb *QueryBuilder) Sum(field string, alias string) *QueryBuilder {
	return qb.Aggregate(AggregationTypeSum, field, alias)
}

func (qb *QueryBuilder) Avg(field string, alias string) *QueryBuilder {
	return qb.Aggregate(AggregationTypeAvg, field, alias)
}

func (qb *QueryBuilder) Min(field string, alias string) *QueryBuilder {
	return qb.Aggregate(AggregationTypeMin, field, alias)
}

func (qb *QueryBuilder) Max(field string, alias string) *QueryBuilder {
	return qb.Aggregate(AggregationTypeMax, field, alias)
}

func (qb *QueryBuilder) GroupBy(groupField string) AggregationBuilderInterface {
	return &AggregationBuilder{
		queryBuilder: qb,
		aggregation: &AggregationConfiguration{
			Groups: []string{groupField},
		},
	}
}

// Distinct
func (qb *QueryBuilder) Distinct() *QueryBuilder {
	isDistinct := true
	qb.query.Distinct = &QueryDistinctConfig{
		IsDistinct: &isDistinct,
	}
	return qb
}

func (qb *QueryBuilder) DistinctBy(fields ...string) *QueryBuilder {
	qb.query.Distinct = &QueryDistinctConfig{
		Fields: fields,
	}
	return qb
}

// Union Operations
func (qb *QueryBuilder) Union(otherQuery Query) *QueryBuilder {
	if qb.query.Union == nil {
		qb.query.Union = &QueryUnion{
			Queries: []Query{qb.query, otherQuery},
			Type:    "union",
		}
	} else {
		qb.query.Union.Queries = append(qb.query.Union.Queries, otherQuery)
	}
	return qb
}

func (qb *QueryBuilder) UnionAll(otherQuery Query) *QueryBuilder {
	if qb.query.Union == nil {
		qb.query.Union = &QueryUnion{
			Queries: []Query{qb.query, otherQuery},
			Type:    "all",
		}
	} else {
		qb.query.Union.Queries = append(qb.query.Union.Queries, otherQuery)
	}
	return qb
}

func (qb *QueryBuilder) Intersect(otherQuery Query) *QueryBuilder {
	if qb.query.Union == nil {
		qb.query.Union = &QueryUnion{
			Queries: []Query{qb.query, otherQuery},
			Type:    "intersect",
		}
	} else {
		qb.query.Union.Queries = append(qb.query.Union.Queries, otherQuery)
	}
	return qb
}

func (qb *QueryBuilder) Except(otherQuery Query) *QueryBuilder {
	if qb.query.Union == nil {
		qb.query.Union = &QueryUnion{
			Queries: []Query{qb.query, otherQuery},
			Type:    "except",
		}
	} else {
		qb.query.Union.Queries = append(qb.query.Union.Queries, otherQuery)
	}
	return qb
}

// Query Hints
func (qb *QueryBuilder) AddHint(hintType string) *QueryBuilder {
	hint := QueryHint{
		"type": hintType,
	}
	qb.query.Hints = append(qb.query.Hints, hint)
	return qb
}

func (qb *QueryBuilder) UseIndex(index string) *QueryBuilder {
	hint := QueryHint{
		"type":  "use_index",
		"index": index,
	}
	qb.query.Hints = append(qb.query.Hints, hint)
	return qb
}

func (qb *QueryBuilder) ForceIndex(index string) *QueryBuilder {
	hint := QueryHint{
		"type":  "force_index",
		"index": index,
	}
	qb.query.Hints = append(qb.query.Hints, hint)
	return qb
}

func (qb *QueryBuilder) NoIndex(index string) *QueryBuilder {
	hint := QueryHint{
		"type":  "no_index",
		"index": index,
	}
	qb.query.Hints = append(qb.query.Hints, hint)
	return qb
}

func (qb *QueryBuilder) MaxExecutionTime(seconds int) *QueryBuilder {
	hint := QueryHint{
		"type":    "max_execution_time",
		"seconds": seconds,
	}
	qb.query.Hints = append(qb.query.Hints, hint)
	return qb
}

// FilterConditionBuilder implementation
type FilterConditionBuilder struct {
	queryBuilder *QueryBuilder
	field        string
}

func (fcb *FilterConditionBuilder) buildCondition(operator ComparisonOperator, value any) *QueryBuilder {
	condition := QueryFilter{
		Condition: &FilterCondition{
			Field:    fcb.field,
			Operator: operator,
			Value:    convertToFilterValue(value),
		},
	}
	return fcb.queryBuilder.AndFilter(condition)
}

func (fcb *FilterConditionBuilder) Eq(value any) *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorEq, value)
}

func (fcb *FilterConditionBuilder) Neq(value any) *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorNeq, value)
}

func (fcb *FilterConditionBuilder) Lt(value any) *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorLt, value)
}

func (fcb *FilterConditionBuilder) Lte(value any) *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorLte, value)
}

func (fcb *FilterConditionBuilder) Gt(value any) *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorGt, value)
}

func (fcb *FilterConditionBuilder) Gte(value any) *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorGte, value)
}

func (fcb *FilterConditionBuilder) In(values ...any) *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorIn, values)
}

func (fcb *FilterConditionBuilder) Nin(values ...any) *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorNin, values)
}

func (fcb *FilterConditionBuilder) Contains(value any) *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorContains, value)
}

func (fcb *FilterConditionBuilder) NotContains(value any) *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorNotContains, value)
}

func (fcb *FilterConditionBuilder) Exists() *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorExists, true)
}

func (fcb *FilterConditionBuilder) NotExists() *QueryBuilder {
	return fcb.buildCondition(ComparisonOperatorNotExists, true)
}

func (fcb *FilterConditionBuilder) Custom(operator ComparisonOperator, value any) *QueryBuilder {
	return fcb.buildCondition(operator, value)
}

// FilterGroupBuilder implementation
type FilterGroupBuilder struct {
	queryBuilder *QueryBuilder
	operator     common.LogicalOperator
	conditions   []QueryFilter
}

func (fgb *FilterGroupBuilder) Where(field string) FilterConditionBuilderInGroupInterface {
	return &FilterConditionBuilderInGroup{
		groupBuilder: fgb,
		field:        field,
	}
}

func (fgb *FilterGroupBuilder) WhereGroup(operator common.LogicalOperator) FilterGroupBuilderInterface {
	// This creates a new sub-group builder. The resulting group from this new builder
	// would need to be added to the current group builder via the .Group() method.
	return &FilterGroupBuilder{
		queryBuilder: fgb.queryBuilder, // The top-level query builder
		operator:     operator,
		conditions:   []QueryFilter{},
	}
}

func (fgb *FilterGroupBuilder) Group(filter QueryFilter) *FilterGroupBuilder {
	fgb.conditions = append(fgb.conditions, filter)
	return fgb
}

func (fgb *FilterGroupBuilder) End() *QueryBuilder {
	groupFilter := QueryFilter{
		Group: &FilterGroup{
			Operator:   fgb.operator,
			Conditions: fgb.conditions,
		},
	}
	return fgb.queryBuilder.AndFilter(groupFilter)
}

func (fgb *FilterGroupBuilder) WhereTextSearch(field string) TextSearchBuilderInGroupInterface {
	return &TextSearchBuilderInGroup{
		groupBuilder: fgb,
		field:        field,
	}
}

// FilterConditionBuilderInGroup implementation
type FilterConditionBuilderInGroup struct {
	groupBuilder *FilterGroupBuilder
	field        string
}

func (fcbig *FilterConditionBuilderInGroup) buildCondition(operator ComparisonOperator, value any) *FilterGroupBuilder {
	condition := QueryFilter{
		Condition: &FilterCondition{
			Field:    fcbig.field,
			Operator: operator,
			Value:    convertToFilterValue(value),
		},
	}
	fcbig.groupBuilder.conditions = append(fcbig.groupBuilder.conditions, condition)
	return fcbig.groupBuilder
}

func (fcbig *FilterConditionBuilderInGroup) Eq(value any) *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorEq, value)
}

func (fcbig *FilterConditionBuilderInGroup) Neq(value any) *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorNeq, value)
}

func (fcbig *FilterConditionBuilderInGroup) Lt(value any) *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorLt, value)
}

func (fcbig *FilterConditionBuilderInGroup) Lte(value any) *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorLte, value)
}

func (fcbig *FilterConditionBuilderInGroup) Gt(value any) *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorGt, value)
}

func (fcbig *FilterConditionBuilderInGroup) Gte(value any) *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorGte, value)
}

func (fcbig *FilterConditionBuilderInGroup) In(values ...any) *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorIn, values)
}

func (fcbig *FilterConditionBuilderInGroup) Nin(values ...any) *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorNin, values)
}

func (fcbig *FilterConditionBuilderInGroup) Contains(value any) *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorContains, value)
}

func (fcbig *FilterConditionBuilderInGroup) NotContains(value any) *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorNotContains, value)
}

func (fcbig *FilterConditionBuilderInGroup) Exists() *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorExists, true)
}

func (fcbig *FilterConditionBuilderInGroup) NotExists() *FilterGroupBuilder {
	return fcbig.buildCondition(ComparisonOperatorNotExists, true)
}

func (fcbig *FilterConditionBuilderInGroup) Custom(operator ComparisonOperator, value any) *FilterGroupBuilder {
	return fcbig.buildCondition(operator, value)
}

// TextSearchBuilder implementation
type TextSearchBuilder struct {
	queryBuilder *QueryBuilder
	field        string
}

func (tsb *TextSearchBuilder) buildTextSearch(searchType TextSearchType, query string) *QueryBuilder {
	textSearch := QueryFilter{
		TextSearchQuery: &TextSearchQuery{
			Query:  query,
			Fields: []string{tsb.field},
			Type:   searchType,
		},
	}
	return tsb.queryBuilder.AndFilter(textSearch)
}

func (tsb *TextSearchBuilder) Contains(query string) *QueryBuilder {
	return tsb.buildTextSearch(TextSearchTypeContains, query)
}

func (tsb *TextSearchBuilder) Exact(query string) *QueryBuilder {
	return tsb.buildTextSearch(TextSearchTypeExact, query)
}

func (tsb *TextSearchBuilder) Phrase(query string) *QueryBuilder {
	return tsb.buildTextSearch(TextSearchTypePhrase, query)
}

// TextSearchBuilderInGroup implementation
type TextSearchBuilderInGroup struct {
	groupBuilder *FilterGroupBuilder
	field        string
}

func (tsbig *TextSearchBuilderInGroup) buildTextSearch(searchType TextSearchType, query string) *FilterGroupBuilder {
	textSearch := QueryFilter{
		TextSearchQuery: &TextSearchQuery{
			Query:  query,
			Fields: []string{tsbig.field},
			Type:   searchType,
		},
	}
	tsbig.groupBuilder.conditions = append(tsbig.groupBuilder.conditions, textSearch)
	return tsbig.groupBuilder
}

func (tsbig *TextSearchBuilderInGroup) Contains(query string) *FilterGroupBuilder {
	return tsbig.buildTextSearch(TextSearchTypeContains, query)
}

func (tsbig *TextSearchBuilderInGroup) Exact(query string) *FilterGroupBuilder {
	return tsbig.buildTextSearch(TextSearchTypeExact, query)
}

func (tsbig *TextSearchBuilderInGroup) Phrase(query string) *FilterGroupBuilder {
	return tsbig.buildTextSearch(TextSearchTypePhrase, query)
}

// ProjectionBuilder implementation
type ProjectionBuilder struct {
	queryBuilder *QueryBuilder
	projection   *ProjectionConfiguration
}

func (pb *ProjectionBuilder) Include(fields ...string) *ProjectionBuilder {
	for _, field := range fields {
		pb.projection.Include = append(pb.projection.Include, ProjectionField{
			Name: field,
		})
	}
	return pb
}

func (pb *ProjectionBuilder) IncludeNested(field string, nestedConfig *ProjectionConfiguration) *ProjectionBuilder {
	pb.projection.Include = append(pb.projection.Include, ProjectionField{
		Name:   field,
		Nested: nestedConfig,
	})
	return pb
}

func (pb *ProjectionBuilder) Exclude(fields ...string) *ProjectionBuilder {
	for _, field := range fields {
		pb.projection.Exclude = append(pb.projection.Exclude, ProjectionField{
			Name: field,
		})
	}
	return pb
}

func (pb *ProjectionBuilder) AddComputed(alias string, functionName string, args ...any) *ProjectionBuilder {
	var filterArgs []FilterValue
	for _, arg := range args {
		filterArgs = append(filterArgs, convertToFilterValue(arg))
	}

	computed := ProjectionComputedItem{
		ComputedFieldExpression: &ComputedFieldExpression{
			Type: "computed_field",
			Expression: &FunctionCall{
				Function:  functionName,
				Arguments: filterArgs,
			},
			Alias: alias,
		},
	}
	pb.projection.Computed = append(pb.projection.Computed, computed)
	return pb
}

func (pb *ProjectionBuilder) AddCase(alias string) CaseExpressionBuilderInterface {
	return &CaseExpressionBuilder{
		projectionBuilder: pb,
		alias:             alias,
		conditions:        []CaseCondition{},
	}
}

func (pb *ProjectionBuilder) End() *QueryBuilder {
	pb.queryBuilder.query.Projection = pb.projection
	return pb.queryBuilder
}

// CaseExpressionBuilder implementation
type CaseExpressionBuilder struct {
	projectionBuilder *ProjectionBuilder
	alias             string
	conditions        []CaseCondition
	elseValue         *FilterValue
}

func (ceb *CaseExpressionBuilder) When(filter QueryFilter, then any) *CaseExpressionBuilder {
	ceb.conditions = append(ceb.conditions, CaseCondition{
		When: filter,
		Then: convertToFilterValue(then),
	})
	return ceb
}

func (ceb *CaseExpressionBuilder) Else(value any) *CaseExpressionBuilder {
	elseVal := convertToFilterValue(value)
	ceb.elseValue = &elseVal
	return ceb
}

func (ceb *CaseExpressionBuilder) End() *ProjectionBuilder {
	var elseFilterValue FilterValue
	if ceb.elseValue != nil {
		elseFilterValue = *ceb.elseValue
	}

	caseExprItem := ProjectionComputedItem{
		CaseExpression: &CaseExpression{
			Type:       "case",
			Conditions: ceb.conditions,
			Else:       elseFilterValue,
			Alias:      ceb.alias,
		},
	}

	pb := ceb.projectionBuilder
	pb.projection.Computed = append(pb.projection.Computed, caseExprItem)
	return pb
}

// JoinBuilder implementation
type JoinBuilder struct {
	queryBuilder *QueryBuilder
	joinConfig   *JoinConfiguration
}

func (jb *JoinBuilder) On(filter QueryFilter) *JoinBuilder {
	jb.joinConfig.On = &filter
	return jb
}

func (jb *JoinBuilder) Alias(alias string) *JoinBuilder {
	jb.joinConfig.Target.Alias = &alias
	return jb
}

func (jb *JoinBuilder) Schema(schema *definition.Schema) *JoinBuilder {
	jb.joinConfig.Target.Schema = schema
	return jb
}

func (jb *JoinBuilder) WithProjection(projection *ProjectionConfiguration) *JoinBuilder {
	jb.joinConfig.Projection = projection
	return jb
}

func (jb *JoinBuilder) End() *QueryBuilder {
	jb.queryBuilder.query.Joins = append(jb.queryBuilder.query.Joins, *jb.joinConfig)
	return jb.queryBuilder
}

// AggregationBuilder implementation
type AggregationBuilder struct {
	queryBuilder *QueryBuilder
	aggregation  *AggregationConfiguration
}

func (ab *AggregationBuilder) Type(aggType AggregationType) *AggregationBuilder {
	ab.aggregation.Type = aggType
	return ab
}

func (ab *AggregationBuilder) Field(field string) *AggregationBuilder {
	ab.aggregation.Field = field
	return ab
}

func (ab *AggregationBuilder) Alias(alias string) *AggregationBuilder {
	if alias != "" {
		ab.aggregation.Alias = &alias
	}
	return ab
}

func (ab *AggregationBuilder) AddGroup(groupField string) *AggregationBuilder {
	ab.aggregation.Groups = append(ab.aggregation.Groups, groupField)
	return ab
}

func (ab *AggregationBuilder) WithFilter(filter QueryFilter) *AggregationBuilder {
	ab.aggregation.Filter = &filter
	return ab
}

func (ab *AggregationBuilder) End() *QueryBuilder {
	ab.queryBuilder.query.Aggregations = append(ab.queryBuilder.query.Aggregations, *ab.aggregation)
	return ab.queryBuilder
}

func convertToFilterValue(value any) FilterValue {
	switch v := value.(type) {
	case string:
		return FilterValue{StringVal: &v}
	case float64:
		return FilterValue{NumberVal: &v}
	case int:
		floatVal := float64(v)
		return FilterValue{NumberVal: &floatVal}
	case bool:
		return FilterValue{BoolVal: &v}
	case []any:
		var arrayVal []FilterValue
		for _, item := range v {
			arrayVal = append(arrayVal, convertToFilterValue(item))
		}
		return FilterValue{ArrayVal: arrayVal}
	case map[string]any:
		return FilterValue{ObjectVal: v}
	case *FieldReference:
		return FilterValue{FieldRefVal: v}
	case FieldReference:
		return FilterValue{FieldRefVal: &v}
	case *SubqueryValue:
		return FilterValue{SubqueryVal: v}
	case SubqueryValue:
		return FilterValue{SubqueryVal: &v}
	case *FunctionCall:
		return FilterValue{FunctionCallVal: v}
	case FunctionCall:
		return FilterValue{FunctionCallVal: &v}
	default:
		// Try to convert to string as fallback
		str := fmt.Sprintf("%v", v)
		return FilterValue{StringVal: &str}
	}
}
