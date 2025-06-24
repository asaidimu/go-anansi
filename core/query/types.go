package query

// Ensure these match your actual type definitions from the DSL.
// For example, if you have these in a 'querydsl.go' or 'types.go' file.

// LogicalOperator for combining conditions.
type LogicalOperator string

const (
	LogicalOperatorAnd LogicalOperator = "and"
	LogicalOperatorOr  LogicalOperator = "or"
	LogicalOperatorNot LogicalOperator = "not"
	LogicalOperatorNor LogicalOperator = "nor"
	LogicalOperatorXor LogicalOperator = "xor"
)

// ComparisonOperator for filtering.
type ComparisonOperator string

const (
	ComparisonOperatorEq       ComparisonOperator = "eq"
	ComparisonOperatorNeq      ComparisonOperator = "neq"
	ComparisonOperatorLt       ComparisonOperator = "lt"
	ComparisonOperatorLte      ComparisonOperator = "lte"
	ComparisonOperatorGt       ComparisonOperator = "gt"
	ComparisonOperatorGte      ComparisonOperator = "gte"
	ComparisonOperatorIn       ComparisonOperator = "in"
	ComparisonOperatorNin      ComparisonOperator = "nin"
	ComparisonOperatorContains ComparisonOperator = "contains"

	// Deprecated: use ComparisonOperatorNotContains instead
	ComparisonOperatorNContains   ComparisonOperator = "ncontains"
	ComparisonOperatorNotContains ComparisonOperator = "ncontains"
	ComparisonOperatorStartsWith  ComparisonOperator = "startswith"
	ComparisonOperatorEndsWith    ComparisonOperator = "endswith"
	ComparisonOperatorExists      ComparisonOperator = "exists"
	// Deprecated: use ComparisonOperatorNotExists instead
	ComparisonOperatorNExists   ComparisonOperator = "nexists"
	ComparisonOperatorNotExists ComparisonOperator = "nexists"
)

// FilterValue represents the supported data types for filter values.
type FilterValue any // Can be string, number, bool, []any

// FunctionCall represents a call to a function (either SQL or Go-based).
type FunctionCall struct {
	Function  FilterValue   // The name/identifier of the function (e.g., "DATE_SUB", "full_name")
	Arguments []FilterValue // Arguments passed to the function
}

// FilterCondition defines a single filtering condition.
type FilterCondition struct {
	Field    string             // The field to filter on
	Operator ComparisonOperator // The comparison operator (e.g., "eq", "gt", "is_adult")
	Value    FilterValue        // The value to compare against
}

// FilterGroup combines multiple conditions with a logical operator.
type FilterGroup struct {
	Operator   LogicalOperator // "and", "or", "not", "nor", "xor"
	Conditions []QueryFilter   // Nested filter conditions or groups
}

// QueryFilter represents a filter condition or a group of conditions.
type QueryFilter struct {
	Condition *FilterCondition `json:",omitempty"` // Single condition
	Group     *FilterGroup     `json:",omitempty"` // Group of conditions
}

// SortDirection for sorting order.
type SortDirection string

const (
	SortDirectionAsc  SortDirection = "asc"
	SortDirectionDesc SortDirection = "desc"
)

// SortConfiguration defines sorting for a field.
type SortConfiguration struct {
	Field     string        // The field to sort by
	Direction SortDirection // "asc" or "desc"
}

// PaginationOptions for controlling query results.
type PaginationOptions struct {
	Type   string  // "offset" or "cursor"
	Limit  int     // Maximum number of records to return
	Offset *int    `json:",omitempty"` // For offset-based pagination
	Cursor *string `json:",omitempty"` // For cursor-based pagination
	// Additional fields for cursor-based pagination (e.g., fields, directions)
	// would go here if you expand it.
}

// ProjectionField defines a field to include/exclude in the projection.
type ProjectionField struct {
	Name   string                   // The name of the field
	Nested *ProjectionConfiguration `json:",omitempty"` // For nested projections
}

