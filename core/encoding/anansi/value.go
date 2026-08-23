package anansi

import (
	"bytes"
	"fmt"
	"math"
	"sort"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// This file implements spec section 2.5 (Field Type Encoding) for each of the
// 16 container.DataType values, plus two deliberate, documented adaptations
// to this concrete engine (see writeRecord/readRecord and
// writeArrayObjectField/readArrayObjectField below):
//
//   - TypeRecord in this engine is a schema-free map[string]any (see
//     core/data/container's DataType doc comment), not a *DataContainer
//     bound to a known schema as the abstract spec's "TypeRecord
//     (*DataContainer)" describes. There is no schema to recursively encode
//     it against, so it is encoded the same self-describing way as
//     TypeUnknown: a recursive, tagged encoding of whatever Go value the map
//     holds (nil/int64/float64/string/bool/[]byte/map[string]any/[]any/etc).
//   - TypeArrayObject elements genuinely are schema-bound *DataContainer
//     values, so each element is encoded exactly as the spec describes:
//     [payload_length varint][nested anansi packet bytes], recursing into
//     this same codec against the field's child schema slot.

// writeInt encodes TypeInt (spec 2.5 TypeInt): zigzag varint.
func writeInt(buf *bytes.Buffer, v int64) { writeVarintTo(buf, v) }

func readInt(r *byteReader) (int64, error) { return r.readVarint() }

// writeFloat encodes TypeFloat (spec 2.5 TypeFloat): 8 bytes little-endian
// IEEE 754.
func writeFloat(buf *bytes.Buffer, v float64) {
	var tmp [8]byte
	putUint64LE(tmp[:], math.Float64bits(v))
	buf.Write(tmp[:])
}

func readFloat(r *byteReader) (float64, error) {
	b, err := r.readN(8)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(getUint64LE(b)), nil
}

func putUint64LE(b []byte, v uint64) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	b[4] = byte(v >> 32)
	b[5] = byte(v >> 40)
	b[6] = byte(v >> 48)
	b[7] = byte(v >> 56)
}

