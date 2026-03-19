package ir_test

import (
	"strconv"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

// address_test.go tests the CompiledAddressSpace build algorithm and the
// Address() method on *Schema.

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

	// BlockBases[Node] must be strictly greater than FrontSize (back-region invariant)
	// and must equal addressSpaceMax - FrontSize (first and only cyclic target,
	// allocated from the top of the space downward by exactly BlockSize).
	// We verify the invariant directly and the exact value by checking
	// BlockBases[Node] + BlockSize[Node] == addressSpaceMax - FrontSize + BlockSize[Node],
	// i.e. BlockBases[Node] + FrontSize == addressSpaceMax.
	// Since addressSpaceMax = 1<<27 - 1, we can compute it here without importing the constant.
	const wantAddressSpaceMax = uint32((1 << 27) - 1)
	if as.BlockBases[nodeSchemaIdx] <= as.FrontSize {
		t.Errorf("BlockBases[Node]=%d must be > FrontSize=%d", as.BlockBases[nodeSchemaIdx], as.FrontSize)
	}
	wantBase := wantAddressSpaceMax - as.FrontSize
	if as.BlockBases[nodeSchemaIdx] != wantBase {
		t.Errorf("BlockBases[Node]: got %d, want addressSpaceMax-FrontSize=%d", as.BlockBases[nodeSchemaIdx], wantBase)
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

// ── Specific ordinal values ───────────────────────────────────────────────────

// TestAddress_FlatSchema_SpecificOrdinals pins the exact ordinal for each field
// in flatSchema. Fields are assigned ordinals in UUID lex order, so:
//   "019ca000-0001..." (name)    → fieldIdx 0 → ordinal 1
//   "019ca000-0002..." (desc)    → fieldIdx 1 → ordinal 2
//   "019ca000-0003..." (version) → fieldIdx 2 → ordinal 3
// A field-swap in buildFieldIndex or buildAddressSpace would be invisible to
// any test that only checks counts or uniqueness.
func TestAddress_FlatSchema_SpecificOrdinals(t *testing.T) {
	cs := mustCompile(flatSchema, nil)

	cases := []struct {
		path    string
		wantID  int32
	}{
		{"name",    1},
		{"desc",    2},
		{"version", 3},
	}
	for _, tc := range cases {
		dp, err := cs.Address(tc.path)
		if err != nil {
			t.Errorf("Address(%q): unexpected error: %v", tc.path, err)
			continue
		}
		if dp.ID() != tc.wantID {
			t.Errorf("Address(%q).ID(): got %d, want %d", tc.path, dp.ID(), tc.wantID)
		}
	}
}

// TestAddress_NestedSchema_SpecificOrdinals pins the exact ordinals for
// nestedObjectSchema. DFS pre-order: address(root,0)=1, street(Address,0)=2,
// city(Address,1)=3.
func TestAddress_NestedSchema_SpecificOrdinals(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	cases := []struct {
		path   string
		wantID int32
	}{
		{"address",        1},
		{"address.street", 2},
		{"address.city",   3},
	}
	for _, tc := range cases {
		dp, err := cs.Address(tc.path)
		if err != nil {
			t.Errorf("Address(%q): unexpected error: %v", tc.path, err)
			continue
		}
		if dp.ID() != tc.wantID {
			t.Errorf("Address(%q).ID(): got %d, want %d", tc.path, dp.ID(), tc.wantID)
		}
	}
}

// TestAddress_SiblingOrdinalOrdering verifies that sibling fields within a
// nested schema are assigned strictly increasing ordinals in UUID lex order.
// street UUID < city UUID, so street ordinal < city ordinal.
func TestAddress_SiblingOrdinalOrdering(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	dpStreet, err := cs.Address("address.street")
	if err != nil {
		t.Fatalf("Address(address.street): %v", err)
	}
	dpCity, err := cs.Address("address.city")
	if err != nil {
		t.Fatalf("Address(address.city): %v", err)
	}
	if dpStreet.ID() >= dpCity.ID() {
		t.Errorf("street ordinal %d must be < city ordinal %d (UUID lex order)",
			dpStreet.ID(), dpCity.ID())
	}
}

// ── Terminal bit correctness ──────────────────────────────────────────────────

// TestTerminalBit_CycleSchema verifies that Pass 5 correctly sets terminal=0 on
// the back-edge field (next) and terminal=1 on scalar fields (value).
// This is the direct regression test for the two-phase cycle-detection fix.
func TestTerminalBit_CycleSchema(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)

	// Find the Node schema index.
	var nodeSchemaIdx uint8
	for idx, m := range cs.Meta {
		if m != nil && m.Name == "Node" {
			nodeSchemaIdx = uint8(idx)
			break
		}
	}
	if nodeSchemaIdx == 0 {
		t.Fatal("Node schema not found in Meta (index 0 is root, not Node)")
	}

	as := cs.AddressSpace
	nameMap := as.FieldNames[nodeSchemaIdx]

	nextFI, ok := nameMap["next"]
	if !ok {
		t.Fatal("field 'next' not found in Node schema FieldNames")
	}
	valueFI, ok := nameMap["value"]
	if !ok {
		t.Fatal("field 'value' not found in Node schema FieldNames")
	}

	start, end := int(uint16(cs.SchemaOffsets[nodeSchemaIdx])), int(uint16(cs.SchemaOffsets[nodeSchemaIdx]>>16))
	if start == end {
		t.Fatal("Node schema has empty descriptor range")
	}

	var nextFD, valueFD uint32
	for _, fd := range cs.Descriptors[start:end] {
		switch ir.ExtractFieldIndex(fd) {
		case nextFI:
			nextFD = fd
		case valueFI:
			valueFD = fd
		}
	}

	if nextFD == 0 {
		t.Fatal("could not find next field descriptor")
	}
	if valueFD == 0 {
		t.Fatal("could not find value field descriptor")
	}

	// next is a back-edge (cycle): terminal bit must be CLEAR.
	if ir.IsTerminal(nextFD) {
		t.Errorf("next field descriptor 0x%08X: terminal bit IS set — should be clear for cycle field", nextFD)
	}
	// value is a scalar: terminal bit must be SET.
	if !ir.IsTerminal(valueFD) {
		t.Errorf("value field descriptor 0x%08X: terminal bit is NOT set — should be set for scalar", valueFD)
	}
}

// ── Union address resolution ──────────────────────────────────────────────────

// TestAddress_UnionSchema verifies that Address() correctly dispatches through
// union variants. resolveUnionTarget is exercised for the first time here.
// payload is a union of VariantA (typeA: string) and VariantB (typeB: integer).
func TestAddress_UnionSchema(t *testing.T) {
	cs := mustCompile(unionSchema, nil)

	dpA, err := cs.Address("payload.typeA")
	if err != nil {
		t.Fatalf("Address(payload.typeA): %v", err)
	}
	if dpA.Type() != document.TypeString {
		t.Errorf("payload.typeA type: got %v, want TypeString", dpA.Type())
	}

	dpB, err := cs.Address("payload.typeB")
	if err != nil {
		t.Fatalf("Address(payload.typeB): %v", err)
	}
	if dpB.Type() != document.TypeInt {
		t.Errorf("payload.typeB type: got %v, want TypeInt", dpB.Type())
	}

	// The two variants must get distinct ordinals.
	if dpA.ID() == dpB.ID() {
		t.Errorf("typeA and typeB got the same ordinal %d — must be distinct", dpA.ID())
	}
}

// ── Address() error cases ─────────────────────────────────────────────────────

// TestAddress_ErrorCases verifies that Address() returns ErrAddressNotFound for
// all invalid path patterns. These protect the early-exit branches of
// resolveOrdinal that have no other coverage.
func TestAddress_ErrorCases(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	cases := []struct {
		path string
		desc string
	}{
		{"", "empty path"},
		{"nonexistent", "unknown root-level field"},
		{"address.nonexistent", "unknown field in nested schema"},
		{"label.city", "non-existent root field used as prefix"},
	}
	for _, tc := range cases {
		_, err := cs.Address(tc.path)
		if err == nil {
			t.Errorf("Address(%q) (%s): expected error, got nil", tc.path, tc.desc)
		}
	}
}

// TestAddress_ScalarInNonTerminalPosition verifies that using a scalar field as
// a non-terminal path segment returns ErrAddressNotFound rather than panicking
// or silently returning a wrong ordinal.
func TestAddress_ScalarInNonTerminalPosition(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	// "address" is a valid schema-bearing field. "address.street" is valid.
	// "address.street.anything" should fail because street is a scalar.
	_, err := cs.Address("address.street.anything")
	if err == nil {
		t.Error("Address(address.street.anything): expected error for scalar in non-terminal position, got nil")
	}
}

// ── DocumentKey and PathCache ─────────────────────────────────────────────────

// TestDocumentKey_PopulatesPathCache verifies that DocumentKey() resolves a path
// and caches it in cs.PathCache so that the path string can be recovered from
// the DocumentKey without a separate reverse-lookup.
func TestDocumentKey_PopulatesPathCache(t *testing.T) {
	cs := mustCompile(flatSchema, nil)

	path := "name"
	dk, err := cs.DocumentKey(path)
	if err != nil {
		t.Fatalf("DocumentKey(%q): %v", path, err)
	}
	if dk == 0 {
		t.Fatal("DocumentKey returned zero key")
	}

	cached, ok := cs.PathCache.GetPath(dk)
	if !ok {
		t.Fatalf("PathCache[DocumentKey(%q)] not populated", path)
	}
	if cached != path {
		t.Errorf("PathCache[DocumentKey(%q)] = %q, want %q", path, cached, path)
	}
}

// TestDocumentKey_MatchesAddress verifies that DocumentKey().DataPoint() returns
// the same DataPoint as Address() for the same path.
func TestDocumentKey_MatchesAddress(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	path := "address.city"
	dp, err := cs.Address(path)
	if err != nil {
		t.Fatalf("Address(%q): %v", path, err)
	}
	dk, err := cs.DocumentKey(path)
	if err != nil {
		t.Fatalf("DocumentKey(%q): %v", path, err)
	}
	if dk.DataPoint() != dp {
		t.Errorf("DocumentKey(%q).DataPoint() = %v, Address(%q) = %v — must match",
			path, dk.DataPoint(), path, dp)
	}
}

// ── Address space internal fields ─────────────────────────────────────────────
//
// The tests below pin the internal CompiledAddressSpace fields that resolveOrdinal
// reads directly. A regression in any of these fields would silently corrupt all
// cyclic ordinal resolution without necessarily failing any higher-level test.

// TestAddressSpace_EntryOrdinal pins EntryOrdinal for cycleSchema.
//
// EntryOrdinal[target] is the front ordinal of the tree-edge field that first
// reached `target` during DFS. For cycleSchema:
//   DFS pre-order: label(0,0)=1, node(0,1)=2, value(1,0)=3, next(1,1)=4
//   treeEdges[Node] = {schema:0, fieldIdx:1}  (the "node" field)
//   EntryOrdinal[Node] = FieldOrdinals[0][1] = 2
//
// If this is wrong by even 1, every cyclic ordinal computed as
//   blockBase + (frontOrdinal - EntryOrdinal[blockOwner])
// shifts by 1 — an off-by-one that the "ordinal is large" tests would not catch.
func TestAddressSpace_EntryOrdinal(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)
	as := cs.AddressSpace

	var nodeIdx uint8
	for idx, m := range cs.Meta {
		if m != nil && m.Name == "Node" {
			nodeIdx = uint8(idx)
			break
		}
	}
	if nodeIdx == 0 {
		t.Fatal("Node schema not found")
	}

	// "node" field in root: UUID "019ca000-0030..." is fieldIdx=1 (lex: 0001 < 0030).
	// DFS visits label(ordinal=1) then node(ordinal=2).
	// EntryOrdinal[Node] must be 2.
	if as.EntryOrdinal[nodeIdx] != 2 {
		t.Errorf("EntryOrdinal[Node]: got %d, want 2", as.EntryOrdinal[nodeIdx])
	}
}

