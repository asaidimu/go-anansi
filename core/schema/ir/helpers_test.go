package ir

import (
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

// helpers_test.go tests the IR helper functions: DescriptorToDataPoint,
// IsSchemaBearing, Extract*, TerminalWalk, FullWalk, CompileError formatting.

// ── DescriptorToDataPoint ──────────────────────────────────────────────────

func TestDescriptorToDataPoint_IdEncoding(t *testing.T) {
	// owner_schema=2, field_index=3 → id = (2<<7)|3 = 259
	fd := packDescriptor(TypeString, 2, 3, 0, false, false, false)
	dp := DescriptorToDataPoint(fd)

	wantID := int32((fd >> 8) & 0x7FFF)
	if dp.ID() != wantID {
		t.Errorf("ID: got %d, want %d", dp.ID(), wantID)
	}
	if dp.Type() != document.TypeString {
		t.Errorf("Type: got %v, want TypeString", dp.Type())
	}
}

func TestDescriptorToDataPoint_TypeMapping(t *testing.T) {
	cases := []struct {
		ft   FieldTypeEnum
		want document.DataType
	}{
		{TypeString, document.TypeString},
		{TypeNumber, document.TypeFloat},
		{TypeInteger, document.TypeInt},
		{TypeBoolean, document.TypeBool},
		{TypeBytes, document.TypeBytes},
		{TypeGeometry, document.TypeGeometry},
		{TypeRecord, document.TypeRecord},
		{TypeObject, document.TypeRecord},
		{TypeUnknown, document.TypeUnknown},
	}
	for _, c := range cases {
		fd := packDescriptor(c.ft, 0, 0, 0, false, false, false)
		dp := DescriptorToDataPoint(fd)
		if dp.Type() != c.want {
			t.Errorf("FieldType %v: got DataType %v, want %v", c.ft, dp.Type(), c.want)
		}
	}
}

func TestDescriptorToDataPoint_IgnoresNonIdentityBits(t *testing.T) {
	// terminal, required, unique, deprecated should not affect the DataPoint.
	base := packDescriptor(TypeString, 1, 2, 0, false, false, false)
	withFlags := packDescriptor(TypeString, 1, 2, 0, true, true, true) | FDMaskTerminal

	dpBase := DescriptorToDataPoint(base)
	dpFlags := DescriptorToDataPoint(withFlags)

	if dpBase.ID() != dpFlags.ID() {
		t.Errorf("ID changed with flag bits: base=%d, flags=%d", dpBase.ID(), dpFlags.ID())
	}
}

// ── IsSchemaBearing ────────────────────────────────────────────────────────

func TestIsSchemaBearing(t *testing.T) {
	bearingTypes := []FieldTypeEnum{
		TypeArray, TypeSet, TypeEnum, TypeObject, TypeRecord, TypeUnion, TypeComposite,
	}
	nonBearingTypes := []FieldTypeEnum{
		TypeUnknown, TypeString, TypeNumber, TypeInteger, TypeBoolean, TypeBytes, TypeGeometry,
	}
	for _, ft := range bearingTypes {
		fd := packDescriptor(ft, 0, 0, 1, false, false, false)
		if !IsSchemaBearing(fd) {
			t.Errorf("type %v: expected schema-bearing", ft)
		}
	}
	for _, ft := range nonBearingTypes {
		fd := packDescriptor(ft, 0, 0, 0, false, false, false)
		if IsSchemaBearing(fd) {
			t.Errorf("type %v: expected not schema-bearing", ft)
		}
	}
}

// ── TerminalWalk ───────────────────────────────────────────────────────────

func TestTerminalWalk_FlatSchema(t *testing.T) {
	cs := mustCompile(flatSchema, nil)
	var visited []uint32
	TerminalWalk(cs, 0, func(fd uint32) {
		visited = append(visited, fd)
	})
	if len(visited) != 3 {
		t.Errorf("TerminalWalk visited %d fields, want 3", len(visited))
	}
}

func TestTerminalWalk_DescendsIntoTerminalObject(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	var visited []uint32
	TerminalWalk(cs, 0, func(fd uint32) {
		visited = append(visited, fd)
	})
	// Root has 1 field (address), Address has 2 fields (street, city) = 3 total.
	if len(visited) != 3 {
		t.Errorf("TerminalWalk visited %d fields, want 3", len(visited))
	}
}

func TestTerminalWalk_StopsAtCycle(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)
	var visited []uint32
	TerminalWalk(cs, 0, func(fd uint32) {
		visited = append(visited, fd)
	})
	// Root: label (terminal scalar), node (terminal → descend into Node).
	// Node: value (terminal scalar), next (non-terminal → visit but don't descend).
	// Total visited = 4. No infinite loop.
	if len(visited) != 4 {
		t.Errorf("TerminalWalk visited %d fields, want 4", len(visited))
	}
}

