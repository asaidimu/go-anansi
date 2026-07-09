package definition_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockPredicate(_ string, validationFn func(value map[string]any, keys []definition.FieldName, params definition.LiteralValue) bool, issue common.Issue) definition.Predicate {

	return func(params definition.PredicateParams) []common.Issue {
		// params.Data is already the field value (or map of values)
		// Just validate it directly
		data, ok := params.Data.(map[string]any)
		if !ok {
			return []common.Issue{{Code: "INVALID_ROOT_TYPE"}}
		}
		if !validationFn(data, params.Keys, params.Parameters) {
			resultIssue := issue
			// Set path if provided in Keys
			if len(params.Keys) > 0 && resultIssue.Path == "" {
				resultIssue.Path = string(params.Keys[0])
			}
			return []common.Issue{resultIssue}
		}
		return nil
	}
}

// getIssueCodes extracts only the Code from a slice of common.Issue
func getIssueCodes(issues []common.Issue) []string {
	codes := make([]string, len(issues))
	for i, issue := range issues {
		codes[i] = issue.Code
	}
	return codes
}

// testValidateWrapper wraps the validator.Validate method to handle non-object root inputs
// and correctly dispatch to the appropriate validation method based on the mode.
func testValidateWrapper(validator *definition.DocumentValidator, data any, mode definition.ValidationMode) ([]common.Issue, bool) {
	if _, ok := data.(map[string]any); !ok {
		// If the root data is not an object, immediately return a type mismatch error
		return []common.Issue{{Code: "INVALID_ROOT_TYPE", Message: "Expected object at root"}}, false
	}

	switch mode {
	case definition.ValidationModeStrict:
		return validator.Validate(data.(map[string]any))
	case definition.ValidationModePartialStrict:
		return validator.ValidatePartial(data.(map[string]any))
	case definition.ValidationModeLoose:
		return validator.ValidateLoose(data.(map[string]any))
	default:
		// Default to strict if mode is not explicitly handled (shouldn't happen with proper test setup)
		return validator.Validate(data.(map[string]any))
	}
}

func TestNewValidator(t *testing.T) {
	// Setup a simple Schema
	testSchema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"field1_id": {Name: "field1", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}

	predicates := definition.PredicateMap{
		"test_predicate": mockPredicate("test_predicate", func(value map[string]any, keys []definition.FieldName, params definition.LiteralValue) bool { return true }, common.Issue{}),
	}

	validator, err := definition.NewDocumentValidator(&testSchema, predicates)
	require.NoError(t, err)
	require.NotNil(t, validator)
	// Direct access to unexported fields removed. Verification done through public methods or side effects.
}

func TestValidator_Validate_NonObjectRoot(t *testing.T) {
	// Create a minimal valid schema for the validator
	schema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"dummy_id": {Name: "dummy", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}
	validator, err := definition.NewDocumentValidator(&schema, nil)
	require.NoError(t, err)
	issues, _ := testValidateWrapper(validator, "not an object", definition.ValidationModeStrict)
	require.Len(t, issues, 1)
	assert.Equal(t, "INVALID_ROOT_TYPE", issues[0].Code)
	assert.Contains(t, issues[0].Message, "Expected object at root")
}

