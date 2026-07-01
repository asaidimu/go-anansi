// Domain-Specific Language (DSL) is for constructing
// database queries. This DSL provides a structured and type-safe way to express
// complex query logic, including filtering, sorting, pagination, and more.
package query

import (
	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

// Logical operators for combining filter conditions.

// ComparisonOperator defines the set of operators that can be used in a filter condition.
type ComparisonOperator string

// ComparisonMap defines a map where keys are operator names (strings) and values
// are functions that perform a comparison between two values, returning a boolean result and an error.
type ComparisonFunction func(left, right any) (bool, error)
type ComparisonMap map[string]ComparisonFunction

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
	ComparisonOperatorExists      ComparisonOperator = "exists"
	ComparisonOperatorNotExists   ComparisonOperator = "nexists"
)

// MatchCountName defines the alias used for the window function that calculates
// the total number of rows matching the query criteria, ignoring LIMIT and OFFSET clauses.
// This is used primarily for UI pagination (e.g., "Showing 10 of 500 matches").
const MatchCountName string = "__matches__"

// TextSearchType defines the type of full-text search to be performed.
type TextSearchType string

// Supported text search types.
const (
	TextSearchTypeContains TextSearchType = "contains"
	TextSearchTypeExact    TextSearchType = "exact"
	TextSearchTypePhrase   TextSearchType = "phrase"
)

// TextOperator defines how multiple terms in a text search query should be combined.
type TextOperator string

// Supported text operators.
const (
	TextOperatorAnd TextOperator = "and" // All terms must match
	TextOperatorOr  TextOperator = "or"  // Any term can match
)

type FieldReference struct {
	Field string `json:"field"` // The field to reference
	Type  string `json:"type"`  // Should be "field"
}

type SubqueryValue struct {
	Type  string `json:"type"` // Should be "subquery"
	Query Query  `json:"query"`
}

// FunctionCall represents a call to a function, which can be either a standard SQL
// function or a custom Go function registered with the query processor.
type FunctionCall struct {
	Function  string        `json:"function"`  // The name or identifier of the function.
	Arguments []FilterValue `json:"arguments"` // The arguments to be passed to the function.
}

// Define a type for general function executors for computed fields/function calls
type FunctionExecutor func(args ...any) (any, error)
type FunctionMap map[string]FunctionExecutor

// FilterValue represents a union type for values used in filter conditions.
// It can hold a primitive value (string, number, boolean, object),
// an array of FilterValues, a FieldReference, a SubqueryValue, or a FunctionCall.
type FilterValue struct {
	// Pointers to hold one of the possible values.
	// Using pointers allows us to distinguish between a zero value and a missing value,
	// and enables omitempty behavior when marshalling.
	StringVal       *string         `json:"string_value,omitempty"`
	NumberVal       *float64        `json:"number_value,omitempty"`
	BoolVal         *bool           `json:"bool_value,omitempty"`
	ObjectVal       map[string]any  `json:"object_value,omitempty"`
	ArrayVal        []FilterValue   `json:"array_value,omitempty"`
	FieldRefVal     *FieldReference `json:"field_reference_value,omitempty"`
	SubqueryVal     *SubqueryValue  `json:"subquery_value,omitempty"`
	FunctionCallVal *FunctionCall   `json:"function_call_value,omitempty"`
}

// FilterCondition defines a single condition for filtering the results of a query.
type FilterCondition struct {
	Field    string             `json:"field"`
	Operator ComparisonOperator `json:"operator"`
	Value    FilterValue        `json:"value"`
}

// FilterGroup combines multiple filter conditions using a logical operator.
type FilterGroup struct {
	Operator   common.LogicalOperator `json:"operator"`
	Conditions []QueryFilter          `json:"conditions"`
}

// TextSearchQuery defines a full-text search query.
type TextSearchQuery struct {
	Query         string         `json:"query"`
	Fields        []string       `json:"fields,omitempty"`
	Type          TextSearchType `json:"type,omitempty"`
	Operator      TextOperator   `json:"operator,omitempty"`
	CaseSensitive *bool          `json:"case_sensitive,omitempty"`
}

// QueryFilter is a union type that can represent either a single filter condition,
// a group of conditions, or a full-text search query.
type QueryFilter struct {
	Condition       *FilterCondition `json:"condition,omitempty"`
	Group           *FilterGroup     `json:"group,omitempty"`
	TextSearchQuery *TextSearchQuery `json:"text_search_query,omitempty"`
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
	Field     string        `json:"field"`
	Direction SortDirection `json:"direction"`
}

type PaginationType string

const (
	PaginationTypeOffset PaginationType = "offset"
	PaginationTypeCursor PaginationType = "cursor"
)

