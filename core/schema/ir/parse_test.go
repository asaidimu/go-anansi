package ir

import "testing"

// parse_test.go tests Pass 1: JSON → source model.

func TestParse_ValidMinimal(t *testing.T) {
	ss, err := Parse(flatSchema)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if ss == nil {
		t.Fatal("expected non-nil SourceSchema")
	}
	if ss.inner.Name != "Flat" {
		t.Errorf("name: got %q, want %q", ss.inner.Name, "Flat")
	}
	if ss.inner.Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", ss.inner.Version, "1.0.0")
	}
	if len(ss.inner.Fields) != 3 {
		t.Errorf("fields: got %d, want 3", len(ss.inner.Fields))
	}
}

func TestParse_MissingName(t *testing.T) {
	_, err := Parse(invalidMissingName)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ce := firstError(err)
	if ce.Pass != PassParse {
		t.Errorf("pass: got %v, want %v", ce.Pass, PassParse)
	}
}

func TestParse_MissingVersion(t *testing.T) {
	_, err := Parse(invalidMissingVersion)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ce := firstError(err)
	if ce.Pass != PassParse {
		t.Errorf("pass: got %v, want %v", ce.Pass, PassParse)
	}
}

func TestParse_BadJSON(t *testing.T) {
	_, err := Parse(invalidBadJSON)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ce := firstError(err)
	if ce.Pass != PassParse {
		t.Errorf("pass: got %v, want %v", ce.Pass, PassParse)
	}
}

func TestParse_WithNestedSchemas(t *testing.T) {
	ss, err := Parse(nestedObjectSchema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ss.inner.Schemas) != 1 {
		t.Errorf("schemas: got %d, want 1", len(ss.inner.Schemas))
	}
}

func TestParse_EmptyFields(t *testing.T) {
	// A schema with no fields at the root level should parse successfully —
	// field presence is a constraint on object schemas, not on the parser.
	src := []byte(`{"name":"Empty","version":"1.0.0"}`)
	ss, err := Parse(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ss.inner.Fields) != 0 {
		t.Errorf("fields: got %d, want 0", len(ss.inner.Fields))
	}
}
