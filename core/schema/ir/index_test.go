package ir

import (
	"strings"
	"testing"
)

// index_test.go tests Pass 2 (schema index assignment) and Pass 3 (field
// index assignment) in isolation via their internal functions.

// ── Pass 2: schema index ───────────────────────────────────────────────────

func TestBuildSchemaIndex_NoNestedSchemas(t *testing.T) {
	src := mustParse(flatSchema).inner
	si, errs := buildSchemaIndex(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	// Root is implicitly 0 and is not in byUUID.
	if len(si.byUUID) != 0 {
		t.Errorf("byUUID: got %d entries, want 0", len(si.byUUID))
	}
	if len(si.order) != 0 {
		t.Errorf("order: got %d entries, want 0", len(si.order))
	}
}

func TestBuildSchemaIndex_OneNestedSchema(t *testing.T) {
	src := mustParse(nestedObjectSchema).inner
	si, errs := buildSchemaIndex(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(si.byUUID) != 1 {
		t.Fatalf("byUUID: got %d entries, want 1", len(si.byUUID))
	}
	idx, ok := si.byUUID[nestedAddressSchemaUUID]
	if !ok {
		t.Fatalf("nested schema UUID not found in byUUID")
	}
	// Root is 0; first nested schema must be 1.
	if idx != 1 {
		t.Errorf("nested schema index: got %d, want 1", idx)
	}
}

func TestBuildSchemaIndex_MultipleNestedSchemasAreStable(t *testing.T) {
	src := mustParse(unionSchema).inner
	si, errs := buildSchemaIndex(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	// Two variant schemas + one union schema = 3 nested schemas... but
	// unionSchema only has two VariantA and VariantB schemas (the union field
	// itself lives in root). Verify both are indexed at 1 and 2 in lex order.
	if len(si.byUUID) != 2 {
		t.Fatalf("byUUID: got %d entries, want 2", len(si.byUUID))
	}
	idxA := si.byUUID[unionVariantAUUID]
	idxB := si.byUUID[unionVariantBUUID]
	// UUIDs: A = ...0040, B = ...0041 — A < B lexicographically → A=1, B=2.
	if idxA != 1 {
		t.Errorf("variantA index: got %d, want 1", idxA)
	}
	if idxB != 2 {
		t.Errorf("variantB index: got %d, want 2", idxB)
	}
	// order must be [A, B].
	if len(si.order) != 2 || si.order[0] != unionVariantAUUID || si.order[1] != unionVariantBUUID {
		t.Errorf("order: got %v", si.order)
	}
}

func TestBuildSchemaIndex_TooManySchemas(t *testing.T) {
	// Build a source with 128 nested schemas (one over the 127 limit).
	schemas := make(map[string]sourceNestedSchema, 128)
	for i := 0; i < 128; i++ {
		// Generate unique UUID-shaped keys.
		uuid := "019ca000-" + itoa(1000+i) + "-7000-0000-000000000000"
		schemas[uuid] = sourceNestedSchema{
			Name: "S" + itoa(i),
			Type: "enum",
		}
	}
	src := &sourceSchema{
		Name:    "Big",
		Version: "1.0.0",
		Schemas: schemas,
	}
	_, errs := buildSchemaIndex(src)
	if len(errs) == 0 {
		t.Fatal("expected error for too many schemas, got none")
	}
	if errs[0].Pass != PassSchemaIndex {
		t.Errorf("pass: got %v, want %v", errs[0].Pass, PassSchemaIndex)
	}
}

// ── Pass 3: field index ────────────────────────────────────────────────────

func TestBuildFieldIndex_LexOrder(t *testing.T) {
	src := mustParse(flatSchema).inner
	si, _ := buildSchemaIndex(src)
	fi, errs := buildFieldIndex(src, si)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	// UUIDs: 0001 < 0002 < 0003 → field_index 0, 1, 2 respectively.
	cases := []struct {
		uuid string
		want uint8
	}{
		{flatNameUUID, 0},
		{flatDescUUID, 1},
		{flatVersionUUID, 2},
	}
	for _, c := range cases {
		key := schemaFieldKey{0, c.uuid}
		got, ok := fi.bySchemaField[key]
		if !ok {
			t.Errorf("field UUID %s not found in bySchemaField", c.uuid)
			continue
		}
		if got != c.want {
			t.Errorf("field %s: got field_index %d, want %d", c.uuid, got, c.want)
		}
	}

	// byOrder[0] must be the sorted slice.
	order := fi.byOrder[0]
	if len(order) != 3 {
		t.Fatalf("byOrder[0]: got %d entries, want 3", len(order))
	}
	for i := 0; i < len(order)-1; i++ {
		if order[i] >= order[i+1] {
			t.Errorf("byOrder[0] not sorted at position %d: %q >= %q", i, order[i], order[i+1])
		}
	}
}

func TestBuildFieldIndex_NestedSchema(t *testing.T) {
	src := mustParse(nestedObjectSchema).inner
	si, _ := buildSchemaIndex(src)
	fi, errs := buildFieldIndex(src, si)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	schemaIdx := si.byUUID[nestedAddressSchemaUUID]
	// objStreetUUID (0011) < objCityUUID (0012) — street=0, city=1.
	streetIdx := fi.bySchemaField[schemaFieldKey{schemaIdx, objStreetUUID}]
	cityIdx := fi.bySchemaField[schemaFieldKey{schemaIdx, objCityUUID}]
	if streetIdx != 0 {
		t.Errorf("street field_index: got %d, want 0", streetIdx)
	}
	if cityIdx != 1 {
		t.Errorf("city field_index: got %d, want 1", cityIdx)
	}
}

func TestBuildFieldIndex_TooManyFields(t *testing.T) {
	fields := make(map[string]sourceField, 128)
	for i := 0; i < 128; i++ {
		uuid := "019ca000-" + itoa(1000+i) + "-7000-0000-000000000000"
		fields[uuid] = sourceField{Name: "f" + itoa(i), Type: "string"}
	}
	src := &sourceSchema{
		Name:    "Big",
		Version: "1.0.0",
		Fields:  fields,
	}
	si := &schemaIndex{byUUID: map[string]uint8{}, order: nil}
	_, errs := buildFieldIndex(src, si)
	if len(errs) == 0 {
		t.Fatal("expected error for too many fields, got none")
	}
	if errs[0].Pass != PassFieldIndex {
		t.Errorf("pass: got %v, want %v", errs[0].Pass, PassFieldIndex)
	}
	if !strings.Contains(errs[0].Message, "128") {
		t.Errorf("message should mention count, got: %q", errs[0].Message)
	}
}
