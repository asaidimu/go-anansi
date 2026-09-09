package anansi

import (
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

// decodeDenseBody reads a Dense packet body (state map + value blocks,
// spec 3.1) from r into doc, given the slot resources and pool to use for any
// nested TypeArrayObject children.
func decodeDenseBody(r *byteReader, cs *definition.CompiledSchema, version header, doc *container.DataContainer, res *slotCodec, pool *container.Pool) error {
	states, err := decodeDenseStateMap(r, len(res.fields))
	if err != nil {
		return err
	}

	for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
		if dt == container.TypeBool {
			// bools iterate their block in wire order (byType preserves
			// canonical order), so values map back one-to-one.
			var count int
			for _, wf := range res.byType[dt] {
				if states[wf.idx] == stateHasValue {
					count++
				}
			}
			values, err := readBoolBits(r, count)
			if err != nil {
				return fmt.Errorf("anansi: read dense bool block: %w", err)
			}
			next := 0
			for _, wf := range res.byType[dt] {
				if states[wf.idx] != stateHasValue {
					continue
				}
				if err := doc.SetBool(wf.key, values[next]); err != nil {
					return err
				}
				next++
			}
			continue
		}
		for _, wf := range res.byType[dt] {
			switch states[wf.idx] {
			case stateNotSet:
				// Nothing stored; leave the field absent.
			case stateNull:
				doc.SetNull(wf.key)
			case stateHasValue:
				if err := readFieldPayload(r, cs, doc, wf, pool); err != nil {
					return fmt.Errorf("anansi: decode dense field %q: %w", wf.name, err)
				}
			}
		}
	}
	return nil
}
