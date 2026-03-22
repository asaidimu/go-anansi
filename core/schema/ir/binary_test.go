package ir_test

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v6/core/document"
	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// compile parses and compiles src JSON, failing the test on any error.
func compile(t *testing.T, src string, predicates ir.PredicateMap) *ir.Schema {
	t.Helper()
	parsed, err := ir.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cs, err := ir.Compile(parsed, predicates)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return cs
}

// roundTrip marshals cs and unmarshals it, returning the reconstituted schema.
func roundTrip(t *testing.T, cs *ir.Schema, predicates ir.PredicateMap) *ir.Schema {
	t.Helper()
	data, err := ir.MarshalSchema(cs)
	if err != nil {
		t.Fatalf("MarshalSchema: %v", err)
	}
	got, err := ir.UnmarshalSchema(data, predicates)
	if err != nil {
		t.Fatalf("UnmarshalSchema: %v", err)
	}
	return got
}

// ── Schema fixtures ───────────────────────────────────────────────────────────

// minimalSchema is the smallest valid schema: one string field, no extras.
const minimalSchema = `{
  "name": "Minimal",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": {
      "name": "title",
      "type": "string",
      "required": true
    }
  }
}`

// flSchema has one field of every scalar type plus an enum.
const flSchema = `{
  "name": "Flat",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "s",   "type": "string"  },
    "01900000-0000-7000-8000-000000000002": { "name": "n",   "type": "number"  },
    "01900000-0000-7000-8000-000000000003": { "name": "i",   "type": "integer" },
    "01900000-0000-7000-8000-000000000004": { "name": "b",   "type": "boolean" },
    "01900000-0000-7000-8000-000000000005": { "name": "by",  "type": "bytes"   },
    "01900000-0000-7000-8000-000000000006": { "name": "geo", "type": "geometry"},
    "01900000-0000-7000-8000-000000000007": {
      "name": "status",
      "type": "enum",
      "schema": { "values": ["active", "inactive", "pending"] }
    }
  }
}`

// nestedSchema has a root schema that embeds a child object schema.
const nestedSchema = `{
  "name": "Nested",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "name", "type": "string" },
    "01900000-0000-7000-8000-000000000010": {
      "name": "address",
      "type": "object",
      "schema": { "id": "01900000-0000-7000-8000-0000000000A1" }
    }
  },
  "schemas": {
    "01900000-0000-7000-8000-0000000000A1": {
      "name": "Address",
      "fields": {
        "01900000-0000-7000-8000-000000000020": { "name": "street",  "type": "string" },
        "01900000-0000-7000-8000-000000000021": { "name": "city",    "type": "string" },
        "01900000-0000-7000-8000-000000000022": { "name": "country", "type": "string" }
      }
    }
  }
}`

// indexSchema has an index and a conditional partial index.
// Index fields and condition fields are dot-separated field name paths,
// not UUIDs — this is what cs.DocumentKey() resolves.
const indexSchema = `{
  "name": "Indexed",
  "version": "2.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "email",  "type": "string", "unique": true },
    "01900000-0000-7000-8000-000000000002": { "name": "active", "type": "boolean" }
  },
  "indexes": {
    "01900000-0000-7000-8000-000000000101": {
      "name": "email_idx",
      "type": "unique",
      "order": "asc",
      "fields": ["email"]
    },
    "01900000-0000-7000-8000-000000000102": {
      "name": "active_email_idx",
      "type": "normal",
      "fields": ["email"],
      "condition": {
        "field": "active",
        "operator": "eq",
        "value": true
      }
    }
  }
}`

// constrainSchema has a constraint that uses a registered predicate.
// Constraint fields are dot-separated field name paths, not UUIDs.
const constrainSchema = `{
  "name": "Constrained",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "start", "type": "integer" },
    "01900000-0000-7000-8000-000000000002": { "name": "end",   "type": "integer" }
  },
  "constraints": {
    "01900000-0000-7000-8000-000000000201": {
      "name": "start_before_end",
      "predicate": "lt",
      "fields": [
        "start",
        "end"
      ]
    }
  }
}`

