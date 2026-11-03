package anansijson

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"sync"
	"unsafe"
)

// ============================================================================
// PLAN-DRIVEN ENCODER
// ============================================================================
//
// Encodes a struct directly into a byte buffer through its compiled plan: no
// per-call reflection, no boxing into container slots, no intermediate maps,
// no sort. Writers reproduce core/encoding/json's byte-exact output
// (HTML-escaped strings, whole-value float formatting, base64 bytes).

const maxPooledEncodeBuffer = 64 * 1024

var encPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 512)
		return &b
	},
}

// sliceHeader mirrors the runtime slice header; the plan stores element sizes
// and types, so arrays are walked without knowing their concrete element type.
type sliceHeader struct {
	data unsafe.Pointer
	len  int
	cap  int
}

var errNilValue = errors.New("anansijson: nil value")

// planValue resolves v to an addressable struct value for offset-based field
// access. Value inputs are copied into a fresh cell once (pointer inputs — the
// hot path — skip this).
func planValue(v any) (reflect.Value, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return reflect.Value{}, errNilValue
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("anansijson: value must be a struct, got %T", v)
	}
	if !rv.CanAddr() {
		fresh := reflect.New(rv.Type())
		fresh.Elem().Set(rv)
		rv = fresh.Elem()
	}
	return rv, nil
}