func getUint64LE(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

// writeString encodes TypeString (spec 2.5 TypeString): varint length
// followed by raw UTF-8 bytes.
func writeString(buf *bytes.Buffer, v string) {
	writeUvarintTo(buf, uint64(len(v)))
	buf.WriteString(v)
}

func readString(r *byteReader) (string, error) {
	n, err := r.readUvarint()
	if err != nil {
		return "", err
	}
	b, err := r.readN(int(n))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// writeBoolSparse encodes a single TypeBool value the Sparse way (spec 2.5
// TypeBool): one byte, 0x00/0x01.
func writeBoolSparse(buf *bytes.Buffer, v bool) {
	if v {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
}

func readBoolSparse(r *byteReader) (bool, error) {
	b, err := r.readByte()
	if err != nil {
		return false, err
	}
	return b != 0, nil
}

// writeBytes encodes TypeBytes (spec 2.5 TypeBytes): varint length + raw
// bytes.
func writeBytes(buf *bytes.Buffer, v []byte) {
	writeUvarintTo(buf, uint64(len(v)))
	buf.Write(v)
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

// writeGeometry encodes TypeGeometry (spec 2.5 TypeGeometry): ring count,
// then per ring a point count and float64-LE (x, y) pairs.
func writeGeometry(buf *bytes.Buffer, rings [][]float64) {
	writeUvarintTo(buf, uint64(len(rings)))
	for _, ring := range rings {
		npoints := len(ring) / 2
		writeUvarintTo(buf, uint64(npoints))
		for i := 0; i+1 < len(ring); i += 2 {
			writeFloat(buf, ring[i])
			writeFloat(buf, ring[i+1])
		}
	}
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

func writeArrayInt(buf *bytes.Buffer, v []int64) {
	writeUvarintTo(buf, uint64(len(v)))
	for _, x := range v {
		writeInt(buf, x)
	}
}

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

func writeArrayFloat(buf *bytes.Buffer, v []float64) {
	writeUvarintTo(buf, uint64(len(v)))
	for _, x := range v {
		writeFloat(buf, x)
	}
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

func writeArrayString(buf *bytes.Buffer, v []string) {
	writeUvarintTo(buf, uint64(len(v)))
	for _, s := range v {
		writeString(buf, s)
	}
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

// writeArrayBool encodes []bool (spec 2.5 TypeArrayBool): count, then
// ceil(count/8) bytes of LSB-first packed bits.
func writeArrayBool(buf *bytes.Buffer, v []bool) {
	writeUvarintTo(buf, uint64(len(v)))
	nBytes := (len(v) + 7) / 8
	packed := make([]byte, nBytes)
	for i, b := range v {
		if b {
			packed[i/8] |= 1 << uint(i%8)
		}
	}
	buf.Write(packed)
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

func writeArrayBytes(buf *bytes.Buffer, v [][]byte) {
	writeUvarintTo(buf, uint64(len(v)))
	for _, b := range v {
		writeBytes(buf, b)
	}
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

func writeArrayGeometry(buf *bytes.Buffer, v [][][]float64) {
	writeUvarintTo(buf, uint64(len(v)))
	for _, g := range v {
		writeGeometry(buf, g)
	}
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
// Generic tagged "any" encoding, used for TypeUnknown, TypeArrayUnknown, and
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

// writeAny writes a self-describing tagged value (spec 2.5 TypeUnknown).
func writeAny(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteByte(tagNull)
	case bool:
		buf.WriteByte(tagBool)
		writeBoolSparse(buf, t)
	case string:
		buf.WriteByte(tagString)
		writeString(buf, t)
	case int:
		buf.WriteByte(tagInt)
		writeInt(buf, int64(t))
	case int64:
		buf.WriteByte(tagInt)
		writeInt(buf, t)
	case float64:
		buf.WriteByte(tagFloat)
		writeFloat(buf, t)
	case []byte:
		buf.WriteByte(tagBytes)
		writeBytes(buf, t)
	case [][]float64:
		buf.WriteByte(tagGeom)
		writeGeometry(buf, t)
	case map[string]any:
		buf.WriteByte(tagRecord)
		return writeRecordBody(buf, t)
	case []any:
		buf.WriteByte(tagArrAny)
		writeUvarintTo(buf, uint64(len(t)))
		for _, e := range t {
			if err := writeAny(buf, e); err != nil {
				return err
			}
		}
	case []int64:
		buf.WriteByte(tagArrInt)
		writeArrayInt(buf, t)
	case []float64:
		buf.WriteByte(tagArrFlt)
		writeArrayFloat(buf, t)
	case []string:
		buf.WriteByte(tagArrStr)
		writeArrayString(buf, t)
	case []bool:
		buf.WriteByte(tagArrBool)
		writeArrayBool(buf, t)
	default:
		return fmt.Errorf("anansi: unsupported value type %T for TypeUnknown/TypeRecord slot", v)
	}
	return nil
}

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

// writeRecordBody encodes a schema-free map[string]any: a varint entry
// count followed by (key, tagged value) pairs sorted by key for a
// deterministic byte-for-byte encoding of the same logical map.
func writeRecordBody(buf *bytes.Buffer, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	writeUvarintTo(buf, uint64(len(keys)))
	for _, k := range keys {
		writeString(buf, k)
		if err := writeAny(buf, m[k]); err != nil {
			return fmt.Errorf("anansi: record key %q: %w", k, err)
		}
	}
	return nil
}

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

// writeRecord encodes TypeRecord. See the file-level doc comment: this
// engine's TypeRecord is a schema-free map[string]any, so it is encoded as a
// self-describing tagged body directly (no outer type tag needed, since the
// container/schema already know this slot is a record).
func writeRecord(buf *bytes.Buffer, m map[string]any) error {
	return writeRecordBody(buf, m)
}

func readRecord(r *byteReader) (map[string]any, error) {
	return readRecordBody(r)
}

// ---------------------------------------------------------------------------
// TypeArrayObject: [count varint] then per element [payload_length varint]
// [nested anansi packet bytes] (spec 2.5 TypeArrayObject), recursing into
// this codec against the field's child schema slot.
// ---------------------------------------------------------------------------

// writeArrayObjectField encodes an array of schema-bound child documents.
// Each element is independently encoded as its own complete Anansi packet
// (auto-selecting Dense/Sparse per element), matching the spec's per-element
// [payload_length][anansi_packet_bytes] framing.
func writeArrayObjectField(buf *bytes.Buffer, cs *definition.CompiledSchema, version header, children []*container.DataContainer, childIdx uint8, childPath definition.ResolvedPath) error {
	writeUvarintTo(buf, uint64(len(children)))
	for i, child := range children {
		payload, err := encodePacket(cs, childIdx, child, version, childPath)
		if err != nil {
			return fmt.Errorf("anansi: encode array-object element %d: %w", i, err)
		}
		writeUvarintTo(buf, uint64(len(payload)))
		buf.Write(payload)
	}
	return nil
}

func readArrayObjectField(r *byteReader, cs *definition.CompiledSchema, childIdx uint8, childPath definition.ResolvedPath, pool *container.Pool) ([]*container.DataContainer, error) {
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
		if err := decodePacketInto(cs, childIdx, payload, child, pool, childPath); err != nil {
			return nil, fmt.Errorf("anansi: decode array-object element %d: %w", i, err)
		}
		out = append(out, child)
	}
	return out, nil
}
