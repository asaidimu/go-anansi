package query

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"go.uber.org/zap"
)

// AggregationFunctionsMap holds a map of supported aggregation functions.
type AggregationFunctionsMap map[AggregationType]AggregateFunction

// QueryHelper provides reusable methods for working with collections of map[string]any records.
// It is portable and can be used by any in-memory persistence implementation.
// This helper supports filtering, sorting, pagination (offset and cursor-based),
// and field projection (include, exclude, and basic computed fields/case expressions).
// It can be extended with custom comparison operators via the PredicateMap.
// Now also supports aggregations and distinct operations.
// Advanced features like text search, joins, unions, and hints are still
// not fully supported by this in-memory helper beyond their DSL structure.
type QueryHelper struct {
	query             *Query
	operators         *ComparisonMap           // Custom operators map
	aggregates        *AggregationFunctionsMap // Custom aggregation functions map
	functions         *FunctionMap             // Custom functions map
	computeFunctions  map[string]ComputeFunction
	goFilterFunctions map[ComparisonOperator]PredicateFunction
	mu                sync.RWMutex
	logger            *zap.Logger
}

// NewQueryHelper creates a new QueryHelper with the given Query and an optional PredicateMap for custom operators,
// and an optional AggregationFunctionsMap for custom aggregation functions.
// It validates the query structure and returns an error if invalid.
// If operators is nil, only standard operators defined in the DSL will be supported.
// If aggregateFunctions is nil, only standard aggregations defined within the helper will be used (if any).
func NewQueryHelper(query *Query, operators *ComparisonMap, aggregateFunctions *AggregationFunctionsMap, registeredFunctions *FunctionMap) (*QueryHelper, error) {
	if query == nil {
		return nil, common.NewSystemError("ERR_QUERY_CANNOT_BE_NIL", "query cannot be nil")
	}

	helper := &QueryHelper{
		query:             query,
		operators:         operators,
		aggregates:        aggregateFunctions,
		functions:         registeredFunctions,
		goFilterFunctions: make(map[ComparisonOperator]PredicateFunction),
		computeFunctions:  make(map[string]ComputeFunction),
		logger:            zap.NewNop(),
	}

	// Validate the query structure
	if err := helper.validateQuery(); err != nil {
		return nil, common.NewSystemError("ERR_QUERY_INVALID_QUERY", "invalid query").WithOperation("NewQueryHelper").WithCause(err)
	}

	return helper, nil
}

// RegisterComputeFunction registers a Go function that can be used for computed fields.
func (h *QueryHelper) RegisterComputeFunction(name string, fn ComputeFunction) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.computeFunctions[name] = fn
	h.logger.Info("Registered compute function", zap.String("name", name))
}

// RegisterFilterFunction registers a Go function that can be used for custom filtering.
func (h *QueryHelper) RegisterFilterFunction(operator ComparisonOperator, fn PredicateFunction) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.goFilterFunctions[operator] = fn
	h.logger.Info("Registered filter function", zap.String("operator", string(operator)))
}

// RegisterComputeFunctions registers multiple compute functions from a map.
func (h *QueryHelper) RegisterComputeFunctions(functionMap map[string]ComputeFunction) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for name, fn := range functionMap {
		h.computeFunctions[name] = fn
		h.logger.Info("Registered compute function", zap.String("name", name))
	}
}

// RegisterFilterFunctions registers multiple filter functions from a map.
func (h *QueryHelper) RegisterFilterFunctions(functionMap map[ComparisonOperator]PredicateFunction) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for operator, fn := range functionMap {
		h.goFilterFunctions[operator] = fn
		h.logger.Info("Registered filter function", zap.String("operator", string(operator)))
	}
}

// validateQuery validates the query structure for common errors and unsupported features.
func (h *QueryHelper) validateQuery() error {
	// Add target validation
	if h.query.Target != nil && h.query.Target.Name == "" {
		return common.NewSystemError("ERR_QUERY_TARGET_NAME_EMPTY", "target name cannot be empty when target is specified")
	}

	// Validate filters
	if h.query.Filters != nil {
		if err := h.validateQueryFilter(h.query.Filters); err != nil {
			return common.NewSystemError("ERR_QUERY_INVALID_FILTERS", "invalid filters").WithOperation("validateQuery").WithCause(err)
		}
	}

	// Validate sort configurations
	for _, sortConfig := range h.query.Sort {
		if sortConfig.Field == "" {
			return ErrSortFieldEmpty
		}
		if sortConfig.Direction != SortDirectionAsc && sortConfig.Direction != SortDirectionDesc {
			return common.NewSystemError(ErrInvalidSortDirection.Code, fmt.Sprintf("invalid sort direction: %s", sortConfig.Direction)).WithOperation("validateQuery").WithCause(ErrInvalidSortDirection)
		}
	}

	// Validate pagination
	if h.query.Pagination != nil {
		if len(h.query.Pagination.Type) > 0 && h.query.Pagination.Type != "offset" {
			return common.NewSystemError(ErrInvalidPaginationType.Code, fmt.Sprintf("invalid pagination type: %s", h.query.Pagination.Type)).WithOperation("validateQuery").WithCause(ErrInvalidPaginationType)
		}
		if h.query.Pagination.Limit <= 0 && !bool(*h.query.Pagination.IncludeTotal){
			return common.NewSystemError("ERR_QUERY_PAGINATION_LIMIT_NOT_POSITIVE", "pagination limit must be greater than 0")
		}
		if h.query.Pagination.Offset != nil && *h.query.Pagination.Offset < 0 {
			return common.NewSystemError("ERR_QUERY_PAGINATION_OFFSET_NEGATIVE", "pagination offset cannot be negative").WithOperation("validateQuery")
		}
	}

	// Validate projection
	if h.query.Projection != nil {
		if err := h.validateProjectionConfiguration(h.query.Projection); err != nil {
			return common.NewSystemError("ERR_QUERY_INVALID_PROJECTION", "invalid projection").WithOperation("validateQuery").WithCause(err)
		}
	}

	// Validate aggregations
	if h.query.Aggregations != nil {
		if err := h.validateAggregationConfiguration(h.query.Aggregations); err != nil {
			return common.NewSystemError("ERR_QUERY_INVALID_AGGREGATION_CONFIGURATION", "invalid aggregation configuration").WithOperation("validateQuery").WithCause(err)
		}
	}

	if h.query.Distinct != nil {
		if err := h.validateDistinctConfiguration(h.query.Distinct); err != nil {
			return common.NewSystemError("ERR_QUERY_INVALID_DISTINCT_CONFIGURATION", "invalid distinct configuration").WithOperation("validateQuery").WithCause(err)
		}
	}

	return nil
}

