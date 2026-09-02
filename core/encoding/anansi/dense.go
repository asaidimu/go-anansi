package anansi

import (
	"bytes"
	"fmt"
	"unsafe"

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

// decodeDenseStateMap reads back the per-field state codes written by
// the encoder.
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
		switch fieldState(code) {
		case stateNotSet:
			out[i] = stateNotSet
		case stateNull:
			out[i] = stateNull
		case stateHasValue:
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
//
// The TypeBool block is bit-packed (spec 2.5 TypeBool Dense Mode): all
// present bools, in wire order, packed LSB-first into ceil(n/8) bytes.
func encodeDenseBody(buf *bytes.Buffer, cs *definition.CompiledSchema, version header, doc *container.DataContainer, fields []wireField) error {
	// Use Walk to get positions directly
	var positions map[int64]int32
	doc.Walk(func(p map[int64]int32, _ func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		positions = p
		return nil, nil
	})

	// Pre-allocate state map - all zeros (stateNotSet) by default
	nBits := 2 * len(fields)
	nBytes := (nBits + 7) / 8
	var stackBuf [32]byte
	var stateMap []byte
	if nBytes <= len(stackBuf) {
		stateMap = stackBuf[:nBytes]
	} else {
		stateMap = make([]byte, nBytes)
	}

	// Set bits for present fields in one pass through positions
	for i, wf := range fields {
		if idx, exists := positions[int64(wf.key)]; exists {
			var code byte
			if idx < 0 {
				code = stateBitsNull
			} else {
				code = stateBitsHasValue
			}
			bitPos := i * 2
			stateMap[bitPos/8] |= code << uint(bitPos%8)
		}
	}
	buf.Write(stateMap)

	// Write values in DataType order (original approach)
	for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
		if dt == container.TypeBool {
			var values []bool
			for _, wf := range fields {
				if wf.fd.DataType() != dt || fieldStateOf(doc, wf.key) != stateHasValue {
					continue
				}
				v, _, err := doc.GetBool(wf.key)
				if err != nil {
					return fmt.Errorf("anansi: encode dense field %q: %w", wf.name, err)
				}
				values = append(values, v)
			}
			writeBoolBits(buf, values)
			continue
		}
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
		if dt == container.TypeBool {
			var count int
			for i, wf := range fields {
				if wf.fd.DataType() == dt && states[i] == stateHasValue {
					count++
				}
			}
			values, err := readBoolBits(r, count)
			if err != nil {
				return fmt.Errorf("anansi: read dense bool block: %w", err)
			}
			next := 0
			for i, wf := range fields {
				if wf.fd.DataType() != dt || states[i] != stateHasValue {
					continue
				}
				if err := doc.SetBool(wf.key, values[next]); err != nil {
					return err
				}
				next++
			}
			continue
		}
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
