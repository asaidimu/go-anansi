package query

import (
	"errors"
	"strings"
)

// QueryError represents errors specific to query operations.
type QueryError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *QueryError) Error() string {
	var b strings.Builder
	b.WriteString(e.Operation)
	b.WriteString(" operation failed")

	if e.Key != "" {
		b.WriteString(" for key '")
		b.WriteString(e.Key)
		b.WriteString("': ")
	} else {
		b.WriteString(": ")
	}
	b.WriteString(e.Message)

	if e.Cause != nil {
		b.WriteString(" (caused by: ")
		b.WriteString(e.Cause.Error())
		b.WriteString(")")
	}
	return b.String()
}

func (e *QueryError) Unwrap() error {
	return e.Cause
}

// Pre-defined errors for the query package.
var (
	ErrQueryFilterMustHaveOneFieldPopulated = errors.New("exactly one of Condition, Group, or TextSearchQuery must be set") // Updated message
	ErrQueryFilterMutuallyExclusive         = errors.New("QueryFilter cannot have both condition and group populated - they are mutually exclusive")
	ErrFilterConditionFieldEmpty            = errors.New("filter condition field cannot be empty")
	ErrUnknownComparisonOperator            = errors.New("unknown comparison operator")
	ErrTextSearchQueryEmpty                 = errors.New("text search query cannot be empty")
	ErrTextSearchFieldsEmpty                = errors.New("text search must specify at least one field")
	ErrInvalidSortDirection                 = errors.New("invalid sort direction")
	ErrPaginationLimitNotPositive           = errors.New("pagination limit must be positive") // Updated message
	ErrPaginationOffsetNegative             = errors.New("pagination offset cannot be negative")
	ErrInvalidFunctionArgument              = errors.New("invalid function argument")
	ErrInvalidNestedProjection              = errors.New("invalid nested projection")
	ErrFailedToMarshalRecordForDistinct     = errors.New("failed to marshal record for distinct check")
	ErrFailedToMarshalDistinctKey           = errors.New("failed to marshal distinct key")
	ErrUnsupportedAggregationType           = errors.New("unsupported aggregation type")
	ErrStreamingJoinProjection              = errors.New("projection error during streaming join")
	ErrFailedToApplyNestedExclusion         = errors.New("failed to apply nested exclusion")
	ErrFailedToApplyNestedProjection        = errors.New("failed to apply nested projection")

	// New errors
	ErrQueryCannotBeNil                 = errors.New("query cannot be nil")
	ErrTargetNameEmpty                  = errors.New("target name cannot be empty when target is specified")
	ErrSortFieldEmpty                   = errors.New("sort field cannot be empty")
	ErrInvalidPaginationType            = errors.New("invalid pagination type")
	ErrFilterCannotBeNil                = errors.New("filter cannot be nil")
	ErrFilterGroupEmpty                 = errors.New("filter group must have at least one condition")
	ErrTextSearchNotImplemented         = errors.New("text search is not implemented in this version of the helper")
	ErrFieldRefTypeInvalid              = errors.New("field reference type must be 'field'")
	ErrFieldRefFieldEmpty               = errors.New("field reference field cannot be empty")
	ErrSubqueryValueTypeInvalid         = errors.New("subquery value type must be 'subquery'")
	ErrSubqueriesNotSupported           = errors.New("subqueries are not supported by this in-memory query helper")
	ErrFunctionCallNameEmpty            = errors.New("function call function name cannot be empty")
	ErrFilterValueMultipleTypes         = errors.New("FilterValue can only have one type of value set")
	ErrProjectionConfigNil              = errors.New("projection configuration cannot be nil")
	ErrProjectionIncludeExcludeConflict = errors.New("cannot use both include and exclude in the same projection")
	ErrProjectionIncludeFieldEmpty      = errors.New("projection include field name cannot be empty")
	ErrProjectionExcludeFieldEmpty      = errors.New("projection exclude field name cannot be empty")
	ErrComputedFieldTypeInvalid         = errors.New("computed field expression type must be 'computed_field'")
	ErrComputedFieldExpressionNil       = errors.New("computed field expression cannot be nil")
	ErrComputedFieldAliasEmpty          = errors.New("computed field alias cannot be empty")
	ErrCaseExpressionTypeInvalid        = errors.New("case expression type must be 'case_expression'")
	ErrCaseExpressionNoConditions       = errors.New("case expression must have at least one condition")
	ErrCaseExpressionAliasEmpty         = errors.New("case expression alias cannot be empty")
	ErrProjectionComputedItemConflict   = errors.New("ProjectionComputedItem must have exactly one of ComputedFieldExpression or CaseExpression set")
	ErrDistinctConfigNil                = errors.New("distinct configuration cannot be nil")
	ErrDistinctConfigMissingFields      = errors.New("distinct configuration must specify 'is_distinct' or 'fields'")
	ErrDistinctConfigConflict           = errors.New("distinct configuration cannot specify both 'is_distinct' and 'fields'")
	ErrDistinctFieldNameEmpty           = errors.New("distinct field name cannot be empty")
	ErrAggregationConfigEmpty           = errors.New("aggregation configuration cannot be empty if specified")
	ErrAggregationFieldEmpty            = errors.New("aggregation field cannot be empty for non-count aggregations")
	ErrAggregationGroupFieldEmpty       = errors.New("aggregation group field cannot be empty")
	ErrInvalidFilterNoConditionGroupText = errors.New("invalid filter: no condition, group, or text match specified")
	ErrInOperatorRequiresSliceOrArray   = errors.New("IN operator requires a slice or array value as the comparison target")
	ErrContainsOperatorRequiresString   = errors.New("CONTAINS operator requires string values for comparison target")
	ErrFunctionNotImplementedOrRegistered = errors.New("function not implemented or registered")
	ErrJoinConfigNil                    = errors.New("join configuration cannot be nil")
	ErrUnsupportedJoinType              = errors.New("unsupported join type")
	ErrTargetCannotBeNil                = errors.New("target cannot be nil")
	ErrTargetNameEmptyStream            = errors.New("target name cannot be empty")
	ErrJoinConditionNil                 = errors.New("join condition ('on') cannot be nil")
)