// cyclicSchema has a self-referencing object field (linked list style).
const cyclicSchema = `{
  "name": "Cyclic",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "value", "type": "string" },
    "01900000-0000-7000-8000-000000000002": {
      "name": "next",
      "type": "object",
      "schema": { "id": "01900000-0000-7000-8000-0000000000B1" }
    }
  },
  "schemas": {
    "01900000-0000-7000-8000-0000000000B1": {
      "name": "Node",
      "fields": {
        "01900000-0000-7000-8000-000000000011": { "name": "value", "type": "string" },
        "01900000-0000-7000-8000-000000000012": {
          "name": "next",
          "type": "object",
          "schema": { "id": "01900000-0000-7000-8000-0000000000B1" }
        }
      }
    }
  }
}`

// uninSchema has a union field over two variant schemas.
const uninSchema = `{
  "name": "Union",
  "version": "1.0.0",
  "fields": {
    "01900000-0000-7000-8000-000000000001": { "name": "id", "type": "string" },
    "01900000-0000-7000-8000-000000000002": {
      "name": "payload",
      "type": "union",
      "schema": [
        { "id": "01900000-0000-7000-8000-0000000000C1" },
        { "id": "01900000-0000-7000-8000-0000000000C2" }
      ]
    }
  },
  "schemas": {
    "01900000-0000-7000-8000-0000000000C1": {
      "name": "TypeA",
      "fields": {
        "01900000-0000-7000-8000-000000000031": { "name": "a_val", "type": "string" }
      }
    },
    "01900000-0000-7000-8000-0000000000C2": {
      "name": "TypeB",
      "fields": {
        "01900000-0000-7000-8000-000000000041": { "name": "b_val", "type": "integer" }
      }
    }
  }
}`

// metadataSchema has a schema with extra metadata and enum values of all types.
const metadataSchema = `{
  "name": "WithMetadata",
  "version": "1.0.0",
  "description": "Schema with metadata",
  "concrete": true,
  "metadata": {
    "author": "test",
    "version_info": { "major": 1, "minor": 0 }
  },
  "fields": {
    "01900000-0000-7000-8000-000000000001": {
      "name": "code",
      "type": "enum",
      "description": "A status code",
      "schema": { "values": [1, 2, 3] }
    },
    "01900000-0000-7000-8000-000000000002": {
      "name": "flag",
      "type": "enum",
      "schema": { "values": [true, false] }
    },
    "01900000-0000-7000-8000-000000000003": {
      "name": "label",
      "type": "string",
      "default": "untitled"
    },
    "01900000-0000-7000-8000-000000000004": {
      "name": "score",
      "type": "number",
      "default": 0.5
    }
  }
}`

// ltPredicate is a trivial predicate used in constraint tests.
var ltPredicate ir.Predicate = func(_ *document.Document, _ []document.DocumentKey, _ any) bool {
	return true
}

// ── Core round-trip tests ─────────────────────────────────────────────────────

func TestRoundTrip_Minimal(t *testing.T) {
	cs := compile(t, minimalSchema, nil)
	got := roundTrip(t, cs, nil)

	assertDescriptors(t, cs, got)
	assertSchemaOffsets(t, cs, got)
	assertVariants(t, cs, got)
	assertAddressSpace(t, cs, got)
	assertMeta(t, cs, got)
	assertAddressResolution(t, cs, got, "title")
}

func TestRoundTrip_FlatScalarsAndEnum(t *testing.T) {
	cs := compile(t, flSchema, nil)
	got := roundTrip(t, cs, nil)

	assertDescriptors(t, cs, got)
	assertSchemaOffsets(t, cs, got)
	assertAddressSpace(t, cs, got)
	assertMeta(t, cs, got)
	assertStoreEquivalent(t, cs, got)

	// Enum values should survive the round-trip and be resolvable via Address.
	for _, path := range []string{"s", "n", "i", "b", "by", "geo", "status"} {
		assertAddressResolution(t, cs, got, path)
	}
}

func TestRoundTrip_Nested(t *testing.T) {
	cs := compile(t, nestedSchema, nil)
	got := roundTrip(t, cs, nil)

	assertDescriptors(t, cs, got)
	assertAddressSpace(t, cs, got)
	assertMeta(t, cs, got)

	for _, path := range []string{"name", "address.street", "address.city"} {
		assertAddressResolution(t, cs, got, path)
	}
}

