package definition

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
)

// Null-handling parity tests (spec: three field states; `nullable:true` makes
// explicit null a legal value that also satisfies `required`).
//
// Default nullable is TRUE (backward compat).  Explicitly set nullable:false
// to reject null.
//
// Matrix:
//
//	required + absent            → REQUIRED_FIELD_MISSING
//	optional + absent            → no issues
//	nullable  + null             → no issues (also satisfies required)
//	nullable:false + null        → TYPE_MISMATCH ("not nullable")
//	partialStrict + req. missing → suppressed
func TestValidator_NullHandling(t *testing.T) {
	schema := &Schema{
		BaseSchema: BaseSchema{
			Name: "null_matrix",
			Fields: map[FieldId]Field{
				"req":    {Name: "req", Type: FieldTypeString, Required: true},
				"opt":    {Name: "opt", Type: FieldTypeString},
				"nul":    {Name: "nul", Type: FieldTypeString, Nullable: BoolPtr(true), Required: true},
				"strict": {Name: "strict", Type: FieldTypeString, Nullable: BoolPtr(false)},
			},
		},
		Version: common.MustNewVersion("1.0.0"),
	}

	v, err := NewDocumentValidator(schema, nil)
	if err != nil {
		t.Fatalf("build validator: %v", err)
	}

	codes := func(doc map[string]any) []string {
		issues, _ := v.Validate(doc)
		out := make([]string, 0, len(issues))
		for _, i := range issues {
			out = append(out, i.Code)
		}
		return out
	}
	has := func(codesList []string, code string) bool {
		for _, c := range codesList {
			if c == code {
				return true
			}
		}
		return false
	}

	t.Run("required+absent", func(t *testing.T) {
		got := codes(map[string]any{})
		if !has(got, "REQUIRED_FIELD_MISSING") {
			t.Fatalf("expected REQUIRED_FIELD_MISSING, got %v", got)
		}
	})

	t.Run("optional+absent", func(t *testing.T) {
		doc := map[string]any{"req": "x", "nul": "z"}
		if got := codes(doc); len(got) != 0 {
			t.Fatalf("unexpected issues: %v", got)
		}
	})

	t.Run("nullable+null satisfies required", func(t *testing.T) {
		doc := map[string]any{"req": "x", "nul": nil}
		if got := codes(doc); len(got) != 0 {
			t.Fatalf("explicit null on nullable field must pass, got %v", got)
		}
	})

	t.Run("non-nullable+null is TYPE_MISMATCH", func(t *testing.T) {
		doc := map[string]any{"req": "x", "strict": nil, "nul": "z"}
		got := codes(doc)
		if !has(got, "TYPE_MISMATCH") {
			t.Fatalf("expected TYPE_MISMATCH for non-nullable null, got %v", got)
		}
		if has(got, "REQUIRED_FIELD_MISSING") {
			t.Fatalf("null presence must satisfy required")
		}
	})

	t.Run("default nullable accepts null", func(t *testing.T) {
		s := &Schema{
			BaseSchema: BaseSchema{
				Name: "default_nullable",
				Fields: map[FieldId]Field{
					"f": {Name: "f", Type: FieldTypeString},
				},
			},
			Version: common.MustNewVersion("1.0.0"),
		}
		dv, derr := NewDocumentValidator(s, nil)
		if derr != nil {
			t.Fatalf("default validator: %v", derr)
		}
		dissues, _ := dv.Validate(map[string]any{"f": nil})
		if len(dissues) != 0 {
			t.Fatalf("default nullable should accept null, got %v", dissues)
		}
	})

	t.Run("explicit nullable:false rejects null", func(t *testing.T) {
		s := &Schema{
			BaseSchema: BaseSchema{
				Name: "explicit_non_nullable",
				Fields: map[FieldId]Field{
					"f": {Name: "f", Type: FieldTypeString, Nullable: BoolPtr(false)},
				},
			},
			Version: common.MustNewVersion("1.0.0"),
		}
		dv, derr := NewDocumentValidator(s, nil)
		if derr != nil {
			t.Fatalf("strict validator: %v", derr)
		}
		dissues, _ := dv.Validate(map[string]any{"f": nil})
		if !has(getIssueCodes(dissues), "TYPE_MISMATCH") {
			t.Fatalf("explicit nullable:false should reject null, got %v", dissues)
		}
	})

	t.Run("partialStrict suppresses only missing-required", func(t *testing.T) {
		pv, perr := NewDocumentValidatorWithConfig(schema, nil, ValidationConfig{
			Mode:     ValidationModePartialStrict,
			MaxDepth: 20,
		})
		if perr != nil {
			t.Fatalf("partial validator: %v", perr)
		}
		issues, _ := pv.ValidatePartial(map[string]any{})
		for _, i := range issues {
			if i.Code == "REQUIRED_FIELD_MISSING" {
				t.Fatalf("partialStrict must suppress REQUIRED_FIELD_MISSING")
			}
		}
	})
}

func getIssueCodes(issues []common.Issue) []string {
	out := make([]string, len(issues))
	for i, iss := range issues {
		out[i] = iss.Code
	}
	return out
}

func testNullMatrixSchema() *Schema {
	return &Schema{
		BaseSchema: BaseSchema{
			Name: "null_matrix",
			Fields: map[FieldId]Field{
				"req":    {Name: "req", Type: FieldTypeString, Required: true},
				"opt":    {Name: "opt", Type: FieldTypeString},
				"nul":    {Name: "nul", Type: FieldTypeString, Nullable: BoolPtr(true), Required: true},
				"strict": {Name: "strict", Type: FieldTypeString, Nullable: BoolPtr(false)},
			},
		},
		Version: common.MustNewVersion("1.0.0"),
	}
}
