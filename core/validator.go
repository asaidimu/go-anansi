package core

import (
	"fmt"
	"reflect"
	"maps"
	"strconv"
	"strings"
)

// Validator handles schema validation with detailed error reporting.
type Validator struct {
	schema *SchemaDefinition
	fmap   FunctionMap
	issues []Issue // This will be reset for each validation run
}

// NewValidator creates and returns a new Validator instance configured with a schema and function map.
// This instance can be re-used to validate multiple data inputs against the same schema.
func NewValidator(schema *SchemaDefinition, fmap FunctionMap) *Validator {
	return &Validator{
		schema: schema,
		fmap:   fmap,
		issues: make([]Issue, 0), // Initialize empty, will be reset on each Validate call
	}
}

// Validate validates data against the validator's schema definition using provided predicate functions.
// Returns true if validation passes, false otherwise, along with any validation issues.
// This method can be called multiple times on the same Validator instance.
func (v *Validator) Validate(data map[string]any, loose bool) (bool, []Issue) {
	// Reset issues slice for a new validation run
	v.issues = make([]Issue, 0)

	v.validateData(data, "")
	v.validateSchemaConstraints(data, "")
	finalIssues := v.issues // Start with all issues

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

// coerceValue attempts to coerce a value to the expected type.
// Returns the coerced value and whether coercion was successful.
func (v *Validator) coerceValue(value any, expectedType FieldType) (any, bool) {
	// Handle null coercion first
	if value == nil {
		return nil, true
	}

	// Handle string "null" to nil
	if str, ok := value.(string); ok && strings.ToLower(str) == "null" {
		return nil, true
	}

	// Only attempt coercion from strings to other types
	str, ok := value.(string)
	if !ok {
		return value, false // Not a string, no coercion needed
	}

	switch expectedType {
	case FieldTypeBoolean:
		lower := strings.ToLower(str)
		if lower == "true" {
			return true, true
		} else if lower == "false" {
			return false, true
		}
		return value, false

	case FieldTypeInteger:
		// Only coerce if it's a valid integer (no decimal point)
		if intVal, err := strconv.ParseInt(str, 10, 64); err == nil {
			// Check if the string representation matches (no leading zeros, etc.)
			if strconv.FormatInt(intVal, 10) == str {
				return int(intVal), true
			}
		}
		return value, false

	case FieldTypeNumber, FieldTypeDecimal:
		if floatVal, err := strconv.ParseFloat(str, 64); err == nil {
			return floatVal, true
		}
		return value, false

	default:
		return value, false // No coercion for other types
	}
}

// validateData validates the main data structure against schema fields.
func (v *Validator) validateData(data map[string]any, path string) {
	// Check required fields
	for fieldName, fieldDef := range v.schema.Fields {
		fieldPath := v.buildPath(path, fieldName)
		value, exists := data[fieldName]

		// Check if required field is missing
		if fieldDef.Required != nil && *fieldDef.Required && !exists {
			v.addIssue("REQUIRED_FIELD_MISSING", fmt.Sprintf("Required field '%s' is missing", fieldName), fieldPath)
			continue
		}

		// Skip validation if field doesn't exist and isn't required
		if !exists {
			continue
		}

		// Validate the field value
		v.validateFieldValue(value, fieldDef, fieldPath)
	}

	// Check for unexpected fields (strict validation)
	for dataKey := range data {
		if _, exists := v.schema.Fields[dataKey]; !exists {
			v.addIssue("UNEXPECTED_FIELD", fmt.Sprintf("Unexpected field '%s' not defined in schema", dataKey), v.buildPath(path, dataKey))
		}
	}
}

// validateFieldValue validates a single field value against its definition.
func (v *Validator) validateFieldValue(value any, fieldDef *FieldDefinition, path string) {
	// Validate field type (with coercion)
	coercedValue, typeValid := v.validateFieldTypeWithCoercion(value, fieldDef.Type, fieldDef, path)
	if !typeValid {
		return // Type validation failed, skip further validation
	}

	// Use coerced value for further validation
	value = coercedValue

	// Validate field constraints
	v.validateFieldConstraints(value, fieldDef.Constraints, path)

	// Validate unique constraint (if applicable)
	if fieldDef.Unique != nil && *fieldDef.Unique {
		// Note: Unique validation would require access to all data instances
		// This is a placeholder for unique constraint validation
		// In a real implementation, this would check against a database or collection
	}

	// Validate enum values
	if fieldDef.Type == FieldTypeEnum && len(fieldDef.Values) > 0 {
		v.validateEnumValue(value, fieldDef.Values, path)
	}

	// Validate nested schemas for object and union types
	switch fieldDef.Type {
	case FieldTypeObject:
		v.validateObjectField(value, fieldDef, path)
	case FieldTypeUnion:
		v.validateUnionField(value, fieldDef, path)
	case FieldTypeArray, FieldTypeSet:
		v.validateArrayField(value, fieldDef, path)
	}
}

// validateFieldTypeWithCoercion validates that a value matches the expected field type, attempting coercion if needed.
// Returns the (potentially coerced) value and whether validation passed.
func (v *Validator) validateFieldTypeWithCoercion(value any, expectedType FieldType, fieldDef *FieldDefinition, path string) (any, bool) {
	// Handle null values first
	if value == nil || (v.isStringNull(value)) {
		coercedValue, _ := v.coerceValue(value, expectedType)
		if coercedValue == nil {
			if fieldDef.Required != nil && *fieldDef.Required {
				v.addIssue("NULL_VALUE", fmt.Sprintf("Field cannot be null"), path)
				return nil, false
			}
			return nil, true
		}
		value = coercedValue
	}

	// Try coercion first
	if coercedValue, coerced := v.coerceValue(value, expectedType); coerced {
		value = coercedValue
		// After coercion, check if it now matches the expected type
		if v.validateFieldType(value, expectedType, fieldDef, path) {
			return value, true
		}
	}

	// If coercion didn't work or wasn't attempted, validate original type
	if v.validateFieldType(value, expectedType, fieldDef, path) {
		return value, true
	}

	return value, false
}

// isStringNull checks if a value is the string "null"
func (v *Validator) isStringNull(value any) bool {
	if str, ok := value.(string); ok {
		return strings.ToLower(str) == "null"
	}
	return false
}

// validateFieldType validates that a value matches the expected field type.
func (v *Validator) validateFieldType(value any, expectedType FieldType, fieldDef *FieldDefinition, path string) bool {
	if value == nil {
		if fieldDef.Required != nil && *fieldDef.Required {
			v.addIssue("NULL_VALUE", fmt.Sprintf("Field cannot be null"), path)
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
	case FieldTypeEnum:
		// Enum type validation is handled separately in validateEnumValue
		return true
	case FieldTypeUnion:
		// Union type validation is handled separately in validateUnionField
		return true
	}

	return true
}

// validateFieldConstraints validates all constraints for a field.
func (v *Validator) validateFieldConstraints(value any, constraints SchemaConstraint[FieldType], path string) {
	for _, rule := range constraints {
		v.validateConstraintRule(value, rule, path)
	}
}

// validateConstraintRule validates a single constraint rule (Constraint or ConstraintGroup).
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

// validateConstraint validates a single constraint.
func (v *Validator) validateConstraint(value any, constraint Constraint[FieldType], path string) {
	predicateFunc, exists := v.fmap[constraint.Predicate]
	if !exists {
		v.addIssue("MISSING_PREDICATE", fmt.Sprintf("Predicate function '%s' not found", constraint.Predicate), path)
		return
	}

	// Type assert to Predicate function
	predicate, ok := predicateFunc.(func(PredicateParams[any]) bool)
	if !ok {
		v.addIssue("INVALID_PREDICATE_TYPE", fmt.Sprintf("Predicate '%s' has invalid type, expected func(PredicateParams[any]) bool", constraint.Predicate), path)
		return
	}

	// Prepare predicate parameters
	params := PredicateParams[any]{
		Data:  value,
		Field: constraint.Field,
		Args:  constraint.Parameters,
	}

	// Execute predicate
	if !predicate(params) {
		message := fmt.Sprintf("Constraint '%s' failed", constraint.Name)
		if constraint.ErrorMessage != nil {
			message = *constraint.ErrorMessage
		}
		v.addIssue("CONSTRAINT_VIOLATION", message, path)
	}
}

// validateConstraintGroup validates a constraint group with logical operators.
func (v *Validator) validateConstraintGroup(value any, group ConstraintGroup[FieldType], path string) {
	results := make([]bool, len(group.Rules))

	// Evaluate all rules (no short-circuiting for debugging purposes)
	for i, rule := range group.Rules {
		initialIssueCount := len(v.issues)
		v.validateConstraintRule(value, rule, path)
		// Check if this rule passed (no new issues added)
		results[i] = len(v.issues) == initialIssueCount
	}

	// Apply logical operator
	groupResult := v.evaluateLogicalOperator(group.Operator, results)

	if !groupResult {
		v.addIssue("CONSTRAINT_GROUP_VIOLATION", fmt.Sprintf("Constraint group '%s' with operator '%s' failed", group.Name, group.Operator), path)
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
		// NOT operator should have exactly one operand
		if len(results) != 1 {
			return false
		}
		return !results[0]
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
	default:
		return false
	}
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

	// Handle FieldSchema or []FieldSchema
	switch schema := fieldDef.Schema.(type) {
	case FieldSchema:
		v.validateFieldSchema(objectData, schema, path)
	case []FieldSchema:
		// For object type, data must match exactly one schema
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

	// Try to match against each schema
	matched := false
	for i, schema := range schemas {
		schemaPath := fmt.Sprintf("%s[schema:%d]", path, i)
		initialIssueCount := len(v.issues)

		v.validateFieldSchema(objectData, schema, schemaPath)

		// If no new issues were added, this schema matches
		if len(v.issues) == initialIssueCount {
			matched = true
			break
		} else {
			// Remove issues from this failed attempt
			v.issues = v.issues[:initialIssueCount]
		}
	}

	if !matched {
		v.addIssue("UNION_NO_MATCH", "Value does not match any of the union schemas", path)
	}
}

// validateArrayField validates array or set fields.
func (v *Validator) validateArrayField(value any, fieldDef *FieldDefinition, path string) {
	arrayValue, ok := value.([]any)
	if !ok {
		v.addIssue("TYPE_MISMATCH", fmt.Sprintf("Expected array, got %T", value), path)
		return
	}

	// Validate items type if specified
	if fieldDef.ItemsType != nil {
		for i, item := range arrayValue {
			itemPath := fmt.Sprintf("%s[%d]", path, i)

			// Create a temporary field definition for the item
			itemFieldDef := &FieldDefinition{
				Type: *fieldDef.ItemsType,
			}

			v.validateFieldValue(item, itemFieldDef, itemPath)
		}
	}

	// For sets, validate uniqueness
	if fieldDef.Type == FieldTypeSet {
		v.validateSetUniqueness(arrayValue, path)
	}
}

// validateSetUniqueness validates that all items in a set are unique.
func (v *Validator) validateSetUniqueness(items []any, path string) {
	seen := make(map[string]bool)
	for i, item := range items {
		// Create a string representation for comparison
		key := fmt.Sprintf("%v", item)
		if seen[key] {
			v.addIssue("SET_DUPLICATE", fmt.Sprintf("Duplicate value found in set at index %d", i), path)
		}
		seen[key] = true
	}
}

// validateFieldSchema validates data against a FieldSchema (nested schema).
func (v *Validator) validateFieldSchema(data map[string]any, fieldSchema FieldSchema, path string) {
	nestedSchema, exists := v.schema.NestedSchemas[fieldSchema.ID]
	if !exists {
		v.addIssue("NESTED_SCHEMA_NOT_FOUND", fmt.Sprintf("Nested schema '%s' not found", fieldSchema.ID), path)
		return
	}

	// Create a temporary schema definition for the nested validation
	tempSchemaDef := &SchemaDefinition{
		Fields: make(map[string]*FieldDefinition),
	}

	// Handle different types of nested schemas
	if nestedSchema.isStructured {
		if nestedSchema.StructuredFieldsMap != nil {
			tempSchemaDef.Fields = nestedSchema.StructuredFieldsMap
		} else if nestedSchema.StructuredFieldsArray != nil {
			// Handle conditional fields
			for _, fieldGroup := range nestedSchema.StructuredFieldsArray {
				if fieldGroup.When != nil {
					// Check condition
					if fieldValue, exists := data[fieldGroup.When.Field]; exists {
						if reflect.DeepEqual(fieldValue, fieldGroup.When.Value) {
							maps.Copy(tempSchemaDef.Fields, fieldGroup.Fields)
						}
					}
				} else {
					// No condition, always include
					maps.Copy(tempSchemaDef.Fields, fieldGroup.Fields)
				}
			}
		}
	} else {
		// Literal schema - validate the entire data as a single value
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

	// Apply field schema constraints if any
	if len(fieldSchema.Constraints) > 0 {
		v.validateFieldConstraints(data, fieldSchema.Constraints, path)
	}

	// Validate nested data using a temporary validator to manage its own issues
	nestedValidator := &Validator{
		schema: tempSchemaDef,
		fmap:   v.fmap,
		issues: make([]Issue, 0), // Fresh issues slice for nested validation
	}

	nestedValidator.validateData(data, path)

	// Merge issues from the nested validation into the parent validator's issues
	for _, issue := range nestedValidator.issues {
		v.issues = append(v.issues, issue)
	}
}

// validateSchemaConstraints validates schema-level constraints.
func (v *Validator) validateSchemaConstraints(data map[string]any, path string) {
	for _, rule := range v.schema.Constraints {
		v.validateConstraintRule(data, rule, path)
	}
}

// Helper methods for type checking
func (v *Validator) isNumericType(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

func (v *Validator) isIntegerType(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func (v *Validator) isArrayType(value any) bool {
	rv := reflect.ValueOf(value)
	return rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array
}

func (v *Validator) isObjectType(value any) bool {
	_, ok := value.(map[string]any)
	return ok
}

// buildPath constructs a path string for error reporting.
func (v *Validator) buildPath(basePath, fieldName string) string {
	if basePath == "" {
		return fieldName
	}
	return basePath + "." + fieldName
}

// addIssue adds a validation issue to the issues slice.
func (v *Validator) addIssue(code, message, path string) {
	issue := Issue{
		Code:     code,
		Message:  message,
		Path:     path,
		Severity: "error",
	}
	v.issues = append(v.issues, issue)
}