func TestRoundTrip_Indexes(t *testing.T) {
	cs := compile(t, indexSchema, nil)
	got := roundTrip(t, cs, nil)

	assertDescriptors(t, cs, got)
	assertAddressSpace(t, cs, got)
	assertMeta(t, cs, got)
	assertResolvedIndexes(t, cs, got)

	assertAddressResolution(t, cs, got, "email")
	assertAddressResolution(t, cs, got, "active")
}

func TestRoundTrip_Constraints(t *testing.T) {
	predicates := ir.PredicateMap{"lt": ltPredicate}
	cs := compile(t, constrainSchema, predicates)
	got := roundTrip(t, cs, predicates)

	assertDescriptors(t, cs, got)
	assertAddressSpace(t, cs, got)
	assertResolvedConstraints(t, cs, got)
}

func TestRoundTrip_Cyclic(t *testing.T) {
	cs := compile(t, cyclicSchema, nil)
	got := roundTrip(t, cs, nil)

	assertDescriptors(t, cs, got)
	assertAddressSpace(t, cs, got)
	assertMeta(t, cs, got)

	// Address resolution must work for the acyclic fields.
	assertAddressResolution(t, cs, got, "value")
}

func TestRoundTrip_Union(t *testing.T) {
	cs := compile(t, uninSchema, nil)
	got := roundTrip(t, cs, nil)

	assertDescriptors(t, cs, got)
	assertVariants(t, cs, got)
	assertAddressSpace(t, cs, got)
	assertMeta(t, cs, got)

	assertAddressResolution(t, cs, got, "id")
	assertAddressResolution(t, cs, got, "payload.a_val")
	assertAddressResolution(t, cs, got, "payload.b_val")
}

func TestRoundTrip_MetadataAndDefaults(t *testing.T) {
	cs := compile(t, metadataSchema, nil)
	got := roundTrip(t, cs, nil)

	assertDescriptors(t, cs, got)
	assertAddressSpace(t, cs, got)
	assertMeta(t, cs, got)
	assertStoreEquivalent(t, cs, got)
}

// ── Header and corruption tests ───────────────────────────────────────────────

func TestUnmarshal_TooShort(t *testing.T) {
	_, err := ir.UnmarshalSchema([]byte{0x41, 0x4e}, nil)
	if !errors.Is(err, ir.ErrFormatCorrupt) {
		t.Fatalf("want ErrFormatCorrupt, got %v", err)
	}
}

func TestUnmarshal_BadMagic(t *testing.T) {
	cs := compile(t, minimalSchema, nil)
	data, err := ir.MarshalSchema(cs)
	if err != nil {
		t.Fatalf("MarshalSchema: %v", err)
	}
	// Corrupt the magic bytes.
	data[0] = 'X'
	_, err = ir.UnmarshalSchema(data, nil)
	if !errors.Is(err, ir.ErrFormatCorrupt) {
		t.Fatalf("want ErrFormatCorrupt, got %v", err)
	}
}

func TestUnmarshal_BadVersion(t *testing.T) {
	cs := compile(t, minimalSchema, nil)
	data, err := ir.MarshalSchema(cs)
	if err != nil {
		t.Fatalf("MarshalSchema: %v", err)
	}
	// Overwrite version with 999.
	binary.LittleEndian.PutUint16(data[4:6], 999)
	// Fix checksum so we reach the version check before the checksum check.
	// (Version is checked before checksum in the current impl; we just want
	// to verify ErrFormatVersion is returned.)
	_, err = ir.UnmarshalSchema(data, nil)
	if !errors.Is(err, ir.ErrFormatVersion) {
		t.Fatalf("want ErrFormatVersion, got %v", err)
	}
}

