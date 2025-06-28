package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-anansi/core/schema"
)

// SqliteQueryGeneratorFactory implements the QueryGeneratorFactory for SQLite.
type SqliteQueryGeneratorFactory struct{}

// NewSqliteQueryGeneratorFactory creates a new instance of SqliteQueryGeneratorFactory.
func NewSqliteQueryGeneratorFactory() *SqliteQueryGeneratorFactory {
	return &SqliteQueryGeneratorFactory{}
}

// CreateGenerator creates a new SqliteQuery (which is a QueryGenerator) for the given schema.
func (f *SqliteQueryGeneratorFactory) CreateGenerator(schema *schema.SchemaDefinition) (query.QueryGenerator, error) {
	return NewSqliteQuery(schema)
}

// SqliteQuery is a schema-aware query generator for SQLite.
// It leverages a SchemaDefinition to correctly translate high-level queries
// against defined fields (including nested JSON fields) into valid SQLite SQL.
type SqliteQuery struct {
	schema *schema.SchemaDefinition
}

// NewSqliteQuery creates a new schema-aware query generator for SQLite.
func NewSqliteQuery(schema *schema.SchemaDefinition) (*SqliteQuery, error) {
	if schema == nil {
		return nil, fmt.Errorf("SchemaDefinition cannot be nil")
	}
	if schema.Name == "" {
		return nil, fmt.Errorf("schema must define a table name")
	}
	return &SqliteQuery{schema: schema}, nil
}

// quoteIdentifier properly quotes an identifier for SQLite.
func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// getFieldSQL translates a logical field path into the correct SQL accessor string.
func (s *SqliteQuery) getFieldSQL(fieldPath string) (string, error) {
	parts := strings.Split(fieldPath, ".")
	if len(parts) == 0 {
		return "", fmt.Errorf("field path cannot be empty")
	}

	rootField, ok := s.schema.Fields[parts[0]]
	if !ok {
		return "", fmt.Errorf("field '%s' not found in schema", parts[0])
	}

	if len(parts) == 1 {
		return quoteIdentifier(parts[0]), nil
	}

	switch rootField.Type {
	case schema.FieldTypeObject, schema.FieldTypeRecord, schema.FieldTypeUnion:
		jsonPath := "$." + strings.Join(parts[1:], ".")
		return fmt.Sprintf("json_extract(%s, '%s')", quoteIdentifier(parts[0]), jsonPath), nil
	default:
		return "", fmt.Errorf("field '%s' of type %s does not support nested querying", parts[0], rootField.Type)
	}
}

// prepareValueForQuery prepares a Go value for use as a SQL query parameter,
// performing type conversions based on the schema's FieldType.
// This ensures that Go types are correctly mapped to SQLite's underlying storage types,
// including boolean to integer conversion and JSON serialization for complex types.
func (s *SqliteQuery) prepareValueForQuery(fieldName string, value any) (any, error) {
	field, exists := s.schema.Fields[fieldName]
	if !exists {
		return nil, fmt.Errorf("field '%s' not found in schema for value preparation", fieldName)
	}

	if value == nil {
		return nil, nil
	}

	switch field.Type {
	case schema.FieldTypeBoolean:
		if boolVal, ok := value.(bool); ok {
			if boolVal {
				return 1, nil // Go true -> SQLite 1
			}
			return 0, nil // Go false -> SQLite 0
		}
		// Handle string "true" or "false" coercion
		if strVal, ok := value.(string); ok {
			lowerStr := strings.ToLower(strVal)
			if lowerStr == "true" {
				return 1, nil
			} else if lowerStr == "false" {
				return 0, nil
			}
		}
		// Attempt common conversions for boolean fields (e.g., from JSON numbers)
		if intVal, ok := value.(int); ok {
			return intVal, nil
		} else if int64Val, ok := value.(int64); ok {
			return int64Val, nil
		} else if float64Val, ok := value.(float64); ok { // Handle JSON numbers like 0.0, 1.0
			if float64Val == 1.0 {
				return 1, nil
			}
			if float64Val == 0.0 {
				return 0, nil
			}
		}
		return nil, fmt.Errorf("expected boolean for FieldTypeBoolean, got %T for field '%s'", value, fieldName)

	case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeSet, schema.FieldTypeRecord, schema.FieldTypeUnion:
		// For complex types, marshal to JSON string as they are stored as TEXT in SQLite.
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize field '%s' to JSON: %w", fieldName, err)
		}
		return string(jsonBytes), nil

	case schema.FieldTypeEnum:
		// For enum, ensure it's a string, as enums are stored as TEXT
		if strVal, ok := value.(string); ok {
			return strVal, nil
		}
		// Attempt a generic string conversion for other types used as enum values
		return fmt.Sprintf("%v", value), nil

	// For other basic types (String, Number, Integer, Decimal),
	// the `database/sql` driver typically handles them correctly if passed directly.
	default:
		return value, nil
	}
}

