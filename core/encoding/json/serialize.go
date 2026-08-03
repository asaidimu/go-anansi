package json

import (
	"bytes"
	"encoding/base64"
	stdjson "encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const maxPooledBufferSize = 64 * 1024

var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

var fieldEntryPool = sync.Pool{
	New: func() any {
		s := make([]fieldEntry, 0, 32)
		return &s
	},
}

type fieldEntry struct {
	name string
	key  container.DataContainerKey
	idx  int32
}

func SerializeJSON(cs *definition.CompiledSchema, doc *container.DataContainer) ([]byte, error) {
	return serializeJSONFiltered(cs, doc, "", nil)
}

// SerializeJSONCanonical serializes the container as canonical JSON for
// hashing/signing. skip holds fully-qualified schema paths to omit (e.g.
// "_metadata_.checksum"), mirroring the data-layer's canonical document map.
// Unlike the plain serializer it never builds intermediate maps: keys are
// emitted sorted and numerics written in a stable, whole-value form, directly
// from the container's typed slots.
func SerializeJSONCanonical(cs *definition.CompiledSchema, doc *container.DataContainer, skip map[string]bool) ([]byte, error) {
	return serializeJSONFiltered(cs, doc, "", skip)
}

func serializeJSONFiltered(cs *definition.CompiledSchema, doc *container.DataContainer, prefix string, skip map[string]bool) ([]byte, error) {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	if err := writeObject(cs, doc, prefix, buf, skip); err != nil {
		bufferPool.Put(buf)
		return nil, err
	}

	res := bytes.Clone(buf.Bytes())

	if buf.Cap() <= maxPooledBufferSize {
		bufferPool.Put(buf)
	}

	return res, nil
}

func writeObject(cs *definition.CompiledSchema, doc *container.DataContainer, prefix string, buf *bytes.Buffer, skip map[string]bool) error {
	entriesPtr := fieldEntryPool.Get().(*[]fieldEntry)
	entries := (*entriesPtr)[:0]

	var slotFunc func(container.DataType, ...int) unsafe.Pointer

	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		slotFunc = slot
		for k, idx := range positions {
			key := container.DataContainerKey(k)
			name := nameFor(cs, key)
			if skip[name] {
				continue
			}
			entries = append(entries, fieldEntry{
				name: name,
				key:  key,
				idx:  idx,
			})
		}
		return nil, nil
	})
	if err != nil {
		resetAndRecycleEntries(entriesPtr, entries)
		return err
	}

	if err := emitObject(cs, slotFunc, entries, prefix, buf, skip); err != nil {
		resetAndRecycleEntries(entriesPtr, entries)
		return err
	}

	resetAndRecycleEntries(entriesPtr, entries)
	return nil
}

// emitObject writes the JSON object for one container (or for a nested object
// within it). Each entry carries a fully-qualified schema path; prefix marks
// how many leading path segments describe the container currently being
// emitted, so the relative field name is what remains. Named-object children
// flatten into the same container's key space, so descendant leaves are
// grouped back into nested JSON objects; array-of-object fields are emitted as
// arrays of per-element objects keyed by the child schema's field names.
func emitObject(cs *definition.CompiledSchema, slotFunc func(container.DataType, ...int) unsafe.Pointer, entries []fieldEntry, prefix string, buf *bytes.Buffer, skip map[string]bool) error {
	groups := make(map[string][]fieldEntry, len(entries))
	var order []string
	for _, e := range entries {
		rel := stripPrefix(e.name, prefix)
		if rel == "" {
			continue
		}
		seg := firstSegment(rel)
		if _, ok := groups[seg]; !ok {
			order = append(order, seg)
		}
		groups[seg] = append(groups[seg], e)
	}
	slices.Sort(order)

	buf.WriteByte('{')
	first := true
	for _, seg := range order {
		group := groups[seg]
		var direct *fieldEntry
		var descendants []fieldEntry
		for i := range group {
			if stripPrefix(group[i].name, prefix) == seg {
				if direct != nil {
					return fmt.Errorf("json: serialize: duplicate field %q", seg)
				}
				direct = &group[i]
			} else {
				descendants = append(descendants, group[i])
			}
		}

		if !first {
			buf.WriteByte(',')
		}
		writeString(buf, seg)
		buf.WriteByte(':')
		switch {
		case direct != nil && len(descendants) > 0:
			return fmt.Errorf("json: serialize: field %q has both a value and child fields", seg)
		case direct != nil:
			if err := writeValue(cs, slotFunc, direct.key, direct.idx, joinPrefix(prefix, seg), buf, skip); err != nil {
				return err
			}
		case len(descendants) > 0:
			if err := emitObject(cs, slotFunc, descendants, joinPrefix(prefix, seg), buf, skip); err != nil {
				return err
			}
		default:
			return fmt.Errorf("json: serialize: field %q has no value", seg)
		}
		first = false
	}
	buf.WriteByte('}')
	return nil
}

