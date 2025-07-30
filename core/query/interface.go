// Package query defines the interfaces for generating database-specific queries
// from the abstract Query.
package query

import (
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// QueryGeneratorFactory defines the interface for a factory that creates QueryGenerator instances.
// This allows for the creation of query generators that are specific to a given database schema.
type QueryGeneratorFactory interface {
	// CreateGenerator creates a new QueryGenerator for a specific schema.
	CreateGenerator(schema *schema.SchemaDefinition) (QueryGenerator, error)
}

// QueryGenerator defines the interface for generating database-specific query strings
// from a generic Query object. Each implementation of this interface will be
// responsible for translating the abstract query representation into a concrete
// SQL dialect (e.g., PostgreSQL, SQLite, MySQL).
type QueryGenerator interface {
	// GenerateSelectSQL creates a SQL SELECT query string and its corresponding parameters
	// from a Query object. This includes translating filters, sorting, pagination,
	// and projections into the target SQL dialect.
	GenerateSelectSQL(dsl *Query) (string, []any, error)

	// GenerateUpdateSQL creates a SQL UPDATE query string and its parameters from a map
	// of updates and a QueryFilter. It is responsible for constructing the SET and
	// WHERE clauses of the update statement.
	GenerateUpdateSQL(updates map[string]any, filters *QueryFilter) (string, []any, error)

	// GenerateInsertSQL creates a SQL INSERT query string and its parameters from a slice
	// of records. It supports both single and batch inserts, generating the appropriate
	// syntax for the target database.
	GenerateInsertSQL(records []map[string]any) (string, []any, error)

	// GenerateDeleteSQL creates a SQL DELETE query string and its parameters from a
	// QueryFilter. For safety, it requires a WHERE clause unless the `unsafeDelete`
	// flag is explicitly set to true.
	GenerateDeleteSQL(filters *QueryFilter, unsafeDelete bool) (string, []any, error)
}

// QueryBuilder defines the interface for building database queries.
// It provides a fluent API to construct complex Query structures.
type QueryBuilderInterface interface {
	// Core Operations
	Build() Query
	Clone() *QueryBuilder
	Reset() *QueryBuilder
	Validate() QueryValidationResult
	String() string

	// Filtering
	Where(field string) FilterConditionBuilderInterface
	WhereGroup(operator LogicalOperator) FilterGroupBuilderInterface
	TextSearch(field string) TextSearchBuilderInterface // Proposed addition

	// Concatenation of Filters
	ApplyFilter(newFilter QueryFilter) *QueryBuilder // Combines with existing filters (default AND)
	AndFilter(newFilter QueryFilter) *QueryBuilder   // Explicitly combines with existing filters using AND
	OrFilter(newFilter QueryFilter) *QueryBuilder    // Explicitly combines with existing filters using OR

	// Sorting
	OrderBy(field string, direction SortDirection) *QueryBuilder
	OrderByAsc(field string) *QueryBuilder
	OrderByDesc(field string) *QueryBuilder
	ThenSortBy(field string, direction SortDirection) *QueryBuilder // Appends additional sort
	ThenSortByAsc(field string) *QueryBuilder                       // Appends additional ascending sort
	ThenSortByDesc(field string) *QueryBuilder                      // Appends additional descending sort

	// Pagination
	Limit(limit int) *QueryBuilder
	Offset(offset int) *QueryBuilder
	

	// Projection
	Select() ProjectionBuilderInterface

	// Joins
	Join(joinType JoinType, target string) JoinBuilderInterface
	InnerJoin(target string) JoinBuilderInterface
	LeftJoin(target string) JoinBuilderInterface
	RightJoin(target string) JoinBuilderInterface
	FullJoin(target string) JoinBuilderInterface

	// Aggregations
	Aggregate(aggType AggregationType, field string, alias string) *QueryBuilder
	Count(field string, alias string) *QueryBuilder
	Sum(field string, alias string) *QueryBuilder
	Avg(field string, alias string) *QueryBuilder
	Min(field string, alias string) *QueryBuilder
	Max(field string, alias string) *QueryBuilder
	GroupBy(groupField string) AggregationBuilderInterface

	// Distinct
	Distinct() *QueryBuilder
	DistinctBy(fields ...string) *QueryBuilder

	// Union Operations
	Union(otherQuery Query) *QueryBuilder
	UnionAll(otherQuery Query) *QueryBuilder
	Intersect(otherQuery Query) *QueryBuilder
	Except(otherQuery Query) *QueryBuilder

	// Query Hints
	AddHint(hintType string) *QueryBuilder
	UseIndex(index string) *QueryBuilder
	ForceIndex(index string) *QueryBuilder
	NoIndex(index string) *QueryBuilder
	MaxExecutionTime(seconds int) *QueryBuilder
}

// FilterConditionBuilderInterface defines the interface for building a single filter condition.
type FilterConditionBuilderInterface interface {
	Eq(value any) *QueryBuilder
	Neq(value any) *QueryBuilder
	Lt(value any) *QueryBuilder
	Lte(value any) *QueryBuilder
	Gt(value any) *QueryBuilder
	Gte(value any) *QueryBuilder
	In(values ...any) *QueryBuilder
	Nin(values ...any) *QueryBuilder
	Contains(value any) *QueryBuilder
	NotContains(value any) *QueryBuilder
	Exists() *QueryBuilder
	NotExists() *QueryBuilder
	Custom(operator ComparisonOperator, value any) *QueryBuilder
}

// FilterGroupBuilderInterface defines the interface for building a group of filter conditions.
type FilterGroupBuilderInterface interface {
	Where(field string) FilterConditionBuilderInGroupInterface
	WhereGroup(operator LogicalOperator) FilterGroupBuilderInterface
	Group(filter QueryFilter) *FilterGroupBuilder
	End() *QueryBuilder
	WhereTextSearch(field string) TextSearchBuilderInGroupInterface // Proposed addition to nested groups
}

// FilterConditionBuilderInGroupInterface defines the interface for building a single filter condition within a group.
type FilterConditionBuilderInGroupInterface interface {
	Eq(value any) *FilterGroupBuilder
	Neq(value any) *FilterGroupBuilder
	Lt(value any) *FilterGroupBuilder
	Lte(value any) *FilterGroupBuilder
	Gt(value any) *FilterGroupBuilder
	Gte(value any) *FilterGroupBuilder
	In(values ...any) *FilterGroupBuilder
	Nin(values ...any) *FilterGroupBuilder
	Contains(value any) *FilterGroupBuilder
	NotContains(value any) *FilterGroupBuilder
	Exists() *FilterGroupBuilder
	NotExists() *FilterGroupBuilder
	Custom(operator ComparisonOperator, value any) *FilterGroupBuilder
}

// TextSearchBuilderInterface defines the interface for building text search conditions. (Proposed addition)
type TextSearchBuilderInterface interface {
	Contains(query string) *QueryBuilder
	Exact(query string) *QueryBuilder
	Phrase(query string) *QueryBuilder
}

// TextSearchBuilderInGroupInterface defines the interface for building text search conditions within a group. (Proposed addition)
type TextSearchBuilderInGroupInterface interface {
	Contains(query string) *FilterGroupBuilder
	Exact(query string) *FilterGroupBuilder
	Phrase(query string) *FilterGroupBuilder
}

// ProjectionBuilderInterface defines the interface for building the projection part of a query.
type ProjectionBuilderInterface interface {
	Include(fields ...string) *ProjectionBuilder
	IncludeNested(field string, nestedConfig *ProjectionConfiguration) *ProjectionBuilder
	Exclude(fields ...string) *ProjectionBuilder
	AddComputed(alias string, functionName string, args ...any) *ProjectionBuilder
	AddCase(alias string) CaseExpressionBuilderInterface
	End() *QueryBuilder
}

// CaseExpressionBuilderInterface defines the interface for building a case expression for a computed field.
type CaseExpressionBuilderInterface interface {
	When(filter QueryFilter, then any) *CaseExpressionBuilder
	Else(value any) *CaseExpressionBuilder
	End() *ProjectionBuilder
}

// JoinBuilderInterface defines the interface for building a join configuration.
type JoinBuilderInterface interface {
	On(filter QueryFilter) *JoinBuilder
	Alias(alias string) *JoinBuilder
	WithProjection(projection *ProjectionConfiguration) *JoinBuilder
	End() *QueryBuilder
}

// AggregationBuilderInterface defines the interface for building aggregation configurations.
type AggregationBuilderInterface interface {
	Type(aggType AggregationType) *AggregationBuilder
	Field(field string) *AggregationBuilder
	Alias(alias string) *AggregationBuilder
	AddGroup(groupField string) *AggregationBuilder
	WithFilter(filter QueryFilter) *AggregationBuilder
	End() *QueryBuilder
}

// SortingCapabilities defines the specific sorting features supported by a database.
type SortingCapabilities struct {
	// SupportsNullsOrdering indicates if the database can handle NULLS FIRST / NULLS LAST syntax.
	SupportsNullsOrdering bool
	// SupportsExpression indicates if the database can sort by the result of an expression (e.g., ORDER BY LOWER(name)).
	SupportsExpression bool
}

// FunctionCapabilities defines where and how functions can be used.
type FunctionCapabilities struct {
	// AllowedInFilters indicates if the function can be used in WHERE clauses.
	AllowedInFilters bool
	// AllowedInProjections indicates if the function can be used in SELECT clauses (computed fields).
	AllowedInProjections bool
	// AllowedInSort indicates if the function can be used in ORDER BY clauses.
	AllowedInSort bool
}

// Capabilities defines the features and limitations of a database backend.
// This struct is used by the QueryPartitioner to split a QueryDSL query
// into a database query and a post-processing query.
type Capabilities struct {
	// SupportedLogicalOperators is a set of logical operators (AND, OR, NOT) that the database can handle in filter expressions.
	SupportedLogicalOperators map[LogicalOperator]struct{}
	// SupportedComparisonOperators is a set of comparison operators (e.g., Eq, Gt, Lt) that the database can handle natively.
	SupportedComparisonOperators map[ComparisonOperator]struct{}
	// SupportedExpressionOperators is a set of operators for computed fields or filters (e.g., MULTIPLY, ADD).
	// This allows translation of an abstract function like `MULTIPLY(col, 2)` to `col * 2`.
	SupportedExpressionOperators map[string]struct{}
	// SupportedFunctions is a map of functions the database can execute, detailing their allowed contexts.
	SupportedFunctions map[string]FunctionCapabilities
	// SupportedJoinTypes is a set of JOIN types (e.g., INNER, LEFT) that the database supports.
	// If empty, it implies joins are not supported.
	SupportedJoinTypes map[JoinType]struct{}
	// SupportedAggregationFunctions is a set of aggregation functions (e.g., COUNT, SUM) that the database supports.
	// If empty, it implies aggregate functions are not supported.
	SupportedAggregationFunctions map[AggregationType]struct{}
	// SupportedPaginationTypes is a set of pagination methods (e.g., OFFSET, CURSOR) supported by the database.
	SupportedPaginationTypes map[PaginationType]struct{}
	// SupportedTextSearchTypes is a set of text search types (e.g., CONTAINS, EXACT) that the database supports.
	SupportedTextSearchTypes map[TextSearchType]struct{}
	// Sorting details the database's sorting capabilities.
	Sorting SortingCapabilities
	// SupportsGroupBy indicates whether the database can perform GROUP BY operations.
	SupportsGroupBy bool
	// SupportsDistinct indicates whether the database can perform DISTINCT operations.
	SupportsDistinct bool
	// SupportsNestedFields indicates whether the database can query nested document structures.
	SupportsNestedFields bool
	// MaxWhereConditions specifies the maximum number of WHERE conditions allowed in a single query. 0 means no limit.
	MaxWhereConditions int
	// MaxJoinClauses specifies the maximum number of JOIN clauses allowed in a single query. 0 means no limit.
	MaxJoinClauses int
}

// QueryPartitionerInterface defines the interface for splitting a query.
type QueryPartitionerInterface interface {
	Partition(dsl *Query) (dbQuery *Query, postProcessingQuery *Query, err error)
}

// ComputeFunction is a function that computes a new value for a row of data.
// It takes a document (representing a single row) and a set of arguments, and
// returns the computed value.
type ComputeFunction func(row schema.Document, args []FilterValue) (any, error)

// PredicateFunction is a function that performs custom filtering logic on a row.
// It returns true if the row should be included in the result set, and false otherwise.
type PredicateFunction func(doc schema.Document, field string, value FilterValue) (bool, error)

// PartitionedQuery holds the result of a query partitioning operation.
type PartitionedQuery struct {
	DbQuery             *Query
	PostProcessingQuery *Query
}

// QueryCache defines the interface for a cache that stores partitioned queries.
type QueryCache interface {
	Get(key uint64) (*PartitionedQuery, bool)
	Set(key uint64, value *PartitionedQuery)
}

