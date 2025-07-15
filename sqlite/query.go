// Package sqlite provides a concrete implementation of the query.QueryGenerator interface
// for SQLite databases. It is responsible for translating the abstract QueryDSL into
// valid SQLite SQL queries.
package sqlite

import (
	"encoding/json"
	"errors" // Import the errors package
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// Pre-defined errors for the sqlite query generator.
var (
	ErrSchemaNil               = errors.New("SchemaDefinition cannot be nil")
	ErrTableNameMissing        = errors.New("schema must define a table name")
	ErrFieldPathEmpty          = errors.New("field path cannot be empty")
	ErrFieldNotFound           = errors.New("field not found in schema")
	ErrNestedQueryNotSupported = errors.New("field does not support nested querying")
	ErrInvalidBooleanType      = errors.New("expected boolean for FieldTypeBoolean")
	ErrSerializationFailed     = errors.New("failed to serialize field to JSON")
	ErrQueryDSLNil             = errors.New("QueryDSL cannot be nil")
	ErrMissingLogicalOperator  = errors.New("logical operator missing in filter group")
	ErrInvalidFilterStructure  = errors.New("invalid filter structure")
	ErrUnsupportedOperator     = errors.New("unsupported comparison operator for direct SQL")
	ErrNoFieldsForUpdate       = errors.New("no fields provided for update")
	ErrNoRecordsForInsert      = errors.New("no records provided for insert")
	ErrNoValidFieldsInRecords  = errors.New("no valid fields found in records")
	ErrDeleteWithoutWhere      = errors.New("DELETE without WHERE clause is not allowed for safety")
)

// SqliteQueryGeneratorFactory is an implementation of the query.QueryGeneratorFactory
// for SQLite. It creates instances of the SqliteQuery generator.
type SqliteQueryGeneratorFactory struct{}

// NewSqliteQueryGeneratorFactory creates a new instance of the SqliteQueryGeneratorFactory.
func NewSqliteQueryGeneratorFactory() *SqliteQueryGeneratorFactory {
	return &SqliteQueryGeneratorFactory{}
}

// CreateGenerator creates a new SqliteQuery, which is a query.QueryGenerator for the
// given schema.
func (f *SqliteQueryGeneratorFactory) CreateGenerator(schema *schema.SchemaDefinition) (query.QueryGenerator, error) {
	return NewSqliteQuery(schema)
}

// SqliteQuery is a schema-aware query generator for SQLite. It uses a schema.SchemaDefinition
// to translate a high-level QueryDSL into valid SQLite SQL, including handling nested
// JSON fields.
type SqliteQuery struct {
	schema *schema.SchemaDefinition
}

// NewSqliteQuery creates a new schema-aware query generator for SQLite.
func NewSqliteQuery(schema *schema.SchemaDefinition) (*SqliteQuery, error) {
	if schema == nil {
		return nil, ErrSchemaNil // Use pre-defined error
	}
	if schema.Name == "" {
		return nil, ErrTableNameMissing // Use pre-defined error
	}
	return &SqliteQuery{schema: schema}, nil
}

// quoteIdentifier safely quotes an identifier for use in an SQLite query.
func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// getFieldSQL translates a field path into the correct SQL accessor string, handling
// nested fields in JSON objects.
func (s *SqliteQuery) getFieldSQL(fieldPath string) (string, error) {
	parts := strings.Split(fieldPath, ".")
	if len(parts) == 0 {
		return "", ErrFieldPathEmpty // Use pre-defined error
	}

	rootField := s.schema.FindField(parts[0])

	if rootField == nil {
		return "", fmt.Errorf("%w '%s' in schema", ErrFieldNotFound, parts[0]) // Wrap pre-defined error
	}

	if len(parts) == 1 {
		return quoteIdentifier(parts[0]), nil
	}

	switch rootField.Type {
	case schema.FieldTypeObject, schema.FieldTypeRecord, schema.FieldTypeUnion:
		jsonPath := "$." + strings.Join(parts[1:], ".")
		return fmt.Sprintf("json_extract(%s, '%s')", quoteIdentifier(parts[0]), jsonPath), nil
	default:
		return "", fmt.Errorf("%w '%s' of type %s", ErrNestedQueryNotSupported, parts[0], rootField.Type) // Wrap pre-defined error
	}
}

// prepareValueForQuery prepares a Go value for use as a SQL query parameter, converting
// it to a type that is compatible with the underlying SQLite driver.
func (s *SqliteQuery) prepareValueForQuery(fieldName string, value any) (any, error) {
	field := s.schema.FindField(fieldName)

	if field == nil {
		return nil, fmt.Errorf("%w for value preparation '%s'", ErrFieldNotFound, fieldName) // Wrap pre-defined error
	}

	if value == nil {
		return nil, nil
	}

	switch field.Type {
	case schema.FieldTypeBoolean:
		if boolVal, ok := value.(bool); ok {
			if boolVal {
				return 1, nil
			}
			return 0, nil
		}
		if strVal, ok := value.(string); ok {
			lowerStr := strings.ToLower(strVal)
			switch lowerStr {
			case "true":
				return 1, nil
			case "false":
				return 0, nil
			}
		}
		if intVal, ok := value.(int); ok {
			return intVal, nil
		} else if int64Val, ok := value.(int64); ok {
			return int64Val, nil
		} else if float64Val, ok := value.(float64); ok {
			if float64Val == 1.0 {
				return 1, nil
			}
			if float64Val == 0.0 {
				return 0, nil
			}
		}
		return nil, fmt.Errorf("%w, got %T for field '%s'", ErrInvalidBooleanType, value, fieldName) // Wrap pre-defined error

	case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeSet, schema.FieldTypeRecord, schema.FieldTypeUnion:
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("%w field '%s': %w", ErrSerializationFailed, fieldName, err) // Wrap pre-defined error
		}
		return string(jsonBytes), nil

	case schema.FieldTypeEnum:
		if strVal, ok := value.(string); ok {
			return strVal, nil
		}
		return fmt.Sprintf("%v", value), nil

	default:
		return value, nil
	}
}

