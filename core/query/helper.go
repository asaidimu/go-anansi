package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/data"
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
		return nil, errors.New("query cannot be nil")
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
		return nil, fmt.Errorf("invalid query: %w", err)
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
		return errors.New("target name cannot be empty when target is specified")
	}

	// Validate filters
	if h.query.Filters != nil {
		if err := h.validateQueryFilter(h.query.Filters); err != nil {
			return fmt.Errorf("invalid filters: %w", err)
		}
	}

	// Validate sort configurations
	for _, sortConfig := range h.query.Sort {
		if sortConfig.Field == "" {
			return errors.New("sort field cannot be empty")
		}
		if sortConfig.Direction != SortDirectionAsc && sortConfig.Direction != SortDirectionDesc {
			return fmt.Errorf("invalid sort direction: %s", sortConfig.Direction)
		}
	}

	// Validate pagination
	if h.query.Pagination != nil {
		if h.query.Pagination.Type != "offset" {
			return fmt.Errorf("invalid pagination type: %s", h.query.Pagination.Type)
		}
		if h.query.Pagination.Limit <= 0 {
			return errors.New("pagination limit must be greater than 0")
		}
		if h.query.Pagination.Offset != nil && *h.query.Pagination.Offset < 0 {
			return errors.New("pagination offset cannot be negative")
		}
	}

	// Validate projection
	if h.query.Projection != nil {
		if err := h.validateProjectionConfiguration(h.query.Projection); err != nil {
			return fmt.Errorf("invalid projection: %w", err)
		}
	}

	// Validate aggregations
	if h.query.Aggregations != nil {
		if err := h.validateAggregationConfiguration(h.query.Aggregations); err != nil {
			return fmt.Errorf("invalid aggregation configuration: %w", err)
		}
	}

	if h.query.Distinct != nil {
		if err := h.validateDistinctConfiguration(h.query.Distinct); err != nil {
			return fmt.Errorf("invalid distinct configuration: %w", err)
		}
	}

	return nil
}

// validateQueryFilter validates a QueryFilter structure recursively.
func (h *QueryHelper) validateQueryFilter(filter *QueryFilter) error {
	if filter == nil {
		return errors.New("filter cannot be nil")
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
		return errors.New("exactly one of Condition, Group, or TextSearchQuery must be set")
	}

	// Validate condition
	if filter.Condition != nil {
		if filter.Condition.Field == "" {
			return errors.New("condition field cannot be empty")
		}

		// *** MODIFIED LOGIC HERE: Check if operator is custom or standard ***
		isCustom := false
		if h.operators != nil {
			if _, ok := (*h.operators)[string(filter.Condition.Operator)]; ok {
				isCustom = true
			}
		}

		if !isCustom && !filter.Condition.Operator.IsStandard() {
			return fmt.Errorf("unsupported comparison operator: %s", filter.Condition.Operator)
		}
		// End of MODIFIED LOGIC

		// Basic validation for FilterValue - can be extended if needed
		if err := h.validateFilterValue(&filter.Condition.Value); err != nil {
			return fmt.Errorf("invalid condition value: %w", err)
		}
	}

	// Validate group
	if filter.Group != nil {
		if len(filter.Group.Conditions) == 0 {
			return errors.New("filter group must have at least one condition")
		}
		for i, condition := range filter.Group.Conditions {
			// Pass a pointer to the condition to allow recursive validation
			if err := h.validateQueryFilter(&condition); err != nil {
				return fmt.Errorf("invalid condition at index %d: %w", i, err)
			}
		}
	}

	// TextMatch validation - return error since it's not implemented yet but allowed in DSL
	if filter.TextSearchQuery != nil {
		return errors.New("text search is an advanced feature and not implemented in this version of the helper")
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
				return fmt.Errorf("invalid array value at index %d: %w", i, err)
			}
		}
	}
	if fv.FieldRefVal != nil {
		setFields++
		if fv.FieldRefVal.Type != "field" {
			return errors.New("field reference type must be 'field'")
		}
		if fv.FieldRefVal.Field == "" {
			return errors.New("field reference field cannot be empty")
		}
	}
	if fv.SubqueryVal != nil {
		setFields++
		if fv.SubqueryVal.Type != "subquery" {
			return errors.New("subquery value type must be 'subquery'")
		}
		// Subqueries are not supported by this helper for evaluation in filters
		return errors.New("subqueries are not supported by this in-memory query helper")
	}
	if fv.FunctionCallVal != nil {
		setFields++
		// Function calls in FilterValue are partially supported (evaluates to nil for now)
		// but structure can be validated.
		if fv.FunctionCallVal.Function == "" {
			return errors.New("function call function name cannot be empty")
		}
		for i, arg := range fv.FunctionCallVal.Arguments {
			if err := h.validateFilterValue(&arg); err != nil {
				return fmt.Errorf("invalid function argument at index %d: %w", i, err)
			}
		}
	}

	if setFields > 1 {
		return errors.New("FilterValue can only have one type of value set")
	}
	return nil
}

