package anansi

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

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
