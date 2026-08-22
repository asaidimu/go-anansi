package json

import (
	"bytes"
	"encoding/base64"
	stdjson "encoding/json"
	"fmt"
	"io"
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

// SerializeJSONCanonicalTo serializes the container as canonical JSON, writing
// the bytes directly to w from the pooled buffer. Callers that only consume the
// bytes (e.g. feeding a hash) avoid the copy SerializeJSONCanonical returns.
func SerializeJSONCanonicalTo(w io.Writer, cs *definition.CompiledSchema, doc *container.DataContainer, skip map[string]bool) error {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	if err := writeObject(cs, doc, "", buf, skip); err != nil {
		bufferPool.Put(buf)
		return err
	}

	_, err := w.Write(buf.Bytes())

	if buf.Cap() <= maxPooledBufferSize {
		bufferPool.Put(buf)
	}
	return err
}

// SerializeJSONPrefix serializes only the subtree under a fully-qualified
// schema path (e.g. "customer" or "_metadata_") directly from the container's
// typed slots — no intermediate maps. A single stored value (a record, a
// slice, an array-of-object field) is emitted as that tagged value; a named
// object whose leaves share the container emits a JSON object of its
// descendants. An absent path emits "null".
func SerializeJSONPrefix(cs *definition.CompiledSchema, doc *container.DataContainer, path string) ([]byte, error) {
	buf, err := serializePrefixBuf(cs, doc, path)
	if err != nil {
		return nil, err
	}
	res := bytes.Clone(buf.Bytes())
	recycleBuffer(buf)
	return res, nil
}

// SerializeJSONPrefixString is SerializeJSONPrefix returning a string, saving
// callers that need text (e.g. SQLite binds) a second copy.
func SerializeJSONPrefixString(cs *definition.CompiledSchema, doc *container.DataContainer, path string) (string, error) {
	buf, err := serializePrefixBuf(cs, doc, path)
	if err != nil {
		return "", err
	}
	res := string(buf.Bytes())
	recycleBuffer(buf)
	return res, nil
}

func serializePrefixBuf(cs *definition.CompiledSchema, doc *container.DataContainer, path string) (*bytes.Buffer, error) {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	if err := writePrefixed(cs, doc, path, buf); err != nil {
		recycleBuffer(buf)
		return nil, err
	}
	return buf, nil
}

// recycleBuffer returns a pooled buffer to the pool when it is small enough to
// reuse, avoiding retention of oversized buffers.
func recycleBuffer(buf *bytes.Buffer) {
	if buf.Cap() <= maxPooledBufferSize {
		bufferPool.Put(buf)
	}
}

func writePrefixed(cs *definition.CompiledSchema, doc *container.DataContainer, path string, buf *bytes.Buffer) error {
	entriesPtr := fieldEntryPool.Get().(*[]fieldEntry)
	entries := (*entriesPtr)[:0]

	var slotFunc func(container.DataType, ...int) unsafe.Pointer

	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		slotFunc = slot
		for k, idx := range positions {
			key := container.DataContainerKey(k)
			name := nameFor(cs, key)
			if name == path || strings.HasPrefix(name, path+".") {
				entries = append(entries, fieldEntry{name: name, key: key, idx: idx})
			}
		}
		return nil, nil
	})
	if err != nil {
		resetAndRecycleEntries(entriesPtr, entries)
		return err
	}

	var direct *fieldEntry
	var descendants []fieldEntry
	sortEntries(entries)
	if len(entries) > 0 && entries[0].name == path {
		direct = &entries[0]
		descendants = entries[1:]
	} else {
		descendants = entries
	}

	switch {
	case direct != nil && len(descendants) > 0:
		resetAndRecycleEntries(entriesPtr, entries)
		return fmt.Errorf("json: serialize: field %q has both a value and child fields", path)
	case direct != nil:
		// Pass the field's own path as the prefix so descendants — in
		// particular array-of-object children, whose leaves live in the shared
		// address space under names like "<field>.<child>" — are emitted
		// relative to the field, matching full-document serialize. Otherwise
		// the field-path prefix leaks into each element's key.
		if err := writeValue(cs, slotFunc, direct.key, direct.idx, path, buf, nil); err != nil {
			resetAndRecycleEntries(entriesPtr, entries)
			return err
		}
	case len(descendants) > 0:
		if err := emitObject(cs, slotFunc, descendants, path, buf, nil); err != nil {
			resetAndRecycleEntries(entriesPtr, entries)
			return err
		}
	default:
		buf.WriteString("null")
	}

	resetAndRecycleEntries(entriesPtr, entries)
	return nil
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

	sortEntries(entries)
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
// emitted, so the relative field name is what remains. entries must be sorted
// by name (in place, once per container, by writeObject/writePrefixed): direct
// leaves then precede their descendants, so contiguous runs of equal first
// segment are emitted without building per-object maps, order slices, or
// descendant slices. Named-object children flatten into the same container's
// key space, so descendant runs are emitted as nested JSON objects;
// array-of-object fields are emitted as arrays of per-element objects keyed by
// the child schema's field names.
func emitObject(cs *definition.CompiledSchema, slotFunc func(container.DataType, ...int) unsafe.Pointer, entries []fieldEntry, prefix string, buf *bytes.Buffer, skip map[string]bool) error {
	buf.WriteByte('{')
	first := true
	for i := 0; i < len(entries); {
		rel := relName(entries[i].name, prefix)
		seg := firstSegment(rel)
		j := i + 1
		for j < len(entries) && firstSegment(relName(entries[j].name, prefix)) == seg {
			j++
		}

		if !first {
			buf.WriteByte(',')
		}
		writeString(buf, seg)
		buf.WriteByte(':')
		if rel == seg {
			// A direct leaf sorts before its descendants, so it is the run's
			// first entry; any further entry with the same segment is a child
			// field, which is invalid alongside a value.
			if j > i+1 {
				return fmt.Errorf("json: serialize: field %q has both a value and child fields", seg)
			}
			// writeValue needs the full field path only to emit array-of-object
			// children; defer the join so scalar and nested-object leaves never
			// allocate a prefixed path.
			var childPrefix string
			if entries[i].key.Type() == container.TypeArrayObject {
				childPrefix = joinPrefix(prefix, seg)
			}
			if err := writeValue(cs, slotFunc, entries[i].key, entries[i].idx, childPrefix, buf, skip); err != nil {
				return err
			}
		} else {
			if err := emitObject(cs, slotFunc, entries[i:j], joinPrefix(prefix, seg), buf, skip); err != nil {
				return err
			}
		}
		first = false
		i = j
	}
	buf.WriteByte('}')
	return nil
}

