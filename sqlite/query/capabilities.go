package query

import (
	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/query"
)

// Capabilities returns the capabilities of the SQLite interactor.
func (i *sqliteFactory) Capabilities() query.Capabilities {
	return query.Capabilities{
		SchemaEvolution: query.SchemaEvolution{
			AddColumn:       true,  // ALTER TABLE ... ADD COLUMN
			DropColumn:      true,  // ALTER TABLE ... DROP COLUMN (SQLite 3.35.0+)
			RenameColumn:    false, // requires table recreation
			AlterColumnType: false, // requires table recreation
			AddConstraint:   false, // very limited after creation
			DropConstraint:  false, // not supported
		},
		RequiresTransactionSerialization: true,
		SupportedLogicalOperators: map[common.LogicalOperator]struct{}{
			common.LogicalAnd: {},
			common.LogicalOr:  {},
		},
		SupportedComparisonOperators: map[query.ComparisonOperator]struct{}{
			query.ComparisonOperatorEq:          {},
			query.ComparisonOperatorNeq:         {},
			query.ComparisonOperatorLt:          {},
			query.ComparisonOperatorLte:         {},
			query.ComparisonOperatorGt:          {},
			query.ComparisonOperatorGte:         {},
			query.ComparisonOperatorIn:          {},
			query.ComparisonOperatorNin:         {},
			query.ComparisonOperatorContains:    {},
			query.ComparisonOperatorNotContains: {},
			query.ComparisonOperatorExists:      {},
			query.ComparisonOperatorNotExists:   {},
		},
		SupportedExpressionOperators: map[string]struct{}{
			// SQLite supports basic arithmetic operators
			"ADD":      {},
			"SUBTRACT": {},
			"MULTIPLY": {},
			"DIVIDE":   {},
		},
		SupportedFunctions: map[string]query.FunctionCapabilities{
			// JSON functions
			"json_extract": {
				AllowedInFilters:    true,
				AllowedInProjections: true,
				AllowedInSort:       true,
			},
			"json_valid": {
				AllowedInFilters:    true,
				AllowedInProjections: true,
				AllowedInSort:       false,
			},
			// String functions
			"UPPER": {
				AllowedInFilters:    true,
				AllowedInProjections: true,
				AllowedInSort:       true,
			},
			"LOWER": {
				AllowedInFilters:    true,
				AllowedInProjections: true,
				AllowedInSort:       true,
			},
			"LENGTH": {
				AllowedInFilters:    true,
				AllowedInProjections: true,
				AllowedInSort:       true,
			},
			"SUBSTR": {
				AllowedInFilters:    true,
				AllowedInProjections: true,
				AllowedInSort:       true,
			},
			// Math functions
			"ABS": {
				AllowedInFilters:    true,
				AllowedInProjections: true,
				AllowedInSort:       true,
			},
			"ROUND": {
				AllowedInFilters:    true,
				AllowedInProjections: true,
				AllowedInSort:       true,
			},
			// Date functions
			"datetime": {
				AllowedInFilters:    true,
				AllowedInProjections: true,
				AllowedInSort:       true,
			},
			"date": {
				AllowedInFilters:    true,
				AllowedInProjections: true,
				AllowedInSort:       true,
			},
		},
		SupportedJoinTypes: map[query.JoinType]struct{}{
			query.JoinTypeInner: {},
			query.JoinTypeLeft:  {},
			// Note: Right and Full outer joins are not supported by SQLite
		},
		SupportedAggregationFunctions: map[query.AggregationType]struct{}{
			query.AggregationTypeCount: {},
			query.AggregationTypeSum:   {},
			query.AggregationTypeAvg:   {},
			query.AggregationTypeMin:   {},
			query.AggregationTypeMax:   {},
			// SQLite also supports GROUP_CONCAT which could be mapped to a custom type
		},
		SupportedPaginationTypes: map[query.PaginationType]struct{}{
			query.PaginationTypeOffset: {},
			// SQLite doesn't natively support cursor-based pagination
		},
		SupportedTextSearchTypes: map[query.TextSearchType]struct{}{
			// SQLite FTS would need to be explicitly enabled and configured
			// For now, basic LIKE-based search is handled via Contains operator
		},
		Sorting: query.SortingCapabilities{
			SupportsNullsOrdering: true,  // SQLite supports NULLS FIRST/LAST
			SupportsExpression:    true,  // SQLite can sort by expressions
		},
		SupportsGroupBy:      true,  // SQLite supports GROUP BY
		SupportsDistinct:     true,  // SQLite supports DISTINCT
		SupportsNestedFields: true,  // Via JSON functions like json_extract
		MaxWhereConditions:   0,     // SQLite has no practical limit
		MaxJoinClauses:      63,     // SQLite has a default limit of 64 tables in a join
		ReturnOnUpdate: true,
	}
}
