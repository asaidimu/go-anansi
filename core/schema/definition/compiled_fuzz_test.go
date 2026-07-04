package definition_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

// FuzzFieldDescriptor round-trips FieldDescriptor → DataPoint → FieldDescriptor.
func FuzzFieldDescriptor(f *testing.F) {
	// Seed with a variety of field descriptor values.
	f.Add(uint32(0))
	f.Add(uint32(0xFFFFFFFF))
	f.Add(uint32(0x0F000000)) // all data types
	f.Add(uint32(0x00FC0000)) // SchemaIdx max
	f.Add(uint32(0x007F8000)) // FieldIdx max + Kind
	f.Add(uint32(0x00001F80)) // ChildSchemaIdx max
	f.Add(uint32(0x0000007F)) // flags: Required + HasDefault + Deprecated + Unique + Terminal + Nullable
	f.Add(uint32(0xFFFFFF80)) // all high bits set

	f.Fuzz(func(t *testing.T, raw uint32) {
		fd := definition.FieldDescriptor(raw)

		dp := fd.DataPoint()
		recovered := definition.FieldDescriptorFromDataPoint(dp)

		// Fields stored in bits 31-5 survive the round-trip exactly.
		if recovered.DataType() != fd.DataType() {
			t.Errorf("DataType mismatch: got %d, want %d", recovered.DataType(), fd.DataType())
		}
		if recovered.SchemaIdx() != fd.SchemaIdx() {
			t.Errorf("SchemaIdx mismatch: got %d, want %d", recovered.SchemaIdx(), fd.SchemaIdx())
		}
		if recovered.FieldIdx() != fd.FieldIdx() {
			t.Errorf("FieldIdx mismatch: got %d, want %d", recovered.FieldIdx(), fd.FieldIdx())
		}
		if recovered.ChildSchemaIdx() != fd.ChildSchemaIdx() {
			t.Errorf("ChildSchemaIdx mismatch: got %d, want %d", recovered.ChildSchemaIdx(), fd.ChildSchemaIdx())
		}
		if recovered.Kind() != fd.Kind() {
			t.Errorf("Kind mismatch: got %d, want %d", recovered.Kind(), fd.Kind())
		}
		if recovered.Required() != fd.Required() {
			t.Errorf("Required mismatch: got %v, want %v", recovered.Required(), fd.Required())
		}
		if recovered.HasDefault() != fd.HasDefault() {
			t.Errorf("HasDefault mismatch: got %v, want %v", recovered.HasDefault(), fd.HasDefault())
		}

		// Flags in bits 4-0 (Deprecated, Unique, Terminal, Nullable) are lost
		// in the DataPoint round-trip — they're not part of the ID.

		// The DataPoint must have null bit 0 (it's set at runtime, not stored).
		if dp&1 != 0 {
			t.Errorf("DataPoint null bit must be 0, got 1")
		}

		// The DataPoint's type field (bits 4-1) must match the descriptor's DataType.
		dpType := (dp >> 1) & 0xF
		fdType := (raw >> 28) & 0xF
		if dpType != fdType {
			t.Errorf("DataPoint type field mismatch: got %d, want %d", dpType, fdType)
		}

		// The DataPoint's ID field (bits 31-5) must equal descriptor bits 31-5.
		dpID := dp >> 5
		fdID := raw >> 5
		if dpID != fdID {
			t.Errorf("DataPoint ID mismatch: got %d, want %d", dpID, fdID)
		}

		// Only the low 5 bits of the FD are lost in the round-trip.
		low5 := raw & 0x1F
		if low5 != 0 {
			rawRecovered := uint32(recovered)
			if rawRecovered != raw&0xFFFFFFE0 {
				t.Errorf("Round-trip should clear low 5 bits: got %08x, want %08x", rawRecovered, raw&0xFFFFFFE0)
			}
		}
	})
}

// FuzzSchemaLink round-trips Schema JSON → Compile → Link, checking invariants.
func FuzzSchemaLink(f *testing.F) {
	// Seed corpus.
	f.Add([]byte(`{"name": "test", "version": "1.0.0", "fields": {"f1": {"name": "count", "type": "integer"}}}`))
	f.Add([]byte(`{"name": "test", "version": "1.0.0", "fields": {"f1": {"name": "name", "type": "string"}, "f2": {"name": "active", "type": "boolean"}}}`))
	f.Add([]byte(`{"name": "root", "version": "1.0.0", "fields": {"f1": {"name": "user", "type": "object", "schema": {"type": "object", "fields": {"sf1": {"name": "first", "type": "string"}, "sf2": {"name": "last", "type": "string"}}}}}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("fuzzing caused a panic: %v", r)
			}
		}()

		var schema definition.Schema
		if err := json.Unmarshal(data, &schema); err != nil {
			return
		}
		// Schema must have at least a name to be valid.
		if schema.Name == "" {
			return
		}

		rs, err := definition.Compile(&schema)
		if err != nil {
			return
		}

		cs, err := definition.Link(rs)
		if err != nil {
			t.Errorf("Link failed: %v", err)
			return
		}

		// All DataPoints must be unique.
		seen := make(map[uint32]int)
		for i, fd := range cs.Descriptors {
			dp := fd.DataPoint()
			if j, dup := seen[dp]; dup {
				t.Errorf("duplicate DataPoint %08x at index %d and %d", dp, j, i)
				return
			}
			seen[dp] = i

			// Round-trip check for each descriptor.
			recovered := definition.FieldDescriptorFromDataPoint(dp)
			if recovered.DataType() != fd.DataType() {
				t.Errorf("index %d: DataType mismatch after round-trip", i)
				return
			}
			if recovered.Kind() != fd.Kind() {
				t.Errorf("index %d: Kind mismatch after round-trip", i)
				return
			}
			if recovered.SchemaIdx() != fd.SchemaIdx() {
				t.Errorf("index %d: SchemaIdx mismatch after round-trip", i)
				return
			}
			if recovered.FieldIdx() != fd.FieldIdx() {
				t.Errorf("index %d: FieldIdx mismatch after round-trip", i)
				return
			}
			if recovered.Required() != fd.Required() {
				t.Errorf("index %d: Required mismatch after round-trip", i)
				return
			}
			if recovered.HasDefault() != fd.HasDefault() {
				t.Errorf("index %d: HasDefault mismatch after round-trip", i)
				return
			}
			if recovered.Deprecated() != fd.Deprecated() {
				t.Errorf("index %d: Deprecated mismatch after round-trip", i)
				return
			}
			if recovered.Unique() != fd.Unique() {
				t.Errorf("index %d: Unique mismatch after round-trip", i)
				return
			}
		}

		// Verify schema slot count.
		if len(cs.Schemas) == 0 {
			t.Errorf("CompiledSchema must have at least the root schema slot")
		}
	})
}
