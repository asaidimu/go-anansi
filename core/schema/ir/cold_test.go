package ir

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

// cold_test.go tests Pass 9 (SchemaMetadata), Pass 10 (ResolvedIndexes),
// and Pass 11 (ResolvedConstraints).

// ── Pass 9: SchemaMetadata ─────────────────────────────────────────────────

func TestMeta_RootSchemaPresent(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	m, ok := cs.Meta[0]
	if !ok || m == nil {
		t.Fatal("Meta[0] not present for root schema")
	}
	if m.Name != "Flat" {
		t.Errorf("Name: got %q, want %q", m.Name, "Flat")
	}
	if m.Version != "1.0.0" {
		t.Errorf("Version: got %q, want %q", m.Version, "1.0.0")
	}
	if m.UUID != "" {
		t.Errorf("UUID: got %q, want empty", m.UUID)
	}
}

func TestMeta_NestedSchemaPresent(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	m, ok := cs.Meta[1]
	if !ok || m == nil {
		t.Fatal("Meta[1] not present for nested schema")
	}
	if m.Name != "Address" {
		t.Errorf("Name: got %q, want %q", m.Name, "Address")
	}
	if m.UUID != nestedAddressSchemaUUID {
		t.Errorf("UUID: got %q, want %q", m.UUID, nestedAddressSchemaUUID)
	}
}

func TestMeta_FieldsMapContainsAllFields(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	m := cs.Meta[0]
	if len(m.Fields) != 3 {
		t.Errorf("Fields count: got %d, want 3", len(m.Fields))
	}
	names := map[string]bool{}
	for _, fm := range m.Fields {
		names[fm.Name] = true
	}
	for _, want := range []string{"name", "desc", "version"} {
		if !names[want] {
			t.Errorf("field %q missing from Meta.Fields", want)
		}
	}
}

func TestMeta_FieldsMapKeyMatchesDescriptor(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	descriptorSet := map[uint32]bool{}
	for _, fd := range cs.Descriptors {
		descriptorSet[fd] = true
	}
	for schemaIdx, m := range cs.Meta {
		for fd := range m.Fields {
			if !descriptorSet[fd] {
				t.Errorf("Meta[%d].Fields key 0x%08X not in Descriptors", schemaIdx, fd)
			}
		}
	}
}

func TestMeta_IndexOrdinalsInitialised(t *testing.T) {
	cs := mustCompile(indexedSchema, nil)
	m := cs.Meta[0]
	if m.IndexOrdinals == nil {
		t.Fatal("IndexOrdinals is nil")
	}
}

func TestMeta_ColdIndexesForwardedCorrectly(t *testing.T) {
	cs := mustCompile(indexedSchema, nil)
	m := cs.Meta[0]
	cold, ok := m.Indexes["019ca000-0050-7000-0000-000000000050"]
	if !ok {
		t.Fatal("cold index not found in Meta.Indexes")
	}
	if cold.Name != "idx_sku" {
		t.Errorf("index name: got %q, want %q", cold.Name, "idx_sku")
	}
	if cold.Type != IndexTypeUnique {
		t.Errorf("index type: got %v, want IndexTypeUnique", cold.Type)
	}
	if len(cold.Fields) != 1 || cold.Fields[0] != "sku" {
		t.Errorf("index fields: got %v, want [sku]", cold.Fields)
	}
}

func TestMeta_ConstraintTreeBuilt(t *testing.T) {
	cs := mustCompileWithStubPredicate(constrainedSchema, "isEmail")
	m := cs.Meta[0]
	if m.Constraints == nil {
		t.Fatal("Constraints tree is nil for constrained schema")
	}
	if len(m.Constraints.Roots) != 1 {
		t.Errorf("constraint roots: got %d, want 1", len(m.Constraints.Roots))
	}
	if len(m.Constraints.Ordinals) == 0 {
		t.Error("Ordinals map is empty")
	}
}

// ── Pass 10: ResolvedIndexes ───────────────────────────────────────────────

