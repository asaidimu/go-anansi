package anansi

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
)

// This file implements the decode half of spec section 2.5 (Field Type
// Encoding) for each of the 16 container.DataType values, plus two
// deliberate, documented adaptations to this concrete engine (see the
// readRecord and readArrayObjectField comments):
//
//   - TypeRecord in this engine is a schema-free map[string]any (see
//     core/data/container's DataType doc comment), not a *DataContainer
//     bound to a known schema as the abstract spec's "TypeRecord
//     (*DataContainer)" describes. It is encoded/decoded the same
//     self-describing way as TypeUnknown.
//   - TypeArrayObject elements genuinely are schema-bound *DataContainer
//     values, each encoded as [payload_length varint][nested anansi packet
//     bytes] against the field's child schema slot.
//
// The encode half lives in inverted.go as size twins + binPut writers
// mirroring every layout below byte-for-byte.

func readInt(r *byteReader) (int64, error) { return r.readVarint() }

func readFloat(r *byteReader) (float64, error) {
	b, err := r.readN(8)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(getUint64LE(b)), nil
}

func getUint64LE(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

// readString reads a length-prefixed string. With aliasing enabled on the
// reader (zero-copy decode), it returns an immutable view into the reader's
// buffer — which the decoder has attached as the container's backing.
func readString(r *byteReader) (string, error) {
	n, err := r.readUvarint()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	b, err := r.readN(int(n))
	if err != nil {
		return "", err
	}
	if !r.alias {
		return string(b), nil
	}
	// Zero-copy: an immutable view into the container's backing buffer.
	// Equal values are intentionally NOT deduplicated — per-string map
	// hashing costs more than the duplicates save on realistic payloads.
	s := unsafe.String(&b[0], len(b))
	return s, nil
}

func readBoolSparse(r *byteReader) (bool, error) {
	b, err := r.readByte()
	if err != nil {
		return false, err
	}
	return b != 0, nil
}

// readBoolBits reads back n LSB-first packed bools written by putBoolBits.
func readBoolBits(r *byteReader, n int) ([]bool, error) {
	packed, err := r.readN((n + 7) / 8)
	if err != nil {
		return nil, err
	}
	out := make([]bool, n)
	for i := range out {
		out[i] = packed[i/8]&(1<<uint(i%8)) != 0
	}
	return out, nil
}

func readBytes(r *byteReader) ([]byte, error) {
	n, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	b, err := r.readN(int(n))
	if err != nil {
		return nil, err
	}
	// Copy out: the underlying buffer may be reused/short-lived by the
	// caller, and container storage must own its bytes.
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

func readGeometry(r *byteReader) ([][]float64, error) {
	nRings, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	rings := make([][]float64, 0, nRings)
	for i := uint64(0); i < nRings; i++ {
		nPoints, err := r.readUvarint()
		if err != nil {
			return nil, err
		}
		ring := make([]float64, 0, nPoints*2)
		for p := uint64(0); p < nPoints; p++ {
			x, err := readFloat(r)
			if err != nil {
				return nil, err
			}
			y, err := readFloat(r)
			if err != nil {
				return nil, err
			}
			ring = append(ring, x, y)
		}
		rings = append(rings, ring)
	}
	return rings, nil
}

// ---------------------------------------------------------------------------
// Array types (spec 2.5 TypeArray*)
// ---------------------------------------------------------------------------

func readArrayInt(r *byteReader) ([]int64, error) {
	n, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, n)
	for i := uint64(0); i < n; i++ {
		x, err := readInt(r)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

func readArrayFloat(r *byteReader) ([]float64, error) {
	n, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	out := make([]float64, 0, n)
	for i := uint64(0); i < n; i++ {
		x, err := readFloat(r)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

func readArrayString(r *byteReader) ([]string, error) {
	n, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, n)
	for i := uint64(0); i < n; i++ {
		s, err := readString(r)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func readArrayBool(r *byteReader) ([]bool, error) {
	n, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	nBytes := (int(n) + 7) / 8
	packed, err := r.readN(nBytes)
	if err != nil {
		return nil, err
	}
	out := make([]bool, n)
	for i := range out {
		out[i] = packed[i/8]&(1<<uint(i%8)) != 0
	}
	return out, nil
}

func readArrayBytes(r *byteReader) ([][]byte, error) {
	n, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	out := make([][]byte, 0, n)
	for i := uint64(0); i < n; i++ {
		b, err := readBytes(r)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func readArrayGeometry(r *byteReader) ([][][]float64, error) {
	n, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	out := make([][][]float64, 0, n)
	for i := uint64(0); i < n; i++ {
		g, err := readGeometry(r)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Generic tagged "any" decoding, used for TypeUnknown, TypeArrayUnknown, and
// (as a documented adaptation) this engine's schema-free TypeRecord.
// ---------------------------------------------------------------------------

// anyTag mirrors container.DataType's iota values (spec 2.5 TypeUnknown:
// "type_tag corresponds to the DataType iota value of the actual runtime
// type"), plus a dedicated tag for Go's untyped nil since TypeUnknown's own
// iota (0) must still be distinguishable from "no value".
type anyTag = uint8

const (
	tagNull    anyTag = 0 // explicit nil carried inside an "any" slot
	tagInt     anyTag = uint8(container.TypeInt)
	tagFloat   anyTag = uint8(container.TypeFloat)
	tagString  anyTag = uint8(container.TypeString)
	tagBool    anyTag = uint8(container.TypeBool)
	tagBytes   anyTag = uint8(container.TypeBytes)
	tagGeom    anyTag = uint8(container.TypeGeometry)
	tagRecord  anyTag = uint8(container.TypeRecord)
	tagArrAny  anyTag = uint8(container.TypeArrayUnknown)
	tagArrInt  anyTag = uint8(container.TypeArrayInt)
	tagArrFlt  anyTag = uint8(container.TypeArrayFloat)
	tagArrStr  anyTag = uint8(container.TypeArrayString)
	tagArrBool anyTag = uint8(container.TypeArrayBool)
)

func readAny(r *byteReader) (any, error) {
	tag, err := r.readByte()
	if err != nil {
		return nil, err
	}
	switch tag {
	case tagNull:
		return nil, nil
	case tagBool:
		return readBoolSparse(r)
	case tagString:
		return readString(r)
	case tagInt:
		return readInt(r)
	case tagFloat:
		return readFloat(r)
	case tagBytes:
		return readBytes(r)
	case tagGeom:
		return readGeometry(r)
	case tagRecord:
		return readRecordBody(r)
	case tagArrAny:
		n, err := r.readUvarint()
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, n)
		for i := uint64(0); i < n; i++ {
			e, err := readAny(r)
			if err != nil {
				return nil, err
			}
			out = append(out, e)
		}
		return out, nil
	case tagArrInt:
		return readArrayInt(r)
	case tagArrFlt:
		return readArrayFloat(r)
	case tagArrStr:
		return readArrayString(r)
	case tagArrBool:
		return readArrayBool(r)
	default:
		return nil, fmt.Errorf("anansi: unknown type tag %d in tagged value", tag)
	}
}

// readRecordBody decodes a schema-free map[string]any written by putRecord:
// a varint entry count followed by (key, tagged value) pairs sorted by key
// for a deterministic byte-for-byte encoding of the same logical map.
func readRecordBody(r *byteReader) (map[string]any, error) {
	n, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, n)
	for i := uint64(0); i < n; i++ {
		k, err := readString(r)
		if err != nil {
			return nil, err
		}
		v, err := readAny(r)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func readRecord(r *byteReader) (map[string]any, error) {
	return readRecordBody(r)
}

// ---------------------------------------------------------------------------
// Nested packets
// ---------------------------------------------------------------------------

// decodeNestedPacket decodes one nested TypeArrayObject element sub-packet
// (header + optional nested hash + body) into doc (spec 2.5 TypeArrayObject:
// per element [payload_length][nested anansi packet bytes]). res is the
// element slot's prebuilt resource tree. Nested packets never carry
// transforms (the encoder strips those bits), so there is nothing to open
// beyond the optional integrity digest.
func decodeNestedPacket(res *slotCodec, r *byteReader, doc *container.DataContainer, pool *container.Pool) error {
	h, err := readHeader(r)
	if err != nil {
		return err
	}
	if h.Flags.Compressed() || h.Flags.Encrypted() {
		return fmt.Errorf("anansi: compressed/encrypted nested packets are not supported by this codec")
	}
	if err := readAndVerifyNestedHash(r, h.Flags); err != nil {
		return err
	}
	switch h.Flags.PacketType() {
	case PacketDense:
		return decodeDenseBody(r, res.cs, h, doc, res, pool)
	case PacketSparse:
		return decodeSparseBody(r, res.cs, h, doc, res, pool)
	default:
		return fmt.Errorf("anansi: unsupported nested packet type %s", h.Flags.PacketType())
	}
}

// readArrayObjectField decodes an array of schema-bound child documents.
// Each element is an independently-framed complete Anansi packet. The
// element slot's prebuilt resource tree means no per-element schema walks.
func readArrayObjectField(r *byteReader, res *slotCodec, pool *container.Pool) ([]*container.DataContainer, error) {
	n, err := r.readUvarint()
	if err != nil {
		return nil, err
	}
	out := make([]*container.DataContainer, 0, n)
	for i := uint64(0); i < n; i++ {
		length, err := r.readUvarint()
		if err != nil {
			return nil, err
		}
		payload, err := r.readN(int(length))
		if err != nil {
			return nil, err
		}
		var child *container.DataContainer
		if pool != nil {
			child = pool.Get()
		} else {
			child = container.NewDataContainer()
		}
		if r.alias {
			// Element strings alias the root backing; attach it so an
			// extracted child remains valid independently of its parent.
			child.OwnBacking(r.data)
		}
		if err := decodeNestedPacket(res, r.child(payload), child, pool); err != nil {
			return nil, fmt.Errorf("anansi: decode array-object element %d: %w", i, err)
		}
		out = append(out, child)
	}
	return out, nil
}
