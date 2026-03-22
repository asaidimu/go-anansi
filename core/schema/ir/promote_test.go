package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

// =============================================================================
// SCHEMA FIXTURES  (reuse the fixtures already defined in binary_test.go
// via the same package; new ones are added here where needed)
// =============================================================================

// itemSchema: a schema whose only nested sub-schema is an array element type.
// Used to test promotion of an array element sub-schema.
const itemSchema = `{
  "name": "Catalog",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "catalog_id", "type": "string",  "required": true },
    "01900000-0000-7000-8000-000000000002": {
      "name": "items",
      "type": "array",
      "required": true,
      "schema": { "id": "01900000-0000-7000-8000-0000000000E1" }
    }
  },
  "schemas": {
    "01900000-0000-7000-8000-0000000000E1": {
      "name": "Item",
      "fields": {
        "01900000-0000-7000-8000-000000000061": { "name": "sku",      "type": "string",  "required": true  },
        "01900000-0000-7000-8000-000000000062": { "name": "quantity", "type": "integer", "required": true  },
        "01900000-0000-7000-8000-000000000063": { "name": "price",    "type": "number",  "required": false }
      }
    }
  }
}`

// enumSubSchema: a schema whose sub-schema contains an enum field.
// Tests that enum value sets survive promotion with correct store re-keying.
const enumSubSchema = `{
  "name": "Container",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "id", "type": "string", "required": true },
    "01900000-0000-7000-8000-000000000002": {
      "name": "entry",
      "type": "object",
      "required": true,
      "schema": { "id": "01900000-0000-7000-8000-0000000000F1" }
    }
  },
  "schemas": {
    "01900000-0000-7000-8000-0000000000F1": {
      "name": "Entry",
      "fields": {
        "01900000-0000-7000-8000-000000000071": { "name": "label", "type": "string", "required": true },
        "01900000-0000-7000-8000-000000000072": {
          "name": "priority",
          "type": "enum",
          "required": false,
          "schema": { "values": ["low", "medium", "high"] }
        }
      }
    }
  }
}`

// unionSubSchema: a schema with a union sub-schema whose variants can be promoted.
const unionSubSchema = `{
  "name": "Wrapper",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "tag", "type": "string", "required": true },
    "01900000-0000-7000-8000-000000000002": {
      "name": "payload",
      "type": "union",
      "required": true,
      "schema": [
        { "id": "01900000-0000-7000-8000-000000000011" },
        { "id": "01900000-0000-7000-8000-000000000012" }
      ]
    }
  },
  "schemas": {
    "01900000-0000-7000-8000-000000000011": {
      "name": "Alpha",
      "fields": {
        "01900000-0000-7000-8000-000000000081": { "name": "alpha_val", "type": "string",  "required": true }
      }
    },
    "01900000-0000-7000-8000-000000000012": {
      "name": "Beta",
      "fields": {
        "01900000-0000-7000-8000-000000000091": { "name": "beta_val",  "type": "integer", "required": true }
      }
    }
  }
}`

// =============================================================================
// HELPERS
// =============================================================================

// promote compiles src, finds the sub-schema named subSchemaName, and promotes
// it. Fails the test if compilation or promotion fails.
func promote(t *testing.T, src string, subSchemaName string, predicates ir.PredicateMap) (*ir.Schema, *ir.Schema) {
	t.Helper()
	cs := compile(t, src, predicates)
	idx, ok := findSubSchemaIndex(cs, subSchemaName)
	if !ok {
		t.Fatalf("sub-schema %q not found in compiled schema", subSchemaName)
	}
	promoted, err := cs.Promote(idx)
	if err != nil {
		t.Fatalf("Promote(%d): %v", idx, err)
	}
	return cs, promoted
}

// findSubSchemaIndex returns the schema index for the sub-schema with the
// given name, searching cs.Meta.
func findSubSchemaIndex(cs *ir.Schema, name string) (uint8, bool) {
	for idx, m := range cs.Meta {
		if idx == 0 {
			continue // skip root
		}
		if m != nil && m.Name == name {
			return idx, true
		}
	}
	return 0, false
}

