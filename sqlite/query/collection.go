package query

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

func (f *sqliteFactory) buildCreateTableTree(q *query.Query) (SQLNode, error) {
	return &createTableTree{schema: q.Target.Schema}, nil
}

func (t *createTableTree) Value() (string, []any, error) {
	if t.schema == nil {
		return "", nil, fmt.Errorf("schema is not defined for create table tree")
	}

	sc := (*t.schema)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n", t.schema.Name))


	var columns []string
	var primaryKeys []string

	for _, index := range sc.Indexes {
		if index.Type == schema.IndexTypePrimary && len(index.Fields) > 0 {
			primaryKeys = index.Fields
			break
		}
	}

	for _, name := range sc.FieldNames() {
		field := sc.FindField(name)
		columnDef, err := t.buildColumnDefinition(name, field)
		if err != nil {
			return "", nil, fmt.Errorf("error on field '%s': %w", name, err)
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
func (s *createTableTree) buildColumnDefinition(fieldName string, field *schema.FieldDefinition) (string, error) {
	var parts []string
	parts = append(parts, quoteIdentifier(fieldName), s.getColumnType(field.Type))

	if field.Required != nil && *field.Required {
		parts = append(parts, "NOT NULL")
	}
	if field.Default != nil {
		defVal, err := s.formatDefaultValue(field.Default, field)
		if err != nil {
			return "", err
		}
		parts = append(parts, "DEFAULT "+defVal)
	}
	if field.Unique != nil && *field.Unique {
		parts = append(parts, "UNIQUE")
	}
	if field.Type == schema.FieldTypeEnum && len(field.Values) > 0 {
		var checkValues []string
		for _, v := range field.Values {
			valStr, _ := s.formatDefaultValue(v, &schema.FieldDefinition{Type: schema.FieldTypeString})
			checkValues = append(checkValues, valStr)
		}
		parts = append(parts, fmt.Sprintf("CHECK(%s IN (%s))", quoteIdentifier(fieldName), strings.Join(checkValues, ", ")))
	}
	return strings.Join(parts, " "), nil
}


func (s *createTableTree) formatDefaultValue(value any, fieldDef *schema.FieldDefinition) (string, error) {
	if value == nil {
		return "NULL", nil
	}

	result, err := toSQLiteValue(fieldDef, value)
	if err != nil {
		return "", err
	}

	switch fieldDef.Type {
	case schema.FieldTypeString, schema.FieldTypeEnum:
		resultS := result.(string)
		return fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", resultS), "'", "''")), nil
	case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeSet, schema.FieldTypeRecord, schema.FieldTypeUnion:
		resultS := result.(string)
		return fmt.Sprintf("'%s'", strings.ReplaceAll(resultS, "'", "''")), nil
	default:
		return fmt.Sprintf("%v", result), nil
	}
}

// getColumnType maps a schema.FieldType to its corresponding SQLite column type.
func (s *createTableTree) getColumnType(fieldType schema.FieldType) string {
	switch fieldType {
	case schema.FieldTypeString, schema.FieldTypeEnum:
		return "TEXT"
	case schema.FieldTypeNumber, schema.FieldTypeDecimal:
		return "REAL"
	case schema.FieldTypeInteger:
		return "INTEGER"
	case schema.FieldTypeBoolean:
		return "INTEGER"
	case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeSet, schema.FieldTypeRecord, schema.FieldTypeUnion:
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
		return "", nil, fmt.Errorf("schema is not defined for drop table tree")
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
		return "", nil, fmt.Errorf("table name is not defined for check table tree")
	}
	sql := "SELECT name FROM sqlite_master WHERE type='table' AND name=?"
	params := []any{t.name}
	return sql, params, nil
}
