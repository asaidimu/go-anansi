// Package schema provides the SchemaMigrationHelper interface, which defines a set of
// methods for building schema migrations in a structured and programmatic way.
package schema

// SchemaMigrationHelper defines the interface for a helper that assists in the
// creation of schema migrations. It provides a fluent API for defining a series
// of changes to a schema, such as adding or removing fields, indexes, and
// constraints. Each method on the helper corresponds to a specific type of schema
// change, and the helper is responsible for generating the appropriate `SchemaChange`
// objects for both the forward migration and the reverse (rollback) migration.
type SchemaMigrationHelper interface {
	// AddField adds a new field to the schema.
	AddField(fieldName string, fieldDefinition *FieldDefinition)

	// RemoveField removes an existing field from the schema.
	RemoveField(fieldName string)

	// DeprecateField marks a field as deprecated, indicating that it should no longer be used.
	DeprecateField(fieldName string)

	// ModifyField modifies an existing field in the schema.
	ModifyField(fieldName string, changes map[string]any)

	// AddIndex adds a new index to the schema.
	AddIndex(indexDefinition IndexDefinition)

	// RemoveIndex removes an existing index from the schema.
	RemoveIndex(indexName string)

	// ModifyIndex modifies an existing index in the schema.
	ModifyIndex(indexName string, changes map[string]any)

	// AddConstraint adds a new constraint to the schema.
	AddConstraint(constraint any)

	// RemoveConstraint removes an existing constraint from the schema.
	RemoveConstraint(constraintName string)

	// ModifyConstraint modifies an existing constraint in the schema.
	ModifyConstraint(constraintName string, changes map[string]any)

	// AddNestedSchema adds a new nested schema to the schema.
	AddNestedSchema(schemaId string, nestedDefinition *NestedSchemaDefinition)

	// RemoveNestedSchema removes an existing nested schema from the schema.
	RemoveNestedSchema(schemaId string)

	// ModifyNestedSchema modifies an existing nested schema in the schema.
	ModifyNestedSchema(schemaId string, changes map[string]any)

	// Changes returns the list of schema changes for both the forward migration and the rollback.
	Changes() (migrate []SchemaChange[any], rollback []SchemaChange[any])
}
