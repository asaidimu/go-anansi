// Package query provides the DataProcessor, which is responsible for handling
// in-memory data transformations, filtering, and projections that cannot be
// pushed down to the database.
package query

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"github.com/asaidimu/go-anansi/core/schema"
	"go.uber.org/zap"
)

// ComputeFunction is a function that computes a new value for a row of data.
// It takes a document (representing a single row) and a set of arguments, and
// returns the computed value.	ype ComputeFunction func(row schema.Document, args FilterValue) (any, error)

// PredicateFunction is a function that performs custom filtering logic on a row.
// It returns true if the row should be included in the result set, and false otherwise.	ype PredicateFunction func(doc schema.Document, field string, args FilterValue) (bool, error)

// DataProcessor handles Go-based data transformations, filtering, and projections.
// It is used to perform operations on data after it has been fetched from the database,
// allowing for complex logic that may not be supported by the underlying database.	ype DataProcessor struct {
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

// RegisterComputeFunction registers a Go function that can be used for computed fields.
func (p *DataProcessor) RegisterComputeFunction(name string, fn ComputeFunction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.goComputeFunctions[name] = fn
	p.logger.Info("Registered compute function", zap.String("name", name))
}

// RegisterFilterFunction registers a Go function that can be used for custom filtering.
func (p *DataProcessor) RegisterFilterFunction(operator ComparisonOperator, fn PredicateFunction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.goFilterFunctions[operator] = fn
	p.logger.Info("Registered filter function", zap.String("operator", string(operator)))
}

// RegisterComputeFunctions registers multiple compute functions from a map.
func (p *DataProcessor) RegisterComputeFunctions(functionMap map[string]ComputeFunction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for name, fn := range functionMap {
		p.goComputeFunctions[name] = fn
		p.logger.Info("Registered compute function", zap.String("name", name))
	}
}

// RegisterFilterFunctions registers multiple filter functions from a map.
func (p *DataProcessor) RegisterFilterFunctions(functionMap map[ComparisonOperator]PredicateFunction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for operator, fn := range functionMap {
		p.goFilterFunctions[operator] = fn
		p.logger.Info("Registered filter function", zap.String("operator", string(operator)))
	}
}

// DetermineFieldsToSelect analyzes the QueryDSL to determine which fields need to be
// selected from the database. This includes fields that are explicitly requested in
// the projection, as well as any fields that are required for in-memory computations
// or filters.
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

// collectGoFilterRequiredFields recursively traverses the filter DSL to find all
// fields that are required for Go-based filter functions.
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

// ProcessRows applies all registered Go-based transformations, filters, and
// projections to a set of rows. It can also skip certain standard operators
// that have already been applied by the database.
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

// applyGoFilters applies all registered Go-based filter functions to a set of rows.
// It can skip operators that have already been handled by the database.
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
func (p *DataProcessor) evaluateGoFilter(row schema.Document, filter *QueryFilter, skip map[ComparisonOperator]struct{}) (bool, error) {
	if filter.Condition != nil {
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

// evaluateStandardCondition performs in-memory evaluation of standard comparison operators.
func (p *DataProcessor) evaluateStandardCondition(row schema.Document, condition *FilterCondition) (bool, error) {
	fieldValue, ok := row[condition.Field]
	if !ok {
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

// applyGoComputeFunctions applies all registered Go-based compute functions to a set of rows.
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

// applyFinalProjection applies the final include/exclude projection to a set of rows.
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
// It returns true if the data matches all filter conditions, and false otherwise.
func (p *DataProcessor) Match(ctx context.Context, filters *QueryFilter, data schema.Document) (bool, error) {
	if filters == nil {
		return true, nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.evaluateGoFilter(data, filters, map[ComparisonOperator]struct{}{})
}
