package query

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

func (f *sqliteFactory) buildCreateTableTree(q *query.Query) (SQLNode, error) {
	return &createTableTree{schema: q.Target.Schema}, nil
}

func (t *createTableTree) Value() (string, []any, error) {
	if t.schema == nil {
		return "", nil, ErrCollectionSchemaNotDefined
	}

	sc := (*t.schema)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n", t.schema.Name))


	var columns []string
	var primaryKeys []string

	for _, index := range sc.Indexes {
		if index.Type == definition.IndexTypePrimary && len(index.Fields) > 0 {
			for _, fieldName := range index.Fields {
				if _, field, ok := sc.GetFieldByName(fieldName); ok {
					primaryKeys = append(primaryKeys, string(field.Name))
				}
			}
			break
		}
	}

	for _, name := range sc.FieldNames() {
		_, field := sc.FindField(name)
		columnDef, err := t.buildColumnDefinition(name, field)
		if err != nil {
			return "", nil, ErrCollectionFieldError.WithCause(fmt.Errorf("error on field '%s': %w", name, err))
		}
		columns = append(columns, "    "+columnDef)
	}

	sb.WriteString(strings.Join(columns, ",\n"))

	if len(primaryKeys) > 0 {
		quotedPKs := make([]string, len(primaryKeys))
		for i, pk := range primaryKeys {
			quotedPKs[i] = quoteIdentifier(pk)
		}
		sb.WriteString(",\n    PRIMARY KEY (" + strings.Join(quotedPKs, ", ") + ")")
	}

	sb.WriteString("\n);")

	return sb.String(), nil, nil
}


// buildColumnDefinition constructs the DDL string for a single column, including its
// name, data type, and any constraints.
func (s *createTableTree) buildColumnDefinition(fieldName string, field *definition.Field) (string, error) {
	var parts []string
	parts = append(parts, quoteIdentifier(fieldName), s.getColumnType(field.Type))

	if field.Required {
		parts = append(parts, "NOT NULL")
	}
	if !field.Default.IsZero() && !field.Default.IsNull() {
		defVal, err := s.formatDefaultValue(field.Default, field)
		if err != nil {
			return "", err
		}
		parts = append(parts, "DEFAULT "+defVal)
	}
	if field.Unique {
		parts = append(parts, "UNIQUE")
	}

	// Handle enum CHECK constraint (supports multiple references)
	if field.Type == definition.FieldTypeEnum && !field.Schema.IsZero() {
		var allEnumValues []definition.LiteralValue

		// Collect values from either a single reference or multiple
		if field.Schema.IsSingle() {
			ref, err := definition.FieldSchemaAs[definition.SchemaReference](field.Schema)
			if err == nil {
				values, err := s.collectEnumValuesFromRef(ref)
				if err == nil {
					allEnumValues = append(allEnumValues, values...)
				}
			}
		} else if field.Schema.IsMultiple() {
			refs, err := definition.FieldSchemaAs[[]definition.SchemaReference](field.Schema)
			if err == nil {
				for _, ref := range refs {
					values, err := s.collectEnumValuesFromRef(ref)
					if err == nil {
						allEnumValues = append(allEnumValues, values...)
					}
				}
			}
		}

		if len(allEnumValues) > 0 {
			var checkValues []string
			for _, v := range allEnumValues {
				valStr, err := s.formatDefaultValue(v, &definition.Field{FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}})
				if err != nil {
					continue
				}
				checkValues = append(checkValues, valStr)
			}
			if len(checkValues) > 0 {
				parts = append(parts, fmt.Sprintf("CHECK(%s IN (%s))", quoteIdentifier(fieldName), strings.Join(checkValues, ", ")))
			}
		}
	}
	return strings.Join(parts, " "), nil
}

// Helper to collect LiteralValues from a single SchemaReference (named or inline)
func (s *createTableTree) collectEnumValuesFromRef(ref definition.SchemaReference) ([]definition.LiteralValue, error) {
	if ref.ID != "" {
		if nested, ok := s.schema.Schemas[ref.ID]; ok {
			return nested.Values, nil
		}
		return nil, fmt.Errorf("enum schema %s not found", ref.ID)
	} else if len(ref.Values) > 0 {
		return ref.Values, nil
	}
	return nil, fmt.Errorf("inline enum descriptor has no values")
}

func (s *createTableTree) formatDefaultValue(value definition.LiteralValue, fieldDef *definition.Field) (string, error) {
	if value.IsZero() || value.IsNull() {
		return "NULL", nil
	}

	result, err := toSQLiteValue(fieldDef, value.Value())
	if err != nil {
		return "", err
	}

	switch fieldDef.Type {
	case definition.FieldTypeString, definition.FieldTypeEnum:
		resultS := result.(string)
		return fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", resultS), "'", "''")), nil
	case definition.FieldTypeObject, definition.FieldTypeArray, definition.FieldTypeRecord, definition.FieldTypeUnion:
		resultS := result.(string)
		return fmt.Sprintf("'%s'", strings.ReplaceAll(resultS, "'", "''")), nil
	default:
		return fmt.Sprintf("%v", result), nil
	}
}

