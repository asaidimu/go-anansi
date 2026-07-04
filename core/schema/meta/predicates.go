package meta

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

var primitiveTypes = map[string]bool{
	"string": true, "number": true, "integer": true,
	"decimal": true, "boolean": true, "geometry": true,
	"bytes": true, "unknown": true,
}

var collectionTypes = map[string]bool{"array": true}

var baseSchemaIndicators = []string{"fields"}
var fieldPropsIndicators = []string{"type", "default", "values", "schema"}

var numericTypes = map[string]bool{"number": true, "integer": true, "decimal": true, "string": true}

// ---------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------

// getFieldType returns the type string of a field definition and a boolean indicating presence.
func getFieldType(field map[string]any) (string, bool) {
	t, ok := field["type"]
	if !ok {
		return "", false
	}
	ts, ok := t.(string)
	return ts, ok
}

// isPrimitiveType checks if the type string is a primitive type.
func isPrimitiveType(t string) bool {
	return primitiveTypes[t]
}

// isNumericType checks if the type string is numeric (including string for enum values).
func isNumericType(t string) bool {
	return numericTypes[t]
}

// isCollectionType checks if the type is a collection (array, set).
func isCollectionType(t string) bool {
	return collectionTypes[t]
}

// getSchemaReference extracts schema reference(s) from a field definition.
// Returns a slice of schema IDs and a boolean indicating if the reference is an array.
func getSchemaReference(field map[string]any) (refs []string, isArray bool) {
	schemaVal, hasSchema := field["schema"]
	if !hasSchema || schemaVal == nil {
		return nil, false
	}

	switch v := schemaVal.(type) {
	case []any:
		isArray = true
		for _, ref := range v {
			if refMap, ok := ref.(map[string]any); ok {
				if idVal, ok := refMap["id"]; ok {
					if idStr, ok := idVal.(string); ok && idStr != "" {
						refs = append(refs, idStr)
					}
				}
			}
		}
	case map[string]any:
		if idVal, ok := v["id"]; ok {
			if idStr, ok := idVal.(string); ok && idStr != "" {
				refs = []string{idStr}
			}
		}
	}
	return refs, isArray
}

// getSchemaByID retrieves a schema definition from the root by its ID.
func getSchemaByID(root map[string]any, id string) (map[string]any, bool) {
	schemas, ok := root["schemas"].(map[string]any)
	if !ok {
		return nil, false
	}
	schema, ok := schemas[id]
	if !ok {
		return nil, false
	}
	schemaMap, ok := schema.(map[string]any)
	return schemaMap, ok
}

// isObjectLikeSchema checks if a schema definition represents an object type.
func isObjectLikeSchema(schema map[string]any) bool {
	// Has non-empty fields
	if fields, ok := schema["fields"]; ok {
		if fmap, ok := fields.(map[string]any); ok && len(fmap) > 0 {
			return true
		}
	}
	// Or type is object/record
	if t, ok := schema["type"]; ok {
		if ts, ok := t.(string); ok && (ts == "object" || ts == "record") {
			return true
		}
	}
	return false
}

func getSchemaReferenceObjects(field map[string]any) (refs []any, isArray bool) {
	schemaVal, hasSchema := field["schema"]
	if !hasSchema || schemaVal == nil {
		return nil, false
	}
	switch v := schemaVal.(type) {
	case []any:
		isArray = true
		refs = v
	case map[string]any:
		refs = []any{v}
	}
	return refs, isArray
}

// validateEnumReference validates a single enum reference (named or inline).
// Returns issues if invalid.
func validateEnumReference(ref any, fieldPath string, root map[string]any) []common.Issue {
	refMap, ok := ref.(map[string]any)
	if !ok {
		return []common.Issue{{
			Code:     "ENUM_REF_INVALID",
			Message:  "Enum schema reference must be an object",
			Path:     fieldPath,
			Severity: "error",
		}}
	}

	// Check if named or inline
	_, hasID := refMap["id"]
	_, hasType := refMap["type"]

	if hasID && hasType {
		return []common.Issue{{
			Code:     "ENUM_REF_AMBIGUOUS",
			Message:  "Enum schema reference cannot have both 'id' (named) and 'type' (inline)",
			Path:     fieldPath,
			Severity: "error",
		}}
	}

	if hasID {
		// Named reference
		idVal, ok := refMap["id"]
		if !ok {
			return []common.Issue{{
				Code:     "ENUM_NAMED_REF_MISSING_ID",
				Message:  "Named enum reference missing 'id' field",
				Path:     fieldPath,
				Severity: "error",
			}}
		}
		idStr, ok := idVal.(string)
		if !ok || idStr == "" {
			return []common.Issue{{
				Code:     "ENUM_NAMED_REF_INVALID_ID",
				Message:  "Named enum reference 'id' must be a non-empty string",
				Path:     fieldPath,
				Severity: "error",
			}}
		}
		// Validate the referenced schema
		return validateNamedEnumSchema(root, idStr, fieldPath)
	}

	if hasType {
		// Inline descriptor
		return validateInlineEnumDescriptor(refMap, fieldPath)
	}

	return []common.Issue{{
		Code:     "ENUM_REF_NO_ID_OR_TYPE",
		Message:  "Enum schema reference must have either 'id' (named) or 'type' (inline)",
		Path:     fieldPath,
		Severity: "error",
	}}
}