func stripPrefix(name, prefix string) string {
	if prefix == "" {
		return name
	}
	return strings.TrimPrefix(name, prefix+".")
}

func firstSegment(rel string) string {
	if i := strings.IndexByte(rel, '.'); i >= 0 {
		return rel[:i]
	}
	return rel
}

func joinPrefix(prefix, seg string) string {
	if prefix == "" {
		return seg
	}
	return prefix + "." + seg
}

func resetAndRecycleEntries(ptr *[]fieldEntry, entries []fieldEntry) {
	for i := range entries {
		entries[i] = fieldEntry{}
	}
	*ptr = entries[:0]
	fieldEntryPool.Put(ptr)
}

func writeValue(cs *definition.CompiledSchema, slot func(container.DataType, ...int) unsafe.Pointer, key container.DataContainerKey, idx int32, prefix string, buf *bytes.Buffer, skip map[string]bool) error {
	if idx < 0 {
		buf.WriteString("null")
		return nil
	}

	dataType := key.Type()
	ptr := slot(dataType)

	switch dataType {
	case container.TypeInt:
		writeFormatInt(buf, (*(*[]int64)(ptr))[idx])
	case container.TypeFloat:
		return writeFloat(buf, (*(*[]float64)(ptr))[idx])
	case container.TypeString:
		writeString(buf, (*(*[]string)(ptr))[idx])
	case container.TypeBool:
		if (*(*[]bool)(ptr))[idx] {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case container.TypeBytes:
		writeBytes(buf, (*(*[][]byte)(ptr))[idx])
	case container.TypeGeometry:
		return writeFloat2D(buf, (*(*[][][]float64)(ptr))[idx])
	case container.TypeRecord:
		return writeAny(buf, (*(*[]map[string]any)(ptr))[idx])
	case container.TypeArrayInt:
		return writeIntArray(buf, (*(*[][]int64)(ptr))[idx])
	case container.TypeArrayFloat:
		return writeFloatArray(buf, (*(*[][]float64)(ptr))[idx])
	case container.TypeArrayString:
		writeStringArray(buf, (*(*[][]string)(ptr))[idx])
	case container.TypeArrayBool:
		writeBoolArray(buf, (*(*[][]bool)(ptr))[idx])
	case container.TypeArrayBytes:
		writeBytesArray(buf, (*(*[][][]byte)(ptr))[idx])
	case container.TypeArrayGeometry:
		return writeFloat3D(buf, (*(*[][][][]float64)(ptr))[idx])
	case container.TypeArrayUnknown:
		return writeAnyArray(buf, (*(*[][]any)(ptr))[idx])
	case container.TypeArrayObject:
		return writeChildren(cs, (*(*[][]*container.DataContainer)(ptr))[idx], prefix, buf, skip)
	case container.TypeUnknown:
		return writeAny(buf, (*(*[]any)(ptr))[idx])
	}
	return nil
}

func writeChildren(cs *definition.CompiledSchema, children []*container.DataContainer, prefix string, buf *bytes.Buffer, skip map[string]bool) error {
	buf.WriteByte('[')
	for i, child := range children {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeObject(cs, child, prefix, buf, skip); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

func writeAny(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		writeString(buf, t)
	case int64:
		writeFormatInt(buf, t)
	case float64:
		return writeFloat(buf, t)
	case []byte:
		writeBytes(buf, t)
	case []any:
		return writeAnyArray(buf, t)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		slices.Sort(keys)

		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeString(buf, k)
			buf.WriteByte(':')
			if err := writeAny(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	default:
		enc, err := stdjson.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(enc)
		return nil
	}
	return nil
}

func writeAnyArray(buf *bytes.Buffer, a []any) error {
	buf.WriteByte('[')
	for i, v := range a {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeAny(buf, v); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

func writeString(buf *bytes.Buffer, s string) {
	const hex = "0123456789abcdef"
	buf.WriteByte('"')

	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' || c < 0x20 || c == '<' || c == '>' || c == '&' {
			if i > start {
				buf.WriteString(s[start:i])
			}
			switch c {
			case '"':
				buf.WriteString(`\"`)
			case '\\':
				buf.WriteString(`\\`)
			case '\n':
				buf.WriteString(`\n`)
			case '\r':
				buf.WriteString(`\r`)
			case '\t':
				buf.WriteString(`\t`)
			case '<':
				buf.WriteString(`\u003c`)
			case '>':
				buf.WriteString(`\u003e`)
			case '&':
				buf.WriteString(`\u0026`)
			default:
				buf.WriteString(`\u00`)
				buf.WriteByte(hex[c>>4])
				buf.WriteByte(hex[c&0xF])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		buf.WriteString(s[start:])
	}
	buf.WriteByte('"')
}

func writeFormatInt(buf *bytes.Buffer, val int64) {
	b := buf.AvailableBuffer()
	b = strconv.AppendInt(b, val, 10)
	buf.Write(b)
}

func writeFloat(buf *bytes.Buffer, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("json: cannot serialize non-finite float %v", f)
	}
	format := byte('f')
	abs := math.Abs(f)
	if abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	b := buf.AvailableBuffer()
	b = strconv.AppendFloat(b, f, format, -1, 64)
	buf.Write(b)
	return nil
}

func writeBytes(buf *bytes.Buffer, b []byte) {
	buf.WriteByte('"')
	encodedLen := base64.StdEncoding.EncodedLen(len(b))
	out := buf.AvailableBuffer()
	if cap(out) < encodedLen {
		out = make([]byte, encodedLen)
	} else {
		out = out[:encodedLen]
	}
	base64.StdEncoding.Encode(out, b)
	buf.Write(out)
	buf.WriteByte('"')
}

func writeIntArray(buf *bytes.Buffer, a []int64) error {
	buf.WriteByte('[')
	for i, v := range a {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeFormatInt(buf, v)
	}
	buf.WriteByte(']')
	return nil
}

func writeFloatArray(buf *bytes.Buffer, a []float64) error {
	buf.WriteByte('[')
	for i, v := range a {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeFloat(buf, v); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

func writeStringArray(buf *bytes.Buffer, a []string) {
	buf.WriteByte('[')
	for i, v := range a {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeString(buf, v)
	}
	buf.WriteByte(']')
}

func writeBoolArray(buf *bytes.Buffer, a []bool) {
	buf.WriteByte('[')
	for i, v := range a {
		if i > 0 {
			buf.WriteByte(',')
		}
		if v {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	}
	buf.WriteByte(']')
}

func writeBytesArray(buf *bytes.Buffer, a [][]byte) {
	buf.WriteByte('[')
	for i, v := range a {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeBytes(buf, v)
	}
	buf.WriteByte(']')
}

func writeFloat2D(buf *bytes.Buffer, a [][]float64) error {
	buf.WriteByte('[')
	for i, v := range a {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeFloatArray(buf, v); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

func writeFloat3D(buf *bytes.Buffer, a [][][]float64) error {
	buf.WriteByte('[')
	for i, v := range a {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeFloat2D(buf, v); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}
