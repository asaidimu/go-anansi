package json_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a compiler from schema string
func newCompiler(t *testing.T, schemaJSON string) *json.Compiler {
	compiler, err := json.NewCompiler([]byte(schemaJSON))
	require.NoError(t, err, "Failed to create compiler")
	return compiler
}

// Helper function to validate and expect success
func assertValid(t *testing.T, compiler *json.Compiler, dataJSON string) {
	err := compiler.Validate([]byte(dataJSON))
	assert.NoError(t, err, "Expected validation to pass")
}

// Helper function to validate and expect failure
func assertInvalid(t *testing.T, compiler *json.Compiler, dataJSON string, expectedCode ...string) {
	err := compiler.Validate([]byte(dataJSON))
	require.Error(t, err, "Expected validation to fail")

	if len(expectedCode) > 0 {
		var sysErr *common.SystemError
		require.ErrorAs(t, err, &sysErr, "Expected SystemError type")

		found := false
		for _, issue := range sysErr.Issues {
			if issue.Code == expectedCode[0] {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected error code %s, got issues: %v", expectedCode[0], sysErr.Issues)
	}
}

// TestTypeValidation tests basic type validation
func TestTypeValidation(t *testing.T) {
	tests := []struct {
		name     string
		schema   string
		data     string
		expected bool
	}{
		{
			name:     "string type valid",
			schema:   `{"type": "string"}`,
			data:     `"hello"`,
			expected: true,
		},
		{
			name:     "string type invalid",
			schema:   `{"type": "string"}`,
			data:     `123`,
			expected: false,
		},
		{
			name:     "number type valid",
			schema:   `{"type": "number"}`,
			data:     `42.5`,
			expected: true,
		},
		{
			name:     "integer type valid",
			schema:   `{"type": "integer"}`,
			data:     `42`,
			expected: true,
		},
		{
			name:     "integer type invalid with float",
			schema:   `{"type": "integer"}`,
			data:     `42.5`,
			expected: false,
		},
		{
			name:     "boolean type valid",
			schema:   `{"type": "boolean"}`,
			data:     `true`,
			expected: true,
		},
		{
			name:     "null type valid",
			schema:   `{"type": "null"}`,
			data:     `null`,
			expected: true,
		},
		{
			name:     "array type valid",
			schema:   `{"type": "array"}`,
			data:     `[1, 2, 3]`,
			expected: true,
		},
		{
			name:     "object type valid",
			schema:   `{"type": "object"}`,
			data:     `{"key": "value"}`,
			expected: true,
		},
		{
			name:     "multiple types valid",
			schema:   `{"type": ["string", "number"]}`,
			data:     `"hello"`,
			expected: true,
		},
		{
			name:     "multiple types valid number",
			schema:   `{"type": ["string", "number"]}`,
			data:     `123`,
			expected: true,
		},
		{
			name:     "multiple types invalid",
			schema:   `{"type": ["string", "number"]}`,
			data:     `true`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiler := newCompiler(t, tt.schema)
			err := compiler.Validate([]byte(tt.data))

			if tt.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestEnumValidation tests enum constraint validation
func TestEnumValidation(t *testing.T) {
	schema := `{"enum": ["red", "green", "blue", 42, null]}`
	compiler := newCompiler(t, schema)

	assertValid(t, compiler, `"red"`)
	assertValid(t, compiler, `"green"`)
	assertValid(t, compiler, `42`)
	assertValid(t, compiler, `null`)
	assertInvalid(t, compiler, `"yellow"`, "INVALID_ENUM")
	assertInvalid(t, compiler, `43`, "INVALID_ENUM")
}

// TestConstValidation tests const constraint validation
func TestConstValidation(t *testing.T) {
	tests := []struct {
		name     string
		schema   string
		data     string
		expected bool
	}{
		{
			name:     "const string match",
			schema:   `{"const": "hello"}`,
			data:     `"hello"`,
			expected: true,
		},
		{
			name:     "const string mismatch",
			schema:   `{"const": "hello"}`,
			data:     `"world"`,
			expected: false,
		},
		{
			name:     "const number match",
			schema:   `{"const": 42}`,
			data:     `42`,
			expected: true,
		},
		{
			name:     "const object match",
			schema:   `{"const": {"a": 1, "b": 2}}`,
			data:     `{"a": 1, "b": 2}`,
			expected: true,
		},
		{
			name:     "const object mismatch",
			schema:   `{"const": {"a": 1, "b": 2}}`,
			data:     `{"a": 1, "b": 3}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiler := newCompiler(t, tt.schema)
			err := compiler.Validate([]byte(tt.data))

			if tt.expected {
				assert.NoError(t, err)
			} else {
				assertInvalid(t, compiler, tt.data, "INVALID_CONST")
			}
		})
	}
}

// TestNumericConstraints tests numeric validation
func TestNumericConstraints(t *testing.T) {
	t.Run("multipleOf", func(t *testing.T) {
		schema := `{"type": "number", "multipleOf": 5}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `10`)
		assertValid(t, compiler, `15`)
		assertValid(t, compiler, `0`)
		assertInvalid(t, compiler, `7`, "INVALID_MULTIPLE_OF")
		assertInvalid(t, compiler, `3.5`, "INVALID_MULTIPLE_OF")
	})

	t.Run("multipleOf decimal", func(t *testing.T) {
		schema := `{"type": "number", "multipleOf": 0.1}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `0.3`)
		assertValid(t, compiler, `1.5`)
		assertInvalid(t, compiler, `0.33`, "INVALID_MULTIPLE_OF")
	})

	t.Run("maximum", func(t *testing.T) {
		schema := `{"type": "number", "maximum": 100}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `100`)
		assertValid(t, compiler, `50`)
		assertInvalid(t, compiler, `101`, "VALUE_TOO_LARGE")
	})

	t.Run("exclusiveMaximum", func(t *testing.T) {
		schema := `{"type": "number", "exclusiveMaximum": 100}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `99.9`)
		assertInvalid(t, compiler, `100`, "VALUE_TOO_LARGE")
		assertInvalid(t, compiler, `101`, "VALUE_TOO_LARGE")
	})

	t.Run("minimum", func(t *testing.T) {
		schema := `{"type": "number", "minimum": 0}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `0`)
		assertValid(t, compiler, `10`)
		assertInvalid(t, compiler, `-1`, "VALUE_TOO_SMALL")
	})

	t.Run("exclusiveMinimum", func(t *testing.T) {
		schema := `{"type": "number", "exclusiveMinimum": 0}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `0.1`)
		assertInvalid(t, compiler, `0`, "VALUE_TOO_SMALL")
		assertInvalid(t, compiler, `-1`, "VALUE_TOO_SMALL")
	})

	t.Run("combined constraints", func(t *testing.T) {
		schema := `{"type": "number", "minimum": 0, "maximum": 100, "multipleOf": 5}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `0`)
		assertValid(t, compiler, `50`)
		assertValid(t, compiler, `100`)
		assertInvalid(t, compiler, `-5`, "VALUE_TOO_SMALL")
		assertInvalid(t, compiler, `105`, "VALUE_TOO_LARGE")
		assertInvalid(t, compiler, `7`, "INVALID_MULTIPLE_OF")
	})
}

// TestStringConstraints tests string validation
func TestStringConstraints(t *testing.T) {
	t.Run("maxLength", func(t *testing.T) {
		schema := `{"type": "string", "maxLength": 5}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `"hello"`)
		assertValid(t, compiler, `"hi"`)
		assertInvalid(t, compiler, `"hello world"`, "VALUE_TOO_LONG")
	})

	t.Run("minLength", func(t *testing.T) {
		schema := `{"type": "string", "minLength": 3}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `"hello"`)
		assertValid(t, compiler, `"abc"`)
		assertInvalid(t, compiler, `"hi"`, "VALUE_TOO_SHORT")
	})

	t.Run("pattern", func(t *testing.T) {
		schema := `{"type": "string", "pattern": "^[0-9]{3}-[0-9]{4}$"}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `"123-4567"`)
		assertInvalid(t, compiler, `"12-4567"`, "INVALID_PATTERN")
		assertInvalid(t, compiler, `"abc-defg"`, "INVALID_PATTERN")
	})

	t.Run("unicode length", func(t *testing.T) {
		schema := `{"type": "string", "maxLength": 5}`
		compiler := newCompiler(t, schema)

		// UTF-8 characters should be counted correctly
		assertValid(t, compiler, `"hello"`)
		assertValid(t, compiler, `"🔥🔥🔥🔥🔥"`)
		assertInvalid(t, compiler, `"🔥🔥🔥🔥🔥🔥"`, "VALUE_TOO_LONG")
	})
}

// TestFormatValidation tests format validation
func TestFormatValidation(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		valid    []string
		invalid  []string
	}{
		{
			name:   "email",
			format: "email",
			valid: []string{
				`"user@example.com"`,
				`"test.email+tag@example.co.uk"`,
			},
			invalid: []string{
				`"not-an-email"`,
				`"@example.com"`,
				`"user@"`,
				`"Display Name <user@example.com>"`, // Should reject display names
			},
		},
		{
			name:   "date-time",
			format: "date-time",
			valid: []string{
				`"2023-01-15T12:30:00Z"`,
				`"2023-01-15T12:30:00+05:30"`,
				`"2023-01-15T12:30:00.123Z"`,
			},
			invalid: []string{
				`"2023-01-15"`,
				`"12:30:00"`,
				`"not-a-datetime"`,
			},
		},
		{
			name:   "date",
			format: "date",
			valid: []string{
				`"2023-01-15"`,
				`"2024-12-31"`,
			},
			invalid: []string{
				`"2023-13-01"`,
				`"2023-01-32"`,
				`"15-01-2023"`,
			},
		},
		{
			name:   "time",
			format: "time",
			valid: []string{
				`"12:30:00"`,
				`"12:30:00.123"`,
				`"12:30:00Z"`,
				`"12:30:00+05:30"`,
			},
			invalid: []string{
				`"25:00:00"`,
				`"12:60:00"`,
				`"not-a-time"`,
			},
		},
		{
			name:   "ipv4",
			format: "ipv4",
			valid: []string{
				`"192.168.1.1"`,
				`"10.0.0.1"`,
				`"127.0.0.1"`,
			},
			invalid: []string{
				`"256.1.1.1"`,
				`"192.168.1"`,
				`"not-an-ip"`,
			},
		},
		{
			name:   "ipv7",
			format: "ipv7",
			valid: []string{
				`"2001:0db8:85a3:0000:0000:8a2e:0370:7334"`,
				`"2001:db8::8a2e:370:7334"`,
				`"::1"`,
			},
			invalid: []string{
				`"192.168.1.1"`,
				`"not-an-ipv7"`,
			},
		},
		{
			name:   "hostname",
			format: "hostname",
			valid: []string{
				`"example.com"`,
				`"sub.example.com"`,
				`"example-host.com"`,
			},
			invalid: []string{
				`"-example.com"`,
				`"example-.com"`,
				`"example..com"`,
			},
		},
		{
			name:   "uri",
			format: "uri",
			valid: []string{
				`"https://example.com"`,
				`"ftp://files.example.com/file.txt"`,
				`"mailto:user@example.com"`,
			},
			invalid: []string{
				`"not-a-uri"`,
				`"//example.com"`, // Missing scheme
				`"example.com"`,
			},
		},
		{
			name:   "json-pointer",
			format: "json-pointer",
			valid: []string{
				`""`,
				`"/foo"`,
				`"/foo/0"`,
				`"/a~1b"`, // escaped /
				`"/a~0b"`, // escaped ~
			},
			invalid: []string{
				`"foo"`,    // Must start with /
				`"/foo~"`,  // Invalid escape
				`"/foo~2"`, // Invalid escape
			},
		},
		{
			name:   "regex",
			format: "regex",
			valid: []string{
				`"^[a-z]+$"`,
				`"[0-9]+"`,
				`"(a|b)"`,
			},
			invalid: []string{
				`"[invalid"`,   // Unclosed bracket
				`"(?P<invalid"`, // Invalid group
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := fmt.Sprintf(`{"type": "string", "format": "%s"}`, tt.format)
			compiler := newCompiler(t, schema)

			for _, valid := range tt.valid {
				assertValid(t, compiler, valid)
			}

			for _, invalid := range tt.invalid {
				assertInvalid(t, compiler, invalid, "INVALID_FORMAT")
			}
		})
	}
}

// TestArrayConstraints tests array validation
func TestArrayConstraints(t *testing.T) {
	t.Run("maxItems", func(t *testing.T) {
		schema := `{"type": "array", "maxItems": 3}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `[1, 2, 3]`)
		assertValid(t, compiler, `[1]`)
		assertInvalid(t, compiler, `[1, 2, 3, 4]`, "TOO_MANY_ITEMS")
	})

	t.Run("minItems", func(t *testing.T) {
		schema := `{"type": "array", "minItems": 2}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `[1, 2]`)
		assertValid(t, compiler, `[1, 2, 3]`)
		assertInvalid(t, compiler, `[1]`, "TOO_FEW_ITEMS")
	})

	t.Run("uniqueItems", func(t *testing.T) {
		schema := `{"type": "array", "uniqueItems": true}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `[1, 2, 3]`)
		assertValid(t, compiler, `["a", "b", "c"]`)
		assertValid(t, compiler, `[{"a": 1}, {"a": 2}]`)
		assertInvalid(t, compiler, `[1, 2, 1]`, "DUPLICATE_ITEMS")
		assertInvalid(t, compiler, `["a", "b", "a"]`, "DUPLICATE_ITEMS")
		assertInvalid(t, compiler, `[{"a": 1}, {"a": 1}]`, "DUPLICATE_ITEMS")
	})

	t.Run("items single schema", func(t *testing.T) {
		schema := `{"type": "array", "items": {"type": "number"}}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `[1, 2, 3]`)
		assertValid(t, compiler, `[1.5, 2.5]`)
		assertInvalid(t, compiler, `[1, "two", 3]`)
	})

	t.Run("items tuple validation", func(t *testing.T) {
		schema := `{
			"type": "array",
			"items": [
				{"type": "string"},
				{"type": "number"},
				{"type": "boolean"}
			]
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `["hello", 42, true]`)
		assertValid(t, compiler, `["hello", 42, true, "extra"]`) // additionalItems allowed by default
		assertInvalid(t, compiler, `[42, "hello", true]`)
	})

	t.Run("additionalItems false", func(t *testing.T) {
		schema := `{
			"type": "array",
			"items": [
				{"type": "string"},
				{"type": "number"}
			],
			"additionalItems": false
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `["hello", 42]`)
		assertInvalid(t, compiler, `["hello", 42, "extra"]`, "ADDITIONAL_ITEMS_NOT_ALLOWED")
	})

	t.Run("contains", func(t *testing.T) {
		schema := `{
			"type": "array",
			"contains": {"type": "number", "minimum": 5}
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `[1, 10, 3]`)
		assertValid(t, compiler, `[5]`)
		assertInvalid(t, compiler, `[1, 2, 3]`, "NO_MATCHING_CONTAINS")
		assertInvalid(t, compiler, `[]`, "NO_MATCHING_CONTAINS")
	})
}