// getColumnType maps a definition.FieldType to its corresponding SQLite column type.
func (s *createTableTree) getColumnType(fieldType definition.FieldType) string {
	switch fieldType {
	case definition.FieldTypeString, definition.FieldTypeEnum:
		return "TEXT"
	case definition.FieldTypeNumber, definition.FieldTypeDecimal:
		return "REAL"
	case definition.FieldTypeInteger:
		return "INTEGER"
	case definition.FieldTypeBoolean:
		return "INTEGER"
	case definition.FieldTypeObject, definition.FieldTypeArray, definition.FieldTypeRecord, definition.FieldTypeUnion:
		return "TEXT"
	default:
		return "BLOB"
	}
}


func (f *sqliteFactory) buildAddColumnTree(q *query.Query, field definition.Field) (SQLNode, error) {
	return &alterTableAddColumnTree{collection: q.Target.Name, field: field}, nil
}

type alterTableAddColumnTree struct {
	collection string
	field      definition.Field
}

func (t *alterTableAddColumnTree) Value() (string, []any, error) {
	if t.collection == "" {
		return "", nil, ErrCollectionSchemaNotDefined
	}
	colDef, err := t.buildColumnDefinition(string(t.field.Name), &t.field)
	if err != nil {
		return "", nil, err
	}
	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", t.collection, colDef)
	return sql, nil, nil
}

func (t *alterTableAddColumnTree) buildColumnDefinition(fieldName string, field *definition.Field) (string, error) {
	var parts []string
	parts = append(parts, quoteIdentifier(fieldName), t.getColumnType(field.Type))

	if field.Required {
		parts = append(parts, "NOT NULL")
	}
	if !field.Default.IsZero() && !field.Default.IsNull() {
		defVal, err := t.formatDefaultValue(field.Default, field)
		if err != nil {
			return "", err
		}
		parts = append(parts, "DEFAULT "+defVal)
	}
	if field.Unique {
		parts = append(parts, "UNIQUE")
	}
	return strings.Join(parts, " "), nil
}

func (t *alterTableAddColumnTree) getColumnType(fieldType definition.FieldType) string {
	switch fieldType {
	case definition.FieldTypeString, definition.FieldTypeEnum:
		return "TEXT"
	case definition.FieldTypeNumber, definition.FieldTypeDecimal:
		return "REAL"
	case definition.FieldTypeInteger:
		return "INTEGER"
	case definition.FieldTypeBoolean:
		return "INTEGER"
	case definition.FieldTypeObject, definition.FieldTypeArray, definition.FieldTypeRecord, definition.FieldTypeUnion:
		return "TEXT"
	default:
		return "BLOB"
	}
}

func (t *alterTableAddColumnTree) formatDefaultValue(value definition.LiteralValue, fieldDef *definition.Field) (string, error) {
	if value.IsZero() || value.IsNull() {
		return "NULL", nil
	}
	result, err := toSQLiteValue(fieldDef, value.Value())
	if err != nil {
		return "", err
	}
	switch fieldDef.Type {
	case definition.FieldTypeString, definition.FieldTypeEnum:
		resultS := result.(string)
		return fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", resultS), "'", "''")), nil
	case definition.FieldTypeObject, definition.FieldTypeArray, definition.FieldTypeRecord, definition.FieldTypeUnion:
		resultS := result.(string)
		return fmt.Sprintf("'%s'", strings.ReplaceAll(resultS, "'", "''")), nil
	default:
		return fmt.Sprintf("%v", result), nil
	}
}

func (f *sqliteFactory) buildDropTableTree(q *query.Query) (SQLNode, error) {
	return &dropTableTree{name: q.Target.Name}, nil
}

func (t *dropTableTree) Value() (string, []any, error) {
	if len(t.name) ==0 {
		return "", nil, ErrCollectionSchemaNotDefined
	}

	return fmt.Sprintf("DROP TABLE IF EXISTS %s;", t.name), nil, nil
}

func (f *sqliteFactory) buildCheckTableTree(q *query.Query) (SQLNode, error) {
	return &checkTableTree{name: q.Target.Name}, nil
}

type checkTableTree struct {
	name string
}

func (t *checkTableTree) Value() (string, []any, error) {
	if len(t.name) == 0 {
		return "", nil, ErrCollectionTableNameNotDefined
	}
	sql := "SELECT name FROM sqlite_master WHERE type='table' AND name=?"
	params := []any{t.name}
	return sql, params, nil
}
