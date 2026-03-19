package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestSchemaOffsets_FlatSchema(t *testing.T) {
	cs := mustCompile(flatSchema, nil)

	// flatSchema has only root schema (index 0), 3 fields.
	start, end := ir.SchemaOffsetRange(cs, 0)
	if start != 0 {
		t.Errorf("Root start: got %d, want 0", start)
	}
	if end != 3 {
		t.Errorf("Root end: got %d, want 3", end)
	}

	// Schema index 1 should be empty.
	start, end = ir.SchemaOffsetRange(cs, 1)
	if start != end {
		t.Errorf("Schema 1 should be empty, got %d..%d", start, end)
	}
}

func TestSchemaOffsets_NestedObjectSchema(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	// Root (0) has 1 field (address).
	start0, end0 := ir.SchemaOffsetRange(cs, 0)
	if start0 != 0 {
		t.Errorf("Root start: got %d, want 0", start0)
	}
	if end0 != 1 {
		t.Errorf("Root end: got %d, want 1", end0)
	}

	// Address (1) has 2 fields (street, city).
	// Descriptors are appended in index order.
	start1, end1 := ir.SchemaOffsetRange(cs, 1)
	if start1 != 1 {
		t.Errorf("Address start: got %d, want 1", start1)
	}
	if end1 != 3 {
		t.Errorf("Address end: got %d, want 3", end1)
	}
}