// validateProjectionConfiguration validates the projection settings.
func (h *QueryHelper) validateProjectionConfiguration(proj *ProjectionConfiguration) error {
	if proj == nil {
		return errors.New("projection configuration cannot be nil")
	}

	if len(proj.Include) > 0 && len(proj.Exclude) > 0 {
		return errors.New("cannot use both include and exclude in the same projection")
	}

	for _, field := range proj.Include {
		if field.Name == "" {
			return errors.New("projection include field name cannot be empty")
		}
		if field.Nested != nil {
			if err := h.validateProjectionConfiguration(field.Nested); err != nil {
				return fmt.Errorf("invalid nested projection for field %s: %w", field.Name, err)
			}
		}
	}

	for _, field := range proj.Exclude {
		if field.Name == "" {
			return errors.New("projection exclude field name cannot be empty")
		}
		if field.Nested != nil {
			// For exclude, nested means exclude nested fields. It's conceptually simpler than include.
			if err := h.validateProjectionConfiguration(field.Nested); err != nil {
				return fmt.Errorf("invalid nested projection for excluded field %s: %w", field.Name, err)
			}
		}
	}

	for _, computed := range proj.Computed {
		setFields := 0
		if computed.ComputedFieldExpression != nil {
			setFields++
			if computed.ComputedFieldExpression.Type != "computed_field" {
				return errors.New("computed field expression type must be 'computed_field'")
			}
			if computed.ComputedFieldExpression.Expression == nil {
				return errors.New("computed field expression cannot be nil")
			}
			if computed.ComputedFieldExpression.Alias == "" {
				return errors.New("computed field alias cannot be empty")
			}
			// Validate arguments of the function call within computed field
			if err := h.validateFunctionCall(computed.ComputedFieldExpression.Expression); err != nil {
				return fmt.Errorf("invalid function call in computed field '%s': %w", computed.ComputedFieldExpression.Alias, err)
			}
		}
		if computed.CaseExpression != nil {
			setFields++
			if computed.CaseExpression.Type != "case_expression" {
				return errors.New("case expression type must be 'case_expression'")
			}
			if len(computed.CaseExpression.Conditions) == 0 {
				return errors.New("case expression must have at least one condition")
			}
			if computed.CaseExpression.Alias == "" {
				return errors.New("case expression alias cannot be empty")
			}
			for i, cond := range computed.CaseExpression.Conditions {
				if err := h.validateQueryFilter(&cond.When); err != nil {
					return fmt.Errorf("invalid 'when' condition in case expression at index %d: %w", i, err)
				}
				if err := h.validateFilterValue(&cond.Then); err != nil {
					return fmt.Errorf("invalid 'then' value in case expression at index %d: %w", i, err)
				}
			}
			if err := h.validateFilterValue(&computed.CaseExpression.Else); err != nil {
				return fmt.Errorf("invalid 'else' value in case expression: %w", err)
			}
		}
		if setFields != 1 {
			return errors.New("ProjectionComputedItem must have exactly one of ComputedFieldExpression or CaseExpression set")
		}
	}
	return nil
}

// validateFunctionCall validates a FunctionCall structure.
func (h *QueryHelper) validateFunctionCall(fc *FunctionCall) error {
	if fc.Function == "" {
		return errors.New("function call name cannot be empty")
	}
	for i, arg := range fc.Arguments {
		if err := h.validateFilterValue(&arg); err != nil {
			return fmt.Errorf("invalid argument %d for function '%s': %w", i, fc.Function, err)
		}
	}
	return nil
}