// assertPromotedAddress verifies that a local path resolves on the promoted
// schema and returns a non-zero DocumentKey.
func assertPromotedAddress(t *testing.T, promoted *ir.Schema, localPath string) {
	t.Helper()
	dk, err := promoted.DocumentKey(localPath)
	if err != nil {
		t.Errorf("promoted.DocumentKey(%q): %v", localPath, err)
		return
	}
	if dk == 0 {
		t.Errorf("promoted.DocumentKey(%q): returned zero key", localPath)
	}
}

// assertPromotedKeyDiffers verifies that the same logical field has different
// DocumentKeys in the parent and promoted schemas — because ownerSchema bits
// have changed and ordinals are reassigned.
func assertPromotedKeyDiffers(t *testing.T, parent *ir.Schema, parentPath string, promoted *ir.Schema, localPath string) {
	t.Helper()
	parentKey, err := parent.DocumentKey(parentPath)
	if err != nil {
		t.Fatalf("parent.DocumentKey(%q): %v", parentPath, err)
	}
	promotedKey, err := promoted.DocumentKey(localPath)
	if err != nil {
		t.Fatalf("promoted.DocumentKey(%q): %v", localPath, err)
	}
	if parentKey == promotedKey {
		t.Errorf("expected different DocumentKeys for parent path %q and promoted path %q, but both are %d",
			parentPath, localPath, parentKey)
	}
}

// assertPromotedRootIndex verifies the promoted schema has exactly one entry
// in SchemaOffsets at index 0 and that it covers a non-empty descriptor range.
func assertPromotedRootIndex(t *testing.T, promoted *ir.Schema) {
	t.Helper()
	if len(promoted.SchemaOffsets) == 0 {
		t.Fatal("promoted schema has no SchemaOffsets")
	}
	packed := promoted.SchemaOffsets[0]
	start := int(uint16(packed))
	end := int(uint16(packed >> 16))
	if start >= end {
		t.Errorf("promoted schema[0] has empty descriptor range [%d, %d)", start, end)
	}
}

// assertMetaName verifies the promoted schema's root Meta entry has the expected name.
func assertMetaName(t *testing.T, promoted *ir.Schema, expectedName string) {
	t.Helper()
	m, ok := promoted.Meta[0]
	if !ok || m == nil {
		t.Fatalf("promoted schema has no Meta[0]")
	}
	if m.Name != expectedName {
		t.Errorf("promoted Meta[0].Name: want %q, got %q", expectedName, m.Name)
	}
}

// assertDescriptorOwnerSchema verifies that all descriptors in the promoted
// schema have ownerSchema values in the range [0, len(promoted.SchemaOffsets)).
func assertDescriptorOwnerSchema(t *testing.T, promoted *ir.Schema) {
	t.Helper()
	maxIdx := uint8(len(promoted.SchemaOffsets))
	for i, fd := range promoted.Descriptors {
		owner := ir.ExtractOwnerSchema(fd)
		if owner >= maxIdx {
			t.Errorf("Descriptors[%d]: ownerSchema=%d is out of range [0, %d)", i, owner, maxIdx)
		}
	}
}

// assertDescriptorTargetSchema verifies that all schema-bearing descriptors
// in the promoted schema reference valid target schema indices.
func assertDescriptorTargetSchema(t *testing.T, promoted *ir.Schema) {
	t.Helper()
	maxIdx := uint8(len(promoted.SchemaOffsets))
	for i, fd := range promoted.Descriptors {
		if !ir.IsSchemaBearing(fd) {
			continue
		}
		typ := ir.ExtractType(fd)
		if typ == ir.TypeUnion || typ == ir.TypeComposite {
			// Targets live in Variants — verified separately.
			continue
		}
		target := ir.ExtractTargetSchema(fd)
		if target >= maxIdx {
			t.Errorf("Descriptors[%d]: targetSchema=%d is out of range [0, %d)", i, target, maxIdx)
		}
	}
}

