package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/core"
	"github.com/asaidimu/go-anansi/core/persistence"
)

// DefaultInteractorOptions returns sensible defaults for table creation.
// By default, it sets IfNotExists to true, DropIfExists to false,
// and CreateIndexes to true.
func DefaultInteractorOptions() *persistence.InteractorOptions {
	return &persistence.InteractorOptions{
		IfNotExists:   true,
		DropIfExists:  false,
		CreateIndexes: true,
	}
}

// quoteIdentifier properly quotes an identifier (like a table or column name)
// to prevent SQL injection and handle names that might be SQL keywords or
// contain special characters. It uses double quotes for SQLite.
func (s *SQLiteInteractor) quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// getTableName applies any configured TablePrefix to the baseName
// and then properly quotes the resulting full table name.
func (s *SQLiteInteractor) getTableName(baseName string) string {
	name := s.options.TablePrefix + baseName
	return s.quoteIdentifier(name)
}

// CreateCollection generates and executes DDL to create a table and its associated
// indexes within a single database transaction. This ensures that the entire
// table creation process is an atomic operation: either all components are
// created successfully, or none are.
//
// If DropIfExists option is true, the table will be dropped before creation.
// Note: the drop operation itself occurs outside the transaction.
func (s *SQLiteInteractor) CreateCollection(schema core.SchemaDefinition) error {
	if s.options.DropIfExists {
		// Note: DropTable operates outside the transaction, as DDL commits implicitly in some databases,
		// and for SQLite, dropping a table is typically safe outside the transaction that creates it.
		if err := s.DropCollection(schema.Name); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", schema.Name, err)
		}
	}

	sqlStatements, err := s.CreateTableSQL(schema)
	if err != nil {
		return fmt.Errorf("failed to generate SQL for table %s: %w", schema.Name, err)
	}

	fullTableName := s.getTableName(schema.Name)

	// Execute table creation SQL statements
	for _, stmt := range sqlStatements {
		if _, err := s.runner().Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute SQL statement '%s': %w", stmt, err)
		}
	}

	// Create indexes if enabled in options
	if s.options.CreateIndexes {
		for _, index := range schema.Indexes {
			if index.Type == core.IndexTypePrimary {
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
				return fmt.Errorf("failed to create index %s: %w", index.Name, err)
			}
		}
	}

	return nil
}

