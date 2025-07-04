// Package sqlite provides the mapping logic from the abstract schema definition to
// concrete SQLite DDL (Data Definition Language). It is responsible for generating
// the SQL statements required to create tables, indexes, and other database objects.
package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/persistence"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// DefaultInteractorOptions returns a set of sensible default options for the
// SQLite interactor. These defaults are intended to provide a safe and common
// configuration for creating and managing database tables.
func DefaultInteractorOptions() *persistence.InteractorOptions {
	return &persistence.InteractorOptions{
		IfNotExists:   true, // Prevent errors if a table already exists.
		CreateIndexes: true, // Automatically create indexes defined in the schema.
	}
}

// quoteIdentifier safely quotes an identifier, such as a table or column name,
// to prevent SQL injection and to handle names that might be keywords or contain
// special characters.
func (s *SQLiteInteractor) quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// getTableName constructs the full, quoted table name by applying the configured
// table prefix to the base name.
func (s *SQLiteInteractor) getTableName(baseName string) string {
	name := s.options.CollectionPrefix + baseName
	return s.quoteIdentifier(name)
}

// CreateCollection generates and executes the DDL statements to create a table
// and its associated indexes. The entire process is transactional, ensuring that
// either all components are created successfully, or no changes are made.
func (s *SQLiteInteractor) CreateCollection(sc schema.SchemaDefinition) error {
	sqlStatements, err := s.CreateTableSQL(sc)
	if err != nil {
		return fmt.Errorf("failed to generate SQL for table %s: %w", sc.Name, err)
	}

	fullTableName := s.getTableName(sc.Name)

	for _, stmt := range sqlStatements {
		if _, err := s.runner().Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute SQL statement '%s': %w", stmt, err)
		}
	}

	if s.options.CreateIndexes {
		for _, index := range sc.Indexes {
			if index.Type == schema.IndexTypePrimary {
				continue
			}

			sqlIndex, err := s.CreateIndexSQL(fullTableName, index)
			if err != nil {
				return fmt.Errorf("failed to generate SQL for index %s: %w", index.Name, err)
			}
			if sqlIndex == "" {
				continue
			}
			if _, err := s.runner().Exec(sqlIndex); err != nil {
				return fmt.Errorf("failed to create index %s: %w \n %s \n", index.Name, err, sqlIndex)
			}
		}
	}

	return nil
}

// CreateTableSQL generates the DDL SQL statements required to create a table from a
// schema definition. It includes column definitions, constraints, and primary key
// definitions.
func (s *SQLiteInteractor) CreateTableSQL(sc schema.SchemaDefinition) ([]string, error) {
	collection := s.getTableName(sc.Name)
	var sb strings.Builder
	sb.WriteString("CREATE TABLE ")
	if s.options.IfNotExists {
		sb.WriteString("IF NOT EXISTS ")
	}
	sb.WriteString(collection + " (\n")

	var columns []string
	var primaryKeys []string

	for _, index := range sc.Indexes {
		if index.Type == schema.IndexTypePrimary && len(index.Fields) > 0 {
			primaryKeys = index.Fields
			break
		}
	}

	for _, field := range sc.Fields {
		columnDef, err := s.buildColumnDefinition(field.Name, field)
		if err != nil {
			return nil, fmt.Errorf("error on field '%s': %w", field.Name, err)
		}
		columns = append(columns, "    "+columnDef)
	}
	sb.WriteString(strings.Join(columns, ",\n"))

	if len(primaryKeys) > 0 {
		quotedPKs := make([]string, len(primaryKeys))
		for i, pk := range primaryKeys {
			quotedPKs[i] = s.quoteIdentifier(pk)
		}
		sb.WriteString(",\n    PRIMARY KEY (" + strings.Join(quotedPKs, ", ") + ")")
	}

	sb.WriteString("\n);")
	return []string{sb.String()}, nil
}

// buildColumnDefinition constructs the DDL string for a single column, including its
// name, data type, and any constraints.
func (s *SQLiteInteractor) buildColumnDefinition(fieldName string, field *schema.FieldDefinition) (string, error) {
	var parts []string
	parts = append(parts, s.quoteIdentifier(fieldName), s.GetColumnType(field.Type, field))

	if field.Required != nil && *field.Required {
		parts = append(parts, "NOT NULL")
	}
	if field.Default != nil {
		defVal, err := s.formatDefaultValue(field.Default, field.Type)
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
			valStr, _ := s.formatDefaultValue(v, schema.FieldTypeString)
			checkValues = append(checkValues, valStr)
		}
		parts = append(parts, fmt.Sprintf("CHECK(%s IN (%s))", s.quoteIdentifier(fieldName), strings.Join(checkValues, ", ")))
	}
	return strings.Join(parts, " "), nil
}