func TestValidator_Validate_Structural(t *testing.T) {
	// Create a schema with one required string field and one optional integer field
	schema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"field1_id": {Name: "field1", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"field2_id": {Name: "field2", Required: false, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
		},
	}
	validator, err := definition.NewDocumentValidator(&schema, nil)
	require.NoError(t, err)
	tests := []struct {
		name           string
		object         map[string]any
		mode           definition.ValidationMode
		expectedIssues int
		expectedCodes  []string
	}{
		{
			name:           "Strict - Missing required field",
			object:         map[string]any{"field2": 123},
			mode:           definition.ValidationModeStrict,
			expectedIssues: 1,
			expectedCodes:  []string{"REQUIRED_FIELD_MISSING"},
		},
		{
			name:           "Strict - All fields present and correct types",
			object:         map[string]any{"field1": "hello", "field2": 123},
			mode:           definition.ValidationModeStrict,
			expectedIssues: 0,
			expectedCodes:  nil,
		},
		{
			name:           "PartialStrict - Missing required field (should not error structural, just skip type check)",
			object:         map[string]any{"field2": 123},
			mode:           definition.ValidationModePartialStrict,
			expectedIssues: 0,
			expectedCodes:  nil,
		},
		{
			name:           "Loose - Missing required field (should not error structural, just skip type check)",
			object:         map[string]any{"field2": 123},
			mode:           definition.ValidationModeLoose,
			expectedIssues: 0,
			expectedCodes:  nil,
		},
		{
			name:           "Strict - Type mismatch for field1 (string expected, int given)",
			object:         map[string]any{"field1": 123, "field2": 456},
			mode:           definition.ValidationModeStrict,
			expectedIssues: 1,
			expectedCodes:  []string{"TYPE_MISMATCH"},
		},
		{
			name:           "Strict - Unexpected field (warning)",
			object:         map[string]any{"field1": "hello", "field2": 123, "extra": true},
			mode:           definition.ValidationModeStrict,
			expectedIssues: 1,
			expectedCodes:  []string{"UNEXPECTED_FIELD"},
		},
		{
			name:           "PartialStrict - Type mismatch for field1 (string expected, int given)",
			object:         map[string]any{"field1": 123, "field2": 456},
			mode:           definition.ValidationModePartialStrict,
			expectedIssues: 1,
			expectedCodes:  []string{"TYPE_MISMATCH"},
		},
		{
			name:           "Loose - Type mismatch for field1 (string expected, int given)",
			object:         map[string]any{"field1": 123, "field2": 456},
			mode:           definition.ValidationModeLoose,
			expectedIssues: 1,
			expectedCodes:  []string{"TYPE_MISMATCH"},
		},
		{
			name:           "Strict - Field1 present, Field2 missing (optional)",
			object:         map[string]any{"field1": "value1"},
			mode:           definition.ValidationModeStrict,
			expectedIssues: 0,
			expectedCodes:  nil,
		},
		{
			name:           "PartialStrict - Field1 present, Field2 missing (optional)",
			object:         map[string]any{"field1": "value1"},
			mode:           definition.ValidationModePartialStrict,
			expectedIssues: 0,
			expectedCodes:  nil,
		},
		{
			name:           "Loose - Field1 present, Field2 missing (optional)",
			object:         map[string]any{"field1": "value1"},
			mode:           definition.ValidationModeLoose,
			expectedIssues: 0,
			expectedCodes:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var issues []common.Issue
			switch tt.mode {
			case definition.ValidationModeStrict:
				issues, _ = validator.Validate(tt.object)
			case definition.ValidationModePartialStrict:
				issues, _ = validator.ValidatePartial(tt.object)
			case definition.ValidationModeLoose:
				issues, _ = validator.ValidateLoose(tt.object)
			}

			require.Len(t, issues, tt.expectedIssues)
			// Issues slice is unordered, so check codes exist rather than exact order
			assert.ElementsMatch(t, tt.expectedCodes, getIssueCodes(issues))
		})
	}
}

