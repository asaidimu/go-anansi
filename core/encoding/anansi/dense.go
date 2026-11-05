package anansi

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// stateNotSetBits, stateNullBits, stateHasValueBits are the 2-bit state map
// codes (spec 3.1.2). 0b11 is reserved and never produced by this codec.
const (
	stateBitsNotSet   = 0b00
	stateBitsNull     = 0b01
	stateBitsHasValue = 0b10
)

// decodeDenseBody reads a Dense packet body (state map + value blocks,
// spec 3.1) from r into doc, given the slot resources and factory to use for any
// nested TypeArrayObject children.
//
// The decode path is built to the same standard as the encoder's populate
// pass (inverted.go), resolving #dcd001 and its children:
//
//   - #dcd002: the state map is read in place via stateAtBit — no
//     []fieldState is materialized (mirroring columnar.go).
//   - #dcd003: one validation pass over the state map yields the exact
//     per-DataType value counts, and every used type slot is pre-sized from
//     them in a single Walk — one allocation per used type, zero append
//     growth, exactly like the encoder sizing its output from the same map.
//   - #dcd004: values are written through the typed AppendXxx API instead of
//     SetXxx, skipping the positions-map probe and hole-claim scan that are
//     guaranteed no-ops on a decode target (fresh, Clear()ed, or factory-fresh —
//     the direct-write contract Walk documents).
//   - #dcd006: the type dispatch is hoisted out of the per-field loop; the
//     scalar types (int/float/string) read and append inline, the complex
//     types share fillValue.
//   - #dcd007: the bool block is unpacked in place — no intermediate []bool.
func decodeDenseBody(r *byteReader, cs *definition.CompiledSchema, version header, doc *container.DataContainer, res *slotCodec, factory container.Factory) error {
	packed, err := r.readN((2*len(res.fields) + 7) / 8)
	if err != nil {
		return fmt.Errorf("anansi: read dense state map: %w", err)
	}

	// Validate the state codes and count exact per-type value counts in one
	// pass over the canonical wire order.
	var counts [16]int
	for i := range res.fields {
		switch stateAtBit(packed, i*2) {
		case stateNotSet, stateNull:
		case stateHasValue:
			counts[res.fields[i].fd.DataType()]++
		default:
			return fmt.Errorf("anansi: reserved state map code 0b11 at field index %d", i)
		}
	}

	// Pre-size every used type slot from the counted values (#dcd003). A
	// fresh container allocates each typed slice exactly once; a reused or
	// pooled one keeps its retained capacity (PreSize is a no-op then).
	for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
		if counts[dt] > 0 {
			doc.PreSize(dt, counts[dt])
		}
	}

	for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
		typeFields := res.byType[dt]
		if len(typeFields) == 0 {
			continue
		}
		if dt == container.TypeBool {
			// The type's values share one LSB-first bit block (spec 2.5) in
			// wire order, so values map back one-to-one; unpack in place.
			values, err := r.readN((counts[dt] + 7) / 8)
			if err != nil {
				return fmt.Errorf("anansi: read dense bool block: %w", err)
			}
			next := 0
			for _, wf := range typeFields {
				if stateAtBit(packed, wf.idx*2) != stateHasValue {
					continue
				}
				v := values[next/8]&(1<<uint(next%8)) != 0
				next++
				if err := doc.AppendBool(wf.key, v); err != nil {
					return fmt.Errorf("anansi: decode dense field %q: %w", wf.name, err)
				}
			}
			continue
		}
		switch dt {
		case container.TypeInt:
			for _, wf := range typeFields {
				switch stateAtBit(packed, wf.idx*2) {
				case stateNotSet:
					// Nothing stored; leave the field absent.
				case stateNull:
					doc.SetNull(wf.key)
				case stateHasValue:
					v, err := readInt(r)
					if err != nil {
						return fmt.Errorf("anansi: decode dense field %q: %w", wf.name, err)
					}
					if err := doc.AppendInt(wf.key, v); err != nil {
						return fmt.Errorf("anansi: decode dense field %q: %w", wf.name, err)
					}
				}
			}
		case container.TypeFloat:
			for _, wf := range typeFields {
				switch stateAtBit(packed, wf.idx*2) {
				case stateNotSet:
				case stateNull:
					doc.SetNull(wf.key)
				case stateHasValue:
					v, err := readFloat(r)
					if err != nil {
						return fmt.Errorf("anansi: decode dense field %q: %w", wf.name, err)
					}
					if err := doc.AppendFloat(wf.key, v); err != nil {
						return fmt.Errorf("anansi: decode dense field %q: %w", wf.name, err)
					}
				}
			}
		case container.TypeString:
			for _, wf := range typeFields {
				switch stateAtBit(packed, wf.idx*2) {
				case stateNotSet:
				case stateNull:
					doc.SetNull(wf.key)
				case stateHasValue:
					v, err := readString(r)
					if err != nil {
						return fmt.Errorf("anansi: decode dense field %q: %w", wf.name, err)
					}
					if err := doc.AppendString(wf.key, v); err != nil {
						return fmt.Errorf("anansi: decode dense field %q: %w", wf.name, err)
					}
				}
			}
		default:
			for _, wf := range typeFields {
				switch stateAtBit(packed, wf.idx*2) {
				case stateNotSet:
					// Nothing stored; leave the field absent.
				case stateNull:
					doc.SetNull(wf.key)
				case stateHasValue:
					if err := fillValue(r, cs, doc, wf, factory); err != nil {
						return fmt.Errorf("anansi: decode dense field %q: %w", wf.name, err)
					}
				}
			}
		}
	}
	return nil
}
