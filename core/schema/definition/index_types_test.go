package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

func TestIndexType_String(t *testing.T) {
	tests := []struct {
		name     string
		it       definition.IndexType
		expected string
	}{
		{"Normal", definition.IndexTypeNormal, "normal"},
		{"Unique", definition.IndexTypeUnique, "unique"},
		{"Primary", definition.IndexTypePrimary, "primary"},
		{"Spatial", definition.IndexTypeSpatial, "spatial"},
		{"FullText", definition.IndexTypeFullText, "fulltext"},
		{"Invalid", definition.IndexType(99), "normal"}, // Test an unknown value, defaults to normal
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.it.String(); got != tt.expected {
				t.Errorf("IndexType.String() for %v = %s, want %s", tt.it, got, tt.expected)
			}
		})
	}
}

func TestIndexType_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		it       definition.IndexType
		expected string
		wantErr  bool
	}{
		{"Normal", definition.IndexTypeNormal, `"normal"`, false},
		{"Unique", definition.IndexTypeUnique, `"unique"`, false},
		{"Invalid", definition.IndexType(99), `"normal"`, false}, // Marshals to default "normal"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.it)
			if (err != nil) != tt.wantErr {
				t.Errorf("IndexType.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != tt.expected {
				t.Errorf("IndexType.MarshalJSON() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestIndexType_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected definition.IndexType
		wantErr  bool
	}{
		{"Normal", `"normal"`, definition.IndexTypeNormal, false},
		{"Unique", `"unique"`, definition.IndexTypeUnique, false},
		{"InvalidString", `"not-an-index-type"`, definition.IndexTypeNormal, false}, // Invalid string maps to default normal
		{"InvalidJSON", `invalid json`, definition.IndexTypeNormal, true},           // Malformed JSON
		{"NumberJSON", `123`, definition.IndexTypeNormal, true},                     // Not a string
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var it definition.IndexType
			err := json.Unmarshal([]byte(tt.input), &it)
			if (err != nil) != tt.wantErr {
				t.Errorf("IndexType.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && it != tt.expected {
				t.Errorf("IndexType.UnmarshalJSON() = %v, want %v", it, tt.expected)
			}
		})
	}
}
