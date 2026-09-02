package anansi

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// Columnar Batch packets (spec 3.3.3): after the common 2-byte header
// (flags bit 3 set), record_count varint and a batch_flags byte with bit 0
// (batchFlagColumnar) set, the body is laid out per DataType in iota order.
// A DataType with at least one wire field contributes one block:
//
//      [State Map Column] — field-major: for each field of this type, in
//        canonical wire order, record_count consecutive 2-bit entries
//        (00 not-set / 01 null / 10 has-value); the column is byte-aligned
//        (padded with 00 bits).
//      [Value Array] × each field of this type — the payload bytes of every
//        record whose state is HasValue, in record order. Payload encoding is
//        identical to the row-oriented packets (spec 2.5).
//
// Deviation from the spec's illustrative "fixed-width raw bytes" note
// (3.3.3): TypeInt stays zigzag-varint, exactly as everywhere else in this
// codec (spec 2.5 governs value encodings; 3.3.3's note is advisory).
// TypeBool value arrays ARE bit-packed (spec 2.5 TypeBool Dense Mode): each
// field's present values pack LSB-first into ceil(n/8) bytes, mirroring the
// Dense packet block. The columnar win here is locality — a single field's
// values are contiguous — not fixed-width int packing.
//
// Datatypes with no schema fields contribute no bytes at all, so block
// boundaries are implied by the compiled schema alone (self-delineating,
// like Dense).
//
// The encoder follows the inverted two-pass discipline (inverted.go): scan
// sizes every block from the captured rows (one positions lookup per
// field per record), the packet is allocated exactly, and populate writes
// each field's value array in place — the per-field sub-writer is bounded
// by the scanned size by construction.

// EncodeBatchColumnar encodes docs as a single columnar Batch packet
// (spec 3.3.3). All documents must be bound to schema slot 0 of cs.
func EncodeBatchColumnar(cs *definition.CompiledSchema, docs []*container.DataContainer, fullVersion uint16, opts ...EncodeOption) ([]byte, error) {
	epoch, version, err := schemaVersionByte(fullVersion)
	if err != nil {
		return nil, err
	}
	res, err := resourcesFor(cs)
	if err != nil {
		return nil, err
	}
	cfg := newEncodeConfig(opts)

	for i, doc := range docs {
		if doc == nil {
			return nil, fmt.Errorf("anansi: encode columnar batch: document at index %d is nil", i)
		}
	}

	h := header{Version: version}
	h.Flags = Flags(epoch)<<flagEpochShift | newFlags(PacketBatch, true)

	arena := plansPool.Get().(*plansBuf)
	defer putPlans(arena)

	// One capture per record, shared by every per-DataType block.
	views := make([]*docView, len(docs))
	for i := range docs {
		views[i] = viewDoc(docs[i], res, arena)
	}

	// Pass 1: exact body size = record_count varint + batch_flags byte +
	// per DataType [state column + per-field value arrays].
	bodySize := uvarintLen(uint64(len(docs))) + 1
	for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
		typeFields := res.byType[dt]
		if len(typeFields) == 0 {
			continue
		}
		bodySize += (2*len(typeFields)*len(docs) + 7) / 8 // state column
		if dt == container.TypeBool {
			for _, wf := range typeFields {
				n := 0
				for i := range views {
					if views[i].rows[wf.idx].st == stateHasValue {
						n++
					}
				}
				bodySize += (n + 7) / 8
			}
			continue
		}
		for _, wf := range typeFields {
			for i := range views {
				if rs := views[i].rows[wf.idx]; rs.st == stateHasValue {
					bodySize += scanValue(views[i], &wf, rs, arena)
				}
			}
		}
	}
	if arena.err != nil {
		return nil, arena.err
	}

	populate := func(buf []byte, start int) {
		w := binPut{buf: buf, pos: start}
		w.uvarint(uint64(len(docs)))
		w.buf[w.pos] = batchFlagColumnar
		w.pos++

		for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
			typeFields := res.byType[dt]
			if len(typeFields) == 0 {
				continue
			}
			// State column: field-major, byte-aligned.
			nB := (2*len(typeFields)*len(docs) + 7) / 8
			w.zero(nB)
			base := w.pos
			for fi := range typeFields {
				wf := &typeFields[fi]
				for ri := range views {
					var code byte
					switch views[ri].rows[wf.idx].st {
					case stateNull:
						code = stateBitsNull
					case stateHasValue:
						code = stateBitsHasValue
					default:
						code = stateBitsNotSet
					}
					bit := (fi*len(docs) + ri) * 2
					w.buf[base+bit/8] |= code << uint(bit%8)
				}
			}
			w.pos += nB

			if dt == container.TypeBool {
				for _, wf := range typeFields {
					// Gather present values in record order into the arena
					// scratch and bit-pack them.
					n := 0
					for i := range views {
						if views[i].rows[wf.idx].st == stateHasValue {
							n++
						}
					}
					nB := (n + 7) / 8
					if nB > 0 {
						w.zero(nB)
						off := w.pos
						next := 0
						for i := range views {
							rs := views[i].rows[wf.idx]
							if rs.st != stateHasValue {
								continue
							}
							if views[i].boolAt(rs) {
								w.buf[off+next/8] |= 1 << uint(next%8)
							}
							next++
						}
						w.pos += nB
					}
				}
				continue
			}

			for fi := range typeFields {
				wf := &typeFields[fi]
				for ri := range views {
					if rs := views[ri].rows[wf.idx]; rs.st == stateHasValue {
						putValue(&w, views[ri], wf, rs, h, arena)
					}
				}
			}
		}
		if w.pos != start+bodySize {
			panic(fmt.Sprintf("anansi: inverted columnar encoder size mismatch: scanned %d body bytes, wrote %d", bodySize, w.pos-start))
		}
	}
	return finishInverted(h, bodySize, cfg, arena, populate)
}

