// Package schema provides the Validator, a key component for ensuring that data
// conforms to a given schema definition. It supports detailed error reporting and
// can be extended with custom validation logic.
package schema

import (
	"fmt"
	"maps"
	"reflect"
	"strconv"
	"strings"
)

// Validator is responsible for validating data against a schema. It checks for
// type correctness, required fields, and custom constraints, and it can be
// configured with a map of predicate functions for extensibility.
type Validator struct {
	schema *SchemaDefinition
	fmap   FunctionMap
	issues []Issue
}

// NewValidator creates a new Validator instance for a given schema and function map.
// The returned validator can be reused for multiple validation operations.
func NewValidator(schema *SchemaDefinition, fmap FunctionMap) *Validator {
	return &Validator{
		schema: schema,
		fmap:   fmap,
		issues: make([]Issue, 0),
	}
}

// Validate checks if a given data map conforms to the validator's schema.
// It returns a boolean indicating whether the validation was successful, and a slice
// of any issues that were found. The `loose` parameter can be used to ignore
// missing required fields.
func (v *Validator) Validate(data map[string]any, loose bool) (bool, []Issue) {
	v.issues = make([]Issue, 0)

	v.validateData(data, "")
	v.validateSchemaConstraints(data, "")

	finalIssues := v.issues
	if loose {
		filteredIssues := make([]Issue, 0, len(v.issues))
		for _, issue := range v.issues {
			if issue.Code != "REQUIRED_FIELD_MISSING" {
				filteredIssues = append(filteredIssues, issue)
			}
		}
		finalIssues = filteredIssues
	}

	return len(finalIssues) == 0, finalIssues
}

// coerceValue attempts to convert a value to the expected type.
func (v *Validator) coerceValue(value any, expectedType FieldType) (any, bool) {
	if value == nil {
		return nil, true
	}

	if str, ok := value.(string); ok && strings.ToLower(str) == "null" {
		return nil, true
	}

	str, ok := value.(string)
	if !ok {
		return value, false
	}

	switch expectedType {
	case FieldTypeBoolean:
		lower := strings.ToLower(str)
		if lower == "true" {
			return true, true
		} else if lower == "false" {
			return false, true
		}
	case FieldTypeInteger:
		if intVal, err := strconv.ParseInt(str, 10, 64); err == nil {
			if strconv.FormatInt(intVal, 10) == str {
				return int(intVal), true
			}
		}
	case FieldTypeNumber, FieldTypeDecimal:
		if floatVal, err := strconv.ParseFloat(str, 64); err == nil {
			return floatVal, true
		}
	}
	return value, false
}

// validateData is the main validation function that checks all fields in the data.
func (v *Validator) validateData(data map[string]any, path string) {
	for fieldName, fieldDef := range v.schema.Fields {
		fieldPath := v.buildPath(path, fieldName)
		value, exists := data[fieldName]

		if fieldDef.Required != nil && *fieldDef.Required && !exists {
			v.addIssue("REQUIRED_FIELD_MISSING", fmt.Sprintf("Required field '%s' is missing", fieldName), fieldPath)
			continue
		}

		if !exists {
			continue
		}

		v.validateFieldValue(value, fieldDef, fieldPath)
	}

	for dataKey := range data {
		if _, exists := v.schema.Fields[dataKey]; !exists {
			v.addIssue("UNEXPECTED_FIELD", fmt.Sprintf("Unexpected field '%s' not defined in schema", dataKey), v.buildPath(path, dataKey))
		}
	}
}

// validateFieldValue validates a single field's value against its definition.
func (v *Validator) validateFieldValue(value any, fieldDef *FieldDefinition, path string) {
	coercedValue, typeValid := v.validateFieldTypeWithCoercion(value, fieldDef.Type, fieldDef, path)
	if !typeValid {
		return
	}

	value = coercedValue

	v.validateFieldConstraints(value, fieldDef.Constraints, path)

	if fieldDef.Unique != nil && *fieldDef.Unique {
		// This is a placeholder for uniqueness validation, which would typically be handled
		// by the database or a service that has access to the entire collection.
	}

	if fieldDef.Type == FieldTypeEnum && len(fieldDef.Values) > 0 {
		v.validateEnumValue(value, fieldDef.Values, path)
	}

	switch fieldDef.Type {
	case FieldTypeObject:
		v.validateObjectField(value, fieldDef, path)
	case FieldTypeUnion:
		v.validateUnionField(value, fieldDef, path)
	case FieldTypeArray, FieldTypeSet:
		v.validateArrayField(value, fieldDef, path)
	}
}