// validateNamedEnumSchema validates a named enum schema.
func validateNamedEnumSchema(root map[string]any, schemaID string, fieldPath string) []common.Issue {
	schemaMap, ok := getSchemaByID(root, schemaID)
	if !ok {
		return []common.Issue{{
			Code:     "ENUM_NAMED_SCHEMA_NOT_FOUND",
			Message:  fmt.Sprintf("Enum referenced schema '%s' does not exist", schemaID),
			Path:     fieldPath,
			Severity: "error",
		}}
	}

	var issues []common.Issue

	// Must have a 'type' (string or numeric)
	typeVal, hasType := schemaMap["type"]
	if !hasType || typeVal == nil {
		issues = append(issues, common.Issue{
			Code:     "ENUM_NAMED_SCHEMA_NO_TYPE",
			Message:  fmt.Sprintf("Enum referenced schema '%s' must have a 'type' (string or numeric)", schemaID),
			Path:     fieldPath,
			Severity: "error",
		})
		return issues
	}
	typeStr, ok := typeVal.(string)
	if !ok || !(typeStr == "string" || isNumericType(typeStr)) {
		issues = append(issues, common.Issue{
			Code:     "ENUM_NAMED_SCHEMA_INVALID_TYPE",
			Message:  fmt.Sprintf("Enum referenced schema '%s' type must be string or numeric, got '%v'", schemaID, typeVal),
			Path:     fieldPath,
			Severity: "error",
		})
		return issues
	}

	// Must have non-empty 'values'
	valuesVal, hasValues := schemaMap["values"]
	if !hasValues || valuesVal == nil {
		issues = append(issues, common.Issue{
			Code:     "ENUM_NAMED_SCHEMA_MISSING_VALUES",
			Message:  fmt.Sprintf("Enum schema '%s' must have a 'values' array", schemaID),
			Path:     fieldPath,
			Severity: "error",
		})
		return issues
	}
	valuesArr, ok := valuesVal.([]any)
	if !ok || len(valuesArr) == 0 {
		issues = append(issues, common.Issue{
			Code:     "ENUM_NAMED_SCHEMA_EMPTY_VALUES",
			Message:  fmt.Sprintf("Enum schema '%s' 'values' must be a non-empty array", schemaID),
			Path:     fieldPath,
			Severity: "error",
		})
		return issues
	}

	// Check value types match the schema type
	isStringEnum := typeStr == "string"
	for i, val := range valuesArr {
		if val == nil {
			continue
		}
		if isStringEnum {
			if _, ok := val.(string); !ok {
				issues = append(issues, common.Issue{
					Code:     "ENUM_NAMED_VALUE_TYPE_MISMATCH",
					Message:  fmt.Sprintf("Enum schema '%s': value at index %d must be string, got %T", schemaID, i, val),
					Path:     fmt.Sprintf("%s (values[%d])", fieldPath, i),
					Severity: "error",
				})
			}
		} else if isNumericType(typeStr) {
			switch val.(type) {
			case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8, float64, float32:
				// valid
			default:
				issues = append(issues, common.Issue{
					Code:     "ENUM_NAMED_VALUE_TYPE_MISMATCH",
					Message:  fmt.Sprintf("Enum schema '%s': value at index %d must be numeric (type '%s'), got %T", schemaID, i, typeStr, val),
					Path:     fmt.Sprintf("%s (values[%d])", fieldPath, i),
					Severity: "error",
				})
			}
		}
	}

	// Ensure enum schema has no base schema indicators (fields, indexes, constraints)
	for _, key := range baseSchemaIndicators {
		if val, exists := schemaMap[key]; exists && val != nil {
			if m, ok := val.(map[string]any); ok && len(m) > 0 {
				issues = append(issues, common.Issue{
					Code:     "ENUM_NAMED_SCHEMA_HAS_FIELDS",
					Message:  fmt.Sprintf("Enum referenced schema '%s' must not have '%s' (must be a simple value schema)", schemaID, key),
					Path:     fieldPath,
					Severity: "error",
				})
			}
		}
	}

	return issues
}

