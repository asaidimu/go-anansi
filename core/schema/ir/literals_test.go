package ir_test

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiteralValue_Type(t *testing.T) {
	testCases := []struct {
		name        string
		lv          ir.LiteralValue
		expectedTyp ir.LiteralType
		expectError bool
	}{
		{"String", MustNewLiteralValueStrict("hello"), ir.LiteralTypeString, false},
		{"Int", MustNewLiteralValueStrict(int64(123)), ir.LiteralTypeInteger, false},
		{"Float", MustNewLiteralValueStrict(123.45), ir.LiteralTypeFloat, false},
		{"Boolean", MustNewLiteralValueStrict(true), ir.LiteralTypeBoolean, false},
		{"Object", MustNewLiteralValueStrict(map[string]any{"key": "value"}), ir.LiteralTypeObject, false},
		{"Array", MustNewLiteralValueStrict([]any{"a", int64(1)}), ir.LiteralTypeArray, false},
		{"Null", ir.NewNullLiteral(), ir.LiteralTypeNull, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			typ, err := tc.lv.Type()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedTyp, typ)
			}
		})
	}
}

func TestLiteralValue_IsZero_IsNull(t *testing.T) {
	assert.True(t, (ir.LiteralValue{}).IsZero(), "Zero value should be zero")
	assert.False(t, MustNewLiteralValueStrict("").IsZero(), "Empty string should not be zero")
	assert.False(t, MustNewLiteralValueStrict(int64(0)).IsZero(), "Int value 0 should not be zero")
	assert.False(t, MustNewLiteralValueStrict(float64(0.0)).IsZero(), "Float value 0.0 should not be zero")

	assert.True(t, ir.NewNullLiteral().IsNull(), "Null value should be null")
	assert.False(t, (ir.LiteralValue{}).IsNull(), "Zero value should not be null")
	assert.False(t, MustNewLiteralValueStrict("").IsNull(), "Empty string should not be null")
}

func TestLiteralValue_JSON_Marshaling_Unmarshaling(t *testing.T) {
	testCases := []struct {
		name         string
		value        ir.LiteralValue
		expectedJSON string
	}{
		{"String", MustNewLiteralValueStrict("hello world"), `"hello world"`},
		{"Integer", MustNewLiteralValueStrict(int64(42)), `42`},
		{"Large Integer", MustNewLiteralValueStrict(int64(math.MaxInt64)), fmt.Sprintf("%d", int64(math.MaxInt64))},
		{"Float", MustNewLiteralValueStrict(3.14), `3.14`},
		{"Boolean true", MustNewLiteralValueStrict(true), `true`},
		{"Boolean false", MustNewLiteralValueStrict(false), `false`},
		{"Null", ir.NewNullLiteral(), `null`},
		{"Empty Object", MustNewLiteralValueStrict(map[string]any{}), `{}`},
		{"Simple Object", MustNewLiteralValueStrict(map[string]any{"key": "value", "num": int64(123)}), `{"key":"value","num":123}`},
		{"Empty Array", MustNewLiteralValueStrict([]any{}), `[]`},
		{"Simple Array", MustNewLiteralValueStrict([]any{"a", int64(1), true, nil}), `["a",1,true,null]`},
		{"Zero Value", ir.LiteralValue{}, `null`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test Marshaling
			jsonData, err := json.Marshal(tc.value)
			require.NoError(t, err, "Marshaling failed")
			require.JSONEq(t, tc.expectedJSON, string(jsonData), "Marshaled JSON does not match")

			// Test Unmarshaling
			var unmarshaledValue ir.LiteralValue
			err = json.Unmarshal(jsonData, &unmarshaledValue)
			require.NoError(t, err, "Unmarshaling failed")

			// Compare the original and unmarshaled values
			// Due to number type differences (e.g., int vs int64), direct comparison might fail.
			// It's better to compare the marshaled output again.
			remarshaledJSON, err := json.Marshal(unmarshaledValue)
			require.NoError(t, err, "Re-marshaling failed")
			require.JSONEq(t, string(jsonData), string(remarshaledJSON), "Unmarshaled value is not equivalent to original")

			// Also check IsNull and IsZero for edge cases
			if tc.name == "Null" || tc.name == "Zero Value" {
				assert.True(t, unmarshaledValue.IsNull())
			}
		})
	}
}

