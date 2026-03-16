package ir

import (
	"os"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

// e2e_test.go exercises the full Parse → Compile pipeline with realistic
// schemas, including the meta_schema.json itself. These tests verify end-to-end
// invariants: descriptor count consistency, offset coverage, meta completeness,
// and structural soundness.

// ── Compile the meta_schema.json itself ────────────────────────────────────

func TestE2E_MetaSchemaCompiles(t *testing.T) {
	src, err := os.ReadFile("./meta_schema.json")
	if err != nil {
		// File not found relative to test location — skip rather than fail.
		t.Skipf("meta_schema.json not found: %v", err)
	}

	ss, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// meta_schema.json has two constraints using predicates that are not
	// registered — we supply stubs so compilation can succeed.
	pm := PredicateMap{
		"requireUuidV7OnIdPaths": func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool { return true },
		"requireUuidV7OnKeys":    func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool { return true },
	}

	cs, err := Compile(ss, pm)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// Basic soundness: at least the root schema must be present.
	if cs.Meta[0] == nil {
		t.Error("Meta[0] is nil")
	}
	if cs.Meta[0].Name != "Schema" {
		t.Errorf("root schema name: got %q, want %q", cs.Meta[0].Name, "Schema")
	}
}

// ── Full-pipeline invariant checks ─────────────────────────────────────────

// TestE2E_InvariantDescriptorCountMatchesOffsets verifies that the sum of all
// non-sentinel offset ranges equals len(Descriptors).
func TestE2E_InvariantDescriptorCountMatchesOffsets(t *testing.T) {
	schemas := [][]byte{
		flatSchema,
		nestedObjectSchema,
		enumSchema,
		unionSchema,
		indexedSchema,
		constrainedSchema,
		defaultSchema,
		cycleSchema,
	}

	for _, src := range schemas {
		cs := mustCompileAny(t, src)
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
			t.Errorf("offset sum=%d != len(Descriptors)=%d", total, len(cs.Descriptors))
		}
	}
}

// TestE2E_InvariantReservedBitAlwaysZero verifies bit 0 is never set.
func TestE2E_InvariantReservedBitAlwaysZero(t *testing.T) {
	schemas := [][]byte{
		flatSchema, nestedObjectSchema, enumSchema, unionSchema,
		indexedSchema, cycleSchema, defaultSchema,
	}
	for _, src := range schemas {
		cs := mustCompileAny(t, src)
		for i, fd := range cs.Descriptors {
			if fd&1 != 0 {
				t.Errorf("descriptor[%d]: reserved bit 0 is set (0x%08X)", i, fd)
			}
		}
	}
}

// TestE2E_InvariantOwnerSchemaMatchesOffsetSlot verifies that each descriptor's
// owner_schema bits agree with its position within SchemaOffsets.
func TestE2E_InvariantOwnerSchemaMatchesOffsetSlot(t *testing.T) {
	schemas := [][]byte{flatSchema, nestedObjectSchema, unionSchema, cycleSchema}
	for _, src := range schemas {
		cs := mustCompileAny(t, src)
		for schemaIdx := range cs.SchemaOffsets {
			start, end := descriptorRange(cs, uint8(schemaIdx))
			if start == 0 && end == 0 {
				continue
			}
			for pos := start; pos < end; pos++ {
				fd := cs.Descriptors[pos]
				got := ExtractOwnerSchema(fd)
				if got != uint8(schemaIdx) {
					t.Errorf("Descriptors[%d]: owner_schema=%d, expected %d", pos, got, schemaIdx)
				}
			}
		}
	}
}

// TestE2E_InvariantFieldIndexUniqueWithinSchema verifies that within each
// schema, no two descriptors share the same field_index.
func TestE2E_InvariantFieldIndexUniqueWithinSchema(t *testing.T) {
	schemas := [][]byte{flatSchema, nestedObjectSchema, unionSchema}
	for _, src := range schemas {
		cs := mustCompileAny(t, src)
		for schemaIdx := range cs.SchemaOffsets {
			start, end := descriptorRange(cs, uint8(schemaIdx))
			if start == 0 && end == 0 {
				continue
			}
			seen := map[uint8]bool{}
			for pos := start; pos < end; pos++ {
				fi := ExtractFieldIndex(cs.Descriptors[pos])
				if seen[fi] {
					t.Errorf("schema %d: duplicate field_index %d", schemaIdx, fi)
				}
				seen[fi] = true
			}
		}
	}
}

// TestE2E_InvariantScalarTargetSchemaIsZero verifies that all non-schema-bearing
// descriptors have target_schema=0.
func TestE2E_InvariantScalarTargetSchemaIsZero(t *testing.T) {
	schemas := [][]byte{flatSchema, nestedObjectSchema, enumSchema, unionSchema}
	for _, src := range schemas {
		cs := mustCompileAny(t, src)
		for i, fd := range cs.Descriptors {
			if !IsSchemaBearing(fd) && ExtractTargetSchema(fd) != 0 {
				t.Errorf("descriptor[%d]: non-schema-bearing field has target_schema=%d",
					i, ExtractTargetSchema(fd))
			}
		}
	}
}

// TestE2E_InvariantSchemaBearingTargetSchemaInBounds verifies that
// target_schema for schema-bearing (non-union/composite) fields is a valid
// index into SchemaOffsets.
func TestE2E_InvariantSchemaBearingTargetSchemaInBounds(t *testing.T) {
	schemas := [][]byte{nestedObjectSchema, enumSchema}
	for _, src := range schemas {
		cs := mustCompileAny(t, src)
		maxIdx := len(cs.SchemaOffsets)
		for i, fd := range cs.Descriptors {
			if !IsSchemaBearing(fd) {
				continue
			}
			typ := ExtractType(fd)
			if typ == TypeUnion || typ == TypeComposite {
				// Variants — target_schema is 0 by spec; variants are in Variants map.
				continue
			}
			target := int(ExtractTargetSchema(fd))
			if target >= maxIdx {
				t.Errorf("descriptor[%d]: target_schema=%d out of bounds (max %d)", i, target, maxIdx-1)
			}
		}
	}
}