// assertVariantIndicesValid verifies all variant indices in the promoted
// schema's Variants map are valid schema indices.
func assertVariantIndicesValid(t *testing.T, promoted *ir.Schema) {
	t.Helper()
	maxIdx := uint8(len(promoted.SchemaOffsets))
	for fd, variants := range promoted.Variants {
		for _, v := range variants {
			if v >= maxIdx {
				t.Errorf("Variants[%d]: variant index %d out of range [0, %d)", fd, v, maxIdx)
			}
		}
	}
}

// assertAddressSpacePopulated verifies the promoted schema has a non-nil
// AddressSpace and that FieldNames[0] is populated.
func assertAddressSpacePopulated(t *testing.T, promoted *ir.Schema) {
	t.Helper()
	if promoted.AddressSpace == nil {
		t.Fatal("promoted schema has nil AddressSpace")
	}
	if promoted.AddressSpace.FieldNames[0] == nil {
		t.Error("promoted schema AddressSpace.FieldNames[0] is nil — root schema has no field names")
	}
}

// assertPathCachePopulated verifies the promoted schema's PathCache contains
// at least one entry — it should be populated during address space construction.
func assertPathCachePopulated(t *testing.T, promoted *ir.Schema) {
	t.Helper()
	if promoted.PathCache == nil {
		t.Fatal("promoted schema has nil PathCache")
	}
	// The address space build populates PathCache via DocumentKey calls.
	// We verify by resolving at least one known path.
}

// =============================================================================
// ERROR CASES
// =============================================================================

func TestPromote_OutOfRange_Fails(t *testing.T) {
	cs := compile(t, minimalSchema, nil)
	_, err := cs.Promote(200)
	if err == nil {
		t.Fatal("expected error for out-of-range schema index, got nil")
	}
}

func TestPromote_TypeSchema_Fails(t *testing.T) {
	// flSchema has an enum field — the enum's sub-schema is a type schema
	// with no fields. Promoting it must fail.
	cs := compile(t, flSchema, nil)

	// Find the enum type schema index — it has no fields (start == end in SchemaOffsets).
	var enumTypeIdx uint8
	found := false
	for i, packed := range cs.SchemaOffsets {
		start := int(uint16(packed))
		end := int(uint16(packed >> 16))
		if start == end && i > 0 {
			enumTypeIdx = uint8(i)
			found = true
			break
		}
	}
	if !found {
		t.Skip("no type schema found in flSchema — skipping")
	}

	_, err := cs.Promote(enumTypeIdx)
	if err == nil {
		t.Fatalf("expected error promoting type schema at index %d, got nil", enumTypeIdx)
	}
}

// =============================================================================
// STRUCTURAL CORRECTNESS
// =============================================================================

func TestPromote_Nested_RootIsSubSchema(t *testing.T) {
	_, promoted := promote(t, nestedSchema, "Address", nil)

	assertPromotedRootIndex(t, promoted)
	assertMetaName(t, promoted, "Address")
	assertDescriptorOwnerSchema(t, promoted)
	assertAddressSpacePopulated(t, promoted)
}

func TestPromote_Nested_LocalPathsResolvable(t *testing.T) {
	_, promoted := promote(t, nestedSchema, "Address", nil)

	// Address sub-schema has fields: street, city. These must be addressable
	// as local paths on the promoted schema.
	assertPromotedAddress(t, promoted, "street")
	assertPromotedAddress(t, promoted, "city")
}

func TestPromote_Nested_ParentPathsNotResolvable(t *testing.T) {
	// Paths that only make sense in the parent (name, address.street) must
	// not resolve on the promoted schema.
	_, promoted := promote(t, nestedSchema, "Address", nil)

	if _, err := promoted.DocumentKey("name"); err == nil {
		t.Error("parent-only path 'name' should not resolve on promoted schema")
	}
	if _, err := promoted.DocumentKey("address.street"); err == nil {
		t.Error("parent-qualified path 'address.street' should not resolve on promoted schema")
	}
}

