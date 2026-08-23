package anansi

import (
	"bytes"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// stateNotSetBits, stateNullBits, stateHasValueBits are the 2-bit state map
// codes (spec 3.1.2). 0b11 is reserved and never produced by this codec.
const (
	stateBitsNotSet   = 0b00
	stateBitsNull     = 0b01
	stateBitsHasValue = 0b10
)

// encodeDenseStateMap writes the fixed-size, byte-aligned 2-bit-per-field
// state map (spec 3.1.2) for fields (in their canonical wire order) against
// doc.
func encodeDenseStateMap(buf *bytes.Buffer, doc *container.DataContainer, fields []wireField) {
	nBits := 2 * len(fields)
	nBytes := (nBits + 7) / 8
	packed := make([]byte, nBytes)
	for i, wf := range fields {
		var code byte
		switch fieldStateOf(doc, wf.key) {
		case stateNotSet:
			code = stateBitsNotSet
		case stateNull:
			code = stateBitsNull
		case stateHasValue:
			code = stateBitsHasValue
		}
		bitPos := i * 2
		packed[bitPos/8] |= code << uint(bitPos%8)
	}
	buf.Write(packed)
}

// decodeDenseStateMap reads back the per-field state codes written by
// encodeDenseStateMap.
func decodeDenseStateMap(r *byteReader, nFields int) ([]fieldState, error) {
	nBits := 2 * nFields
	nBytes := (nBits + 7) / 8
	packed, err := r.readN(nBytes)
	if err != nil {
		return nil, fmt.Errorf("anansi: read dense state map: %w", err)
	}
	out := make([]fieldState, nFields)
	for i := 0; i < nFields; i++ {
		bitPos := i * 2
		code := (packed[bitPos/8] >> uint(bitPos%8)) & 0x03
		switch code {
		case stateBitsNotSet:
			out[i] = stateNotSet
		case stateBitsNull:
			out[i] = stateNull
		case stateBitsHasValue:
			out[i] = stateHasValue
		default:
			return nil, fmt.Errorf("anansi: reserved state map code 0b11 at field index %d", i)
		}
	}
	return out, nil
}

// encodeDenseBody writes the state map followed by per-DataType value
// blocks (spec 3.1.3) for fields against doc. It does not write the 2-byte
// packet header; callers combine this with writeHeader.
func encodeDenseBody(buf *bytes.Buffer, cs *definition.CompiledSchema, version header, doc *container.DataContainer, fields []wireField) error {
	encodeDenseStateMap(buf, doc, fields)

	// Value blocks appear in DataType iota order (spec 3.1.3): TypeUnknown
	// (0) through TypeArrayGeometry (15). Within a block, fields appear in
	// their canonical wire order (a stable subsequence of `fields`).
	for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
		for _, wf := range fields {
			if wf.fd.DataType() != dt {
				continue
			}
			if fieldStateOf(doc, wf.key) != stateHasValue {
				continue
			}
			if err := writeFieldPayload(buf, cs, version, doc, wf); err != nil {
				return fmt.Errorf("anansi: encode dense field %q: %w", wf.name, err)
			}
		}
	}
	return nil
}

// decodeDenseBody reads a Dense packet body (state map + value blocks,
// spec 3.1) from r into doc, given the field list and pool to use for any
// nested TypeArrayObject children.
func decodeDenseBody(r *byteReader, cs *definition.CompiledSchema, version header, doc *container.DataContainer, fields []wireField, pool *container.Pool) error {
	states, err := decodeDenseStateMap(r, len(fields))
	if err != nil {
		return err
	}

	for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
		for i, wf := range fields {
			if wf.fd.DataType() != dt {
				continue
			}
			switch states[i] {
			case stateNotSet:
				// Nothing stored; leave the field absent.
			case stateNull:
				doc.SetNull(wf.key)
			case stateHasValue:
				if err := readFieldPayload(r, cs, version, doc, wf, pool); err != nil {
					return fmt.Errorf("anansi: decode dense field %q: %w", wf.name, err)
				}
			}
		}
	}
	return nil
}