func TestResolvedIndexes_EmptyForNoIndexes(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	if len(cs.ResolvedIndexes) != 0 {
		t.Errorf("ResolvedIndexes: got %d entries, want 0", len(cs.ResolvedIndexes))
	}
}

func TestResolvedIndexes_KeyPackingCorrect(t *testing.T) {
	// indexedSchema has one index on root schema (index 0), ordinal 0.
	// Expected key = (0<<8) | 0 = 0.
	cs := mustCompile(indexedSchema, nil)
	if len(cs.ResolvedIndexes) != 1 {
		t.Fatalf("ResolvedIndexes: got %d entries, want 1", len(cs.ResolvedIndexes))
	}
	key := uint16(0)<<8 | uint16(0)
	ri, ok := cs.ResolvedIndexes[key]
	if !ok {
		t.Fatal("ResolvedIndexes key 0 not found")
	}
	if ri.Type != IndexTypeUnique {
		t.Errorf("ResolvedIndex type: got %v, want IndexTypeUnique", ri.Type)
	}
}

func TestResolvedIndexes_OrdinalWrittenToMeta(t *testing.T) {
	cs := mustCompile(indexedSchema, nil)
	m := cs.Meta[0]
	ordinal, ok := m.IndexOrdinals["019ca000-0050-7000-0000-000000000050"]
	if !ok {
		t.Fatal("index UUID not found in IndexOrdinals")
	}
	if ordinal != 0 {
		t.Errorf("ordinal: got %d, want 0", ordinal)
	}
}

// ── Pass 11: ResolvedConstraints ───────────────────────────────────────────

func TestResolvedConstraints_EmptyForNoConstraints(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	if len(cs.ResolvedConstraints) != 0 {
		t.Errorf("ResolvedConstraints: got %d entries, want 0", len(cs.ResolvedConstraints))
	}
}

func TestResolvedConstraints_UnknownPredicateIsError(t *testing.T) {
	ss := mustParse(constrainedSchema)
	_, err := Compile(ss, PredicateMap{})
	if err == nil {
		t.Fatal("expected error for unknown predicate")
	}
	ce := firstError(err)
	if ce.Pass != PassConstraints {
		t.Errorf("pass: got %v, want %v", ce.Pass, PassConstraints)
	}
}

func TestResolvedConstraints_KnownPredicateSucceeds(t *testing.T) {
	cs := mustCompileWithStubPredicate(constrainedSchema, "isEmail")
	if len(cs.ResolvedConstraints) != 1 {
		t.Fatalf("ResolvedConstraints: got %d entries, want 1", len(cs.ResolvedConstraints))
	}
	rt, ok := cs.ResolvedConstraints[0]
	if !ok || rt == nil {
		t.Fatal("ResolvedConstraints[0] not found")
	}
	if len(rt.Roots) != 1 {
		t.Errorf("resolved roots: got %d, want 1", len(rt.Roots))
	}
}

func TestResolvedConstraints_IndexMatchesOrdinals(t *testing.T) {
	cs := mustCompileWithStubPredicate(constrainedSchema, "isEmail")
	rt := cs.ResolvedConstraints[0]
	m := cs.Meta[0]
	for uuid, ordinal := range m.Constraints.Ordinals {
		if _, ok := rt.Index[ordinal]; !ok {
			t.Errorf("ordinal %d for UUID %s has no entry in ResolvedConstraintTree.Index", ordinal, uuid)
		}
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

// mustCompileWithStubPredicate compiles src with a no-op Predicate registered
// under each given name. Panics on compile error.
func mustCompileWithStubPredicate(src []byte, names ...string) *CompiledSchema {
	pm := PredicateMap{}
	for _, name := range names {
		pm[name] = func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool {
			return true
		}
	}
	ss := mustParse(src)
	cs, err := Compile(ss, pm)
	if err != nil {
		panic("mustCompileWithStubPredicate: " + err.Error())
	}
	return cs
}