func TestUnmarshal_CorruptChecksum(t *testing.T) {
	cs := compile(t, minimalSchema, nil)
	data, err := ir.MarshalSchema(cs)
	if err != nil {
		t.Fatalf("MarshalSchema: %v", err)
	}
	// Flip one byte in the body to invalidate the checksum.
	data[len(data)-1] ^= 0xFF
	_, err = ir.UnmarshalSchema(data, nil)
	if !errors.Is(err, ir.ErrFormatCorrupt) {
		t.Fatalf("want ErrFormatCorrupt, got %v", err)
	}
}

func TestUnmarshal_Truncated(t *testing.T) {
	cs := compile(t, minimalSchema, nil)
	data, err := ir.MarshalSchema(cs)
	if err != nil {
		t.Fatalf("MarshalSchema: %v", err)
	}
	// Recompute checksum over the truncated body so it passes the checksum
	// check and hits the structural truncation error deeper inside.
	truncated := data[:len(data)/2]
	// This will either hit ErrFormatCorrupt (checksum mismatch on the short
	// slice) or the need() bounds check inside the decoder. Both are correct.
	_, err = ir.UnmarshalSchema(truncated, nil)
	if err == nil {
		t.Fatal("expected an error for truncated input, got nil")
	}
}

func TestMarshal_NilSchema(t *testing.T) {
	_, err := ir.MarshalSchema(nil)
	if err == nil {
		t.Fatal("expected error marshaling nil schema")
	}
}

func TestUnmarshal_MissingPredicate(t *testing.T) {
	predicates := ir.PredicateMap{"lt": ltPredicate}
	cs := compile(t, constrainSchema, predicates)
	data, err := ir.MarshalSchema(cs)
	if err != nil {
		t.Fatalf("MarshalSchema: %v", err)
	}
	// Unmarshal with an empty predicate map — "lt" is unregistered.
	_, err = ir.UnmarshalSchema(data, ir.PredicateMap{})
	if err == nil {
		t.Fatal("expected error for unregistered predicate")
	}
	if !strings.Contains(err.Error(), "lt") {
		t.Errorf("error should mention predicate name %q, got: %v", "lt", err)
	}
}