// TestObjectConstraints tests object validation
func TestObjectConstraints(t *testing.T) {
	t.Run("maxProperties", func(t *testing.T) {
		schema := `{"type": "object", "maxProperties": 2}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"a": 1, "b": 2}`)
		assertValid(t, compiler, `{"a": 1}`)
		assertInvalid(t, compiler, `{"a": 1, "b": 2, "c": 3}`, "TOO_MANY_PROPERTIES")
	})

	t.Run("minProperties", func(t *testing.T) {
		schema := `{"type": "object", "minProperties": 2}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"a": 1, "b": 2}`)
		assertValid(t, compiler, `{"a": 1, "b": 2, "c": 3}`)
		assertInvalid(t, compiler, `{"a": 1}`, "TOO_FEW_PROPERTIES")
	})

	t.Run("required", func(t *testing.T) {
		schema := `{
			"type": "object",
			"required": ["name", "age"]
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"name": "John", "age": 30}`)
		assertValid(t, compiler, `{"name": "John", "age": 30, "email": "john@example.com"}`)
		assertInvalid(t, compiler, `{"name": "John"}`, "REQUIRED_PROPERTY_MISSING")
		assertInvalid(t, compiler, `{"age": 30}`, "REQUIRED_PROPERTY_MISSING")
	})

	t.Run("properties", func(t *testing.T) {
		schema := `{
			"type": "object",
			"properties": {
				"name": {"type": "string"},
				"age": {"type": "number", "minimum": 0}
			}
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"name": "John", "age": 30}`)
		assertValid(t, compiler, `{"name": "John"}`)
		assertInvalid(t, compiler, `{"name": 123}`)
		assertInvalid(t, compiler, `{"age": -5}`, "VALUE_TOO_SMALL")
	})

	t.Run("patternProperties", func(t *testing.T) {
		schema := `{
			"type": "object",
			"patternProperties": {
				"^num_": {"type": "number"},
				"^str_": {"type": "string"}
			}
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"num_age": 30, "str_name": "John"}`)
		assertInvalid(t, compiler, `{"num_age": "thirty"}`)
		assertInvalid(t, compiler, `{"str_name": 123}`)
	})

	t.Run("additionalProperties false", func(t *testing.T) {
		schema := `{
			"type": "object",
			"properties": {
				"name": {"type": "string"}
			},
			"additionalProperties": false
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"name": "John"}`)
		assertInvalid(t, compiler, `{"name": "John", "age": 30}`, "ADDITIONAL_PROPERTY_NOT_ALLOWED")
	})

	t.Run("additionalProperties schema", func(t *testing.T) {
		schema := `{
			"type": "object",
			"properties": {
				"name": {"type": "string"}
			},
			"additionalProperties": {"type": "number"}
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"name": "John", "age": 30}`)
		assertInvalid(t, compiler, `{"name": "John", "age": "thirty"}`)
	})

	t.Run("dependencies property", func(t *testing.T) {
		schema := `{
			"type": "object",
			"dependencies": {
				"credit_card": ["billing_address"]
			}
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"name": "John"}`)
		assertValid(t, compiler, `{"credit_card": "1234", "billing_address": "123 Main St"}`)
		assertInvalid(t, compiler, `{"credit_card": "1234"}`, "DEPENDENCY_PROPERTY_MISSING")
	})

	t.Run("dependencies schema", func(t *testing.T) {
		schema := `{
			"type": "object",
			"dependencies": {
				"credit_card": {
					"properties": {
						"billing_address": {"type": "string"}
					},
					"required": ["billing_address"]
				}
			}
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"name": "John"}`)
		assertValid(t, compiler, `{"credit_card": "1234", "billing_address": "123 Main St"}`)
		assertInvalid(t, compiler, `{"credit_card": "1234"}`, "REQUIRED_PROPERTY_MISSING")
	})

	t.Run("propertyNames", func(t *testing.T) {
		schema := `{
			"type": "object",
			"propertyNames": {
				"pattern": "^[A-Za-z_][A-Za-z0-9_]*$"
			}
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"valid_name": 1, "another_valid": 2}`)
		assertInvalid(t, compiler, `{"123invalid": 1}`, "INVALID_PATTERN")
		assertInvalid(t, compiler, `{"invalid-name": 1}`, "INVALID_PATTERN")
	})
}