// validateFieldTypeWithCoercion checks the type of a field, attempting to coerce it if necessary.
func (v *Validator) validateFieldTypeWithCoercion(value any, expectedType FieldType, fieldDef *FieldDefinition, path string) (any, bool) {
	if value == nil || v.isStringNull(value) {
		coercedValue, _ := v.coerceValue(value, expectedType)
		if coercedValue == nil {
			if fieldDef.Required != nil && *fieldDef.Required {
				v.addIssue("NULL_VALUE", "Field cannot be null", path)
				return nil, false
			}
			return nil, true
		}
		value = coercedValue
	}

	if coercedValue, coerced := v.coerceValue(value, expectedType); coerced {
		value = coercedValue
		if v.validateFieldType(value, expectedType, fieldDef, path) {
			return value, true
		}
	}

	if v.validateFieldType(value, expectedType, fieldDef, path) {
		return value, true
	}

	return value, false
}

// isStringNull checks if a value is the string "null", case-insensitively.
func (v *Validator) isStringNull(value any) bool {
	if str, ok := value.(string); ok {
		return strings.ToLower(str) == "null"
	}
	return false
}

// validateFieldType checks if a value's type matches the expected type.
func (v *Validator) validateFieldType(value any, expectedType FieldType, fieldDef *FieldDefinition, path string) bool {
	if value == nil {
		if fieldDef.Required != nil && *fieldDef.Required {
			v.addIssue("NULL_VALUE", "Field cannot be null", path)
			return false
		}
		return true
	}

	switch expectedType {
	case FieldTypeString:
		if _, ok := value.(string); !ok {
			v.addIssue("TYPE_MISMATCH", fmt.Sprintf("Expected string, got %T", value), path)
			return false
		}
	case FieldTypeNumber, FieldTypeDecimal:
		if !v.isNumericType(value) {
			v.addIssue("TYPE_MISMATCH", fmt.Sprintf("Expected number, got %T", value), path)
			return false
		}
	case FieldTypeInteger:
		if !v.isIntegerType(value) {
			v.addIssue("TYPE_MISMATCH", fmt.Sprintf("Expected integer, got %T", value), path)
			return false
		}
	case FieldTypeBoolean:
		if _, ok := value.(bool); !ok {
			v.addIssue("TYPE_MISMATCH", fmt.Sprintf("Expected boolean, got %T", value), path)
			return false
		}
	case FieldTypeArray, FieldTypeSet:
		if !v.isArrayType(value) {
			v.addIssue("TYPE_MISMATCH", fmt.Sprintf("Expected array, got %T", value), path)
			return false
		}
	case FieldTypeObject, FieldTypeRecord:
		if !v.isObjectType(value) {
			v.addIssue("TYPE_MISMATCH", fmt.Sprintf("Expected object, got %T", value), path)
			return false
		}
	}
	return true
}

// validateFieldConstraints validates all constraints for a given field.
func (v *Validator) validateFieldConstraints(value any, constraints SchemaConstraint[FieldType], path string) {
	for _, rule := range constraints {
		v.validateConstraintRule(value, rule, path)
	}
}

// validateConstraintRule validates a single constraint rule, which can be either a
// single constraint or a group of constraints.
func (v *Validator) validateConstraintRule(value any, rule SchemaConstraintRule[FieldType], path string) {
	switch r := rule.(type) {
	case Constraint[FieldType]:
		v.validateConstraint(value, r, path)
	case ConstraintGroup[FieldType]:
		v.validateConstraintGroup(value, r, path)
	default:
		v.addIssue("UNKNOWN_CONSTRAINT_TYPE", fmt.Sprintf("Unknown constraint rule type: %T", rule), path)
	}
}

// validateConstraint validates a single constraint by executing its predicate function.
func (v *Validator) validateConstraint(value any, constraint Constraint[FieldType], path string) {
	predicateFunc, exists := v.fmap[constraint.Predicate]
	if !exists {
		v.addIssue("MISSING_PREDICATE", fmt.Sprintf("Predicate function '%s' not found", constraint.Predicate), path)
		return
	}

	predicate, ok := predicateFunc.(func(PredicateParams[any]) bool)
	if !ok {
		v.addIssue("INVALID_PREDICATE_TYPE", fmt.Sprintf("Predicate '%s' has invalid type", constraint.Predicate), path)
		return
	}

	params := PredicateParams[any]{
		Data:  value,
		Field: constraint.Field,
		Args:  constraint.Parameters,
	}

	if !predicate(params) {
		message := fmt.Sprintf("Constraint '%s' failed", constraint.Name)
		if constraint.ErrorMessage != nil {
			message = *constraint.ErrorMessage
		}
		v.addIssue("CONSTRAINT_VIOLATION", message, path)
	}
}

