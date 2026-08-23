package anansi

import (
	"bytes"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// Columnar Batch packets (spec 3.3.3): after the common 2-byte header
// (flags bit 3 set), record_count varint and a batch_flags byte with bit 0
// (batchFlagColumnar) set, the body is laid out per DataType in iota order.
// A DataType with at least one wire field contributes one block:
//
//	[State Map Column] — field-major: for each field of this type, in
//	  canonical wire order, record_count consecutive 2-bit entries
//	  (00 not-set / 01 null / 10 has-value); the column is byte-aligned
//	  (padded with 00 bits).
//	[Value Array] × each field of this type — the payload bytes of every
//	  record whose state is HasValue, in record order. Payload encoding is
//	  identical to the row-oriented packets (spec 2.5 via writeFieldPayload).
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

// EncodeBatchColumnar encodes docs as a single columnar Batch packet
// (spec 3.3.3). All documents must be bound to schema slot 0 of cs.
func EncodeBatchColumnar(cs *definition.CompiledSchema, docs []*container.DataContainer, fullVersion uint16, opts ...EncodeOption) ([]byte, error) {
	epoch, version, err := schemaVersionByte(fullVersion)
	if err != nil {
		return nil, err
	}
	h := header{Version: version}
	h.Flags = Flags(epoch)<<flagEpochShift | newFlags(PacketBatch, true)

	fields, err := collectWireFieldsCached(cs, rootSlot, nil)
	if err != nil {
		return nil, err
	}

	for i, doc := range docs {
		if doc == nil {
			return nil, fmt.Errorf("anansi: encode columnar batch: document at index %d is nil", i)
		}
	}

	buf := bytes.NewBuffer(nil)
	writeUvarintTo(buf, uint64(len(docs)))
	buf.WriteByte(batchFlagColumnar)

	for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
		typeFields := typeFieldsOf(fields, dt)
		if len(typeFields) == 0 {
			continue
		}
		if err := encodeColumnarBlock(buf, cs, h, docs, typeFields); err != nil {
			return nil, err
		}
	}
	return finishFrame(h, buf.Bytes(), newEncodeConfig(opts))
}

// encodeColumnarBlock writes one DataType's block: the byte-aligned state
// map column followed by one value array per field.
func encodeColumnarBlock(buf *bytes.Buffer, cs *definition.CompiledSchema, h header, docs []*container.DataContainer, typeFields []wireField) error {
	nBits := 2 * len(typeFields) * len(docs)
	packed := make([]byte, (nBits+7)/8)
	bit := 0
	for _, wf := range typeFields {
		for _, doc := range docs {
			var code byte
			switch fieldStateOf(doc, wf.key) {
			case stateNotSet:
				code = stateBitsNotSet
			case stateNull:
				code = stateBitsNull
			case stateHasValue:
				code = stateBitsHasValue
			}
			packed[bit/8] |= code << uint(bit%8)
			bit += 2
		}
	}
	buf.Write(packed)

	// TypeBool value arrays are bit-packed per field (spec 2.5 Dense Mode).
	if typeFields[0].fd.DataType() == container.TypeBool {
		for _, wf := range typeFields {
			var values []bool
			for _, doc := range docs {
				if fieldStateOf(doc, wf.key) != stateHasValue {
					continue
				}
				v, _, err := doc.GetBool(wf.key)
				if err != nil {
					return fmt.Errorf("anansi: encode columnar field %q: %w", wf.name, err)
				}
				values = append(values, v)
			}
			writeBoolBits(buf, values)
		}
		return nil
	}

	for _, wf := range typeFields {
		for _, doc := range docs {
			if fieldStateOf(doc, wf.key) != stateHasValue {
				continue
			}
			if err := writeFieldPayload(buf, cs, h, doc, wf); err != nil {
				return fmt.Errorf("anansi: encode columnar field %q: %w", wf.name, err)
			}
		}
	}
	return nil
}

// decodeColumnarBatch decodes a columnar Batch body (after header,
// record_count and batch_flags) into pre-allocated docs, using pool for any
// nested TypeArrayObject children.
func decodeColumnarBatch(r *byteReader, cs *definition.CompiledSchema, h header, docs []*container.DataContainer, fields []wireField, pool *container.Pool) error {
	for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
		typeFields := typeFieldsOf(fields, dt)
		if len(typeFields) == 0 {
			continue
		}
		if err := decodeColumnarBlock(r, cs, h, docs, typeFields, pool); err != nil {
			return err
		}
	}
	return nil
}

func decodeColumnarBlock(r *byteReader, cs *definition.CompiledSchema, h header, docs []*container.DataContainer, typeFields []wireField, pool *container.Pool) error {
	nBits := 2 * len(typeFields) * len(docs)
	packed, err := r.readN((nBits + 7) / 8)
	if err != nil {
		return fmt.Errorf("anansi: read columnar state column: %w", err)
	}

	states := make([][]fieldState, len(typeFields))
	bit := 0
	for fi := range typeFields {
		states[fi] = make([]fieldState, len(docs))
		for ri := range docs {
			code := (packed[bit/8] >> uint(bit%8)) & 0x03
			switch code {
			case stateBitsNotSet:
				states[fi][ri] = stateNotSet
			case stateBitsNull:
				states[fi][ri] = stateNull
			case stateBitsHasValue:
				states[fi][ri] = stateHasValue
			default:
				return fmt.Errorf("anansi: reserved state code 0b11 for field %q record %d", typeFields[fi].name, ri)
			}
			bit += 2
		}
	}

	// TypeBool value arrays are bit-packed per field; see encodeColumnarBlock.
	if typeFields[0].fd.DataType() == container.TypeBool {
		for fi, wf := range typeFields {
			var count int
			for ri := range docs {
				if states[fi][ri] == stateHasValue {
					count++
				}
			}
			values, err := readBoolBits(r, count)
			if err != nil {
				return fmt.Errorf("anansi: read columnar bool array for field %q: %w", wf.name, err)
			}
			next := 0
			for ri := range docs {
				if states[fi][ri] != stateHasValue {
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

	for fi, wf := range typeFields {
		for ri := range docs {
			switch states[fi][ri] {
			case stateNotSet:
				// Leave absent.
			case stateNull:
				docs[ri].SetNull(wf.key)
			case stateHasValue:
				if err := readFieldPayload(r, cs, h, docs[ri], wf, pool); err != nil {
					return fmt.Errorf("anansi: decode columnar field %q record %d: %w", wf.name, ri, err)
				}
			}
		}
	}
	return nil
}

// typeFieldsOf returns the subsequence of fields whose descriptor DataType
// equals dt, preserving canonical wire order.
func typeFieldsOf(fields []wireField, dt container.DataType) []wireField {
	out := make([]wireField, 0, len(fields))
	for _, wf := range fields {
		if wf.fd.DataType() == dt {
			out = append(out, wf)
		}
	}
	return out
}
