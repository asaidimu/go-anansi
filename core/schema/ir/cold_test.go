package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestSchemaMetadata_FlatSchema(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	m := cs.Meta[0]
	if m == nil {
		t.Fatal("Meta[0] is nil")
	}
	if m.Name != "Flat" {
		t.Errorf("Name: got %q, want \"Flat\"", m.Name)
	}
	if len(m.Fields) != 3 {
		t.Errorf("Fields: got %d, want 3", len(m.Fields))
	}
}

func TestSchemaMetadata_NestedObjectSchema(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	// Root (0)
	m0 := cs.Meta[0]
	if m0.Name != "Person" {
		t.Errorf("Root name: got %q, want \"Person\"", m0.Name)
	}

	// Address (1)
	m1 := cs.Meta[1]
	if m1.Name != "Address" {
		t.Errorf("Address name: got %q, want \"Address\"", m1.Name)
	}
	if m1.UUID != nestedAddressSchemaUUID {
		t.Errorf("Address UUID: got %q, want %q", m1.UUID, nestedAddressSchemaUUID)
	}
}

func TestResolvedIndexes_FlatSchema(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	// flatSchema has no indexes.
	if len(cs.ResolvedIndexes) != 0 {
		t.Errorf("ResolvedIndexes: got %d entries, want 0", len(cs.ResolvedIndexes))
	}
}

func TestResolvedIndexes_IndexedSchema(t *testing.T) {
	cs := mustCompile(indexedSchema, nil)

	// indexedSchema has 1 index in root (schemaIdx 0), index ordinal 0.
	// Key = (schemaIdx << 8) | indexOrdinal
	key := uint16(0)<<8 | uint16(0)
	ri, ok := cs.ResolvedIndexes[key]
	if !ok {
		t.Fatal("ResolvedIndex not found for key 0")
	}

	if ri.Type != ir.IndexTypeUnique {
		t.Error("Index should be unique type")
	}
	if len(ri.Fields) != 1 {
		t.Fatalf("Index fields: got %d, want 1", len(ri.Fields))
	}
}

func TestResolvedConstraints_ConstrainedSchema(t *testing.T) {
	cs := mustCompileWithStubPredicate(constrainedSchema, "isEmail")

	if cs.ResolvedConstraints == nil {
		t.Fatal("ResolvedConstraints is nil")
	}

	// constrainedSchema has 1 root constraint.
	roots := cs.ResolvedConstraints.Roots[0]
	if roots == nil {
		t.Fatal("Root constraints for schema 0 is nil")
	}
}