type PaginationCursor struct {
	Field  *string      `json:"field"`
	Cursor *FilterValue `json:"cursor"`
}

// PaginationOptions defines how the query results should be paginated.
type PaginationOptions struct {
	Type         PaginationType      `json:"type"`             // The type of pagination.
	Limit        int                `json:"limit,omitempty"`  // The maximum number of records to return.
	Offset       *int                `json:"offset,omitempty"` // The starting offset for offset-based pagination.
	Cursor       *PaginationCursor   `json:"cursor,omitempty"` // The cursor for cursor-based pagination, if nil starts with the first entry
	Order        []SortConfiguration `json:"order,omitempty"`  // The fields to order by, defaults to id
	IncludeTotal *bool                `json:"include_total"`
}

// ProjectionField defines a field to be included or excluded in the query result.
type ProjectionField struct {
	Name   string                   `json:"name"`
	Alias  *string                  `json:"alias,omitempty"`
	Nested *ProjectionConfiguration `json:"nested,omitempty"`
}

// ComputedFieldExpression defines a field that is computed at query time using a function.
type ComputedFieldExpression struct {
	Type       string        `json:"type"`
	Expression *FunctionCall `json:"expression"`
	Alias      string        `json:"alias"`
}

// CaseCondition represents a single WHEN/THEN clause in a CASE expression.
type CaseCondition struct {
	When QueryFilter `json:"when"`
	Then FilterValue `json:"then"`
}

// CaseExpression defines a conditional expression, similar to a SQL CASE statement.
type CaseExpression struct {
	Type       string          `json:"type"`
	Conditions []CaseCondition `json:"conditions"`
	Else       FilterValue     `json:"else"`
	Alias      string          `json:"alias"`
}

// ProjectionComputedItem is a union type that can be either a computed field or a case expression.
type ProjectionComputedItem struct {
	ComputedFieldExpression *ComputedFieldExpression `json:"computed_field_expression,omitempty"`
	CaseExpression          *CaseExpression          `json:"case_expression,omitempty"`
}

// ProjectionConfiguration defines which fields should be returned in the query result.
type ProjectionConfiguration struct {
	Include  []ProjectionField        `json:"include,omitempty"`
	Exclude  []ProjectionField        `json:"exclude,omitempty"`
	Computed []ProjectionComputedItem `json:"computed,omitempty"`
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

type QueryTarget struct {
	Name   string             `json:"name,omitempty"`
	Alias  *string            `json:"alias,omitempty"`
	Schema *definition.Schema `json:"schema,omitempty"`
}

// JoinConfiguration defines a join operation with another table.
type JoinConfiguration struct {
	Type       JoinType                 `json:"type"`
	Target     QueryTarget              `json:"target"`
	On         *QueryFilter             `json:"on"`
	Projection *ProjectionConfiguration `json:"projection,omitempty"`
}

// AggregationType specifies the type of aggregation to be performed.
type AggregationType string

type AggregateFunction func(records []map[string]any, field string) (any, error)

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
	Type   AggregationType `json:"type"`
	Field  string          `json:"field"`
	Alias  *string         `json:"alias,omitempty"`  // Change to pointer and omitempty
	Groups []string        `json:"groups,omitempty"` // Add this field
	Filter *QueryFilter    `json:"filter,omitempty"` // Add this field
}

// QueryHint provides a way to pass optimization hints to the database.
type QueryHint map[string]any

// QueryUnion defines a union operation between multiple queries.
type QueryUnion struct {
	Queries []Query `json:"queries"`
	Type    string  `json:"type"` // Corresponds to "union" | "all" | "intersect" | "except"
}

// QueryDistinctConfig represents the distinct configuration for a query.
// It can be a boolean (true for distinct all) or an object specifying distinct fields.
type QueryDistinctConfig struct {
	// IsDistinct represents the boolean distinct option (e.g., `distinct: true`).
	// It should be non-nil only when the distinct setting is a boolean.
	IsDistinct *bool `json:"is_distinct,omitempty"` // Renamed from "boolean" for clarity
	// Fields represents the distinct by fields option (e.g., `distinct: { fields: ["id", "name"] }`).
	// It should be non-nil only when the distinct setting is an object with a 'fields' array.
	Fields []string `json:"fields,omitempty"`
}

// RawQueryTarget specifies a collection and its schema version
// to be used in a raw query.
type RawQueryTarget struct {
	Collection string `json:"collection"`
	Version    string `json:"version,omitempty"` // If omitted, resolves to the active version
}

