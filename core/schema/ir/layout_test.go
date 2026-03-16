package ir

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

// layout_test.go tests Pass 6 (Variants), Pass 7 (SchemaOffsets), and
// Pass 8 (Store) in isolation and via compiled output.

// ── Pass 6: Variants ───────────────────────────────────────────────────────

func TestVariants_UnionFieldHasVariants(t *testing.T) {
	cs := mustCompile(unionSchema, nil)

	payloadFd := findDescriptor(cs, 0, "payload")
	if payloadFd == 0 {
		t.Fatal("payload descriptor not found")
	}
	if ExtractType(payloadFd) != TypeUnion {
		t.Errorf("payload type: got %v, want TypeUnion", ExtractType(payloadFd))
	}

	variants, ok := cs.Variants[payloadFd]
	if !ok {
		t.Fatal("Variants entry not found for payload field")
	}
	if len(variants) != 2 {
		t.Errorf("variant count: got %d, want 2", len(variants))
	}
	// Variant indices must be 1 and 2 (A=1, B=2 in lex order).
	variantSet := map[uint8]bool{}
	for _, v := range variants {
		variantSet[v] = true
	}
	if !variantSet[1] || !variantSet[2] {
		t.Errorf("expected variant indices {1,2}, got %v", variants)
	}
}

func TestVariants_NonUnionFieldHasNoEntry(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	if len(cs.Variants) != 0 {
		t.Errorf("flatSchema should have empty Variants, got %d entries", len(cs.Variants))
	}
}

func TestVariants_NilForNonUnionCompositeTypes(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	addrFd := findDescriptor(cs, 0, "address")
	if _, ok := cs.Variants[addrFd]; ok {
		t.Error("object field should not have a Variants entry")
	}
}

// ── Pass 7: SchemaOffsets ─────────────────────────────────────────────────

func TestSchemaOffsets_FlatSchema(t *testing.T) {
	cs := mustCompile(flatSchema, nil)

	// flatSchema has only root schema (index 0), 3 fields.
	if len(cs.SchemaOffsets) != 1 {
		t.Fatalf("SchemaOffsets length: got %d, want 1", len(cs.SchemaOffsets))
	}

	start, end := descriptorRange(cs, 0)
	if start != 0 {
		t.Errorf("start: got %d, want 0", start)
	}
	if end != 3 {
		t.Errorf("end: got %d, want 3", end)
	}
	if end-start != len(cs.Descriptors) {
		t.Errorf("range width %d != total descriptors %d", end-start, len(cs.Descriptors))
	}
}

func TestSchemaOffsets_NestedObjectSchema(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)

	// Two schemas: root (1 field) at index 0, Address (2 fields) at index 1.
	if len(cs.SchemaOffsets) != 2 {
		t.Fatalf("SchemaOffsets length: got %d, want 2", len(cs.SchemaOffsets))
	}

	start0, end0 := descriptorRange(cs, 0)
	start1, end1 := descriptorRange(cs, 1)

	if end0-start0 != 1 {
		t.Errorf("root schema field count: got %d, want 1", end0-start0)
	}
	if end1-start1 != 2 {
		t.Errorf("address schema field count: got %d, want 2", end1-start1)
	}
	// Ranges must not overlap and must cover all descriptors contiguously.
	if end0 != start1 {
		t.Errorf("ranges not contiguous: root end=%d, address start=%d", end0, start1)
	}
	if end1 != len(cs.Descriptors) {
		t.Errorf("address end=%d != total descriptors=%d", end1, len(cs.Descriptors))
	}
}

func TestSchemaOffsets_TypeSchemasHaveZeroSentinel(t *testing.T) {
	// enumSchema has a root (1 field) + 1 enum type schema (no fields).
	cs := mustCompile(enumSchema, nil)

	if len(cs.SchemaOffsets) != 2 {
		t.Fatalf("SchemaOffsets length: got %d, want 2", len(cs.SchemaOffsets))
	}

	// The enum type schema (index 1) has no fields → zero sentinel.
	packed := cs.SchemaOffsets[1]
	start := uint16(packed)
	end := uint16(packed >> 16)
	if start != 0 || end != 0 {
		t.Errorf("type schema sentinel: got start=%d end=%d, want 0 0", start, end)
	}
}