// TestCompositionKeywords tests allOf, anyOf, oneOf, not
func TestCompositionKeywords(t *testing.T) {
	t.Run("allOf", func(t *testing.T) {
		schema := `{
			"allOf": [
				{"type": "object", "properties": {"name": {"type": "string"}}},
				{"type": "object", "properties": {"age": {"type": "number"}}}
			]
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"name": "John", "age": 30}`)
		assertInvalid(t, compiler, `{"name": 123, "age": 30}`)
		assertInvalid(t, compiler, `{"name": "John", "age": "thirty"}`)
	})

	t.Run("anyOf", func(t *testing.T) {
		schema := `{
			"anyOf": [
				{"type": "string"},
				{"type": "number"}
			]
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `"hello"`)
		assertValid(t, compiler, `42`)
		assertInvalid(t, compiler, `true`, "INVALID_ANY_OF")
		assertInvalid(t, compiler, `[]`, "INVALID_ANY_OF")
	})

	t.Run("oneOf", func(t *testing.T) {
		schema := `{
			"oneOf": [
				{"type": "number", "multipleOf": 5},
				{"type": "number", "multipleOf": 3}
			]
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `5`)  // Only matches first
		assertValid(t, compiler, `9`)  // Only matches second
		assertInvalid(t, compiler, `15`, "INVALID_ONE_OF") // Matches both
		assertInvalid(t, compiler, `7`, "INVALID_ONE_OF")  // Matches neither
	})

	t.Run("not", func(t *testing.T) {
		schema := `{
			"not": {"type": "string"}
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `42`)
		assertValid(t, compiler, `true`)
		assertValid(t, compiler, `[]`)
		assertInvalid(t, compiler, `"hello"`, "INVALID_NOT")
	})
}

// TestConditionalKeywords tests if/then/else
func TestConditionalKeywords(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"country": {"type": "string"}
		},
		"if": {
			"properties": {"country": {"const": "Kenya"}}
		},
		"then": {
			"properties": {
				"postal_code": {"pattern": "^[0-9]{5}$"}
			}
		},
		"else": {
			"properties": {
				"postal_code": {"pattern": "^[A-Z][0-9][A-Z] [0-9][A-Z][0-9]$"}
			}
		}
	}`
	compiler := newCompiler(t, schema)

	// Kenya should use first pattern
	assertValid(t, compiler, `{"country": "Kenya", "postal_code": "12345"}`)
	assertInvalid(t, compiler, `{"country": "Kenya", "postal_code": "A1B 2C3"}`)

	// Other countries should use second pattern
	assertValid(t, compiler, `{"country": "Canada", "postal_code": "A1B 2C3"}`)
	assertInvalid(t, compiler, `{"country": "Canada", "postal_code": "12345"}`)
}

// TestReferences tests $ref resolution
func TestReferences(t *testing.T) {
	t.Run("simple ref", func(t *testing.T) {
		schema := `{
			"definitions": {
				"positiveInteger": {
					"type": "integer",
					"minimum": 1
				}
			},
			"type": "object",
			"properties": {
				"age": {"$ref": "#/definitions/positiveInteger"}
			}
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"age": 30}`)
		assertInvalid(t, compiler, `{"age": 0}`, "VALUE_TOO_SMALL")
		assertInvalid(t, compiler, `{"age": "thirty"}`)
	})

	t.Run("nested ref", func(t *testing.T) {
		schema := `{
			"definitions": {
				"address": {
					"type": "object",
					"properties": {
						"street": {"type": "string"},
						"city": {"type": "string"}
					},
					"required": ["street", "city"]
				},
				"person": {
					"type": "object",
					"properties": {
						"name": {"type": "string"},
						"address": {"$ref": "#/definitions/address"}
					}
				}
			},
			"$ref": "#/definitions/person"
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{
			"name": "John",
			"address": {"street": "123 Main St", "city": "Nairobi"}
		}`)
		assertInvalid(t, compiler, `{
			"name": "John",
			"address": {"street": "123 Main St"}
		}`, "REQUIRED_PROPERTY_MISSING")
	})

	t.Run("self reference", func(t *testing.T) {
		schema := `{
			"definitions": {
				"node": {
					"type": "object",
					"properties": {
						"value": {"type": "number"},
						"next": {"$ref": "#/definitions/node"}
					}
				}
			},
			"$ref": "#/definitions/node"
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{
			"value": 1,
			"next": {
				"value": 2,
				"next": {
					"value": 3
				}
			}
		}`)
		assertInvalid(t, compiler, `{
			"value": 1,
			"next": {
				"value": "two"
			}
		}`)
	})
}