func encodeWithPlan(root *objPlan, v any) ([]byte, error) {
	rv, err := planValue(v)
	if err != nil {
		if err == errNilValue {
			// The legacy path walks zero fields of a nil pointer and emits an
			// empty document.
			return []byte("{}"), nil
		}
		return nil, err
	}

	bp := encPool.Get().(*[]byte)
	dst := (*bp)[:0]
	if cap(dst) < root.sizeHint {
		dst = make([]byte, 0, root.sizeHint)
	}

	dst, err = encodeObject(root, dst, unsafe.Pointer(rv.UnsafeAddr()))

	res := bytes.Clone(dst)
	if cap(dst) <= maxPooledEncodeBuffer {
		*bp = dst[:0]
		encPool.Put(bp)
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

// marshalAppendPlan encodes v onto dst through the compiled plan, writing
// straight into the caller's buffer: no pooled scratch, no output clone, no
// per-call garbage. On error, dst is returned truncated to its original
// length — a failed append leaves the buffer untouched.
func marshalAppendPlan(dst []byte, root *objPlan, v any) ([]byte, error) {
	rv, err := planValue(v)
	if err != nil {
		if err == errNilValue {
			return append(dst, '{', '}'), nil
		}
		return dst, err
	}
	mark := len(dst)
	out, err := encodeObject(root, dst, unsafe.Pointer(rv.UnsafeAddr()))
	if err != nil {
		return dst[:mark], err
	}
	return out, nil
}

func encodeObject(p *objPlan, dst []byte, base unsafe.Pointer) ([]byte, error) {
	dst = append(dst, '{')
	first := true
	for i := range p.fields {
		n := &p.fields[i]
		fp := unsafe.Pointer(uintptr(base) + n.offset)

		if n.omitEmpty && nodeIsZero(n, fp) {
			continue
		}

		// An object whose fields were all omitted emits nothing at all in the
		// legacy path (no leaves land in the container); roll back the key.
		objMark := -1
		prevFirst := first
		if n.kind == kObject {
			objMark = len(dst)
		}

		if !first {
			dst = append(dst, ',')
		}
		first = false
		dst = append(dst, n.keyLit...)

		var err error
		switch n.kind {
		case kString:
			dst = appendJSONString(dst, *(*string)(fp))
		case kInt:
			dst = appendInt(dst, intValue(fp, n.size))
		case kUint:
			dst = appendInt(dst, int64(uintValue(fp, n.size)))
		case kFloat:
			dst, err = appendFloat(dst, floatValue(fp, n.size))
		case kBool:
			if *(*bool)(fp) {
				dst = append(dst, "true"...)
			} else {
				dst = append(dst, "false"...)
			}
		case kBytes:
			dst = appendBase64(dst, *(*[]byte)(fp))
		case kObject:
			dst, err = encodeObject(n.child, dst, fp)
			if err == nil && len(dst) == objMark+len(n.keyLit)+2 {
				dst = dst[:objMark]
				first = prevFirst
			}
		case kArrayObject:
			dst, err = encodeArrayObject(n, dst, fp)
		case kArrayString:
			dst = encodeStringArray(n, dst, fp)
		case kArrayInt:
			dst = encodeIntArray(n, dst, fp)
		case kArrayUint:
			dst = encodeUintArray(n, dst, fp)
		case kArrayFloat:
			dst, err = encodeFloatArray(n, dst, fp)
		case kArrayBool:
			dst = encodeBoolArray(n, dst, fp)
		default:
			return nil, errRefused
		}
		if err != nil {
			return nil, err
		}
	}
	return append(dst, '}'), nil
}

// nodeIsZero mirrors the legacy walker's reflect.Value.IsZero omission for
// omitempty fields.
func nodeIsZero(n *fieldNode, fp unsafe.Pointer) bool {
	switch n.kind {
	case kString:
		return len(*(*string)(fp)) == 0
	case kInt:
		return intValue(fp, n.size) == 0
	case kUint:
		return uintValue(fp, n.size) == 0
	case kFloat:
		return floatValue(fp, n.size) == 0
	case kBool:
		return !*(*bool)(fp)
	case kBytes, kArrayString, kArrayInt, kArrayUint, kArrayFloat, kArrayBool, kArrayObject:
		return *(*unsafe.Pointer)(fp) == nil
	case kObject:
		if n.zeroFn == nil {
			return false
		}
		return n.zeroFn(fp)
	}
	return false
}

func intValue(fp unsafe.Pointer, size uint8) int64 {
	switch size {
	case 1:
		return int64(*(*int8)(fp))
	case 2:
		return int64(*(*int16)(fp))
	case 4:
		return int64(*(*int32)(fp))
	default:
		return *(*int64)(fp)
	}
}

func uintValue(fp unsafe.Pointer, size uint8) uint64 {
	switch size {
	case 1:
		return uint64(*(*uint8)(fp))
	case 2:
		return uint64(*(*uint16)(fp))
	case 4:
		return uint64(*(*uint32)(fp))
	default:
		return *(*uint64)(fp)
	}
}

func floatValue(fp unsafe.Pointer, size uint8) float64 {
	if size == 4 {
		return float64(*(*float32)(fp))
	}
	return *(*float64)(fp)
}

func encodeArrayObject(n *fieldNode, dst []byte, fp unsafe.Pointer) ([]byte, error) {
	h := (*sliceHeader)(fp)
	dst = append(dst, '[')
	for i := 0; i < h.len; i++ {
		if i > 0 {
			dst = append(dst, ',')
		}
		ep := unsafe.Pointer(uintptr(h.data) + uintptr(i)*n.elemSize)
		var err error
		dst, err = encodeObject(n.child, dst, ep)
		if err != nil {
			return nil, err
		}
	}
	return append(dst, ']'), nil
}

func encodeStringArray(n *fieldNode, dst []byte, fp unsafe.Pointer) []byte {
	h := (*sliceHeader)(fp)
	dst = append(dst, '[')
	for i := 0; i < h.len; i++ {
		if i > 0 {
			dst = append(dst, ',')
		}
		s := *(*string)(unsafe.Pointer(uintptr(h.data) + uintptr(i)*n.elemSize))
		dst = appendJSONString(dst, s)
	}
	return append(dst, ']')
}

func encodeIntArray(n *fieldNode, dst []byte, fp unsafe.Pointer) []byte {
	h := (*sliceHeader)(fp)
	dst = append(dst, '[')
	for i := 0; i < h.len; i++ {
		if i > 0 {
			dst = append(dst, ',')
		}
		ep := unsafe.Pointer(uintptr(h.data) + uintptr(i)*n.elemSize)
		dst = appendInt(dst, intValue(ep, n.size))
	}
	return append(dst, ']')
}

func encodeUintArray(n *fieldNode, dst []byte, fp unsafe.Pointer) []byte {
	h := (*sliceHeader)(fp)
	dst = append(dst, '[')
	for i := 0; i < h.len; i++ {
		if i > 0 {
			dst = append(dst, ',')
		}
		ep := unsafe.Pointer(uintptr(h.data) + uintptr(i)*n.elemSize)
		dst = appendInt(dst, int64(uintValue(ep, n.size)))
	}
	return append(dst, ']')
}

func encodeFloatArray(n *fieldNode, dst []byte, fp unsafe.Pointer) ([]byte, error) {
	h := (*sliceHeader)(fp)
	dst = append(dst, '[')
	for i := 0; i < h.len; i++ {
		if i > 0 {
			dst = append(dst, ',')
		}
		ep := unsafe.Pointer(uintptr(h.data) + uintptr(i)*n.elemSize)
		var err error
		dst, err = appendFloat(dst, floatValue(ep, n.size))
		if err != nil {
			return nil, err
		}
	}
	return append(dst, ']'), nil
}

func encodeBoolArray(n *fieldNode, dst []byte, fp unsafe.Pointer) []byte {
	h := (*sliceHeader)(fp)
	dst = append(dst, '[')
	for i := 0; i < h.len; i++ {
		if i > 0 {
			dst = append(dst, ',')
		}
		if *(*bool)(unsafe.Pointer(uintptr(h.data) + uintptr(i)*n.elemSize)) {
			dst = append(dst, "true"...)
		} else {
			dst = append(dst, "false"...)
		}
	}
	return append(dst, ']')
}

// appendJSONString escapes exactly like core/encoding/json's writeString:
// quotes, backslash, control bytes, and the HTML-sensitive <, >, &.
func appendJSONString(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' || c < 0x20 || c == '<' || c == '>' || c == '&' {
			return appendEscapedString(dst, s, i)
		}
	}
	dst = append(dst, '"')
	dst = append(dst, s...)
	return append(dst, '"')
}

