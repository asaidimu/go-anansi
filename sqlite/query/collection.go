package query

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
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
			for _, fieldId := range index.Fields {
				primaryKeys = append(primaryKeys, string(fieldId))
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

	if field.Type == definition.FieldTypeEnum && !field.Schema.IsZero() {
		ref, err := definition.FieldSchemaAs[definition.SchemaReference](field.Schema)
		if err == nil {
			if nested, ok := s.schema.Schemas[ref.ID]; ok && len(nested.Values) > 0 {
				var checkValues []string
				for _, v := range nested.Values {
					valStr, _ := s.formatDefaultValue(v, &definition.Field{FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}})
					checkValues = append(checkValues, valStr)
				}
				parts = append(parts, fmt.Sprintf("CHECK(%s IN (%s))", quoteIdentifier(fieldName), strings.Join(checkValues, ", ")))
			}
		}
	}
	return strings.Join(parts, " "), nil
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
	case definition.FieldTypeObject, definition.FieldTypeArray, definition.FieldTypeSet, definition.FieldTypeRecord, definition.FieldTypeUnion:
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
	case definition.FieldTypeObject, definition.FieldTypeArray, definition.FieldTypeSet, definition.FieldTypeRecord, definition.FieldTypeUnion:
		return "TEXT"
	default:
		return "BLOB"
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
