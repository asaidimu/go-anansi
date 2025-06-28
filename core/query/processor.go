package query

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"github.com/asaidimu/go-anansi/core/schema"
	"go.uber.org/zap"
)

// GoComputeFunction is a pure Go function that computes a new value for a row.
// It takes a Row (representing the current data) and returns the computed value
// for a new field, and an error if computation fails.
type ComputeFunction func(row schema.Document, args FilterValue) (any, error)

// GoFilterFunction is a pure Go function that performs custom filtering logic on a row.
// It takes a Row and returns true if the row passes the filter, false otherwise,
// and an error if evaluation fails.
type PredicateFunction func(doc schema.Document, field string, args FilterValue) (bool, error)

// DataProcessor handles Go-based data transformations, filtering, and projections.
type DataProcessor struct {
	goComputeFunctions map[string]ComputeFunction
	goFilterFunctions  map[ComparisonOperator]PredicateFunction
	mu                 sync.RWMutex
	logger             *zap.Logger
}

// NewDataProcessor creates a new DataProcessor instance.
func NewDataProcessor(logger *zap.Logger) *DataProcessor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DataProcessor{
		goComputeFunctions: make(map[string]ComputeFunction),
		goFilterFunctions:  make(map[ComparisonOperator]PredicateFunction),
		logger:             logger,
	}
}

// RegisterComputeFunction registers a Go function for computed fields.
func (p *DataProcessor) RegisterComputeFunction(name string, fn ComputeFunction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.goComputeFunctions[name] = fn
	p.logger.Info("Registered compute function", zap.String("name", name))
}

// RegisterFilterFunction registers a Go function for custom filtering.
func (p *DataProcessor) RegisterFilterFunction(operator ComparisonOperator, fn PredicateFunction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.goFilterFunctions[operator] = fn
	p.logger.Info("Registered filter function", zap.String("operator", string(operator)))
}

// RegisterComputeFunctions registers multiple GoComputeFunction functions from a map.
func (p *DataProcessor) RegisterComputeFunctions(functionMap map[string]ComputeFunction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for name, fn := range functionMap {
		p.goComputeFunctions[name] = fn
		p.logger.Info("Registered compute function", zap.String("name", name))
	}
}

// RegisterFilterFunctions registers multiple GoFilterFunction functions from a map.
func (p *DataProcessor) RegisterFilterFunctions(functionMap map[ComparisonOperator]PredicateFunction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for operator, fn := range functionMap {
		p.goFilterFunctions[operator] = fn
		p.logger.Info("Registered filter function", zap.String("operator", string(operator)))
	}
}

// DetermineFieldsToSelect dynamically analyzes the DSL to find all fields that need to be
// selected from the database, including dependencies for Go functions.
func (p *DataProcessor) DetermineFieldsToSelect(dsl *QueryDSL) []ProjectionField {
	requiredFields := make(map[string]struct{})

	if dsl.Projection != nil {
		for _, field := range dsl.Projection.Include {
			if field.Name != "" {
				requiredFields[field.Name] = struct{}{}
			}
		}

		p.mu.RLock()
		defer p.mu.RUnlock()
		for _, computedItem := range dsl.Projection.Computed {
			if computedItem.ComputedFieldExpression != nil && computedItem.ComputedFieldExpression.Expression != nil {
				for _, arg := range computedItem.ComputedFieldExpression.Expression.Arguments {
					if fieldName, ok := arg.(string); ok {
						requiredFields[fieldName] = struct{}{}
					}
				}
			}
		}
	}

	p.collectGoFilterRequiredFields(dsl.Filters, requiredFields)

	finalFields := make([]ProjectionField, 0, len(requiredFields))
	for fieldName := range requiredFields {
		finalFields = append(finalFields, ProjectionField{Name: fieldName})
	}
	return finalFields
}

// collectGoFilterRequiredFields recursively traverses the filter DSL to find
// fields referenced by Go-based filter functions.
func (p *DataProcessor) collectGoFilterRequiredFields(filter *QueryFilter, fields map[string]struct{}) {
	if filter == nil {
		return
	}
	if filter.Condition != nil {
		if !filter.Condition.Operator.IsStandard() {
			fields[filter.Condition.Field] = struct{}{}
		}
	}
	if filter.Group != nil {
		for _, subFilter := range filter.Group.Conditions {
			p.collectGoFilterRequiredFields(&subFilter, fields)
		}
	}
}