// TestAddressSpace_BackEdgeOrdinal pins BackEdgeOrdinal for cycleSchema.
//
// Node has exactly one back-edge field ("next"). It is the only back-edge
// targeting Node, so BackEdgeOrdinal[Node][next_fieldIdx] must be 1
// (1-based, sorted by field UUID among all back-edges to the same target).
func TestAddressSpace_BackEdgeOrdinal(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)
	as := cs.AddressSpace

	var nodeIdx uint8
	for idx, m := range cs.Meta {
		if m != nil && m.Name == "Node" {
			nodeIdx = uint8(idx)
			break
		}
	}
	if nodeIdx == 0 {
		t.Fatal("Node schema not found")
	}

	// "next" field in Node: UUID "019ca000-0012..." → fieldIdx=1 (lex: 0011 < 0012).
	nextFieldIdx := as.FieldNames[nodeIdx]["next"]
	if as.BackEdgeOrdinal[nodeIdx][nextFieldIdx] != 1 {
		t.Errorf("BackEdgeOrdinal[Node][next]: got %d, want 1",
			as.BackEdgeOrdinal[nodeIdx][nextFieldIdx])
	}
}

// TestAddressSpace_FieldNamesComplete verifies that FieldNames[schemaIdx]
// contains exactly the fields present in the schema — no extras, no missing.
func TestAddressSpace_FieldNamesComplete(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	as := cs.AddressSpace

	nameMap := as.FieldNames[0]
	if len(nameMap) != 3 {
		t.Errorf("FieldNames[0] length: got %d, want 3", len(nameMap))
	}
	for _, name := range []string{"name", "desc", "version"} {
		if _, ok := nameMap[name]; !ok {
			t.Errorf("FieldNames[0] missing field %q", name)
		}
	}
}

