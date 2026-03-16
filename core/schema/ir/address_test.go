package ir

import "github.com/asaidimu/go-anansi/v6/core/document"

import (
	"testing"

)

// address_test.go tests the CompiledAddressSpace build algorithm and the
// Address() method on *CompiledSchema.
//
// Tests are organised around the spec guarantees (Sections 8 and 9) and the
// worked example (Section 7). All fixtures are from testdata_test.go; this
// file adds no new JSON fixtures.

// ── Address space build tests ─────────────────────────────────────────────

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
	// All three ordinals must be distinct.
	ordinals := map[uint32]string{}
	for name, fi := range nameMap {
		ord := as.FieldOrdinals[0][fi]
		if prev, clash := ordinals[ord]; clash {
			t.Errorf("ordinal %d assigned to both %q and %q", ord, prev, name)
		}
		ordinals[ord] = name
	}
}

func TestAddressSpace_FlatSchema_NoCyclicTargets(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	as := cs.AddressSpace
	for i, base := range as.BlockBases {
		if base != 0 {
			t.Errorf("BlockBases[%d] = %d, want 0 (no cyclic targets in flat schema)", i, base)
		}
	}
}

func TestAddressSpace_NestedObjectSchema_DescendsIntoTarget(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	as := cs.AddressSpace

	// nestedObjectSchema: root has 1 field (address → Address schema).
	// Address has 2 fields (street, city). Acyclic: address=1, street=2, city=3.
	if as.FrontSize != 3 {
		t.Errorf("FrontSize: got %d, want 3", as.FrontSize)
	}

	// address (root, fieldIdx=0) should have ordinal 1.
	addrFI := as.FieldNames[0]["address"]
	if as.FieldOrdinals[0][addrFI] != 1 {
		t.Errorf("address ordinal: got %d, want 1", as.FieldOrdinals[0][addrFI])
	}

	// street and city should have ordinals 2 and 3 (lex order: street < city
	// only if street UUID < city UUID; in our fixture ...0011 < ...0012 so
	// street=2, city=3).
	addrIdx := uint8(1) // nestedAddressSchemaUUID → index 1
	streetFI := as.FieldNames[addrIdx]["street"]
	cityFI := as.FieldNames[addrIdx]["city"]
	streetOrd := as.FieldOrdinals[addrIdx][streetFI]
	cityOrd := as.FieldOrdinals[addrIdx][cityFI]

	if streetOrd != 2 {
		t.Errorf("street ordinal: got %d, want 2", streetOrd)
	}
	if cityOrd != 3 {
		t.Errorf("city ordinal: got %d, want 3", cityOrd)
	}
}

func TestAddressSpace_CycleSchema_BlockAllocated(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)
	as := cs.AddressSpace

	// cycleSchema: root has label (scalar) + node (→ Node).
	// Node has value (scalar) + next (→ Node, back-edge).
	// Acyclic nodes: label=1, node=2, value=3, next=4. FrontSize=4.
	if as.FrontSize != 4 {
		t.Errorf("FrontSize: got %d, want 4", as.FrontSize)
	}

	// Find the Node schema index.
	var nodeSchemaIdx uint8
	for idx, m := range cs.Meta {
		if m != nil && m.Name == "Node" {
			nodeSchemaIdx = idx
			break
		}
	}
	if nodeSchemaIdx == 0 {
		t.Fatal("Node schema not found in Meta")
	}

	// Node is a cyclic target — it has one back-edge field (next).
	// AcyclicSubtreeSize[Node] = 2 (value, next). BackEdgeCount = 1.
	// BlockSize[Node] = 2 × 1 = 2.
	if as.AcyclicSubtreeSize[nodeSchemaIdx] != 2 {
		t.Errorf("AcyclicSubtreeSize[Node]: got %d, want 2", as.AcyclicSubtreeSize[nodeSchemaIdx])
	}
	if as.BlockSize[nodeSchemaIdx] != 2 {
		t.Errorf("BlockSize[Node]: got %d, want 2", as.BlockSize[nodeSchemaIdx])
	}

	// BlockBases[Node] = 2^27 - 1 - FrontSize = 134217727 - 4 = 134217723.
	wantBase := addressSpaceMax - as.FrontSize
	if as.BlockBases[nodeSchemaIdx] != wantBase {
		t.Errorf("BlockBases[Node]: got %d, want %d", as.BlockBases[nodeSchemaIdx], wantBase)
	}
}

func TestAddressSpace_FieldNames_CoverAllFields(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	as := cs.AddressSpace

	// Every field in every schema must appear in FieldNames.
	for schemaIdx, m := range cs.Meta {
		if m == nil {
			continue
		}
		nameMap := as.FieldNames[schemaIdx]
		for _, fm := range m.Fields {
			if _, ok := nameMap[fm.Name]; !ok {
				t.Errorf("FieldNames[%d] missing field %q", schemaIdx, fm.Name)
			}
		}
	}
}

