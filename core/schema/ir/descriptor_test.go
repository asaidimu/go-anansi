package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestDescriptors_FlatSchema_TerminalBits(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	// All scalar fields are terminal.
	for _, fd := range cs.Descriptors {
		if (fd & ir.FDMaskTerminal) == 0 {
			t.Errorf("field descriptor 0x%08X: terminal bit NOT set", fd)
		}
	}
}

func TestDescriptors_NestedObjectSchema_TerminalBits(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	// Person.address(0) in Person(0). Address is index 1.
	// Address is not a cycle target for this field, so terminal=1.
	for _, fd := range cs.Descriptors {
		// All fields in this schema are terminal.
		if (fd & ir.FDMaskTerminal) == 0 {
			t.Errorf("field descriptor 0x%08X: terminal bit NOT set", fd)
		}
	}
}

func TestDescriptors_CycleSchema_TerminalBits(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)

	// Node.next(0) in Node(0) points back to Node(0).
	// This is a back-edge (cycle), so terminal=0.
	foundCycleField := false
	for _, fd := range cs.Descriptors {
		if ir.ExtractType(fd) == ir.TypeObject {
			if (fd & ir.FDMaskTerminal) != 0 {
				t.Errorf("next field descriptor 0x%08X: terminal bit IS set", fd)
			}
			foundCycleField = true
		}
	}
	if !foundCycleField {
		t.Error("Did not find any object fields (potential cycle fields) in cycleSchema")
	}
}

func TestDescriptors_FlatSchema_PackDescriptorRoundTrip(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	fd := cs.Descriptors[0] // name

	if ir.ExtractType(fd) != ir.TypeString {
		t.Errorf("Type: got %v, want TypeString", ir.ExtractType(fd))
	}
	if ir.ExtractOwnerSchema(fd) != 0 {
		t.Errorf("OwnerSchema: got %d, want 0", ir.ExtractOwnerSchema(fd))
	}
	if ir.ExtractFieldIndex(fd) != 0 {
		t.Errorf("FieldIndex: got %d, want 0", ir.ExtractFieldIndex(fd))
	}
}
