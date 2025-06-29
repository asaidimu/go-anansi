package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringPtr(t *testing.T) {
	s := "test_string"
	ptr := StringPtr(s)
	assert.NotNil(t, ptr)
	assert.Equal(t, s, *ptr)
}

func TestInt64Ptr(t *testing.T) {
	i := int64(12345)
	ptr := Int64Ptr(i)
	assert.NotNil(t, ptr)
	assert.Equal(t, i, *ptr)
}

func TestBoolPtr(t *testing.T) {
	b := true
	ptr := BoolPtr(b)
	assert.NotNil(t, ptr)
	assert.Equal(t, b, *ptr)
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
		success  bool
	}{
		{"int", 10, 10.0, true},
		{"int8", int8(20), 20.0, true},
		{"int16", int16(30), 30.0, true},
		{"int32", int32(40), 40.0, true},
		{"int64", int64(50), 50.0, true},
		{"float32", float32(60.5), 60.5, true},
		{"float64", 70.5, 70.5, true},
		{"string_valid_int", "100", 100.0, true},
		{"string_valid_float", "123.45", 123.45, true},
		{"string_invalid", "abc", 0.0, false},
		{"nil", nil, 0.0, false},
		{"unsupported_type", struct{}{}, 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := ToFloat64(tt.input)
			assert.Equal(t, tt.success, ok)
			if tt.success {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
