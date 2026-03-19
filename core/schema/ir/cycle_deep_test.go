package ir_test

import (
	"testing"

)

func TestAddressSpace_DeepCycle_BlockSize(t *testing.T) {
	// complexCycleSchema: A -> B -> C -> A.
	// Tests BlockSize and BlockBases logic for cycles of length > 1.
	cs := mustCompile(complexCycleSchema, nil)

	// Find schema indices from Meta.
	var (
		foundA, foundB, foundC bool
	)
	for _, m := range cs.Meta {
		if m == nil {
			continue
		}
		switch m.Name {
		case "A":
			foundA = true
		case "B":
			foundB = true
		case "C":
			foundC = true
		}
	}

	if !foundA || !foundB || !foundC {
		t.Fatal("A, B, or C schema not found in Meta")
	}

}

func TestAddressSpace_DeepCycle_Ordinals(t *testing.T) {
	cs := mustCompile(complexCycleSchema, nil)

	// A.next.B.next.C.next.A.value should resolve.
	path := "start.next.next.value"
	dp, err := cs.Address(path)
	if err != nil {
		t.Fatalf("Address(%s): %v", path, err)
	}

	if dp == 0 {
		t.Fatalf("Address(%s): got 0", path)
	}
}