// validateDistinctConfiguration validates the distinct settings.
func (h *QueryHelper) validateDistinctConfiguration(distinct *QueryDistinctConfig) error {
	if distinct == nil {
		return errors.New("distinct configuration cannot be nil")
	}
	setFields := 0
	if distinct.IsDistinct != nil {
		setFields++
	}
	if distinct.Fields != nil {
		setFields++
	}

	if setFields == 0 {
		return errors.New("distinct configuration must specify 'is_distinct' or 'fields'")
	}
	if setFields > 1 {
		return errors.New("distinct configuration cannot specify both 'is_distinct' and 'fields'")
	}
	if distinct.Fields != nil {
		if slices.Contains(distinct.Fields, "") {
			return errors.New("distinct field name cannot be empty")
		}
	}
	return nil
}

// validateAggregationConfiguration validates the aggregation settings.
func (h *QueryHelper) validateAggregationConfiguration(aggregations []AggregationConfiguration) error {
	if len(aggregations) == 0 {
		return errors.New("aggregation configuration cannot be empty if specified")
	}

	for _, agg := range aggregations {
		if agg.Field == "" && agg.Type != AggregationTypeCount { // Count can be on no specific field
			return errors.New("aggregation field cannot be empty for non-count aggregations")
		}

		// Check if the aggregation type is supported
		if h.aggregates != nil {
			if _, ok := (*h.aggregates)[agg.Type]; !ok {
				return fmt.Errorf("unsupported aggregation type: %s", agg.Type)
			}
		}

		if agg.Filter != nil {
			if err := h.validateQueryFilter(agg.Filter); err != nil {
				return fmt.Errorf("invalid filter for aggregation on field '%s': %w", agg.Field, err)
			}
		}

		if slices.Contains(agg.Groups, "") {
			return errors.New("aggregation group field cannot be empty")
		}
	}
	return nil
}

// (Rest of the validateQueryFilter, validateFilterValue, validateProjectionConfiguration,
// validateFunctionCall, validateDistinctConfiguration remain unchanged)

