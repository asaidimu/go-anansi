package meta

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

var primitiveTypes = map[string]bool{
	"string": true, "number": true, "integer": true,
	"decimal": true, "boolean": true, "geometry": true,
}

var collectionTypes = map[string]bool{"array": true, "set": true}

var baseSchemaIndicators = []string{"fields", "indexes", "constraints"}
var fieldPropsIndicators = []string{"type", "default", "values", "schema"}

// Only check for types that can be used in enums
var numericTypes = map[string]bool{"number": true, "integer": true, "decimal": true, "string": true,}

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

	// Predicate 17: Record schema cardinality validation
	"record_schema_cardinality": func(params definition.PredicateParams) []common.Issue {
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
				if typeStr == "record" {
					if hasSchema && schemaVal != nil {
						// Check if schema is an array (which is invalid for record)
						if _, isArray := schemaVal.([]any); isArray {
							return []common.Issue{{
								Code:     "RECORD_SCHEMA_ARRAY",
								Message:  "Record type must have zero or one schema reference, not an array",
								Severity: "error",
							}}
						}
					}
				}
			}
		}

		return nil
	},

	// Predicate 18: Enum values must match declared type
	"enum_values_type_match": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{
				Code:    "INVALID_DATA_TYPE",
				Message: "Expected object data",
			}}
		}

		typeVal, hasType := data["type"]
		valuesVal, hasValues := data["values"]

		// Only validate if both type and values are present
		if !hasType || !hasValues || valuesVal == nil {
			return nil
		}

		typeStr, ok := typeVal.(string)
		if !ok {
			return nil
		}

		isNumeric := numericTypes[typeStr]
		isString := typeStr == "string"

		// If type is neither string nor numeric, skip validation
		if !isString && !isNumeric {
			return nil
		}

		valuesArray, ok := valuesVal.([]any)
		if !ok {
			return nil
		}

		var issues []common.Issue
		for i, value := range valuesArray {
			if value == nil {
				continue
			}

			if isString {
				if _, ok := value.(string); !ok {
					issues = append(issues, common.Issue{
						Code:     "ENUM_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Enum value at index %d must be string (declared type is 'string'), got %T", i, value),
						Path:     fmt.Sprintf("values[%d]", i),
						Severity: "error",
					})
				}
			} else if isNumeric {
				// Check if value is a numeric type
				switch value.(type) {
				case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8, float64, float32:
					// Valid numeric type
				default:
					issues = append(issues, common.Issue{
						Code:     "ENUM_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Enum value at index %d must be numeric (declared type is '%s'), got %T", i, typeStr, value),
						Path:     fmt.Sprintf("values[%d]", i),
						Severity: "error",
					})
				}
			}
		}

		return issues
	},

	// Predicate 19 (Rule 15): Schema Reference Integrity
	"schema_reference_exists": func(params definition.PredicateParams) []common.Issue {
		rootData, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}

		// Get the schemas map from root
		schemas, ok := rootData["schemas"].(map[string]any)
		if !ok {
			return nil
		}

		// Navigate to the field containing the schema reference
		// params.Keys tells us which field to check (e.g., ["schema"])
		if len(params.Keys) == 0 {
			return nil
		}

		// Get the schema field value from the current context
		// For Field validation, we need to look at fields.*.schema
		// The actual path depends on where this constraint is applied

		// Helper to validate a single schema reference
		validateRef := func(ref any, index int) []common.Issue {
			refMap, ok := ref.(map[string]any)
			if !ok {
				return nil
			}

			idVal, hasID := refMap["id"]
			if !hasID {
				return nil
			}

			idStr, ok := idVal.(string)
			if !ok {
				return nil
			}

			if _, exists := schemas[idStr]; !exists {
				path := "schema"
				if index >= 0 {
					path = fmt.Sprintf("schema[%d]", index)
				}
				return []common.Issue{{
					Code:     "SCHEMA_REFERENCE_NOT_FOUND",
					Message:  fmt.Sprintf("Schema reference '%s' does not exist in schema hierarchy", idStr),
					Path:     path,
					Severity: "error",
				}}
			}
			return nil
		}

		// This is tricky - we need to find all schema references in the document
		// For now, let's implement a recursive search
		var issues []common.Issue

		var checkSchemaRefs func(data any, path string)
		checkSchemaRefs = func(data any, path string) {
			switch v := data.(type) {
			case map[string]any:
				// Check if this is a schema reference
				if schemaVal, hasSchema := v["schema"]; hasSchema && schemaVal != nil {
					// Check if it's an array of references
					if refArray, ok := schemaVal.([]any); ok {
						for i, ref := range refArray {
							issues = append(issues, validateRef(ref, i)...)
						}
					} else {
						// Single reference
						issues = append(issues, validateRef(schemaVal, -1)...)
					}
				}

				// Recurse into nested maps
				for key, val := range v {
					newPath := path
					if newPath == "" {
						newPath = key
					} else {
						newPath = path + "." + key
					}
					checkSchemaRefs(val, newPath)
				}

			case []any:
				for i, item := range v {
					checkSchemaRefs(item, fmt.Sprintf("%s[%d]", path, i))
				}
			}
		}

		checkSchemaRefs(rootData, "")
		return issues
	},

	// Predicate 20 (Rule 16): Default Value Type Matching
	"default_matches_type": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}

		// This predicate is applied to Field definitions
		// We need to check all fields in the schema
		var issues []common.Issue

		var checkField func(fieldData map[string]any, fieldPath string)
		checkField = func(fieldData map[string]any, fieldPath string) {
			typeVal, hasType := fieldData["type"]
			defaultVal, hasDefault := fieldData["default"]

			if !hasType || !hasDefault || defaultVal == nil {
				return
			}

			typeStr, ok := typeVal.(string)
			if !ok {
				return
			}

			// Validate based on type
			switch typeStr {
			case "string":
				if _, ok := defaultVal.(string); !ok {
					issues = append(issues, common.Issue{
						Code:     "DEFAULT_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Default value must be string, got %T", defaultVal),
						Path:     fieldPath + ".default",
						Severity: "error",
					})
				}

			case "boolean":
				if _, ok := defaultVal.(bool); !ok {
					issues = append(issues, common.Issue{
						Code:     "DEFAULT_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Default value must be boolean, got %T", defaultVal),
						Path:     fieldPath + ".default",
						Severity: "error",
					})
				}

			case "integer":
				switch defaultVal.(type) {
				case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
					// Valid
				default:
					issues = append(issues, common.Issue{
						Code:     "DEFAULT_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Default value must be integer, got %T", defaultVal),
						Path:     fieldPath + ".default",
						Severity: "error",
					})
				}

			case "number", "decimal":
				switch defaultVal.(type) {
				case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8, float64, float32:
					// Valid
				default:
					issues = append(issues, common.Issue{
						Code:     "DEFAULT_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Default value must be numeric, got %T", defaultVal),
						Path:     fieldPath + ".default",
						Severity: "error",
					})
				}

			case "array", "set":
				if _, ok := defaultVal.([]any); !ok {
					issues = append(issues, common.Issue{
						Code:     "DEFAULT_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Default value must be array, got %T", defaultVal),
						Path:     fieldPath + ".default",
						Severity: "error",
					})
				}

			case "object", "record":
				if _, ok := defaultVal.(map[string]any); !ok {
					issues = append(issues, common.Issue{
						Code:     "DEFAULT_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Default value must be object, got %T", defaultVal),
						Path:     fieldPath + ".default",
						Severity: "error",
					})
				}

			case "geometry":
				outerArray, ok := defaultVal.([]any)
				if !ok {
					issues = append(issues, common.Issue{
						Code:     "DEFAULT_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Default value for geometry must be array of arrays, got %T", defaultVal),
						Path:     fieldPath + ".default",
						Severity: "error",
					})
					return
				}

				for i, inner := range outerArray {
					innerArray, ok := inner.([]any)
					if !ok {
						issues = append(issues, common.Issue{
							Code:     "DEFAULT_VALUE_TYPE_MISMATCH",
							Message:  fmt.Sprintf("Geometry inner element at index %d must be array, got %T", i, inner),
							Path:     fmt.Sprintf("%s.default[%d]", fieldPath, i),
							Severity: "error",
						})
						return
					}

					for j, elem := range innerArray {
						switch elem.(type) {
						case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8, float64, float32:
							// Valid
						default:
							issues = append(issues, common.Issue{
								Code:     "DEFAULT_VALUE_TYPE_MISMATCH",
								Message:  fmt.Sprintf("Geometry element at [%d][%d] must be numeric, got %T", i, j, elem),
								Path:     fmt.Sprintf("%s.default[%d][%d]", fieldPath, i, j),
								Severity: "error",
							})
						}
					}
				}
			}
		}

		// Check all fields
		if fields, ok := data["fields"].(map[string]any); ok {
			for fieldID, fieldData := range fields {
				if fieldMap, ok := fieldData.(map[string]any); ok {
					checkField(fieldMap, "fields."+fieldID)
				}
			}
		}

		// Check nested schemas
		if schemas, ok := data["schemas"].(map[string]any); ok {
			for schemaID, schemaData := range schemas {
				if schemaMap, ok := schemaData.(map[string]any); ok {
					// Check if nested schema has fields
					if nestedFields, ok := schemaMap["fields"].(map[string]any); ok {
						for fieldID, fieldData := range nestedFields {
							if fieldMap, ok := fieldData.(map[string]any); ok {
								checkField(fieldMap, fmt.Sprintf("schemas.%s.fields.%s", schemaID, fieldID))
							}
						}
					}
					// Check if nested schema has default (type mode)
					checkField(schemaMap, "schemas."+schemaID)
				}
			}
		}

		return issues
	},

	// Predicate 21 (Rule 12): Index Field References
	"index_fields_reference_valid": func(params definition.PredicateParams) []common.Issue {
		rootData, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}

		// Get schema fields
		schemaFields, ok := rootData["fields"].(map[string]any)
		if !ok {
			return nil
		}

		var issues []common.Issue

		// Check all indexes in the schema
		if indexes, ok := rootData["indexes"].(map[string]any); ok {
			for indexID, indexData := range indexes {
				indexMap, ok := indexData.(map[string]any)
				if !ok {
					continue
				}

				fieldsVal, ok := indexMap["fields"]
				if !ok {
					continue
				}

				fieldsArray, ok := fieldsVal.([]any)
				if !ok {
					continue
				}

				for i, fieldRef := range fieldsArray {
					fieldID, ok := fieldRef.(string)
					if !ok {
						continue
					}

					if _, exists := schemaFields[fieldID]; !exists {
						issues = append(issues, common.Issue{
							Code:     "INDEX_FIELD_NOT_FOUND",
							Message:  fmt.Sprintf("Index '%s' references non-existent field '%s'", indexID, fieldID),
							Path:     fmt.Sprintf("indexes.%s.fields[%d]", indexID, i),
							Severity: "error",
						})
					}
				}
			}
		}

		return issues
	},

	// Predicate 22 (Rule 14): Constraint Field References
	"constraint_fields_reference_valid": func(params definition.PredicateParams) []common.Issue {
		rootData, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}

		var issues []common.Issue

		// Helper to resolve a field path
		resolveFieldPath := func(path string, contextFields map[string]any) bool {
			parts := strings.Split(path, ".")
			current := contextFields

			for i, part := range parts {
				if field, exists := current[part]; exists {
					if i == len(parts)-1 {
						// Last part - found it
						return true
					}

					// Need to follow nested structure
					fieldMap, ok := field.(map[string]any)
					if !ok {
						return false
					}

					// Check if field has schema reference for nested object
					typeVal, hasType := fieldMap["type"]
					if hasType && typeVal == "object" {
						if schemaRef, hasSchema := fieldMap["schema"].(map[string]any); hasSchema {
							if schemaID, ok := schemaRef["id"].(string); ok {
								if schemas, ok := rootData["schemas"].(map[string]any); ok {
									if nestedSchema, exists := schemas[schemaID]; exists {
										nestedMap := nestedSchema.(map[string]any)
										if nestedFields, ok := nestedMap["fields"].(map[string]any); ok {
											current = nestedFields
											continue
										}
									}
								}
							}
						}
					}
					return false
				}
				return false
			}
			return false
		}

		// Check constraints
		var checkConstraints func(constraints map[string]any, contextFields map[string]any, basePath string)
		checkConstraints = func(constraints map[string]any, contextFields map[string]any, basePath string) {
			for constraintID, constraintData := range constraints {
				constraintMap, ok := constraintData.(map[string]any)
				if !ok {
					continue
				}

				// Check if it's a constraint rule (has predicate and fields)
				if predicateVal, hasPredicate := constraintMap["predicate"]; hasPredicate && predicateVal != nil {
					if fieldsVal, hasFields := constraintMap["fields"]; hasFields {
						if fieldsArray, ok := fieldsVal.([]any); ok {
							for i, fieldPath := range fieldsArray {
								pathStr, ok := fieldPath.(string)
								if !ok {
									continue
								}

								if !resolveFieldPath(pathStr, contextFields) {
									constraintPath := basePath + "constraints." + constraintID
									issues = append(issues, common.Issue{
										Code:     "CONSTRAINT_FIELD_NOT_FOUND",
										Message:  fmt.Sprintf("Constraint '%s' references non-existent field path '%s'", constraintID, pathStr),
										Path:     fmt.Sprintf("%s.fields[%d]", constraintPath, i),
										Severity: "error",
									})
								}
							}
						}
					}
				}
			}
		}

		// Check top-level constraints
		if constraints, ok := rootData["constraints"].(map[string]any); ok {
			if fields, ok := rootData["fields"].(map[string]any); ok {
				checkConstraints(constraints, fields, "")
			}
		}

		// Check nested schema constraints
		if schemas, ok := rootData["schemas"].(map[string]any); ok {
			for schemaID, schemaData := range schemas {
				schemaMap, ok := schemaData.(map[string]any)
				if !ok {
					continue
				}

				if constraints, ok := schemaMap["constraints"].(map[string]any); ok {
					if fields, ok := schemaMap["fields"].(map[string]any); ok {
						basePath := fmt.Sprintf("schemas.%s.", schemaID)
						checkConstraints(constraints, fields, basePath)
					}
				}
			}
		}

		return issues
	},

	// Predicate 23 (Rule 13): Index Condition Value Type Matching
	"index_condition_value_matches_field_type": func(params definition.PredicateParams) []common.Issue {
		rootData, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}

		schemaFields, ok := rootData["fields"].(map[string]any)
		if !ok {
			return nil
		}

		var issues []common.Issue

		// Helper to get field type
		getFieldType := func(fieldID string) string {
			if field, exists := schemaFields[fieldID]; exists {
				if fieldMap, ok := field.(map[string]any); ok {
					if typeVal, hasType := fieldMap["type"]; hasType {
						if typeStr, ok := typeVal.(string); ok {
							return typeStr
						}
					}
				}
			}
			return ""
		}

		// Helper to validate condition value matches type
		validateCondition := func(condition map[string]any, indexPath string) {
			fieldVal, hasField := condition["field"]
			valueVal, hasValue := condition["value"]

			if !hasField || !hasValue {
				return
			}

			fieldID, ok := fieldVal.(string)
			if !ok {
				return
			}

			fieldType := getFieldType(fieldID)
			if fieldType == "" {
				return // Field not found, will be caught by Rule 12
			}

			// Validate value matches field type
			switch fieldType {
			case "string":
				if _, ok := valueVal.(string); !ok {
					issues = append(issues, common.Issue{
						Code:     "INDEX_CONDITION_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Condition value for field '%s' must be string, got %T", fieldID, valueVal),
						Path:     indexPath + ".value",
						Severity: "error",
					})
				}

			case "boolean":
				if _, ok := valueVal.(bool); !ok {
					issues = append(issues, common.Issue{
						Code:     "INDEX_CONDITION_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Condition value for field '%s' must be boolean, got %T", fieldID, valueVal),
						Path:     indexPath + ".value",
						Severity: "error",
					})
				}

			case "integer":
				switch valueVal.(type) {
				case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
					// Valid
				default:
					issues = append(issues, common.Issue{
						Code:     "INDEX_CONDITION_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Condition value for field '%s' must be integer, got %T", fieldID, valueVal),
						Path:     indexPath + ".value",
						Severity: "error",
					})
				}

			case "number", "decimal":
				switch valueVal.(type) {
				case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8, float64, float32:
					// Valid
				default:
					issues = append(issues, common.Issue{
						Code:     "INDEX_CONDITION_VALUE_TYPE_MISMATCH",
						Message:  fmt.Sprintf("Condition value for field '%s' must be numeric, got %T", fieldID, valueVal),
						Path:     indexPath + ".value",
						Severity: "error",
					})
				}
			}
		}

		// Check all indexes with conditions
		if indexes, ok := rootData["indexes"].(map[string]any); ok {
			for indexID, indexData := range indexes {
				indexMap, ok := indexData.(map[string]any)
				if !ok {
					continue
				}

				if conditionVal, hasCondition := indexMap["condition"]; hasCondition && conditionVal != nil {
					if conditionMap, ok := conditionVal.(map[string]any); ok {
						indexPath := fmt.Sprintf("indexes.%s.condition", indexID)
						validateCondition(conditionMap, indexPath)

						// TODO: Handle nested condition groups recursively
					}
				}
			}
		}

		return issues
	},
}