func TestLiteralValue_Validate(t *testing.T) {
	testCases := []struct {
		name                string
		newLiteralFunc      func() (ir.LiteralValue, error)
		expectedIssues      int
		expectCreationError bool
	}{
		{"Valid String", func() (ir.LiteralValue, error) { return ir.NewLiteralValue("hello") }, 0, false},
		{"Valid Int", func() (ir.LiteralValue, error) { return ir.NewLiteralValue(int64(123)) }, 0, false},
		{"Valid Float", func() (ir.LiteralValue, error) { return ir.NewLiteralValue(123.45) }, 0, false},
		{"Valid Boolean", func() (ir.LiteralValue, error) { return ir.NewLiteralValue(true) }, 0, false},
		{"Valid Object", func() (ir.LiteralValue, error) {
			return ir.NewLiteralValue(map[string]any{"key": "value", "num": int64(123)})
		}, 0, false},
		{"Valid Array", func() (ir.LiteralValue, error) {
			return ir.NewLiteralValue([]any{"a", int64(1), true, ir.NewNullLiteral().Value()})
		}, 0, false},
		{"Valid Null", func() (ir.LiteralValue, error) { return ir.NewNullLiteral(), nil }, 0, false},
		{"Zero Value", func() (ir.LiteralValue, error) { return ir.LiteralValue{}, nil }, 0, false},

		// Test cases where NewLiteralValue is expected to return an error (invalid creation)
		{
			name: "Array with invalid element (struct)",
			newLiteralFunc: func() (ir.LiteralValue, error) {
				return ir.NewLiteralValue([]any{"a", struct{ Name string }{"test"}}) // Struct is not a literal
			},
			expectedIssues:      0, // Issues are not checked if creation errors
			expectCreationError: true,
		},
		{
			name: "Array with invalid element (channel)",
			newLiteralFunc: func() (ir.LiteralValue, error) {
				return ir.NewLiteralValue([]any{"a", make(chan int)}) // Channel is not a literal
			},
			expectedIssues:      0, // Issues are not checked if creation errors
			expectCreationError: true,
		},
		{
			name: "Object with invalid value (struct)",
			newLiteralFunc: func() (ir.LiteralValue, error) {
				return ir.NewLiteralValue(map[string]any{"key": "value", "invalid": struct{ Name string }{"test"}})
			},
			expectedIssues:      0, // Issues are not checked if creation errors
			expectCreationError: true,
		},
		{
			name: "Object with invalid value (channel)",
			newLiteralFunc: func() (ir.LiteralValue, error) {
				return ir.NewLiteralValue(map[string]any{"key": "value", "invalid": make(chan int)})
			},
			expectedIssues:      0, // Issues are not checked if creation errors
			expectCreationError: true,
		},
		{
			name:                "Object with nil value",
			newLiteralFunc:      func() (ir.LiteralValue, error) { return ir.NewLiteralValue(map[string]any{"key": nil}) },
			expectedIssues:      0, // nil is a valid literal map value - this was verified as passing before
			expectCreationError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lv, err := tc.newLiteralFunc()

			if tc.expectCreationError {
				assert.Error(t, err, "NewLiteralValue should have returned an error for invalid input")
				return // Skip validation if creation failed
			} else {
				require.NoError(t, err, "NewLiteralValue should not have returned an error for valid input")
			}

			issues := lv.Validate()
			assert.Len(t, issues, tc.expectedIssues)
			if tc.expectedIssues > 0 {
				assert.NotEmpty(t, issues[0].Code)
			}
		})
	}
}

