package meta

import "github.com/asaidimu/go-anansi/v6/core/schema/definition"

// MetaSchemaConstraints defines the semantic validation rules for schemas.
// These constraints encode the rules about when fields must/must not have schema references,
// what types require certain properties, and structural requirements.
var MetaSchemaConstraints = map[definition.ConstraintId]definition.Constraint{
	// Constraint 1: Primitives (string, number, integer, decimal, boolean, geometry) cannot have schema property
	"primitives_no_schema": {
		Name:        "primitives_no_schema",
		Description: "Primitive types (string, number, integer, decimal, boolean, geometry) must not have a schema reference",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "primitives_prohibit_schema",
		}),
	},

	// Constraint 2: Enums must have schema reference
	"enums_require_schema": {
		Name:        "enums_require_schema",
		Description: "Enum types must have a schema reference",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "enum_requires_schema",
		}),
	},

	// Constraint 3: Arrays/Sets must have schema reference
	"arrays_require_schema": {
		Name:        "arrays_require_schema",
		Description: "Array and set types must have a schema reference",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "collection_requires_schema",
		}),
	},

	// Constraint 4: Objects must have schema reference
	"objects_require_schema": {
		Name:        "objects_require_schema",
		Description: "Object types must have a schema reference",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "object_requires_schema",
		}),
	},

	// Constraint 5: Unions must have multiple schema references (array)
	"unions_require_multiple_schemas": {
		Name:        "unions_require_multiple_schemas",
		Description: "Union types must have an array of schema references (at least 2)",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "union_requires_multiple_schemas",
		}),
	},

	// Constraint 6: Composites must have multiple schema references (array)
	"composites_require_multiple_schemas": {
		Name:        "composites_require_multiple_schemas",
		Description: "Composite types must have an array of schema references (at least 2)",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "composite_requires_multiple_schemas",
		}),
	},

	// Constraint 7: Enum schemas must have values array
	"enum_schemas_require_values": {
		Name:        "enum_schemas_require_values",
		Description: "NestedSchemas used for enum element types must have a non-empty values array",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "enum_schema_requires_values",
		}),
	},

	// Constraint 8: NestedSchema mutual exclusivity - BaseSchema XOR FieldProperties
	"nested_schema_mode_exclusive": {
		Name:        "nested_schema_mode_exclusive",
		Description: "NestedSchema must use either BaseSchema fields (for objects) OR FieldProperties (for element types), not both or neither",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "nested_schema_exclusive_mode",
		}),
	},

	// Constraint 9: Constraint must be EITHER ConstraintRule OR ConstraintGroup, no mixing
	"constraint_exclusive_type": {
		Name:        "constraint_exclusive_type",
		Description: "Constraint must have either (predicate) OR (operator+rules), never both or neither",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "constraint_type_exclusive",
		}),
	},

	// Constraint 10: ConstraintRule must have predicate (required field enforcement)
	"constraint_rule_requires_predicate": {
		Name:        "constraint_rule_requires_predicate",
		Description: "If constraint is a ConstraintRule (has predicate), predicate field must not be empty",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "required_string_predicate",
		}),
	},

	// Constraint 11: ConstraintGroup must have operator and rules
	"constraint_group_requires_operator_and_rules": {
		Name:        "constraint_group_requires_operator_and_rules",
		Description: "If constraint is a ConstraintGroup (has operator), both operator and rules must be present and valid",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "constraint_group_complete",
		}),
	},

	// Constraint 12: IndexCondition must be exclusive (either single condition OR group)
	"index_condition_exclusive_type": {
		Name:        "index_condition_exclusive_type",
		Description: "IndexCondition must have either (field+operator+value) OR (conditions+operator), never both",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "index_condition_type_exclusive",
		}),
	},

	// Constraint 13: Schema name must not be empty
	"schema_name_required": {
		Name:        "schema_name_required",
		Description: "Schema name must be a non-empty string",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "schema_name_required",
		}),
	},

	// Constraint 14: Field name must not be empty
	"field_name_required": {
		Name:        "field_name_required",
		Description: "Field name must be a non-empty string",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "field_name_required",
		}),
	},

	// Constraint 15: Index must have at least one field
	"index_fields_not_empty": {
		Name:        "index_fields_not_empty",
		Description: "Index must reference at least one field",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "index_fields_not_empty",
		}),
	},

	// Constraint 16: SchemaReference ID must not be empty
	"schema_reference_id_required": {
		Name:        "schema_reference_id_required",
		Description: "SchemaReference must have a non-empty ID",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "schema_reference_id_required",
		}),
	},
}