// validateQueryFilter validates a QueryFilter structure recursively.
func (h *QueryHelper) validateQueryFilter(filter *QueryFilter) error {
	if filter == nil {
		return common.NewSystemError("ERR_QUERY_FILTER_CANNOT_BE_NIL", "filter cannot be nil").WithOperation("validateQueryFilter")
	}

	// Check that exactly one of the union fields is set
	setFields := 0
	if filter.Condition != nil {
		setFields++
	}

	if filter.Group != nil {
		setFields++
	}

	if filter.TextSearchQuery != nil {
		setFields++
	}

	if setFields != 1 {
		return common.NewSystemError("ERR_QUERY_FILTER_MUST_HAVE_ONE_FIELD_POPULATED", "exactly one of Condition, Group, or TextSearchQuery must be set")
	}

	// Validate condition
	if filter.Condition != nil {
		if filter.Condition.Field == "" {
			return common.NewSystemError("ERR_QUERY_FILTER_CONDITION_FIELD_EMPTY", "condition field cannot be empty")
		}

		// *** MODIFIED LOGIC HERE: Check if operator is custom or standard ***
		isCustom := false
		if h.operators != nil {
			if _, ok := (*h.operators)[string(filter.Condition.Operator)]; ok {
				isCustom = true
			}
		}

		if !isCustom && !filter.Condition.Operator.IsStandard() {
			return common.NewSystemError(ErrUnknownComparisonOperator.Code, fmt.Sprintf("unsupported comparison operator: %s", filter.Condition.Operator)).WithOperation("validateQueryFilter").WithCause(ErrUnknownComparisonOperator)
		}
		// End of MODIFIED LOGIC

		// Basic validation for FilterValue - can be extended if needed
		if err := h.validateFilterValue(&filter.Condition.Value); err != nil {
			return common.NewSystemError("ERR_QUERY_INVALID_CONDITION_VALUE", "invalid condition value").WithOperation("validateQueryFilter").WithCause(err)
		}
	}

	// Validate group
	if filter.Group != nil {
		if len(filter.Group.Conditions) == 0 {
			return common.NewSystemError("ERR_QUERY_FILTER_GROUP_EMPTY", "filter group must have at least one condition")
		}
		for i, condition := range filter.Group.Conditions {
			// Pass a pointer to the condition to allow recursive validation
			if err := h.validateQueryFilter(&condition); err != nil {
				return common.NewSystemError("ERR_QUERY_INVALID_CONDITION_AT_INDEX", fmt.Sprintf("invalid condition at index %d", i)).WithOperation("validateQueryFilter").WithCause(err)
			}
		}
	}

	// TextMatch validation - return error since it's not implemented yet but allowed in DSL
	if filter.TextSearchQuery != nil {
		return common.NewSystemError("ERR_QUERY_TEXT_SEARCH_NOT_IMPLEMENTED", "text search is an advanced feature and not implemented in this version of the helper").WithOperation("validateQueryFilter")
	}

	return nil
}

// validateFilterValue validates the structure of a FilterValue.
func (h *QueryHelper) validateFilterValue(fv *FilterValue) error {
	setFields := 0
	if fv.StringVal != nil {
		setFields++
	}
	if fv.NumberVal != nil {
		setFields++
	}
	if fv.BoolVal != nil {
		setFields++
	}
	if fv.ObjectVal != nil {
		setFields++
	}
	if fv.ArrayVal != nil {
		setFields++
		for i, val := range fv.ArrayVal {
			if err := h.validateFilterValue(&val); err != nil {
				return common.NewSystemError("ERR_QUERY_INVALID_ARRAY_VALUE_AT_INDEX", fmt.Sprintf("invalid array value at index %d", i)).WithOperation("validateFilterValue").WithCause(err)
			}
		}
	}
	if fv.FieldRefVal != nil {
		setFields++
		if fv.FieldRefVal.Type != "field" {
			return common.NewSystemError("ERR_QUERY_FIELD_REF_TYPE_INVALID", "field reference type must be 'field'")
		}
		if fv.FieldRefVal.Field == "" {
			return common.NewSystemError("ERR_QUERY_FIELD_REF_FIELD_EMPTY", "field reference field cannot be empty")
		}
	}
	if fv.SubqueryVal != nil {
		setFields++
		if fv.SubqueryVal.Type != "subquery" {
			return common.NewSystemError("ERR_QUERY_SUBQUERY_VALUE_TYPE_INVALID", "subquery value type must be 'subquery'")
		}
		// Subqueries are not supported by this helper for evaluation in filters
		return common.NewSystemError("ERR_QUERY_SUBQUERIES_NOT_SUPPORTED", "subqueries are not supported by this in-memory query helper")
	}
	if fv.FunctionCallVal != nil {
		setFields++
		// Function calls in FilterValue are partially supported (evaluates to nil for now)
		// but structure can be validated.
		if fv.FunctionCallVal.Function == "" {
			return common.NewSystemError("ERR_QUERY_FUNCTION_CALL_NAME_EMPTY", "function call function name cannot be empty")
		}
		for i, arg := range fv.FunctionCallVal.Arguments {
			if err := h.validateFilterValue(&arg); err != nil {
				return common.NewSystemError(ErrInvalidFunctionArgument.Code, fmt.Sprintf("%s at index %d", ErrInvalidFunctionArgument.Error(), i)).WithOperation("validateFilterValue").WithCause(err)
			}
		}
	}

	if setFields > 1 {
		return common.NewSystemError("ERR_QUERY_FILTER_VALUE_MULTIPLE_TYPES", "FilterValue can only have one type of value set").WithOperation("validateFilterValue")
	}
	return nil
}

// validateProjectionConfiguration validates the projection settings.
func (h *QueryHelper) validateProjectionConfiguration(proj *ProjectionConfiguration) error {
	if proj == nil {
		return common.NewSystemError("ERR_QUERY_PROJECTION_CONFIG_NIL", "projection configuration cannot be nil").WithOperation("validateProjectionConfiguration")
	}

	if len(proj.Include) > 0 && len(proj.Exclude) > 0 {
		return common.NewSystemError("ERR_QUERY_PROJECTION_INCLUDE_EXCLUDE_CONFLICT", "cannot use both include and exclude in the same projection").WithOperation("validateProjectionConfiguration")
	}

	for _, field := range proj.Include {
		if field.Name == "" {
			return common.NewSystemError("ERR_QUERY_PROJECTION_INCLUDE_FIELD_EMPTY", "projection include field name cannot be empty")
		}
		if field.Nested != nil {
			if err := h.validateProjectionConfiguration(field.Nested); err != nil {
				return common.NewSystemError("ERR_QUERY_INVALID_NESTED_PROJECTION_INCLUDE", fmt.Sprintf("invalid nested projection for field '%s'", field.Name)).WithOperation("validateProjectionConfiguration").WithCause(err)
			}
		}
	}

	for _, field := range proj.Exclude {
		if field.Name == "" {
			return common.NewSystemError("ERR_QUERY_PROJECTION_EXCLUDE_FIELD_EMPTY", "projection exclude field name cannot be empty")
		}
		if field.Nested != nil {
			// For exclude, nested means exclude nested fields. It's conceptually simpler than include.
			if err := h.validateProjectionConfiguration(field.Nested); err != nil {
				return common.NewSystemError("ERR_QUERY_INVALID_NESTED_PROJECTION_EXCLUDE", fmt.Sprintf("invalid nested projection for field '%s'", field.Name)).WithOperation("validateProjectionConfiguration").WithCause(err)
			}
		}
	}

	for _, computed := range proj.Computed {
		setFields := 0
		if computed.ComputedFieldExpression != nil {
			setFields++
			if computed.ComputedFieldExpression.Type != "computed_field" {
				return common.NewSystemError("ERR_QUERY_COMPUTED_FIELD_TYPE_INVALID", "computed field expression type must be 'computed_field'")
			}
			if computed.ComputedFieldExpression.Expression == nil {
				return common.NewSystemError("ERR_QUERY_COMPUTED_FIELD_EXPRESSION_NIL", "computed field expression cannot be nil")
			}
			if computed.ComputedFieldExpression.Alias == "" {
				return common.NewSystemError("ERR_QUERY_COMPUTED_FIELD_ALIAS_EMPTY", "computed field alias cannot be empty")
			}
			// Validate arguments of the function call within computed field
			if err := h.validateFunctionCall(computed.ComputedFieldExpression.Expression); err != nil {
				return common.NewSystemError("ERR_QUERY_INVALID_FUNCTION_CALL_COMPUTED_FIELD", fmt.Sprintf("invalid function call in computed field '%s'", computed.ComputedFieldExpression.Alias)).WithOperation("validateProjectionConfiguration").WithCause(err)
			}
		}
		if computed.CaseExpression != nil {
			setFields++
			if computed.CaseExpression.Type != "case_expression" {
				return common.NewSystemError("ERR_QUERY_CASE_EXPRESSION_TYPE_INVALID", "case expression type must be 'case_expression'")
			}
			if len(computed.CaseExpression.Conditions) == 0 {
				return common.NewSystemError("ERR_QUERY_CASE_EXPRESSION_NO_CONDITIONS", "case expression must have at least one condition")
			}
			if computed.CaseExpression.Alias == "" {
				return common.NewSystemError("ERR_QUERY_CASE_EXPRESSION_ALIAS_EMPTY", "case expression alias cannot be empty")
			}
			for i, cond := range computed.CaseExpression.Conditions {
				if err := h.validateQueryFilter(&cond.When); err != nil {
					return common.NewSystemError("ERR_QUERY_INVALID_CASE_WHEN_CONDITION", fmt.Sprintf("invalid 'when' condition in case expression at index %d", i)).WithOperation("validateProjectionConfiguration").WithCause(err)
				}
				if err := h.validateFilterValue(&cond.Then); err != nil {
					return common.NewSystemError("ERR_QUERY_INVALID_CASE_THEN_VALUE", fmt.Sprintf("invalid 'then' value in case expression at index %d", i)).WithOperation("validateProjectionConfiguration").WithCause(err)
				}
			}
			if err := h.validateFilterValue(&computed.CaseExpression.Else); err != nil {
				return common.NewSystemError("ERR_QUERY_INVALID_CASE_ELSE_VALUE", "invalid 'else' value in case expression").WithOperation("validateProjectionConfiguration").WithCause(err)
			}
		}
		if setFields != 1 {
			return common.NewSystemError("ERR_QUERY_PROJECTION_COMPUTED_ITEM_CONFLICT", "ProjectionComputedItem must have exactly one of ComputedFieldExpression or CaseExpression set")
		}
	}
	return nil
}