// relName returns name relative to a prefix ("prefix.…" → the remainder), as a
// zero-allocation substring. Names not under prefix are returned unchanged.
func relName(name, prefix string) string {
	if prefix == "" {
		return name
	}
	if len(name) > len(prefix) && name[:len(prefix)] == prefix && name[len(prefix)] == '.' {
		return name[len(prefix)+1:]
	}
	return name
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

// sortEntries orders entries by fully-qualified name so direct leaves precede
// their descendants and each object's keys come out in canonical (sorted)
// order. Sorting in place keeps sub-slices passed to emitObject sorted too.
func sortEntries(entries []fieldEntry) {
	slices.SortFunc(entries, func(a, b fieldEntry) int {
		return strings.Compare(a.name, b.name)
	})
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
		fd := definition.FieldDescriptor(key.Descriptor())
		return writeChildren(cs, (*(*[][]*container.DataContainer)(ptr))[idx], fd.ChildSchemaIdx(), prefix, buf, skip)
	case container.TypeUnknown:
		return writeAny(buf, (*(*[]any)(ptr))[idx])
	}
	return nil
}

// writeChildren emits an array-of-object field as a JSON array of
// per-element objects.
//
// @note #encoder-array-object-recursion issue status=resolved priority=P0 tags=#encoder,#arrays : emitObject recursed forever encoding array<object> children resolved through the root address map
// @assignee opencode
//
// RESOLVED 2026-08-23: writeChildren now emits elements via writeObjectAt,
// which resolves entry names against the CHILD schema slot (bare local field
// names) instead of the root address reverse-map. Regression tests in
// notif_matrix_test.go (TestDecode_GeneratedNotifSchema/d-actions,
// TestEncode_ArrayObjectElements) pin the shape.
//
// The bug: array-element containers are keyed by descriptors of the child
// schema slot, but nameFor resolved their DataPoint IDs through
// PathNameForAddress, which returned full ROOT paths (e.g.
// "payload.actions.label"). Under writeChildren's element prefix ("actions")
// those names never strip, so emitObject kept recursing on
// joinPrefix(prefix,"payload") — unbounded prefix growth, one '{' and a sort
// per level, RSS to OOM within seconds. Triggered by any document containing
// a populated array<object> field reaching this encoder (e.g. metadata
// checksum finalization inside DocumentPool.FromJSON). Known limitation of
// the slot-local naming: object-typed MEMBERS inside array children emit
// flattened rather than nested; no crash, fix upstream when needed. Elements are containers keyed by the CHILD schema slot's
// fields, so each element is emitted via writeObjectAt against that slot —
// resolving names slot-locally. Resolving them through the root address map
// would return full root paths (e.g. "payload.actions.label") that never match
// the element prefix, sending emitObject into unbounded prefix-growing
// recursion.
func writeChildren(cs *definition.CompiledSchema, children []*container.DataContainer, childSlotIdx uint8, prefix string, buf *bytes.Buffer, skip map[string]bool) error {
	buf.WriteByte('[')
	for i, child := range children {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeObjectAt(cs, child, childSlotIdx, prefix, buf, skip); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

// writeObjectAt is writeObject for a container whose fields belong to schema
// slot schemaIdx (an array-of-object element). Entry names are resolved
// against that slot: a key whose descriptor lives in the slot maps to the
// bare local field name; anything else falls back to the global nameFor.
func writeObjectAt(cs *definition.CompiledSchema, doc *container.DataContainer, schemaIdx uint8, prefix string, buf *bytes.Buffer, skip map[string]bool) error {
	if schemaIdx >= uint8(len(cs.Schemas)) {
		return writeObject(cs, doc, prefix, buf, skip)
	}

	entriesPtr := fieldEntryPool.Get().(*[]fieldEntry)
	entries := (*entriesPtr)[:0]

	var slotFunc func(container.DataType, ...int) unsafe.Pointer

	slot := cs.Schemas[schemaIdx]

	_, err := doc.Walk(func(positions map[int64]int32, walkSlot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		slotFunc = walkSlot
		for k, idx := range positions {
			key := container.DataContainerKey(k)
			name := localSlotFieldName(cs, slot, key)
			if name == "" {
				name = nameFor(cs, key)
			}
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

	sortEntries(entries)
	if err := emitObject(cs, slotFunc, entries, prefix, buf, skip); err != nil {
		resetAndRecycleEntries(entriesPtr, entries)
		return err
	}

	resetAndRecycleEntries(entriesPtr, entries)
	return nil
}

// localSlotFieldName returns the bare field name of the slot field whose
// descriptor matches key's, or "" when the key belongs to no field of the
// slot. Allocation-free: descriptor words compare directly.
func localSlotFieldName(cs *definition.CompiledSchema, slot definition.SchemaSlot, key container.DataContainerKey) string {
	desc := key.Descriptor()
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		if uint32(cs.Descriptors[abs]) == desc {
			return cs.FieldsMeta[abs].Name
		}
	}
	return ""
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