// RawQuery represents a raw query payload for execution.
// It combines a string-based query template with structured options.
type RawQuery struct {
	// Template is a string-based query.
	// For SQL, this is the SQL string.
	// For MongoDB, this could be a JSON string representing the filter,
	// an aggregation pipeline, or a command body.
	// Placeholders like {{collection.users}} are resolved here using the 'Collections' map.
	Template string `json:"template,omitempty"`

	// Options provides structured, database-specific parameters that modify the query's behavior.
	// For SQL, this might be hints, expectRows boolean for schema inclusion.
	// For MongoDB, this could be limit, sort, projection, includeSchema, etc.
	// This is a map to allow flexible, database-specific key-value pairs.
	Options map[string]any `json:"options,omitempty"`

	// Maps placeholder names in the 'Template' string to specific collection targets.
	// This field is only relevant when 'Template' is used.
	// TODO Review whether this should be a slice or a map
	Collections map[string]RawQueryTarget `json:"from,omitempty"`

	// Parameters are parameters for the query. Primarily for SQL.
	// For MongoDB, if Template is a JSON string, Parameters could map to custom placeholders
	// within that JSON string (e.g., {{arg0}}, {{arg1}}).
	Parameters []any `json:"args,omitempty"`
}

// Query represents a query to be executed against a collection.
// It contains fields for filtering, projecting, sorting, and limiting results.
type Query struct {
	Target       *QueryTarget               `json:"target,omitempty"`
	Filters      *QueryFilter               `json:"filters,omitempty"`
	Projection   *ProjectionConfiguration   `json:"projection,omitempty"`
	Sort         []SortConfiguration        `json:"sort,omitempty"`
	Limit        *int                       `json:"limit,omitempty"`
	Pagination   *PaginationOptions         `json:"pagination,omitempty"`
	Joins        []JoinConfiguration        `json:"joins,omitempty"`
	Distinct     *QueryDistinctConfig       `json:"distinct,omitempty"`
	Aggregations []AggregationConfiguration `json:"aggregations,omitempty"`
	Union        *QueryUnion                `json:"union,omitempty"`
	Hints        []QueryHint                `json:"hints,omitempty"`

	// New raw query field - takes precedence over DSL fields when present
	Raw *RawQuery `json:"raw,omitempty"`
}

// IsEmpty checks if the query is empty (has no operations defined).
func (q *Query) IsEmpty() bool {
	return q.Filters == nil &&
		len(q.Sort) == 0 &&
		q.Pagination == nil &&
		q.Projection == nil &&
		len(q.Joins) == 0 &&
		q.Distinct == nil &&
		len(q.Aggregations) == 0 &&
		q.Union == nil &&
		len(q.Hints) == 0
}

// QueryResult represents the result of a database query.
// TODO. Why am I not using this struct?
type QueryResult struct {
	Data           []map[string]any `json:"data"`
	Count          int              `json:"count,omitempty"`
	Total          *int             `json:"total,omitempty"`
	PaginationInfo *PaginationInfo  `json:"pagination_info,omitempty"`
	SearchScore    *float64         `json:"search_score,omitempty"`
}

// PaginationResult contains the pagination information for a query result.
type PaginationResult struct {
	Total *int `json:"total,omitempty"` // Keep Total as it exists in TS, though deprecated
}

// PaginationInfo provides comprehensive pagination metadata for a query result.
type PaginationInfo struct {
	Number int `json:"number"` // 1-based page number
	Size   int `json:"size"`   // Maximum items per page (limit)
	Count  int `json:"count"`  // Items in the current page
	Total  int `json:"total"`  // Total items across all pages
	Pages  int `json:"pages"`  // Total number of pages
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
	ComparisonOperatorExists:      {},
	ComparisonOperatorNotExists:   {},
}

// standardTextSearchTypes is a set of all the standard, built-in text search types.
var standardTextSearchTypes = map[TextSearchType]struct{}{
	TextSearchTypeContains: {}, // Changed from TextSearchTypeMatch
	TextSearchTypeExact:    {}, // Added
	TextSearchTypePhrase:   {},
}

// IsStandard checks if a comparison operator is one of the standard, built-in operators.
func (c ComparisonOperator) IsStandard() bool {
	_, ok := standardComparisonOperators[c]
	return ok
}

// IsStandard checks if a text search type is one of the standard, built-in types.
func (t TextSearchType) IsStandard() bool {
	_, ok := standardTextSearchTypes[t]
	return ok
}

// GetStandardComparisonOperators returns a map of all standard comparison operators.
// This is useful for validation and for registering the standard operators.
func GetStandardComparisonOperators() map[ComparisonOperator]struct{} {
	return standardComparisonOperators
}

// GetStandardTextSearchTypes returns a map of all standard text search types.
// This is useful for validation and for registering the standard text search types.
func GetStandardTextSearchTypes() map[TextSearchType]struct{} {
	return standardTextSearchTypes
}