// GenerateSelectSQL creates a complete SQL SELECT query string and its corresponding
// parameters from a `query.QueryDSL` object.
func (s *SqliteQuery) GenerateSelectSQL(dsl *query.QueryDSL) (string, []any, error) {
	if dsl == nil {
		return "", nil, fmt.Errorf("QueryDSL cannot be nil")
	}
	quotedTableName := quoteIdentifier(s.schema.Name)

	var selectFields, whereClauses, orderByClauses []string
	var queryParams []any
	limit, offset := -1, 0

	if dsl.Projection != nil && len(dsl.Projection.Include) > 0 {
		for _, field := range dsl.Projection.Include {
			accessor, err := s.getFieldSQL(field.Name)
			if err != nil {
				return "", nil, fmt.Errorf("projection error: %w", err)
			}
			selectFields = append(selectFields, fmt.Sprintf("%s AS %s", accessor, quoteIdentifier(field.Name)))
		}
	} else {
		selectFields = append(selectFields, "*")
	}

	if dsl.Filters != nil {
		whereSQL, err := s.buildWhereClause(dsl.Filters, &queryParams)
		if err != nil {
			return "", nil, fmt.Errorf("error building WHERE clause: %w", err)
		}
		if whereSQL != "" {
			whereClauses = append(whereClauses, whereSQL)
		}
	}

	if len(dsl.Sort) > 0 {
		for _, sortCfg := range dsl.Sort {
			accessor, err := s.getFieldSQL(sortCfg.Field)
			if err != nil {
				return "", nil, fmt.Errorf("sort error: %w", err)
			}
			orderByClauses = append(orderByClauses, fmt.Sprintf("%s %s", accessor, strings.ToUpper(string(sortCfg.Direction))))
		}
	}

	if dsl.Pagination != nil {
		limit = dsl.Pagination.Limit
		if dsl.Pagination.Offset != nil {
			offset = *dsl.Pagination.Offset
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SELECT %s FROM %s", strings.Join(selectFields, ", "), quotedTableName))
	if len(whereClauses) > 0 {
		sb.WriteString(" WHERE " + strings.Join(whereClauses, " AND "))
	}
	if len(orderByClauses) > 0 {
		sb.WriteString(" ORDER BY " + strings.Join(orderByClauses, ", "))
	}
	if limit > -1 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", limit))
	}
	if offset > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", offset))
	}

	return sb.String() + ";", queryParams, nil
}

// buildWhereClause recursively builds the WHERE clause from a `core.QueryFilter` object.
func (s *SqliteQuery) buildWhereClause(filter *query.QueryFilter, params *[]any) (string, error) {
	if filter.Condition != nil {
		return s.buildCondition(filter.Condition, params)
	}
	if filter.Group != nil {
		if filter.Group.Operator == "" { // Add this check
			return "", fmt.Errorf("logical operator missing in filter group")
		}
		var clauses []string
		for _, cond := range filter.Group.Conditions {
			clause, err := s.buildWhereClause(&cond, params)
			if err != nil {
				return "", err
			}
			if clause != "" {
				clauses = append(clauses, clause)
			}
		}
		if len(clauses) == 0 {
			return "", nil
		}
		op := strings.ToUpper(string(filter.Group.Operator))
		return fmt.Sprintf("(%s)", strings.Join(clauses, " "+op+" ")), nil
	}
	return "", fmt.Errorf("invalid filter structure: neither Condition nor Group is set")
}