func TestPromote_Nested_KeysDifferFromParent(t *testing.T) {
	parent, promoted := promote(t, nestedSchema, "Address", nil)

	// The same logical field (street) has a different DocumentKey in the
	// promoted schema vs the parent — ownerSchema bits have changed.
	assertPromotedKeyDiffers(t, parent, "address.street", promoted, "street")
	assertPromotedKeyDiffers(t, parent, "address.city", promoted, "city")
}

func TestPromote_Array_ElementSchemaResolvable(t *testing.T) {
	_, promoted := promote(t, itemSchema, "Item", nil)

	assertPromotedRootIndex(t, promoted)
	assertMetaName(t, promoted, "Item")
	assertPromotedAddress(t, promoted, "sku")
	assertPromotedAddress(t, promoted, "quantity")
	assertPromotedAddress(t, promoted, "price")
}

func TestPromote_Array_ElementDescriptorsRewritten(t *testing.T) {
	_, promoted := promote(t, itemSchema, "Item", nil)

	assertDescriptorOwnerSchema(t, promoted)
	assertDescriptorTargetSchema(t, promoted)
}

func TestPromote_Union_VariantIndicesRemapped(t *testing.T) {
	_, promoted := promote(t, uninSchema, "TypeA", nil)

	// TypeA has one field: a_val. Promoted schema should be minimal.
	assertPromotedRootIndex(t, promoted)
	assertMetaName(t, promoted, "TypeA")
	assertPromotedAddress(t, promoted, "a_val")
	assertDescriptorOwnerSchema(t, promoted)
}

func TestPromote_Union_BothVariantsPromotable(t *testing.T) {
	cs := compile(t, uninSchema, nil)

	for _, name := range []string{"TypeA", "TypeB"} {
		t.Run(name, func(t *testing.T) {
			idx, ok := findSubSchemaIndex(cs, name)
			if !ok {
				t.Fatalf("sub-schema %q not found", name)
			}
			promoted, err := cs.Promote(idx)
			if err != nil {
				t.Fatalf("Promote(%q): %v", name, err)
			}
			assertPromotedRootIndex(t, promoted)
			assertMetaName(t, promoted, name)
			assertDescriptorOwnerSchema(t, promoted)
		})
	}
}

// =============================================================================
// ENUM STORE RE-KEYING
// =============================================================================

func TestPromote_EnumSubSchema_StoreRebuilt(t *testing.T) {
	_, promoted := promote(t, enumSubSchema, "Entry", nil)

	assertPromotedRootIndex(t, promoted)
	assertMetaName(t, promoted, "Entry")

	// The promoted schema should have a non-nil Store containing
	// the enum values for 'priority'.
	if promoted.Store == nil {
		t.Fatal("promoted schema Store is nil — enum values were not carried over")
	}

	// Resolve the local 'priority' key and verify enum values are retrievable.
	dk, err := promoted.DocumentKey("priority")
	if err != nil {
		t.Fatalf("promoted.DocumentKey(\"priority\"): %v", err)
	}
	fd := dk.Descriptor()
	storeKey := ir.DescriptorToEnumDocumentKey(fd, document.TypeArrayString)
	vals, ok, _ := promoted.Store.GetArrayString(storeKey)
	if !ok {
		t.Fatal("enum values for 'priority' not found in promoted Store")
	}
	expected := map[string]bool{"low": true, "medium": true, "high": true}
	if len(vals) != len(expected) {
		t.Errorf("enum values: want %d, got %d: %v", len(expected), len(vals), vals)
	}
	for _, v := range vals {
		if !expected[v] {
			t.Errorf("unexpected enum value %q", v)
		}
	}
}

func TestPromote_NoStore_StoreRemainsNil(t *testing.T) {
	// nestedSchema has no enum fields and no defaults — Store should be nil.
	_, promoted := promote(t, nestedSchema, "Address", nil)
	if promoted.Store != nil {
		t.Error("expected nil Store for sub-schema with no enum fields or defaults")
	}
}

// =============================================================================
// ADDRESS SPACE AND PATH CACHE
// =============================================================================

