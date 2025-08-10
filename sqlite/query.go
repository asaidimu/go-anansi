// Package sqlite provides a concrete implementation of the query.QueryGenerator interface
// for SQLite databases. It is responsible for translating the abstract QueryDSL into
// valid SQLite SQL queries.
package sqlite

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
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
		return nil, fmt.Errorf("SchemaDefinition cannot be nil")
	}
	if schema.Name == "" {
		return nil, fmt.Errorf("schema must define a table name")
	}
	return &SqliteQuery{schema: schema}, nil
}

// quoteIdentifier safely quotes an identifier for use in an SQLite query.
func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// getFieldSQL translates a field path (e.g., "alias.field.nested") into the correct
// SQL accessor string, handling table aliases and nested fields in JSON objects.
func (s *SqliteQuery) getFieldSQL(fieldPath string, mainTableAlias string) (string, error) {
	parts := strings.Split(fieldPath, ".")
	var tableAlias string
	var columnName string
	var nestedParts []string

	if len(parts) > 1 {
		// Assume qualified path like "alias.field"
		tableAlias = parts[0]
		columnName = parts[1]
		nestedParts = parts[2:]
	} else {
		// Assume unqualified path for the main table
		tableAlias = mainTableAlias
		columnName = parts[0]
		nestedParts = parts[1:]
	}

	// The base accessor for a potentially nested field, e.g., "users"."profile"
	baseAccessor := fmt.Sprintf("%s.%s", quoteIdentifier(tableAlias), quoteIdentifier(columnName))

	// Check if this field belongs to the main schema to validate it.
	if tableAlias == mainTableAlias {
		rootField := s.findField(columnName)
		if rootField == nil {
			return "", fmt.Errorf("field '%s' not found in schema for main table alias '%s'", columnName, tableAlias)
		}

		if len(nestedParts) > 0 { // Nested access like "users.profile.city"
			switch rootField.Type {
			case schema.FieldTypeObject, schema.FieldTypeRecord, schema.FieldTypeUnion, schema.FieldTypeArray:
				// Type is valid for nesting, proceed to generate json_extract
			default:
				return "", fmt.Errorf("field '%s' of type %s on table '%s' does not support nested querying", columnName, rootField.Type, tableAlias)
			}
		}
	}
	// For joined tables, we can't validate the field, so we trust the path.

	if len(nestedParts) > 0 {
		// Generate json_extract for any nested path.
		jsonPath := "$." + strings.Join(nestedParts, ".")
		return fmt.Sprintf("json_extract(%s, '%s')", baseAccessor, jsonPath), nil
	}

	// Not a nested path, just return the qualified identifier.
	return baseAccessor, nil
}

func (s *SqliteQuery) findField(fieldName string) *schema.FieldDefinition {
	// findField now correctly handles the possibility of s.schema.Fields being nil
	if s.schema.Fields == nil {
		return nil
	}
	for _, field := range s.schema.Fields {
		if field.Name == fieldName {
			return field
		}
	}
	return nil
}


