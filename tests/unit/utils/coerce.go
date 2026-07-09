package utils_test

import (
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/utils"
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
			result, ok := utils.CoerceToString(tt.input)
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
			result, ok := utils.CoerceToInt(tt.input)
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
			result, ok := utils.CoerceToFloat64(tt.input)
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
			result, ok := utils.CoerceToBool(tt.input)
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
			result, ok := utils.CoerceToTime(tt.input)
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
