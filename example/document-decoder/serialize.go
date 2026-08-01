package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
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
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	if err := writeObject(cs, doc, buf); err != nil {
		bufferPool.Put(buf)
		return nil, err
	}

	res := bytes.Clone(buf.Bytes())

	if buf.Cap() <= maxPooledBufferSize {
		bufferPool.Put(buf)
	}

	return res, nil
}

func writeObject(cs *definition.CompiledSchema, doc *container.DataContainer, buf *bytes.Buffer) error {
	entriesPtr := fieldEntryPool.Get().(*[]fieldEntry)
	entries := (*entriesPtr)[:0]

	var slotFunc func(container.DataType, ...int) unsafe.Pointer

	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		slotFunc = slot
		for k, idx := range positions {
			key := container.DataContainerKey(k)
			entries = append(entries, fieldEntry{
				name: nameFor(cs, key),
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

	slices.SortFunc(entries, func(a, b fieldEntry) int {
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		return 0
	})

	buf.WriteByte('{')
	for i := 0; i < len(entries); i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeString(buf, entries[i].name)
		buf.WriteByte(':')
		if err := writeValue(cs, slotFunc, entries[i].key, entries[i].idx, buf); err != nil {
			resetAndRecycleEntries(entriesPtr, entries)
			return err
		}
	}
	buf.WriteByte('}')

	resetAndRecycleEntries(entriesPtr, entries)
	return nil
}

func resetAndRecycleEntries(ptr *[]fieldEntry, entries []fieldEntry) {
	for i := range entries {
		entries[i] = fieldEntry{}
	}
	*ptr = entries[:0]
	fieldEntryPool.Put(ptr)
}

func writeValue(cs *definition.CompiledSchema, slot func(container.DataType, ...int) unsafe.Pointer, key container.DataContainerKey, idx int32, buf *bytes.Buffer) error {
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
		return writeChildren(cs, (*(*[][]*container.DataContainer)(ptr))[idx], buf)
	case container.TypeUnknown:
		return writeAny(buf, (*(*[]any)(ptr))[idx])
	}
	return nil
}

func writeChildren(cs *definition.CompiledSchema, children []*container.DataContainer, buf *bytes.Buffer) error {
	buf.WriteByte('[')
	for i, child := range children {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeObject(cs, child, buf); err != nil {
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
		enc, err := json.Marshal(v)
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
		return fmt.Errorf("document-decoder: cannot serialize non-finite float %v", f)
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
