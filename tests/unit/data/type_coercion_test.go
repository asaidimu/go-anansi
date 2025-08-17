package data_test

import (
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/require"
)

func TestCoerceToString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
		ok       bool
	}{
		{name: "string", input: "hello", expected: "hello", ok: true},
		{name: "int", input: 123, expected: "123", ok: true},
		{name: "float", input: 123.45, expected: "123.45", ok: true},
		{name: "bool_true", input: true, expected: "true", ok: true},
		{name: "bool_false", input: false, expected: "false", ok: true},
		{name: "nil", input: nil, expected: "", ok: true},
		{name: "time", input: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), expected: "2023-01-01 00:00:00 +0000 UTC", ok: true},
		{name: "struct", input: struct{ A int }{A: 1}, expected: "{1}", ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := data.CoerceToString(tt.input)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCoerceToInt(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
		ok       bool
	}{
		{name: "int", input: 123, expected: 123, ok: true},
		{name: "float", input: 123.45, expected: 123, ok: true},
		{name: "string_int", input: "123", expected: 123, ok: true},
		{name: "string_float", input: "123.45", expected: 123, ok: true},
		{name: "bool_true", input: true, expected: 1, ok: true},
		{name: "bool_false", input: false, expected: 0, ok: true},
		{name: "invalid_string", input: "abc", expected: 0, ok: false},
		{name: "nil", input: nil, expected: 0, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := data.CoerceToInt(tt.input)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCoerceToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected float64
		ok       bool
	}{
		{name: "int", input: 123, expected: 123.0, ok: true},
		{name: "float", input: 123.45, expected: 123.45, ok: true},
		{name: "string_int", input: "123", expected: 123.0, ok: true},
		{name: "string_float", input: "123.45", expected: 123.45, ok: true},
		{name: "bool_true", input: true, expected: 1.0, ok: true},
		{name: "bool_false", input: false, expected: 0.0, ok: true},
		{name: "invalid_string", input: "abc", expected: 0.0, ok: false},
		{name: "nil", input: nil, expected: 0.0, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := data.CoerceToFloat64(tt.input)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCoerceToBool(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected bool
		ok       bool
	}{
		{name: "bool_true", input: true, expected: true, ok: true},
		{name: "bool_false", input: false, expected: false, ok: true},
		{name: "string_true_lower", input: "true", expected: true, ok: true},
		{name: "string_true_upper", input: "TRUE", expected: true, ok: true},
		{name: "string_1", input: "1", expected: true, ok: true},
		{name: "string_yes", input: "yes", expected: true, ok: true},
		{name: "string_on", input: "on", expected: true, ok: true},
		{name: "string_false_lower", input: "false", expected: false, ok: true},
		{name: "string_false_upper", input: "FALSE", expected: false, ok: true},
		{name: "string_0", input: "0", expected: false, ok: true},
		{name: "string_no", input: "no", expected: false, ok: true},
		{name: "string_off", input: "off", expected: false, ok: true},
		{name: "string_empty", input: "", expected: false, ok: true},
		{name: "invalid_string", input: "abc", expected: false, ok: false},
		{name: "int_1", input: 1, expected: true, ok: true},
		{name: "int_0", input: 0, expected: false, ok: true},
		{name: "float_1", input: 1.0, expected: true, ok: true},
		{name: "float_0", input: 0.0, expected: false, ok: true},
		{name: "nil", input: nil, expected: false, ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := data.CoerceToBool(tt.input)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCoerceToTime(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		input    any
		expected time.Time
		ok       bool
	}{
		{name: "time", input: now, expected: now, ok: true},
		{name: "string_rfc3339", input: "2023-01-01T12:30:00Z", expected: time.Date(2023, 1, 1, 12, 30, 0, 0, time.UTC), ok: true},
		{name: "string_rfc3339nano", input: "2023-01-01T12:30:00.123456789Z", expected: time.Date(2023, 1, 1, 12, 30, 0, 123456789, time.UTC), ok: true},
		{name: "string_date_only", input: "2023-01-01", expected: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), ok: true},
		{name: "int64_unix", input: now.Unix(), expected: time.Unix(now.Unix(), 0), ok: true},
		{name: "float64_unix", input: float64(now.Unix()), expected: time.Unix(now.Unix(), 0), ok: true},
		{name: "invalid_string", input: "invalid-time", expected: time.Time{}, ok: false},
		{name: "nil", input: nil, expected: time.Time{}, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := data.CoerceToTime(tt.input)
			require.Equal(t, tt.ok, ok)
			// Compare times with a tolerance for float/string conversions
			if tt.ok {
				require.WithinDuration(t, tt.expected, result, time.Second)
			} else {
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestDocument_TypeSafeAccessors(t *testing.T) {
	doc, err := data.NewDocument(map[string]any{
		"str":    "hello",
		"int":    123,
		"float":  123.45,
		"bool":   true,
		"time":   "2023-01-01T10:00:00Z",
		"nested": map[string]any{"key": "value"},
		"arr":    []any{"a", "b"},
		"doc_arr": []data.Document{
			data.MustNewDocument(map[string]any{"id": "1"}),
			data.MustNewDocument(map[string]any{"id": "2"}),
		},
		"str_int": "123",
		"uncoercible_int": "hello",
		"str_float": "123.45",
		"uncoercible_float": "hello",
		"str_bool_true": "true",
		"uncoercible_bool": "abc",
		"int_time": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
		"float_time": float64(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()),
		"uncoercible_time": "abc",
	})
	require.NoError(t, err)

	// GetString - successful string retrieval
	str, err := doc.GetString("str")
	require.NoError(t, err)
	require.Equal(t, "hello", str)

	// GetString - successful coercion from int
	str, err = doc.GetString("int")
	require.NoError(t, err)
	require.Equal(t, "123", str) // int 123 should coerce to string "123"

	// GetString - successful coercion from float
	str, err = doc.GetString("float")
	require.NoError(t, err)
	require.Equal(t, "123.45", str) // float 123.45 should coerce to string "123.45"

	// GetString - successful coercion from bool
	str, err = doc.GetString("bool")
	require.NoError(t, err)
	require.Equal(t, "true", str) // bool true should coerce to string "true"

	// GetString - key not found
	_, err = doc.GetString("non_existent")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrKeyNotFound)

	// GetInt - successful int retrieval
	intVal, err := doc.GetInt("int")
	require.NoError(t, err)
	require.Equal(t, 123, intVal)

	// GetInt - successful coercion from string ("123")
	intVal, err = doc.GetInt("str_int")
	require.NoError(t, err)
	require.Equal(t, 123, intVal)

	// GetInt - successful coercion from float
	intVal, err = doc.GetInt("float")
	require.NoError(t, err)
	require.Equal(t, 123, intVal) // float 123.45 should coerce to int 123

	// GetInt - successful coercion from bool
	intVal, err = doc.GetInt("bool")
	require.NoError(t, err)
	require.Equal(t, 1, intVal) // bool true should coerce to int 1

	// GetInt - key not found
	_, err = doc.GetInt("non_existent")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrKeyNotFound)

	// GetInt - cannot coerce (e.g., "hello")
	_, err = doc.GetInt("uncoercible_int")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrTypeMismatch)

	// GetFloat64 - successful float retrieval
	floatVal, err := doc.GetFloat64("float")
	require.NoError(t, err)
	require.Equal(t, 123.45, floatVal)

	// GetFloat64 - successful coercion from int
	floatVal, err = doc.GetFloat64("int")
	require.NoError(t, err)
	require.Equal(t, 123.0, floatVal)

	// GetFloat64 - successful coercion from string ("123.45")
	floatVal, err = doc.GetFloat64("str_float")
	require.NoError(t, err)
	require.Equal(t, 123.45, floatVal)

	// GetFloat64 - successful coercion from bool
	floatVal, err = doc.GetFloat64("bool")
	require.NoError(t, err)
	require.Equal(t, 1.0, floatVal) // bool true should coerce to float 1.0

	// GetFloat64 - key not found
	_, err = doc.GetFloat64("non_existent")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrKeyNotFound)

	// GetFloat64 - cannot coerce (e.g., "hello")
	_, err = doc.GetFloat64("uncoercible_float")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrTypeMismatch)

	// GetBool - successful bool retrieval
	boolVal, err := doc.GetBool("bool")
	require.NoError(t, err)
	require.True(t, boolVal)

	// GetBool - successful coercion from string ("true")
	boolVal, err = doc.GetBool("str_bool_true")
	require.NoError(t, err)
	require.True(t, boolVal)

	// GetBool - successful coercion from int (1)
	boolVal, err = doc.GetBool("int")
	require.NoError(t, err)
	require.True(t, boolVal) // int 123 should coerce to true

	// GetBool - key not found
	_, err = doc.GetBool("non_existent")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrKeyNotFound)

	// GetBool - cannot coerce (e.g., "abc")
	_, err = doc.GetBool("uncoercible_bool")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrTypeMismatch)

	// GetTime - successful time retrieval
	timeVal, err := doc.GetTime("time")
	require.NoError(t, err)
	require.Equal(t, time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC), timeVal)

	// GetTime - successful coercion from int (unix timestamp)
	timeVal, err = doc.GetTime("int_time")
	require.NoError(t, err)
	require.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), timeVal)

	// GetTime - successful coercion from float (unix timestamp)
	timeVal, err = doc.GetTime("float_time")
	require.NoError(t, err)
	require.Equal(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), timeVal)

	// GetTime - key not found
	_, err = doc.GetTime("non_existent")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrKeyNotFound)

	// GetTime - cannot coerce (e.g., "abc")
	_, err = doc.GetTime("uncoercible_time")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrTypeMismatch)

	// GetDocument - successful document retrieval
	nestedDoc, err := doc.GetDocument("nested")
	require.NoError(t, err)
	require.Equal(t, data.Document{"key": "value"}, nestedDoc)

	// GetDocument - key not found
	_, err = doc.GetDocument("non_existent")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrKeyNotFound)

	// GetDocument - cannot coerce (e.g., a string)
	_, err = doc.GetDocument("str")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrTypeMismatch)

	// GetDocumentArray - successful document array retrieval
	docArr, err := doc.GetDocumentArray("doc_arr")
	require.NoError(t, err)
	require.Len(t, docArr, 2)
	require.Equal(t, data.Document{"id": "1"}, docArr[0])

	// GetDocumentArray - key not found
	_, err = doc.GetDocumentArray("non_existent")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrKeyNotFound)

	// GetDocumentArray - cannot coerce (e.g., a string)
	_, err = doc.GetDocumentArray("str")
	require.Error(t, err)
	require.ErrorIs(t, err, data.ErrTypeMismatch)
}