// validateInlineEnumDescriptor validates an inline enum descriptor.
func validateInlineEnumDescriptor(refMap map[string]any, fieldPath string) []common.Issue {
	var issues []common.Issue

	// Must have a 'type' (string or numeric)
	typeVal, hasType := refMap["type"]
	if !hasType || typeVal == nil {
		issues = append(issues, common.Issue{
			Code:     "ENUM_INLINE_NO_TYPE",
			Message:  "Inline enum descriptor must have a 'type' (string or numeric)",
			Path:     fieldPath,
			Severity: "error",
		})
		return issues
	}
	typeStr, ok := typeVal.(string)
	if !ok || !(typeStr == "string" || isNumericType(typeStr)) {
		issues = append(issues, common.Issue{
			Code:     "ENUM_INLINE_INVALID_TYPE",
			Message:  fmt.Sprintf("Inline enum descriptor type must be string or numeric, got '%v'", typeVal),
			Path:     fieldPath,
			Severity: "error",
		})
		return issues
	}

	// Must have non-empty 'values'
	valuesVal, hasValues := refMap["values"]
	if !hasValues || valuesVal == nil {
		issues = append(issues, common.Issue{
			Code:     "ENUM_INLINE_MISSING_VALUES",
			Message:  "Inline enum descriptor must have a 'values' array",
			Path:     fieldPath,
			Severity: "error",
		})
		return issues
	}
	valuesArr, ok := valuesVal.([]any)
	if !ok || len(valuesArr) == 0 {
		issues = append(issues, common.Issue{
			Code:     "ENUM_INLINE_EMPTY_VALUES",
			Message:  "Inline enum descriptor 'values' must be a non-empty array",
			Path:     fieldPath,
			Severity: "error",
		})
		return issues
	}

	// Check value types match the declared type
	isStringEnum := typeStr == "string"
	for i, val := range valuesArr {
		if val == nil {
			continue
		}
		if isStringEnum {
			if _, ok := val.(string); !ok {
				issues = append(issues, common.Issue{
					Code:     "ENUM_INLINE_VALUE_TYPE_MISMATCH",
					Message:  fmt.Sprintf("Inline enum value at index %d must be string, got %T", i, val),
					Path:     fmt.Sprintf("%s.values[%d]", fieldPath, i),
					Severity: "error",
				})
			}
		} else if isNumericType(typeStr) {
			switch val.(type) {
			case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8, float64, float32:
				// valid
			default:
				issues = append(issues, common.Issue{
					Code:     "ENUM_INLINE_VALUE_TYPE_MISMATCH",
					Message:  fmt.Sprintf("Inline enum value at index %d must be numeric (type '%s'), got %T", i, typeStr, val),
					Path:     fmt.Sprintf("%s.values[%d]", fieldPath, i),
					Severity: "error",
				})
			}
		}
	}

	return issues
}

