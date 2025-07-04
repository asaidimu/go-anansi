// Package query defines the Domain-Specific Language (DSL) for constructing
// database queries. This DSL provides a structured and type-safe way to express
// complex query logic, including filtering, sorting, pagination, and more.
package query

import (
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// Logical operators for combining filter conditions.
const (
	LogicalOperatorAnd schema.LogicalOperator = "and"
	LogicalOperatorOr  schema.LogicalOperator = "or"
	LogicalOperatorNot schema.LogicalOperator = "not"
	LogicalOperatorNor schema.LogicalOperator = "nor"
	LogicalOperatorXor schema.LogicalOperator = "xor"
)

// ComparisonOperator defines the set of operators that can be used in a filter condition.
type ComparisonOperator string

// Supported comparison operators.
const (
	ComparisonOperatorEq          ComparisonOperator = "eq"
	ComparisonOperatorNeq         ComparisonOperator = "neq"
	ComparisonOperatorLt          ComparisonOperator = "lt"
	ComparisonOperatorLte         ComparisonOperator = "lte"
	ComparisonOperatorGt          ComparisonOperator = "gt"
	ComparisonOperatorGte         ComparisonOperator = "gte"
	ComparisonOperatorIn          ComparisonOperator = "in"
	ComparisonOperatorNin         ComparisonOperator = "nin"
	ComparisonOperatorContains    ComparisonOperator = "contains"
	ComparisonOperatorNotContains ComparisonOperator = "ncontains"
	ComparisonOperatorStartsWith  ComparisonOperator = "startswith"
	ComparisonOperatorEndsWith    ComparisonOperator = "endswith"
	ComparisonOperatorExists      ComparisonOperator = "exists"
	ComparisonOperatorNotExists   ComparisonOperator = "nexists"
)

// FilterValue represents the value used in a filter condition. It can be of any type,
// allowing for flexible query construction.
type FilterValue any

// FunctionCall represents a call to a function, which can be either a standard SQL
// function or a custom Go function registered with the query processor.
type FunctionCall struct {
	Function  FilterValue   // The name or identifier of the function.
	Arguments []FilterValue // The arguments to be passed to the function.
}

// FilterCondition defines a single condition for filtering the results of a query.
type FilterCondition struct {
	Field    string             // The field to apply the filter on.
	Operator ComparisonOperator // The comparison operator to use.
	Value    FilterValue        // The value to compare against.
}

// FilterGroup combines multiple filter conditions using a logical operator.
// This allows for the construction of complex, nested filter logic.
type FilterGroup struct {
	Operator   schema.LogicalOperator // The logical operator (AND, OR, etc.) to combine the conditions.
	Conditions []QueryFilter          // The list of conditions or nested groups.
}

// QueryFilter is a union type that can represent either a single filter condition
// or a group of conditions.
type QueryFilter struct {
	Condition *FilterCondition `json:",omitempty"` // A single filter condition.
	Group     *FilterGroup     `json:",omitempty"` // A group of filter conditions.
}

// SortDirection specifies the direction for sorting.
type SortDirection string

// Supported sort directions.
const (
	SortDirectionAsc  SortDirection = "asc"
	SortDirectionDesc SortDirection = "desc"
)

// SortConfiguration defines the sorting order for a specific field.
type SortConfiguration struct {
	Field     string        // The field to sort by.
	Direction SortDirection // The direction of the sort (ascending or descending).
}

// PaginationOptions defines how the query results should be paginated.
type PaginationOptions struct {
	Type   string  // The type of pagination, either "offset" or "cursor".
	Limit  int     // The maximum number of records to return.
	Offset *int    `json:",omitempty"` // The starting offset for offset-based pagination.
	Cursor *string `json:",omitempty"` // The cursor for cursor-based pagination.
}

// ProjectionField defines a field to be included or excluded in the query result.
type ProjectionField struct {
	Name   string                   // The name of the field.
	Nested *ProjectionConfiguration `json:",omitempty"` // For specifying projections on nested fields.
}

// ComputedFieldExpression defines a field that is computed at query time using a function.
type ComputedFieldExpression struct {
	Type       string        // The type of the expression, e.g., "computed".
	Expression *FunctionCall // The function call that computes the value.
	Alias      string        // The alias for the computed field in the result.
}

// CaseCondition represents a single WHEN/THEN clause in a CASE expression.
type CaseCondition struct {
	When QueryFilter // The condition to be met.
	Then FilterValue // The value to be returned if the condition is met.
}

// CaseExpression defines a conditional expression, similar to a SQL CASE statement.
type CaseExpression struct {
	Type  string          // The type of the expression, e.g., "case".
	Cases []CaseCondition // The list of WHEN/THEN conditions.
	Else  FilterValue     // The value to be returned if no conditions are met.
	Alias string          // The alias for the result of the case expression.
}

// ProjectionComputedItem is a union type that can be either a computed field or a case expression.
type ProjectionComputedItem struct {
	ComputedFieldExpression *ComputedFieldExpression `json:",omitempty"`
	CaseExpression          *CaseExpression          `json:",omitempty"`
}

// ProjectionConfiguration defines which fields should be returned in the query result.
type ProjectionConfiguration struct {
	Include  []ProjectionField        `json:",omitempty"` // A list of fields to include.
	Exclude  []ProjectionField        `json:",omitempty"` // A list of fields to exclude.
	Computed []ProjectionComputedItem `json:",omitempty"` // A list of computed fields.
}

// JoinType specifies the type of join to be performed.
type JoinType string

// Supported join types.
const (
	JoinTypeInner JoinType = "inner"
	JoinTypeLeft  JoinType = "left"
	JoinTypeRight JoinType = "right"
	JoinTypeFull  JoinType = "full"
)

// JoinConfiguration defines a join operation with another table.
type JoinConfiguration struct {
	Type        JoinType                 // The type of join.
	TargetTable string                   // The table to join with.
	On          QueryFilter              // The condition for the join.
	Alias       string                   // An alias for the joined table.
	Projection  *ProjectionConfiguration `json:",omitempty"` // The projection for the joined table.
}

// AggregationType specifies the type of aggregation to be performed.
type AggregationType string

// Supported aggregation types.
const (
	AggregationTypeCount AggregationType = "count"
	AggregationTypeSum   AggregationType = "sum"
	AggregationTypeAvg   AggregationType = "avg"
	AggregationTypeMin   AggregationType = "min"
	AggregationTypeMax   AggregationType = "max"
)

// AggregationConfiguration defines an aggregation operation to be performed on a field.
type AggregationConfiguration struct {
	Type  AggregationType // The type of aggregation.
	Field string          // The field to aggregate.
	Alias string          // An alias for the result of the aggregation.
}

// QueryHint provides a way to pass optimization hints to the database.
type QueryHint struct {
	Type    string `json:"type"`       // The type of hint (e.g., "index", "max_execution_time").
	Index   string `json:",omitempty"` // The name of the index to use, for index hints.
	Seconds int    `json:",omitempty"` // The maximum execution time in seconds.
}

// QueryDSL is the top-level structure that represents a complete database query.
// It combines all the different parts of a query, such as filters, sorting, and pagination.
type QueryDSL struct {
	Filters      *QueryFilter               `json:",omitempty"`
	Sort         []SortConfiguration        `json:",omitempty"`
	Pagination   *PaginationOptions         `json:",omitempty"`
	Projection   *ProjectionConfiguration   `json:",omitempty"`
	Joins        []JoinConfiguration        `json:",omitempty"`
	Aggregations []AggregationConfiguration `json:",omitempty"`
	Hints        []QueryHint                `json:",omitempty"`
}

// QueryResult represents the result of a database query.
type QueryResult struct {
	Data         any              `json:"data"`
	Count        int              `json:"count"`
	Pagination   *PaginationResult `json:",omitempty"`
	Aggregations map[string]any   `json:",omitempty"`
}

// PaginationResult contains the pagination information for a query result.
type PaginationResult struct {
	Total      *int    `json:",omitempty"`
	NextCursor *string `json:",omitempty"`
}

// standardComparisonOperators is a set of all the standard, built-in comparison operators.
var standardComparisonOperators = map[ComparisonOperator]struct{}{
	ComparisonOperatorEq:          {},
	ComparisonOperatorNeq:         {},
	ComparisonOperatorLt:          {},
	ComparisonOperatorLte:         {},
	ComparisonOperatorGt:          {},
	ComparisonOperatorGte:         {},
	ComparisonOperatorIn:          {},
	ComparisonOperatorNin:         {},
	ComparisonOperatorContains:    {},
	ComparisonOperatorNotContains: {},
	ComparisonOperatorStartsWith:  {},
	ComparisonOperatorEndsWith:    {},
	ComparisonOperatorExists:      {},
	ComparisonOperatorNotExists:   {},
}

// IsStandard checks if a comparison operator is one of the standard, built-in operators.
func (c ComparisonOperator) IsStandard() bool {
	_, ok := standardComparisonOperators[c]
	return ok
}

// GetStandardComparisonOperators returns a map of all standard comparison operators.
// This is useful for validation and for registering the standard operators.
func GetStandardComparisonOperators() map[ComparisonOperator]struct{} {
	return standardComparisonOperators
}
