package anansi

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// batchFlagColumnar and batchFlagSparse are the two bits of the Batch
// packet's own batch_flags byte (spec 3.3): bit 0 orientation, bit 1
// density.
const (
	batchFlagColumnar = 0x01
	batchFlagSparse   = 0x02
)

// EncodeBatchRows encodes docs (each bound to schema slot 0 of cs) as a
// single row-oriented Batch packet (spec 3.3.1/3.3.2), via the inverted
// two-pass encoder (inverted.go): one scan pass over all records sizes the
// packet exactly; one populate pass writes every record's body straight
// into the packet. All records use the same per-record packing (Dense or
// Sparse), chosen once via the same density heuristic as SerializeAnansi,
// evaluated over the batch's average density so a batch of mixed-density
// documents doesn't thrash between per-record formats — record boundaries
// are self-delineating either way, so the choice only affects size/speed,
// not correctness.
//
// The plansBuf arena is shared across the whole batch: row states and
// nested element plans accumulate in scan order (record 0's plans, then
// record 1's, ...) which is exactly the populate write order.
func EncodeBatchRows(cs *definition.CompiledSchema, docs []*container.DataContainer, fullVersion uint16, opts ...EncodeOption) ([]byte, error) {
	epoch, version, err := schemaVersionByte(fullVersion)
	if err != nil {
		return nil, err
	}
	res, err := resourcesFor(cs)
	if err != nil {
		return nil, err
	}
	cfg := newEncodeConfig(opts)

	h := header{Version: version}
	h.Flags = Flags(epoch)<<flagEpochShift | Flags(PacketBatch)

	arena := plansPool.Get().(*plansBuf)
	defer putPlans(arena)

	// One capture per record; density comes straight from the rows.
	views := make([]*docView, len(docs))
	for i := range docs {
		views[i] = viewDoc(docs[i], res, arena)
	}

	useSparse := false
	if len(res.fields) > 0 && len(docs) > 0 {
		var totalPresent, totalPossible int
		for i := range views {
			for j := range views[i].rows {
				if views[i].rows[j].st != stateNotSet {
					totalPresent++
				}
			}
			totalPossible += len(res.fields)
		}
		density := float64(totalPresent) / float64(totalPossible)
		useSparse = len(res.fields) > 64 && density <= 0.25
	}
	pt := PacketDense
	batchFlags := byte(0)
	if useSparse {
		pt = PacketSparse
		batchFlags |= batchFlagSparse
	}

	// Pass 1: exact body size (record_count varint + flags byte + records).
	bodySize := uvarintLen(uint64(len(docs))) + 1
	for i := range views {
		bodySize += scanPacketBodyAs(views[i], res, arena, pt)
	}
	if arena.err != nil {
		return nil, arena.err
	}

	populate := func(buf []byte, start int) {
		w := binPut{buf: buf, pos: start}
		w.uvarint(uint64(len(docs)))
		w.buf[w.pos] = batchFlags
		w.pos++
		for i := range views {
			putPacketBody(&w, views[i], res, pt, h, arena)
		}
		if w.pos != start+bodySize {
			panic(fmt.Sprintf("anansi: inverted batch encoder size mismatch: scanned %d body bytes, wrote %d", bodySize, w.pos-start))
		}
	}
	return finishInverted(h, bodySize, cfg, arena, populate)
}

// DecodeBatchRows decodes a Batch packet produced by EncodeBatchRows or
// EncodeBatchColumnar into freshly allocated (or pool-sourced, if pool is
// non-nil) *container.DataContainer values bound to schema slot 0 of cs.
// Both orientations are accepted; the packet's own batch_flags byte (with
// the header flags bit 3 as a cross-check) selects the layout. Encrypted
// packets require WithDecryptionKey; compression and integrity digests are
// handled automatically.
func DecodeBatchRows(cs *definition.CompiledSchema, data []byte, pool *container.Pool, opts ...DecodeOption) ([]*container.DataContainer, uint16, error) {
	r := newByteReader(data)
	h, err := readHeader(r)
	if err != nil {
		return nil, 0, err
	}
	if h.Flags.PacketType() != PacketBatch {
		return nil, 0, fmt.Errorf("anansi: DecodeBatchRows: packet type is %s, not Batch", h.Flags.PacketType())
	}
	cfg := newDecodeConfig(opts)
	// Spec 4.1.4/6.4.1: the transform envelope sits between the header and
	// the payload (record_count included); open it before parsing.
	body, fresh, err := openFrame(r, h.Flags, cfg)
	if err != nil {
		return nil, 0, err
	}

	recordCount, err := body.readUvarint()
	if err != nil {
		return nil, 0, fmt.Errorf("anansi: read batch record count: %w", err)
	}
	batchFlags, err := body.readByte()
	if err != nil {
		return nil, 0, fmt.Errorf("anansi: read batch flags: %w", err)
	}

	res, err := resourcesFor(cs)
	if err != nil {
		return nil, 0, err
	}

	docs := make([]*container.DataContainer, recordCount)
	for i := range docs {
		if pool != nil {
			docs[i] = pool.Get()
		} else {
			docs[i] = container.NewDataContainer()
		}
	}

	if !cfg.copyStrings && len(docs) > 0 {
		if !fresh {
			// Snapshot the caller's wire into pooled backing retained by
			// the first record; every record's strings alias this one
			// shared buffer.
			buf := docs[0].AcquireBacking(body.remaining())
			copy(buf, body.data[body.pos:])
			body = newByteReader(buf)
		}
		for _, d := range docs {
			d.OwnBacking(body.data)
		}
		body.alias = true
	}

	switch {
	case batchFlags&batchFlagColumnar != 0 || h.Flags.BatchColumnar():
		if err := decodeColumnarBatch(body, docs, res, pool); err != nil {
			return nil, 0, err
		}
	case batchFlags&batchFlagSparse != 0:
		for i := uint64(0); i < recordCount; i++ {
			if err := decodeSparseBody(body, res.cs, h, docs[i], res, pool); err != nil {
				return nil, 0, fmt.Errorf("anansi: decode batch record %d: %w", i, err)
			}
		}
	default:
		for i := uint64(0); i < recordCount; i++ {
			if err := decodeDenseBody(body, res.cs, h, docs[i], res, pool); err != nil {
				return nil, 0, fmt.Errorf("anansi: decode batch record %d: %w", i, err)
			}
		}
	}
	return docs, h.FullVersion(), nil
}