// buildCondition translates a single `core.FilterCondition` into a SQL condition string.
func (s *SqliteQuery) buildCondition(cond *query.FilterCondition, params *[]any) (string, error) {
	accessor, err := s.getFieldSQL(cond.Field)
	if err != nil {
		return "", err
	}

	// Prepare the value based on the field's schema type before appending to params.
	preparedValue, err := s.prepareValueForQuery(cond.Field, cond.Value)
	if err != nil {
		return "", fmt.Errorf("failed to prepare value for condition field '%s': %w", cond.Field, err)
	}

	switch cond.Operator {
	case query.ComparisonOperatorEq:
		*params = append(*params, preparedValue)
		return fmt.Sprintf("%s = ?", accessor), nil
	case query.ComparisonOperatorNeq:
		*params = append(*params, preparedValue)
		return fmt.Sprintf("%s != ?", accessor), nil
	case query.ComparisonOperatorLt:
		*params = append(*params, preparedValue)
		return fmt.Sprintf("%s < ?", accessor), nil
	case query.ComparisonOperatorLte:
		*params = append(*params, preparedValue)
		return fmt.Sprintf("%s <= ?", accessor), nil
	case query.ComparisonOperatorGt:
		*params = append(*params, preparedValue)
		return fmt.Sprintf("%s > ?", accessor), nil
	case query.ComparisonOperatorGte:
		*params = append(*params, preparedValue)
		return fmt.Sprintf("%s >= ?", accessor), nil
	case query.ComparisonOperatorIn, query.ComparisonOperatorNin:
		vals, ok := preparedValue.([]any) // preparedValue should already be []any if original was
		if !ok { // If it wasn't a slice, try to make it one for single-value IN
			if preparedValue != nil {
				vals = []any{preparedValue}
				ok = true
			}
		}
		if !ok || len(vals) == 0 {
			// If IN/NIN gets an empty or non-array value, return appropriate SQL that evaluates to true/false
			if cond.Operator == query.ComparisonOperatorIn {
				return "1=0", nil // IN empty list is always false
			}
			return "1=1", nil // NOT IN empty list is always true
		}

		placeholders := strings.Repeat("?,", len(vals)-1) + "?"
		for _, v := range vals {
			*params = append(*params, v)
		}
		op := "IN"
		if cond.Operator == query.ComparisonOperatorNin {
			op = "NOT IN"
		}
		return fmt.Sprintf("%s %s (%s)", accessor, op, placeholders), nil
	case query.ComparisonOperatorContains:
		// Ensure preparedValue is string before formatting for LIKE
		strVal := fmt.Sprintf("%v", preparedValue)
		*params = append(*params, "%"+strVal+"%")
		return fmt.Sprintf("%s LIKE ?", accessor), nil
	case query.ComparisonOperatorNotContains:
		strVal := fmt.Sprintf("%v", preparedValue)
		*params = append(*params, "%"+strVal+"%")
		return fmt.Sprintf("%s NOT LIKE ?", accessor), nil
	case query.ComparisonOperatorStartsWith:
		strVal := fmt.Sprintf("%v", preparedValue)
		*params = append(*params, strVal+"%")
		return fmt.Sprintf("%s LIKE ?", accessor), nil
	case query.ComparisonOperatorEndsWith:
		strVal := fmt.Sprintf("%v", preparedValue)
		*params = append(*params, "%"+strVal)
		return fmt.Sprintf("%s LIKE ?", accessor), nil
	case query.ComparisonOperatorExists:
		return fmt.Sprintf("%s IS NOT NULL", accessor), nil
	case query.ComparisonOperatorNotExists:
		return fmt.Sprintf("%s IS NULL", accessor), nil
	default:
		return "", fmt.Errorf("unsupported comparison operator for direct SQL: %s", cond.Operator)
	}
}

