package ir_test

import (
	"strconv"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

// address_test.go tests the CompiledAddressSpace build algorithm and the
// Address() method on *CompiledSchema.

func TestAddressSpace_FlatSchema_FrontOrdinals(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	as := cs.AddressSpace
	if as == nil {
		t.Fatal("AddressSpace is nil")
	}

	// flatSchema root (index 0) has 3 fields in lex order: name(0), desc(1), version(2).
	// Expected front ordinals: name=1, desc=2, version=3. FrontSize=3.
	if as.FrontSize != 3 {
		t.Errorf("FrontSize: got %d, want 3", as.FrontSize)
	}

	// Verify via FieldNames → FieldOrdinals round-trip.
	nameMap := as.FieldNames[0]
	if nameMap == nil {
		t.Fatal("FieldNames[0] is nil")
	}
	for _, name := range []string{"name", "desc", "version"} {
		fi, ok := nameMap[name]
		if !ok {
			t.Errorf("FieldNames[0][%q] not found", name)
			continue
		}
		ord := as.FieldOrdinals[0][fi]
		if ord == 0 {
			t.Errorf("FieldOrdinals[0][%d] (%s) is 0 — sentinel", fi, name)
		}
	}
}

func TestAddressSpace_CycleSchema_BlockAssignment(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)
	as := cs.AddressSpace

	// Find the Node schema index.
	var nodeSchemaIdx uint8
	for idx, m := range cs.Meta {
		if m != nil && m.Name == "Node" {
			nodeSchemaIdx = uint8(idx)
			break
		}
	}
	if nodeSchemaIdx == 0 {
		t.Fatal("Node schema not found in Meta")
	}

	// Node is a cyclic target — it has one back-edge field (next).
	if as.AcyclicSubtreeSize[nodeSchemaIdx] != 2 {
		t.Errorf("AcyclicSubtreeSize[Node]: got %d, want 2", as.AcyclicSubtreeSize[nodeSchemaIdx])
	}
	if as.BlockSize[nodeSchemaIdx] != 2 {
		t.Errorf("BlockSize[Node]: got %d, want 2", as.BlockSize[nodeSchemaIdx])
	}

	// BlockBases[Node] = AddressSpaceMax - FrontSize.
	wantBase := ir.AddressSpaceMax - as.FrontSize
	if as.BlockBases[nodeSchemaIdx] != wantBase {
		t.Errorf("BlockBases[Node]: got %d, want %d", as.BlockBases[nodeSchemaIdx], wantBase)
	}
}

func TestAddressSpace_Invariant_OrdinalsUnique(t *testing.T) {
	schemas := [][]byte{flatSchema, nestedObjectSchema, unionSchema}
	for _, src := range schemas {
		cs := mustCompile(src, nil)
		as := cs.AddressSpace
		seen := map[uint32]string{}
		for schemaIdx, nameMap := range as.FieldNames {
			for name, fi := range nameMap {
				ord := as.FieldOrdinals[schemaIdx][fi]
				key := strconv.Itoa(int(schemaIdx)) + "." + name
				if prev, clash := seen[ord]; clash {
					t.Errorf("ordinal %d assigned to both %q and %q", ord, prev, key)
				}
				seen[ord] = key
			}
		}
	}
}

func TestAddress_SingleSegment(t *testing.T) {
	cs := mustCompile(flatSchema, nil)

	dp, err := cs.Address("name")
	if err != nil {
		t.Fatalf("Address(name): unexpected error: %v", err)
	}
	if dp.Type() != document.TypeString {
		t.Errorf("Address(name): DataType got %v, want TypeString", dp.Type())
	}
}

func TestAddress_TwoSegments(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	dp, err := cs.Address("address.city")
	if err != nil {
		t.Fatalf("Address(address.city): %v", err)
	}
	if dp.Type() != document.TypeString {
		t.Errorf("DataType: got %v, want TypeString", dp.Type())
	}
}

func TestAddress_CycleOrdinalsLarge(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)

	dpNode, err := cs.Address("node")
	if err != nil {
		t.Fatalf("Address(node): %v", err)
	}
	dpNext, err := cs.Address("node.next")
	if err != nil {
		t.Fatalf("Address(node.next): %v", err)
	}
	dpValue, err := cs.Address("node.value")
	if err != nil {
		t.Fatalf("Address(node.value): %v", err)
	}

	if dpNext.ID() <= dpNode.ID() || dpNext.ID() <= dpValue.ID() {
		t.Errorf("Cyclic ordinal %d should be much larger than front ordinals %d, %d",
			dpNext.ID(), dpNode.ID(), dpValue.ID())
	}
}

func TestAddress_DeepCycles(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)

	dp1, err := cs.Address("node.next")
	if err != nil {
		t.Fatalf("Address(node.next): %v", err)
	}
	dp2, err := cs.Address("node.next.next")
	if err != nil {
		t.Fatalf("Address(node.next.next): %v", err)
	}
	dp3, err := cs.Address("node.next.value")
	if err != nil {
		t.Fatalf("Address(node.next.value): %v", err)
	}

	if dp2.ID() >= dp1.ID() {
		t.Errorf("Deeper cycle ordinal %d should be smaller than %d", dp2.ID(), dp1.ID())
	}
	if dp3.ID() >= dp1.ID() {
		t.Errorf("Deeper cycle ordinal %d should be smaller than %d", dp3.ID(), dp1.ID())
	}
}