// validateFunctionCall validates a FunctionCall structure.
func (h *QueryHelper) validateFunctionCall(fc *FunctionCall) error {
	if fc.Function == "" {
		return common.NewSystemError("ERR_QUERY_FUNCTION_CALL_NAME_EMPTY", "function call name cannot be empty").WithOperation("validateFunctionCall")
	}
	for i, arg := range fc.Arguments {
		if err := h.validateFilterValue(&arg); err != nil {
			return common.NewSystemError("ERR_QUERY_INVALID_FUNCTION_CALL_ARGUMENT", fmt.Sprintf("invalid argument %d for function '%s'", i, fc.Function)).WithOperation("validateFunctionCall").WithCause(err)
		}
	}
	return nil
}

// validateDistinctConfiguration validates the distinct settings.
func (h *QueryHelper) validateDistinctConfiguration(distinct *QueryDistinctConfig) error {
	if distinct == nil {
		return common.NewSystemError("ERR_QUERY_DISTINCT_CONFIG_NIL", "distinct configuration cannot be nil").WithOperation("validateDistinctConfiguration")
	}
	setFields := 0
	if distinct.IsDistinct != nil {
		setFields++
	}
	if distinct.Fields != nil {
		setFields++
	}

	if setFields == 0 {
		return common.NewSystemError("ERR_QUERY_DISTINCT_CONFIG_MISSING_FIELDS", "distinct configuration must specify 'is_distinct' or 'fields'").WithOperation("validateDistinctConfiguration")
	}
	if setFields > 1 {
		return common.NewSystemError("ERR_QUERY_DISTINCT_CONFIG_CONFLICT", "distinct configuration cannot specify both 'is_distinct' and 'fields'").WithOperation("validateDistinctConfiguration")
	}
	if distinct.Fields != nil {
		if slices.Contains(distinct.Fields, "") {
			return common.NewSystemError("ERR_QUERY_DISTINCT_FIELD_NAME_EMPTY", "distinct field name cannot be empty").WithOperation("validateDistinctConfiguration")
		}
	}
	return nil
}

// validateAggregationConfiguration validates the aggregation settings.
func (h *QueryHelper) validateAggregationConfiguration(aggregations []AggregationConfiguration) error {
	if len(aggregations) == 0 {
		return common.NewSystemError("ERR_QUERY_AGGREGATION_CONFIG_EMPTY", "aggregation configuration cannot be empty if specified").WithOperation("validateAggregationConfiguration")
	}

	for _, agg := range aggregations {
		if agg.Field == "" && agg.Type != AggregationTypeCount { // Count can be on no specific field
			return common.NewSystemError("ERR_QUERY_AGGREGATION_FIELD_EMPTY", "aggregation field cannot be empty for non-count aggregations").WithOperation("validateAggregationConfiguration")
		}

		// Check if the aggregation type is supported
		if h.aggregates != nil {
			if _, ok := (*h.aggregates)[agg.Type]; !ok {
				return common.NewSystemError(ErrUnsupportedAggregationType.Code, fmt.Sprintf("unsupported aggregation type: %s", agg.Type)).WithOperation("validateAggregationConfiguration").WithCause(ErrUnsupportedAggregationType)
			}
		}

		if agg.Filter != nil {
			if err := h.validateQueryFilter(agg.Filter); err != nil {
				return common.NewSystemError("ERR_QUERY_INVALID_AGGREGATION_FILTER", fmt.Sprintf("invalid filter for aggregation on field '%s'", agg.Field)).WithOperation("validateAggregationConfiguration").WithCause(err)
			}
		}

		if slices.Contains(agg.Groups, "") {
			return common.NewSystemError("ERR_QUERY_AGGREGATION_GROUP_FIELD_EMPTY", "aggregation group field cannot be empty").WithOperation("validateAggregationConfiguration")
		}
	}
	return nil
}

// (Rest of the validateQueryFilter, validateFilterValue, validateProjectionConfiguration,
// validateFunctionCall, validateDistinctConfiguration remain unchanged)

// Match evaluates a single record against the provided filters.
// If no filters are provided, it returns true.
func (h *QueryHelper) Match(record map[string]any, filters ...*QueryFilter) (bool, error) {
	// If specific filters are provided, use them. Otherwise, use the helper's default query filters.
	var filtersToApply *QueryFilter
	if len(filters) > 0 && filters[0] != nil {
		filtersToApply = filters[0]
	} else {
		filtersToApply = h.query.Filters
	}

	if filtersToApply == nil {
		return true, nil
	}

	return h.evaluateQueryFilter(record, filtersToApply)
}

// Filter applies the provided filters to a collection of records.
// Returns a new slice containing only the records that match the filters.
// If no filters are provided, the helper's default query filters are used.
func (h *QueryHelper) Filter(records []map[string]any, filters ...*QueryFilter) ([]map[string]any, error) {
	// If specific filters are provided, use them. Otherwise, use the helper's default query filters.
	var filtersToApply *QueryFilter
	if len(filters) > 0 && filters[0] != nil {
		filtersToApply = filters[0]
	} else {
		filtersToApply = h.query.Filters
	}

	if filtersToApply == nil {
		return records, nil
	}

	var filtered []map[string]any
	for _, record := range records {
		matches, err := h.Match(record, filtersToApply) // Pass the resolved filters
		if err != nil {
			return nil, err
		}
		if matches {
			filtered = append(filtered, record)
		}
	}

	return filtered, nil
}

// ApplyDistinct applies the distinct configuration to a collection of records.
// Returns a new slice with only distinct records.
func (h *QueryHelper) ApplyDistinct(records []map[string]any) ([]map[string]any, error) {
	if h.query.Distinct == nil {
		return records, nil
	}

	if h.query.Distinct.IsDistinct != nil && *h.query.Distinct.IsDistinct {
		// Distinct all records
		seen := make(map[string]struct{})
		var distinctRecords []map[string]any
		for _, record := range records {
			// Convert record to a string representation for map key
			// This is a simplistic approach and might not work for complex nested objects
			recordBytes, err := json.Marshal(record)
			if err != nil {
				return nil, common.NewSystemError(ErrFailedToMarshalRecordForDistinct.Code, ErrFailedToMarshalRecordForDistinct.Error()).WithOperation("ApplyDistinct").WithCause(err)
			}
			recordStr := string(recordBytes)
			if _, ok := seen[recordStr]; !ok {
				seen[recordStr] = struct{}{}
				distinctRecords = append(distinctRecords, record)
			}
		}
		return distinctRecords, nil
	} else if h.query.Distinct.Fields != nil {
		// Distinct by specific fields
		type distinctKey []any
		seen := make(map[string]struct{})
		var distinctRecords []map[string]any

		for _, record := range records {
			keyValues := make(distinctKey, len(h.query.Distinct.Fields))
			for i, field := range h.query.Distinct.Fields {
				keyValues[i], _ = utils.GetValueByPath(record, field)
			}
			// Create a string representation of the key values for map lookup
			keyBytes, err := json.Marshal(keyValues)
			if err != nil {
				return nil, common.NewSystemError(ErrFailedToMarshalDistinctKey.Code, ErrFailedToMarshalDistinctKey.Error()).WithOperation("ApplyDistinct").WithCause(err)
			}
			keyStr := string(keyBytes)
			if _, ok := seen[keyStr]; !ok {
				seen[keyStr] = struct{}{}
				distinctRecords = append(distinctRecords, record)
			}
		}
		return distinctRecords, nil
	}
	return records, nil
}