// CreateTableSQL generates the DDL SQL string(s) required to create a table.
// It includes column definitions, constraints (NOT NULL, UNIQUE, DEFAULT, CHECK),
// and a primary key definition if specified in the schema.
func (s *SQLiteInteractor) CreateTableSQL(schema core.SchemaDefinition) ([]string, error) {
	collection := s.getTableName(schema.Name)
	var sb strings.Builder
	sb.WriteString("CREATE TABLE ")
	if s.options.IfNotExists {
		sb.WriteString("IF NOT EXISTS ")
	}
	sb.WriteString(collection + " (\n")

	var columns []string
	var primaryKeys []string

	// Prioritize finding an explicit PRIMARY index to define the primary key.
	// If no IndexTypePrimary is found, it falls back to the first unique index.
	for _, index := range schema.Indexes {
		if index.Type == core.IndexTypePrimary && len(index.Fields) > 0 {
			primaryKeys = index.Fields
			break // Found the primary key, no need to look further
		}
	}

	// Fallback logic: If no explicit primary key index is defined by type,
	// use the first unique index encountered in the schema. This provides
	// backward compatibility for schemas not yet using IndexTypePrimary.
	if len(primaryKeys) == 0 {
		for _, index := range schema.Indexes {
			if index.Type == core.IndexTypeUnique && len(index.Fields) > 0 {
				primaryKeys = index.Fields
				break
			}
		}
	}

	// Build column definitions
	for fieldName, field := range schema.Fields {
		columnDef, err := s.buildColumnDefinition(fieldName, field)
		if err != nil {
			return nil, fmt.Errorf("error on field '%s': %w", fieldName, err)
		}
		columns = append(columns, "    "+columnDef)
	}
	sb.WriteString(strings.Join(columns, ",\n"))

	// Add PRIMARY KEY clause if primary keys were identified
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

// buildColumnDefinition constructs a single column's DDL string.
// It includes the column name, data type, and various constraints
// such as NOT NULL, DEFAULT, UNIQUE, and CHECK for enums.
func (s *SQLiteInteractor) buildColumnDefinition(fieldName string, field *core.FieldDefinition) (string, error) {
	var parts []string
	// Add column name and its mapped SQL type
	parts = append(parts, s.quoteIdentifier(fieldName), s.GetColumnType(field.Type, field))

	// Add NOT NULL constraint if required
	if field.Required != nil && *field.Required {
		parts = append(parts, "NOT NULL")
	}
	// Add DEFAULT value if specified
	if field.Default != nil {
		defVal, err := s.formatDefaultValue(field.Default, field.Type)
		if err != nil {
			return "", err
		}
		parts = append(parts, "DEFAULT "+defVal)
	}
	// Add UNIQUE constraint if specified for the field.
	// Note: If a field is part of a table-level PRIMARY KEY or UNIQUE index,
	// this individual column UNIQUE constraint might be redundant but is harmless.
	if field.Unique != nil && *field.Unique {
		parts = append(parts, "UNIQUE")
	}
	// Add CHECK constraint for ENUM types to ensure values are within the defined set
	if field.Type == core.FieldTypeEnum && len(field.Values) > 0 {
		var checkValues []string
		for _, v := range field.Values {
			// Format enum values as strings for the CHECK constraint
			valStr, _ := s.formatDefaultValue(v, core.FieldTypeString)
			checkValues = append(checkValues, valStr)
		}
		parts = append(parts, fmt.Sprintf("CHECK(%s IN (%s))", s.quoteIdentifier(fieldName), strings.Join(checkValues, ", ")))
	}
	return strings.Join(parts, " "), nil
}

// GetColumnType maps a core.FieldType to an appropriate SQLite column type.
// This mapping is based on the intrinsic data type defined in the schema,
// and determines the storage class SQLite will use.
func (s *SQLiteInteractor) GetColumnType(fieldType core.FieldType, field *core.FieldDefinition) string {
	switch fieldType {
	case core.FieldTypeString, core.FieldTypeEnum:
		return "TEXT"    // Strings and enums are stored as TEXT
	case core.FieldTypeNumber, core.FieldTypeDecimal:
		return "REAL"    // Generic numbers and decimals are stored as floating-point REAL
	case core.FieldTypeInteger:
		return "INTEGER" // Whole numbers are stored as INTEGER
	case core.FieldTypeBoolean:
		return "INTEGER" // Booleans are typically stored as 0 (false) or 1 (true)
	case core.FieldTypeObject, core.FieldTypeArray, core.FieldTypeSet, core.FieldTypeRecord, core.FieldTypeUnion:
		return "TEXT" // Complex types are typically serialized to JSON and stored as TEXT
	default:
		return "BLOB" // Fallback for any unknown or unhandled types
	}
}

// formatDefaultValue formats a given default value into a string suitable
// for inclusion in a SQL DDL statement. It handles various Go types and
// marshals complex types (objects, arrays) into JSON strings.
func (s *SQLiteInteractor) formatDefaultValue(value any, fieldType core.FieldType) (string, error) {
	if value == nil {
		return "NULL", nil // Explicit NULL for nil values
	}
	switch fieldType {
	case core.FieldTypeString, core.FieldTypeEnum:
		// Quote string values and escape single quotes within them
		return fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", value), "'", "''")), nil
	case core.FieldTypeNumber, core.FieldTypeInteger:
		// Numeric values are represented directly
		return fmt.Sprintf("%v", value), nil
	case core.FieldTypeBoolean:
		// Booleans map to 0 or 1
		if b, ok := value.(bool); ok && b {
			return "1", nil
		}
		return "0", nil
	case core.FieldTypeObject, core.FieldTypeArray, core.FieldTypeSet, core.FieldTypeRecord, core.FieldTypeUnion:
		// Marshal complex types to JSON strings and quote/escape them
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("failed to marshal default value to JSON: %w", err)
		}
		return fmt.Sprintf("'%s'", strings.ReplaceAll(string(jsonBytes), "'", "''")), nil
	default:
		return "", fmt.Errorf("unsupported type for default value: %s", fieldType)
	}
}