// validateConstraintGroup validates a group of constraints.
func (v *Validator) validateConstraintGroup(value any, group ConstraintGroup[FieldType], path string) {
	results := make([]bool, len(group.Rules))
	for i, rule := range group.Rules {
		initialIssueCount := len(v.issues)
		v.validateConstraintRule(value, rule, path)
		results[i] = len(v.issues) == initialIssueCount
	}

	if !v.evaluateLogicalOperator(group.Operator, results) {
		v.addIssue("CONSTRAINT_GROUP_VIOLATION", fmt.Sprintf("Constraint group '%s' failed", group.Name), path)
	}
}

// evaluateLogicalOperator evaluates a logical operator on a set of boolean results.
func (v *Validator) evaluateLogicalOperator(operator LogicalOperator, results []bool) bool {
	switch operator {
	case LogicalAnd:
		for _, result := range results {
			if !result {
				return false
			}
		}
		return true
	case LogicalOr:
		for _, result := range results {
			if result {
				return true
			}
		}
		return false
	case LogicalNot:
		return len(results) == 1 && !results[0]
	case LogicalNor:
		for _, result := range results {
			if result {
				return false
			}
		}
		return true
	case LogicalXor:
		trueCount := 0
		for _, result := range results {
			if result {
				trueCount++
			}
		}
		return trueCount == 1
	}
	return false
}

// validateEnumValue validates that a value is one of the allowed enum values.
func (v *Validator) validateEnumValue(value any, allowedValues []any, path string) {
	for _, allowedValue := range allowedValues {
		if reflect.DeepEqual(value, allowedValue) {
			return
		}
	}
	v.addIssue("ENUM_VIOLATION", fmt.Sprintf("Value must be one of: %v", allowedValues), path)
}

// validateObjectField validates an object field against its schema.
func (v *Validator) validateObjectField(value any, fieldDef *FieldDefinition, path string) {
	objectData, ok := value.(map[string]any)
	if !ok {
		v.addIssue("TYPE_MISMATCH", fmt.Sprintf("Expected object, got %T", value), path)
		return
	}

	if fieldDef.Schema == nil {
		return
	}

	switch schema := fieldDef.Schema.(type) {
	case FieldSchema:
		v.validateFieldSchema(objectData, schema, path)
	case []FieldSchema:
		if len(schema) == 1 {
			v.validateFieldSchema(objectData, schema[0], path)
		} else {
			v.addIssue("INVALID_OBJECT_SCHEMA", "Object type should have exactly one schema definition", path)
		}
	default:
		v.addIssue("INVALID_SCHEMA_TYPE", fmt.Sprintf("Invalid schema type: %T", schema), path)
	}
}

// validateUnionField validates a union field against its possible schemas.
func (v *Validator) validateUnionField(value any, fieldDef *FieldDefinition, path string) {
	if fieldDef.Schema == nil {
		v.addIssue("MISSING_UNION_SCHEMA", "Union field must have schema definitions", path)
		return
	}

	schemas, ok := fieldDef.Schema.([]FieldSchema)
	if !ok {
		v.addIssue("INVALID_UNION_SCHEMA", "Union field schema must be an array of FieldSchema", path)
		return
	}

	objectData, ok := value.(map[string]any)
	if !ok {
		v.addIssue("TYPE_MISMATCH", fmt.Sprintf("Expected object for union validation, got %T", value), path)
		return
	}

	matched := false
	for i, schema := range schemas {
		schemaPath := fmt.Sprintf("%s[schema:%d]", path, i)
		initialIssueCount := len(v.issues)

		v.validateFieldSchema(objectData, schema, schemaPath)

		if len(v.issues) == initialIssueCount {
			matched = true
			break
		} else {
			v.issues = v.issues[:initialIssueCount]
		}
	}

	if !matched {
		v.addIssue("UNION_NO_MATCH", "Value does not match any of the union schemas", path)
	}
}