// ---------------------------------------------------------------------
// MetaSchemaPredicates contains all predicate functions for schema validation
// ---------------------------------------------------------------------
var MetaSchemaPredicates = definition.PredicateMap{
	"primitives_prohibit_schema": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		typeVal, hasType := data["type"]
		schemaVal, hasSchema := data["schema"]
		if hasType {
			if typeStr, ok := typeVal.(string); ok && isPrimitiveType(typeStr) && hasSchema && schemaVal != nil {
				return []common.Issue{{
					Code:     "PRIMITIVE_HAS_SCHEMA",
					Message:  fmt.Sprintf("Primitive type '%s' cannot have a schema reference", typeStr),
					Severity: "error",
				}}
			}
		}
		return nil
	},

	"enum_fields_valid": func(params definition.PredicateParams) []common.Issue {
		field, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		typ, ok := getFieldType(field)
		if !ok || typ != "enum" {
			return nil
		}

		// Check schema exists
		schemaVal, hasSchema := field["schema"]
		if !hasSchema || schemaVal == nil {
			return []common.Issue{{
				Code:     "ENUM_MISSING_SCHEMA",
				Message:  "Enum field must have a schema reference",
				Severity: "error",
			}}
		}

		// Get raw references
		refs, isArray := getSchemaReferenceObjects(field)
		if len(refs) == 0 {
			return []common.Issue{{
				Code:     "ENUM_NO_VALID_REFS",
				Message:  "Enum field has no valid schema references",
				Severity: "error",
			}}
		}

		// For single reference (non-array), we still allow it; for array, we allow multiple.
		var allIssues []common.Issue
		for i, ref := range refs {
			refPath := "schema"
			if isArray {
				refPath = fmt.Sprintf("schema[%d]", i)
			}
			issues := validateEnumReference(ref, refPath, params.Root)
			allIssues = append(allIssues, issues...)
		}
		return allIssues
	},

	"composite_requires_multiple_schemas": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		const minSchemas = 2
		typeVal, hasType := data["type"]
		schemaVal, hasSchema := data["schema"]
		if hasType {
			if typeStr, ok := typeVal.(string); ok && typeStr == "composite" {
				if !hasSchema || schemaVal == nil {
					return []common.Issue{{
						Code:     "COMPOSITE_MISSING_SCHEMA",
						Message:  "Composite type must have schema references",
						Severity: "error",
					}}
				}
				if arr, ok := schemaVal.([]any); ok {
					if len(arr) < minSchemas {
						return []common.Issue{{
							Code:     "COMPOSITE_INSUFFICIENT_SCHEMAS",
							Message:  fmt.Sprintf("Composite must have at least %d schema references", minSchemas),
							Severity: "error",
						}}
					}
				} else {
					return []common.Issue{{
						Code:     "COMPOSITE_SCHEMA_NOT_ARRAY",
						Message:  "Composite 'schema' must be an array of references",
						Severity: "error",
					}}
				}
			}
		}
		return nil
	},

	"composite_referenced_schemas_must_be_objects": func(params definition.PredicateParams) []common.Issue {
		fieldData, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}

		typ, ok := getFieldType(fieldData)
		if !ok || typ != "composite" {
			return nil
		}

		refs, isArray := getSchemaReference(fieldData)
		if !isArray || len(refs) == 0 {
			return nil
		}

		root := params.Root
		var issues []common.Issue

		var validateSchemaIsObjectLike func(schemaID string, fieldPath string) []common.Issue
		validateSchemaIsObjectLike = func(schemaID string, fieldPath string) []common.Issue {
			schemaMap, ok := getSchemaByID(root, schemaID)
			if !ok {
				return nil
			}

			var currentIssues []common.Issue

			// If it's a composite, skip (will be validated elsewhere)
			if t, ok := schemaMap["type"]; ok {
				if ts, ok := t.(string); ok && ts == "composite" {
					return nil
				}
				// If it's a union, recursively check its variants
				if t == "union" {
					unionRefs, _ := getSchemaReference(schemaMap)
					for i, unionID := range unionRefs {
						currentIssues = append(currentIssues, validateSchemaIsObjectLike(unionID, fmt.Sprintf("%s.schema[%d]", fieldPath, i))...)
					}
					return currentIssues
				}
			}

			if !isObjectLikeSchema(schemaMap) {
				currentIssues = append(currentIssues, common.Issue{
					Code:     "COMPOSITE_REF_NOT_OBJECT",
					Message:  fmt.Sprintf("Composite schema '%s' must effectively represent an object type (has fields, or type 'object'/'record')", schemaID),
					Path:     fieldPath,
					Severity: "error",
				})
			}
			return currentIssues
		}

		for i, schemaID := range refs {
			issues = append(issues, validateSchemaIsObjectLike(schemaID, fmt.Sprintf("schema[%d]", i))...)
		}
		return issues
	},

	"object_requires_schema": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		typeVal, hasType := data["type"]
		schemaVal, hasSchema := data["schema"]
		if hasType {
			if typeStr, ok := typeVal.(string); ok && typeStr == "object" {
				if !hasSchema || schemaVal == nil {
					return []common.Issue{{
						Code:     "OBJECT_MISSING_SCHEMA",
						Message:  "Object type must have a schema reference",
						Severity: "error",
					}}
				}
			}
		}
		return nil
	},

	"object_referenced_schema_has_fields": func(params definition.PredicateParams) []common.Issue {
		root := params.Root
		fieldData, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		typ, ok := getFieldType(fieldData)
		if !ok || typ != "object" {
			return nil
		}
		refs, _ := getSchemaReference(fieldData)
		if len(refs) == 0 {
			return nil
		}
		schemaID := refs[0]
		schemaMap, ok := getSchemaByID(root, schemaID)
		if !ok {
			return nil
		}
		if fields, ok := schemaMap["fields"]; !ok {
			return []common.Issue{{
				Code:     "OBJECT_REF_NO_FIELDS",
				Message:  fmt.Sprintf("Object field references schema '%s' which has no 'fields' (must be Schema mode)", schemaID),
				Severity: "error",
			}}
		} else if fmap, ok := fields.(map[string]any); !ok || len(fmap) == 0 {
			return []common.Issue{{
				Code:     "OBJECT_REF_EMPTY_FIELDS",
				Message:  fmt.Sprintf("Object field references schema '%s' which has empty fields", schemaID),
				Severity: "error",
			}}
		}
		return nil
	},

	"spatial_index_on_geometry_field": func(params definition.PredicateParams) []common.Issue {
		root := params.Root
		indexData := params.Data.(map[string]any)
		indexType, _ := indexData["type"]
		if indexType != "spatial" {
			return nil
		}
		fieldsVal, _ := indexData["fields"]
		fieldsArr, ok := fieldsVal.([]any)
		if !ok {
			return nil
		}
		schemaFields, ok := root["fields"].(map[string]any)
		if !ok {
			return nil
		}
		for _, f := range fieldsArr {
			fieldID, ok := f.(string)
			if !ok {
				continue
			}
			fieldDef, exists := schemaFields[fieldID]
			if !exists {
				continue
			}
			fieldMap := fieldDef.(map[string]any)
			fieldType, _ := fieldMap["type"]
			if fieldType != "geometry" {
				return []common.Issue{{
					Code:     "SPATIAL_INDEX_NON_GEOMETRY",
					Message:  fmt.Sprintf("Spatial index can only reference geometry fields, but field '%s' has type '%v'", fieldID, fieldType),
					Severity: "error",
				}}
			}
		}
		return nil
	},

	"index_condition_value_matches_field_type": func(params definition.PredicateParams) []common.Issue {
		root := params.Root
		condition := params.Data.(map[string]any)
		fieldVal, _ := condition["field"]
		fieldID, ok := fieldVal.(string)
		if !ok {
			return nil
		}
		value := condition["value"]
		schemaFields, ok := root["fields"].(map[string]any)
		if !ok {
			return nil
		}
		fieldDef, exists := schemaFields[fieldID]
		if !exists {
			return nil
		}
		fieldMap := fieldDef.(map[string]any)
		fieldType, _ := fieldMap["type"]
		switch fieldType {
		case "string":
			if _, ok := value.(string); !ok && value != nil {
				return []common.Issue{{Code: "INDEX_CONDITION_VALUE_TYPE_MISMATCH", Message: fmt.Sprintf("Value for field '%s' must be string", fieldID), Severity: "error"}}
			}
		case "boolean":
			if _, ok := value.(bool); !ok && value != nil {
				return []common.Issue{{Code: "INDEX_CONDITION_VALUE_TYPE_MISMATCH", Message: fmt.Sprintf("Value for field '%s' must be boolean", fieldID), Severity: "error"}}
			}
		case "integer":
			switch value.(type) {
			case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
			default:
				if value != nil {
					return []common.Issue{{Code: "INDEX_CONDITION_VALUE_TYPE_MISMATCH", Message: fmt.Sprintf("Value for field '%s' must be integer", fieldID), Severity: "error"}}
				}
			}
		case "number", "decimal":
			switch value.(type) {
			case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8, float64, float32:
			default:
				if value != nil {
					return []common.Issue{{Code: "INDEX_CONDITION_VALUE_TYPE_MISMATCH", Message: fmt.Sprintf("Value for field '%s' must be numeric", fieldID), Severity: "error"}}
				}
			}
		}
		return nil
	},

	"schema_reference_exists": func(params definition.PredicateParams) []common.Issue {
		root, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		schemasMap, ok := root["schemas"].(map[string]any)
		if !ok {
			schemasMap = make(map[string]any)
		}
		var issues []common.Issue
		var walk func(data any, path string)
		walk = func(data any, path string) {
			switch v := data.(type) {
			case map[string]any:
				if idVal, hasID := v["id"]; hasID {
					if idStr, ok := idVal.(string); ok {
						if _, exists := schemasMap[idStr]; !exists && idStr != "" {
							issues = append(issues, common.Issue{
								Code:     "SCHEMA_REFERENCE_NOT_FOUND",
								Message:  fmt.Sprintf("Referenced schema '%s' does not exist", idStr),
								Path:     path,
								Severity: "error",
							})
						}
					}
				}
				if schemaVal, hasSchema := v["schema"]; hasSchema {
					switch arr := schemaVal.(type) {
					case []any:
						for i, ref := range arr {
							walk(ref, fmt.Sprintf("%s.schema[%d]", path, i))
						}
					case map[string]any:
						walk(schemaVal, path+".schema")
					}
				}
				for k, val := range v {
					walk(val, fmt.Sprintf("%s.%s", path, k))
				}
			case []any:
				for i, item := range v {
					walk(item, fmt.Sprintf("%s[%d]", path, i))
				}
			}
		}
		walk(root, "")
		return issues
	},

	"default_matches_type": func(params definition.PredicateParams) []common.Issue {
		fieldData, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		typeVal, hasType := fieldData["type"]
		defaultVal, hasDefault := fieldData["default"]
		if !hasType || !hasDefault || defaultVal == nil {
			return nil
		}
		typeStr, ok := typeVal.(string)
		if !ok {
			return nil
		}
		var errMsg string
		switch typeStr {
		case "string":
			if _, ok := defaultVal.(string); !ok {
				errMsg = "must be string"
			}
		case "boolean":
			if _, ok := defaultVal.(bool); !ok {
				errMsg = "must be boolean"
			}
		case "integer":
			switch defaultVal.(type) {
			case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
			default:
				errMsg = "must be integer"
			}
		case "number", "decimal":
			switch defaultVal.(type) {
			case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8, float64, float32:
			default:
				errMsg = "must be numeric"
			}
		case "array":
			if _, ok := defaultVal.([]any); !ok {
				errMsg = "must be array"
			}
		case "object", "record":
			if _, ok := defaultVal.(map[string]any); !ok {
				errMsg = "must be object"
			}
		case "geometry":
			if _, ok := defaultVal.([]any); !ok {
				errMsg = "must be array of coordinate arrays"
			}
		}
		if errMsg != "" {
			return []common.Issue{{
				Code:     "DEFAULT_VALUE_TYPE_MISMATCH",
				Message:  fmt.Sprintf("Default value %v %s for type %s", defaultVal, errMsg, typeStr),
				Severity: "error",
			}}
		}
		return nil
	},

	"global_field_id_uniqueness": func(params definition.PredicateParams) []common.Issue {
		root, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		seen := make(map[string]string)
		var issues []common.Issue
		var walkFields func(fieldsMap any, path string)
		walkFields = func(fieldsMap any, path string) {
			m, ok := fieldsMap.(map[string]any)
			if !ok {
				return
			}
			for id := range m {
				if existingPath, exists := seen[id]; exists {
					issues = append(issues, common.Issue{
						Code:     "DUPLICATE_FIELD_ID",
						Message:  fmt.Sprintf("Field ID '%s' is not unique; already used at %s", id, existingPath),
						Path:     fmt.Sprintf("%s.%s", path, id),
						Severity: "error",
					})
				} else {
					seen[id] = fmt.Sprintf("%s.%s", path, id)
				}
			}
		}
		if fields, ok := root["fields"]; ok {
			walkFields(fields, "fields")
		}
		if schemas, ok := root["schemas"].(map[string]any); ok {
			for schemaID, schemaVal := range schemas {
				if schemaMap, ok := schemaVal.(map[string]any); ok {
					if fields, ok := schemaMap["fields"]; ok {
						walkFields(fields, fmt.Sprintf("schemas.%s.fields", schemaID))
					}
				}
			}
		}
		return issues
	},

	"field_id_must_be_uuidv7": func(params definition.PredicateParams) []common.Issue {
		root, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		var issues []common.Issue

		// checkMapKeys validates all keys in a map[string]any are valid UUIDv7.
		checkMapKeys := func(m any, path, kind string) {
			mm, ok := m.(map[string]any)
			if !ok {
				return
			}
			for id := range mm {
				if !isUUIDv7(id) {
					issues = append(issues, common.Issue{
						Code:     "ID_NOT_UUIDV7",
						Message:  fmt.Sprintf("%s '%s' is not a valid UUIDv7", kind, id),
						Path:     fmt.Sprintf("%s.%s", path, id),
						Severity: "error",
					})
				}
			}
		}

		// walkScope validates all ID-bearing maps in a single schema scope.
		walkScope := func(scope any, path string) {
			m, ok := scope.(map[string]any)
			if !ok {
				return
			}
			if f, ok := m["fields"]; ok {
				checkMapKeys(f, path+".fields", "Field ID")
			}
			if idx, ok := m["indexes"]; ok {
				checkMapKeys(idx, path+".indexes", "Index ID")
			}
			if c, ok := m["constraints"]; ok {
				checkMapKeys(c, path+".constraints", "Constraint ID")
			}
		}

		// Root scope.
		walkScope(root, "")

		// Schema IDs (schemas map keys).
		if schemas, ok := root["schemas"].(map[string]any); ok {
			checkMapKeys(schemas, "schemas", "Schema ID")
			for schemaID, schemaVal := range schemas {
				walkScope(schemaVal, "schemas."+schemaID)
			}
		}

		return issues
	},

	"constraint_fields_exist": func(params definition.PredicateParams) []common.Issue {
		root := params.Root
		var issues []common.Issue

		resolveFieldPath := func(path string, contextFields map[string]any) bool {
			parts := strings.Split(path, ".")
			current := contextFields
			for i, part := range parts {
				if field, exists := current[part]; exists {
					if i == len(parts)-1 {
						return true
					}
					fieldMap, ok := field.(map[string]any)
					if !ok {
						return false
					}
					if typeVal, hasType := fieldMap["type"]; hasType && typeVal == "object" {
						if schemaRef, hasSchema := fieldMap["schema"].(map[string]any); hasSchema {
							if schemaID, ok := schemaRef["id"].(string); ok {
								if nestedSchema, ok := getSchemaByID(root, schemaID); ok {
									if nestedFields, ok := nestedSchema["fields"].(map[string]any); ok {
										current = nestedFields
										continue
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

		var checkConstraints func(constraints map[string]any, contextFields map[string]any, basePath string)
		checkConstraints = func(constraints map[string]any, contextFields map[string]any, basePath string) {
			for constraintID, constraintData := range constraints {
				constraintMap, ok := constraintData.(map[string]any)
				if !ok {
					continue
				}
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

		if constraints, ok := root["constraints"].(map[string]any); ok {
			if fields, ok := root["fields"].(map[string]any); ok {
				checkConstraints(constraints, fields, "")
			}
		}
		if schemas, ok := root["schemas"].(map[string]any); ok {
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

	"inline_type_descriptor_valid": func(params definition.PredicateParams) []common.Issue {
		field, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		referenceVal, hasReference := field["schema"]
		if !hasReference || referenceVal == nil {
			return nil
		}
		reference, ok := referenceVal.(map[string]any)
		if !ok {
			return nil
		}
		fieldTypeVal, hasFieldType := field["type"]
		if !hasFieldType || fieldTypeVal == nil {
			return nil
		}
		typeVal, hasType := reference["type"]
		if !hasType || typeVal == nil {
			return nil
		}
		allowedToHaveInline := map[string]bool{
			"record": true,
			"array":  true,
			"enum":   true,
		}
		fieldTypeStr, ok := fieldTypeVal.(string)
		if !ok {
			return []common.Issue{{
				Code:     "INLINE_DESCRIPTOR_FIELD_TYPE_NOT_STRING",
				Message:  "Field 'type' must be a string",
				Severity: "error",
			}}
		}
		if !allowedToHaveInline[fieldTypeStr] {
			return []common.Issue{{
				Code:     "INLINE_DESCRIPTOR_NOT_ALLOWED_FOR_FIELD_TYPE",
				Message:  fmt.Sprintf("Inline type descriptors are only allowed for fields of type record, array, set, or enum, but got '%s'", fieldTypeStr),
				Severity: "error",
			}}
		}
		typeStr, ok := typeVal.(string)
		if !ok {
			return []common.Issue{{
				Code:     "INLINE_DESCRIPTOR_TYPE_NOT_STRING",
				Message:  "Inline descriptor 'type' must be a string",
				Severity: "error",
			}}
		}
		allowedInline := map[string]bool{
			"string": true, "number": true, "integer": true,
			"decimal": true, "boolean": true, "bytes": true,
			"unknown": true, "record": true,
		}

		if !allowedInline[typeStr] {
			return []common.Issue{{
				Code:     "INLINE_DESCRIPTOR_INVALID_TYPE",
				Message:  fmt.Sprintf("Inline descriptor type '%s' is not allowed (only primitives and 'record')", typeStr),
				Severity: "error",
			}}
		}
		if valuesVal, hasValues := field["values"]; hasValues && valuesVal != nil {
			if typeStr != "string" && !isNumericType(typeStr) {
				return []common.Issue{{
					Code:     "INLINE_DESCRIPTOR_VALUES_WITHOUT_ENUM",
					Message:  "'values' can only be used with string or numeric inline types",
					Severity: "error",
				}}
			}
			if arr, ok := valuesVal.([]any); !ok || len(arr) == 0 {
				return []common.Issue{{
					Code:     "INLINE_DESCRIPTOR_VALUES_NOT_ARRAY",
					Message:  "'values' must be a non‑empty array",
					Severity: "error",
				}}
			}
		}
		return nil
	},

	"schema_reference_form_correct": func(params definition.PredicateParams) []common.Issue {
		fieldData, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		typeStr, ok := getFieldType(fieldData)
		if !ok {
			return nil
		}
		schemaVal, hasSchema := fieldData["schema"]
		if !hasSchema {
			return nil
		}
		if schemaVal == nil {
			return nil
		}

		// Helper to classify a single reference map
		classifyRef := func(refMap map[string]any) (isNamed, isInline bool) {
			_, hasID := refMap["id"]
			_, hasType := refMap["type"]
			return hasID, hasType
		}

		switch v := schemaVal.(type) {
		case map[string]any:
			// Single reference
			isNamed, isInline := classifyRef(v)
			switch typeStr {
			case "array", "record", "enum":
				if !isNamed && !isInline {
					return []common.Issue{{
						Code:     "SCHEMA_REF_FORM_INVALID",
						Message:  fmt.Sprintf("Field type '%s' requires a single named reference or an inline descriptor", typeStr),
						Severity: "error",
					}}
				}
			case "object":
				if !isNamed {
					return []common.Issue{{
						Code:     "SCHEMA_REF_FORM_INVALID",
						Message:  "Object field requires a single named schema reference",
						Severity: "error",
					}}
				}
			case "union", "composite":
				return []common.Issue{{
					Code:     "SCHEMA_REF_FORM_INVALID",
					Message:  fmt.Sprintf("Field type '%s' requires an array of references", typeStr),
					Severity: "error",
				}}
			}

		case []any:
			// Array of references
			if len(v) == 0 {
				return []common.Issue{{
					Code:     "SCHEMA_REF_FORM_INVALID",
					Message:  fmt.Sprintf("Field type '%s' requires at least one reference", typeStr),
					Severity: "error",
				}}
			}

			switch typeStr {
			case "union", "composite", "enum": // enum now allowed
				for i, item := range v {
					refMap, ok := item.(map[string]any)
					if !ok {
						return []common.Issue{{
							Code:     "SCHEMA_REF_FORM_INVALID",
							Message:  fmt.Sprintf("Element %d of schema array is not an object", i),
							Severity: "error",
						}}
					}
					isNamed, isInline := classifyRef(refMap)
					if typeStr == "enum" {
						// Enum allows named or inline references in the array
						if !isNamed && !isInline {
							return []common.Issue{{
								Code:     "SCHEMA_REF_FORM_INVALID",
								Message:  fmt.Sprintf("Enum array element %d must be a named reference or inline descriptor", i),
								Severity: "error",
							}}
						}
					} else {
						// union/composite require named references only
						if !isNamed {
							return []common.Issue{{
								Code:     "SCHEMA_REF_FORM_INVALID",
								Message:  fmt.Sprintf("Field type '%s' array element %d must be a named schema reference (with 'id')", typeStr, i),
								Severity: "error",
							}}
						}
					}
				}
			default:
				return []common.Issue{{
					Code:     "SCHEMA_REF_FORM_INVALID",
					Message:  fmt.Sprintf("Field type '%s' cannot use an array of references", typeStr),
					Severity: "error",
				}}
			}

		default:
			return []common.Issue{{
				Code:     "SCHEMA_REF_FORM_INVALID",
				Message:  "Field 'schema' must be an object or array",
				Severity: "error",
			}}
		}
		return nil
	},
	// ---------------------------------------------------------------------
	// Additional predicates from original constraints.go
	// ---------------------------------------------------------------------
	"collection_requires_schema": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		typeStr, ok := getFieldType(data)
		if !ok {
			return nil
		}
		schemaVal, hasSchema := data["schema"]
		if isCollectionType(typeStr) && (!hasSchema || schemaVal == nil) {
			return []common.Issue{{
				Code:     "COLLECTION_MISSING_SCHEMA",
				Message:  fmt.Sprintf("Collection type '%s' must have a schema reference", typeStr),
				Severity: "error",
			}}
		}
		return nil
	},

	"union_requires_multiple_schemas": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		const minSchemas = 2
		typeStr, ok := getFieldType(data)
		if !ok || typeStr != "union" {
			return nil
		}
		schemaVal, hasSchema := data["schema"]
		if !hasSchema || schemaVal == nil {
			return []common.Issue{{
				Code:     "UNION_MISSING_SCHEMA",
				Message:  "Union type must have schema references",
				Severity: "error",
			}}
		}
		arr, ok := schemaVal.([]any)
		if !ok {
			return []common.Issue{{
				Code:     "UNION_SCHEMA_NOT_ARRAY",
				Message:  "Union type schema must be an array of schema references",
				Severity: "error",
			}}
		}
		if len(arr) < minSchemas {
			return []common.Issue{{
				Code:     "UNION_INSUFFICIENT_SCHEMAS",
				Message:  fmt.Sprintf("Union type must have at least %d schema references, got %d", minSchemas, len(arr)),
				Severity: "error",
			}}
		}
		return nil
	},

	"record_schema_cardinality": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		typeStr, ok := getFieldType(data)
		if !ok || typeStr != "record" {
			return nil
		}
		schemaVal, hasSchema := data["schema"]
		if hasSchema && schemaVal != nil {
			if _, isArray := schemaVal.([]any); isArray {
				return []common.Issue{{
					Code:     "RECORD_SCHEMA_ARRAY",
					Message:  "Record type must have zero or one schema reference, not an array",
					Severity: "error",
				}}
			}
		}
		return nil
	},

	"nested_schema_exclusive_mode": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
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

	"constraint_type_exclusive": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}

		name := "PLEASE_NAME_YOUR_CONSTRAINTS"
		if k, hasNameTemp := data["name"]; hasNameTemp {
			if n, ok := k.(string); ok {
				name = n
			}
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
				Path:     name,
				Severity: "error",
			}}
		}

		if !hasRuleFields && !hasGroupFields {
			return []common.Issue{{
				Code:     "CONSTRAINT_NO_TYPE",
				Message:  "Constraint must have either predicate (rule) or operator+rules (group)",
				Path:     name,
				Severity: "error",
			}}
		}
		return nil
	},

	"constraint_rule_requires_predicate": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
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

	"index_condition_type_exclusive": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
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

	"schema_name_required": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
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

	"field_name_required": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
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

	"index_fields_not_empty": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
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

	"schema_reference_id_required": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
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

	"elements_must_be_unique": func(params definition.PredicateParams) []common.Issue {
		data, ok := params.Data.([]any)
		if !ok {
			return nil
		}
		if len(data) <= 1 {
			return nil
		}
		for i := 1; i < len(data); i++ {
			for j := 0; j < i; j++ {
				if reflect.DeepEqual(data[i], data[j]) {
					return []common.Issue{{
						Code:     "DUPLICATE_ELEMENT",
						Message:  fmt.Sprintf("Duplicate value at index %d (first seen at index %d)", i, j),
						Severity: "error",
					}}
				}
			}
		}
		return nil
	},

	"index_fields_reference_valid": func(params definition.PredicateParams) []common.Issue {
		root := params.Root
		data, ok := params.Data.(map[string]any)
		if !ok {
			return nil
		}
		schemaFields, ok := root["fields"].(map[string]any)
		if !ok {
			return nil
		}
		var issues []common.Issue
		fieldsVal, ok := data["fields"]
		if !ok {
			return nil
		}
		fieldsArray, ok := fieldsVal.([]any)
		if !ok {
			return nil
		}

		name := "PLEASE_NAME_YOUR_INDEXES"
		if k, hasNameTemp := data["name"]; hasNameTemp {
			if n, ok := k.(string); ok {
				name = n
			}
		}
		for i, fieldRef := range fieldsArray {
			fieldID, ok := fieldRef.(string)
			if !ok {
				continue
			}
			if _, exists := schemaFields[fieldID]; !exists {
				issues = append(issues, common.Issue{
					Code:     "INDEX_FIELD_NOT_FOUND",
					Message:  fmt.Sprintf("Index '%s' references non-existent field '%s'", name, fieldID),
					Path:     fmt.Sprintf("indexes.%s.fields[%d]", name, i),
					Severity: "error",
				})
			}
		}
		return issues
	},
}

// isUUIDv7 checks whether s is a valid UUIDv7 string.
// Format: xxxxxxxx-xxxx-7xxx-{8,9,a,b}xxx-xxxxxxxxxxxx
func isUUIDv7(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		case 14:
			if c != '7' {
				return false
			}
		case 19:
			if c != '8' && c != '9' && c != 'a' && c != 'b' {
				return false
			}
		default:
			if !isHex(c) {
				return false
			}
		}
	}
	return true
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

