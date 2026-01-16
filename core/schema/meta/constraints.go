package meta

import "github.com/asaidimu/go-anansi/v6/core/schema/definition"

// MetaSchemaConstraints defines the semantic validation rules for schemas.
// These constraints encode the rules about when fields must/must not have schema references,
// what types require certain properties, and structural requirements.
var MetaSchemaConstraints = map[definition.ConstraintId]definition.Constraint{
	// Constraint 1: Primitives (string, number, integer, decimal, boolean, geometry) cannot have schema property
	"primitives_no_schema": {
		Name: "primitives_no_schema",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "primitives_prohibit_schema",
			Fields:    []definition.FieldName{"type", "schema"},
		}),
	},
	// Constraint 2: Enums must have schema reference
	"enums_require_schema": {
		Name:        "enums_require_schema",
		Description: "Enum types must have a schema reference",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "enum_requires_schema",
			Fields:    []definition.FieldName{"type", "schema"},
		}),
	},

	// Constraint 3: Arrays/Sets must have schema reference
	"arrays_require_schema": {
		Name:        "arrays_require_schema",
		Description: "Array and set types must have a schema reference",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "collection_requires_schema",
			Fields:    []definition.FieldName{"type", "schema"},
		}),
	},

	// Constraint 4: Objects must have schema reference
	"objects_require_schema": {
		Name:        "objects_require_schema",
		Description: "Object types must have a schema reference",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "object_requires_schema",
			Fields:    []definition.FieldName{"type", "schema"},
		}),
	},

	// Constraint 5: Unions must have multiple schema references (array)
	"unions_require_multiple_schemas": {
		Name:        "unions_require_multiple_schemas",
		Description: "Union types must have an array of schema references (at least 2)",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "union_requires_multiple_schemas",
			Fields:    []definition.FieldName{"type", "schema"},
		}),
	},

	// Constraint 6: Composites must have multiple schema references (array)
	"composites_require_multiple_schemas": {
		Name:        "composites_require_multiple_schemas",
		Description: "Composite types must have an array of schema references (at least 2)",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "composite_requires_multiple_schemas",
			Fields:    []definition.FieldName{"type", "schema"},
		}),
	},

	// Constraint 7: Enum schemas must have values array
	"enum_schemas_require_values": {
		Name:        "enum_schemas_require_values",
		Description: "NestedSchemas used for enum element types must have a non-empty values array",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "enum_schema_requires_values",
			Fields:    []definition.FieldName{"type", "values"},
		}),
	},

	// Constraint 8: NestedSchema mutual exclusivity - BaseSchema XOR FieldProperties
	"nested_schema_mode_exclusive": {
		Name:        "nested_schema_mode_exclusive",
		Description: "NestedSchema must use either BaseSchema fields (for objects) OR FieldProperties (for element types), not both or neither",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "nested_schema_exclusive_mode",
			Fields:    []definition.FieldName{"fields", "indexes", "constraints", "type", "default", "values", "schema"},
		}),
	},

	// Constraint 9: Constraint must be EITHER ConstraintRule OR ConstraintGroup, no mixing
	"constraint_exclusive_type": {
		Name:        "constraint_exclusive_type",
		Description: "Constraint must have either (predicate) OR (operator+rules), never both or neither",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "constraint_type_exclusive",
			Fields:    []definition.FieldName{"predicate", "operator", "rules"},
		}),
	},

	// Constraint 10: ConstraintRule must have predicate (required field enforcement)
	"constraint_rule_requires_predicate": {
		Name:        "constraint_rule_requires_predicate",
		Description: "If constraint is a ConstraintRule (has predicate), predicate field must not be empty",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "required_string_predicate",
			Fields:    []definition.FieldName{"predicate"},
		}),
	},

	// Constraint 11: ConstraintGroup must have operator and rules
	"constraint_group_requires_operator_and_rules": {
		Name:        "constraint_group_requires_operator_and_rules",
		Description: "If constraint is a ConstraintGroup (has operator), both operator and rules must be present and valid",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "constraint_group_complete",
			Fields:    []definition.FieldName{"operator", "rules"},
		}),
	},

	// Constraint 12: IndexCondition must be exclusive (either single condition OR group)
	"index_condition_exclusive_type": {
		Name:        "index_condition_exclusive_type",
		Description: "IndexCondition must have either (field+operator+value) OR (conditions+operator), never both",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "index_condition_type_exclusive",
			Fields:    []definition.FieldName{"field", "operator", "value", "conditions"},
		}),
	},

	// Constraint 13: Schema name must not be empty
	"schema_name_required": {
		Name:        "schema_name_required",
		Description: "Schema name must be a non-empty string",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "schema_name_required",
			Fields:    []definition.FieldName{"name"},
		}),
	},

	// Constraint 14: Field name must not be empty
	"field_name_required": {
		Name:        "field_name_required",
		Description: "Field name must be a non-empty string",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "field_name_required",
			Fields:    []definition.FieldName{"name"},
		}),
	},

	// Constraint 15: Index must have at least one field
	"index_fields_not_empty": {
		Name:        "index_fields_not_empty",
		Description: "Index must reference at least one field",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "index_fields_not_empty",
			Fields:    []definition.FieldName{"fields"},
		}),
	},

	// Constraint 16: SchemaReference ID must not be empty
	"schema_reference_id_required": {
		Name:        "schema_reference_id_required",
		Description: "SchemaReference must have a non-empty ID",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "schema_reference_id_required",
			Fields:    []definition.FieldName{"id"},
		}),
	},

	// NEW CONSTRAINT: Records allow optional single schema
	"records_allow_optional_schema": {
		Name:        "records_allow_optional_schema",
		Description: "Record types can have zero or one schema reference (not array)",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "record_schema_cardinality",
			Fields:    []definition.FieldName{"type", "schema"},
		}),
	},

	// NEW CONSTRAINT: Enum values must match declared type
	"enum_values_match_type": {
		Name:        "enum_values_match_type",
		Description: "Enum schema values must match the declared type (string or numeric)",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "enum_values_type_match",
			Fields:    []definition.FieldName{"type", "values"},
		}),
	},

	// RULE 15: Schema Reference Integrity - All schema references must resolve to existing schemas
	"schema_reference_integrity": {
		Name:        "schema_reference_integrity",
		Description: "All schema references must resolve to existing schemas in the schema hierarchy",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "schema_reference_exists",
			Fields:    []definition.FieldName{"schema"},
		}),
	},

	// RULE 16: Default Value Type Matching - Default values must match field types
	"default_value_type_match": {
		Name:        "default_value_type_match",
		Description: "Default values must match the declared field type",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "default_matches_type",
			Fields:    []definition.FieldName{"type", "default"},
		}),
	},

	// RULE 12: Index Field References - Index fields must reference existing field IDs
	"index_fields_exist": {
		Name:        "index_fields_exist",
		Description: "Index fields must reference existing field IDs in the schema",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "index_fields_reference_valid",
			Fields:    []definition.FieldName{"fields"},
		}),
	},

	// RULE 14: Constraint Field References - Constraint field paths must resolve to existing fields
	"constraint_fields_exist": {
		Name:        "constraint_fields_exist",
		Description: "Constraint field paths must resolve to existing fields",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "constraint_fields_reference_valid",
			Fields:    []definition.FieldName{"fields"},
		}),
	},

	// RULE 13: Index Condition Value Type Matching
	"index_condition_value_type_match": {
		Name:        "index_condition_value_type_match",
		Description: "Index condition values must match the referenced field's type",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "index_condition_value_matches_field_type",
			Fields:    []definition.FieldName{"field", "value"},
		}),
	},
}
