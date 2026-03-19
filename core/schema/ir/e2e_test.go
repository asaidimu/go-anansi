package ir_test

import (
	"bytes"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestCompile_E2E_FlatSchema(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	if cs == nil {
		t.Fatal("Compile returned nil Schema")
	}

	// Basic checks.
	if cs.AddressSpace.FrontSize != 3 {
		t.Errorf("FrontSize: got %d, want 3", cs.AddressSpace.FrontSize)
	}
	if len(cs.Descriptors) != 3 {
		t.Errorf("Descriptors: got %d, want 3", len(cs.Descriptors))
	}
}

func TestCompile_E2E_NestedObjectSchema(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	// Person has 1 field (address). Address has 2 fields (street, city).
	// Total FrontSize = 1 + 2 = 3.
	if cs.AddressSpace.FrontSize != 3 {
		t.Errorf("FrontSize: got %d, want 3", cs.AddressSpace.FrontSize)
	}
	// Total descriptors = 1 (Person.address) + 2 (Address.street, Address.city) = 3.
	if len(cs.Descriptors) != 3 {
		t.Errorf("Descriptors: got %d, want 3", len(cs.Descriptors))
	}
}

func TestCompile_E2E_CycleSchema(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)

	// Node has 2 fields: next and value. next is a cycle.
	// FrontSize = 2.
	if cs.AddressSpace.FrontSize != 4 {
		t.Errorf("FrontSize: got %d, want 4", cs.AddressSpace.FrontSize)
	}
}

func TestCompile_E2E_RoundTrip(t *testing.T) {
	schemas := [][]byte{flatSchema, nestedObjectSchema, unionSchema}
	for _, src := range schemas {
		cs := mustCompile(src, nil)
		out, err := ir.Serialize(cs)
		if err != nil {
			t.Errorf("Serialize failed: %v", err)
			continue
		}

		ss := mustParse(out)
		cs2, err := ir.Compile(ss, nil)
		if err != nil {
			t.Errorf("Compile of serialized IR failed: %v", err)
			continue
		}

		out2, _ := ir.Serialize(cs2)
		if !bytes.Equal(out, out2) {
			t.Errorf("Round-trip serialization mismatch")
		}
	}
}
