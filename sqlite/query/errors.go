package query

import "github.com/asaidimu/go-anansi/v7/core/common"

var (
	// Delete errors
	ErrDeleteNoTarget          = common.NewSystemError("ERR_QUERY_DELETE_NO_TARGET", "no target specified for delete")
	ErrDeleteStatementNoTarget = common.NewSystemError("ERR_QUERY_DELETE_STATEMENT_NO_TARGET", "delete statement must have a target")
	ErrDeleteQueryNoTarget     = common.NewSystemError("ERR_QUERY_DELETE_QUERY_NO_TARGET", "delete query must have a target")

	// Select errors
	ErrSelectUnsupportedFieldReference  = common.NewSystemError("ERR_QUERY_SELECT_UNSUPPORTED_FIELD_REFERENCE", "unsupported field reference")
	ErrSelectUnsupportedAggregationType = common.NewSystemError("ERR_QUERY_SELECT_UNSUPPORTED_AGGREGATION_TYPE", "unsupported aggregation type")
	ErrSelectBinaryOperatorArgs         = common.NewSystemError("ERR_QUERY_SELECT_BINARY_OPERATOR_ARGS", "binary operator expects 2 arguments")
	ErrSelectEmptyFilter                = common.NewSystemError("ERR_QUERY_SELECT_EMPTY_FILTER", "empty filter")
	ErrSelectUnsupportedOperator        = common.NewSystemError("ERR_QUERY_SELECT_UNSUPPORTED_OPERATOR", "unsupported operator")
	ErrSelectUnsupportedLogicalOperator = common.NewSystemError("ERR_QUERY_SELECT_UNSUPPORTED_LOGICAL_OPERATOR", "unsupported logical operator")
	ErrSelectUnsupportedTextSearchType  = common.NewSystemError("ERR_QUERY_SELECT_UNSUPPORTED_TEXT_SEARCH_TYPE", "unsupported text search type")
	ErrSelectUnsupportedFilterValue     = common.NewSystemError("ERR_QUERY_SELECT_UNSUPPORTED_FILTER_VALUE", "unsupported filter value")
	ErrSelectSubqueryNotImplemented     = common.NewSystemError("ERR_QUERY_SELECT_SUBQUERY_NOT_IMPLEMENTED", "subquery support not yet implemented")
	ErrSelectNoTargetSpecified          = common.NewSystemError("ERR_QUERY_SELECT_NO_TARGET_SPECIFIED", "no target specified")
	ErrSelectUnsupportedJoinType        = common.NewSystemError("ERR_QUERY_SELECT_UNSUPPORTED_JOIN_TYPE", "unsupported join type")
	ErrSelectLimitInvalid               = common.NewSystemError("ERR_QUERY_SELECT_LIMIT_INVALID", "limit must be greater than zero for pagination")
	ErrSelectBuildError                 = common.NewSystemError("ERR_QUERY_SELECT_BUILD_ERROR", "error building query part")

	// Convert errors
	ErrConvertMarshalValueFailed = common.NewSystemError("ERR_QUERY_CONVERT_MARSHAL_VALUE_FAILED", "failed to marshal value to JSON")
	ErrConvertMarshalFieldFailed = common.NewSystemError("ERR_QUERY_CONVERT_MARSHAL_FIELD_FAILED", "failed to marshal field to JSON")

	// Collection errors
	ErrCollectionSchemaNotDefined    = common.NewSystemError("ERR_QUERY_COLLECTION_SCHEMA_NOT_DEFINED", "schema is not defined")
	ErrCollectionFieldError          = common.NewSystemError("ERR_QUERY_COLLECTION_FIELD_ERROR", "error on field")
	ErrCollectionTableNameNotDefined = common.NewSystemError("ERR_QUERY_COLLECTION_TABLE_NAME_NOT_DEFINED", "table name is not defined")

	// Update errors
	ErrUpdateInvalidComputedFieldQuery = common.NewSystemError("ERR_QUERY_UPDATE_INVALID_COMPUTED_FIELD_QUERY", "invalid query for computed field")
	ErrUpdateStatementNoTarget         = common.NewSystemError("ERR_QUERY_UPDATE_STATEMENT_NO_TARGET", "update statement must have a target")
	ErrUpdateStatementNoAssignments    = common.NewSystemError("ERR_QUERY_UPDATE_STATEMENT_NO_ASSIGNMENTS", "update statement must have assignments")
	ErrUpdateNoTargetSpecified         = common.NewSystemError("ERR_QUERY_UPDATE_NO_TARGET_SPECIFIED", "no target specified for update")
	ErrUpdateQueryNoTarget             = common.NewSystemError("ERR_QUERY_UPDATE_QUERY_NO_TARGET", "update query must have a target")
	ErrUpdateQueryNoDataPayload        = common.NewSystemError("ERR_QUERY_UPDATE_QUERY_NO_DATA_PAYLOAD", "update query must have data payload for set or compute")
	ErrBuilderInvalidUpdatePayload     = common.NewSystemError("ERR_QUERY_BUILDER_INVALID_UPDATE_PAYLOAD", "invalid data type for update payload: expected map[string]any")
	ErrUpdateInvalidSetType            = common.NewSystemError("ERR_QUERY_UPDATE_INVALID_SET_TYPE", "invalid data type for 'set' in update")
	ErrUpdateInvalidComputeType        = common.NewSystemError("ERR_QUERY_UPDATE_INVALID_COMPUTE_TYPE", "invalid data type for 'compute' in update")

	// Builder errors
	ErrBuilderUnsupportedStatementType = common.NewSystemError("ERR_QUERY_BUILDER_UNSUPPORTED_STATEMENT_TYPE", "unsupported statement type")

	// Insert errors
	ErrInsertNoDataProvided                  = common.NewSystemError("ERR_QUERY_INSERT_NO_DATA_PROVIDED", "no data provided for insert")
	ErrInsertSingleAndBatchMutuallyExclusive = common.NewSystemError("ERR_QUERY_INSERT_SINGLE_AND_BATCH_MUTUALLY_EXCLUSIVE", "cannot specify both single document and batch")
	ErrInsertSchemaNoFields                  = common.NewSystemError("ERR_QUERY_INSERT_SCHEMA_NO_FIELDS", "provided schema has no fields defined for insert")
	ErrInsertEmptyBatch                      = common.NewSystemError("ERR_QUERY_INSERT_EMPTY_BATCH", "empty batch provided for insert")
	ErrInsertEmptyDocument                   = common.NewSystemError("ERR_QUERY_INSERT_EMPTY_DOCUMENT", "empty document provided for insert")
	ErrInsertDocumentError                   = common.NewSystemError("ERR_QUERY_INSERT_DOCUMENT_ERROR", "document error")
	ErrInsertFieldConversionFailed           = common.NewSystemError("ERR_QUERY_INSERT_FIELD_CONVERSION_FAILED", "failed to convert field")
	ErrInsertStatementNoTarget               = common.NewSystemError("ERR_QUERY_INSERT_STATEMENT_NO_TARGET", "insert statement must have a target")
	ErrInsertStatementNoValues               = common.NewSystemError("ERR_QUERY_INSERT_STATEMENT_NO_VALUES", "insert statement must have values")
	ErrInsertNoTargetSpecified               = common.NewSystemError("ERR_QUERY_INSERT_NO_TARGET_SPECIFIED", "no target specified for insert")
	ErrInsertQueryNoTarget                   = common.NewSystemError("ERR_QUERY_INSERT_QUERY_NO_TARGET", "insert query must have a target")
	ErrInsertQueryNoData                     = common.NewSystemError("ERR_QUERY_INSERT_QUERY_NO_DATA", "insert query must have data")
	ErrInsertInvalidDataType                 = common.NewSystemError("ERR_QUERY_INSERT_INVALID_DATA_TYPE", "invalid data type for insert")

	// Index errors
	ErrIndexExtraNotIndexDefinition = common.NewSystemError("ERR_QUERY_INDEX_EXTRA_NOT_INDEX_DEFINITION", "extra is not an IndexDefinition")
	ErrIndexSchemaNotDefined        = common.NewSystemError("ERR_QUERY_INDEX_SCHEMA_NOT_DEFINED", "schema is not defined for create index tree")
	ErrIndexIndexNotDefined         = common.NewSystemError("ERR_QUERY_INDEX_INDEX_NOT_DEFINED", "index is not defined for create index tree")
)