// GenerateSelectSQL generates a SQL SELECT query from a QueryDSL object.
func (s *SqliteQuery) GenerateSelectSQL(dsl *query.Query) (string, []any, error) {
	if dsl == nil {
		return "", nil, fmt.Errorf("QueryDSL cannot be nil")
	}

	// Determine main table physical name and alias
	physicalName := s.schema.Name
	alias := physicalName // Default alias
	if dsl.Target != nil {
		if dsl.Target.Name != "" {
			physicalName = dsl.Target.Name
		}
		if dsl.Target.Alias != nil && *dsl.Target.Alias != "" {
			alias = *dsl.Target.Alias
		} else {
			alias = physicalName // Fallback to physical name if alias is empty
		}
	}
	mainTableAlias := alias

	var selectFields, whereClauses, orderByClauses []string
	var queryParams []any
	limit, offset := -1, 0

	// Projection
	if dsl.Projection != nil && len(dsl.Projection.Include) > 0 {
		for _, field := range dsl.Projection.Include {
			accessor, err := s.getFieldSQL(field.Name, mainTableAlias)
			if err != nil {
				return "", nil, fmt.Errorf("projection error: %w", err)
			}
			// Use field alias if provided, otherwise use the full name
			selectAlias := field.Name
			if field.Alias != nil && *field.Alias != "" {
				selectAlias = *field.Alias
			}
			selectFields = append(selectFields, fmt.Sprintf("%s AS %s", accessor, quoteIdentifier(selectAlias)))
		}
	} else {
		selectFields = append(selectFields, "*")
	}

	// From and Joins
	var fromClause strings.Builder
	fromClause.WriteString(fmt.Sprintf("%s AS %s", quoteIdentifier(physicalName), quoteIdentifier(mainTableAlias)))

	if len(dsl.Joins) > 0 {
		for _, join := range dsl.Joins {
			if join.Type == query.JoinTypeRight || join.Type == query.JoinTypeFull {
				return "", nil, fmt.Errorf("unsupported join type for SQLite: %s", join.Type)
			}
			if join.Target.Name == "" {
				return "", nil, fmt.Errorf("join target physical name cannot be empty")
			}

			joinAlias := join.Target.Name
			if join.Target.Alias != nil && *join.Target.Alias != "" {
				joinAlias = *join.Target.Alias
			}

			joinType := strings.ToUpper(string(join.Type)) + " JOIN"
			fromClause.WriteString(fmt.Sprintf(" %s %s AS %s", joinType, quoteIdentifier(join.Target.Name), quoteIdentifier(joinAlias)))

			if join.On != nil {
				onSQL, err := s.buildWhereClause(join.On, &queryParams, mainTableAlias)
				if err != nil {
					return "", nil, fmt.Errorf("error building ON clause for join on target '%s': %w", join.Target.Name, err)
				}
				fromClause.WriteString(fmt.Sprintf(" ON %s", onSQL))
			} else {
				return "", nil, fmt.Errorf("join on target '%s' must have an ON condition", join.Target.Name)
			}
		}
	}

	// Filters
	if dsl.Filters != nil {
		whereSQL, err := s.buildWhereClause(dsl.Filters, &queryParams, mainTableAlias)
		if err != nil {
			return "", nil, fmt.Errorf("error building WHERE clause: %w", err)
		}
		if whereSQL != "" {
			whereClauses = append(whereClauses, whereSQL)
		}
	}

	// Sort
	if len(dsl.Sort) > 0 {
		for _, sortCfg := range dsl.Sort {
			accessor, err := s.getFieldSQL(sortCfg.Field, mainTableAlias)
			if err != nil {
				return "", nil, fmt.Errorf("sort error: %w", err)
			}
			orderByClauses = append(orderByClauses, fmt.Sprintf("%s %s", accessor, strings.ToUpper(string(sortCfg.Direction))))
		}
	}

	// Pagination
	if dsl.Pagination != nil {
		limit = dsl.Pagination.Limit
		if dsl.Pagination.Offset != nil {
			offset = *dsl.Pagination.Offset
		}
	}

	// Assemble Query
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SELECT %s FROM %s", strings.Join(selectFields, ", "), fromClause.String()))
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


	sql := sb.String() + ";"
	fmt.Printf("\n\nQuery\n %s \nSQL \n %s %v\n", dsl.MustToJSON(), sql, queryParams)
	return sql, queryParams, nil
}