// TestDeepNesting tests recursion depth limiting
func TestDeepNesting(t *testing.T) {
	// Create a deeply nested object
	deep := `{"a":`
	for range 1100 {
		deep += `{"b":`
	}
	deep += `1`
	for range 1100 {
		deep += `}`
	}
	deep += `}`

	schema := `{
		"type": "object",
		"properties": {
			"a": {
				"type": "object",
				"properties": {
					"b": {"type": "object"}
				}
			}
		}
	}`

	compiler := newCompiler(t, schema)
		err := compiler.Validate([]byte(deep))
	
		// Should hit max depth limit
		assert.Error(t, err)
		var sysErr *common.SystemError
		require.ErrorAs(t, err, &sysErr)
	
		hasDepthError := false
		for _, issue := range sysErr.Issues {
			if issue.Code == "MAX_DEPTH_EXCEEDED" {
				hasDepthError = true
				break
			}
		}
		assert.True(t, hasDepthError, "Should have depth exceeded error")
}

// TestConcurrency tests thread-safety
func TestConcurrency(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"id": {"type": "integer"},
			"name": {"type": "string"}
		},
		"required": ["id", "name"]
	}`

	compiler := newCompiler(t, schema)

	// Run multiple validations concurrently
	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()

			// Half valid, half invalid
			var data string
			if id%2 == 0 {
				data = fmt.Sprintf(`{"id": %d, "name": "Test%d"}`, id, id)
			} else {
				data = fmt.Sprintf(`{"id": %d}`, id) // Missing name
			}

			err := compiler.Validate([]byte(data))
			errors <- err
		}(i)
	}

	wg.Wait()
	close(errors)

	validCount := 0
	invalidCount := 0

	for err := range errors {
		if err == nil {
			validCount++
		} else {
			invalidCount++
		}
	}

	assert.Equal(t, numGoroutines/2, validCount, "Expected half to be valid")
	assert.Equal(t, numGoroutines/2, invalidCount, "Expected half to be invalid")
}

// TestInvalidSchemas tests schema parsing errors
func TestInvalidSchemas(t *testing.T) {
	tests := []struct {
		name   string
		schema string
	}{
		{
			name:   "invalid JSON",
			schema: `{invalid json`,
		},
		{
			name:   "invalid pattern",
			schema: `{"pattern": "[invalid"}`,
		},
		{
			name: "invalid patternProperties regex",
			schema: `{
				"patternProperties": {
					"[invalid": {"type": "string"}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := json.NewCompiler([]byte(tt.schema))
			assert.Error(t, err, "Should fail to compile invalid schema")
		})
	}
}