// Sort applies the sorting configuration to a collection of records.
// Returns a new slice with records sorted according to the configuration.
func (h *QueryHelper) Sort(records []map[string]any) ([]map[string]any, error) {
	if len(h.query.Sort) == 0 {
		return records, nil
	}

	// Create a copy to avoid modifying the original slice
	sorted := make([]map[string]any, len(records))
	copy(sorted, records)

	sort.Slice(sorted, func(i, j int) bool {
		for _, sortConfig := range h.query.Sort {
			valueI, _ := utils.GetValueByPath(sorted[i], sortConfig.Field)
			valueJ, _ := utils.GetValueByPath(sorted[j], sortConfig.Field)

			comparison := h.compareValues(valueI, valueJ)
			if comparison == 0 {
				continue // Values are equal, check next sort field
			}

			if sortConfig.Direction == SortDirectionAsc {
				return comparison < 0
			}
			return comparison > 0
		}
		return false // All sort fields are equal
	})

	return sorted, nil
}

// Paginate applies pagination to a collection of records.
// Returns a new slice with the paginated records and pagination result information.
func (h *QueryHelper) Paginate(records []map[string]any) ([]map[string]any, *PaginationResult, error) {
	if h.query.Pagination == nil {
		return records, nil, nil
	}

	if len(h.query.Pagination.Type) == 0 {
		return records, nil, nil
	}

	pagination := h.query.Pagination
	totalCount := len(records)

	switch pagination.Type {
	case "offset":
		offset := 0
		if pagination.Offset != nil {
			offset = *pagination.Offset
		}

		if offset >= totalCount {
			return []map[string]any{}, &PaginationResult{
				Total: &totalCount,
			}, nil
		}

		end := min(offset+pagination.Limit, totalCount)

		return records[offset:end], &PaginationResult{
			Total: &totalCount,
		}, nil

	default:
		return nil, nil, common.NewSystemError(ErrInvalidPaginationType.Code, fmt.Sprintf("unsupported pagination type: %s", pagination.Type)).WithOperation("Paginate").WithCause(ErrInvalidPaginationType)
	}
}

// Project applies field projection to a collection of records.
// Returns a new slice with records containing only the projected fields.
func (h *QueryHelper) Project(records []map[string]any) ([]map[string]any, error) {
	if h.query.Projection == nil {
		return records, nil
	}

	projected := make([]map[string]any, len(records))
	for i, record := range records {
		projectedRecord, err := h.projectRecord(record, h.query.Projection)
		if err != nil {
			return nil, err
		}
		projected[i] = projectedRecord
	}

	return projected, nil
}

// Project applies field projection to a collection of records.
// Returns a new slice with records containing only the projected fields.
func (h *QueryHelper) ProjectSingle(record map[string]any) (map[string]any, error) {
	if h.query.Projection == nil {
		return record, nil
	}
	return h.projectRecord(record, h.query.Projection)
}

// projectRecord applies projection to a single record, including nested projections.
func (h *QueryHelper) projectRecord(record map[string]any, projectionConfig *ProjectionConfiguration) (map[string]any, error) {
	result := make(map[string]any)

	// If no include fields specified, start with all fields (before exclusions)
	if len(projectionConfig.Include) == 0 {
		maps.Copy(result, record)
	} else {
		// Include only specified fields and handle nested projections
		for _, field := range projectionConfig.Include {
			parts := strings.Split(field.Name, ".")
			currentRecord := record
			currentResult := result
			for i, part := range parts {
				value, exists := currentRecord[part]
				if !exists {
					break // Field not found, skip
				}

				if i == len(parts)-1 {
					// Last part of the path, apply nested projection if it exists
					if field.Nested != nil {
						nestedMap, ok := value.(map[string]any)
						if !ok {
							nestedMap, ok = value.(map[string]any)
						}
						if ok {
							nestedResult, err := h.projectRecord(nestedMap, field.Nested)
							if err != nil {
								return nil, err
							}
							currentResult[part] = nestedResult
						} else {
							// If nested projection is specified but value is not a map, include as is or error
							currentResult[part] = value
						}
					} else {
						currentResult[part] = value
					}
				} else {
					nestedMap, ok := value.(map[string]any)
					if !ok {
						nestedMap, ok = value.(map[string]any)
					}
					// Not the last part, traverse into nested map
					if ok {
						_, isDoc := currentResult[part].(map[string]any)
						if _, ok := currentResult[part].(map[string]any); !ok && !isDoc {
							currentResult[part] = make(map[string]any)
						}
						currentRecord = nestedMap
						currentResult = currentResult[part].(map[string]any)
					} else {
						// Path indicates nested, but value is not a map, so cannot traverse.
						break
					}
				}
			}
		}
	}

	// Remove excluded fields (after includes are processed)
	// This exclusion logic also handles nested exclusion
	for _, field := range projectionConfig.Exclude {
		parts := strings.Split(field.Name, ".")
		currentResult := result // Start from the root of the result
		for i, part := range parts {
			if i == len(parts)-1 {
				// Last part of the path, delete the field
				if field.Nested != nil {
					// If nested exclude is defined, recursively exclude within the target field
					targetMap, ok := currentResult[part].(map[string]any)
					if !ok {
						targetMap, ok = currentResult[part].(map[string]any)
					}
					if ok {
						nestedResult, err := h.projectRecord(targetMap, &ProjectionConfiguration{Exclude: field.Nested.Exclude})
						if err != nil {
							return nil, err
						}
						currentResult[part] = nestedResult
					}
				} else {
					delete(currentResult, part)
				}
			} else {
				// Not the last part, traverse into nested map in the result
				nestedMap, ok := currentResult[part].(map[string]any)
				if !ok {
					nestedMap, ok = currentResult[part].(map[string]any)
				}
				if ok {
					currentResult = nestedMap
				} else {
					// Path indicates nested, but value is not a map in the result, so nothing to exclude further.
					break
				}
			}
		}
	}

	// Add computed fields
	for _, computed := range projectionConfig.Computed {
		if computed.ComputedFieldExpression != nil {
			value, err := h.evaluateComputedField(record, computed.ComputedFieldExpression)
			if err != nil {
				return nil, err
			}
			result[computed.ComputedFieldExpression.Alias] = value
		}
		if computed.CaseExpression != nil {
			value, err := h.evaluateCaseExpression(record, computed.CaseExpression)
			if err != nil {
				return nil, err
			}
			result[computed.CaseExpression.Alias] = value
		}
	}

	return result, nil
}