func TestEffectiveField_Validate_TypeSpecific(t *testing.T) {
	// Create a full schema for resolution
	schema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"string_id":  {Name: "strField", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"integer_id": {Name: "intField", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"number_id":  {Name: "numField", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"boolean_id": {Name: "boolField", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeBoolean}},
			},
		},
	}
	validator, err := definition.NewDocumentValidator(&schema, nil)
	require.NoError(t, err)
	tests := []struct {
		name           string
		fieldName      string // Use field name to construct object for validation
		value          any
		mode           definition.ValidationMode
		expectedIssues int
		expectedCodes  []string
	}{
		// String Field
		{name: "String - Valid", fieldName: "strField", value: "hello", mode: definition.ValidationModeStrict, expectedIssues: 0, expectedCodes: nil},
		{name: "String - Invalid (int)", fieldName: "strField", value: 123, mode: definition.ValidationModeStrict, expectedIssues: 1, expectedCodes: []string{"TYPE_MISMATCH"}},
		{name: "String - Nil", fieldName: "strField", value: nil, mode: definition.ValidationModeStrict, expectedIssues: 0, expectedCodes: nil},

		// Integer Field
		{name: "Integer - Valid (int)", fieldName: "intField", value: 123, mode: definition.ValidationModeStrict, expectedIssues: 0, expectedCodes: nil},
		{name: "Integer - Valid (int64)", fieldName: "intField", value: int64(123), mode: definition.ValidationModeStrict, expectedIssues: 0, expectedCodes: nil},
		{name: "Integer - Invalid (string)", fieldName: "intField", value: "abc", mode: definition.ValidationModeStrict, expectedIssues: 1, expectedCodes: []string{"TYPE_MISMATCH"}},
		{name: "Integer - Invalid (float)", fieldName: "intField", value: 1.23, mode: definition.ValidationModeStrict, expectedIssues: 1, expectedCodes: []string{"TYPE_MISMATCH"}},

		// Number Field (float, int, int64 etc.)
		{name: "Number - Valid (float)", fieldName: "numField", value: 1.23, mode: definition.ValidationModeStrict, expectedIssues: 0, expectedCodes: nil},
		{name: "Number - Valid (int)", fieldName: "numField", value: 123, mode: definition.ValidationModeStrict, expectedIssues: 0, expectedCodes: nil},
		{name: "Number - Invalid (string)", fieldName: "numField", value: "abc", mode: definition.ValidationModeStrict, expectedIssues: 1, expectedCodes: []string{"TYPE_MISMATCH"}},

		// Boolean Field
		{name: "Boolean - Valid (true)", fieldName: "boolField", value: true, mode: definition.ValidationModeStrict, expectedIssues: 0, expectedCodes: nil},
		{name: "Boolean - Valid (false)", fieldName: "boolField", value: false, mode: definition.ValidationModeStrict, expectedIssues: 0, expectedCodes: nil},
		{name: "Boolean - Invalid (string)", fieldName: "boolField", value: "true", mode: definition.ValidationModeStrict, expectedIssues: 1, expectedCodes: []string{"TYPE_MISMATCH"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrap ef.Validate call within validator's validate method to get a proper ValidationContext
			object := map[string]any{tt.fieldName: tt.value}
			var issues []common.Issue
			switch tt.mode {
			case definition.ValidationModeStrict:
				issues, _ = validator.Validate(object)
			case definition.ValidationModePartialStrict:
				issues, _ = validator.ValidatePartial(object)
			case definition.ValidationModeLoose:
				issues, _ = validator.ValidateLoose(object)
			}

			// t.Logf("Issues for %s: %+v", tt.name, issues)
			require.Len(t, issues, tt.expectedIssues)
			assert.ElementsMatch(t, tt.expectedCodes, getIssueCodes(issues))
		})
	}
}

func TestEffectiveField_Validate_Enum(t *testing.T) {
	enumSchemaID := definition.SchemaId("ColorsEnum")
	schema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"color_id": {
					Name: "color",
					FieldProperties: definition.FieldProperties{
						Type:   definition.FieldTypeEnum,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: enumSchemaID}),
					},
				},
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			enumSchemaID: {
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
				Values: []definition.LiteralValue{
					func() definition.LiteralValue { lv, _ := definition.NewLiteralValue("red"); return lv }(),
					func() definition.LiteralValue { lv, _ := definition.NewLiteralValue("green"); return lv }(),
					func() definition.LiteralValue { lv, _ := definition.NewLiteralValue("blue"); return lv }(),
				},
			},
		},
	}
	validator, err := definition.NewDocumentValidator(&schema, nil)
	require.NoError(t, err)

	tests := []struct {
		name           string
		value          any
		expectedIssues int
		expectedCodes  []string
	}{
		{name: "Enum - Valid (red)", value: "red", expectedIssues: 0, expectedCodes: nil},
		{name: "Enum - Valid (blue)", value: "blue", expectedIssues: 0, expectedCodes: nil},
		{name: "Enum - Invalid (yellow)", value: "yellow", expectedIssues: 1, expectedCodes: []string{"ENUM_VIOLATION"}},
		{name: "Enum - Nil", value: nil, expectedIssues: 0, expectedCodes: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			object := map[string]any{"color": tt.value}
			issues, _ := validator.Validate(object)
			require.Len(t, issues, tt.expectedIssues)
			assert.ElementsMatch(t, tt.expectedCodes, getIssueCodes(issues))
		})
	}
}