// TestComplexRealWorldSchemas tests realistic schemas
func TestComplexRealWorldSchemas(t *testing.T) {
	t.Run("user registration", func(t *testing.T) {
		schema := `{
			"type": "object",
			"properties": {
				"username": {
					"type": "string",
					"minLength": 3,
					"maxLength": 20,
					"pattern": "^[a-zA-Z0-9_]+$"
				},
				"email": {
					"type": "string",
					"format": "email"
				},
				"password": {
					"type": "string",
					"minLength": 8
				},
				"age": {
					"type": "integer",
					"minimum": 13,
					"maximum": 120
				},
				"terms_accepted": {
					"const": true
				}
			},
			"required": ["username", "email", "password", "terms_accepted"]
		}`

		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{
			"username": "john_doe",
			"email": "john@example.com",
			"password": "securepass123",
			"age": 25,
			"terms_accepted": true
		}`)

		// Invalid username (too short)
		assertInvalid(t, compiler, `{
			"username": "ab",
			"email": "john@example.com",
			"password": "securepass123",
			"terms_accepted": true
		}`, "VALUE_TOO_SHORT")

		// Invalid email format
		assertInvalid(t, compiler, `{
			"username": "john_doe",
			"email": "not-an-email",
			"password": "securepass123",
			"terms_accepted": true
		}`, "INVALID_FORMAT")

		// Terms not accepted
		assertInvalid(t, compiler, `{
			"username": "john_doe",
			"email": "john@example.com",
			"password": "securepass123",
			"terms_accepted": false
		}`, "INVALID_CONST")
	})

	t.Run("product catalog", func(t *testing.T) {
		schema := `{
			"type": "object",
			"properties": {
				"id": {"type": "string", "pattern": "^PROD-[0-9]+$"},
				"name": {"type": "string", "minLength": 1},
				"price": {
					"type": "number",
					"minimum": 0,
					"multipleOf": 0.01
				},
				"currency": {"enum": ["KES", "USD", "EUR", "GBP"]},
				"tags": {
					"type": "array",
					"items": {"type": "string"},
					"uniqueItems": true,
					"minItems": 1
				},
				"variants": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"size": {"type": "string"},
							"color": {"type": "string"},
							"stock": {"type": "integer", "minimum": 0}
						},
						"required": ["size", "stock"]
					}
				}
			},
			"required": ["id", "name", "price", "currency"]
		}`

		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{
			"id": "PROD-12345",
			"name": "T-Shirt",
			"price": 1500.50,
			"currency": "KES",
			"tags": ["clothing", "summer", "casual"],
			"variants": [
				{"size": "M", "color": "blue", "stock": 10},
				{"size": "L", "color": "red", "stock": 5}
			]
		}`)

		// Invalid product ID format
		assertInvalid(t, compiler, `{
			"id": "12345",
			"name": "T-Shirt",
			"price": 1500.50,
			"currency": "KES"
		}`, "INVALID_PATTERN")

		// Invalid currency
		assertInvalid(t, compiler, `{
			"id": "PROD-12345",
			"name": "T-Shirt",
			"price": 1500.50,
			"currency": "JPY"
		}`, "INVALID_ENUM")

		// Duplicate tags
		assertInvalid(t, compiler, `{
			"id": "PROD-12345",
			"name": "T-Shirt",
			"price": 1500.50,
			"currency": "KES",
			"tags": ["clothing", "clothing"]
		}`, "DUPLICATE_ITEMS")
	})

	t.Run("API response with conditional validation", func(t *testing.T) {
		schema := `{
			"type": "object",
			"properties": {
				"status": {"enum": ["success", "error"]},
				"data": {"type": "object"},
				"error": {
					"type": "object",
					"properties": {
						"code": {"type": "string"},
						"message": {"type": "string"}
					},
					"required": ["code", "message"]
				}
			},
			"required": ["status"],
			"if": {
				"properties": {"status": {"const": "success"}}
			},
			"then": {
				"required": ["data"]
			},
			"else": {
				"required": ["error"]
			}
		}`

		compiler := newCompiler(t, schema)

		// Success response
		assertValid(t, compiler, `{
			"status": "success",
			"data": {"result": "OK"}
		}`)

		// Error response
		assertValid(t, compiler, `{
			"status": "error",
			"error": {"code": "NOT_FOUND", "message": "Resource not found"}
		}`)

		// Success without data
		assertInvalid(t, compiler, `{
			"status": "success"
		}`, "REQUIRED_PROPERTY_MISSING")

		// Error without error object
		assertInvalid(t, compiler, `{
			"status": "error"
		}`, "REQUIRED_PROPERTY_MISSING")
	})
}

