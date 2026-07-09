package definition

// =============================================================================
// COMPILED SCHEMA BINARY SERIALIZATION
// =============================================================================
//
// SerializeCompiledSchema / DeserializeCompiledSchema persist a *CompiledSchema
// to/from a compact binary form, so a process can skip re-running Compile +
// Link on every startup and instead load a previously compiled schema from
// disk (or any other byte-oriented store).
//
// This is a cache for the *compiled artifact*, not an attempt to recover the
// original source *Schema* that produced it — information such as field
// descriptions and the author's original field ordering is already gone by
// the time a ResolvedSchema exists (see resolved_schema.go), long before this
// file ever sees the data.
//
// Format:
//
//	[4 bytes]  magic "ANSC"
//	[1 byte]   format version
//	sections...  (see SerializeCompiledSchema for the exact section order)
//
// A version mismatch on load is treated as "don't trust this cache" — the
// caller should recompile from source rather than attempt to migrate an
// old binary layout forward.
//
// document.Document (used for CompiledSchema.Defaults / CompiledSchema.Enums)
// is not reflection-serializable — its backing storage is a set of
// unsafe.Pointer-typed slices. It exposes Walk specifically so callers can
// implement custom serialization; encodeDocument/decodeDocument below do so.
//
// A note on LiteralValue and nested object/array values: LiteralValue itself
// preserves the exact concrete Go integer/float type it was constructed with
// (e.g. int8 vs int64, float32 vs float64), which this codec preserves
// losslessly via an explicit sub-type tag. Values *nested inside* a
// LiteralTypeObject/LiteralTypeArray, however, are normalized down to the
// plain JSON-shaped set (nil/string/bool/int64/float64/[]any/map[string]any)
// via reflection on decode of anything that isn't already in that shape.
// This matches the only construction path that actually populates real
// schemas (LiteralValue.UnmarshalJSON), which never produces typed
// collections in the first place — but if a LiteralValue is ever built
// programmatically with e.g. a []int nested inside an object, that nested
// value survives the round trip only in its normalized ([]any of int64) form.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/document"
)

const (
	compiledSchemaMagic        = "ANSC"
	compiledSchemaFormatVersion uint8 = 1
)

// =============================================================================
// LOW-LEVEL WRITER
// =============================================================================

type writer struct {
	buf bytes.Buffer
}

func newWriter() *writer {
	return &writer{}
}

func (w *writer) u8(v uint8) {
	w.buf.WriteByte(v)
}

func (w *writer) u16(v uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	w.buf.Write(b[:])
}

func (w *writer) u32(v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	w.buf.Write(b[:])
}

func (w *writer) u64(v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	w.buf.Write(b[:])
}

func (w *writer) i64(v int64) {
	w.u64(uint64(v))
}

func (w *writer) f64(v float64) {
	w.u64(math.Float64bits(v))
}

func (w *writer) boolean(v bool) {
	if v {
		w.u8(1)
	} else {
		w.u8(0)
	}
}

func (w *writer) bytesLP(b []byte) {
	w.u32(uint32(len(b)))
	w.buf.Write(b)
}

func (w *writer) str(s string) {
	w.bytesLP([]byte(s))
}

func (w *writer) strSlice(v []string) {
	w.u32(uint32(len(v)))
	for _, s := range v {
		w.str(s)
	}
}

func (w *writer) strSliceSlice(v [][]string) {
	w.u32(uint32(len(v)))
	for _, s := range v {
		w.strSlice(s)
	}
}

func (w *writer) bytes() []byte {
	return w.buf.Bytes()
}

// =============================================================================
// LOW-LEVEL READER
// =============================================================================

type reader struct {
	data []byte
	pos  int
}

func newReader(data []byte) *reader {
	return &reader{data: data}
}

var errUnexpectedEOF = errors.New("compiled schema deserialize: unexpected end of data")

func (r *reader) need(n int) error {
	if n < 0 || r.pos+n > len(r.data) {
		return errUnexpectedEOF
	}
	return nil
}

func (r *reader) u8() (uint8, error) {
	if err := r.need(1); err != nil {
		return 0, err
	}
	v := r.data[r.pos]
	r.pos++
	return v, nil
}

func (r *reader) u16() (uint16, error) {
	if err := r.need(2); err != nil {
		return 0, err
	}
	v := binary.BigEndian.Uint16(r.data[r.pos : r.pos+2])
	r.pos += 2
	return v, nil
}

func (r *reader) u32() (uint32, error) {
	if err := r.need(4); err != nil {
		return 0, err
	}
	v := binary.BigEndian.Uint32(r.data[r.pos : r.pos+4])
	r.pos += 4
	return v, nil
}

func (r *reader) u64() (uint64, error) {
	if err := r.need(8); err != nil {
		return 0, err
	}
	v := binary.BigEndian.Uint64(r.data[r.pos : r.pos+8])
	r.pos += 8
	return v, nil
}