func TestEffectiveField_Validate_Array(t *testing.T) {
	arrayElementSchemaId := definition.SchemaId("ArrayElementString")
	schema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"array_id": {
					Name: "stringArray",
					FieldProperties: definition.FieldProperties{
						Type:   definition.FieldTypeArray,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: arrayElementSchemaId}),
					},
				},
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			arrayElementSchemaId: {
				FieldProperties: definition.FieldProperties{
					Type: definition.FieldTypeString,
				},
			},
		},
	}
	validator, err := definition.NewDocumentValidator(&schema, nil)
	require.NoError(t, err)

	tests := []struct {
		name           string
		value          any
		expectedIssues int
		expectedCodes  []string
	}{
		{name: "Array - Valid (string array)", value: []any{"a", "b", "c"}, expectedIssues: 0, expectedCodes: nil},
		{name: "Array - Invalid (mixed types - element type mismatch)", value: []any{"a", 123}, expectedIssues: 1, expectedCodes: []string{"TYPE_MISMATCH"}},
		{name: "Array - Invalid (not array)", value: "not an array", expectedIssues: 1, expectedCodes: []string{"ARRAY_TYPE_MISMATCH"}},
		{name: "Array - Empty array", value: []any{}, expectedIssues: 0, expectedCodes: nil},
		{name: "Array - Nil", value: nil, expectedIssues: 0, expectedCodes: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			object := map[string]any{"stringArray": tt.value}
			issues, _ := validator.Validate(object)
			require.Len(t, issues, tt.expectedIssues)
			assert.ElementsMatch(t, tt.expectedCodes, getIssueCodes(issues))
		})
	}
}

