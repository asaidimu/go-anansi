// Package query defines the interfaces for generating database-specific queries
// from the abstract QueryDSL.
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
// from a generic QueryDSL object. Each implementation of this interface will be
// responsible for translating the abstract query representation into a concrete
// SQL dialect (e.g., PostgreSQL, SQLite, MySQL).
type QueryGenerator interface {
	// GenerateSelectSQL creates a SQL SELECT query string and its corresponding parameters
	// from a QueryDSL object. This includes translating filters, sorting, pagination,
	// and projections into the target SQL dialect.
	GenerateSelectSQL(dsl *QueryDSL) (string, []any, error)

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
// It provides a fluent API to construct complex QueryDSL structures.
type QueryBuilderInterface interface {
	// Core Operations
	Build() QueryDSL
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
	Cursor(cursor string) *QueryBuilder

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
	Union(otherQuery QueryDSL) *QueryBuilder
	UnionAll(otherQuery QueryDSL) *QueryBuilder
	Intersect(otherQuery QueryDSL) *QueryBuilder
	Except(otherQuery QueryDSL) *QueryBuilder

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