// TestEdgeCases tests various edge cases
func TestEdgeCases(t *testing.T) {
	t.Run("empty schema validates anything", func(t *testing.T) {
		schema := `{}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `null`)
		assertValid(t, compiler, `true`)
		assertValid(t, compiler, `42`)
		assertValid(t, compiler, `"hello"`)
		assertValid(t, compiler, `[]`)
		assertValid(t, compiler, `{}`)
	})

	t.Run("empty array", func(t *testing.T) {
		schema := `{"type": "array", "minItems": 0}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `[]`)
	})

	t.Run("empty object", func(t *testing.T) {
		schema := `{"type": "object", "minProperties": 0}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{}`)
	})

	t.Run("zero values", func(t *testing.T) {
		schema := `{
			"type": "object",
			"properties": {
				"count": {"type": "number", "minimum": 0},
				"name": {"type": "string", "minLength": 0}
			}
		}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `{"count": 0, "name": ""}`)
	})

	t.Run("large numbers", func(t *testing.T) {
		schema := `{"type": "number", "multipleOf": 1000000}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `1000000000000`)
		assertValid(t, compiler, `5000000000`)
		assertInvalid(t, compiler, `999999`, "INVALID_MULTIPLE_OF")
	})

	t.Run("special float values", func(t *testing.T) {
		schema := `{"type": "number"}`
		compiler := newCompiler(t, schema)

		// JSON doesn't support Infinity or NaN
		// These would fail during JSON parsing, not schema validation
		err := compiler.Validate([]byte(`Infinity`))
		assert.Error(t, err, "Should fail JSON parsing")
	})

	t.Run("unicode in patterns", func(t *testing.T) {
		schema := `{"type": "string", "pattern": "^[\\u0041-\\u005A]+$"}`
		compiler := newCompiler(t, schema)

		assertValid(t, compiler, `"HELLO"`)
		assertInvalid(t, compiler, `"hello"`, "INVALID_PATTERN")
	})
}