func appendEscapedString(dst []byte, s string, first int) []byte {
	dst = append(dst, '"')
	dst = appendEscaped(dst, s[:first])
	dst = appendUnescapedTail(dst, s[first:])
	return append(dst, '"')
}

// appendEscaped writes the escaping for one string span (used for key
// literals at plan-build time and by the slow string path).
func appendEscaped(dst []byte, s string) []byte {
	const hex = "0123456789abcdef"
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '"' && c != '\\' && c >= 0x20 && c != '<' && c != '>' && c != '&' {
			continue
		}
		dst = append(dst, s[start:i]...)
		switch c {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		case '<':
			dst = append(dst, `\u003c`...)
		case '>':
			dst = append(dst, `\u003e`...)
		case '&':
			dst = append(dst, `\u0026`...)
		default:
			dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
		}
		start = i + 1
	}
	return append(dst, s[start:]...)
}

func appendUnescapedTail(dst []byte, s string) []byte {
	return appendEscaped(dst, s)
}

func appendInt(dst []byte, v int64) []byte {
	return strconv.AppendInt(dst, v, 10)
}

func appendFloat(dst []byte, f float64) ([]byte, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("anansijson: cannot serialize non-finite float %v", f)
	}
	format := byte('f')
	abs := math.Abs(f)
	if abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	return strconv.AppendFloat(dst, f, format, -1, 64), nil
}

func appendBase64(dst []byte, b []byte) []byte {
	dst = append(dst, '"')
	n := base64.StdEncoding.EncodedLen(len(b))
	start := len(dst)
	for i := 0; i < n; i++ {
		dst = append(dst, 0)
	}
	base64.StdEncoding.Encode(dst[start:], b)
	return append(dst, '"')
}
