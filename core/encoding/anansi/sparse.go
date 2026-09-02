package anansi

import (
	"bytes"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// encodeSparseBody writes the Sparse packet body (spec 3.2.2): a varint
// field count, then for every set field (value-bearing or null, in
// canonical wire order) its DataPoint (null bit set for null fields)
// followed by the value bytes (omitted for null fields).
//
// positions is the document's captured positions map (positionsOf): both
// the counting pass and the writing pass read the same map directly — no
// per-field container calls anywhere on this path.
func encodeSparseBody(buf *bytes.Buffer, cs *definition.CompiledSchema, version header, doc *container.DataContainer, positions map[int64]int32, res *slotCodec) error {
	// First pass: count set fields so field_count can be written before the
	// entries (spec 3.2.2 puts field_count up front).
	setCount := 0
	for _, wf := range res.fields {
		if stateAt(positions, wf.key) != stateNotSet {
			setCount++
		}
	}
	writeUvarintTo(buf, uint64(setCount))

	for _, wf := range res.fields {
		state := stateAt(positions, wf.key)
		if state == stateNotSet {
			continue
		}
		dp := wf.key.DataPoint()
		if state == stateNull {
			dp |= 1 // set the null bit (spec 3.2.1)
		} else {
			dp &^= 1 // canonical (non-null) DataPoint for value-bearing fields
		}
		writeUvarintTo(buf, uint64(uint32(dp)))
		if state == stateHasValue {
			if err := writeFieldPayload(buf, cs, version, doc, wf); err != nil {
				return fmt.Errorf("anansi: encode sparse field %q: %w", wf.name, err)
			}
		}
	}
	return nil
}

// decodeSparseBody reads a Sparse packet body (spec 3.2.2) from r into doc.
// Each wire DataPoint is matched back to its wireField via the schema
// resources' canonical-DataPoint index (masking off the null bit for the
// comparison), so decoding does not depend on field order matching encode
// order — only on both sides sharing the same compiled schema (and
// therefore the same field->key mapping). The index is built once per
// schema in the resource tree instead of per packet.
func decodeSparseBody(r *byteReader, cs *definition.CompiledSchema, version header, doc *container.DataContainer, res *slotCodec, pool *container.Pool) error {
	n, err := r.readUvarint()
	if err != nil {
		return fmt.Errorf("anansi: read sparse field count: %w", err)
	}
	for i := uint64(0); i < n; i++ {
		raw, err := r.readUvarint()
		if err != nil {
			return fmt.Errorf("anansi: read sparse data point %d: %w", i, err)
		}
		wireDP := int32(uint32(raw))
		isNull := wireDP&1 != 0
		canonical := wireDP &^ 1

		wf, ok := res.byDP[canonical]
		if !ok {
			return fmt.Errorf("anansi: sparse packet references unknown data point %d (canonical %d)", raw, canonical)
		}
		if isNull {
			doc.SetNull(wf.key)
			continue
		}
		if err := readFieldPayload(r, cs, doc, *wf, pool); err != nil {
			return fmt.Errorf("anansi: decode sparse field %q: %w", wf.name, err)
		}
	}
	return nil
}