func TestLiteralValue_String(t *testing.T) {
	testCases := []struct {
		name     string
		lv       ir.LiteralValue
		expected string
	}{
		{"String", MustNewLiteralValueStrict("hello"), `"hello"`},
		{"Integer", MustNewLiteralValueStrict(int64(42)), `42`},
		{"Float", MustNewLiteralValueStrict(3.14), `3.14`},
		{"Boolean true", MustNewLiteralValueStrict(true), `true`},
		{"Boolean false", MustNewLiteralValueStrict(false), `false`},
		{"Object", MustNewLiteralValueStrict(map[string]any{"key": "value"}), `{"key":"value"}`},
		{"Array", MustNewLiteralValueStrict([]any{"a", int64(1), true}), `["a",1,true]`},
		{"Null", ir.NewNullLiteral(), `null`},
		{"Zero Value", ir.LiteralValue{}, `null`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.lv.String())
		})
	}
}

func TestValidateLiteral(t *testing.T) {
	testCases := []struct {
		name     string
		input    any
		expected bool
	}{
		{"String", "hello", true},
		{"Integer", 123, true},
		{"Float", 123.45, true},
		{"Boolean", true, true},
		{"Nil", nil, true},
		{"Object", map[string]any{"key": "value"}, true},
		{"Array", []any{"a", 1}, true},
		{"Array with nested Array", []any{"a", []any{"b", 2}}, true},
		{"Array with nested Object", []any{"a", map[string]any{"key": "value"}}, true},
		{"Object with nested Object", map[string]any{"key": map[string]any{"nested": 1}}, true},
		{"Object with nested Array", map[string]any{"key": []any{"a", 1}}, true},
		{"Pointer to String", new(string), true}, // Pointer to literal type is valid
		{"Pointer to Int", new(int), true},       // Pointer to literal type is valid

		{"Invalid: Struct", struct{ Name string }{"test"}, false},
		{"Invalid: Array with Struct", []any{"a", struct{ Name string }{"test"}}, false},
		{"Invalid: Object with Struct", map[string]any{"key": struct{ Name string }{"test"}}, false},
		{"Invalid: Channel", make(chan int), false},
		{"Invalid: Function", func() {}, false},

		// Additional types for ValidateLiteral (includes coverage for isTypeLiteral)
		{"Uint", uint(123), true},
		{"Uint8", uint8(123), true},
		{"Uint16", uint16(123), true},
		{"Uint32", uint32(123), true},
		{"Uint64", uint64(123), true},
		{"Float32", float32(123.45), true},

		// Complex valid nested types for ValidateLiteral (includes coverage for isTypeLiteral recursion)
		{"Array of Map string to string", []any{map[string]string{"key": "value"}}, true},
		{"Map string to Array of int", map[string]any{"key": []int{1, 2}}, true},
		{"Map string to Array of any", map[string]any{"key": []any{"a", 1, true}}, true},
		{"Array of Pointers to string", []any{new(string)}, true},
		{"Map string to Pointer to int", map[string]any{"key": new(int)}, true},

		// Invalid: Array of invalid maps (non-string key)
		{"Invalid: Array of Map int to string", []any{map[int]string{1: "value"}}, false},
		// Invalid: Map with non-string key
		{"Invalid: Map int to string", map[int]string{1: "value"}, false},
		// Invalid: Pointer to Channel (already somewhat covered, but good to be explicit)
		{"Invalid: Pointer to Channel", (func() *chan int { ch := make(chan int); return &ch })(), false},

		// New cases to hit more branches in isTypeLiteral
		{"Invalid: Slice of Struct", []struct{ Name string }{}, false},
		{"Invalid: Slice of Channel", []chan int{make(chan int)}, false},
		{"Invalid: Map string to Struct", map[string]struct{}{"key": {}}, false},
		{"Invalid: Map string to Channel", map[string]chan int{"key": make(chan int)}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, ir.ValidateLiteral(tc.input))
		})
	}
}