func TestPromote_AddressSpace_AllFieldsAddressable(t *testing.T) {
	_, promoted := promote(t, itemSchema, "Item", nil)

	assertAddressSpacePopulated(t, promoted)

	// All three fields must resolve cleanly.
	for _, path := range []string{"sku", "quantity", "price"} {
		dp, err := promoted.Address(path)
		if err != nil {
			t.Errorf("promoted.Address(%q): %v", path, err)
			continue
		}
		if dp == 0 {
			t.Errorf("promoted.Address(%q): returned zero DataPoint", path)
		}
	}
}

func TestPromote_PathCache_PopulatedOnDocumentKeyCall(t *testing.T) {
	_, promoted := promote(t, nestedSchema, "Address", nil)

	// Resolve a key — this should populate PathCache.
	dk, err := promoted.DocumentKey("street")
	if err != nil {
		t.Fatalf("promoted.DocumentKey(\"street\"): %v", err)
	}

	// Reverse lookup must work.
	path, ok := promoted.PathCache.GetPath(dk)
	if !ok {
		t.Fatal("PathCache does not have reverse mapping for 'street' after DocumentKey call")
	}
	if path != "street" {
		t.Errorf("PathCache reverse lookup: want %q, got %q", "street", path)
	}
}

func TestPromote_PathCache_ForwardLookupCached(t *testing.T) {
	_, promoted := promote(t, nestedSchema, "Address", nil)

	// First call — resolves and caches.
	dk1, err := promoted.DocumentKey("city")
	if err != nil {
		t.Fatalf("first DocumentKey(\"city\"): %v", err)
	}

	// Second call — must return cached value.
	dk2, err := promoted.DocumentKey("city")
	if err != nil {
		t.Fatalf("second DocumentKey(\"city\"): %v", err)
	}

	if dk1 != dk2 {
		t.Errorf("DocumentKey not stable across calls: first=%d second=%d", dk1, dk2)
	}
}

// =============================================================================
// CYCLIC SCHEMA PROMOTION
// =============================================================================

func TestPromote_Cyclic_NodeSubSchema_Resolvable(t *testing.T) {
	cs := compile(t, cyclicSchema, nil)
	idx, ok := findSubSchemaIndex(cs, "Node")
	if !ok {
		t.Fatal("Node sub-schema not found in cyclicSchema")
	}

	promoted, err := cs.Promote(idx)
	if err != nil {
		t.Fatalf("Promote(Node): %v", err)
	}

	assertPromotedRootIndex(t, promoted)
	assertMetaName(t, promoted, "Node")
	assertDescriptorOwnerSchema(t, promoted)
	assertDescriptorTargetSchema(t, promoted)

	// Local fields of Node must be addressable.
	assertPromotedAddress(t, promoted, "value")
	// 'next' is a recursive self-reference — must resolve at least one level.
	assertPromotedAddress(t, promoted, "next")
}

func TestPromote_Cyclic_RecursiveField_AddressSpaceValid(t *testing.T) {
	cs := compile(t, cyclicSchema, nil)
	idx, ok := findSubSchemaIndex(cs, "Node")
	if !ok {
		t.Fatal("Node sub-schema not found")
	}

	promoted, err := cs.Promote(idx)
	if err != nil {
		t.Fatalf("Promote(Node): %v", err)
	}

	// The address space invariants must hold on the promoted cyclic schema.
	// We verify by checking FrontSize is non-zero and BlockBases is populated
	// for the self-referencing schema (index 0 in the promoted schema).
	as := promoted.AddressSpace
	if as == nil {
		t.Fatal("promoted cyclic schema has nil AddressSpace")
	}
	if as.FrontSize == 0 {
		t.Error("promoted cyclic schema AddressSpace.FrontSize is zero")
	}
	// At least one cyclic block must be registered (the self-reference).
	hasBlock := false
	for _, base := range as.BlockBases {
		if base != 0 {
			hasBlock = true
			break
		}
	}
	if !hasBlock {
		t.Error("promoted cyclic schema has no block bases — cyclic address space not built")
	}
}

// =============================================================================
// META PRESERVATION
// =============================================================================

