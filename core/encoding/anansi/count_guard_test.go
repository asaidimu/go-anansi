package anansi

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/require"
)

// maxUvarint is the LEB128 encoding of math.MaxUint64: the largest count a
// hostile packet can put on the wire.
var maxUvarint = []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01}

// denseOnly builds a Dense packet (spec 3.1) in which exactly the named
// field has a value, followed by payload as that field's value bytes.
func denseOnly(t *testing.T, cs *definition.CompiledSchema, field string, payload []byte) []byte {
	t.Helper()
	res, err := resourcesFor(cs)
	require.NoError(t, err)
	idx := -1
	for i := range res.fields {
		if res.fields[i].name == field {
			idx = i
			break
		}
	}
	require.GreaterOrEqual(t, idx, 0, "field %q not in the canonical wire order", field)
	state := make([]byte, (2*len(res.fields)+7)/8)
	state[idx*2/8] |= stateBitsHasValue << uint((idx*2)%8)
	pkt := append([]byte{0x00, 0x01}, state...)
	return append(pkt, payload...)
}

// TestDecodeRejectsOversizedElementCounts guards spec 9.2.1 ("Validate all
// length fields before allocation"): wire-supplied counts must be rejected
// as errors — not turned into huge preallocations or a "makeslice: cap out
// of range" panic — before the bytes they claim to describe are checked.
func TestDecodeRejectsOversizedElementCounts(t *testing.T) {
	cs := loadTestSchema(t)
	recordOfMaxTaggedArray := []byte{
		0x01,       // record: 1 entry
		0x00,       // key: "" (length 0)
		0x08,       // type tag: TypeArrayUnknown
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01, // element count: MaxUint64
	}
	cases := []struct {
		name    string
		field   string
		payload []byte
	}{
		{"array_int count past end of input", "ids", maxUvarint},
		{"array_int count beyond its bytes", "ids", []byte{0x02}},
		{"array_string count past end of input", "tags", maxUvarint},
		{"array_float count past end of input", "scores", maxUvarint},
		{"array_bytes count past end of input", "blobs", maxUvarint},
		{"array_object count past end of input", "items", maxUvarint},
		{"array_bool count past available bits", "flags", []byte{0x09, 0x00}},
		{"record entry count past end of input", "meta", maxUvarint},
		{"tagged array inside record", "meta", recordOfMaxTaggedArray},
		{"geometry ring count past end of input", "shape", maxUvarint},
		{"geometry point count past end of input", "shape", append([]byte{0x01}, maxUvarint...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := DecodeAnansi(cs, denseOnly(t, cs, tc.field, tc.payload))
			require.Error(t, err)
		})
	}
}

// TestDecodeAcceptsBoundaryCounts is the complement: counts that exactly
// match the bytes behind them must still decode, byte-for-byte.
func TestDecodeAcceptsBoundaryCounts(t *testing.T) {
	cs := loadTestSchema(t)
	cases := []struct {
		name    string
		field   string
		payload []byte
	}{
		{"empty array_int", "ids", []byte{0x00}},
		{"empty record", "meta", []byte{0x00}},
		{"empty geometry", "shape", []byte{0x00}},
		{"array_bool exactly filling one byte", "flags", []byte{0x08, 0xFF}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkt := denseOnly(t, cs, tc.field, tc.payload)
			doc, _, err := DecodeAnansi(cs, pkt)
			require.NoError(t, err)
			out, err := EncodeDense(cs, doc, 1)
			require.NoError(t, err)
			require.Equal(t, pkt, out)
		})
	}
}

// TestDecodeRejectsOversizedBatchRecordCount guards the Batch decoder's own
// docs-slice allocation (spec 9.2.1): record_count is bounded by the bytes
// that can describe it before make() runs.
func TestDecodeRejectsOversizedBatchRecordCount(t *testing.T) {
	cs := loadTestSchema(t)

	t.Run("row batch count past end of input", func(t *testing.T) {
		pkt := append([]byte{0x02, 0x01}, maxUvarint...)
		pkt = append(pkt, 0x00) // batch_flags: row, dense
		_, _, err := DecodeBatchRows(cs, pkt, nil)
		require.Error(t, err)
	})

	t.Run("row batch count with no record bytes", func(t *testing.T) {
		// count=100 over an 18-field schema: every record spans ≥ 1 byte,
		// but none follow.
		pkt := []byte{0x02, 0x01, 0x64, 0x00}
		_, _, err := DecodeBatchRows(cs, pkt, nil)
		require.Error(t, err)
	})

	t.Run("zero-field schema falls back to the fixed ceiling", func(t *testing.T) {
		empty, err := definition.FromJSON([]byte(`{"name":"Empty","version":"1.0.0","fields":{}}`))
		require.NoError(t, err)
		rs, err := definition.Compile(empty)
		require.NoError(t, err)
		ecs, err := definition.Link(rs)
		require.NoError(t, err)

		pkt := append([]byte{0x02, 0x01}, maxUvarint...)
		pkt = append(pkt, 0x00)
		_, _, err = DecodeBatchRows(ecs, pkt, nil)
		require.Error(t, err)
	})
}

// TestBatchRoundTripsAroundCountGuards pins the acceptance side of the new
// validation: legitimate batches at the interesting edges still decode.
func TestBatchRoundTripsAroundCountGuards(t *testing.T) {
	cs := loadTestSchema(t)

	t.Run("row batch", func(t *testing.T) {
		res, err := resourcesFor(cs)
		require.NoError(t, err)
		keys := map[string]container.DataContainerKey{}
		for _, wf := range res.fields {
			keys[wf.name] = wf.key
		}
		d1 := container.NewDataContainer()
		require.NoError(t, d1.SetInt(keys["count"], 1))
		d2 := container.NewDataContainer()
		require.NoError(t, d2.SetString(keys["title"], "x"))
		docs := []*container.DataContainer{d1, d2}
		wire, err := EncodeBatchRows(cs, docs, 3)
		require.NoError(t, err)
		back, v, err := DecodeBatchRows(cs, wire, nil)
		require.NoError(t, err)
		require.Equal(t, uint16(3), v)
		require.Len(t, back, 2)
	})

	t.Run("columnar batch all fields not set", func(t *testing.T) {
		// Every record contributes only state-column bits here, so the
		// record count is bounded by ceil(2·N·R/8) bytes rather than by
		// one byte per record — the tightest point of the count guard.
		docs := make([]*container.DataContainer, 1000)
		for i := range docs {
			docs[i] = container.NewDataContainer()
		}
		wire, err := EncodeBatchColumnar(cs, docs, 0)
		require.NoError(t, err)
		back, _, err := DecodeBatchRows(cs, wire, nil)
		require.NoError(t, err)
		require.Len(t, back, 1000)
	})

	t.Run("zero-field schema batch", func(t *testing.T) {
		empty, err := definition.FromJSON([]byte(`{"name":"Empty","version":"1.0.0","fields":{}}`))
		require.NoError(t, err)
		rs, err := definition.Compile(empty)
		require.NoError(t, err)
		ecs, err := definition.Link(rs)
		require.NoError(t, err)

		docs := []*container.DataContainer{
			container.NewDataContainer(),
			container.NewDataContainer(),
			container.NewDataContainer(),
		}
		wire, err := EncodeBatchRows(ecs, docs, 0)
		require.NoError(t, err)
		back, _, err := DecodeBatchRows(ecs, wire, nil)
		require.NoError(t, err)
		require.Len(t, back, 3)
	})
}