func TestLiteralValue_Value_And_LiteralValueAs(t *testing.T) {
	testCases := []struct {
		name          string
		lv            ir.LiteralValue
		expectedValue any
		targetType    string // String representation of the type to cast to
		expectedOk    bool
	}{
		// Value() tests
		{"Value: String", MustNewLiteralValueStrict("hello"), "hello", "", true},
		{"Value: Int", MustNewLiteralValueStrict(int64(123)), int64(123), "", true},
		{"Value: Float", MustNewLiteralValueStrict(3.14), 3.14, "", true},
		{"Value: Boolean", MustNewLiteralValueStrict(true), true, "", true},
		{"Value: Object", MustNewLiteralValueStrict(map[string]any{"a": int64(1)}), map[string]any{"a": int64(1)}, "", true},
		{"Value: Array", MustNewLiteralValueStrict([]any{int64(1), "b"}), []any{int64(1), "b"}, "", true},
		{"Value: Null", ir.NewNullLiteral(), nil, "", true},
		{"Value: Zero", ir.LiteralValue{}, nil, "", true},

		// LiteralValueAs tests - direct matches
		{"As: String to string", MustNewLiteralValueStrict("hello"), "hello", "string", true},
		{"As: Int to int64", MustNewLiteralValueStrict(int64(123)), int64(123), "int64", true},
		{"As: Float to float64", MustNewLiteralValueStrict(3.14), 3.14, "float64", true},
		{"As: Boolean to bool", MustNewLiteralValueStrict(true), true, "bool", true},
		{"As: Object to map[string]any", MustNewLiteralValueStrict(map[string]any{"a": int64(1)}), map[string]any{"a": int64(1)}, "map[string]any", true},
		{"As: Array to []any", MustNewLiteralValueStrict([]any{int64(1), "b"}), []any{int64(1), "b"}, "[]any", true},

		// LiteralValueAs tests - numeric conversions
		{"As: Int to float64", MustNewLiteralValueStrict(int64(123)), float64(0), "float64", false},
		{"As: Float to int64 (exact)", MustNewLiteralValueStrict(123.0), int64(0), "int64", false},

		// LiteralValueAs tests - mismatches
		{"As: String to int64", MustNewLiteralValueStrict("hello"), int64(0), "int64", false},
		{"As: Int to string", MustNewLiteralValueStrict(int64(123)), "", "string", false},
		{"As: Float to string", MustNewLiteralValueStrict(3.14), "", "string", false},
		{"As: Float to int64 (fractional)", MustNewLiteralValueStrict(3.14), int64(0), "int64", false},
		{"As: Bool to string", MustNewLiteralValueStrict(true), "", "string", false}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test Value() method first
			if tc.targetType == "" { // Only for Value() specific tests
				actualValue := tc.lv.Value()
				if tc.expectedValue == nil {
					assert.Nil(t, actualValue)
				} else {
					assert.Equal(t, tc.expectedValue, actualValue)
				}
				// For Value() tests, expectedOk is always true if a value exists or it's null/zero
				assert.Equal(t, tc.expectedOk, actualValue != nil || tc.lv.IsNull() || tc.lv.IsZero())
				return // Skip LiteralValueAs part for Value() specific tests
			}

			// Test LiteralValueAs
			switch tc.targetType {
			case "string":
				val, err := ir.LiteralValueAs[string](tc.lv)
				assert.Equal(t, tc.expectedValue, val)
				if tc.expectedOk {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			case "int64":
				val, err := ir.LiteralValueAs[int64](tc.lv)
				assert.Equal(t, tc.expectedValue, val)
				if tc.expectedOk {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			case "float64":
				val, err := ir.LiteralValueAs[float64](tc.lv)
				assert.Equal(t, tc.expectedValue, val)
				if tc.expectedOk {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			case "bool":
				val, err := ir.LiteralValueAs[bool](tc.lv)
				assert.Equal(t, tc.expectedValue, val)
				if tc.expectedOk {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			case "map[string]any":
				val, err := ir.LiteralValueAs[map[string]any](tc.lv)
				assert.Equal(t, tc.expectedValue, val)
				if tc.expectedOk {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			case "[]any":
				val, err := ir.LiteralValueAs[[]any](tc.lv)
				assert.Equal(t, tc.expectedValue, val)
				if tc.expectedOk {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			default:
				t.Fatalf("Unknown targetType: %s", tc.targetType)
			}
		})
	}
}


// MustNewLiteralValueStrict is a test helper that creates a LiteralValue or panics.
func MustNewLiteralValueStrict[T ir.LiteralValueType](val T) ir.LiteralValue {
	lv, err := ir.NewLiteralValue(val)
	if err != nil {
		panic(err)
	}
	return lv
}