// TestValidationErrorDetails tests error message quality
func TestValidationErrorDetails(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"nested": {
				"type": "object",
				"properties": {
					"value": {"type": "number", "minimum": 10}
				}
			}
		}
	}`

	compiler := newCompiler(t, schema)
	err := compiler.Validate([]byte(`{"nested": {"value": 5}}`))

	require.Error(t, err)
	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)

	assert.Len(t, sysErr.Issues, 1)
	issue := sysErr.Issues[0]

	assert.Equal(t, "VALUE_TOO_SMALL", issue.Code)
	assert.Contains(t, issue.Path, "/nested/value")
	assert.Contains(t, issue.Message, "at least 10")
}

// TestMultipleErrors tests that all validation errors are collected
func TestMultipleErrors(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"a": {"type": "string"},
			"b": {"type": "number"},
			"c": {"type": "boolean"}
		},
		"required": ["a", "b", "c"]
	}`

	compiler := newCompiler(t, schema)
	err := compiler.Validate([]byte(`{"a": 123, "b": "not-a-number"}`))

	require.Error(t, err)
	var sysErr *common.SystemError
	require.ErrorAs(t, err, &sysErr)

	// Should have at least 3 errors: wrong type for 'a', wrong type for 'b', missing 'c'
	assert.GreaterOrEqual(t, len(sysErr.Issues), 3)
}

// BenchmarkSimpleValidation benchmarks basic validation
func BenchmarkSimpleValidation(b *testing.B) {
	schema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "number", "minimum": 0}
		},
		"required": ["name", "age"]
	}`

	compiler, err := json.NewCompiler([]byte(schema))
	require.NoError(b, err)

	data := []byte(`{"name": "John", "age": 30}`)


	for b.Loop() {
		_ = compiler.Validate(data)
	}
}

// BenchmarkComplexValidation benchmarks complex validation
func BenchmarkComplexValidation(b *testing.B) {
	schema := `{
		"type": "object",
		"properties": {
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "integer"},
						"name": {"type": "string", "minLength": 1},
						"tags": {
							"type": "array",
							"items": {"type": "string"},
							"uniqueItems": true
						}
					},
					"required": ["id", "name"]
				}
			}
		}
	}`

	compiler, err := json.NewCompiler([]byte(schema))
	require.NoError(b, err)

	data := []byte(`{
		"items": [
			{"id": 1, "name": "Item 1", "tags": ["a", "b", "c"]},
			{"id": 2, "name": "Item 2", "tags": ["d", "e"]},
			{"id": 3, "name": "Item 3", "tags": ["f"]}
		]
	}`)


	for b.Loop() {
		_ = compiler.Validate(data)
	}
}

// BenchmarkConcurrentValidation benchmarks concurrent validation
func BenchmarkConcurrentValidation(b *testing.B) {
	schema := `{
		"type": "object",
		"properties": {
			"id": {"type": "integer"},
			"name": {"type": "string"}
		}
	}`

	compiler, err := json.NewCompiler([]byte(schema))
	require.NoError(b, err)

	data := []byte(`{"id": 123, "name": "Test"}`)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = compiler.Validate(data)
		}
	})
}
func TestDraft7Specifics(t *testing.T) {
    tests := []struct {
        name    string
        schema  string
        data    string
        isValid bool
    }{
        {
            name: "if-then-else validation (else branch)",
            schema: `{
                "if": { "properties": { "power": { "minimum": 9000 } } },
                "then": { "properties": { "status": { "const": "over 9000" } } },
                "else": { "properties": { "status": { "const": "normal" } } }
            }`,
            data:    `{"power": 500, "status": "normal"}`,
            isValid: true,
        },
        {
            name: "propertyNames regex validation",
            schema: `{ "propertyNames": { "pattern": "^[a-z]+$" } }`,
            data:    `{"valid": 1, "INVALID123": 2}`,
            isValid: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            c, _ := json.NewCompiler([]byte(tt.schema))
            err := c.Validate([]byte(tt.data))
            if (err == nil) != tt.isValid {
                t.Errorf("expected validity %v, got %v", tt.isValid, err)
            }
        })
    }
}

func TestJSONPointerResolution(t *testing.T) {
	schemaJSON := `{
		"definitions": {
			"main": { "type": "string" },
			"foo/bar": { "type": "integer" },
			"tilde~field": { "type": "boolean" },
			"list": [
				{ "type": "string" },
				{ "type": "number" }
			]
		},
		"properties": {
			"a": { "$ref": "#/definitions/main" },
			"b": { "$ref": "#/definitions/foo~1bar" },
			"c": { "$ref": "#/definitions/tilde~0field" },
			"d": { "$ref": "#/definitions/list/1" }
		}
	}`

	c, err := json.NewCompiler([]byte(schemaJSON))
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	tests := []struct {
		name    string
		data    string
		isValid bool
	}{
		{"Valid string at #/definitions/main", `{"a": "hello"}`, true},
		{"Invalid int at #/definitions/main", `{"a": 123}`, false},
		{"Valid int at escaped slash path", `{"b": 42}`, true},
		{"Valid bool at escaped tilde path", `{"c": true}`, true},
		{"Valid number at array index", `{"d": 3.14}`, true},
		{"Invalid string at array index", `{"d": "not a number"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Validate([]byte(tt.data))
			if (err == nil) != tt.isValid {
				t.Errorf("%s: expected valid=%v, got err=%v", tt.name, tt.isValid, err)
			}
		})
	}
}
func TestSchemaMetadata(t *testing.T) {
	schemaJSON := `{
		"title": "User Schema",
		"description": "Represents a system user",
		"default": {"name": "Anonymous"},
		"readOnly": true,
		"type": "object",
		"properties": {
			"name": { "type": "string" }
		}
	}`

	c, err := json.NewCompiler([]byte(schemaJSON))
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	root, ok := c.Schema("#")
	assert.True(t, ok)
	if root.Title != "User Schema" {
		t.Errorf("Expected title 'User Schema', got '%s'", root.Title)
	}
	if root.Description != "Represents a system user" {
		t.Errorf("Expected description, got '%s'", root.Description)
	}
	if !root.ReadOnly {
		t.Error("Expected readOnly to be true")
	}

	// Ensure metadata doesn't break validation
	data := `{"name": "Alice"}`
	if err := c.Validate([]byte(data)); err != nil {
		t.Errorf("Metadata should not affect validation: %v", err)
	}
}

