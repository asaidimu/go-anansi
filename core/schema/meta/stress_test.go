package meta_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/schema/meta"
	"github.com/stretchr/testify/require"
)

func TestMetaSchema_StressPredicates(t *testing.T) {
	vd := meta.DevelopmentSchemaValidator()

	tests := []struct {
		name         string
		schema       map[string]any
		expectedCode string
	}{
		{
			name: "primitives_prohibit_schema",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "string",
						"schema": map[string]any{"id": "SomeSchema"},
					},
				},
			},
			expectedCode: "PRIMITIVE_HAS_SCHEMA",
		},
		{
			name: "enums_require_schema",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name": "f1",
						"type": "enum",
						// missing schema
					},
				},
			},
			expectedCode: "ENUM_MISSING_SCHEMA",
		},
		{
			name: "enum_schemas_require_values_missing",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "enum",
						"schema": map[string]any{"id": "MyEnum"},
					},
				},
				"schemas": map[string]any{
					"MyEnum": map[string]any{
						"name": "MyEnum",
						"type": "string",
						// missing values
					},
				},
			},
			expectedCode: "ENUM_NAMED_SCHEMA_MISSING_VALUES",
		},
		{
			name: "enum_schema_type_valid_invalid",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "enum",
						"schema": map[string]any{"id": "MyEnum"},
					},
				},
				"schemas": map[string]any{
					"MyEnum": map[string]any{
						"name":   "MyEnum",
						"type":   "boolean", // Invalid for enum
						"values": []any{true},
					},
				},
			},
			expectedCode: "ENUM_NAMED_SCHEMA_INVALID_TYPE",
		},
		{
			name: "enum_schema_type_valid_has_fields",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "enum",
						"schema": map[string]any{"id": "MyEnum"},
					},
				},
				"schemas": map[string]any{
					"MyEnum": map[string]any{
						"name":   "MyEnum",
						"type":   "string",
						"values": []any{"A"},
						"fields": map[string]any{"x": map[string]any{"name": "x", "type": "string"}},
					},
				},
			},
			// This triggers NESTED_SCHEMA_MIXED_MODE because NestedSchema (the variant for 'schemas' map values)
			// prohibits mixing BaseSchema (fields) with FieldProperties (type/values/schema).
			expectedCode: "NESTED_SCHEMA_MIXED_MODE",
		},
		{
			name: "composite_requires_multiple_schemas_insufficient",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "composite",
						"schema": []any{map[string]any{"id": "S1"}},
					},
				},
				"schemas": map[string]any{
					"S1": map[string]any{"name": "S1", "fields": map[string]any{"a": map[string]any{"name": "a", "type": "string"}}},
				},
			},
			expectedCode: "COMPOSITE_INSUFFICIENT_SCHEMAS",
		},
		{
			name: "composite_referenced_schemas_must_be_objects",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "composite",
						"schema": []any{map[string]any{"id": "S1"}, map[string]any{"id": "S2"}},
					},
				},
				"schemas": map[string]any{
					"S1": map[string]any{"name": "S1", "fields": map[string]any{"a": map[string]any{"name": "a", "type": "string"}}},
					"S2": map[string]any{"name": "S2", "type": "string"},
				},
			},
			expectedCode: "COMPOSITE_REF_NOT_OBJECT",
		},
		{
			name: "object_requires_schema",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name": "f1",
						"type": "object",
						// missing schema
					},
				},
			},
			expectedCode: "OBJECT_MISSING_SCHEMA",
		},
		{
			name: "object_referenced_schema_has_fields_none",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "object",
						"schema": map[string]any{"id": "S1"},
					},
				},
				"schemas": map[string]any{
					"S1": map[string]any{"name": "S1", "type": "string"},
				},
			},
			expectedCode: "OBJECT_REF_NO_FIELDS",
		},
		{
			name: "spatial_index_on_geometry_field",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{"name": "f1", "type": "string"},
				},
				"indexes": map[string]any{
					"idx1": map[string]any{
						"name":   "idx1",
						"type":   "spatial",
						"fields": []any{"f1"},
					},
				},
			},
			expectedCode: "SPATIAL_INDEX_NON_GEOMETRY",
		},
		{
			name: "index_condition_value_matches_field_type",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{"name": "f1", "type": "integer"},
				},
				"indexes": map[string]any{
					"idx1": map[string]any{
						"name":   "idx1",
						"type":   "normal",
						"fields": []any{"f1"},
						"condition": map[string]any{
							"field":    "f1",
							"operator": "eq",
							"value":    "not-an-integer",
						},
					},
				},
			},
			expectedCode: "INDEX_CONDITION_VALUE_TYPE_MISMATCH",
		},
		{
			name: "schema_reference_exists",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "object",
						"schema": map[string]any{"id": "NonExistent"},
					},
				},
			},
			expectedCode: "SCHEMA_REFERENCE_NOT_FOUND",
		},
		{
			name: "default_matches_type",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":    "f1",
						"type":    "boolean",
						"default": "not-a-bool",
					},
				},
			},
			expectedCode: "DEFAULT_VALUE_TYPE_MISMATCH",
		},
		{
			name: "global_field_id_uniqueness",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"duplicateID": map[string]any{"name": "f1", "type": "string"},
				},
				"schemas": map[string]any{
					"S1": map[string]any{
						"name": "S1",
						"fields": map[string]any{
							"duplicateID": map[string]any{"name": "f2", "type": "string"},
						},
					},
				},
			},
			expectedCode: "DUPLICATE_FIELD_ID",
		},
		{
			name: "constraint_fields_exist",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{"name": "f1", "type": "string"},
				},
				"constraints": map[string]any{
					"c1": map[string]any{
						"name":      "c1",
						"predicate": "p1",
						"fields":    []any{"non-existent-field"},
					},
				},
			},
			expectedCode: "CONSTRAINT_FIELD_NOT_FOUND",
		},
		{
			name: "inline_type_invalid_field_type",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "string", // string cannot have schema reference
						"schema": map[string]any{"values": []any{"A"}, "type": "string"},
					},
				},
			},
			expectedCode: "INLINE_DESCRIPTOR_NOT_ALLOWED_FOR_FIELD_TYPE",
		},
		{
			name: "inline_type_descriptor_no_type",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "enum", // string cannot have schema reference
						"schema": map[string]any{"values": []any{"A"}},
					},
				},
			},
			// The schema for Field.schema is a Union of SchemaReference, SchemaReferenceArray, and InlineTypeDescriptor.
			// If none match (InlineTypeDescriptor requires 'type'), we get UNION_MISMATCH.
			expectedCode: "UNION_MISMATCH",
		},
		{
			name: "schema_reference_form_correct_object",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "object",
						"schema": []any{map[string]any{"id": "S1"}},
					},
				},
				"schemas": map[string]any{
					"S1": map[string]any{"name": "S1", "fields": map[string]any{"a": map[string]any{"name": "a", "type": "string"}}},
				},
			},
			expectedCode: "SCHEMA_REF_FORM_INVALID",
		},
		{
			name: "collection_requires_schema",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name": "f1",
						"type": "array",
						// missing schema
					},
				},
			},
			expectedCode: "COLLECTION_MISSING_SCHEMA",
		},
		{
			name: "union_requires_multiple_schemas",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "union",
						"schema": []any{map[string]any{"id": "S1"}},
					},
				},
				"schemas": map[string]any{
					"S1": map[string]any{"name": "S1", "type": "string"},
				},
			},
			expectedCode: "UNION_INSUFFICIENT_SCHEMAS",
		},
		{
			name: "enum_values_match_type",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "enum",
						"schema": map[string]any{"id": "MyEnum"},
					},
				},
				"schemas": map[string]any{
					"MyEnum": map[string]any{
						"name":   "MyEnum",
						"type":   "integer",
						"values": []any{"not-an-integer"},
					},
				},
			},
			expectedCode: "ENUM_NAMED_VALUE_TYPE_MISMATCH",
		},
		{
			name: "record_schema_cardinality",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name":   "f1",
						"type":   "record",
						"schema": []any{map[string]any{"id": "S1"}},
					},
				},
				"schemas": map[string]any{
					"S1": map[string]any{"name": "S1", "type": "string"},
				},
			},
			expectedCode: "RECORD_SCHEMA_ARRAY",
		},
		{
			name: "nested_schema_exclusive_mode",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"schemas": map[string]any{
					"S1": map[string]any{
						"name":   "S1",
						"fields": map[string]any{"a": map[string]any{"name": "a", "type": "string"}},
						"type":   "string",
					},
				},
			},
			expectedCode: "NESTED_SCHEMA_MIXED_MODE",
		},
		{
			name: "constraint_type_exclusive",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{"name": "f1", "type": "string"},
				},
				"constraints": map[string]any{
					"c1": map[string]any{
						"name":      "c1",
						"predicate": "p1",
						"operator":  "and",
						"rules":     []any{map[string]any{"predicate": "p2"}},
					},
				},
			},
			expectedCode: "CONSTRAINT_MIXED_TYPE",
		},

		{
			name: "constraint_rule_requires_predicate_empty",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{"name": "f1", "type": "string"},
				},
				"constraints": map[string]any{
					"c1": map[string]any{
						"name":      "c1",
						"predicate": "",
					},
				},
			},
			// ConstraintUnion expects either ConstraintRule (predicate required) or ConstraintGroup (operator required).
			// If predicate is empty but present, and required in schema, it might trigger REQUIRED_FIELD_EMPTY if implemented,
			// or UNION_MISMATCH if the variant validator fails.
			expectedCode: "CONSTRAINT_NO_TYPE",
		},
		{
			name: "constraint_group_complete_missing_rules",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{"name": "f1", "type": "string"},
				},
				"constraints": map[string]any{
					"c1": map[string]any{
						"name":     "c1",
						"operator": "and",
						// missing rules
					},
				},
			},
			expectedCode: "REQUIRED_FIELD_MISSING",
		},
		{
			name: "index_condition_type_exclusive",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{"name": "f1", "type": "string"},
				},
				"indexes": map[string]any{
					"idx1": map[string]any{
						"name":   "idx1",
						"type":   "normal",
						"fields": []any{"f1"},
						"condition": map[string]any{
							"field":      "f1",
							"operator":   "eq",
							"value":      "v",
							"conditions": []any{map[string]any{"field": "f1", "operator": "eq", "value": "v2"}},
						},
					},
				},
			},
			expectedCode: "INDEX_CONDITION_MIXED_TYPE",
		},
		{
			name: "schema_name_required",
			schema: map[string]any{
				"version": "1.0.0",
				// missing name
			},
			expectedCode: "REQUIRED_FIELD_MISSING",
		},
		{
			name: "field_name_required",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"type": "string",
						// missing name
					},
				},
			},
			expectedCode: "REQUIRED_FIELD_MISSING",
		},
		{
			name: "index_fields_not_empty",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"indexes": map[string]any{
					"idx1": map[string]any{
						"name":   "idx1",
						"type":   "normal",
						"fields": []any{},
					},
				},
			},
			expectedCode: "INDEX_FIELDS_EMPTY",
		},
		{
			name: "schema_reference_id_required",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name": "f1",
						"type": "object",
						"schema": map[string]any{
							// missing id
							"indexes": map[string]any{},
						},
					},
				},
			},
			expectedCode: "REQUIRED_FIELD_MISSING",
		},
		{
			name: "index_fields_reference_valid",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{"name": "f1", "type": "string"},
				},
				"indexes": map[string]any{
					"idx1": map[string]any{
						"name":   "idx1",
						"type":   "normal",
						"fields": []any{"non-existent-field"},
					},
				},
			},
			expectedCode: "INDEX_FIELD_NOT_FOUND",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issues, ok := vd.Validate(tc.schema)
			if ok {
				t.Logf("Issues found: %v", issues)
			}
			require.False(t, ok, "Schema should be invalid")

			found := false
			for _, issue := range issues {
				if issue.Code == tc.expectedCode {
					found = true
					break
				}
			}
			require.True(t, found, "Expected issue code %s not found in issues: %v", tc.expectedCode, issues)
		})
	}
}

func TestMetaSchema_ValidSchemas(t *testing.T) {
	vd := meta.DevelopmentSchemaValidator()

	tests := []struct {
		name   string
		schema map[string]any
	}{
		{
			name: "valid_inline_array_string",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name": "f1",
						"type": "array",
						"schema": map[string]any{
							"type": "string",
						},
					},
				},
			},
		},
		{
			name: "valid_enum_with_values",
			schema: map[string]any{
				"name":    "Test",
				"version": "1.0.0",
				"fields": map[string]any{
					"f1": map[string]any{
						"name": "f1",
						"type": "enum",
						"schema": map[string]any{
							"type":   "string",
							"values": []any{"A", "B", "C"},
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issues, ok := vd.Validate(tc.schema)
			require.True(t, ok, "Schema should be valid, but got issues: %v", issues)
			require.Empty(t, issues)
		})
	}
}