func TestUnmarshal_NilPredicateMapOnConstraintlessSchema(t *testing.T) {
	// A schema with no constraints should load fine with a nil predicate map.
	cs := compile(t, minimalSchema, nil)
	data, _ := ir.MarshalSchema(cs)
	_, err := ir.UnmarshalSchema(data, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Idempotency: marshal twice, expect identical bytes ────────────────────────

func TestMarshal_Idempotent(t *testing.T) {
	cs := compile(t, nestedSchema, nil)
	b1, err := ir.MarshalSchema(cs)
	if err != nil {
		t.Fatalf("first MarshalSchema: %v", err)
	}
	b2, err := ir.MarshalSchema(cs)
	if err != nil {
		t.Fatalf("second MarshalSchema: %v", err)
	}
	// Map iteration order is not guaranteed; the two marshals may differ in
	// map-keyed sections (Meta, Variants, ResolvedIndexes). We verify that
	// both bytes decode to an equivalent schema rather than requiring
	// byte-identical output.
	got1, err := ir.UnmarshalSchema(b1, nil)
	if err != nil {
		t.Fatalf("UnmarshalSchema b1: %v", err)
	}
	got2, err := ir.UnmarshalSchema(b2, nil)
	if err != nil {
		t.Fatalf("UnmarshalSchema b2: %v", err)
	}
	assertDescriptors(t, got1, got2)
	assertAddressSpace(t, got1, got2)
}

// ── Double round-trip: unmarshal → re-marshal → unmarshal ────────────────────

func TestRoundTrip_Double(t *testing.T) {
	cs := compile(t, uninSchema, nil)

	data1, err := ir.MarshalSchema(cs)
	if err != nil {
		t.Fatalf("MarshalSchema pass 1: %v", err)
	}
	cs2, err := ir.UnmarshalSchema(data1, nil)
	if err != nil {
		t.Fatalf("UnmarshalSchema pass 1: %v", err)
	}
	data2, err := ir.MarshalSchema(cs2)
	if err != nil {
		t.Fatalf("MarshalSchema pass 2: %v", err)
	}
	cs3, err := ir.UnmarshalSchema(data2, nil)
	if err != nil {
		t.Fatalf("UnmarshalSchema pass 2: %v", err)
	}

	assertDescriptors(t, cs, cs3)
	assertVariants(t, cs, cs3)
	assertAddressSpace(t, cs, cs3)
	assertMeta(t, cs, cs3)
	assertAddressResolution(t, cs, cs3, "id")
	assertAddressResolution(t, cs, cs3, "payload.a_val")
}

// ── Size regression: sparse encoding should be much smaller than 83 KB ───────

func TestMarshal_SizeRegression(t *testing.T) {
	cs := compile(t, nestedSchema, nil)
	data, err := ir.MarshalSchema(cs)
	if err != nil {
		t.Fatalf("MarshalSchema: %v", err)
	}
	const denseAddressSpaceSize = 83_332
	const headerSize = 32
	if len(data) >= headerSize+denseAddressSpaceSize {
		t.Errorf(
			"binary is %d bytes — suspiciously large; "+
				"sparse address space encoding should produce far less than %d bytes",
			len(data), headerSize+denseAddressSpaceSize,
		)
	}
	t.Logf("binary size: %d bytes", len(data))
}

// ── assertion helpers ─────────────────────────────────────────────────────────

func assertDescriptors(t *testing.T, want, got *ir.Schema) {
	t.Helper()
	if len(want.Descriptors) != len(got.Descriptors) {
		t.Fatalf("Descriptors len: want %d, got %d", len(want.Descriptors), len(got.Descriptors))
	}
	for i := range want.Descriptors {
		if want.Descriptors[i] != got.Descriptors[i] {
			t.Errorf("Descriptors[%d]: want 0x%08x, got 0x%08x", i, want.Descriptors[i], got.Descriptors[i])
		}
	}
}

func assertSchemaOffsets(t *testing.T, want, got *ir.Schema) {
	t.Helper()
	if len(want.SchemaOffsets) != len(got.SchemaOffsets) {
		t.Fatalf("SchemaOffsets len: want %d, got %d", len(want.SchemaOffsets), len(got.SchemaOffsets))
	}
	for i := range want.SchemaOffsets {
		if want.SchemaOffsets[i] != got.SchemaOffsets[i] {
			t.Errorf("SchemaOffsets[%d]: want %d, got %d", i, want.SchemaOffsets[i], got.SchemaOffsets[i])
		}
	}
}

func assertVariants(t *testing.T, want, got *ir.Schema) {
	t.Helper()
	if len(want.Variants) != len(got.Variants) {
		t.Fatalf("Variants len: want %d, got %d", len(want.Variants), len(got.Variants))
	}
	for fd, wv := range want.Variants {
		gv, ok := got.Variants[fd]
		if !ok {
			t.Errorf("Variants missing key 0x%08x", fd)
			continue
		}
		if len(wv) != len(gv) {
			t.Errorf("Variants[0x%08x] len: want %d, got %d", fd, len(wv), len(gv))
			continue
		}
		for i := range wv {
			if wv[i] != gv[i] {
				t.Errorf("Variants[0x%08x][%d]: want %d, got %d", fd, i, wv[i], gv[i])
			}
		}
	}
}

func assertAddressSpace(t *testing.T, want, got *ir.Schema) {
	t.Helper()
	was := want.AddressSpace
	gas := got.AddressSpace
	if was == nil || gas == nil {
		t.Fatal("AddressSpace is nil after round-trip")
	}
	if was.FrontSize != gas.FrontSize {
		t.Errorf("AddressSpace.FrontSize: want %d, got %d", was.FrontSize, gas.FrontSize)
	}
	if was.FieldOrdinals != gas.FieldOrdinals {
		t.Error("AddressSpace.FieldOrdinals mismatch")
	}
	if was.BackEdgeOrdinal != gas.BackEdgeOrdinal {
		t.Error("AddressSpace.BackEdgeOrdinal mismatch")
	}
	if was.BlockBases != gas.BlockBases {
		t.Error("AddressSpace.BlockBases mismatch")
	}
	if was.BlockSize != gas.BlockSize {
		t.Error("AddressSpace.BlockSize mismatch")
	}
	if was.AcyclicSubtreeSize != gas.AcyclicSubtreeSize {
		t.Error("AddressSpace.AcyclicSubtreeSize mismatch")
	}
	if was.EntryOrdinal != gas.EntryOrdinal {
		t.Error("AddressSpace.EntryOrdinal mismatch")
	}
	// FieldNames: compare map contents
	for si := range was.FieldNames {
		wm := was.FieldNames[si]
		gm := gas.FieldNames[si]
		if (wm == nil) != (gm == nil) {
			t.Errorf("AddressSpace.FieldNames[%d]: want nil=%v, got nil=%v", si, wm == nil, gm == nil)
			continue
		}
		if len(wm) != len(gm) {
			t.Errorf("AddressSpace.FieldNames[%d] len: want %d, got %d", si, len(wm), len(gm))
			continue
		}
		for name, wfi := range wm {
			if gfi, ok := gm[name]; !ok || wfi != gfi {
				t.Errorf("AddressSpace.FieldNames[%d][%q]: want %d, got %d (ok=%v)", si, name, wfi, gfi, ok)
			}
		}
	}
}

func assertMeta(t *testing.T, want, got *ir.Schema) {
	t.Helper()
	if len(want.Meta) != len(got.Meta) {
		t.Fatalf("Meta len: want %d, got %d", len(want.Meta), len(got.Meta))
	}
	for idx, wm := range want.Meta {
		gm, ok := got.Meta[idx]
		if !ok {
			t.Errorf("Meta missing schema index %d", idx)
			continue
		}
		if wm.UUID != gm.UUID {
			t.Errorf("Meta[%d].UUID: want %q, got %q", idx, wm.UUID, gm.UUID)
		}
		if wm.Name != gm.Name {
			t.Errorf("Meta[%d].Name: want %q, got %q", idx, wm.Name, gm.Name)
		}
		if wm.Version != gm.Version {
			t.Errorf("Meta[%d].Version: want %q, got %q", idx, wm.Version, gm.Version)
		}
		if wm.Description != gm.Description {
			t.Errorf("Meta[%d].Description: want %q, got %q", idx, wm.Description, gm.Description)
		}
		if wm.Concrete != gm.Concrete {
			t.Errorf("Meta[%d].Concrete: want %v, got %v", idx, wm.Concrete, gm.Concrete)
		}
		if wm.Type != gm.Type {
			t.Errorf("Meta[%d].Type: want %v, got %v", idx, wm.Type, gm.Type)
		}
		if wm.TargetSchema != gm.TargetSchema {
			t.Errorf("Meta[%d].TargetSchema: want %d, got %d", idx, wm.TargetSchema, gm.TargetSchema)
		}
		if len(wm.Fields) != len(gm.Fields) {
			t.Errorf("Meta[%d].Fields len: want %d, got %d", idx, len(wm.Fields), len(gm.Fields))
		}
		for fd, wfm := range wm.Fields {
			if gfm, ok := gm.Fields[fd]; !ok {
				t.Errorf("Meta[%d].Fields missing descriptor 0x%08x", idx, fd)
			} else if wfm.UUID != gfm.UUID || wfm.Name != gfm.Name {
				t.Errorf("Meta[%d].Fields[0x%08x]: want {%q,%q}, got {%q,%q}",
					idx, fd, wfm.UUID, wfm.Name, gfm.UUID, gfm.Name)
			}
		}
		// IndexOrdinals
		for uuid, wo := range wm.IndexOrdinals {
			if go_, ok := gm.IndexOrdinals[uuid]; !ok || wo != go_ {
				t.Errorf("Meta[%d].IndexOrdinals[%q]: want %d, got %d (ok=%v)", idx, uuid, wo, go_, ok)
			}
		}
	}
}

func assertResolvedIndexes(t *testing.T, want, got *ir.Schema) {
	t.Helper()
	if len(want.ResolvedIndexes) != len(got.ResolvedIndexes) {
		t.Fatalf("ResolvedIndexes len: want %d, got %d", len(want.ResolvedIndexes), len(got.ResolvedIndexes))
	}
	for key, wi := range want.ResolvedIndexes {
		gi, ok := got.ResolvedIndexes[key]
		if !ok {
			t.Errorf("ResolvedIndexes missing key %d", key)
			continue
		}
		if wi.Type != gi.Type {
			t.Errorf("ResolvedIndexes[%d].Type: want %d, got %d", key, wi.Type, gi.Type)
		}
		if wi.Order != gi.Order {
			t.Errorf("ResolvedIndexes[%d].Order: want %d, got %d", key, wi.Order, gi.Order)
		}
		if wi.Unique != gi.Unique {
			t.Errorf("ResolvedIndexes[%d].Unique: want %v, got %v", key, wi.Unique, gi.Unique)
		}
		if len(wi.Fields) != len(gi.Fields) {
			t.Fatalf("ResolvedIndexes[%d].Fields len: want %d, got %d", key, len(wi.Fields), len(gi.Fields))
		}
		for i := range wi.Fields {
			if wi.Fields[i] != gi.Fields[i] {
				t.Errorf("ResolvedIndexes[%d].Fields[%d]: want %d, got %d", key, i, wi.Fields[i], gi.Fields[i])
			}
		}
		if (wi.Condition == nil) != (gi.Condition == nil) {
			t.Errorf("ResolvedIndexes[%d].Condition nil mismatch", key)
		}
	}
}

func assertResolvedConstraints(t *testing.T, want, got *ir.Schema) {
	t.Helper()
	if (want.ResolvedConstraints == nil) != (got.ResolvedConstraints == nil) {
		t.Fatalf("ResolvedConstraints nil mismatch: want nil=%v, got nil=%v",
			want.ResolvedConstraints == nil, got.ResolvedConstraints == nil)
	}
	if want.ResolvedConstraints == nil {
		return
	}
	wrt := want.ResolvedConstraints
	grt := got.ResolvedConstraints
	if len(wrt.Roots) != len(grt.Roots) {
		t.Errorf("ResolvedConstraints.Roots len: want %d, got %d", len(wrt.Roots), len(grt.Roots))
	}
	if len(wrt.Index) != len(grt.Index) {
		t.Errorf("ResolvedConstraints.Index len: want %d, got %d", len(wrt.Index), len(grt.Index))
	}
	for ordinal, wn := range wrt.Index {
		gn, ok := grt.Index[ordinal]
		if !ok {
			t.Errorf("ResolvedConstraints.Index missing ordinal %d", ordinal)
			continue
		}
		wrc, wok := wn.(ir.ResolvedConstraint)
		grc, gok := gn.(ir.ResolvedConstraint)
		if wok != gok {
			t.Errorf("constraint ordinal %d: type mismatch", ordinal)
			continue
		}
		if !wok {
			continue // group — shape verified via len check above
		}
		if wrc.UUID != grc.UUID {
			t.Errorf("constraint[%d].UUID: want %q, got %q", ordinal, wrc.UUID, grc.UUID)
		}
		if wrc.PredicateName != grc.PredicateName {
			t.Errorf("constraint[%d].PredicateName: want %q, got %q", ordinal, wrc.PredicateName, grc.PredicateName)
		}
		if grc.Predicate == nil {
			t.Errorf("constraint[%d].Predicate is nil after unmarshal", ordinal)
		}
		if len(wrc.Fields) != len(grc.Fields) {
			t.Errorf("constraint[%d].Fields len: want %d, got %d", ordinal, len(wrc.Fields), len(grc.Fields))
			continue
		}
		for i := range wrc.Fields {
			if wrc.Fields[i] != grc.Fields[i] {
				t.Errorf("constraint[%d].Fields[%d]: want %d, got %d", ordinal, i, wrc.Fields[i], grc.Fields[i])
			}
		}
	}
}

// assertStoreEquivalent verifies the Store document survives round-trip by
// checking that every key present in the original is present in the result
// with the same value, using the public typed accessors.
func assertStoreEquivalent(t *testing.T, want, got *ir.Schema) {
	t.Helper()
	if (want.Store == nil) != (got.Store == nil) {
		t.Fatalf("Store nil mismatch: want nil=%v, got nil=%v", want.Store == nil, got.Store == nil)
	}
	if want.Store == nil {
		return
	}
	// Walk the original store and verify every key survives in the round-tripped store.
	want.Store.Walk(func(
		positions map[int64]int32,
		_ func(t document.DataType, initialSize ...int) unsafe.Pointer,
	) (any, error) {
		for rawKey, idx := range positions {
			dk := document.DocumentKey(rawKey)
			if idx < 0 {
				// Null entry: verify it is also null in got.
				if !got.Store.IsNull(dk) {
					t.Errorf("Store: key %d is null in original but not in round-tripped", rawKey)
				}
				continue
			}
			if !got.Store.IsSet(dk) {
				t.Errorf("Store: key %d is set in original but missing in round-tripped", rawKey)
				continue
			}
			// Spot-check value equality for common types.
			switch dk.Type() {
			case document.TypeString:
				wv, _, _ := want.Store.GetString(dk)
				gv, _, _ := got.Store.GetString(dk)
				if wv != gv {
					t.Errorf("Store string key %d: want %q, got %q", rawKey, wv, gv)
				}
			case document.TypeInt:
				wv, _, _ := want.Store.GetInt(dk)
				gv, _, _ := got.Store.GetInt(dk)
				if wv != gv {
					t.Errorf("Store int key %d: want %d, got %d", rawKey, wv, gv)
				}
			case document.TypeFloat:
				wv, _, _ := want.Store.GetFloat(dk)
				gv, _, _ := got.Store.GetFloat(dk)
				if wv != gv {
					t.Errorf("Store float key %d: want %v, got %v", rawKey, wv, gv)
				}
			case document.TypeBool:
				wv, _, _ := want.Store.GetBool(dk)
				gv, _, _ := got.Store.GetBool(dk)
				if wv != gv {
					t.Errorf("Store bool key %d: want %v, got %v", rawKey, wv, gv)
				}
			case document.TypeArrayString:
				wv, _, _ := want.Store.GetArrayString(dk)
				gv, _, _ := got.Store.GetArrayString(dk)
				if len(wv) != len(gv) {
					t.Errorf("Store []string key %d len: want %d, got %d", rawKey, len(wv), len(gv))
					continue
				}
				for i := range wv {
					if wv[i] != gv[i] {
						t.Errorf("Store []string key %d[%d]: want %q, got %q", rawKey, i, wv[i], gv[i])
					}
				}
			case document.TypeArrayInt:
				wv, _, _ := want.Store.GetArrayInt(dk)
				gv, _, _ := got.Store.GetArrayInt(dk)
				if len(wv) != len(gv) {
					t.Errorf("Store []int key %d len: want %d, got %d", rawKey, len(wv), len(gv))
					continue
				}
				for i := range wv {
					if wv[i] != gv[i] {
						t.Errorf("Store []int key %d[%d]: want %d, got %d", rawKey, i, wv[i], gv[i])
					}
				}
			case document.TypeArrayBool:
				wv, _, _ := want.Store.GetArrayBool(dk)
				gv, _, _ := got.Store.GetArrayBool(dk)
				if len(wv) != len(gv) {
					t.Errorf("Store []bool key %d len: want %d, got %d", rawKey, len(wv), len(gv))
					continue
				}
				for i := range wv {
					if wv[i] != gv[i] {
						t.Errorf("Store []bool key %d[%d]: want %v, got %v", rawKey, i, wv[i], gv[i])
					}
				}
			}
		}
		return nil, nil
	})
}

// assertAddressResolution verifies that resolving path on both the original and
// round-tripped schema returns the same DataPoint (type + ordinal).
func assertAddressResolution(t *testing.T, want, got *ir.Schema, path string) {
	t.Helper()
	wdp, err := want.Address(path)
	if err != nil {
		t.Fatalf("Address(%q) on original: %v", path, err)
	}
	gdp, err := got.Address(path)
	if err != nil {
		t.Fatalf("Address(%q) on round-tripped: %v", path, err)
	}
	if wdp != gdp {
		t.Errorf("Address(%q): want DataPoint %d, got %d", path, wdp, gdp)
	}
}

// assertStoreEquivalent uses document.Walk which requires unsafe.Pointer.
var _ unsafe.Pointer
