package meta

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

// MetaSchemaPredicates contains all predicate functions needed to validate schemas
var MetaSchemaPredicates = definition.PredicateMap{
	// Predicate 1: Primitives cannot have schema property
	"primitives_prohibit_schema": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		primitiveTypes := map[string]bool{
			"string": true, "number": true, "integer": true,
			"decimal": true, "boolean": true, "geometry": true,
		}

		typeVal, hasType := data["type"]
		schemaVal, hasSchema := data["schema"]

		if hasType {
			if typeStr, ok := typeVal.(string); ok {
				if primitiveTypes[typeStr] && hasSchema && schemaVal != nil {
					return []common.Issue{{
						Code:     "PRIMITIVE_HAS_SCHEMA",
						Message:  fmt.Sprintf("Primitive type '%s' cannot have a schema reference", typeStr),
						Severity: "error",
					}}
				}
			}
		}

		return nil
	},

	// Predicate 2: Enums must have schema reference
	"enum_requires_schema": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		typeVal, hasType := data["type"]
		schemaVal, hasSchema := data["schema"]

		if hasType {
			if typeStr, ok := typeVal.(string); ok {
				if typeStr == "enum" {
					if !hasSchema || schemaVal == nil {
						return []common.Issue{{
							Code:     "ENUM_MISSING_SCHEMA",
							Message:  "Enum type must have a schema reference",
							Severity: "error",
						}}
					}
				}
			}
		}

		return nil
	},

	// Predicate 3: Arrays/Sets must have schema reference
	"collection_requires_schema": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		collectionTypes := map[string]bool{"array": true, "set": true}

		typeVal, hasType := data["type"]
		schemaVal, hasSchema := data["schema"]

		if hasType {
			if typeStr, ok := typeVal.(string); ok {
				if collectionTypes[typeStr] {
					if !hasSchema || schemaVal == nil {
						return []common.Issue{{
							Code:     "COLLECTION_MISSING_SCHEMA",
							Message:  fmt.Sprintf("Collection type '%s' must have a schema reference", typeStr),
							Severity: "error",
						}}
					}
				}
			}
		}

		return nil
	},

	// Predicate 4: Objects must have schema reference
	"object_requires_schema": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		typeVal, hasType := data["type"]
		schemaVal, hasSchema := data["schema"]

		if hasType {
			if typeStr, ok := typeVal.(string); ok {
				if typeStr == "object" {
					if !hasSchema || schemaVal == nil {
						return []common.Issue{{
							Code:     "OBJECT_MISSING_SCHEMA",
							Message:  "Object type must have a schema reference",
							Severity: "error",
						}}
					}
				}
			}
		}

		return nil
	},

	// Predicate 5: Unions must have multiple schema references
	"union_requires_multiple_schemas": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		const minSchemas = 2

		typeVal, hasType := data["type"]
		schemaVal, hasSchema := data["schema"]

		if hasType {
			if typeStr, ok := typeVal.(string); ok {
				if typeStr == "union" {
					if !hasSchema || schemaVal == nil {
						return []common.Issue{{
							Code:     "UNION_MISSING_SCHEMA",
							Message:  "Union type must have schema references",
							Severity: "error",
						}}
					}

					if schemaArray, ok := schemaVal.([]any); ok {
						if len(schemaArray) < minSchemas {
							return []common.Issue{{
								Code:     "UNION_INSUFFICIENT_SCHEMAS",
								Message:  fmt.Sprintf("Union type must have at least %d schema references, got %d", minSchemas, len(schemaArray)),
								Severity: "error",
							}}
						}
					} else {
						return []common.Issue{{
							Code:     "UNION_SCHEMA_NOT_ARRAY",
							Message:  "Union type schema must be an array of schema references",
							Severity: "error",
						}}
					}
				}
			}
		}

		return nil
	},

	// Predicate 6: Composites must have multiple schema references
	"composite_requires_multiple_schemas": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		const minSchemas = 2

		typeVal, hasType := data["type"]
		schemaVal, hasSchema := data["schema"]

		if hasType {
			if typeStr, ok := typeVal.(string); ok {
				if typeStr == "composite" {
					if !hasSchema || schemaVal == nil {
						return []common.Issue{{
							Code:     "COMPOSITE_MISSING_SCHEMA",
							Message:  "Composite type must have schema references",
							Severity: "error",
						}}
					}

					if schemaArray, ok := schemaVal.([]any); ok {
						if len(schemaArray) < minSchemas {
							return []common.Issue{{
								Code:     "COMPOSITE_INSUFFICIENT_SCHEMAS",
								Message:  fmt.Sprintf("Composite type must have at least %d schema references, got %d", minSchemas, len(schemaArray)),
								Severity: "error",
							}}
						}
					} else {
						return []common.Issue{{
							Code:     "COMPOSITE_SCHEMA_NOT_ARRAY",
							Message:  "Composite type schema must be an array of schema references",
							Severity: "error",
						}}
					}
				}
			}
		}

		return nil
	},

	// Predicate 7: Enum schemas must have values array
	"enum_schema_requires_values": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		typeVal, hasType := data["type"]
		valuesVal, hasValues := data["values"]

		if hasType {
			if typeStr, ok := typeVal.(string); ok {
				if typeStr == "string" || typeStr == "integer" || typeStr == "number" {
					if hasValues {
						if valuesArray, ok := valuesVal.([]any); ok {
							if len(valuesArray) == 0 {
								return []common.Issue{{
									Code:     "ENUM_SCHEMA_EMPTY_VALUES",
									Message:  "Enum schema must have at least one value in values array",
									Severity: "error",
								}}
							}
						} else if valuesVal == nil {
							return []common.Issue{{
								Code:     "ENUM_SCHEMA_NULL_VALUES",
								Message:  "Enum schema values cannot be null",
								Severity: "error",
							}}
						}
					}
				}
			}
		}

		return nil
	},

	// Predicate 8: NestedSchema must use either BaseSchema OR FieldProperties
	"nested_schema_exclusive_mode": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		baseSchemaIndicators := []string{"fields", "indexes", "constraints"}
		fieldPropsIndicators := []string{"type", "default", "values", "schema"}

		hasBaseSchema := false
		for _, key := range baseSchemaIndicators {
			if val, exists := data[key]; exists && val != nil {
				if m, ok := val.(map[string]any); ok && len(m) > 0 {
					hasBaseSchema = true
					break
				}
			}
		}

		hasFieldProps := false
		for _, key := range fieldPropsIndicators {
			if val, exists := data[key]; exists && val != nil {
				hasFieldProps = true
				break
			}
		}

		if hasBaseSchema && hasFieldProps {
			return []common.Issue{{
				Code:     "NESTED_SCHEMA_MIXED_MODE",
				Message:  "NestedSchema cannot mix BaseSchema fields (fields/indexes/constraints) with FieldProperties (type/values/schema)",
				Severity: "error",
			}}
		}

		if !hasBaseSchema && !hasFieldProps {
			return []common.Issue{{
				Code:     "NESTED_SCHEMA_NO_MODE",
				Message:  "NestedSchema must have either BaseSchema fields or FieldProperties, not neither",
				Severity: "error",
			}}
		}

		return nil
	},

	// Predicate 9: Constraint must be EITHER rule OR group
	"constraint_type_exclusive": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		predicateVal, hasPredicate := data["predicate"]
		hasRuleFields := false
		if hasPredicate && predicateVal != nil {
			if str, ok := predicateVal.(string); ok && str != "" {
				hasRuleFields = true
			}
		}

		operatorVal, hasOperator := data["operator"]
		rulesVal, hasRules := data["rules"]
		hasGroupFields := false
		if hasOperator && operatorVal != nil && hasRules && rulesVal != nil {
			if arr, ok := rulesVal.([]any); ok && len(arr) > 0 {
				hasGroupFields = true
			}
		}

		if hasRuleFields && hasGroupFields {
			return []common.Issue{{
				Code:     "CONSTRAINT_MIXED_TYPE",
				Message:  "Constraint cannot have both predicate (rule) and operator+rules (group)",
				Severity: "error",
			}}
		}

		if !hasRuleFields && !hasGroupFields {
			return []common.Issue{{
				Code:     "CONSTRAINT_NO_TYPE",
				Message:  "Constraint must have either predicate (rule) or operator+rules (group)",
				Severity: "error",
			}}
		}

		return nil
	},

	// Predicate 10: ConstraintRule predicate must not be empty
	"required_string_predicate": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		val, exists := data["predicate"]
		if !exists {
			return []common.Issue{{
				Code:     "REQUIRED_FIELD_MISSING",
				Message:  "Required field 'predicate' is missing",
				Path:     "predicate",
				Severity: "error",
			}}
		}

		if val == nil {
			return []common.Issue{{
				Code:     "REQUIRED_FIELD_NULL",
				Message:  "Required field 'predicate' cannot be null",
				Path:     "predicate",
				Severity: "error",
			}}
		}

		strVal, ok := val.(string)
		if !ok {
			return []common.Issue{{
				Code:     "REQUIRED_FIELD_WRONG_TYPE",
				Message:  "Required field 'predicate' must be a string",
				Path:     "predicate",
				Severity: "error",
			}}
		}

		if strVal == "" {
			return []common.Issue{{
				Code:     "REQUIRED_FIELD_EMPTY",
				Message:  "Required field 'predicate' cannot be empty",
				Path:     "predicate",
				Severity: "error",
			}}
		}

		return nil
	},

	// Predicate 11: ConstraintGroup must have both operator and rules
	"constraint_group_complete": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		var issues []common.Issue

		operatorVal, hasOperator := data["operator"]
		if !hasOperator || operatorVal == nil {
			issues = append(issues, common.Issue{
				Code:     "CONSTRAINT_GROUP_MISSING_OPERATOR",
				Message:  "ConstraintGroup must have operator field",
				Severity: "error",
			})
		} else if strVal, ok := operatorVal.(string); !ok || strVal == "" {
			issues = append(issues, common.Issue{
				Code:     "CONSTRAINT_GROUP_INVALID_OPERATOR",
				Message:  "ConstraintGroup operator must be a non-empty string",
				Severity: "error",
			})
		}

		rulesVal, hasRules := data["rules"]
		if !hasRules || rulesVal == nil {
			issues = append(issues, common.Issue{
				Code:     "CONSTRAINT_GROUP_MISSING_RULES",
				Message:  "ConstraintGroup must have rules field",
				Severity: "error",
			})
		} else if rulesArr, ok := rulesVal.([]any); !ok {
			issues = append(issues, common.Issue{
				Code:     "CONSTRAINT_GROUP_RULES_NOT_ARRAY",
				Message:  "ConstraintGroup rules must be an array",
				Severity: "error",
			})
		} else if len(rulesArr) == 0 {
			issues = append(issues, common.Issue{
				Code:     "CONSTRAINT_GROUP_RULES_EMPTY",
				Message:  "ConstraintGroup rules array cannot be empty",
				Severity: "error",
			})
		}

		return issues
	},

	// Predicate 12: IndexCondition must be EITHER single OR group
	"index_condition_type_exclusive": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		singleFields := []string{"field", "operator", "value"}
		groupFields := []string{"conditions", "operator"}

		hasSingleFields := true
		for _, key := range singleFields {
			if val, exists := data[key]; !exists || val == nil {
				hasSingleFields = false
				break
			}
		}

		hasGroupFields := true
		for _, key := range groupFields {
			val, exists := data[key]
			if !exists || val == nil {
				hasGroupFields = false
				break
			}
			if key == "conditions" {
				if arr, ok := val.([]any); !ok || len(arr) == 0 {
					hasGroupFields = false
					break
				}
			}
		}

		if hasSingleFields && hasGroupFields {
			return []common.Issue{{
				Code:     "INDEX_CONDITION_MIXED_TYPE",
				Message:  "IndexCondition cannot have both single condition fields and group fields",
				Severity: "error",
			}}
		}

		if !hasSingleFields && !hasGroupFields {
			return []common.Issue{{
				Code:     "INDEX_CONDITION_NO_TYPE",
				Message:  "IndexCondition must be either a single condition or a group",
				Severity: "error",
			}}
		}

		return nil
	},

	// Predicate 13: Schema name must not be empty
	"schema_name_required": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		val, exists := data["name"]
		if !exists || val == nil {
			return []common.Issue{{
				Code:     "SCHEMA_NAME_MISSING",
				Message:  "Schema name is required",
				Path:     "name",
				Severity: "error",
			}}
		}

		strVal, ok := val.(string)
		if !ok || strVal == "" {
			return []common.Issue{{
				Code:     "SCHEMA_NAME_EMPTY",
				Message:  "Schema name must be a non-empty string",
				Path:     "name",
				Severity: "error",
			}}
		}

		return nil
	},

	// Predicate 14: Field name must not be empty
	"field_name_required": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		val, exists := data["name"]
		if !exists || val == nil {
			return []common.Issue{{
				Code:     "FIELD_NAME_MISSING",
				Message:  "Field name is required",
				Path:     "name",
				Severity: "error",
			}}
		}

		strVal, ok := val.(string)
		if !ok || strVal == "" {
			return []common.Issue{{
				Code:     "FIELD_NAME_EMPTY",
				Message:  "Field name must be a non-empty string",
				Path:     "name",
				Severity: "error",
			}}
		}

		return nil
	},

	// Predicate 15: Index must have at least one field
	"index_fields_not_empty": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		val, exists := data["fields"]
		if !exists || val == nil {
			return []common.Issue{{
				Code:     "INDEX_FIELDS_MISSING",
				Message:  "Index fields array is required",
				Path:     "fields",
				Severity: "error",
			}}
		}

		arrVal, ok := val.([]any)
		if !ok || len(arrVal) == 0 {
			return []common.Issue{{
				Code:     "INDEX_FIELDS_EMPTY",
				Message:  "Index must reference at least one field",
				Path:     "fields",
				Severity: "error",
			}}
		}

		return nil
	},

	// Predicate 16: SchemaReference ID must not be empty
	"schema_reference_id_required": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		val, exists := data["id"]
		if !exists || val == nil {
			return []common.Issue{{
				Code:     "SCHEMA_REFERENCE_ID_MISSING",
				Message:  "SchemaReference ID is required",
				Path:     "id",
				Severity: "error",
			}}
		}

		strVal, ok := val.(string)
		if !ok || strVal == "" {
			return []common.Issue{{
				Code:     "SCHEMA_REFERENCE_ID_INVALID",
				Message:  "SchemaReference ID must be a non-empty string",
				Path:     "id",
				Severity: "error",
			}}
		}

		return nil
	},
}
