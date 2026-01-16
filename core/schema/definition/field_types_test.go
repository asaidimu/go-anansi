package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

func TestFieldType_String(t *testing.T) {
	tests := []struct {
		name     string
		ft       definition.FieldType
		expected string
	}{
		{"Unknown", definition.FieldTypeUnknown, "unknown"},
		{"String", definition.FieldTypeString, "string"},
		{"Number", definition.FieldTypeNumber, "number"},
		{"Integer", definition.FieldTypeInteger, "integer"},
		{"Decimal", definition.FieldTypeDecimal, "decimal"},
		{"Boolean", definition.FieldTypeBoolean, "boolean"},
		{"Array", definition.FieldTypeArray, "array"},
		{"Set", definition.FieldTypeSet, "set"},
		{"Enum", definition.FieldTypeEnum, "enum"},
		{"Object", definition.FieldTypeObject, "object"},
		{"Record", definition.FieldTypeRecord, "record"},
		{"Union", definition.FieldTypeUnion, "union"},
		{"Composite", definition.FieldTypeComposite, "composite"},
		{"Geometry", definition.FieldTypeGeometry, "geometry"},
		{"Invalid", definition.FieldType(99), ""}, // Test an unknown value
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
		ft       definition.FieldType
		expected string
		wantErr  bool
	}{
		{"String", definition.FieldTypeString, `"string"`, false},
		{"Integer", definition.FieldTypeInteger, `"integer"`, false},
		{"Unknown", definition.FieldTypeUnknown, `"unknown"`, false},
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
		expected definition.FieldType
		wantErr  bool
	}{
		{"String", `"string"`, definition.FieldTypeString, false},
		{"Integer", `"integer"`, definition.FieldTypeInteger, false},
		{"Unknown", `"unknown"`, definition.FieldTypeUnknown, false},
		{"InvalidString", `"not-a-type"`, definition.FieldTypeUnknown, false}, // Invalid string maps to unknown
		{"InvalidJSON", `invalid json`, definition.FieldTypeUnknown, true},    // Malformed JSON
		{"NumberJSON", `123`, definition.FieldTypeUnknown, true},               // Not a string
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft definition.FieldType
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