// decodeColumnarBatch decodes a columnar Batch body (after header,
// record_count and batch_flags) into pre-allocated docs, using pool for any
// nested TypeArrayObject children. The field-major state column is parsed
// in place — no per-field state slices are materialized.
func decodeColumnarBatch(r *byteReader, docs []*container.DataContainer, res *slotCodec, pool *container.Pool) error {
	for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
		if len(res.byType[dt]) == 0 {
			continue
		}
		if err := decodeColumnarBlock(r, res.cs, docs, res.byType[dt], pool); err != nil {
			return err
		}
	}
	return nil
}

// stateAtBit reads the 2-bit state code at the given bit position of a
// packed field-major state column without materializing per-field state
// slices.
func stateAtBit(packed []byte, bit int) fieldState {
	code := (packed[bit/8] >> uint(bit%8)) & 0x03
	return fieldState(code)
}

func decodeColumnarBlock(r *byteReader, cs *definition.CompiledSchema, docs []*container.DataContainer, typeFields []wireField, pool *container.Pool) error {
	nBits := 2 * len(typeFields) * len(docs)
	packed, err := r.readN((nBits + 7) / 8)
	if err != nil {
		return fmt.Errorf("anansi: read columnar state column: %w", err)
	}

	// TypeBool value arrays are bit-packed per field; see EncodeBatchColumnar.
	if typeFields[0].fd.DataType() == container.TypeBool {
		for fi := range typeFields {
			wf := &typeFields[fi]
			var count int
			for ri := range docs {
				if stateAtBit(packed, (fi*len(docs)+ri)*2) == stateHasValue {
					count++
				}
			}
			values, err := readBoolBits(r, count)
			if err != nil {
				return fmt.Errorf("anansi: read columnar bool array for field %q: %w", wf.name, err)
			}
			next := 0
			for ri := range docs {
				if stateAtBit(packed, (fi*len(docs)+ri)*2) != stateHasValue {
					continue
				}
				if err := docs[ri].SetBool(wf.key, values[next]); err != nil {
					return err
				}
				next++
			}
		}
		return nil
	}

	for fi := range typeFields {
		wf := &typeFields[fi]
		base := fi * len(docs) * 2
		for ri := range docs {
			switch stateAtBit(packed, base+ri*2) {
			case stateNotSet:
				// Leave absent.
			case stateNull:
				docs[ri].SetNull(wf.key)
			case stateHasValue:
				if err := readFieldPayload(r, cs, docs[ri], *wf, pool); err != nil {
					return fmt.Errorf("anansi: decode columnar field %q record %d: %w", wf.name, ri, err)
				}
			}
		}
	}
	return nil
}