// validateArrayField validates an array or set field.
func (v *Validator) validateArrayField(value any, fieldDef *FieldDefinition, path string) {
	arrayValue, ok := value.([]any)
	if !ok {
		v.addIssue("TYPE_MISMATCH", fmt.Sprintf("Expected array, got %T", value), path)
		return
	}

	if fieldDef.ItemsType != nil {
		for i, item := range arrayValue {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			itemFieldDef := &FieldDefinition{Type: *fieldDef.ItemsType}
			v.validateFieldValue(item, itemFieldDef, itemPath)
		}
	}

	if fieldDef.Type == FieldTypeSet {
		v.validateSetUniqueness(arrayValue, path)
	}
}

// validateSetUniqueness validates that all items in a set are unique.
func (v *Validator) validateSetUniqueness(items []any, path string) {
	seen := make(map[string]bool)
	for i, item := range items {
		key := fmt.Sprintf("%v", item)
		if seen[key] {
			v.addIssue("SET_DUPLICATE", fmt.Sprintf("Duplicate value found in set at index %d", i), path)
		}
		seen[key] = true
	}
}

// validateFieldSchema validates data against a nested schema.
func (v *Validator) validateFieldSchema(data map[string]any, fieldSchema FieldSchema, path string) {
	nestedSchema, exists := v.schema.NestedSchemas[fieldSchema.ID]
	if !exists {
		v.addIssue("NESTED_SCHEMA_NOT_FOUND", fmt.Sprintf("Nested schema '%s' not found", fieldSchema.ID), path)
		return
	}

	tempSchemaDef := &SchemaDefinition{Fields: make(map[string]*FieldDefinition)}

	if nestedSchema.isStructured {
		if nestedSchema.StructuredFieldsMap != nil {
			tempSchemaDef.Fields = nestedSchema.StructuredFieldsMap
		} else if nestedSchema.StructuredFieldsArray != nil {
			for _, fieldGroup := range nestedSchema.StructuredFieldsArray {
				if fieldGroup.When != nil {
					if fieldValue, exists := data[fieldGroup.When.Field]; exists && reflect.DeepEqual(fieldValue, fieldGroup.When.Value) {
						maps.Copy(tempSchemaDef.Fields, fieldGroup.Fields)
					}
				} else {
					maps.Copy(tempSchemaDef.Fields, fieldGroup.Fields)
				}
			}
		}
	} else {
		if nestedSchema.Type != nil {
			literalFieldDef := &FieldDefinition{
				Type:        *nestedSchema.Type,
				Constraints: nestedSchema.LiteralConstraints,
				Default:     nestedSchema.LiteralDefault,
				Schema:      nestedSchema.LiteralSchema,
				ItemsType:   nestedSchema.LiteralItemsType,
			}
			v.validateFieldValue(data, literalFieldDef, path)
			return
		}
	}

	if len(fieldSchema.Constraints) > 0 {
		v.validateFieldConstraints(data, fieldSchema.Constraints, path)
	}

	nestedValidator := &Validator{
		schema: tempSchemaDef,
		fmap:   v.fmap,
		issues: make([]Issue, 0),
	}

	nestedValidator.validateData(data, path)

	for _, issue := range nestedValidator.issues {
		v.issues = append(v.issues, issue)
	}
}

// validateSchemaConstraints validates constraints that are defined at the schema level.
func (v *Validator) validateSchemaConstraints(data map[string]any, path string) {
	for _, rule := range v.schema.Constraints {
		v.validateConstraintRule(data, rule, path)
	}
}

// isNumericType checks if a value is a numeric type.
func (v *Validator) isNumericType(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	return false
}

// isIntegerType checks if a value is an integer type.
func (v *Validator) isIntegerType(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	}
	return false
}

// isArrayType checks if a value is an array or slice.
func (v *Validator) isArrayType(value any) bool {
	rv := reflect.ValueOf(value)
	return rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array
}

// isObjectType checks if a value is a map with string keys.
func (v *Validator) isObjectType(value any) bool {
	_, ok := value.(map[string]any)
	return ok
}

// buildPath constructs a dot-separated path string for error reporting.
func (v *Validator) buildPath(basePath, fieldName string) string {
	if basePath == "" {
		return fieldName
	}
	return basePath + "." + fieldName
}

// addIssue adds a new validation issue to the validator's list of issues.
func (v *Validator) addIssue(code, message, path string) {
	issue := Issue{
		Code:     code,
		Message:  message,
		Path:     path,
		Severity: "error",
	}
	v.issues = append(v.issues, issue)
}