func TestEffectiveField_Validate_Object(t *testing.T) {
	objectSchemaId := definition.SchemaId("UserObject")
	schema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"object_id": {
					Name: "user",
					FieldProperties: definition.FieldProperties{
						Type:   definition.FieldTypeObject,
						Schema: definition.NewSchemaReference(definition.SchemaReference{ID: objectSchemaId}),
					},
				},
			},
		},
		Schemas: map[definition.SchemaId]definition.NestedSchema{
			objectSchemaId: {
				BaseSchema: definition.BaseSchema{
					Fields: map[definition.FieldId]definition.Field{
						"name_id": {Name: "name", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
						"age_id":  {Name: "age", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
					},
				},
			},
		},
	}
	validator, _ := definition.NewDocumentValidator(&schema, nil)

	tests := []struct {
		name           string
		value          any
		mode           definition.ValidationMode
		expectedIssues int
		expectedCodes  []string
	}{
		{name: "Object - Valid", value: map[string]any{"name": "Alice", "age": 30}, mode: definition.ValidationModeStrict, expectedIssues: 0, expectedCodes: nil},
		{name: "Object - Missing required", value: map[string]any{"age": 30}, mode: definition.ValidationModeStrict, expectedIssues: 1, expectedCodes: []string{"REQUIRED_FIELD_MISSING"}},
		{name: "Object - Type mismatch (age)", value: map[string]any{"name": "Alice", "age": "thirty"}, mode: definition.ValidationModeStrict, expectedIssues: 1, expectedCodes: []string{"TYPE_MISMATCH"}},
		{name: "Object - Unexpected field (warning)", value: map[string]any{"name": "Alice", "extra": true}, mode: definition.ValidationModeStrict, expectedIssues: 1, expectedCodes: []string{"UNEXPECTED_FIELD"}},
		{name: "Object - Invalid (not object)", value: "not object", mode: definition.ValidationModeStrict, expectedIssues: 1, expectedCodes: []string{"TYPE_MISMATCH"}},
		{name: "Object - Nil", value: nil, mode: definition.ValidationModeStrict, expectedIssues: 0, expectedCodes: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validator.Validate handles the creation of ValidationContext with appropriate mode
			var issues []common.Issue
			switch tt.mode {
			case definition.ValidationModeStrict:
				issues, _ = validator.Validate(map[string]any{"user": tt.value})
			case definition.ValidationModePartialStrict:
				issues, _ = validator.ValidatePartial(map[string]any{"user": tt.value})
			case definition.ValidationModeLoose:
				issues, _ = validator.ValidateLoose(map[string]any{"user": tt.value})
			}

			require.Len(t, issues, tt.expectedIssues)
			assert.ElementsMatch(t, tt.expectedCodes, getIssueCodes(issues))
		})
	}
}
func TestValidator_Validate_Constraints(t *testing.T) {
	predicates := definition.PredicateMap{
		"is_positive": mockPredicate("is_positive",
			func(v map[string]any, keys []definition.FieldName, _ definition.LiteralValue) bool {
				if len(keys) == 0 {
					return false
				}
				k, ok := v[string(keys[0])]
				if !ok {
					return false
				}

				i, ok := k.(int)
				return ok && i > 0
			},
			common.Issue{Code: "NOT_POSITIVE", Message: "Must be > 0"}),
		"is_negative": mockPredicate("is_negative",
			func(v map[string]any, keys []definition.FieldName, _ definition.LiteralValue) bool {
				if len(keys) == 0 {
					return false
				}
				k, ok := v[string(keys[0])]
				if !ok {
					return false
				}

				i, ok := k.(int)
				return ok && i < 0
			},
			common.Issue{Code: "NOT_NEGATIVE", Message: "Must be < 0"}),
		"is_zero": mockPredicate("is_zero",
			func(v map[string]any, keys []definition.FieldName, _ definition.LiteralValue) bool {
				if len(keys) == 0 {
					return false
				}
				k, ok := v[string(keys[0])]
				if !ok {
					return false
				}

				i, ok := k.(int)
				return ok && i == 0
			},
			common.Issue{Code: "NOT_ZERO", Message: "Must be 0"}),
	}

	schema := definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: map[definition.FieldId]definition.Field{
				"f1": {Name: "field1", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"f2": {Name: "field2", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
			},
			Constraints: map[definition.ConstraintId]definition.Constraint{
				// Standalone rule for field1
				"rule_f1": {
					Name: "f1_must_be_positive",
					ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintRule{
						Fields: []definition.FieldName{"field1"}, Predicate: "is_positive",
					}),
				},
				// Group rule: field2 must be negative AND field1 must be zero
				"group_rule": {
					Name: "coordinated_state",
					ConstraintUnion: definition.NewConstrainUnion(&definition.ConstraintGroup{
						Operator: common.LogicalAnd,
						Rules: []definition.ConstraintUnion{
							definition.NewConstrainUnion(&definition.ConstraintRule{
								Fields: []definition.FieldName{"field2"}, Predicate: "is_negative",
							}),
							definition.NewConstrainUnion(&definition.ConstraintRule{
								Fields: []definition.FieldName{"field1"}, Predicate: "is_zero",
							}),
						},
					}),
				},
			},
		},
	}

	validator, _ := definition.NewDocumentValidator(&schema, predicates)

	tests := []struct {
		name           string
		object         map[string]any
		mode           definition.ValidationMode
		expectedIssues int
		expectedCodes  []string
		description    string
	}{
		// ===== STRICT MODE =====
		{
			name:   "Strict - Both constraints check field1",
			object: map[string]any{"field1": -5, "field2": -10},
			mode:   definition.ValidationModeStrict,
			// rule_f1: field1=-5 NOT positive → FAIL
			// group: field2=-10 IS negative ✓, field1=-5 NOT zero → FAIL
			expectedIssues: 3,
			expectedCodes:  []string{"NOT_POSITIVE", "CONSTRAINT_GROUP_VIOLATION", "NOT_ZERO"},
		},
		{
			name:   "Strict - field1 positive satisfies rule_f1 but fails group",
			object: map[string]any{"field1": 10, "field2": -10},
			mode:   definition.ValidationModeStrict,
			// rule_f1: field1=10 IS positive ✓
			// group: field2=-10 IS negative ✓, field1=10 NOT zero → FAIL
			expectedIssues: 2,
			expectedCodes:  []string{"CONSTRAINT_GROUP_VIOLATION", "NOT_ZERO"},
		},
		{
			name:   "Strict - field1=0 satisfies group but fails rule_f1",
			object: map[string]any{"field1": 0, "field2": -10},
			mode:   definition.ValidationModeStrict,
			// rule_f1: field1=0 NOT positive → FAIL
			// group: field2=-10 IS negative ✓, field1=0 IS zero ✓ → PASS
			expectedIssues: 1,
			expectedCodes:  []string{"NOT_POSITIVE"},
		},
		{
			name:   "Strict - Missing field",
			object: map[string]any{"field1": 10},
			mode:   definition.ValidationModeStrict,
			// rule_f1: field1=10 IS positive ✓
			// group: field2 MISSING → FAIL (incomplete)
			expectedIssues: 1,
			expectedCodes:  []string{"CONSTRAINT_INCOMPLETE"},
		},

		// ===== PARTIAL STRICT MODE =====
		{
			name:   "PartialStrict - Partial group update FAILS",
			object: map[string]any{"field1": 10},
			mode:   definition.ValidationModePartialStrict,
			// rule_f1: field1=10 IS positive ✓
			// group: field1 present, field2 MISSING → PARTIAL UPDATE → FAIL
			expectedIssues: 1,
			expectedCodes:  []string{"CONSTRAINT_PARTIAL_UPDATE"},
		},
		{
			name:   "PartialStrict - field1=0 (group passes, standalone fails)",
			object: map[string]any{"field1": 0, "field2": -10},
			mode:   definition.ValidationModePartialStrict,
			// rule_f1: field1=0 NOT positive → FAIL
			// group: both present, field2=-10 negative ✓, field1=0 zero ✓ → PASS
			expectedIssues: 1,
			expectedCodes:  []string{"NOT_POSITIVE"},
		},
		{
			name:   "PartialStrict - No fields present",
			object: map[string]any{},
			mode:   definition.ValidationModePartialStrict,
			// rule_f1: field1 missing → SKIP
			// group: all missing → SKIP
			expectedIssues: 0,
			expectedCodes:  []string{},
		},

		// ===== LOOSE MODE =====
		{
			name:   "Loose - Partial data skips group",
			object: map[string]any{"field1": 10},
			mode:   definition.ValidationModeLoose,
			// rule_f1: field1=10 IS positive ✓
			// group: field1 present, field2 missing → SKIP (loose is forgiving)
			expectedIssues: 0,
			expectedCodes:  []string{},
		},
		{
			name:   "Loose - Standalone fails, group skips",
			object: map[string]any{"field1": -5},
			mode:   definition.ValidationModeLoose,
			// rule_f1: field1=-5 NOT positive → FAIL
			// group: field1 present, field2 missing → SKIP
			expectedIssues: 1,
			expectedCodes:  []string{"NOT_POSITIVE"},
		},
		{
			name:   "Loose - Both fields present, both evaluated",
			object: map[string]any{"field1": -5, "field2": 10},
			mode:   definition.ValidationModeLoose,
			// rule_f1: field1=-5 NOT positive → FAIL
			// group: field2=10 NOT negative → FAIL, field1=-5 NOT zero → FAIL
			expectedIssues: 4,
			expectedCodes:  []string{"NOT_POSITIVE", "CONSTRAINT_GROUP_VIOLATION", "NOT_NEGATIVE", "NOT_ZERO"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var issues []common.Issue
			switch tt.mode {
			case definition.ValidationModeStrict:
				issues, _ = validator.Validate(tt.object)
			case definition.ValidationModePartialStrict:
				issues, _ = validator.ValidatePartial(tt.object)
			case definition.ValidationModeLoose:
				issues, _ = validator.ValidateLoose(tt.object)
			}

			if len(issues) != tt.expectedIssues {
				t.Errorf("\n%s\nExpected %d issues, got %d\nIssues: %+v",
					tt.description, tt.expectedIssues, len(issues), issues)
			}

			actualCodes := []string{}
			for _, iss := range issues {
				actualCodes = append(actualCodes, iss.Code)
			}

			if !assert.ElementsMatch(t, tt.expectedCodes, actualCodes) {
				t.Errorf("\n%s\nExpected codes: %v\nActual codes: %v",
					tt.description, tt.expectedCodes, actualCodes)
			}
		})
	}
}