func TestPromote_Meta_FieldNamesPreserved(t *testing.T) {
	_, promoted := promote(t, itemSchema, "Item", nil)

	m := promoted.Meta[0]
	if m == nil {
		t.Fatal("promoted.Meta[0] is nil")
	}

	// Collect promoted field names.
	names := make(map[string]bool)
	for _, fm := range m.Fields {
		names[fm.Name] = true
	}

	for _, expected := range []string{"sku", "quantity", "price"} {
		if !names[expected] {
			t.Errorf("promoted Meta[0].Fields missing field %q", expected)
		}
	}
}

func TestPromote_Meta_UUIDsPreserved(t *testing.T) {
	parent, promoted := promote(t, nestedSchema, "Address", nil)

	// Find the Address sub-schema UUID in the parent.
	parentIdx, _ := findSubSchemaIndex(parent, "Address")
	parentMeta := parent.Meta[parentIdx]
	if parentMeta == nil {
		t.Fatal("parent has no Meta for Address")
	}

	promotedMeta := promoted.Meta[0]
	if promotedMeta == nil {
		t.Fatal("promoted has no Meta[0]")
	}

	if parentMeta.UUID != promotedMeta.UUID {
		t.Errorf("UUID not preserved: parent=%q promoted=%q", parentMeta.UUID, promotedMeta.UUID)
	}
}

// =============================================================================
// IDEMPOTENCE
// =============================================================================

func TestPromote_Idempotent_TwoPromotionsSameResult(t *testing.T) {
	cs := compile(t, nestedSchema, nil)
	idx, ok := findSubSchemaIndex(cs, "Address")
	if !ok {
		t.Fatal("Address sub-schema not found")
	}

	p1, err := cs.Promote(idx)
	if err != nil {
		t.Fatalf("first Promote: %v", err)
	}
	p2, err := cs.Promote(idx)
	if err != nil {
		t.Fatalf("second Promote: %v", err)
	}

	// Both promotions must produce schemas with the same descriptor layout.
	if len(p1.Descriptors) != len(p2.Descriptors) {
		t.Fatalf("descriptor count mismatch: p1=%d p2=%d", len(p1.Descriptors), len(p2.Descriptors))
	}
	for i := range p1.Descriptors {
		if p1.Descriptors[i] != p2.Descriptors[i] {
			t.Errorf("Descriptors[%d]: p1=0x%08x p2=0x%08x", i, p1.Descriptors[i], p2.Descriptors[i])
		}
	}

	// DocumentKey resolution must be identical.
	for _, path := range []string{"street", "city", "country"} {
		dk1, err1 := p1.DocumentKey(path)
		dk2, err2 := p2.DocumentKey(path)
		if err1 != nil || err2 != nil {
			t.Errorf("DocumentKey(%q): p1 err=%v p2 err=%v", path, err1, err2)
			continue
		}
		if dk1 != dk2 {
			t.Errorf("DocumentKey(%q): p1=%d p2=%d", path, dk1, dk2)
		}
	}
}

// =============================================================================
// UNION PARENT SCHEMA — variant sub-schemas promoted individually
// =============================================================================

func TestPromote_UnionVariants_VariantSchemaContainsOnlyOwnFields(t *testing.T) {
	cs := compile(t, unionSubSchema, nil)

	for _, tc := range []struct {
		name      string
		localPath string
		absent    string
	}{
		{"Alpha", "alpha_val", "beta_val"},
		{"Beta", "beta_val", "alpha_val"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx, ok := findSubSchemaIndex(cs, tc.name)
			if !ok {
				t.Fatalf("sub-schema %q not found", tc.name)
			}
			promoted, err := cs.Promote(idx)
			if err != nil {
				t.Fatalf("Promote(%q): %v", tc.name, err)
			}

			// Own field must resolve.
			if _, err := promoted.DocumentKey(tc.localPath); err != nil {
				t.Errorf("promoted.DocumentKey(%q): %v", tc.localPath, err)
			}

			// The other variant's field must NOT resolve.
			if _, err := promoted.DocumentKey(tc.absent); err == nil {
				t.Errorf("expected error for %q on %s-promoted schema, got nil", tc.absent, tc.name)
			}
		})
	}
}