// buildWhereClause recursively builds the WHERE clause of a SQL query.
func (s *SqliteQuery) buildWhereClause(filter *query.QueryFilter, params *[]any, mainTableAlias string) (string, error) {
	if filter.Condition != nil {
		// Properly qualify the field path for the condition
		fieldPath := filter.Condition.Field

		// If the field is not qualified with a table alias, add the main table alias
		if !strings.Contains(fieldPath, ".") {
			// Simple field like "id" becomes "mainTable.id"
			fieldPath = mainTableAlias + "." + fieldPath
		} else {
			// Check if this is a nested field reference that needs proper qualification
			parts := strings.Split(fieldPath, ".")

			// If the first part doesn't look like a table alias (no quotes, not the main table alias)
			// then assume it's a field.nestedfield format and qualify it
			if len(parts) >= 2 && parts[0] != mainTableAlias && !strings.Contains(parts[0], `"`) {
				// This is likely a "field.nested.path" format, qualify it as "mainTable.field.nested.path"
				fieldPath = mainTableAlias + "." + fieldPath
			}
		}

		// Create a new condition with the properly qualified field path
		qualifiedCondition := &query.FilterCondition{
			Field:    fieldPath,
			Operator: filter.Condition.Operator,
			Value:    filter.Condition.Value,
		}

		return s.buildCondition(qualifiedCondition, params, mainTableAlias)
	}
	if filter.Group != nil {
		if filter.Group.Operator == "" {
			return "", fmt.Errorf("logical operator missing in filter group")
		}
		var clauses []string
		for _, cond := range filter.Group.Conditions {
			clause, err := s.buildWhereClause(&cond, params, mainTableAlias)
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
	if filter.TextSearchQuery != nil {
		return "", fmt.Errorf("TextSearchQuery is not supported in this generator")
	}
	return "", fmt.Errorf("invalid filter structure")
}

// buildCondition translates a single filter condition into a SQL condition string.
func (s *SqliteQuery) buildCondition(cond *query.FilterCondition, params *[]any, mainTableAlias string) (string, error) {
	accessor, err := s.getFieldSQL(cond.Field, mainTableAlias)
	if err != nil {
		return "", err
	}

	// Handle operators that don't need a value
	switch cond.Operator {
	case query.ComparisonOperatorExists:
		return fmt.Sprintf("%s IS NOT NULL", accessor), nil
	case query.ComparisonOperatorNotExists:
		return fmt.Sprintf("%s IS NULL", accessor), nil
	}

	// Handle operators that use an array value ('in', 'nin')
	if cond.Operator == query.ComparisonOperatorIn || cond.Operator == query.ComparisonOperatorNin {
		if cond.Value.ArrayVal == nil {
			return "", fmt.Errorf("operator '%s' requires an array value", cond.Operator)
		}
		if len(cond.Value.ArrayVal) == 0 {
			if cond.Operator == query.ComparisonOperatorIn {
				return "1=0", nil /* FALSE */
			}
			return "1=1", nil /* TRUE */
		}

		var placeholders []string
		for _, filterVal := range cond.Value.ArrayVal {
			var val any
			if filterVal.StringVal != nil {
				val = *filterVal.StringVal
			} else if filterVal.NumberVal != nil {
				val = *filterVal.NumberVal
			} else if filterVal.BoolVal != nil {
				val = *filterVal.BoolVal
			} else {
				return "", fmt.Errorf("unsupported value type in array for 'IN' operator")
			}

			preparedValue, err := s.prepareValueForQuery(cond.Field, val, mainTableAlias)
			if err != nil {
				return "", err
			}
			*params = append(*params, preparedValue)
			placeholders = append(placeholders, "?")
		}

		op := "IN"
		if cond.Operator == query.ComparisonOperatorNin {
			op = "NOT IN"
		}
		return fmt.Sprintf("%s %s (%s)", accessor, op, strings.Join(placeholders, ",")), nil
	}

	// Handle all other operators that expect a single value
	var value any
	if cond.Value.StringVal != nil {
		value = *cond.Value.StringVal
	} else if cond.Value.NumberVal != nil {
		value = *cond.Value.NumberVal
	} else if cond.Value.BoolVal != nil {
		value = *cond.Value.BoolVal
	} else if cond.Value.ObjectVal != nil {
		value = cond.Value.ObjectVal
	} else {
		return "", fmt.Errorf("a value is required for operator '%s'", cond.Operator)
	}

	preparedValue, err := s.prepareValueForQuery(cond.Field, value, mainTableAlias)
	if err != nil {
		return "", fmt.Errorf("failed to prepare value for condition field '%s': %w", cond.Field, err)
	}

	var op string
	switch cond.Operator {
	case query.ComparisonOperatorEq:
		op = "="
	case query.ComparisonOperatorNeq:
		op = "!="
	case query.ComparisonOperatorLt:
		op = "<"
	case query.ComparisonOperatorLte:
		op = "<="
	case query.ComparisonOperatorGt:
		op = ">"
	case query.ComparisonOperatorGte:
		op = ">="
	case query.ComparisonOperatorContains:
		*params = append(*params, fmt.Sprintf("%%%v%%", preparedValue))
		return fmt.Sprintf("%s LIKE ?", accessor), nil
	case query.ComparisonOperatorNotContains:
		*params = append(*params, fmt.Sprintf("%%%v%%", preparedValue))
		return fmt.Sprintf("%s NOT LIKE ?", accessor), nil
	default:
		return "", fmt.Errorf("unsupported comparison operator for direct SQL: %s", cond.Operator)
	}

	*params = append(*params, preparedValue)
	return fmt.Sprintf("%s %s ?", accessor, op), nil
}

// GenerateUpdateSQL generates a SQL UPDATE query.
func (s *SqliteQuery) GenerateUpdateSQL(updates map[string]any, filters *query.QueryFilter) (string, []any, error) {
	quotedTableName := quoteIdentifier(s.schema.Name)
	mainTableAlias := s.schema.Name // Use schema name as the main table alias
	var setClauses []string
	var queryParams []any

	if len(updates) == 0 {
		return "", nil, fmt.Errorf("no fields provided for update")
	}

	for fieldName, value := range updates {
		// Ensure field is qualified with table alias
		qualifiedFieldName := fieldName
		if !strings.Contains(fieldName, ".") {
			qualifiedFieldName = mainTableAlias + "." + fieldName
		}

		accessor, err := s.getFieldSQL(qualifiedFieldName, mainTableAlias)
		if err != nil {
			return "", nil, fmt.Errorf("update set clause error for field '%s': %w", fieldName, err)
		}

		preparedValue, err := s.prepareValueForQuery(qualifiedFieldName, value, mainTableAlias)
		if err != nil {
			return "", nil, fmt.Errorf("error preparing value for field '%s': %w", fieldName, err)
		}

		_, ok := preparedValue.(string)
	    fmt.Printf("\n\n Prepared Value \n%s\n IS String %v \n", preparedValue, ok)
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", accessor))
		queryParams = append(queryParams, preparedValue)
	}
	setSQL := strings.Join(setClauses, ", ")

	var whereSQL string
	if filters != nil {
		var err error
		whereSQL, err = s.buildWhereClause(filters, &queryParams, mainTableAlias)
		if err != nil {
			return "", nil, fmt.Errorf("error building WHERE clause for update: %w", err)
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("UPDATE %s SET %s", quotedTableName, setSQL))
	if whereSQL != "" {
		sb.WriteString(" WHERE " + whereSQL)
	}

	dsl := query.Query{
		Filters: filters,
	}

	sql := sb.String() + ";"
	fmt.Printf("\n\nQuery\n %s \nSQL \n %s %v\n", dsl.MustToJSON(), sql, queryParams)
	return sql, queryParams, nil
}

// GenerateInsertSQL generates a SQL INSERT query.
func (s *SqliteQuery) GenerateInsertSQL(records []common.Document) (string, []any, error) {
	if len(records) == 0 {
		return "", nil, fmt.Errorf("no records provided for insert")
	}
	quotedTableName := quoteIdentifier(s.schema.Name)
	mainTableAlias := s.schema.Name // Use schema name as the main table alias

	fieldSet := make(map[string]bool)
	for _, record := range records {
		for fieldName := range record {
			if f := s.findField(fieldName); f == nil {
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
			// Qualify the field name for prepareValueForQuery
			qualifiedFieldName := mainTableAlias + "." + fieldName
			preparedValue, err := s.prepareValueForQuery(qualifiedFieldName, value, mainTableAlias)
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

// GenerateDeleteSQL generates a SQL DELETE query.
func (s *SqliteQuery) GenerateDeleteSQL(filters *query.QueryFilter, unsafeDelete bool) (string, []any, error) {
	quotedTableName := quoteIdentifier(s.schema.Name)
	mainTableAlias := s.schema.Name // Use schema name as the main table alias
	var queryParams []any

	if filters == nil && !unsafeDelete {
		return "", nil, fmt.Errorf("DELETE without WHERE clause is not allowed for safety. Set unsafeDelete=true to override")
	}

	var whereSQL string
	if filters != nil {
		var err error
		whereSQL, err = s.buildWhereClause(filters, &queryParams, mainTableAlias)
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

// prepareValueForQuery prepares a Go value for use as a SQL query parameter.
// It uses the schema for fields on the main table and makes safe assumptions for joined fields.
func (s *SqliteQuery) prepareValueForQuery(fieldPath string, value any, mainTableAlias string) (any, error) {
	if value == nil {
		return nil, nil
	}

	parts := strings.Split(fieldPath, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("field path must be qualified with a table alias (e.g., 'alias.field'); got '%s'", fieldPath)
	}
	tableAlias := parts[0]
	columnName := parts[1]

	if tableAlias == mainTableAlias {
		// We have the schema for the main table, so we can do detailed type preparation.
		field := s.findField(columnName)
		if field == nil {
			return nil, fmt.Errorf("field '%s' not found in schema for main table alias '%s'", columnName, mainTableAlias)
		}

		switch field.Type {
		case schema.FieldTypeString, schema.FieldTypeEnum:
			// Strings should be passed as-is to the SQL driver (it handles quoting)
			return value, nil

		case schema.FieldTypeNumber, schema.FieldTypeDecimal, schema.FieldTypeInteger:
			// Numeric types should be passed as-is
			return value, nil

		case schema.FieldTypeBoolean:
			// Convert boolean to integer for SQLite
			if boolVal, ok := value.(bool); ok {
				if boolVal {
					return 1, nil
				}
				return 0, nil
			}
			// Try to coerce the value using schema type coercion
			boolVal, ok := field.Type.Coerce(value)
			if ok {
				val := boolVal.(bool)
				if val {
					return 1, nil
				}
				return 0, nil
			}
			return nil, fmt.Errorf("expected boolean for FieldTypeBoolean, got %T for field '%s'", value, columnName)

		case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeSet, schema.FieldTypeRecord, schema.FieldTypeUnion:
			// Complex types should be JSON-encoded
			jsonBytes, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("failed to serialize field '%s' to JSON: %w", columnName, err)
			}
			return string(jsonBytes), nil

		default:
			// For unknown schema types, try to determine the correct handling based on Go type
			return s.handleValueByGoType(value, columnName)
		}
	}

	// For joined fields, we don't have schema info. Handle based on Go type.
	return s.handleValueByGoType(value, columnName)
}

// handleValueByGoType handles values based on their Go type when schema info is not available
func (s *SqliteQuery) handleValueByGoType(value any, fieldName string) (any, error) {
	switch v := value.(type) {
	case string:
		// Strings are passed as-is to the SQL driver
		return v, nil

	case bool:
		// Convert booleans to integers for SQLite
		if v {
			return 1, nil
		}
		return 0, nil

	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		// Numeric types are passed as-is
		return v, nil

	case map[string]any, []any:
		// Generic complex types - JSON encode
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize field '%s' to JSON: %w", fieldName, err)
		}
		return string(jsonBytes), nil

	case []string, []int, []int64, []float64, []bool:
		// Typed slices - JSON encode
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize field '%s' to JSON: %w", fieldName, err)
		}
		return string(jsonBytes), nil

	default:
		// Use reflection for other types
		return s.handleComplexTypeWithReflection(value, fieldName)
	}
}

// handleComplexTypeWithReflection uses reflection to handle complex types
func (s *SqliteQuery) handleComplexTypeWithReflection(value any, fieldName string) (any, error) {
	rv := reflect.ValueOf(value)
	rt := reflect.TypeOf(value)

	switch rt.Kind() {
	case reflect.String:
		return value, nil

	case reflect.Bool:
		if rv.Bool() {
			return 1, nil
		}
		return 0, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value, nil

	case reflect.Float32, reflect.Float64:
		return value, nil

	case reflect.Slice, reflect.Array, reflect.Map:
		// Complex types - JSON encode
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize complex field '%s' to JSON: %w", fieldName, err)
		}
		return string(jsonBytes), nil

	case reflect.Struct:
		// Handle special struct types
		if rt.String() == "time.Time" {
			if t, ok := value.(time.Time); ok {
				return t.Format(time.RFC3339), nil
			}
		}
		// For other structs, JSON-encode them
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize struct field '%s' to JSON: %w", fieldName, err)
		}
		return string(jsonBytes), nil

	case reflect.Ptr:
		if rv.IsNil() {
			return nil, nil
		}
		// Dereference pointer and handle the underlying value
		return s.handleValueByGoType(rv.Elem().Interface(), fieldName)

	default:
		// For any other type, try to pass it as-is
		return value, nil
	}
}