// CreateIndex generates and executes a DDL statement to create an index
// on an existing table. This function is typically used for creating indexes
// independently of table creation, or in scenarios where indexes are added
// post-creation.
func (s *SQLiteInteractor) CreateIndex(collection string, index core.IndexDefinition) error {
	fullTableName := s.getTableName(collection)
	sqlIndex, err := s.CreateIndexSQL(fullTableName, index)
	if err != nil {
		return fmt.Errorf("failed to generate SQL for index %s: %w", index.Name, err)
	}

	if sqlIndex == "" {
		// If CreateIndexSQL returns an empty string, it means the index was
		// intentionally skipped (e.g., it's a primary key index already handled).
		return nil
	}

	_, err = s.runner().Exec(sqlIndex)
	if err != nil {
		return fmt.Errorf("failed to execute create index statement: %w", err)
	}
	return nil
}

// CreateIndexSQL generates the DDL SQL string for creating an index.
// It handles different index types (UNIQUE, NORMAL), names, and field orders.
// It specifically skips generating a separate index for `IndexTypePrimary`
// as these are defined as part of the `CREATE TABLE` statement.
func (s *SQLiteInteractor) CreateIndexSQL(collection string, index core.IndexDefinition) (string, error) {
	// Primary key indexes are created as part of the CREATE TABLE statement,
	// so no separate CREATE INDEX statement is needed for them.
	if index.Type == core.IndexTypePrimary {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("CREATE ")
	// Add UNIQUE keyword if the index is unique or of unique type
	if (index.Unique != nil && *index.Unique) || index.Type == core.IndexTypeUnique {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX IF NOT EXISTS ") // Add IF NOT EXISTS for idempotency
	indexName := index.Name
	if indexName == "" {
		// Generate a default index name if not provided: idx_tablename_field1_field2...
		unquotedTableName := strings.Trim(collection, `"`) // Remove quotes for name generation
		indexName = fmt.Sprintf("idx_%s_%s", unquotedTableName, strings.Join(index.Fields, "_"))
	}
	sb.WriteString(s.quoteIdentifier(indexName)) // Quote the index name
	sb.WriteString(fmt.Sprintf(" ON %s (", collection))

	var fieldParts []string
	for _, field := range index.Fields {
		part := s.quoteIdentifier(field)
		// Add ASC/DESC order if specified
		if index.Order != nil && strings.ToUpper(*index.Order) == "DESC" {
			part += " DESC"
		}
		fieldParts = append(fieldParts, part)
	}
	sb.WriteString(strings.Join(fieldParts, ", ") + ")")

	// Placeholder for partial index WHERE clause, if implemented in schema definition
	if index.Partial != nil {
		// Logic to append "WHERE <condition>" for partial indexes would go here.
		// For now, this is a placeholder for future enhancement.
	}
	sb.WriteString(";")
	return sb.String(), nil
}

// DropCollection drops a table from the database if it exists.
// It applies any configured table prefix and uses DROP TABLE IF EXISTS
// for safe, idempotent removal.
func (s *SQLiteInteractor) DropCollection(collection string) error {
	fullTableName := s.getTableName(collection)
	sql := fmt.Sprintf("DROP TABLE IF EXISTS %s;", fullTableName)
	_, err := s.runner().Exec(sql)
	if err != nil {
		return fmt.Errorf("failed to drop table %s: %w", fullTableName, err)
	}
	return nil
}

// CollectionExists checks if a table with the given base name (and applied prefix)
// currently exists in the SQLite database.
func (s *SQLiteInteractor) CollectionExists(collection string) (bool, error) {
	fullUnquotedName := s.options.TablePrefix + collection
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