// Match evaluates a single record against the provided filters.
// If no filters are provided, it returns true.
func (h *QueryHelper) Match(record data.Document, filters ...*QueryFilter) (bool, error) {
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
func (h *QueryHelper) Filter(records []data.Document, filters ...*QueryFilter) ([]data.Document, error) {
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

	var filtered []data.Document
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
				return nil, fmt.Errorf("failed to marshal record for distinct check: %w", err)
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
				doc := data.Document(record)
				keyValues[i], _ = h.getValueByPath(doc, field)
			}
			// Create a string representation of the key values for map lookup
			keyBytes, err := json.Marshal(keyValues)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal distinct key: %w", err)
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
func (h *QueryHelper) Sort(records []data.Document) ([]data.Document, error) {
	if len(h.query.Sort) == 0 {
		return records, nil
	}

	// Create a copy to avoid modifying the original slice
	sorted := make([]data.Document, len(records))
	copy(sorted, records)

	sort.Slice(sorted, func(i, j int) bool {
		for _, sortConfig := range h.query.Sort {
			doci := data.Document(sorted[i])
			docj := data.Document(sorted[j])
			valueI, _ := h.getValueByPath(doci, sortConfig.Field)
			valueJ, _ := h.getValueByPath(docj, sortConfig.Field)

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
func (h *QueryHelper) Paginate(records []data.Document) ([]data.Document, *PaginationResult, error) {
	if h.query.Pagination == nil {
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
			return []data.Document{}, &PaginationResult{
				Total: &totalCount,
			}, nil
		}

		end := min(offset+pagination.Limit, totalCount)

		return records[offset:end], &PaginationResult{
			Total: &totalCount,
		}, nil

	default:
		return nil, nil, fmt.Errorf("unsupported pagination type: %s", pagination.Type)
	}
}

// Project applies field projection to a collection of records.
// Returns a new slice with records containing only the projected fields.
func (h *QueryHelper) Project(records []data.Document) ([]data.Document, error) {
	if h.query.Projection == nil {
		return records, nil
	}

	projected := make([]data.Document, len(records))
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
func (h *QueryHelper) ProjectSingle(record data.Document) (data.Document, error) {
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
							nestedMap, ok = value.(data.Document)
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
						nestedMap, ok = value.(data.Document)
					}
					// Not the last part, traverse into nested map
					if ok {
						_, isDoc := currentResult[part].(data.Document)
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
						targetMap, ok = currentResult[part].(data.Document)
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
					nestedMap, ok = currentResult[part].(data.Document)
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
func (h *QueryHelper) ApplyAggregations(records []data.Document) (data.Document, error) {
	if h.query.Aggregations == nil {
		return nil, nil // No aggregations defined
	}

	results := make(data.Document)

	for _, aggConfig := range h.query.Aggregations {
		// Step 1: Apply filtering for this specific aggregation
		currentRecords := records
		if aggConfig.Filter != nil {
			var err error
			// Use the modified Filter method to apply the specific aggregation filter
			filteredRecords, err := h.Filter(currentRecords, aggConfig.Filter)
			if err != nil {
				return nil, fmt.Errorf("error applying filter for aggregation '%s': %w", aggConfig.AliasOrDefault(), err)
			}
			currentRecords = filteredRecords
		}

		// Resolve the aggregation function
		aggFunc, ok := (*h.aggregates)[aggConfig.Type]
		if !ok {
			return nil, fmt.Errorf("unsupported aggregation type: %s", aggConfig.Type)
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
				return nil, fmt.Errorf("error performing aggregation for field '%s': %w", aggConfig.Field, err)
			}
			results[aggConfig.AliasOrDefault()] = aggregatedValue
		}
	}

	return results, nil
}

// processGroupedAggregation handles aggregations with a 'groups' clause.
func (h *QueryHelper) processGroupedAggregation(records []data.Document, aggConfig AggregationConfiguration, aggFunc AggregateFunction) ([]data.Document, error) {
	groupedData := make(map[string][]data.Document)
	groupKeyMap := make(map[string]data.Document) // To store the actual group key values for output

	for _, record := range records {
		// Create a composite key for grouping
		groupKeyParts := make([]string, len(aggConfig.Groups))
		currentGroupKeyValues := make(data.Document)

		for i, groupField := range aggConfig.Groups {
			doc := data.Document(record)
			val, _ := h.getValueByPath(doc, groupField)
			groupKeyParts[i] = fmt.Sprintf("%v", val) // Convert to string for map key
			currentGroupKeyValues[groupField] = val   // Store actual values for later
		}
		groupKey := strings.Join(groupKeyParts, "|") // Delimiter for composite key

		groupedData[groupKey] = append(groupedData[groupKey], record)
		if _, ok := groupKeyMap[groupKey]; !ok {
			groupKeyMap[groupKey] = currentGroupKeyValues // Store group field values once per group
		}
	}

	var finalGroupedResults []data.Document

	for groupKey, groupRecords := range groupedData {
		aggregatedValue, err := aggFunc(groupRecords, aggConfig.Field)
		if err != nil {
			return nil, fmt.Errorf("error performing grouped aggregation for key '%s', field '%s': %w", groupKey, aggConfig.Field, err)
		}

		// Construct the result object for this group
		groupResult := make(data.Document)
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
func (h *QueryHelper) evaluateQueryFilter(record data.Document, filter *QueryFilter) (bool, error) {
	if filter.Condition != nil {
		return h.evaluateCondition(record, filter.Condition)
	}

	if filter.Group != nil {
		return h.evaluateGroup(record, filter.Group)
	}

	if filter.TextSearchQuery != nil {
		// Text search is not implemented for this helper
		return false, errors.New("text search is not implemented for this in-memory helper")
	}

	return false, errors.New("invalid filter: no condition, group, or text match specified")
}

// evaluateCondition evaluates a FilterCondition against a record.
func (h *QueryHelper) evaluateCondition(record data.Document, condition *FilterCondition) (bool, error) {
	// Use rich filter function if available
	if fn, ok := h.goFilterFunctions[condition.Operator]; ok {
		return fn(record, condition.Field, condition.Value)
	}

	doc := data.Document(record)
	fieldValue, _ := h.getValueByPath(doc, condition.Field)
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
		return false, fmt.Errorf("unsupported operator: %s", condition.Operator)
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
		doc := data.Document(record)
		result, _ := h.getValueByPath(doc, fv.FieldRefVal.Field)
		return result, nil
	}

	if fv.SubqueryVal != nil {
		// Subqueries are not supported by this helper for evaluation in filters
		return nil, errors.New("subqueries are not supported by this in-memory query helper")
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
			return false, fmt.Errorf("evaluating condition %d: %w", i, err)
		}
		results[i] = result
	}
	return group.Operator.Evaluate(results)
}

// evaluateInOperator evaluates the IN operator.
func (h *QueryHelper) evaluateInOperator(fieldValue, conditionValue any) (bool, error) {
	conditionSlice := reflect.ValueOf(conditionValue)
	if conditionSlice.Kind() != reflect.Slice && conditionSlice.Kind() != reflect.Array {
		return false, errors.New("IN operator requires a slice or array value as the comparison target")
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
		return false, errors.New("CONTAINS operator requires string values for comparison target")
	}
	return strings.Contains(fieldStr, conditionStr), nil
}

// evaluateComputedField evaluates a computed field expression.
func (h *QueryHelper) evaluateComputedField(record map[string]any, expr *ComputedFieldExpression) (any, error) {
	if expr.Expression == nil {
		return nil, errors.New("computed field expression cannot be nil")
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

	return nil, fmt.Errorf("function '%s' is not implemented or registered in this helper", fc.Function)
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
func (h *QueryHelper) combineDocs(leftName string, leftDoc data.Document, rightName string, rightDoc data.Document) data.Document {
	combinedDoc := make(data.Document)

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
func (h *QueryHelper) Join(left, right []data.Document, config *JoinConfiguration) ([]data.Document, error) {
	// Validate inputs
	if config == nil {
		return nil, errors.New("join configuration cannot be nil")
	}

	// Determine the right-side collection name to use in the combined document
	rightName := config.Target.Name
	if config.Target.Alias != nil && *config.Target.Alias != "" {
		rightName = *config.Target.Alias
	}

	// Determine left-side collection name
	leftName := h.getLeftCollectionName()

	var result []data.Document
	switch config.Type {
	case JoinTypeInner:
		result = h.performInnerJoin(leftName, left, rightName, right, config.On)
	case JoinTypeLeft:
		result = h.performLeftJoin(leftName, left, rightName, right, config.On)
	case JoinTypeRight:
		result = h.performRightJoin(leftName, left, rightName, right, config.On)
	case JoinTypeFull:
		result = h.performFullJoin(leftName, left, rightName, right, config.On)
	default:
		return nil, fmt.Errorf("unsupported join type: %s", config.Type)
	}

	// Apply projection if specified in the join configuration
	if config.Projection != nil {
		var projectedResult []data.Document
		for _, doc := range result {
			projected, err := h.projectRecord(doc, config.Projection)
			if err != nil {
				return nil, fmt.Errorf("projection error during join: %w", err)
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
func (h *QueryHelper) JoinStreams(left, right <-chan data.Document, target *QueryTarget, config *JoinConfiguration) (<-chan data.Document, <-chan error) {
	resultChan := make(chan data.Document)
	errorChan := make(chan error, 1)

	go func() {
		defer close(resultChan)
		defer close(errorChan)

		// Validate inputs
		if config == nil {
			errorChan <- errors.New("join configuration cannot be nil")
			return
		}

		if target == nil {
			errorChan <- errors.New("target cannot be nil")
			return
		}

		if target.Name == "" {
			errorChan <- errors.New("target name cannot be empty")
			return
		}

		if config.On == nil {
			errorChan <- errors.New("join condition ('on') cannot be nil")
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
			errorChan <- fmt.Errorf("unsupported join type: %s", config.Type)
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
func (h *QueryHelper) performStreamingInnerJoin(leftName string, left <-chan data.Document, rightName string, right <-chan data.Document, config *JoinConfiguration, resultChan chan<- data.Document, errorChan chan<- error) {
	// For streaming inner joins, we need to buffer one side (typically the smaller one)
	// This is a simplified implementation - in practice, you might want more sophisticated buffering
	var rightDocs []data.Document

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
				errorChan <- fmt.Errorf("error evaluating join condition: %w", err)
				return
			}

			if matches {
				// Apply projection if specified
				if config.Projection != nil {
					projected, err := h.projectRecord(combinedDoc, config.Projection)
					if err != nil {
						errorChan <- fmt.Errorf("projection error during streaming join: %w", err)
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
func (h *QueryHelper) performStreamingLeftJoin(leftName string, left <-chan data.Document, rightName string, right <-chan data.Document, config *JoinConfiguration, resultChan chan<- data.Document, errorChan chan<- error) {
	// Buffer all right documents
	var rightDocs []data.Document
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
				errorChan <- fmt.Errorf("error evaluating join condition: %w", err)
				return
			}

			if matches {
				// Apply projection if specified
				if config.Projection != nil {
					projected, err := h.projectRecord(combinedDoc, config.Projection)
					if err != nil {
						errorChan <- fmt.Errorf("projection error during streaming join: %w", err)
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
					errorChan <- fmt.Errorf("projection error during streaming join: %w", err)
					return
				}
				combinedDoc = projected
			}
			resultChan <- combinedDoc
		}
	}
}

// performStreamingRightJoin implements streaming right join logic.
func (h *QueryHelper) performStreamingRightJoin(leftName string, left <-chan data.Document, rightName string, right <-chan data.Document, config *JoinConfiguration, resultChan chan<- data.Document, errorChan chan<- error) {
	// Buffer all left documents
	var leftDocs []data.Document
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
				errorChan <- fmt.Errorf("error evaluating join condition: %w", err)
				return
			}

			if matches {
				// Apply projection if specified
				if config.Projection != nil {
					projected, err := h.projectRecord(combinedDoc, config.Projection)
					if err != nil {
						errorChan <- fmt.Errorf("projection error during streaming join: %w", err)
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
					errorChan <- fmt.Errorf("projection error during streaming join: %w", err)
					return
				}
				combinedDoc = projected
			}
			resultChan <- combinedDoc
		}
	}
}

// performStreamingFullJoin implements streaming full outer join logic.
func (h *QueryHelper) performStreamingFullJoin(leftName string, left <-chan data.Document, rightName string, right <-chan data.Document, config *JoinConfiguration, resultChan chan<- data.Document, errorChan chan<- error) {
	// Buffer both sides for full outer join
	var leftDocs []data.Document
	var rightDocs []data.Document

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
				errorChan <- fmt.Errorf("error evaluating join condition: %w", err)
				return
			}

			if matches {
				// Apply projection if specified
				if config.Projection != nil {
					projected, err := h.projectRecord(combinedDoc, config.Projection)
					if err != nil {
						errorChan <- fmt.Errorf("projection error during streaming join: %w", err)
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
					errorChan <- fmt.Errorf("projection error during streaming join: %w", err)
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
					errorChan <- fmt.Errorf("projection error during streaming join: %w", err)
					return
				}
				combinedDoc = projected
			}
			resultChan <- combinedDoc
		}
	}
}

func (h *QueryHelper) performInnerJoin(leftName string, leftDocs []data.Document, rightName string, rightDocs []data.Document, condition *QueryFilter) []data.Document {
	var result []data.Document

	for _, leftDoc := range leftDocs {
		for _, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)

			matches, err := h.Match(combinedDoc, condition)
			if err != nil {
				continue // Skip on error
			}

			if matches {
				result = append(result, combinedDoc)
			}
		}
	}

	return result
}

func (h *QueryHelper) performLeftJoin(leftName string, leftDocs []data.Document, rightName string, rightDocs []data.Document, condition *QueryFilter) []data.Document {
	var result []data.Document

	for _, leftDoc := range leftDocs {
		hasMatch := false
		for _, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)
			matches, err := h.Match(combinedDoc, condition)
			if err != nil {
				continue
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

	return result
}

func (h *QueryHelper) performRightJoin(leftName string, leftDocs []data.Document, rightName string, rightDocs []data.Document, condition *QueryFilter) []data.Document {
	var result []data.Document
	matchedRightIndices := make(map[int]bool)

	for _, leftDoc := range leftDocs {
		for rightIndex, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)
			matches, err := h.Match(combinedDoc, condition)
			if err != nil {
				continue
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
	return result
}

func (h *QueryHelper) performFullJoin(leftName string, leftDocs []data.Document, rightName string, rightDocs []data.Document, condition *QueryFilter) []data.Document {
	var result []data.Document
	matchedLeftIndices := make(map[int]bool)
	matchedRightIndices := make(map[int]bool)

	for leftIndex, leftDoc := range leftDocs {
		hasMatch := false
		for rightIndex, rightDoc := range rightDocs {
			combinedDoc := h.combineDocs(leftName, leftDoc, rightName, rightDoc)
			matches, err := h.Match(combinedDoc, condition)
			if err != nil {
				continue
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

	return result
}

func (h *QueryHelper) getValueByPath(doc data.Document, path string) (any, bool) {
	return utils.GetValueByPath(doc, path)
}
