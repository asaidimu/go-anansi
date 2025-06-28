package schema


// SchemaMigrationHelper defines a helper for building schema migrations.
// It mirrors the TypeScript SchemaMigrationHelper's methods.
type SchemaMigrationHelper interface {
	AddField(fieldName string, fieldDefinition *FieldDefinition)
	RemoveField(fieldName string)
	DeprecateField(fieldName string)
	ModifyField(fieldName string, changes map[string]any) // `Partial<FieldDefinition<any>>` simplified to `map[string]any`
	AddIndex(indexDefinition IndexDefinition)
	RemoveIndex(indexName string)
	ModifyIndex(indexName string, changes map[string]any) // `Partial<IndexDefinition>` simplified to `map[string]any`
	AddConstraint(constraint any)                         // Can be `Constraint` or `ConstraintGroup`
	RemoveConstraint(constraintName string)
	ModifyConstraint(constraintName string, changes map[string]any) // `Partial<Constraint<any>>` simplified to `map[string]any`
	AddNestedSchema(schemaId string, nestedDefinition *NestedSchemaDefinition)
	RemoveNestedSchema(schemaId string)
	ModifyNestedSchema(schemaId string, changes map[string]any) // `Partial<NestedSchemaDefinition>` simplified to `map[string]any`
	Changes() (migrate []SchemaChange[any], rollback []SchemaChange[any])
}