// ApplyAggregations applies the aggregation configurations to a collection of records.
// Returns a map where keys are aggregation aliases and values are the aggregated results.
func (h *QueryHelper) ApplyAggregations(records []map[string]any) (map[string]any, error) {
	if h.query.Aggregations == nil {
		return nil, nil // No aggregations defined
	}

	results := make(map[string]any)

	for _, aggConfig := range h.query.Aggregations {
		// Step 1: Apply filtering for this specific aggregation
		currentRecords := records
		if aggConfig.Filter != nil {
			var err error
			// Use the modified Filter method to apply the specific aggregation filter
			filteredRecords, err := h.Filter(currentRecords, aggConfig.Filter)
			if err != nil {
				return nil, common.NewSystemError("ERR_QUERY_AGGREGATION_FILTER_FAILED", fmt.Sprintf("error applying filter for aggregation '%s'", aggConfig.AliasOrDefault())).WithOperation("ApplyAggregations").WithCause(err)
			}
			currentRecords = filteredRecords
		}

		// Resolve the aggregation function
		aggFunc, ok := (*h.aggregates)[aggConfig.Type]
		if !ok {
			return nil, common.NewSystemError(ErrUnsupportedAggregationType.Code, fmt.Sprintf("unsupported aggregation type: %s", aggConfig.Type)).WithOperation("ApplyAggregations").WithCause(ErrUnsupportedAggregationType)
		}

		// Step 2: Handle Grouping
		if len(aggConfig.Groups) > 0 {
			groupedResults, err := h.processGroupedAggregation(currentRecords, aggConfig, aggFunc)
			if err != nil {
				return nil, err
			}
			results[aggConfig.AliasOrDefault()] = groupedResults
		} else {
			// Step 3: Perform simple (non-grouped) aggregation
			aggregatedValue, err := aggFunc(currentRecords, aggConfig.Field)
			if err != nil {
				return nil, common.NewSystemError("ERR_QUERY_AGGREGATION_PERFORMANCE_FAILED", fmt.Sprintf("error performing aggregation for field '%s'", aggConfig.Field)).WithOperation("ApplyAggregations").WithCause(err)
			}
			results[aggConfig.AliasOrDefault()] = aggregatedValue
		}
	}

	return results, nil
}

// processGroupedAggregation handles aggregations with a 'groups' clause.
func (h *QueryHelper) processGroupedAggregation(records []map[string]any, aggConfig AggregationConfiguration, aggFunc AggregateFunction) ([]map[string]any, error) {
	groupedData := make(map[string][]map[string]any)
	groupKeyMap := make(map[string]map[string]any) // To store the actual group key values for output

	for _, record := range records {
		// Create a composite key for grouping
		groupKeyParts := make([]string, len(aggConfig.Groups))
		currentGroupKeyValues := make(map[string]any)

		for i, groupField := range aggConfig.Groups {
			doc := map[string]any(record)
			val, _ := utils.GetValueByPath(doc, groupField)
			groupKeyParts[i] = fmt.Sprintf("%v", val) // Convert to string for map key
			currentGroupKeyValues[groupField] = val   // Store actual values for later
		}
		groupKey := strings.Join(groupKeyParts, "|") // Delimiter for composite key

		groupedData[groupKey] = append(groupedData[groupKey], record)
		if _, ok := groupKeyMap[groupKey]; !ok {
			groupKeyMap[groupKey] = currentGroupKeyValues // Store group field values once per group
		}
	}

	var finalGroupedResults []map[string]any

	for groupKey, groupRecords := range groupedData {
		aggregatedValue, err := aggFunc(groupRecords, aggConfig.Field)
		if err != nil {
			return nil, common.NewSystemError("ERR_QUERY_GROUPED_AGGREGATION_FAILED", fmt.Sprintf("error performing grouped aggregation for key '%s', field '%s'", groupKey, aggConfig.Field)).WithOperation("processGroupedAggregation").WithCause(err)
		}

		// Construct the result object for this group
		groupResult := make(map[string]any)
		// Add the group by fields
		maps.Copy(groupResult, groupKeyMap[groupKey])
		// Add the aggregated value with its alias
		groupResult[aggConfig.AliasOrDefault()] = aggregatedValue
		finalGroupedResults = append(finalGroupedResults, groupResult)
	}

	return finalGroupedResults, nil
}

// AliasOrDefault returns the alias if set, otherwise generates a default alias.
func (ac *AggregationConfiguration) AliasOrDefault() string {
	if ac.Alias != nil && *ac.Alias != "" {
		return *ac.Alias
	}
	// For count without a field, use "count" as default alias.
	if ac.Type == AggregationTypeCount && ac.Field == "" {
		return "count"
	}
	return string(ac.Type) + "_" + ac.Field // Example default alias
}

// evaluateQueryFilter evaluates a QueryFilter against a record.
func (h *QueryHelper) evaluateQueryFilter(record map[string]any, filter *QueryFilter) (bool, error) {
	if filter.Condition != nil {
		return h.evaluateCondition(record, filter.Condition)
	}

	if filter.Group != nil {
		return h.evaluateGroup(record, filter.Group)
	}

	if filter.TextSearchQuery != nil {
		// Text search is not implemented for this helper
		return false, common.NewSystemError("ERR_QUERY_TEXT_SEARCH_NOT_IMPLEMENTED_EVAL", "text search is not implemented for this in-memory helper").WithOperation("evaluateQueryFilter")
	}

	return false, common.NewSystemError("ERR_QUERY_INVALID_FILTER_NO_CONDITION_GROUP_TEXT", "invalid filter: no condition, group, or text match specified").WithOperation("evaluateQueryFilter")
}

// evaluateCondition evaluates a FilterCondition against a record.
func (h *QueryHelper) evaluateCondition(record map[string]any, condition *FilterCondition) (bool, error) {
	// Use rich filter function if available
	if fn, ok := h.goFilterFunctions[condition.Operator]; ok {
		return fn(record, condition.Field, condition.Value)
	}

	doc := map[string]any(record)
	fieldValue, _ := utils.GetValueByPath(doc, condition.Field)
	conditionVal, err := h.resolveFilterValue(record, &condition.Value)
	if err != nil {
		return false, err
	}

	// Try to use custom operator first
	if h.operators != nil {
		if customPredicate, ok := (*h.operators)[string(condition.Operator)]; ok {
			return customPredicate(fieldValue, conditionVal)
		}
	}

	// Fallback to standard operators if no custom operator was found
	switch condition.Operator {
	case ComparisonOperatorEq:
		return h.compareValues(fieldValue, conditionVal) == 0, nil
	case ComparisonOperatorNeq:
		return h.compareValues(fieldValue, conditionVal) != 0, nil
	case ComparisonOperatorLt:
		return h.compareValues(fieldValue, conditionVal) < 0, nil
	case ComparisonOperatorLte:
		return h.compareValues(fieldValue, conditionVal) <= 0, nil
	case ComparisonOperatorGt:
		return h.compareValues(fieldValue, conditionVal) > 0, nil
	case ComparisonOperatorGte:
		return h.compareValues(fieldValue, conditionVal) >= 0, nil
	case ComparisonOperatorIn:
		return h.evaluateInOperator(fieldValue, conditionVal)
	case ComparisonOperatorNin:
		result, err := h.evaluateInOperator(fieldValue, conditionVal)
		return !result, err
	case ComparisonOperatorContains:
		return h.evaluateContains(fieldValue, conditionVal)
	case ComparisonOperatorNotContains:
		result, err := h.evaluateContains(fieldValue, conditionVal)
		return !result, err
	case ComparisonOperatorExists:
		return fieldValue != nil, nil
	case ComparisonOperatorNotExists:
		return fieldValue == nil, nil
	default:
		return false, common.NewSystemError(ErrUnknownComparisonOperator.Code, fmt.Sprintf("unsupported operator: %s", condition.Operator)).WithOperation("evaluateCondition").WithCause(ErrUnknownComparisonOperator)
	}
}