func TestAddressSpace_Invariant_OrdinalsNonZero(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	as := cs.AddressSpace

	for schemaIdx, nameMap := range as.FieldNames {
		if nameMap == nil {
			continue
		}
		for name, fi := range nameMap {
			ord := as.FieldOrdinals[schemaIdx][fi]
			if ord == 0 {
				t.Errorf("FieldOrdinals[%d][%d] (%s): ordinal 0 assigned", schemaIdx, fi, name)
			}
		}
	}
}

func TestAddressSpace_Invariant_BlockBasesAboveFrontSize(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)
	as := cs.AddressSpace

	for i, base := range as.BlockBases {
		if base != 0 && base <= as.FrontSize {
			t.Errorf("BlockBases[%d]=%d <= FrontSize=%d: front/back overlap", i, base, as.FrontSize)
		}
	}
}

func TestAddressSpace_Invariant_OrdinalsUnique(t *testing.T) {
	schemas := [][]byte{flatSchema, nestedObjectSchema, unionSchema}
	for _, src := range schemas {
		cs := mustCompileAny(t, src)
		as := cs.AddressSpace
		seen := map[uint32]string{}
		for schemaIdx, nameMap := range as.FieldNames {
			if nameMap == nil {
				continue
			}
			for name, fi := range nameMap {
				ord := as.FieldOrdinals[uint8(schemaIdx)][fi]
				key := itoa(schemaIdx) + "." + name
				if prev, clash := seen[ord]; clash {
					t.Errorf("ordinal %d assigned to both %q and %q", ord, prev, key)
				}
				seen[ord] = key
			}
		}
	}
}

// ── Address() tests ───────────────────────────────────────────────────────