func TestSchemaOffsets_TotalDescriptorsMatchSumOfRanges(t *testing.T) {
	cs := mustCompile(unionSchema, nil)

	total := 0
	for i, packed := range cs.SchemaOffsets {
		start := int(uint16(packed))
		end := int(uint16(packed >> 16))
		if start == 0 && end == 0 {
			continue // type schema sentinel
		}
		if start > end {
			t.Errorf("schema %d: start=%d > end=%d", i, start, end)
		}
		total += end - start
	}
	if total != len(cs.Descriptors) {
		t.Errorf("sum of ranges=%d != len(Descriptors)=%d", total, len(cs.Descriptors))
	}
}

// ── Pass 8: Store ─────────────────────────────────────────────────────────

func TestStore_NilForNoEnumsOrDefaults(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	if cs.Store != nil {
		t.Error("expected nil Store for schema with no enums or defaults")
	}
}

func TestStore_NamedEnumValuesPopulated(t *testing.T) {
	cs := mustCompile(enumSchema, nil)
	if cs.Store == nil {
		t.Fatal("Store should not be nil for schema with enum field")
	}

	statusFd := findDescriptor(cs, 0, "status")
	if statusFd == 0 {
		t.Fatal("status descriptor not found")
	}

	// Enum value sets are stored as TypeArrayString keyed by the descriptor's
	// identity bits with array DataType.
	id := int32((statusFd >> 8) & 0x7FFF)
	dp, err := document.NewDataPoint(document.TypeArrayString, id)
	if err != nil {
		t.Fatalf("NewDataPoint: %v", err)
	}

	vals, ok, err := cs.Store.GetArrayString(dp)
	if err != nil {
		t.Fatalf("GetArrayString: %v", err)
	}
	if !ok {
		t.Fatal("enum values not found in Store")
	}
	if len(vals) != 3 {
		t.Errorf("enum values count: got %d, want 3", len(vals))
	}
	want := map[string]bool{"pending": true, "active": true, "closed": true}
	for _, v := range vals {
		if !want[v] {
			t.Errorf("unexpected enum value: %q", v)
		}
	}
}

func TestStore_InlineEnumValuesPopulated(t *testing.T) {
	cs := mustCompile(inlineEnumSchema, nil)
	if cs.Store == nil {
		t.Fatal("Store should not be nil for schema with inline enum field")
	}

	priorityFd := findDescriptor(cs, 0, "priority")
	if priorityFd == 0 {
		t.Fatal("priority descriptor not found")
	}

	id := int32((priorityFd >> 8) & 0x7FFF)
	dp, err := document.NewDataPoint(document.TypeArrayString, id)
	if err != nil {
		t.Fatalf("NewDataPoint: %v", err)
	}
	vals, ok, err := cs.Store.GetArrayString(dp)
	if err != nil {
		t.Fatalf("GetArrayString: %v", err)
	}
	if !ok {
		t.Fatal("inline enum values not found in Store")
	}
	if len(vals) != 3 {
		t.Errorf("enum values count: got %d, want 3", len(vals))
	}
}

func TestStore_FieldDefaultPopulated(t *testing.T) {
	cs := mustCompile(defaultSchema, nil)
	if cs.Store == nil {
		t.Fatal("Store should not be nil for schema with field default")
	}

	retriesFd := findDescriptor(cs, 0, "retries")
	if retriesFd == 0 {
		t.Fatal("retries descriptor not found")
	}

	id := int32((retriesFd >> 8) & 0x7FFF)
	dp, err := document.NewDataPoint(document.TypeInt, id)
	if err != nil {
		t.Fatalf("NewDataPoint: %v", err)
	}
	val, ok, err := cs.Store.GetInt(dp)
	if err != nil {
		t.Fatalf("GetInt: %v", err)
	}
	if !ok {
		t.Fatal("default value not found in Store")
	}
	if val != 3 {
		t.Errorf("default value: got %d, want 3", val)
	}
}