// resolveFilterValue extracts the actual value from a FilterValue union type.
// It also handles FieldReference and FunctionCall within a FilterValue.
func (h *QueryHelper) resolveFilterValue(record map[string]any, fv *FilterValue) (any, error) {
	if fv.StringVal != nil {
		return *fv.StringVal, nil
	}

	if fv.NumberVal != nil {
		return *fv.NumberVal, nil
	}

	if fv.BoolVal != nil {
		return *fv.BoolVal, nil
	}

	if fv.ObjectVal != nil {
		return fv.ObjectVal, nil
	}

	if fv.ArrayVal != nil {
		resolvedArray := make([]any, len(fv.ArrayVal))
		for i, val := range fv.ArrayVal {
			resolved, err := h.resolveFilterValue(record, &val)
			if err != nil {
				return nil, err
			}
			resolvedArray[i] = resolved
		}
		return resolvedArray, nil
	}

	if fv.FieldRefVal != nil {
		doc := map[string]any(record)
		result, _ := utils.GetValueByPath(doc, fv.FieldRefVal.Field)
		return result, nil
	}

	if fv.SubqueryVal != nil {
		// Subqueries are not supported by this helper for evaluation in filters
		return nil, common.NewSystemError("ERR_QUERY_SUBQUERIES_NOT_SUPPORTED_RESOLVE", "subqueries are not supported by this in-memory query helper").WithOperation("resolveFilterValue")
	}

	if fv.FunctionCallVal != nil {
		return h.evaluateFunctionCall(record, fv.FunctionCallVal)
	}

	return nil, nil // Represents a nil or unspecified value
}

// evaluateGroup evaluates a FilterGroup against a record.
func (h *QueryHelper) evaluateGroup(record map[string]any, group *FilterGroup) (bool, error) {
	results := make([]bool, len(group.Conditions))
	for i, condition := range group.Conditions {
		result, err := h.evaluateQueryFilter(record, &condition)
		if err != nil {
			return false, common.NewSystemError("ERR_QUERY_EVALUATING_GROUP_CONDITION", fmt.Sprintf("evaluating condition %d", i)).WithOperation("evaluateGroup").WithCause(err)
		}
		results[i] = result
	}
	return group.Operator.Evaluate(results)
}

// evaluateInOperator evaluates the IN operator.
func (h *QueryHelper) evaluateInOperator(fieldValue, conditionValue any) (bool, error) {
	conditionSlice := reflect.ValueOf(conditionValue)
	if conditionSlice.Kind() != reflect.Slice && conditionSlice.Kind() != reflect.Array {
		return false, common.NewSystemError("ERR_QUERY_IN_OPERATOR_REQUIRES_SLICE_OR_ARRAY", "IN operator requires a slice or array value as the comparison target").WithOperation("evaluateInOperator")
	}

	for i := 0; i < conditionSlice.Len(); i++ {
		item := conditionSlice.Index(i).Interface()
		if h.compareValues(fieldValue, item) == 0 {
			return true, nil
		}
	}
	return false, nil
}

// evaluateContains evaluates the CONTAINS operator.
func (h *QueryHelper) evaluateContains(fieldValue, conditionValue any) (bool, error) {
	fieldStr, ok := fieldValue.(string)
	if !ok {
		return false, nil
	}
	conditionStr, ok := conditionValue.(string)
	if !ok {
		return false, common.NewSystemError("ERR_QUERY_CONTAINS_OPERATOR_REQUIRES_STRING", "CONTAINS operator requires string values for comparison target").WithOperation("evaluateContains")
	}
	return strings.Contains(fieldStr, conditionStr), nil
}

// evaluateComputedField evaluates a computed field expression.
func (h *QueryHelper) evaluateComputedField(record map[string]any, expr *ComputedFieldExpression) (any, error) {
	if expr.Expression == nil {
		return nil, common.NewSystemError("ERR_QUERY_COMPUTED_FIELD_EXPRESSION_NIL_EVAL", "computed field expression cannot be nil").WithOperation("evaluateComputedField")
	}

	return h.evaluateFunctionCall(record, expr.Expression)
}

// evaluateCaseExpression evaluates a case expression.
func (h *QueryHelper) evaluateCaseExpression(record map[string]any, expr *CaseExpression) (any, error) {
	for _, caseCondition := range expr.Conditions {
		result, err := h.evaluateQueryFilter(record, &caseCondition.When)
		if err != nil {
			return nil, err
		}
		if result {
			return h.resolveFilterValue(record, &caseCondition.Then)
		}
	}
	return h.resolveFilterValue(record, &expr.Else)
}

// evaluateFunctionCall evaluates a function call.
func (h *QueryHelper) evaluateFunctionCall(record map[string]any, fc *FunctionCall) (any, error) {
	// Use rich compute function if available
	if fn, ok := h.computeFunctions[fc.Function]; ok {
		return fn(record, fc.Arguments)
	}

	resolvedArgs := make([]any, len(fc.Arguments))
	for i, arg := range fc.Arguments {
		val, err := h.resolveFilterValue(record, &arg)
		if err != nil {
			return nil, err
		}
		resolvedArgs[i] = val
	}

	if h.functions != nil {
		// Look up and execute the registered function
		if fn, ok := (*h.functions)[fc.Function]; ok {
			return fn(resolvedArgs...)
		}
	}

	return nil, common.NewSystemError(ErrFunctionNotImplementedOrRegistered.Code, fmt.Sprintf("function '%s' is not implemented or registered in this helper", fc.Function)).WithOperation("evaluateFunctionCall").WithCause(ErrFunctionNotImplementedOrRegistered)
}

func getFloat(v any) (float64, bool) {
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(val.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(val.Uint()), true
	case reflect.Float32, reflect.Float64:
		return val.Float(), true
	default:
		return 0, false
	}
}

// compareValues compares two values and returns -1, 0, or 1.
func (h *QueryHelper) compareValues(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil || b == nil {
		return -1 // Consider nil less than any non-nil value
	}

	aFloat, aIsNum := getFloat(a)
	bFloat, bIsNum := getFloat(b)

	if aIsNum && bIsNum {
		if aFloat < bFloat {
			return -1
		}
		if aFloat > bFloat {
			return 1
		}
		return 0
	}

	aStr, aIsStr := a.(string)
	bStr, bIsStr := b.(string)

	if aIsStr && bIsStr {
		return strings.Compare(aStr, bStr)
	}
	// Fallback to reflect.DeepEqual for other types like booleans, maps, etc.
	if reflect.DeepEqual(a, b) {
		return 0
	}

	return -1 // Default to not equal
}
func (h *QueryHelper) combineDocs(leftName string, leftDoc map[string]any, rightName string, rightDoc map[string]any) map[string]any {
	combinedDoc := make(map[string]any)

	// Always create a consistent nested structure
	combinedDoc[leftName] = leftDoc

	if rightDoc != nil {
		combinedDoc[rightName] = rightDoc
	} else {
		combinedDoc[rightName] = nil
	}

	return combinedDoc
}

// Join performs a join operation between left and right document collections.
// The target parameter specifies the logical name and alias for the right-side collection.
// The config parameter defines the join type, conditions, and optional projections.
func (h *QueryHelper) Join(left, right []map[string]any, config *JoinConfiguration) ([]map[string]any, error) {
	// Validate inputs
	if config == nil {
		return nil, common.NewSystemError("ERR_QUERY_JOIN_CONFIG_NIL", "join configuration cannot be nil").WithOperation("Join")
	}

	// Determine the right-side collection name to use in the combined document
	rightName := config.Target.Name
	if config.Target.Alias != nil && *config.Target.Alias != "" {
		rightName = *config.Target.Alias
	}

	// Determine left-side collection name
	leftName := h.getLeftCollectionName()

	var result []map[string]any
	var joinErr error

	switch config.Type {
	case JoinTypeInner:
		result, joinErr = h.performInnerJoin(leftName, left, rightName, right, config.On)
	case JoinTypeLeft:
		result, joinErr = h.performLeftJoin(leftName, left, rightName, right, config.On)
	case JoinTypeRight:
		result, joinErr = h.performRightJoin(leftName, left, rightName, right, config.On)
	case JoinTypeFull:
		result, joinErr = h.performFullJoin(leftName, left, rightName, right, config.On)
	default:
		return nil, common.NewSystemError(ErrUnsupportedJoinType.Code, fmt.Sprintf("unsupported join type: %s", config.Type)).WithOperation("Join").WithCause(ErrUnsupportedJoinType)
	}

	if joinErr != nil {
		return nil, joinErr
	}

	// Apply projection if specified in the join configuration
	if config.Projection != nil {
		var projectedResult []map[string]any
		for _, doc := range result {
			projected, err := h.projectRecord(doc, config.Projection)
			if err != nil {
				return nil, common.NewSystemError("ERR_QUERY_JOIN_PROJECTION_FAILED", "projection error during join").WithOperation("Join").WithCause(err)
			}
			projectedResult = append(projectedResult, projected)
		}
		result = projectedResult
	}

	return result, nil
}