// GenerateSelectSQL generates a SQL SELECT query from a QueryDSL object.
func (s *SqliteQuery) GenerateSelectSQL(dsl *query.QueryDSL) (string, []any, error) {
	if dsl == nil {
		return "", nil, ErrQueryDSLNil // Use pre-defined error
	}
	quotedTableName := quoteIdentifier(s.schema.Name)

	var selectFields, whereClauses, orderByClauses []string
	var queryParams []any
	limit, offset := -1, 0

	if dsl.Projection != nil && len(dsl.Projection.Include) > 0 {
		for _, field := range dsl.Projection.Include {
			accessor, err := s.getFieldSQL(field.Name)
			if err != nil {
				return "", nil, fmt.Errorf("projection error: %w", err) // Wrap original error
			}
			selectFields = append(selectFields, fmt.Sprintf("%s AS %s", accessor, quoteIdentifier(field.Name)))
		}
	} else {
		selectFields = append(selectFields, "*")
	}

	if dsl.Filters != nil {
		whereSQL, err := s.buildWhereClause(dsl.Filters, &queryParams)
		if err != nil {
			return "", nil, fmt.Errorf("error building WHERE clause: %w", err) // Wrap original error
		}
		if whereSQL != "" {
			whereClauses = append(whereClauses, whereSQL)
		}
	}

	if len(dsl.Sort) > 0 {
		for _, sortCfg := range dsl.Sort {
			accessor, err := s.getFieldSQL(sortCfg.Field)
			if err != nil {
				return "", nil, fmt.Errorf("sort error: %w", err) // Wrap original error
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

// buildWhereClause recursively builds the WHERE clause of a SQL query.
func (s *SqliteQuery) buildWhereClause(filter *query.QueryFilter, params *[]any) (string, error) {
	if filter.Condition != nil {
		return s.buildCondition(filter.Condition, params)
	}
	if filter.Group != nil {
		if filter.Group.Operator == "" {
			return "", ErrMissingLogicalOperator // Use pre-defined error
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
	return "", ErrInvalidFilterStructure // Use pre-defined error
}

// buildCondition translates a single filter condition into a SQL condition string.
func (s *SqliteQuery) buildCondition(cond *query.FilterCondition, params *[]any) (string, error) {
	accessor, err := s.getFieldSQL(cond.Field)
	if err != nil {
		return "", err
	}

	field := s.schema.FindField(cond.Field)
	if field == nil {
		return "", fmt.Errorf("%w '%s' in schema", ErrFieldNotFound, cond.Field) // Wrap pre-defined error
	}
	preparedValue, err := s.prepareValueForQuery(cond.Field, cond.Value)
	if err != nil {
		return "", fmt.Errorf("failed to prepare value for condition field '%s': %w", cond.Field, err) // Wrap original error
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
		vals, ok := preparedValue.([]any)
		if !ok {
			if preparedValue != nil {
				vals = []any{preparedValue}
				ok = true
			}
		}
		if !ok || len(vals) == 0 {
			if cond.Operator == query.ComparisonOperatorIn {
				return "1=0", nil
			}
			return "1=1", nil
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
		strVal := fmt.Sprintf("%%%v%%", preparedValue)
		*params = append(*params, strVal)
		return fmt.Sprintf("%s LIKE ?", accessor), nil
	case query.ComparisonOperatorNotContains:
		strVal := fmt.Sprintf("%%%v%%", preparedValue)
		*params = append(*params, strVal)
		return fmt.Sprintf("%s NOT LIKE ?", accessor), nil
	case query.ComparisonOperatorStartsWith:
		strVal := fmt.Sprintf("%v%%", preparedValue)
		*params = append(*params, strVal)
		return fmt.Sprintf("%s LIKE ?", accessor), nil
	case query.ComparisonOperatorEndsWith:
		strVal := fmt.Sprintf("%%%v", preparedValue)
		*params = append(*params, strVal)
		return fmt.Sprintf("%s LIKE ?", accessor), nil
	case query.ComparisonOperatorExists:
		return fmt.Sprintf("%s IS NOT NULL", accessor), nil
	case query.ComparisonOperatorNotExists:
		return fmt.Sprintf("%s IS NULL", accessor), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedOperator, cond.Operator) // Wrap pre-defined error
	}
}

// GenerateUpdateSQL generates a SQL UPDATE query.
func (s *SqliteQuery) GenerateUpdateSQL(updates map[string]any, filters *query.QueryFilter) (string, []any, error) {
	quotedTableName := quoteIdentifier(s.schema.Name)
	var setClauses []string
	var queryParams []any

	if len(updates) == 0 {
		return "", nil, ErrNoFieldsForUpdate // Use pre-defined error
	}

	for fieldName, value := range updates {
		accessor, err := s.getFieldSQL(fieldName)
		if err != nil {
			return "", nil, fmt.Errorf("update set clause error for field '%s': %w", fieldName, err) // Wrap original error
		}
		preparedValue, err := s.prepareValueForQuery(fieldName, value)
		if err != nil {
			return "", nil, fmt.Errorf("error preparing value for field '%s': %w", fieldName, err) // Wrap original error
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
			return "", nil, fmt.Errorf("error building WHERE clause for update: %w", err) // Wrap original error
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("UPDATE %s SET %s", quotedTableName, setSQL))
	if whereSQL != "" {
		sb.WriteString(" WHERE " + whereSQL)
	}
	sb.WriteString(" RETURNING *;")
	return sb.String(), queryParams, nil
}

// GenerateInsertSQL generates a SQL INSERT query.
func (s *SqliteQuery) GenerateInsertSQL(records []map[string]any) (string, []any, error) {
	if len(records) == 0 {
		return "", nil, ErrNoRecordsForInsert // Use pre-defined error
	}
	quotedTableName := quoteIdentifier(s.schema.Name)

	fieldSet := make(map[string]bool)
	for _, record := range records {
		for fieldName := range record {
			if f := s.schema.FindField(fieldName); f == nil {
				return "", nil, fmt.Errorf("%w '%s' in schema", ErrFieldNotFound, fieldName) // Wrap pre-defined error
			}
			fieldSet[fieldName] = true
		}
	}

	var fields []string
	for fieldName := range fieldSet {
		fields = append(fields, fieldName)
	}

	if len(fields) == 0 {
		return "", nil, ErrNoValidFieldsInRecords // Use pre-defined error
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
			preparedValue, err := s.prepareValueForQuery(fieldName, value)
			if err != nil {
				return "", nil, fmt.Errorf("error preparing value for field '%s': %w", fieldName, err) // Wrap original error
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

// GenerateDeleteSQL generates a SQL DELETE query.
func (s *SqliteQuery) GenerateDeleteSQL(filters *query.QueryFilter, unsafeDelete bool) (string, []any, error) {
	quotedTableName := quoteIdentifier(s.schema.Name)
	var queryParams []any

	if filters == nil && !unsafeDelete {
		return "", nil, ErrDeleteWithoutWhere // Use pre-defined error
	}

	var whereSQL string
	if filters != nil {
		var err error
		whereSQL, err = s.buildWhereClause(filters, &queryParams)
		if err != nil {
			return "", nil, fmt.Errorf("error building WHERE clause for delete: %w", err) // Wrap original error
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DELETE FROM %s", quotedTableName))
	if whereSQL != "" {
		sb.WriteString(" WHERE " + whereSQL)
	}
	return sb.String() + ";", queryParams, nil
}