func (r *reader) i64() (int64, error) {
	v, err := r.u64()
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

func (r *reader) f64() (float64, error) {
	v, err := r.u64()
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(v), nil
}

func (r *reader) boolean() (bool, error) {
	v, err := r.u8()
	if err != nil {
		return false, err
	}
	return v != 0, nil
}

func (r *reader) bytesLP() ([]byte, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if err := r.need(int(n)); err != nil {
		return nil, err
	}
	out := make([]byte, n)
	copy(out, r.data[r.pos:r.pos+int(n)])
	r.pos += int(n)
	return out, nil
}

func (r *reader) str() (string, error) {
	b, err := r.bytesLP()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *reader) strSlice() ([]string, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([]string, n)
	for i := range out {
		s, err := r.str()
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

func (r *reader) strSliceSlice() ([][]string, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([][]string, n)
	for i := range out {
		s, err := r.strSlice()
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

// =============================================================================
// GENERIC SLICE HELPERS
// =============================================================================

func encodeInt64Slice(w *writer, v []int64) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.i64(x)
	}
}

func decodeInt64Slice(r *reader) ([]int64, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([]int64, n)
	for i := range out {
		v, err := r.i64()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func encodeFloat64Slice(w *writer, v []float64) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.f64(x)
	}
}

func decodeFloat64Slice(r *reader) ([]float64, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([]float64, n)
	for i := range out {
		v, err := r.f64()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func encodeBoolSlice(w *writer, v []bool) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.boolean(x)
	}
}

func decodeBoolSlice(r *reader) ([]bool, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([]bool, n)
	for i := range out {
		v, err := r.boolean()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func encodeBytesSlice(w *writer, v [][]byte) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.bytesLP(x)
	}
}

func decodeBytesSlice(r *reader) ([][]byte, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([][]byte, n)
	for i := range out {
		v, err := r.bytesLP()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// encodeFloat2D / decodeFloat2D handle the document.TypeGeometry value shape
// ([][]float64 — a single geometry, a list of coordinate rings).
func encodeFloat2D(w *writer, v [][]float64) {
	w.u32(uint32(len(v)))
	for _, ring := range v {
		encodeFloat64Slice(w, ring)
	}
}

func decodeFloat2D(r *reader) ([][]float64, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([][]float64, n)
	for i := range out {
		ring, err := decodeFloat64Slice(r)
		if err != nil {
			return nil, err
		}
		out[i] = ring
	}
	return out, nil
}

// encodeFloat3D / decodeFloat3D handle the document.TypeArrayGeometry value
// shape ([][][]float64 — an array of geometries).
func encodeFloat3D(w *writer, v [][][]float64) {
	w.u32(uint32(len(v)))
	for _, g := range v {
		encodeFloat2D(w, g)
	}
}

func decodeFloat3D(r *reader) ([][][]float64, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([][][]float64, n)
	for i := range out {
		g, err := decodeFloat2D(r)
		if err != nil {
			return nil, err
		}
		out[i] = g
	}
	return out, nil
}

func encodeDocumentSlice(w *writer, v []*document.Document) error {
	w.u32(uint32(len(v)))
	for _, d := range v {
		if err := encodeDocument(w, d); err != nil {
			return err
		}
	}
	return nil
}

func decodeDocumentSlice(r *reader) ([]*document.Document, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([]*document.Document, n)
	for i := range out {
		d, err := decodeDocument(r)
		if err != nil {
			return nil, err
		}
		out[i] = d
	}
	return out, nil
}

// =============================================================================
// GENERIC "ANY" VALUE CODEC (for []any / map[string]any content)
// =============================================================================

// normalizeLiteralAny reduces any value accepted by ValidateLiteral down to
// the plain JSON-shaped set: nil, string, bool, int64, float64, []any, or
// map[string]any. This is a normalizing operation, not an exact-type-
// preserving one — see the file-level doc comment for why that's the right
// tradeoff here.
func normalizeLiteralAny(v any) any {
	if v == nil {
		return nil
	}

	switch vv := v.(type) {
	case string:
		return vv
	case bool:
		return vv
	case int64:
		return vv
	case float64:
		return vv
	case int:
		return int64(vv)
	case int8:
		return int64(vv)
	case int16:
		return int64(vv)
	case int32:
		return int64(vv)
	case uint:
		return int64(vv)
	case uint8:
		return int64(vv)
	case uint16:
		return int64(vv)
	case uint32:
		return int64(vv)
	case uint64:
		return int64(vv)
	case float32:
		return float64(vv)
	case []any:
		out := make([]any, len(vv))
		for i, e := range vv {
			out[i] = normalizeLiteralAny(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(vv))
		for k, e := range vv {
			out[k] = normalizeLiteralAny(e)
		}
		return out
	}

	// Reflection fallback for typed slices/maps/pointers, matching the shapes
	// ValidateLiteral itself accepts.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		n := rv.Len()
		out := make([]any, n)
		for i := 0; i < n; i++ {
			out[i] = normalizeLiteralAny(rv.Index(i).Interface())
		}
		return out
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = normalizeLiteralAny(iter.Value().Interface())
		}
		return out
	case reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return normalizeLiteralAny(rv.Elem().Interface())
	default:
		return nil
	}
}

func encodeAnyValue(w *writer, v any) error {
	switch t := normalizeLiteralAny(v).(type) {
	case nil:
		w.u8(0)
	case string:
		w.u8(1)
		w.str(t)
	case bool:
		w.u8(2)
		w.boolean(t)
	case int64:
		w.u8(3)
		w.i64(t)
	case float64:
		w.u8(4)
		w.f64(t)
	case []any:
		w.u8(5)
		if err := encodeAnySlice(w, t); err != nil {
			return err
		}
	case map[string]any:
		w.u8(6)
		if err := encodeAnyMap(w, t); err != nil {
			return err
		}
	default:
		return fmt.Errorf("compiled schema serialize: unsupported normalized any value type %T", t)
	}
	return nil
}

func decodeAnyValue(r *reader) (any, error) {
	tag, err := r.u8()
	if err != nil {
		return nil, err
	}
	switch tag {
	case 0:
		return nil, nil
	case 1:
		return r.str()
	case 2:
		return r.boolean()
	case 3:
		return r.i64()
	case 4:
		return r.f64()
	case 5:
		return decodeAnySlice(r)
	case 6:
		return decodeAnyMap(r)
	default:
		return nil, fmt.Errorf("compiled schema deserialize: unknown any-value tag %d", tag)
	}
}

func encodeAnySlice(w *writer, v []any) error {
	w.u32(uint32(len(v)))
	for _, e := range v {
		if err := encodeAnyValue(w, e); err != nil {
			return err
		}
	}
	return nil
}

func decodeAnySlice(r *reader) ([]any, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([]any, n)
	for i := range out {
		v, err := decodeAnyValue(r)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func encodeAnyMap(w *writer, v map[string]any) error {
	w.u32(uint32(len(v)))
	for k, val := range v {
		w.str(k)
		if err := encodeAnyValue(w, val); err != nil {
			return err
		}
	}
	return nil
}

func decodeAnyMap(r *reader) (map[string]any, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, n)
	for i := uint32(0); i < n; i++ {
		k, err := r.str()
		if err != nil {
			return nil, err
		}
		v, err := decodeAnyValue(r)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// =============================================================================
// LITERAL VALUE CODEC (exact-type-preserving for scalars)
// =============================================================================

const (
	intSubInt8   byte = 1
	intSubInt16  byte = 2
	intSubInt32  byte = 3
	intSubInt64  byte = 4
	intSubInt    byte = 5
	intSubUint8  byte = 6
	intSubUint16 byte = 7
	intSubUint32 byte = 8
	intSubUint64 byte = 9
	intSubUint   byte = 10
)

func encodeIntExact(w *writer, val any) error {
	switch v := val.(type) {
	case int8:
		w.u8(intSubInt8)
		w.i64(int64(v))
	case int16:
		w.u8(intSubInt16)
		w.i64(int64(v))
	case int32:
		w.u8(intSubInt32)
		w.i64(int64(v))
	case int64:
		w.u8(intSubInt64)
		w.i64(v)
	case int:
		w.u8(intSubInt)
		w.i64(int64(v))
	case uint8:
		w.u8(intSubUint8)
		w.u64(uint64(v))
	case uint16:
		w.u8(intSubUint16)
		w.u64(uint64(v))
	case uint32:
		w.u8(intSubUint32)
		w.u64(uint64(v))
	case uint64:
		w.u8(intSubUint64)
		w.u64(v)
	case uint:
		w.u8(intSubUint)
		w.u64(uint64(v))
	default:
		return fmt.Errorf("compiled schema serialize: unsupported integer literal concrete type %T", val)
	}
	return nil
}

func decodeIntExact(r *reader) (any, error) {
	sub, err := r.u8()
	if err != nil {
		return nil, err
	}
	switch sub {
	case intSubInt8:
		v, err := r.i64()
		if err != nil {
			return nil, err
		}
		return int8(v), nil
	case intSubInt16:
		v, err := r.i64()
		if err != nil {
			return nil, err
		}
		return int16(v), nil
	case intSubInt32:
		v, err := r.i64()
		if err != nil {
			return nil, err
		}
		return int32(v), nil
	case intSubInt64:
		v, err := r.i64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case intSubInt:
		v, err := r.i64()
		if err != nil {
			return nil, err
		}
		return int(v), nil
	case intSubUint8:
		v, err := r.u64()
		if err != nil {
			return nil, err
		}
		return uint8(v), nil
	case intSubUint16:
		v, err := r.u64()
		if err != nil {
			return nil, err
		}
		return uint16(v), nil
	case intSubUint32:
		v, err := r.u64()
		if err != nil {
			return nil, err
		}
		return uint32(v), nil
	case intSubUint64:
		v, err := r.u64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case intSubUint:
		v, err := r.u64()
		if err != nil {
			return nil, err
		}
		return uint(v), nil
	default:
		return nil, fmt.Errorf("compiled schema deserialize: unknown integer literal sub-type %d", sub)
	}
}

const (
	floatSubFloat32 byte = 1
	floatSubFloat64 byte = 2
)

func encodeFloatExact(w *writer, val any) error {
	switch v := val.(type) {
	case float32:
		w.u8(floatSubFloat32)
		w.f64(float64(v))
	case float64:
		w.u8(floatSubFloat64)
		w.f64(v)
	default:
		return fmt.Errorf("compiled schema serialize: unsupported float literal concrete type %T", val)
	}
	return nil
}

func decodeFloatExact(r *reader) (any, error) {
	sub, err := r.u8()
	if err != nil {
		return nil, err
	}
	v, err := r.f64()
	if err != nil {
		return nil, err
	}
	switch sub {
	case floatSubFloat32:
		return float32(v), nil
	case floatSubFloat64:
		return v, nil
	default:
		return nil, fmt.Errorf("compiled schema deserialize: unknown float literal sub-type %d", sub)
	}
}

// encodeLiteralValue / decodeLiteralValue operate on LiteralValue's private
// fields directly (this file is part of package definition), since the
// public accessors alone (IsZero/IsNull/Type/Value) don't provide a way to
// reconstruct a LiteralValue from scratch.
func encodeLiteralValue(w *writer, lv LiteralValue) error {
	if lv.IsZero() {
		w.u8(0)
		return nil
	}
	if lv.IsNull() {
		w.u8(1)
		return nil
	}

	kind, err := lv.Type()
	if err != nil {
		return fmt.Errorf("compiled schema serialize: literal value: %w", err)
	}
	val := lv.Value()

	switch kind {
	case LiteralTypeString:
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("compiled schema serialize: literal string kind holds %T", val)
		}
		w.u8(2)
		w.str(s)
	case LiteralTypeInteger:
		w.u8(3)
		if err := encodeIntExact(w, val); err != nil {
			return err
		}
	case LiteralTypeFloat:
		w.u8(4)
		if err := encodeFloatExact(w, val); err != nil {
			return err
		}
	case LiteralTypeBoolean:
		b, ok := val.(bool)
		if !ok {
			return fmt.Errorf("compiled schema serialize: literal boolean kind holds %T", val)
		}
		w.u8(5)
		w.boolean(b)
	case LiteralTypeObject:
		m, ok := val.(map[string]any)
		if !ok {
			return fmt.Errorf("compiled schema serialize: literal object kind holds %T", val)
		}
		w.u8(6)
		if err := encodeAnyMap(w, m); err != nil {
			return err
		}
	case LiteralTypeArray:
		a, ok := val.([]any)
		if !ok {
			return fmt.Errorf("compiled schema serialize: literal array kind holds %T", val)
		}
		w.u8(7)
		if err := encodeAnySlice(w, a); err != nil {
			return err
		}
	default:
		return fmt.Errorf("compiled schema serialize: unsupported literal kind %v", kind)
	}
	return nil
}

func decodeLiteralValue(r *reader) (LiteralValue, error) {
	tag, err := r.u8()
	if err != nil {
		return LiteralValue{}, err
	}
	switch tag {
	case 0:
		return LiteralValue{}, nil
	case 1:
		return LiteralValue{kind: LiteralTypeNull}, nil
	case 2:
		s, err := r.str()
		if err != nil {
			return LiteralValue{}, err
		}
		return LiteralValue{kind: LiteralTypeString, value: s}, nil
	case 3:
		v, err := decodeIntExact(r)
		if err != nil {
			return LiteralValue{}, err
		}
		return LiteralValue{kind: LiteralTypeInteger, value: v}, nil
	case 4:
		v, err := decodeFloatExact(r)
		if err != nil {
			return LiteralValue{}, err
		}
		return LiteralValue{kind: LiteralTypeFloat, value: v}, nil
	case 5:
		b, err := r.boolean()
		if err != nil {
			return LiteralValue{}, err
		}
		return LiteralValue{kind: LiteralTypeBoolean, value: b}, nil
	case 6:
		m, err := decodeAnyMap(r)
		if err != nil {
			return LiteralValue{}, err
		}
		return LiteralValue{kind: LiteralTypeObject, value: m}, nil
	case 7:
		a, err := decodeAnySlice(r)
		if err != nil {
			return LiteralValue{}, err
		}
		return LiteralValue{kind: LiteralTypeArray, value: a}, nil
	default:
		return LiteralValue{}, fmt.Errorf("compiled schema deserialize: unknown literal tag %d", tag)
	}
}

// =============================================================================
// RESOLVED CONSTRAINT CODEC
// =============================================================================

func encodeResolvedConstraint(w *writer, rc ResolvedConstraint) error {
	w.str(rc.Name)
	w.u8(byte(rc.Scope))

	switch {
	case rc.Rule != nil:
		w.u8(1)
		if err := encodeResolvedConstraintRule(w, *rc.Rule); err != nil {
			return err
		}
	case rc.Group != nil:
		w.u8(2)
		if err := encodeResolvedConstraintGroup(w, *rc.Group); err != nil {
			return err
		}
	default:
		w.u8(0)
	}

	w.strSlice(rc.AbsFieldPaths)
	w.strSliceSlice(rc.AbsFieldParts)
	w.strSlice(rc.RelFieldPaths)
	w.strSliceSlice(rc.RelFieldParts)
	return nil
}

func decodeResolvedConstraint(r *reader) (ResolvedConstraint, error) {
	var rc ResolvedConstraint

	name, err := r.str()
	if err != nil {
		return rc, err
	}
	rc.Name = name

	scopeB, err := r.u8()
	if err != nil {
		return rc, err
	}
	rc.Scope = ConstraintScope(scopeB)

	kind, err := r.u8()
	if err != nil {
		return rc, err
	}
	switch kind {
	case 0:
		// neither Rule nor Group.
	case 1:
		rule, err := decodeResolvedConstraintRule(r)
		if err != nil {
			return rc, err
		}
		rc.Rule = &rule
	case 2:
		group, err := decodeResolvedConstraintGroup(r)
		if err != nil {
			return rc, err
		}
		rc.Group = &group
	default:
		return rc, fmt.Errorf("compiled schema deserialize: unknown resolved constraint kind %d", kind)
	}

	abs, err := r.strSlice()
	if err != nil {
		return rc, err
	}
	rc.AbsFieldPaths = abs

	absParts, err := r.strSliceSlice()
	if err != nil {
		return rc, err
	}
	rc.AbsFieldParts = absParts

	rel, err := r.strSlice()
	if err != nil {
		return rc, err
	}
	rc.RelFieldPaths = rel

	relParts, err := r.strSliceSlice()
	if err != nil {
		return rc, err
	}
	rc.RelFieldParts = relParts

	return rc, nil
}

func encodeResolvedConstraintRule(w *writer, rr ResolvedConstraintRule) error {
	w.str(string(rr.Predicate))
	w.u32(uint32(len(rr.Fields)))
	for _, f := range rr.Fields {
		w.str(string(f))
	}
	if err := encodeLiteralValue(w, rr.Parameters); err != nil {
		return err
	}
	return nil
}

func decodeResolvedConstraintRule(r *reader) (ResolvedConstraintRule, error) {
	var rr ResolvedConstraintRule

	pred, err := r.str()
	if err != nil {
		return rr, err
	}
	rr.Predicate = PredicateName(pred)

	n, err := r.u32()
	if err != nil {
		return rr, err
	}
	fields := make([]FieldName, n)
	for i := range fields {
		s, err := r.str()
		if err != nil {
			return rr, err
		}
		fields[i] = FieldName(s)
	}
	rr.Fields = fields

	params, err := decodeLiteralValue(r)
	if err != nil {
		return rr, err
	}
	rr.Parameters = params

	return rr, nil
}

func encodeResolvedConstraintGroup(w *writer, rg ResolvedConstraintGroup) error {
	w.u8(byte(rg.Operator))
	w.u32(uint32(len(rg.Members)))
	for _, m := range rg.Members {
		if err := encodeResolvedConstraint(w, m); err != nil {
			return err
		}
	}
	w.strSlice(rg.RequiredFieldPaths)
	w.strSliceSlice(rg.RequiredFieldParts)
	return nil
}

func decodeResolvedConstraintGroup(r *reader) (ResolvedConstraintGroup, error) {
	var rg ResolvedConstraintGroup

	opB, err := r.u8()
	if err != nil {
		return rg, err
	}
	rg.Operator = common.LogicalOperator(opB)

	n, err := r.u32()
	if err != nil {
		return rg, err
	}
	members := make([]ResolvedConstraint, n)
	for i := range members {
		m, err := decodeResolvedConstraint(r)
		if err != nil {
			return rg, err
		}
		members[i] = m
	}
	rg.Members = members

	paths, err := r.strSlice()
	if err != nil {
		return rg, err
	}
	rg.RequiredFieldPaths = paths

	parts, err := r.strSliceSlice()
	if err != nil {
		return rg, err
	}
	rg.RequiredFieldParts = parts

	return rg, nil
}

// =============================================================================
// CONSTRAINT / CONSTRAINT UNION CODEC
// =============================================================================

func encodeConstraintUnion(w *writer, cu ConstraintUnion) error {
	switch cu.kind {
	case ConstraintKindRule:
		rule, ok := cu.payload.(*ConstraintRule)
		if !ok || rule == nil {
			return fmt.Errorf("compiled schema serialize: constraint union rule payload missing")
		}
		w.u8(byte(ConstraintKindRule))
		if err := encodeConstraintRule(w, *rule); err != nil {
			return err
		}
	case ConstraintKindGroup:
		group, ok := cu.payload.(*ConstraintGroup)
		if !ok || group == nil {
			return fmt.Errorf("compiled schema serialize: constraint union group payload missing")
		}
		w.u8(byte(ConstraintKindGroup))
		if err := encodeConstraintGroup(w, *group); err != nil {
			return err
		}
	default:
		return fmt.Errorf("compiled schema serialize: unknown constraint union kind %d", cu.kind)
	}
	return nil
}

func decodeConstraintUnion(r *reader) (ConstraintUnion, error) {
	kindB, err := r.u8()
	if err != nil {
		return ConstraintUnion{}, err
	}
	switch ConstraintKind(kindB) {
	case ConstraintKindRule:
		rule, err := decodeConstraintRule(r)
		if err != nil {
			return ConstraintUnion{}, err
		}
		return ConstraintUnion{kind: ConstraintKindRule, payload: &rule}, nil
	case ConstraintKindGroup:
		group, err := decodeConstraintGroup(r)
		if err != nil {
			return ConstraintUnion{}, err
		}
		return ConstraintUnion{kind: ConstraintKindGroup, payload: &group}, nil
	default:
		return ConstraintUnion{}, fmt.Errorf("compiled schema deserialize: unknown constraint union kind %d", kindB)
	}
}

func encodeConstraintRule(w *writer, cr ConstraintRule) error {
	w.u32(uint32(len(cr.Fields)))
	for _, f := range cr.Fields {
		w.str(string(f))
	}
	w.str(string(cr.Predicate))
	if err := encodeLiteralValue(w, cr.Parameters); err != nil {
		return err
	}
	return nil
}

func decodeConstraintRule(r *reader) (ConstraintRule, error) {
	var cr ConstraintRule

	n, err := r.u32()
	if err != nil {
		return cr, err
	}
	fields := make([]FieldName, n)
	for i := range fields {
		s, err := r.str()
		if err != nil {
			return cr, err
		}
		fields[i] = FieldName(s)
	}
	cr.Fields = fields

	pred, err := r.str()
	if err != nil {
		return cr, err
	}
	cr.Predicate = PredicateName(pred)

	params, err := decodeLiteralValue(r)
	if err != nil {
		return cr, err
	}
	cr.Parameters = params

	return cr, nil
}

func encodeConstraintGroup(w *writer, cg ConstraintGroup) error {
	w.u8(byte(cg.Operator))
	w.u32(uint32(len(cg.Rules)))
	for _, ru := range cg.Rules {
		if err := encodeConstraintUnion(w, ru); err != nil {
			return err
		}
	}
	return nil
}

func decodeConstraintGroup(r *reader) (ConstraintGroup, error) {
	var cg ConstraintGroup

	opB, err := r.u8()
	if err != nil {
		return cg, err
	}
	cg.Operator = common.LogicalOperator(opB)

	n, err := r.u32()
	if err != nil {
		return cg, err
	}
	rules := make([]ConstraintUnion, n)
	for i := range rules {
		ru, err := decodeConstraintUnion(r)
		if err != nil {
			return cg, err
		}
		rules[i] = ru
	}
	cg.Rules = rules

	return cg, nil
}

func encodeConstraint(w *writer, c Constraint) error {
	w.str(c.Name)
	w.str(c.Description)
	return encodeConstraintUnion(w, c.ConstraintUnion)
}

func decodeConstraint(r *reader) (Constraint, error) {
	var c Constraint

	name, err := r.str()
	if err != nil {
		return c, err
	}
	c.Name = name

	desc, err := r.str()
	if err != nil {
		return c, err
	}
	c.Description = desc

	cu, err := decodeConstraintUnion(r)
	if err != nil {
		return c, err
	}
	c.ConstraintUnion = cu

	return c, nil
}

// =============================================================================
// SCHEMA CONSTRAINT CODEC (map[ConstraintId]Constraint)
// =============================================================================

func encodeSchemaConstraint(w *writer, sc SchemaConstraint) error {
	w.u32(uint32(len(sc)))
	for id, c := range sc {
		w.str(string(id))
		if err := encodeConstraint(w, c); err != nil {
			return err
		}
	}
	return nil
}

func decodeSchemaConstraint(r *reader) (SchemaConstraint, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	sc := make(SchemaConstraint, n)
	for i := uint32(0); i < n; i++ {
		idStr, err := r.str()
		if err != nil {
			return nil, err
		}
		c, err := decodeConstraint(r)
		if err != nil {
			return nil, err
		}
		sc[ConstraintId(idStr)] = c
	}
	return sc, nil
}

// =============================================================================
// INDEX CODEC
// =============================================================================

func encodeIndex(w *writer, idx Index) error {
	w.str(idx.Name)
	w.str(idx.Description)
	w.str(idx.Order)

	if idx.Condition.IsZero() {
		w.u8(0)
	} else {
		w.u8(1)
		if err := encodeIndexConditionUnion(w, idx.Condition); err != nil {
			return err
		}
	}

	w.u32(uint32(len(idx.Fields)))
	for _, f := range idx.Fields {
		w.str(string(f))
	}
	w.u8(byte(idx.Type))
	w.boolean(idx.Unique)
	return nil
}

func decodeIndex(r *reader) (Index, error) {
	var idx Index

	name, err := r.str()
	if err != nil {
		return idx, err
	}
	idx.Name = name

	desc, err := r.str()
	if err != nil {
		return idx, err
	}
	idx.Description = desc

	order, err := r.str()
	if err != nil {
		return idx, err
	}
	idx.Order = order

	hasCond, err := r.u8()
	if err != nil {
		return idx, err
	}
	if hasCond == 1 {
		cond, err := decodeIndexConditionUnion(r)
		if err != nil {
			return idx, err
		}
		idx.Condition = cond
	}

	n, err := r.u32()
	if err != nil {
		return idx, err
	}
	fields := make([]FieldName, n)
	for i := range fields {
		s, err := r.str()
		if err != nil {
			return idx, err
		}
		fields[i] = FieldName(s)
	}
	idx.Fields = fields

	typeB, err := r.u8()
	if err != nil {
		return idx, err
	}
	idx.Type = IndexType(typeB)

	uniq, err := r.boolean()
	if err != nil {
		return idx, err
	}
	idx.Unique = uniq

	return idx, nil
}

func encodeIndexConditionUnion(w *writer, icu IndexConditionUnion) error {
	switch icu.kind {
	case IndexConditionKindSingle:
		cond, ok := icu.payload.(*IndexCondition)
		if !ok || cond == nil {
			return fmt.Errorf("compiled schema serialize: index condition single payload missing")
		}
		w.u8(byte(IndexConditionKindSingle))
		if err := encodeIndexCondition(w, *cond); err != nil {
			return err
		}
	case IndexConditionKindGroup:
		group, ok := icu.payload.(*IndexConditionGroup)
		if !ok || group == nil {
			return fmt.Errorf("compiled schema serialize: index condition group payload missing")
		}
		w.u8(byte(IndexConditionKindGroup))
		if err := encodeIndexConditionGroup(w, *group); err != nil {
			return err
		}
	default:
		return fmt.Errorf("compiled schema serialize: unknown index condition kind %d", icu.kind)
	}
	return nil
}

func decodeIndexConditionUnion(r *reader) (IndexConditionUnion, error) {
	kindB, err := r.u8()
	if err != nil {
		return IndexConditionUnion{}, err
	}
	switch IndexConditionKind(kindB) {
	case IndexConditionKindSingle:
		cond, err := decodeIndexCondition(r)
		if err != nil {
			return IndexConditionUnion{}, err
		}
		return IndexConditionUnion{kind: IndexConditionKindSingle, payload: &cond}, nil
	case IndexConditionKindGroup:
		group, err := decodeIndexConditionGroup(r)
		if err != nil {
			return IndexConditionUnion{}, err
		}
		return IndexConditionUnion{kind: IndexConditionKindGroup, payload: &group}, nil
	default:
		return IndexConditionUnion{}, fmt.Errorf("compiled schema deserialize: unknown index condition kind %d", kindB)
	}
}

func encodeIndexCondition(w *writer, ic IndexCondition) error {
	w.str(string(ic.Field))
	if err := encodeLiteralValue(w, ic.Value); err != nil {
		return err
	}
	w.u8(byte(ic.Operator))
	return nil
}

func decodeIndexCondition(r *reader) (IndexCondition, error) {
	var ic IndexCondition

	f, err := r.str()
	if err != nil {
		return ic, err
	}
	ic.Field = FieldName(f)

	val, err := decodeLiteralValue(r)
	if err != nil {
		return ic, err
	}
	ic.Value = val

	opB, err := r.u8()
	if err != nil {
		return ic, err
	}
	ic.Operator = common.ComparisonOperator(opB)

	return ic, nil
}

func encodeIndexConditionGroup(w *writer, g IndexConditionGroup) error {
	w.u8(byte(g.Operator))
	w.u32(uint32(len(g.Conditions)))
	for _, c := range g.Conditions {
		if err := encodeIndexConditionUnion(w, c); err != nil {
			return err
		}
	}
	return nil
}

func decodeIndexConditionGroup(r *reader) (IndexConditionGroup, error) {
	var g IndexConditionGroup

	opB, err := r.u8()
	if err != nil {
		return g, err
	}
	g.Operator = common.LogicalOperator(opB)

	n, err := r.u32()
	if err != nil {
		return g, err
	}
	conds := make([]IndexConditionUnion, n)
	for i := range conds {
		c, err := decodeIndexConditionUnion(r)
		if err != nil {
			return g, err
		}
		conds[i] = c
	}
	g.Conditions = conds

	return g, nil
}

// =============================================================================
// DOCUMENT CODEC (for CompiledSchema.Defaults / CompiledSchema.Enums)
// =============================================================================

func encodeDocument(w *writer, doc *document.Document) error {
	if doc == nil {
		w.u8(0)
		return nil
	}
	w.u8(1)

	_, err := doc.Walk(func(positions map[int64]int32, slot func(t document.DataType, initialSize ...int) unsafe.Pointer) (any, error) {
		keys := make([]int64, 0, len(positions))
		for k := range positions {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

		w.u32(uint32(len(keys)))

		slotCache := make(map[document.DataType]unsafe.Pointer)
		getSlot := func(t document.DataType) unsafe.Pointer {
			if p, ok := slotCache[t]; ok {
				return p
			}
			p := slot(t)
			slotCache[t] = p
			return p
		}

		for _, k := range keys {
			idx := positions[k]
			key := document.DocumentKey(k)
			w.u64(uint64(k))
			if idx < 0 {
				w.u8(1)
				continue
			}
			w.u8(0)

			dt := key.Type()
			switch dt {
			case document.TypeInt:
				v := (*[]int64)(getSlot(dt))
				w.i64((*v)[idx])
			case document.TypeFloat:
				v := (*[]float64)(getSlot(dt))
				w.f64((*v)[idx])
			case document.TypeString:
				v := (*[]string)(getSlot(dt))
				w.str((*v)[idx])
			case document.TypeBool:
				v := (*[]bool)(getSlot(dt))
				w.boolean((*v)[idx])
			case document.TypeBytes:
				v := (*[][]byte)(getSlot(dt))
				w.bytesLP((*v)[idx])
			case document.TypeGeometry:
				v := (*[][][]float64)(getSlot(dt))
				encodeFloat2D(w, (*v)[idx])
			case document.TypeRecord:
				v := (*[]*document.Document)(getSlot(dt))
				if err := encodeDocument(w, (*v)[idx]); err != nil {
					return nil, err
				}
			case document.TypeArrayUnknown:
				v := (*[][]any)(getSlot(dt))
				if err := encodeAnySlice(w, (*v)[idx]); err != nil {
					return nil, err
				}
			case document.TypeArrayInt:
				v := (*[][]int64)(getSlot(dt))
				encodeInt64Slice(w, (*v)[idx])
			case document.TypeArrayFloat:
				v := (*[][]float64)(getSlot(dt))
				encodeFloat64Slice(w, (*v)[idx])
			case document.TypeArrayString:
				v := (*[][]string)(getSlot(dt))
				w.strSlice((*v)[idx])
			case document.TypeArrayBool:
				v := (*[][]bool)(getSlot(dt))
				encodeBoolSlice(w, (*v)[idx])
			case document.TypeArrayBytes:
				v := (*[][][]byte)(getSlot(dt))
				encodeBytesSlice(w, (*v)[idx])
			case document.TypeArrayObject:
				v := (*[][]*document.Document)(getSlot(dt))
				if err := encodeDocumentSlice(w, (*v)[idx]); err != nil {
					return nil, err
				}
			case document.TypeArrayGeometry:
				v := (*[][][][]float64)(getSlot(dt))
				encodeFloat3D(w, (*v)[idx])
			case document.TypeUnknown:
				v := (*[]any)(getSlot(dt))
				if err := encodeAnyValue(w, (*v)[idx]); err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("compiled schema serialize: unsupported document data type %d", dt)
			}
		}
		return nil, nil
	})
	if err != nil {
		return fmt.Errorf("compiled schema serialize: %w", err)
	}
	return nil
}

func decodeDocument(r *reader) (*document.Document, error) {
	presence, err := r.u8()
	if err != nil {
		return nil, err
	}
	if presence == 0 {
		return nil, nil
	}

	doc := document.NewDocument()

	count, err := r.u32()
	if err != nil {
		return nil, err
	}

	for i := uint32(0); i < count; i++ {
		kRaw, err := r.u64()
		if err != nil {
			return nil, err
		}
		key := document.DocumentKey(int64(kRaw))

		isNull, err := r.u8()
		if err != nil {
			return nil, err
		}
		if isNull == 1 {
			doc.SetNull(key)
			continue
		}

		dt := key.Type()
		switch dt {
		case document.TypeInt:
			v, err := r.i64()
			if err != nil {
				return nil, err
			}
			if err := doc.SetInt(key, v); err != nil {
				return nil, err
			}
		case document.TypeFloat:
			v, err := r.f64()
			if err != nil {
				return nil, err
			}
			if err := doc.SetFloat(key, v); err != nil {
				return nil, err
			}
		case document.TypeString:
			v, err := r.str()
			if err != nil {
				return nil, err
			}
			if err := doc.SetString(key, v); err != nil {
				return nil, err
			}
		case document.TypeBool:
			v, err := r.boolean()
			if err != nil {
				return nil, err
			}
			if err := doc.SetBool(key, v); err != nil {
				return nil, err
			}
		case document.TypeBytes:
			v, err := r.bytesLP()
			if err != nil {
				return nil, err
			}
			if err := doc.SetBytes(key, v); err != nil {
				return nil, err
			}
		case document.TypeGeometry:
			v, err := decodeFloat2D(r)
			if err != nil {
				return nil, err
			}
			if err := doc.SetGeometry(key, v); err != nil {
				return nil, err
			}
		case document.TypeRecord:
			v, err := decodeDocument(r)
			if err != nil {
				return nil, err
			}
			if err := doc.SetRecord(key, v); err != nil {
				return nil, err
			}
		case document.TypeArrayUnknown:
			v, err := decodeAnySlice(r)
			if err != nil {
				return nil, err
			}
			if err := doc.SetArrayUnknown(key, v); err != nil {
				return nil, err
			}
		case document.TypeArrayInt:
			v, err := decodeInt64Slice(r)
			if err != nil {
				return nil, err
			}
			if err := doc.SetArrayInt(key, v); err != nil {
				return nil, err
			}
		case document.TypeArrayFloat:
			v, err := decodeFloat64Slice(r)
			if err != nil {
				return nil, err
			}
			if err := doc.SetArrayFloat(key, v); err != nil {
				return nil, err
			}
		case document.TypeArrayString:
			v, err := r.strSlice()
			if err != nil {
				return nil, err
			}
			if err := doc.SetArrayString(key, v); err != nil {
				return nil, err
			}
		case document.TypeArrayBool:
			v, err := decodeBoolSlice(r)
			if err != nil {
				return nil, err
			}
			if err := doc.SetArrayBool(key, v); err != nil {
				return nil, err
			}
		case document.TypeArrayBytes:
			v, err := decodeBytesSlice(r)
			if err != nil {
				return nil, err
			}
			if err := doc.SetArrayBytes(key, v); err != nil {
				return nil, err
			}
		case document.TypeArrayObject:
			v, err := decodeDocumentSlice(r)
			if err != nil {
				return nil, err
			}
			if err := doc.SetArrayObject(key, v); err != nil {
				return nil, err
			}
		case document.TypeArrayGeometry:
			v, err := decodeFloat3D(r)
			if err != nil {
				return nil, err
			}
			if err := doc.SetArrayGeometry(key, v); err != nil {
				return nil, err
			}
		case document.TypeUnknown:
			v, err := decodeAnyValue(r)
			if err != nil {
				return nil, err
			}
			if err := doc.SetUnknown(key, v); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("compiled schema deserialize: unsupported document data type %d", dt)
		}
	}

	return doc, nil
}

// =============================================================================
// TOP-LEVEL COMPILED SCHEMA CODEC
// =============================================================================

// SerializeCompiledSchema encodes cs into a compact, self-contained binary
// form suitable for caching to disk (or any other byte store) and reloading
// via DeserializeCompiledSchema without re-running Compile + Link.
func SerializeCompiledSchema(cs *CompiledSchema) ([]byte, error) {
	if cs == nil {
		return nil, fmt.Errorf("compiled schema serialize: nil CompiledSchema")
	}

	w := newWriter()
	w.buf.WriteString(compiledSchemaMagic)
	w.u8(compiledSchemaFormatVersion)

	// Descriptors
	w.u32(uint32(len(cs.Descriptors)))
	for _, fd := range cs.Descriptors {
		w.u32(uint32(fd))
	}

	// FieldTypes
	w.u32(uint32(len(cs.FieldTypes)))
	for _, ft := range cs.FieldTypes {
		w.u8(byte(ft))
	}

	// FieldsMeta
	w.u32(uint32(len(cs.FieldsMeta)))
	for _, fm := range cs.FieldsMeta {
		w.str(fm.ID)
		w.str(fm.Name)
		w.str(fm.Path)
		w.strSlice(fm.Parts)
		w.str(fm.Description)
		if err := encodeLiteralValue(w, fm.Default); err != nil {
			return nil, fmt.Errorf("compiled schema serialize: field meta default: %w", err)
		}
	}

	// Schemas
	w.u32(uint32(len(cs.Schemas)))
	for _, s := range cs.Schemas {
		w.u16(s.FieldStart)
		w.u16(s.FieldCount)
		w.u32(s.Footprint)
	}

	// SchemasMeta
	w.u32(uint32(len(cs.SchemasMeta)))
	for _, sm := range cs.SchemasMeta {
		w.str(sm.Name)
		w.str(sm.Description)
	}

	// Defaults / Enums documents
	if err := encodeDocument(w, cs.Defaults); err != nil {
		return nil, fmt.Errorf("compiled schema serialize: defaults: %w", err)
	}
	if err := encodeDocument(w, cs.Enums); err != nil {
		return nil, fmt.Errorf("compiled schema serialize: enums: %w", err)
	}

	// Variants
	w.u32(uint32(len(cs.Variants)))
	for k, v := range cs.Variants {
		w.u32(k)
		w.u32(uint32(len(v)))
		for _, b := range v {
			w.u8(b)
		}
	}

	// Constraints
	w.u32(uint32(len(cs.Constraints)))
	for _, rc := range cs.Constraints {
		if err := encodeResolvedConstraint(w, rc); err != nil {
			return nil, fmt.Errorf("compiled schema serialize: constraints: %w", err)
		}
	}

	// Indexes
	w.u32(uint32(len(cs.Indexes)))
	for id, idx := range cs.Indexes {
		w.str(string(id))
		if err := encodeIndex(w, idx); err != nil {
			return nil, fmt.Errorf("compiled schema serialize: indexes: %w", err)
		}
	}

	// SchemaConstraints
	w.u32(uint32(len(cs.SchemaConstraints)))
	for _, sc := range cs.SchemaConstraints {
		if err := encodeSchemaConstraint(w, sc); err != nil {
			return nil, fmt.Errorf("compiled schema serialize: schema constraints: %w", err)
		}
	}

	// FieldRefConstraints
	w.u32(uint32(len(cs.FieldRefConstraints)))
	for k, sc := range cs.FieldRefConstraints {
		w.u32(k)
		if err := encodeSchemaConstraint(w, sc); err != nil {
			return nil, fmt.Errorf("compiled schema serialize: field ref constraints: %w", err)
		}
	}

	return w.bytes(), nil
}

// DeserializeCompiledSchema decodes a *CompiledSchema previously produced by
// SerializeCompiledSchema. A format-version mismatch is returned as an error
// rather than an attempted migration — callers should treat that as a signal
// to recompile from source instead of trusting the cache.
func DeserializeCompiledSchema(data []byte) (*CompiledSchema, error) {
	r := newReader(data)

	if err := r.need(len(compiledSchemaMagic)); err != nil {
		return nil, fmt.Errorf("compiled schema deserialize: %w", err)
	}
	magic := string(r.data[r.pos : r.pos+len(compiledSchemaMagic)])
	r.pos += len(compiledSchemaMagic)
	if magic != compiledSchemaMagic {
		return nil, fmt.Errorf("compiled schema deserialize: bad magic %q", magic)
	}

	version, err := r.u8()
	if err != nil {
		return nil, err
	}
	if version != compiledSchemaFormatVersion {
		return nil, fmt.Errorf(
			"compiled schema deserialize: unsupported format version %d (expected %d); recompile from source",
			version, compiledSchemaFormatVersion,
		)
	}

	cs := &CompiledSchema{}

	// Descriptors
	descCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	cs.Descriptors = make([]FieldDescriptor, descCount)
	for i := range cs.Descriptors {
		v, err := r.u32()
		if err != nil {
			return nil, err
		}
		cs.Descriptors[i] = FieldDescriptor(v)
	}

	// FieldTypes
	ftCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	cs.FieldTypes = make([]FieldType, ftCount)
	for i := range cs.FieldTypes {
		v, err := r.u8()
		if err != nil {
			return nil, err
		}
		cs.FieldTypes[i] = FieldType(v)
	}

	// FieldsMeta
	fmCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	cs.FieldsMeta = make([]FieldMeta, fmCount)
	for i := range cs.FieldsMeta {
		id, err := r.str()
		if err != nil {
			return nil, err
		}
		name, err := r.str()
		if err != nil {
			return nil, err
		}
		path, err := r.str()
		if err != nil {
			return nil, err
		}
		parts, err := r.strSlice()
		if err != nil {
			return nil, err
		}
		desc, err := r.str()
		if err != nil {
			return nil, err
		}
		def, err := decodeLiteralValue(r)
		if err != nil {
			return nil, err
		}
		cs.FieldsMeta[i] = FieldMeta{
			ID:          id,
			Name:        name,
			Path:        path,
			Parts:       parts,
			Description: desc,
			Default:     def,
		}
	}

	// Schemas
	slotCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	cs.Schemas = make([]SchemaSlot, slotCount)
	for i := range cs.Schemas {
		start, err := r.u16()
		if err != nil {
			return nil, err
		}
		count, err := r.u16()
		if err != nil {
			return nil, err
		}
		fp, err := r.u32()
		if err != nil {
			return nil, err
		}
		cs.Schemas[i] = SchemaSlot{FieldStart: start, FieldCount: count, Footprint: fp}
	}

	// SchemasMeta
	smCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	cs.SchemasMeta = make([]SchemaMeta, smCount)
	for i := range cs.SchemasMeta {
		name, err := r.str()
		if err != nil {
			return nil, err
		}
		desc, err := r.str()
		if err != nil {
			return nil, err
		}
		cs.SchemasMeta[i] = SchemaMeta{Name: name, Description: desc}
	}

	// Defaults / Enums documents
	defaults, err := decodeDocument(r)
	if err != nil {
		return nil, fmt.Errorf("compiled schema deserialize: defaults: %w", err)
	}
	cs.Defaults = defaults

	enums, err := decodeDocument(r)
	if err != nil {
		return nil, fmt.Errorf("compiled schema deserialize: enums: %w", err)
	}
	cs.Enums = enums

	// Variants
	varCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	if varCount > 0 {
		cs.Variants = make(map[uint32][]uint8, varCount)
		for i := uint32(0); i < varCount; i++ {
			k, err := r.u32()
			if err != nil {
				return nil, err
			}
			n, err := r.u32()
			if err != nil {
				return nil, err
			}
			vs := make([]uint8, n)
			for j := range vs {
				b, err := r.u8()
				if err != nil {
					return nil, err
				}
				vs[j] = b
			}
			cs.Variants[k] = vs
		}
	}

	// Constraints
	constrCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	cs.Constraints = make([]ResolvedConstraint, constrCount)
	for i := range cs.Constraints {
		rc, err := decodeResolvedConstraint(r)
		if err != nil {
			return nil, fmt.Errorf("compiled schema deserialize: constraints: %w", err)
		}
		cs.Constraints[i] = rc
	}

	// Indexes
	idxCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	if idxCount > 0 {
		cs.Indexes = make(map[IndexID]Index, idxCount)
		for i := uint32(0); i < idxCount; i++ {
			idStr, err := r.str()
			if err != nil {
				return nil, err
			}
			idx, err := decodeIndex(r)
			if err != nil {
				return nil, fmt.Errorf("compiled schema deserialize: indexes: %w", err)
			}
			cs.Indexes[IndexID(idStr)] = idx
		}
	}

	// SchemaConstraints
	scCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	cs.SchemaConstraints = make([]SchemaConstraint, scCount)
	for i := range cs.SchemaConstraints {
		sc, err := decodeSchemaConstraint(r)
		if err != nil {
			return nil, fmt.Errorf("compiled schema deserialize: schema constraints: %w", err)
		}
		cs.SchemaConstraints[i] = sc
	}

	// FieldRefConstraints
	frcCount, err := r.u32()
	if err != nil {
		return nil, err
	}
	if frcCount > 0 {
		cs.FieldRefConstraints = make(map[uint32]SchemaConstraint, frcCount)
		for i := uint32(0); i < frcCount; i++ {
			k, err := r.u32()
			if err != nil {
				return nil, err
			}
			sc, err := decodeSchemaConstraint(r)
			if err != nil {
				return nil, fmt.Errorf("compiled schema deserialize: field ref constraints: %w", err)
			}
			cs.FieldRefConstraints[k] = sc
		}
	}

	return cs, nil
}
