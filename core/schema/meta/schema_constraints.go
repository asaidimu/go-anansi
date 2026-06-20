package meta

import "github.com/asaidimu/go-anansi/v6/core/schema/definition"

// MetaSchemaConstraints defines the semantic validation rules for schemas.
var MetaSchemaConstraints = map[definition.ConstraintId]definition.Constraint{
	ConstraintPrimitivesNoSchema: {
		Name: "primitives_no_schema",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "primitives_prohibit_schema",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintEnumFieldsValid: {
		Name: "enum_fields_valid",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "enum_fields_valid",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintArraysRequireSchema: {
		Name: "arrays_require_schema",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "collection_requires_schema",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintObjectsRequireSchema: {
		Name: "objects_require_schema",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "object_requires_schema",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintUnionsRequireMultipleSchemas: {
		Name: "unions_require_multiple_schemas",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "union_requires_multiple_schemas",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintCompositesRequireMultipleSchemas: {
		Name: "composites_require_multiple_schemas",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "composite_requires_multiple_schemas",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintNestedSchemaModeExclusive: {
		Name: "nested_schema_mode_exclusive",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "nested_schema_exclusive_mode",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintConstraintTypeExclusive: {
		Name: "constraint_type_exclusive",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "constraint_type_exclusive",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintConstraintRuleRequiresPredicate: {
		Name: "constraint_rule_requires_predicate",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "constraint_rule_requires_predicate",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintIndexConditionExclusiveType: {
		Name: "index_condition_exclusive_type",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "index_condition_type_exclusive",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintSchemaNameRequired: {
		Name: "schema_name_required",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "schema_name_required",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintFieldNameRequired: {
		Name: "field_name_required",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "field_name_required",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintIndexFieldsNotEmpty: {
		Name: "index_fields_not_empty",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "index_fields_not_empty",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintSchemaReferenceIdRequired: {
		Name: "schema_reference_id_required",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "schema_reference_id_required",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintRecordsAllowOptionalSchema: {
		Name: "records_allow_optional_schema",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "record_schema_cardinality",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintSchemaReferenceIntegrity: {
		Name: "schema_reference_integrity",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "schema_reference_exists",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintDefaultValueTypeMatch: {
		Name: "default_value_type_match",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "default_matches_type",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintIndexFieldsExist: {
		Name: "index_fields_exist",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "index_fields_reference_valid",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintConstraintFieldsExist: {
		Name: "constraint_fields_exist",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "constraint_fields_exist",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintIndexConditionValueTypeMatch: {
		Name: "index_condition_value_type_match",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "index_condition_value_matches_field_type",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintCompositeReferencedSchemasMustBeObjects: {
		Name: "composite_referenced_schemas_must_be_objects",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "composite_referenced_schemas_must_be_objects",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintObjectReferencedSchemaHasFields: {
		Name: "object_referenced_schema_has_fields",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "object_referenced_schema_has_fields",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintSpatialIndexOnGeometryField: {
		Name: "spatial_index_on_geometry_field",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "spatial_index_on_geometry_field",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintGlobalFieldIdUniqueness: {
		Name: "global_field_id_uniqueness",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "global_field_id_uniqueness",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintInlineTypeDescriptorValid: {
		Name: "inline_type_descriptor_valid",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "inline_type_descriptor_valid",
			Fields:    []definition.FieldName{},
		}),
	},
	ConstraintSchemaReferenceFormCorrect: {
		Name: "schema_reference_form_correct",
		ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
			Predicate: "schema_reference_form_correct",
			Fields:    []definition.FieldName{},
		}),
	},
}