func TestTerminalWalk_NoAllocation(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	// Warm up once.
	noop := func(_ uint32) {}
	TerminalWalk(cs, 0, noop)

	// TerminalWalk must not allocate on the hot path. The visitor is defined
	// outside AllocsPerRun so its closure allocation is not counted.
	allocs := testing.AllocsPerRun(100, func() {
		TerminalWalk(cs, 0, noop)
	})
	if allocs > 0 {
		t.Errorf("TerminalWalk allocated %.0f times, want 0", allocs)
	}
}

// ── FullWalk ───────────────────────────────────────────────────────────────

func TestFullWalk_VisitsEachSchemaOnce(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	schemasSeen := map[uint8]int{}
	FullWalk(cs, 0, func(fd uint32) {
		schemasSeen[ExtractOwnerSchema(fd)]++
	})
	// Schema 0: 1 field. Schema 1: 2 fields.
	if schemasSeen[0] != 1 {
		t.Errorf("schema 0 visited %d times, want 1", schemasSeen[0])
	}
	if schemasSeen[1] != 2 {
		t.Errorf("schema 1 visited %d times, want 2", schemasSeen[1])
	}
}

func TestFullWalk_HandlesUnionVariants(t *testing.T) {
	cs := mustCompile(unionSchema, nil)
	var visited []uint32
	FullWalk(cs, 0, func(fd uint32) {
		visited = append(visited, fd)
	})
	// Root: 1 field (payload). VariantA: 1 field. VariantB: 1 field. = 3 total.
	if len(visited) != 3 {
		t.Errorf("FullWalk visited %d fields, want 3", len(visited))
	}
}

func TestFullWalk_CycleSafe(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)
	var visited []uint32
	FullWalk(cs, 0, func(fd uint32) {
		visited = append(visited, fd)
	})
	// Schema 0: label, node. Schema 1 (Node): value, next. = 4 total.
	// FullWalk visits each schema exactly once regardless of cycles.
	if len(visited) != 4 {
		t.Errorf("FullWalk visited %d fields, want 4", len(visited))
	}
}

func TestFullWalk_NoAllocation(t *testing.T) {
	cs := mustCompile(nestedObjectSchema, nil)
	noop := func(_ uint32) {}
	FullWalk(cs, 0, noop)

	allocs := testing.AllocsPerRun(100, func() {
		FullWalk(cs, 0, noop)
	})
	if allocs > 0 {
		t.Errorf("FullWalk allocated %.0f times, want 0", allocs)
	}
}

// ── CompileError formatting ────────────────────────────────────────────────

func TestCompileError_String(t *testing.T) {
	e := CompileError{
		Pass:       PassDescriptor,
		SchemaUUID: "schema-uuid",
		FieldUUID:  "field-uuid",
		Message:    "something went wrong",
	}
	s := e.Error()
	if !strings.Contains(s, "descriptor") {
		t.Errorf("error string missing pass name: %q", s)
	}
	if !strings.Contains(s, "schema-uuid") {
		t.Errorf("error string missing schema UUID: %q", s)
	}
	if !strings.Contains(s, "field-uuid") {
		t.Errorf("error string missing field UUID: %q", s)
	}
	if !strings.Contains(s, "something went wrong") {
		t.Errorf("error string missing message: %q", s)
	}
}

func TestCompileErrors_String(t *testing.T) {
	ce := CompileErrors{
		{Pass: PassParse, Message: "first error"},
		{Pass: PassDescriptor, Message: "second error"},
	}
	s := ce.Error()
	if !strings.Contains(s, "2 error") {
		t.Errorf("CompileErrors string missing count: %q", s)
	}
	if !strings.Contains(s, "first error") {
		t.Errorf("CompileErrors string missing first error: %q", s)
	}
	if !strings.Contains(s, "second error") {
		t.Errorf("CompileErrors string missing second error: %q", s)
	}
}

func TestCompileErrors_Errors(t *testing.T) {
	ce := CompileErrors{
		{Pass: PassParse, Message: "e1"},
		{Pass: PassStore, Message: "e2"},
	}
	errs := ce.Errors()
	if len(errs) != 2 {
		t.Errorf("Errors() length: got %d, want 2", len(errs))
	}
}