// JoinStreams performs streaming joins between left and right document channels.
// The target parameter specifies the logical name and alias for the right-side collection.
// Returns channels for the joined results and any errors that occur during processing.
func (h *QueryHelper) JoinStreams(left, right <-chan map[string]any, target *QueryTarget, config *JoinConfiguration) (<-chan map[string]any, <-chan error) {
	resultChan := make(chan map[string]any)
	errorChan := make(chan error, 1)

	go func() {
		defer close(resultChan)
		defer close(errorChan)

		// Validate inputs
		if config == nil {
			errorChan <- common.NewSystemError("ERR_QUERY_JOIN_CONFIG_NIL", "join configuration cannot be nil").WithOperation("JoinStreams")
			return
		}

		if target == nil {
			errorChan <- common.NewSystemError("ERR_QUERY_TARGET_CANNOT_BE_NIL", "target cannot be nil").WithOperation("JoinStreams")
			return
		}

		if target.Name == "" {
			errorChan <- common.NewSystemError("ERR_QUERY_TARGET_NAME_EMPTY_STREAM", "target name cannot be empty").WithOperation("JoinStreams")
			return
		}

		if config.On == nil {
			errorChan <- common.NewSystemError("ERR_QUERY_JOIN_CONDITION_NIL", "join condition ('on') cannot be nil").WithOperation("JoinStreams")
			return
		}

		// Determine the right-side collection name to use in the combined document
		rightName := target.Name
		if target.Alias != nil && *target.Alias != "" {
			rightName = *target.Alias
		}

		// Determine left-side collection name
		leftName := h.getLeftCollectionName()

		// For streaming joins, we need to handle different join types
		switch config.Type {
		case JoinTypeInner:
			h.performStreamingInnerJoin(leftName, left, rightName, right, config, resultChan, errorChan)
		case JoinTypeLeft:
			h.performStreamingLeftJoin(leftName, left, rightName, right, config, resultChan, errorChan)
		case JoinTypeRight:
			h.performStreamingRightJoin(leftName, left, rightName, right, config, resultChan, errorChan)
		case JoinTypeFull:
			h.performStreamingFullJoin(leftName, left, rightName, right, config, resultChan, errorChan)
		default:
			errorChan <- common.NewSystemError(ErrUnsupportedJoinType.Code, fmt.Sprintf("unsupported join type: %s", config.Type)).WithOperation("JoinStreams").WithCause(ErrUnsupportedJoinType)
			return
		}
	}()

	return resultChan, errorChan
}

// getLeftCollectionName determines the name to use for the left-side collection in joins.
// It uses the query's target information if available, otherwise defaults to a generic name.
func (h *QueryHelper) getLeftCollectionName() string {
	if h.query.Target != nil {
		if h.query.Target.Alias != nil && *h.query.Target.Alias != "" {
			return *h.query.Target.Alias
		}
		if h.query.Target.Name != "" {
			return h.query.Target.Name
		}
	}
	return "left" // Default fallback name
}

// GetTargetInfo returns the target information from the query.
// This is useful for persistence layers that need to map logical to physical collection names.
func (h *QueryHelper) GetTargetInfo() *QueryTarget {
	return h.query.Target
}

// GetTargetName returns the logical name of the target collection.
func (h *QueryHelper) GetTargetName() string {
	if h.query.Target != nil && h.query.Target.Name != "" {
		return h.query.Target.Name
	}
	return ""
}

// GetTargetAlias returns the alias of the target collection, or the name if no alias is set.
func (h *QueryHelper) GetTargetAlias() string {
	if h.query.Target != nil {
		if h.query.Target.Alias != nil && *h.query.Target.Alias != "" {
			return *h.query.Target.Alias
		}
		return h.query.Target.Name
	}
	return ""
}

// performStreamingInnerJoin implements streaming inner join logic.
func (h *QueryHelper) performStreamingInnerJoin(leftName string, left <-chan map[string]any, rightName string, right <-chan map[string]any, config *JoinConfiguration, resultChan chan<- map[string]any, errorChan chan<- error) {
	// For streaming inner joins, we need to buffer one side (typically the smaller one)
	// This is a simplified implementation - in practice, you might want more sophisticated buffering
	var rightDocs []map[string]any

	// Buffer all right documents
	for rightDoc := range right {
		rightDocs = append(rightDocs, rightDoc)
	}

	// Process left documents against buffered right documents
	for leftDoc := range left {
		for _, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)

			matches, err := h.Match(combinedDoc, config.On)
			if err != nil {
				errorChan <- common.NewSystemError("ERR_QUERY_STREAMING_JOIN_CONDITION_EVAL_FAILED", "error evaluating join condition").WithOperation("performStreamingJoin").WithCause(err)
				return
			}

			if matches {
				// Apply projection if specified
				if config.Projection != nil {
					projected, err := h.projectRecord(combinedDoc, config.Projection)
					if err != nil {
						errorChan <- common.NewSystemError("ERR_QUERY_STREAMING_JOIN_PROJECTION", "projection error during streaming join").WithOperation("performStreamingJoin").WithCause(err)
						return
					}
					combinedDoc = projected
				}
				resultChan <- combinedDoc
			}
		}
	}
}

// performStreamingLeftJoin implements streaming left join logic.
func (h *QueryHelper) performStreamingLeftJoin(leftName string, left <-chan map[string]any, rightName string, right <-chan map[string]any, config *JoinConfiguration, resultChan chan<- map[string]any, errorChan chan<- error) {
	// Buffer all right documents
	var rightDocs []map[string]any
	for rightDoc := range right {
		rightDocs = append(rightDocs, rightDoc)
	}

	// Process left documents
	for leftDoc := range left {
		hasMatch := false
		for _, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)

			matches, err := h.Match(combinedDoc, config.On)
			if err != nil {
				errorChan <- common.NewSystemError("ERR_QUERY_STREAMING_JOIN_CONDITION_EVAL_FAILED", "error evaluating join condition").WithOperation("performStreamingJoin").WithCause(err)
				return
			}

			if matches {
				// Apply projection if specified
				if config.Projection != nil {
					projected, err := h.projectRecord(combinedDoc, config.Projection)
					if err != nil {
						errorChan <- common.NewSystemError("ERR_QUERY_STREAMING_JOIN_PROJECTION", "projection error during streaming join").WithOperation("performStreamingJoin").WithCause(err)
						return
					}
					combinedDoc = projected
				}
				resultChan <- combinedDoc
				hasMatch = true
			}
		}

		// If no matches found, include left document with null right side
		if !hasMatch {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, nil)
			if config.Projection != nil {
				projected, err := h.projectRecord(combinedDoc, config.Projection)
				if err != nil {
					errorChan <- common.NewSystemError(ErrStreamingJoinProjection.Code, ErrStreamingJoinProjection.Error()).WithOperation("performStreamingJoin").WithCause(err)
					return
				}
				combinedDoc = projected
			}
			resultChan <- combinedDoc
		}
	}
}