// GetColumnType maps a schema.FieldType to its corresponding SQLite column type.
func (s *SQLiteInteractor) GetColumnType(fieldType schema.FieldType, field *schema.FieldDefinition) string {
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

// formatDefaultValue formats a default value into a string suitable for use in a SQL DDL statement.
func (s *SQLiteInteractor) formatDefaultValue(value any, fieldType schema.FieldType) (string, error) {
	if value == nil {
		return "NULL", nil
	}
	switch fieldType {
	case schema.FieldTypeString, schema.FieldTypeEnum:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", value), "'", "''")), nil
	case schema.FieldTypeNumber, schema.FieldTypeInteger:
		return fmt.Sprintf("%v", value), nil
	case schema.FieldTypeBoolean:
		if b, ok := value.(bool); ok && b {
			return "1", nil
		}
		return "0", nil
	case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeSet, schema.FieldTypeRecord, schema.FieldTypeUnion:
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("failed to marshal default value to JSON: %w", err)
		}
		return fmt.Sprintf("'%s'", strings.ReplaceAll(string(jsonBytes), "'", "''")), nil
	default:
		return "", fmt.Errorf("unsupported type for default value: %s", fieldType)
	}
}

// CreateIndex generates and executes a DDL statement to create an index on a table.
func (s *SQLiteInteractor) CreateIndex(collection string, index schema.IndexDefinition) error {
	fullTableName := s.getTableName(collection)
	sqlIndex, err := s.CreateIndexSQL(fullTableName, index)
	if err != nil {
		return fmt.Errorf("failed to generate SQL for index %s: %w", index.Name, err)
	}

	if sqlIndex == "" {
		return nil
	}

	_, err = s.runner().Exec(sqlIndex)
	if err != nil {
		return fmt.Errorf("failed to execute create index statement: %w", err)
	}
	return nil
}

// CreateIndexSQL generates the DDL SQL string for creating an index.
func (s *SQLiteInteractor) CreateIndexSQL(collection string, index schema.IndexDefinition) (string, error) {
	if index.Type == schema.IndexTypePrimary {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("CREATE ")
	if (index.Unique != nil && *index.Unique) || index.Type == schema.IndexTypeUnique {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX IF NOT EXISTS ")
	indexName := index.Name
	if indexName == "" {
		unquotedTableName := strings.Trim(collection, `"`)
		indexName = fmt.Sprintf("idx_%s_%s", unquotedTableName, strings.Join(index.Fields, "_"))
	}
	sb.WriteString(s.quoteIdentifier(indexName))
	sb.WriteString(fmt.Sprintf(" ON %s (", collection))

	var fieldParts []string
	for _, field := range index.Fields {
		part := ""
		if strings.Contains(field, ".") {
			jsonPath := "$." + strings.ReplaceAll(field, ".", ".")
			part = fmt.Sprintf("json_extract(%s, '%s')", s.quoteIdentifier(field[:strings.Index(field, ".")]), jsonPath)
		} else {
			part = s.quoteIdentifier(field)
		}
		if index.Order != nil && strings.ToUpper(*index.Order) == "DESC" {
			part += " DESC"
		}
		fieldParts = append(fieldParts, part)
	}
	sb.WriteString(strings.Join(fieldParts, ", ") + ")")
	sb.WriteString(";")
	return sb.String(), nil
}

// DropCollection drops a table from the database.
func (s *SQLiteInteractor) DropCollection(collection string) error {
	fullTableName := s.getTableName(collection)
	sql := fmt.Sprintf("DROP TABLE IF EXISTS %s;", fullTableName)
	_, err := s.runner().Exec(sql)
	if err != nil {
		return fmt.Errorf("failed to drop table %s: %w", fullTableName, err)
	}
	return nil
}

// CollectionExists checks if a table exists in the database.
func (s *SQLiteInteractor) CollectionExists(collection string) (bool, error) {
	fullUnquotedName := s.options.CollectionPrefix + collection
	query := "SELECT name FROM sqlite_master WHERE type='table' AND name = ?;"

	var name string
	err := s.runner().QueryRow(query, fullUnquotedName).Scan(&name)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