// ComputedFieldExpression defines a computed field based on a function call.
type ComputedFieldExpression struct {
	Type       string        // e.g., "computed"
	Expression *FunctionCall // The function call that computes the value
	Alias      string        // The alias for the computed field in the result
}

// CaseCondition for conditional expressions.
type CaseCondition struct {
	When QueryFilter // The condition for this case
	Then FilterValue // The value if the condition is true
}

// CaseExpression defines a SQL CASE expression, translated to Go logic if needed.
type CaseExpression struct {
	Type  string          // e.g., "case"
	Cases []CaseCondition // List of WHEN/THEN pairs
	Else  FilterValue     // The ELSE value if no conditions are met
	Alias string          // Alias for the case expression result
}

// ProjectionComputedItem can be either a ComputedFieldExpression or a CaseExpression.
type ProjectionComputedItem struct {
	ComputedFieldExpression *ComputedFieldExpression `json:",omitempty"`
	CaseExpression          *CaseExpression          `json:",omitempty"`
}

// ProjectionConfiguration defines which fields to include/exclude and computed fields.
type ProjectionConfiguration struct {
	Include  []ProjectionField        `json:",omitempty"` // Fields to include
	Exclude  []ProjectionField        `json:",omitempty"` // Fields to exclude
	Computed []ProjectionComputedItem `json:",omitempty"` // Computed fields
}

// JoinType for join operations.
type JoinType string

const (
	JoinTypeInner JoinType = "inner"
	JoinTypeLeft  JoinType = "left"
	JoinTypeRight JoinType = "right"
	JoinTypeFull  JoinType = "full"
)

// JoinConfiguration defines a join operation.
type JoinConfiguration struct {
	Type        JoinType                 // "inner", "left", etc.
	TargetTable string                   // The table to join with
	On          QueryFilter              // Join condition
	Alias       string                   // Alias for the joined table
	Projection  *ProjectionConfiguration `json:",omitempty"` // Projection for the joined table
}

// AggregationType for aggregation functions.
type AggregationType string

const (
	AggregationTypeCount AggregationType = "count"
	AggregationTypeSum   AggregationType = "sum"
	AggregationTypeAvg   AggregationType = "avg"
	AggregationTypeMin   AggregationType = "min"
	AggregationTypeMax   AggregationType = "max"
)

// AggregationConfiguration defines an aggregation operation.
type AggregationConfiguration struct {
	Type  AggregationType // "count", "sum", "avg", etc.
	Field string          // The field to aggregate
	Alias string          // Alias for the aggregation result
}

// QueryHint for optimization.
type QueryHint struct {
	Type    string `json:"type"`       // e.g., "index", "force_index", "no_index", "max_execution_time"
	Index   string `json:",omitempty"` // For index hints
	Seconds int    `json:",omitempty"` // For max_execution_time
}

// QueryDSL is the main Query DSL structure.
type QueryDSL struct {
	Filters      *QueryFilter               `json:",omitempty"`
	Sort         []SortConfiguration        `json:",omitempty"`
	Pagination   *PaginationOptions         `json:",omitempty"`
	Projection   *ProjectionConfiguration   `json:",omitempty"`
	Joins        []JoinConfiguration        `json:",omitempty"`
	Aggregations []AggregationConfiguration `json:",omitempty"`
	Hints        []QueryHint                `json:",omitempty"`
}

// QueryResult structure.
type QueryResult struct {
	Data       any `json:"data"`
	Count      int `json:"count"`
	Pagination *struct {
		Total      *int    `json:",omitempty"`
		NextCursor *string `json:",omitempty"`
	} `json:",omitempty"`
	Aggregations map[string]any `json:",omitempty"`
}

// Add a helper function to core or as a method on ComparisonOperator
// to distinguish standard vs. custom operators.
// For this example, let's just make a simple map for demonstration.
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

func (c ComparisonOperator) IsStandard() bool {
	_, ok := standardComparisonOperators[c]
	return ok
}

// GetStandardComparisonOperators returns a map of all standard comparison operators.
// This might be useful for external checks or initializations.
func GetStandardComparisonOperators() map[ComparisonOperator]struct{} {
	return standardComparisonOperators
}