// performStreamingRightJoin implements streaming right join logic.
func (h *QueryHelper) performStreamingRightJoin(leftName string, left <-chan map[string]any, rightName string, right <-chan map[string]any, config *JoinConfiguration, resultChan chan<- map[string]any, errorChan chan<- error) {
	// Buffer all left documents
	var leftDocs []map[string]any
	for leftDoc := range left {
		leftDocs = append(leftDocs, leftDoc)
	}

	// Process right documents
	for rightDoc := range right {
		hasMatch := false
		for _, leftDoc := range leftDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)

			matches, err := h.Match(combinedDoc, config.On)
			if err != nil {
				errorChan <- common.NewSystemError("ERR_QUERY_STREAMING_JOIN_CONDITION_EVAL_FAILED", "error evaluating join condition").WithOperation("performStreamingJoin").WithCause(err)
				return
			}

			if matches {
				// Apply projection if specified
				if config.Projection != nil {
					projected, err := h.projectRecord(combinedDoc, config.Projection)
					if err != nil {
						errorChan <- common.NewSystemError("ERR_QUERY_STREAMING_JOIN_PROJECTION", "projection error during streaming join").WithOperation("performStreamingJoin").WithCause(err)
						return
					}
					combinedDoc = projected
				}
				resultChan <- combinedDoc
				hasMatch = true
			}
		}

		// If no matches found, include right document with null left side
		if !hasMatch {
			combinedDoc := h.combineDocs(leftName, nil, rightName, rightDoc)
			if config.Projection != nil {
				projected, err := h.projectRecord(combinedDoc, config.Projection)
				if err != nil {
					errorChan <- common.NewSystemError(ErrStreamingJoinProjection.Code, ErrStreamingJoinProjection.Error()).WithOperation("performStreamingJoin").WithCause(err)
					return
				}
				combinedDoc = projected
			}
			resultChan <- combinedDoc
		}
	}
}

// performStreamingFullJoin implements streaming full outer join logic.
func (h *QueryHelper) performStreamingFullJoin(leftName string, left <-chan map[string]any, rightName string, right <-chan map[string]any, config *JoinConfiguration, resultChan chan<- map[string]any, errorChan chan<- error) {
	// Buffer both sides for full outer join
	var leftDocs []map[string]any
	var rightDocs []map[string]any

	for leftDoc := range left {
		leftDocs = append(leftDocs, leftDoc)
	}

	for rightDoc := range right {
		rightDocs = append(rightDocs, rightDoc)
	}

	matchedLeftIndices := make(map[int]bool)
	matchedRightIndices := make(map[int]bool)

	// Find all matches
	for leftIndex, leftDoc := range leftDocs {
		for rightIndex, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)

			matches, err := h.Match(combinedDoc, config.On)
			if err != nil {
				errorChan <- common.NewSystemError("ERR_QUERY_STREAMING_JOIN_CONDITION_EVAL_FAILED", "error evaluating join condition").WithOperation("performStreamingJoin").WithCause(err)
				return
			}

			if matches {
				// Apply projection if specified
				if config.Projection != nil {
					projected, err := h.projectRecord(combinedDoc, config.Projection)
					if err != nil {
						errorChan <- common.NewSystemError("ERR_QUERY_STREAMING_JOIN_PROJECTION", "projection error during streaming join").WithOperation("performStreamingJoin").WithCause(err)
						return
					}
					combinedDoc = projected
				}
				resultChan <- combinedDoc
				matchedLeftIndices[leftIndex] = true
				matchedRightIndices[rightIndex] = true
			}
		}
	}

	// Add unmatched left documents
	for leftIndex, leftDoc := range leftDocs {
		if !matchedLeftIndices[leftIndex] {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, nil)
			if config.Projection != nil {
				projected, err := h.projectRecord(combinedDoc, config.Projection)
				if err != nil {
					errorChan <- common.NewSystemError(ErrStreamingJoinProjection.Code, ErrStreamingJoinProjection.Error()).WithOperation("performStreamingJoin").WithCause(err)
					return
				}
				combinedDoc = projected
			}
			resultChan <- combinedDoc
		}
	}

	// Add unmatched right documents
	for rightIndex, rightDoc := range rightDocs {
		if !matchedRightIndices[rightIndex] {
			combinedDoc := h.combineDocs(leftName, nil, rightName, rightDoc)
			if config.Projection != nil {
				projected, err := h.projectRecord(combinedDoc, config.Projection)
				if err != nil {
					errorChan <- common.NewSystemError(ErrStreamingJoinProjection.Code, ErrStreamingJoinProjection.Error()).WithOperation("performStreamingJoin").WithCause(err)
					return
				}
				combinedDoc = projected
			}
			resultChan <- combinedDoc
		}
	}
}

func (h *QueryHelper) performInnerJoin(leftName string, leftDocs []map[string]any, rightName string, rightDocs []map[string]any, condition *QueryFilter) ([]map[string]any, error) {
	var result []map[string]any

	for _, leftDoc := range leftDocs {
		for _, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)

			matches, err := h.Match(combinedDoc, condition)
			if err != nil {
				return nil, common.NewSystemError("ERR_QUERY_JOIN_CONDITION_EVAL_FAILED", "error evaluating join condition for inner join").WithOperation("performInnerJoin").WithCause(err)
			}

			if matches {
				result = append(result, combinedDoc)
			}
		}
	}

	return result, nil
}

func (h *QueryHelper) performLeftJoin(leftName string, leftDocs []map[string]any, rightName string, rightDocs []map[string]any, condition *QueryFilter) ([]map[string]any, error) {
	var result []map[string]any

	for _, leftDoc := range leftDocs {
		hasMatch := false
		for _, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)
			matches, err := h.Match(combinedDoc, condition)
			if err != nil {
				return nil, common.NewSystemError("ERR_QUERY_JOIN_CONDITION_EVAL_FAILED", "error evaluating join condition for left join").WithOperation("performLeftJoin").WithCause(err)
			}

			if matches {
				result = append(result, combinedDoc)
				hasMatch = true
			}
		}

		if !hasMatch {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, nil)
			result = append(result, combinedDoc)
		}
	}

	return result, nil
}

func (h *QueryHelper) performRightJoin(leftName string, leftDocs []map[string]any, rightName string, rightDocs []map[string]any, condition *QueryFilter) ([]map[string]any, error) {
	var result []map[string]any
	matchedRightIndices := make(map[int]bool)

	for _, leftDoc := range leftDocs {
		for rightIndex, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)
			matches, err := h.Match(combinedDoc, condition)
			if err != nil {
				return nil, common.NewSystemError("ERR_QUERY_JOIN_CONDITION_EVAL_FAILED", "error evaluating join condition for right join").WithOperation("performRightJoin").WithCause(err)
			}

			if matches {
				result = append(result, combinedDoc)
				matchedRightIndices[rightIndex] = true
			}
		}
	}

	for rightIndex, rightDoc := range rightDocs {
		if !matchedRightIndices[rightIndex] {
			result = append(result, h.combineDocs(leftName, nil, rightName, rightDoc))
		}
	}
	return result, nil
}

func (h *QueryHelper) performFullJoin(leftName string, leftDocs []map[string]any, rightName string, rightDocs []map[string]any, condition *QueryFilter) ([]map[string]any, error) {
	var result []map[string]any
	matchedLeftIndices := make(map[int]bool)
	matchedRightIndices := make(map[int]bool)

	for leftIndex, leftDoc := range leftDocs {
		hasMatch := false
		for rightIndex, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)
			matches, err := h.Match(combinedDoc, condition)
			if err != nil {
				return nil, common.NewSystemError("ERR_QUERY_JOIN_CONDITION_EVAL_FAILED", "error evaluating join condition for full join").WithOperation("performFullJoin").WithCause(err)
			}

			if matches {
				result = append(result, combinedDoc)
				hasMatch = true
				matchedLeftIndices[leftIndex] = true
				matchedRightIndices[rightIndex] = true
			}
		}

		if !hasMatch {
			result = append(result, h.combineDocs(leftName, leftDoc, rightName, nil))
		}
	}

	for rightIndex, rightDoc := range rightDocs {
		if !matchedRightIndices[rightIndex] {
			result = append(result, h.combineDocs(leftName, nil, rightName, rightDoc))
		}
	}

	return result, nil
}