// TestAddressSpace_MultiBackEdge exercises the case where two fields in the
// same schema are both back-edges to the same cyclic target (binaryTreeSchema:
// BTree.left and BTree.right both self-reference BTree).
//
// Expected address space values (derived from UUID lex ordering):
//   Field UUIDs: left(...010) < right(...011) < value(...012)
//   fieldIdx:    left=0, right=1, value=2
//   BackEdgeOrdinal[BTree][left=0]  = 1  (first by UUID lex)
//   BackEdgeOrdinal[BTree][right=1] = 2  (second by UUID lex)
//   AcyclicSubtreeSize[BTree] = 3
//   BlockSize[BTree] = 3 × 2 back-edges = 6
//
// Address("root.left") and Address("root.right") must produce distinct ordinals
// (they use different slots: entryOrdinal=0 vs entryOrdinal=1 respectively).
func TestAddressSpace_MultiBackEdge(t *testing.T) {
	cs := mustCompile(binaryTreeSchema, nil)
	as := cs.AddressSpace

	var btreeIdx uint8
	for idx, m := range cs.Meta {
		if m != nil && m.Name == "BTree" {
			btreeIdx = uint8(idx)
			break
		}
	}
	if btreeIdx == 0 {
		t.Fatal("BTree schema not found")
	}

	leftFI  := as.FieldNames[btreeIdx]["left"]
	rightFI := as.FieldNames[btreeIdx]["right"]

	if as.BackEdgeOrdinal[btreeIdx][leftFI] != 1 {
		t.Errorf("BackEdgeOrdinal[BTree][left]: got %d, want 1",
			as.BackEdgeOrdinal[btreeIdx][leftFI])
	}
	if as.BackEdgeOrdinal[btreeIdx][rightFI] != 2 {
		t.Errorf("BackEdgeOrdinal[BTree][right]: got %d, want 2",
			as.BackEdgeOrdinal[btreeIdx][rightFI])
	}
	if as.AcyclicSubtreeSize[btreeIdx] != 3 {
		t.Errorf("AcyclicSubtreeSize[BTree]: got %d, want 3",
			as.AcyclicSubtreeSize[btreeIdx])
	}
	if as.BlockSize[btreeIdx] != 6 {
		t.Errorf("BlockSize[BTree]: got %d, want 6", as.BlockSize[btreeIdx])
	}

	// Both back-edge fields must resolve to distinct ordinals.
	dpLeft, err := cs.Address("root.left")
	if err != nil {
		t.Fatalf("Address(root.left): %v", err)
	}
	dpRight, err := cs.Address("root.right")
	if err != nil {
		t.Fatalf("Address(root.right): %v", err)
	}
	if dpLeft.ID() == dpRight.ID() {
		t.Errorf("root.left and root.right got same ordinal %d — back-edge slots must be distinct",
			dpLeft.ID())
	}
	// Both must be in the back-region (> FrontSize).
	if dpLeft.ID() <= int32(as.FrontSize) {
		t.Errorf("root.left ordinal %d not in back-region (FrontSize=%d)",
			dpLeft.ID(), as.FrontSize)
	}
	if dpRight.ID() <= int32(as.FrontSize) {
		t.Errorf("root.right ordinal %d not in back-region (FrontSize=%d)",
			dpRight.ID(), as.FrontSize)
	}
}