// GenerateUpdateSQL creates a SQL UPDATE query, using the schema's table name.
func (s *SqliteQuery) GenerateUpdateSQL(updates map[string]any, filters *query.QueryFilter) (string, []any, error) {
	quotedTableName := quoteIdentifier(s.schema.Name)
	var setClauses []string
	var queryParams []any

	if len(updates) == 0 {
		return "", nil, fmt.Errorf("no fields provided for update")
	}

	for fieldName, value := range updates {
		accessor, err := s.getFieldSQL(fieldName)
		if err != nil {
			return "", nil, fmt.Errorf("update set clause error for field '%s': %w", fieldName, err)
		}
		// Use the new prepareValueForQuery
		preparedValue, err := s.prepareValueForQuery(fieldName, value)
		if err != nil {
			return "", nil, fmt.Errorf("error preparing value for field '%s': %w", fieldName, err)
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", accessor))
		queryParams = append(queryParams, preparedValue)
	}
	setSQL := strings.Join(setClauses, ", ")

	var whereSQL string
	if filters != nil {
		var err error
		whereSQL, err = s.buildWhereClause(filters, &queryParams)
		if err != nil {
			return "", nil, fmt.Errorf("error building WHERE clause for update: %w", err)
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("UPDATE %s SET %s", quotedTableName, setSQL))
	if whereSQL != "" {
		sb.WriteString(" WHERE " + whereSQL)
	}
	return sb.String() + ";", queryParams, nil
}

// GenerateInsertSQL creates a SQL INSERT query. It includes the `RETURNING *` clause
// for atomic retrieval of inserted data. NOTE: Requires SQLite version 3.35.0+.
func (s *SqliteQuery) GenerateInsertSQL(records []map[string]any) (string, []any, error) {
	if len(records) == 0 {
		return "", nil, fmt.Errorf("no records provided for insert")
	}
	quotedTableName := quoteIdentifier(s.schema.Name)

	fieldSet := make(map[string]bool)
	for _, record := range records {
		for fieldName := range record {
			if _, exists := s.schema.Fields[fieldName]; !exists {
				return "", nil, fmt.Errorf("field '%s' not found in schema", fieldName)
			}
			fieldSet[fieldName] = true
		}
	}

	var fields []string
	for fieldName := range fieldSet {
		fields = append(fields, fieldName)
	}

	if len(fields) == 0 {
		return "", nil, fmt.Errorf("no valid fields found in records")
	}

	var quotedFields []string
	for _, field := range fields {
		quotedFields = append(quotedFields, quoteIdentifier(field))
	}
	columnsSQL := strings.Join(quotedFields, ", ")

	var valuesClauses []string
	var queryParams []any
	for _, record := range records {
		var rowPlaceholders []string
		for _, fieldName := range fields {
			value, exists := record[fieldName]
			if !exists {
				value = nil
			}
			// Use the new prepareValueForQuery
			preparedValue, err := s.prepareValueForQuery(fieldName, value)
			if err != nil {
				return "", nil, fmt.Errorf("error preparing value for field '%s': %w", fieldName, err)
			}
			rowPlaceholders = append(rowPlaceholders, "?")
			queryParams = append(queryParams, preparedValue)
		}
		valuesClauses = append(valuesClauses, "("+strings.Join(rowPlaceholders, ", ")+")")
	}
	valuesSQL := strings.Join(valuesClauses, ", ")

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s RETURNING *;", quotedTableName, columnsSQL, valuesSQL)
	return sql, queryParams, nil
}

// GenerateDeleteSQL creates a SQL DELETE query, using the schema's table name.
func (s *SqliteQuery) GenerateDeleteSQL(filters *query.QueryFilter, unsafeDelete bool) (string, []any, error) {
	quotedTableName := quoteIdentifier(s.schema.Name)
	var queryParams []any

	if filters == nil && !unsafeDelete {
		return "", nil, fmt.Errorf("DELETE without WHERE clause is not allowed for safety. Set unsafeDelete=true to override")
	}

	var whereSQL string
	if filters != nil {
		var err error
		whereSQL, err = s.buildWhereClause(filters, &queryParams)
		if err != nil {
			return "", nil, fmt.Errorf("error building WHERE clause for delete: %w", err)
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DELETE FROM %s", quotedTableName))
	if whereSQL != "" {
		sb.WriteString(" WHERE " + whereSQL)
	}
	return sb.String() + ";", queryParams, nil
}
