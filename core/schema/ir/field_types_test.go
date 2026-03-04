package ir_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestFieldType_String(t *testing.T) {
	tests := []struct {
		name     string
		ft       ir.FieldType
		expected string
	}{
		{"Unknown", ir.FieldTypeUnknown, "unknown"},
		{"String", ir.FieldTypeString, "string"},
		{"Number", ir.FieldTypeNumber, "number"},
		{"Integer", ir.FieldTypeInteger, "integer"},
		{"Decimal", ir.FieldTypeDecimal, "decimal"},
		{"Boolean", ir.FieldTypeBoolean, "boolean"},
		{"Array", ir.FieldTypeArray, "array"},
		{"Set", ir.FieldTypeSet, "set"},
		{"Enum", ir.FieldTypeEnum, "enum"},
		{"Object", ir.FieldTypeObject, "object"},
		{"Record", ir.FieldTypeRecord, "record"},
		{"Union", ir.FieldTypeUnion, "union"},
		{"Composite", ir.FieldTypeComposite, "composite"},
		{"Geometry", ir.FieldTypeGeometry, "geometry"},
		{"Invalid", ir.FieldType(99), ""}, // Test an unknown value
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ft.String(); got != tt.expected {
				t.Errorf("FieldType.String() for %v = %s, want %s", tt.ft, got, tt.expected)
			}
		})
	}
}

func TestFieldType_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		ft       ir.FieldType
		expected string
		wantErr  bool
	}{
		{"String", ir.FieldTypeString, `"string"`, false},
		{"Integer", ir.FieldTypeInteger, `"integer"`, false},
		{"Unknown", ir.FieldTypeUnknown, `"unknown"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.ft)
			if (err != nil) != tt.wantErr {
				t.Errorf("FieldType.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != tt.expected {
				t.Errorf("FieldType.MarshalJSON() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestFieldType_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ir.FieldType
		wantErr  bool
	}{
		{"String", `"string"`, ir.FieldTypeString, false},
		{"Integer", `"integer"`, ir.FieldTypeInteger, false},
		{"Unknown", `"unknown"`, ir.FieldTypeUnknown, false},
		{"InvalidString", `"not-a-type"`, ir.FieldTypeUnknown, false}, // Invalid string maps to unknown
		{"InvalidJSON", `invalid json`, ir.FieldTypeUnknown, true},    // Malformed JSON
		{"NumberJSON", `123`, ir.FieldTypeUnknown, true},               // Not a string
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft ir.FieldType
			err := json.Unmarshal([]byte(tt.input), &ft)
			if (err != nil) != tt.wantErr {
				t.Errorf("FieldType.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && ft != tt.expected {
				t.Errorf("FieldType.UnmarshalJSON() = %v, want %v", ft, tt.expected)
			}
		})
	}
}