// TestAddressSpace_MultipleCyclicTargets exercises block allocation when two
// independent cyclic target schemas exist (twoCycleSchema: P and Q each
// self-reference).
//
// Schema UUID lex order: ...0000a0 < ...0000b0 (a < b)
// → P is allocated first (higher block base), Q second (lower block base).
// → BlockBases[P] > BlockBases[Q]
// → Blocks are contiguous: BlockBases[P] - BlockSize[P] == BlockBases[Q]
// → No block overlaps FrontSize region: both BlockBases > FrontSize
func TestAddressSpace_MultipleCyclicTargets(t *testing.T) {
	cs := mustCompile(twoCycleSchema, nil)
	as := cs.AddressSpace

	var idxP, idxQ uint8
	for idx, m := range cs.Meta {
		if m == nil {
			continue
		}
		switch m.Name {
		case "P":
			idxP = uint8(idx)
		case "Q":
			idxQ = uint8(idx)
		}
	}
	if idxP == 0 || idxQ == 0 {
		t.Fatal("P or Q schema not found (index 0 is root)")
	}

	if as.BlockSize[idxP] != 2 {
		t.Errorf("BlockSize[P]: got %d, want 2", as.BlockSize[idxP])
	}
	if as.BlockSize[idxQ] != 2 {
		t.Errorf("BlockSize[Q]: got %d, want 2", as.BlockSize[idxQ])
	}

	// P's UUID sorts before Q's, so P gets the higher block base.
	if as.BlockBases[idxP] <= as.BlockBases[idxQ] {
		t.Errorf("BlockBases[P]=%d must be > BlockBases[Q]=%d (P UUID < Q UUID → allocated first)",
			as.BlockBases[idxP], as.BlockBases[idxQ])
	}

	// Blocks must be contiguous with no gap between them.
	if as.BlockBases[idxP]-as.BlockSize[idxP] != as.BlockBases[idxQ] {
		t.Errorf("blocks not contiguous: BlockBases[P](%d) - BlockSize[P](%d) = %d, want BlockBases[Q]=%d",
			as.BlockBases[idxP], as.BlockSize[idxP],
			as.BlockBases[idxP]-as.BlockSize[idxP],
			as.BlockBases[idxQ])
	}

	// Both blocks must be in the back-region.
	if as.BlockBases[idxP] <= as.FrontSize {
		t.Errorf("BlockBases[P]=%d must be > FrontSize=%d", as.BlockBases[idxP], as.FrontSize)
	}
	if as.BlockBases[idxQ] <= as.FrontSize {
		t.Errorf("BlockBases[Q]=%d must be > FrontSize=%d", as.BlockBases[idxQ], as.FrontSize)
	}
}