// ProcessRows applies all Go-based transformations to the provided rows.
// It now accepts 'skippedOperators' to indicate which standard filters were
// already applied by an external component (e.g., a database).
func (p *DataProcessor) ProcessRows(rows []schema.Document, dsl *QueryDSL, skippedOperators []ComparisonOperator) ([]schema.Document, error) {
	processedRows, err := p.applyGoFilters(rows, dsl.Filters, skippedOperators)
	if err != nil {
		return nil, fmt.Errorf("Go filter failed: %w", err)
	}
	p.logger.Debug("Rows remaining after Go filters", zap.Int("count", len(processedRows)))

	processedRows, err = p.applyGoComputeFunctions(processedRows, dsl.Projection)
	if err != nil {
		return nil, fmt.Errorf("Go computed field failed: %w", err)
	}

	finalResults := p.applyFinalProjection(processedRows, dsl.Projection)
	p.logger.Debug("Rows returned after final projection", zap.Int("count", len(finalResults)))

	return finalResults, nil
}

// applyGoFilters iterates through rows and applies Go-based filter functions.
// It now accepts a 'skip' parameter for operators already handled externally (e.g., by DB).
// PRODUCTION WARNING: This filtering happens in-memory. Queries with non-selective
// SQL filters but highly selective Go filters can cause high memory usage.
// Prefer native SQL filters for performance whenever possible.
func (p *DataProcessor) applyGoFilters(rows []schema.Document, filter *QueryFilter, skip []ComparisonOperator) ([]schema.Document, error) {
	if filter == nil {
		return rows, nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	skipMap := make(map[ComparisonOperator]struct{})
	for _, op := range skip {
		skipMap[op] = struct{}{}
	}

	var filteredRows []schema.Document
	for _, row := range rows {
		passes, err := p.evaluateGoFilter(row, filter, skipMap)
		if err != nil {
			return nil, fmt.Errorf("error evaluating Go filter for row %+v: %w", row, err)
		}
		if passes {
			filteredRows = append(filteredRows, row)
		}
	}
	return filteredRows, nil
}

// evaluateGoFilter recursively evaluates a QueryFilter, applying Go functions where necessary.
// It now takes a map of operators to skip.
func (p *DataProcessor) evaluateGoFilter(row schema.Document, filter *QueryFilter, skip map[ComparisonOperator]struct{}) (bool, error) {
	if filter.Condition != nil {
		// If the operator is standard and marked to be skipped, assume it's already handled and passes.
		if _, shouldSkip := skip[filter.Condition.Operator]; shouldSkip {
			return true, nil
		}

		if !filter.Condition.Operator.IsStandard() {
			fn, ok := p.goFilterFunctions[filter.Condition.Operator]
			if !ok {
				return false, fmt.Errorf("unregistered Go filter function for operator: %s", filter.Condition.Operator)
			}
			return fn(row, filter.Condition.Field, filter.Condition.Value)
		}
		// Handle standard SQL conditions in-memory if not skipped.
		return p.evaluateStandardCondition(row, filter.Condition)

	}
	if filter.Group != nil {
		switch filter.Group.Operator {
		case schema.LogicalAnd:
			for _, cond := range filter.Group.Conditions {
				passes, err := p.evaluateGoFilter(row, &cond, skip)
				if err != nil || !passes {
					return false, err
				}
			}
			return true, nil
		case schema.LogicalOr:
			for _, cond := range filter.Group.Conditions {
				passes, err := p.evaluateGoFilter(row, &cond, skip)
				if err != nil {
					return false, err
				}
				if passes {
					return true, nil
				}
			}
			return false, nil
		default:
			return false, fmt.Errorf("unsupported logical operator for Go evaluation: %s", filter.Group.Operator)
		}
	}
	return false, fmt.Errorf("empty or invalid filter structure for Go evaluation")
}

// evaluateStandardCondition performs the in-memory evaluation for standard comparison operators.
func (p *DataProcessor) evaluateStandardCondition(row schema.Document, condition *FilterCondition) (bool, error) {
	fieldValue, ok := row[condition.Field]
	if !ok {
		// If the field doesn't exist in the row, the condition likely fails.
		return false, nil
	}

	switch condition.Operator {
	case ComparisonOperatorEq:
		return fieldValue == condition.Value, nil
	case ComparisonOperatorNeq:
		return fieldValue != condition.Value, nil
	case ComparisonOperatorGt:
		if fvNum, okF := ToFloat64(fieldValue); okF {
			if condNum, okC := ToFloat64(condition.Value); okC {
				return fvNum > condNum, nil
			}
		}
		return false, fmt.Errorf("unsupported type for GT comparison between %T and %T", fieldValue, condition.Value)
	case ComparisonOperatorLt:
		if fvNum, okF := ToFloat64(fieldValue); okF {
			if condNum, okC := ToFloat64(condition.Value); okC {
				return fvNum < condNum, nil
			}
		}
		return false, fmt.Errorf("unsupported type for LT comparison between %T and %T", fieldValue, condition.Value)
	default:
		return false, fmt.Errorf("unsupported standard comparison operator for Go evaluation: %s", condition.Operator)
	}
}

// applyGoComputeFunctions iterates through rows and applies Go-based computed field functions.
func (p *DataProcessor) applyGoComputeFunctions(rows []schema.Document, projection *ProjectionConfiguration) ([]schema.Document, error) {
	if projection == nil || len(projection.Computed) == 0 {
		return rows, nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	for i := range rows {
		for _, item := range projection.Computed {
			if item.ComputedFieldExpression != nil {
				funcName := item.ComputedFieldExpression.Expression.Function
				alias := item.ComputedFieldExpression.Alias
				if alias == "" {
					alias = fmt.Sprintf("%v", funcName)
				}

				fn, ok := p.goComputeFunctions[fmt.Sprintf("%v", funcName)]
				if !ok {
					return nil, fmt.Errorf("unregistered Go compute function: %v", funcName)
				}

				computedValue, err := fn(rows[i], item.ComputedFieldExpression.Expression.Arguments)
				if err != nil {
					return nil, fmt.Errorf("error executing Go compute function '%v': %w", funcName, err)
				}
				rows[i][alias] = computedValue
			}
		}
	}
	return rows, nil
}

// applyFinalProjection processes rows to match the user's requested projection (include/exclude).
func (p *DataProcessor) applyFinalProjection(rows []schema.Document, projection *ProjectionConfiguration) []schema.Document {
	if projection == nil || (len(projection.Include) == 0 && len(projection.Exclude) == 0 && len(projection.Computed) == 0) {
		return rows
	}

	var finalRows []schema.Document
	includeAll := len(projection.Include) == 0
	excludeSet := make(map[string]struct{}, len(projection.Exclude))
	for _, f := range projection.Exclude {
		excludeSet[f.Name] = struct{}{}
	}

	includeSet := make(map[string]struct{}, len(projection.Include))
	if !includeAll {
		for _, f := range projection.Include {
			includeSet[f.Name] = struct{}{}
		}
	}

	for _, computedItem := range projection.Computed {
		if computedItem.ComputedFieldExpression != nil && computedItem.ComputedFieldExpression.Alias != "" {
			alias := computedItem.ComputedFieldExpression.Alias
			if _, excluded := excludeSet[alias]; !excluded {
				if includeAll || len(projection.Include) > 0 {
					includeSet[alias] = struct{}{}
				}
			}
		}
	}

	for _, originalRow := range rows {
		newRow := make(schema.Document)

		if includeAll {
			maps.Copy(newRow, originalRow)
		} else {
			for fieldName, value := range originalRow {
				if _, ok := includeSet[fieldName]; ok {
					newRow[fieldName] = value
				}
			}
		}

		if len(excludeSet) > 0 {
			for fieldName := range excludeSet {
				delete(newRow, fieldName)
			}
		}
		finalRows = append(finalRows, newRow)
	}
	return finalRows
}

// Match evaluates a given data object against a set of QueryFilter conditions.
// It returns true if the data matches all filter conditions, false otherwise,
// and an error if the evaluation encounters an issue. This method is useful
// for applying filtering logic used in queries to in-memory data.
// It assumes no operators are skipped when matching a single in-memory row.
func (p *DataProcessor) Match(ctx context.Context, filters *QueryFilter, data schema.Document) (bool, error) {
	if filters == nil {
		return true, nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.evaluateGoFilter(data, filters, map[ComparisonOperator]struct{}{})
}