// TestE2E_InvariantMetaCoversAllSchemas verifies that cs.Meta has an entry
// for every schema index present in cs.SchemaOffsets.
func TestE2E_InvariantMetaCoversAllSchemas(t *testing.T) {
	schemas := [][]byte{flatSchema, nestedObjectSchema, enumSchema, unionSchema}
	for _, src := range schemas {
		cs := mustCompileAny(t, src)
		for idx := range cs.SchemaOffsets {
			if _, ok := cs.Meta[uint8(idx)]; !ok {
				t.Errorf("Meta missing entry for schema index %d", idx)
			}
		}
	}
}

// TestE2E_InvariantVariantsOnlyForUnionComposite verifies that the Variants
// map has entries only for union and composite fields.
func TestE2E_InvariantVariantsOnlyForUnionComposite(t *testing.T) {
	cs := mustCompileAny(t, unionSchema)
	for fd, variants := range cs.Variants {
		typ := ExtractType(fd)
		if typ != TypeUnion && typ != TypeComposite {
			t.Errorf("Variants entry for non-union/composite type %v (fd=0x%08X)", typ, fd)
		}
		if len(variants) == 0 {
			t.Errorf("Variants entry for fd=0x%08X has zero variants", fd)
		}
	}
}

// TestE2E_InvariantTerminalScalarsAlways1 verifies that every non-schema-bearing
// field has terminal=1 after compilation.
func TestE2E_InvariantTerminalScalarsAlways1(t *testing.T) {
	schemas := [][]byte{flatSchema, nestedObjectSchema, cycleSchema, unionSchema}
	for _, src := range schemas {
		cs := mustCompileAny(t, src)
		for i, fd := range cs.Descriptors {
			if !IsSchemaBearing(fd) && !IsTerminal(fd) {
				t.Errorf("descriptor[%d]: scalar field has terminal=0 (fd=0x%08X)", i, fd)
			}
		}
	}
}

// TestE2E_RoundTrip_MetaFieldNamesResolveToDescriptors verifies that for every
// entry in Meta[schemaIdx].Fields, the descriptor key can be found in
// Descriptors at the position implied by SchemaOffsets.
func TestE2E_RoundTrip_MetaFieldNamesResolveToDescriptors(t *testing.T) {
	schemas := [][]byte{flatSchema, nestedObjectSchema, enumSchema}
	for _, src := range schemas {
		cs := mustCompileAny(t, src)
		for schemaIdx, m := range cs.Meta {
			start, end := descriptorRange(cs, schemaIdx)
			schemaDescriptors := map[uint32]bool{}
			for _, fd := range cs.Descriptors[start:end] {
				schemaDescriptors[fd] = true
			}
			for fd, fm := range m.Fields {
				if !schemaDescriptors[fd] {
					t.Errorf("Meta[%d].Fields[0x%08X] (%s): descriptor not in schema range",
						schemaIdx, fd, fm.Name)
				}
			}
		}
	}
}

// TestE2E_ErrorPropagation_MultipleErrors verifies that multiple errors in the
// same pass are all returned, not just the first.
func TestE2E_ErrorPropagation_MultipleErrors(t *testing.T) {
	// Two fields with unresolvable refs — should produce two errors.
	src := []byte(`{
	  "name": "Broken",
	  "version": "1.0.0",
	  "fields": {
	    "019ca000-0001-7001-810d-141b22293037": {
	      "name": "a", "type": "object",
	      "schema": { "id": "019ca000-0f01-70f1-b13d-444b52596067" }
	    },
	    "019ca000-0002-7002-821a-21282f363d44": {
	      "name": "b", "type": "object",
	      "schema": { "id": "019ca000-0f02-70f2-b24a-51585f666d74" }
	    }
	  }
	}`)

	ss := mustParse(src)
	_, err := Compile(ss, nil)
	if err == nil {
		t.Fatal("expected errors, got nil")
	}
	errs := allErrors(err)
	if len(errs) < 2 {
		t.Errorf("expected at least 2 errors, got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if e.Pass != PassDescriptor {
			t.Errorf("expected PassDescriptor, got %v", e.Pass)
		}
	}
}

// TestE2E_ParseErrorDoesNotProceedToCompile verifies that a Parse error
// prevents Compile from being called.
func TestE2E_ParseErrorDoesNotProceedToCompile(t *testing.T) {
	_, err := Parse(invalidBadJSON)
	if err == nil {
		t.Fatal("Parse should have returned an error")
	}
	// The error must be CompileErrors with PassParse entries.
	ce, ok := err.(CompileErrors)
	if !ok {
		t.Fatalf("error is not CompileErrors: %T", err)
	}
	for _, e := range ce {
		if e.Pass != PassParse {
			t.Errorf("expected PassParse, got %v", e.Pass)
		}
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

// mustCompileAny compiles src with a full stub predicate map (all known
// predicate names return true). Used in invariant tests that don't care about
// predicate behaviour.
func mustCompileAny(t *testing.T, src []byte) *CompiledSchema {
	t.Helper()
	ss, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	pm := PredicateMap{
		"isEmail":                func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool { return true },
		"requireUuidV7OnIdPaths": func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool { return true },
		"requireUuidV7OnKeys":    func(_ *document.DataContainer, _ []document.DataPoint, _ any) bool { return true },
	}
	cs, err := Compile(ss, pm)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	return cs
}