func TestAddress_SingleSegment(t *testing.T) {
	cs := mustCompile(flatSchema, nil)

	dp, err := cs.Address("name")
	if err != nil {
		t.Fatalf("Address(name): unexpected error: %v", err)
	}
	if dp == 0 {
		t.Error("Address(name): got zero DataPoint")
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
	if dp == 0 {
		t.Error("Address(address.city): got zero DataPoint")
	}
	if dp.Type() != document.TypeString {
		t.Errorf("DataType: got %v, want TypeString", dp.Type())
	}
}

func TestAddress_AllFlatFieldsResolve(t *testing.T) {
	cs := mustCompile(flatSchema, nil)

	for _, name := range []string{"name", "desc", "version"} {
		dp, err := cs.Address(name)
		if err != nil {
			t.Errorf("Address(%s): %v", name, err)
			continue
		}
		if dp == 0 {
			t.Errorf("Address(%s): got zero DataPoint", name)
		}
	}
}

func TestAddress_UnknownSegmentReturnsError(t *testing.T) {
	cs := mustCompile(flatSchema, nil)

	_, err := cs.Address("nonexistent")
	if err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

func TestAddress_EmptyPathReturnsError(t *testing.T) {
	cs := mustCompile(flatSchema, nil)

	_, err := cs.Address("")
	if err == nil {
		t.Error("expected error for empty path, got nil")
	}
}

func TestAddress_ScalarInNonTerminalReturnsError(t *testing.T) {
	// "name.anything" — name is a scalar, can't descend into it.
	cs := mustCompile(flatSchema, nil)

	_, err := cs.Address("name.anything")
	if err == nil {
		t.Error("expected error for scalar in non-terminal position, got nil")
	}
}

func TestAddress_AllFrontOrdinalsUnique(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	paths := []string{"address", "address.street", "address.city"}
	ordinals := map[int32]string{}
	for _, path := range paths {
		dp, err := cs.Address(path)
		if err != nil {
			t.Errorf("Address(%s): %v", path, err)
			continue
		}
		if prev, clash := ordinals[dp.ID()]; clash {
			t.Errorf("ordinal collision: %q and %q both got ID %d", prev, path, dp.ID())
		}
		ordinals[dp.ID()] = path
	}
}

func TestAddress_CyclicPath_DepthOne(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)

	// "node" — acyclic, front region.
	dpNode, err := cs.Address("node")
	if err != nil {
		t.Fatalf("Address(node): %v", err)
	}

	// "node.next" — next is a back-edge; this enters the back region.
	dpNext, err := cs.Address("node.next")
	if err != nil {
		t.Fatalf("Address(node.next): %v", err)
	}

	// "node.value" — acyclic descent into Node.
	dpValue, err := cs.Address("node.value")
	if err != nil {
		t.Fatalf("Address(node.value): %v", err)
	}

	// All three must be distinct.
	ids := map[int32]string{
		dpNode.ID():  "node",
		dpNext.ID():  "node.next",
		dpValue.ID(): "node.value",
	}
	if len(ids) != 3 {
		t.Errorf("ordinal collision among node, node.next, node.value: %v", ids)
	}

	// Back-region ordinals must be > FrontSize.
	as := cs.AddressSpace
	if uint32(dpNext.ID()) <= as.FrontSize {
		t.Errorf("node.next ordinal %d is in front region (FrontSize=%d)", dpNext.ID(), as.FrontSize)
	}
}

func TestAddress_CyclicPath_DepthTwo(t *testing.T) {
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

	// All three must be distinct.
	if dp1.ID() == dp2.ID() {
		t.Errorf("node.next and node.next.next share ordinal %d", dp1.ID())
	}
	if dp1.ID() == dp3.ID() {
		t.Errorf("node.next and node.next.value share ordinal %d", dp1.ID())
	}
	if dp2.ID() == dp3.ID() {
		t.Errorf("node.next.next and node.next.value share ordinal %d", dp2.ID())
	}

	// Depth-2 ordinals must be in the back region.
	as := cs.AddressSpace
	if uint32(dp2.ID()) <= as.FrontSize {
		t.Errorf("node.next.next ordinal %d in front region", dp2.ID())
	}
}

func TestAddress_NoAllocation(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	// Warm up.
	_, _ = cs.Address("address.city")

	allocs := testing.AllocsPerRun(100, func() {
		_, _ = cs.Address("address.city")
	})
	// Allow 1 allocation for the DataPoint construction via document.NewDataPoint,
	// but Address itself (the path resolution) must allocate 0.
	// strings.Split allocates; in a zero-alloc hot path callers would pre-split.
	// We document this and test that the overall count is low (≤2).
	if allocs > 2 {
		t.Errorf("Address allocated %.0f times, want ≤2", allocs)
	}
}

func TestAddress_ReturnType_MatchesFieldType(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	cases := []struct {
		path string
		want document.DataType
	}{
		{"address.street", document.TypeString},
		{"address.city", document.TypeString},
	}
	for _, c := range cases {
		dp, err := cs.Address(c.path)
		if err != nil {
			t.Errorf("Address(%s): %v", c.path, err)
			continue
		}
		if dp.Type() != c.want {
			t.Errorf("Address(%s): DataType got %v, want %v", c.path, dp.Type(), c.want)
		}
	}
}

func TestAddress_UnionResolution(t *testing.T) {
	cs := mustCompile(unionSchema, nil)

	// "payload.typeA" — payload is a union, typeA is in VariantA.
	dpA, err := cs.Address("payload.typeA")
	if err != nil {
		t.Fatalf("Address(payload.typeA): %v", err)
	}
	if dpA.Type() != document.TypeString {
		t.Errorf("typeA DataType: got %v, want TypeString", dpA.Type())
	}

	// "payload.typeB" — typeB is in VariantB.
	dpB, err := cs.Address("payload.typeB")
	if err != nil {
		t.Fatalf("Address(payload.typeB): %v", err)
	}
	if dpB.Type() != document.TypeInt {
		t.Errorf("typeB DataType: got %v, want TypeInt", dpB.Type())
	}

	if dpA.ID() == dpB.ID() {
		t.Errorf("typeA and typeB share same ID %d", dpA.ID())
	}
}

func TestAddress_UnionCommonField(t *testing.T) {
	// A union where both variants have a field with the same name.
	// Address() should resolve to the first match instead of an error.
	src := []byte(`{
	  "name": "CommonField",
	  "version": "1.0.0",
	  "fields": {
	    "019ca000-0001-7000-b000-000000000001": {
	      "name": "u", "type": "union",
	      "schema": [
	        { "id": "019ca000-0002-7000-b000-000000000002" },
	        { "id": "019ca000-0003-7000-b000-000000000003" }
	      ]
	    }
	  },
	  "schemas": {
	    "019ca000-0002-7000-b000-000000000002": {
	      "name": "V1", "fields": { "019ca000-0004-7000-b000-000000000004": { "name": "common", "type": "string" } }
	    },
	    "019ca000-0003-7000-b000-000000000003": {
	      "name": "V2", "fields": { "019ca000-0005-7000-b000-000000000005": { "name": "common", "type": "string" } }
	    }
	  }
	}`)
	cs := mustCompile(src, nil)

	dp, err := cs.Address("u.common")
	if err != nil {
		t.Fatalf("Address(u.common): unexpected error: %v", err)
	}
	if dp.Type() != document.TypeString {
		t.Errorf("common DataType: got %v, want TypeString", dp.Type())
	}
}

func TestAddress_TypeSchemaPassthrough(t *testing.T) {
	// A schema where an object points to a type schema (union), which then
	// points to the final object schema. This tests the "jump" through
	// schemas with no fields of their own.
	src := []byte(`{
	  "name": "Proxy",
	  "version": "1.0.0",
	  "fields": {
	    "019ca000-0001-7000-b000-000000000001": {
	      "name": "p", "type": "object",
	      "schema": { "id": "019ca000-0002-7000-b000-000000000002" }
	    }
	  },
	  "schemas": {
	    "019ca000-0002-7000-b000-000000000002": {
	      "name": "Wrapper", "type": "union",
	      "schema": [ { "id": "019ca000-0003-7000-b000-000000000003" } ]
	    },
	    "019ca000-0003-7000-b000-000000000003": {
	      "name": "Target", "fields": { "019ca000-0004-7000-b000-000000000004": { "name": "f", "type": "string" } }
	    }
	  }
	}`)
	cs := mustCompile(src, nil)

	// "p.f" — p points to Wrapper (union) → Target (object) → f.
	dp, err := cs.Address("p.f")
	if err != nil {
		t.Fatalf("Address(p.f): %v", err)
	}
	if dp.Type() != document.TypeString {
		t.Errorf("f DataType: got %v, want TypeString", dp.Type())
	}
}

func TestAddress_CompositeResolution(t *testing.T) {
	// A composite field whose variants are combined. Address should resolve
	// segments against any of the constituent schemas.
	src := []byte(`{
	  "name": "Comp",
	  "version": "1.0.0",
	  "fields": {
	    "019ca000-0001-7000-b000-000000000001": {
	      "name": "c", "type": "composite",
	      "schema": [
	        { "id": "019ca000-0002-7000-b000-000000000002" },
	        { "id": "019ca000-0003-7000-b000-000000000003" }
	      ]
	    }
	  },
	  "schemas": {
	    "019ca000-0002-7000-b000-000000000002": {
	      "name": "S1", "fields": { "019ca000-0004-7000-b000-000000000004": { "name": "f1", "type": "string" } }
	    },
	    "019ca000-0003-7000-b000-000000000003": {
	      "name": "S2", "fields": { "019ca000-0005-7000-b000-000000000005": { "name": "f2", "type": "integer" } }
	    }
	  }
	}`)
	cs := mustCompile(src, nil)

	dp1, err := cs.Address("c.f1")
	if err != nil {
		t.Fatalf("Address(c.f1): %v", err)
	}
	if dp1.Type() != document.TypeString {
		t.Errorf("f1 type: %v", dp1.Type())
	}

	dp2, err := cs.Address("c.f2")
	if err != nil {
		t.Fatalf("Address(c.f2): %v", err)
	}
	if dp2.Type() != document.TypeInt {
		t.Errorf("f2 type: %v", dp2.Type())
	}
}

// ── Integration: indexes and constraints resolve via Address ──────────────

func TestAddress_IndexResolution_PopulatesFields(t *testing.T) {
	// indexedSchema has one index on "sku". After Compile, the ResolvedIndex
	// must have one field DataPoint with a non-zero ID.
	cs := mustCompile(indexedSchema, nil)

	key := uint16(0)<<8 | uint16(0)
	ri, ok := cs.ResolvedIndexes[key]
	if !ok {
		t.Fatal("ResolvedIndexes[0] not found")
	}
	if len(ri.Fields) != 1 {
		t.Fatalf("ResolvedIndex fields: got %d, want 1", len(ri.Fields))
	}
	if ri.Fields[0] == 0 {
		t.Error("ResolvedIndex.Fields[0] is zero DataPoint")
	}
	if ri.Fields[0].Type() != document.TypeString {
		t.Errorf("ResolvedIndex.Fields[0] type: got %v, want TypeString", ri.Fields[0].Type())
	}
}

func TestAddress_ConstraintResolution_PopulatesFields(t *testing.T) {
	cs := mustCompileWithStubPredicate(constrainedSchema, "isEmail")

	rt, ok := cs.ResolvedConstraints[0]
	if !ok || rt == nil {
		t.Fatal("ResolvedConstraints[0] not found")
	}
	rc, ok := rt.Roots[0].(ResolvedConstraint)
	if !ok {
		t.Fatal("root constraint is not a ResolvedConstraint")
	}
	if len(rc.Fields) != 1 {
		t.Fatalf("resolved constraint fields: got %d, want 1", len(rc.Fields))
	}
	if rc.Fields[0] == 0 {
		t.Error("resolved constraint field is zero DataPoint")
	}
}