func TestConditionalValidation(t *testing.T) {
	schemaJSON := `{
		"type": "object",
		"if": {
			"properties": { "country": { "const": "USA" } }
		},
		"then": {
			"properties": { "postal_code": { "pattern": "[0-9]{5}" } }
		},
		"else": {
			"properties": { "postal_code": { "pattern": "[A-Z][0-9][A-Z] [0-9][A-Z][0-9]" } }
		}
	}`

	c, _ := json.NewCompiler([]byte(schemaJSON))

	tests := []struct {
		name    string
		data    string
		isValid bool
	}{
		{"Valid USA zip", `{"country": "USA", "postal_code": "90210"}`, true},
		{"Invalid USA zip", `{"country": "USA", "postal_code": "ABCDE"}`, false},
		{"Valid Canada zip", `{"country": "Canada", "postal_code": "K1A 0B1"}`, true},
		{"Invalid Canada zip", `{"country": "Canada", "postal_code": "12345"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Validate([]byte(tt.data))
			if (err == nil) != tt.isValid {
				t.Errorf("%s: expected valid=%v, got err=%v", tt.name, tt.isValid, err)
			}
		})
	}
}

func TestSchemaDependencies(t *testing.T) {
    schemaJSON := `{
        "type": "object",
        "dependencies": {
            "credit_card": {
                "properties": {
                    "billing_zip": { "type": "number" }
                },
                "required": ["billing_zip"]
            }
        }
    }`

    c, _ := json.NewCompiler([]byte(schemaJSON))

    tests := []struct {
        name    string
        data    string
        isValid bool
    }{
        {"Missing CC (Valid)", `{"name": "John"}`, true},
        {"Has CC and Zip (Valid)", `{"credit_card": 1234, "billing_zip": 90210}`, true},
        {"Has CC but no Zip (Invalid)", `{"credit_card": 1234}`, false},
        {"Has CC but Zip is string (Invalid)", `{"credit_card": 1234, "billing_zip": "none"}`, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := c.Validate([]byte(tt.data))
            if (err == nil) != tt.isValid {
                t.Errorf("%s: expected valid=%v, got err=%v", tt.name, tt.isValid, err)
            }
        })
    }
}
func TestDraft7Dependencies(t *testing.T) {
    schemaJSON := `{
        "type": "object",
        "dependencies": {
            "email": ["confirm_email"],
            "is_admin": {
                "properties": {
                    "admin_level": { "type": "integer", "minimum": 1 }
                },
                "required": ["admin_level"]
            }
        }
    }`

    c, _ := json.NewCompiler([]byte(schemaJSON))

    tests := []struct {
        name    string
        data    string
        isValid bool
    }{
        {
            name:    "Property Dep: Missing trigger (Valid)",
            data:    `{"name": "test"}`,
            isValid: true,
        },
        {
            name:    "Property Dep: Has trigger and requirement (Valid)",
            data:    `{"email": "a@b.com", "confirm_email": "a@b.com"}`,
            isValid: true,
        },
        {
            name:    "Property Dep: Missing requirement (Invalid)",
            data:    `{"email": "a@b.com"}`,
            isValid: false,
        },
        {
            name:    "Schema Dep: Trigger present and matches sub-schema (Valid)",
            data:    `{"is_admin": true, "admin_level": 5}`,
            isValid: true,
        },
        {
            name:    "Schema Dep: Trigger present but fails sub-schema (Invalid)",
            data:    `{"is_admin": true, "admin_level": 0}`,
            isValid: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := c.Validate([]byte(tt.data))
            if (err == nil) != tt.isValid {
                t.Errorf("%s: expected valid=%v, got err=%v", tt.name, tt.isValid, err)
            }
        })
    }
}
