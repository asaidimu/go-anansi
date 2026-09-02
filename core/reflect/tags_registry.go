package reflect

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

// ValueKind identifies the value payload structure of a tag.
type ValueKind uint8

const (
	KindEmpty ValueKind = iota // Key exists without value (flag)
	KindString                 // Single string value
	KindSlice                  // Comma-separated slice of strings
)

// ---------- Materialization Interfaces ----------

// ValueUnmarshaler allows a type K to self-populate from tag string values.
type ValueUnmarshaler interface {
	FromValues(values ...string) error
}

// TagUnmarshaler allows a type K to self-populate from field tags.
type TagUnmarshaler interface {
	FromTags(field string, tags []Tag) error
}

// ---------- Zero-Allocation Tag Descriptor ----------

// Tag wraps a bit-packed 64-bit handle and a reference to the type's byte slab.
//
// Handle bit layout (LSB to MSB):
//   bits  0-14 (15): key offset into slab
//   bits 15-29 (15): key length
//   bits 30-44 (15): value offset into slab
//   bits 45-59 (15): value length
//   bits 60-61  (2): ValueKind, precomputed once at build time (see buildMetadata)
//   bits 62-63  (2): unused
type Tag struct {
	handle uint64
	slab   []byte
}

func (t Tag) IsZero() bool { return t.handle == 0 && t.slab == nil }

// Key returns the tag key (e.g. "json", "validate", "doc").
func (t Tag) Key() string {
	if t.slab == nil { return "" }
	off := uint16(t.handle & 0x7FFF)
	length := uint16((t.handle >> 15) & 0x7FFF)
	if length == 0 { return "" }
	return unsafe.String(&t.slab[off], length)
}

// ValueKind indicates whether the tag is a flag, single string, or slice.
//
// This is a pure bit-mask read: the kind is computed once, at parse time, in
// buildMetadata, and packed into the handle. It cannot change after a Tag is
// constructed, so recomputing it on every call (the original implementation
// re-scanned the value string for a comma each time) was wasted work.
func (t Tag) ValueKind() ValueKind {
	if t.slab == nil { return KindEmpty }
	return ValueKind((t.handle >> 60) & 0x3)
}

// @note #cd05t4 status resolved issue : Unneccessary computations
//
// Resolved: ValueKind is now computed once per tag in buildMetadata and
// packed into 2 previously-unused bits of the handle (bits 60-61). Tag.
// ValueKind() is now a bit-mask extraction with no string scan. See
// computeValueKind and the handle layout documented on the Tag struct.

// Value returns the raw single-string value.
func (t Tag) Value() (string, bool) {
	if t.ValueKind() == KindEmpty {
		return "", false
	}
	return t.rawVal(), true
}

// Values returns all parsed comma-separated values as a slice.
func (t Tag) Values() []string {
	val, ok := t.Value()
	if !ok {
		return nil
	}
	return strings.Split(val, ",")
}

// ValuesIter yields values without allocating a slice.
func (t Tag) ValuesIter() iter.Seq[string] {
	return func(yield func(string) bool) {
		val, ok := t.Value()
		if !ok {
			return
		}
		s := val
		for len(s) > 0 {
			var token string
			if idx := strings.IndexByte(s, ','); idx >= 0 {
				token = s[:idx]
				s = s[idx+1:]
			} else {
				token = s
				s = ""
			}
			if !yield(token) {
				return
			}
		}
	}
}

// Read populates a target struct K using its FromValues interface.
func (t Tag) Read(target ValueUnmarshaler) error {
	if target == nil {
		// @note #1k51l3 issue : Library Anti pattern. Use common.SystemError
		//
		// common.SystemError satisfies error as well as
		// providing means to add richer error context,
		// which is invaluable in tracing issues from low level
		// utilities such as this one, if and when such errors
		// impact higher level implementations.
		return errors.New("tags: target unmarshaler cannot be nil")
	}
	var vals []string
	if val, ok := t.Value(); ok {
		vals = strings.Split(val, ",")
	}
	return target.FromValues(vals...)
}

func (t Tag) rawVal() string {
	off := uint16((t.handle >> 30) & 0x7FFF)
	length := uint16((t.handle >> 45) & 0x7FFF)
	if length == 0 { return "" }
	return unsafe.String(&t.slab[off], length)
}

// ---------- Internal Engine & Handle Packing ----------

// tagRef is a (field name, handle) pair, used by structMetadata.byKey to
// answer "which fields have a tag with key K" without scanning every tag on
// every field.
type tagRef struct {
	field  string
	handle uint64
}

type structMetadata struct {
	slab   []byte
	fields []fieldMetadata

	// byKey maps a tag key to every (field, handle) pair carrying that key,
	// built once in buildMetadata. KeyTags reads directly from this instead
	// of scanning every tag on every field looking for matches.
	byKey map[string][]tagRef
}

type fieldMetadata struct {
	name    string
	handles []uint64
}

// fieldEntry is the payload stored in the package-level fieldIndex, keyed by
// (struct type, field name). It carries everything FieldTag/FieldTags need
// for that one field, reached via a single lookup instead of first finding
// the type's structMetadata and then scanning its fields slice for a name
// match.
type fieldEntry struct {
	slab    []byte
	handles []uint64
	// tagIndex maps a tag key directly to its index in handles, giving
	// FieldTag an O(1) lookup for the (field, key) pair instead of scanning
	// this field's handles for a key match.
	tagIndex map[string]int
}

// fieldIndex is the package-level (type, field name) -> fieldEntry index.
// It is populated once per type, inside buildMetadata, under registryMu.
var fieldIndex = newFieldTagMap[fieldEntry]()

// @note #dxwlvi todo : Observe Unbounded cache
//
// In a very complex, dynamic application where module use flactuates at
// runtime, a dynamic cache may be more performant memory wise.

var registryCache atomic.Pointer[map[reflect.Type]*structMetadata]
var registryMu sync.Mutex

func init() {
	m := make(map[reflect.Type]*structMetadata)
	registryCache.Store(&m)
}

func inspectType(t reflect.Type) *structMetadata {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return &structMetadata{}
	}

	curr := registryCache.Load()
	if meta, ok := (*curr)[t]; ok {
		return meta
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	curr = registryCache.Load()
	if meta, ok := (*curr)[t]; ok {
		return meta
	}

	meta := buildMetadata(t)
	next := make(map[reflect.Type]*structMetadata, len(*curr)+1)
	for k, v := range *curr {
		next[k] = v
	}
	next[t] = meta
	registryCache.Store(&next)
	return meta
}

// computeValueKind classifies a tag's raw value once, at build time, so that
// Tag.ValueKind() never has to re-scan the value string. See the note on
// #cd05t4.
func computeValueKind(val string, valLen uint16) ValueKind {
	if valLen == 0 {
		return KindEmpty
	}
	if strings.Contains(val, ",") {
		return KindSlice
	}
	return KindString
}

func buildMetadata(t reflect.Type) *structMetadata {
	slab := make([]byte, 0, 256)
	fields := make([]fieldMetadata, 0, t.NumField())

	add := func(s string) (uint16, uint16) {
		if s == "" { return 0, 0 }
		off := len(slab)
		slab = append(slab, s...)
		return uint16(off), uint16(len(s))
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" && !f.Anonymous {
			continue
		}

		rawTag := string(f.Tag)
		var handles []uint64

		s := rawTag
		for s != "" {
			j := 0
			for j < len(s) && s[j] == ' ' { j++ }
			s = s[j:]
			if s == "" { break }

			j = 0
			for j < len(s) && s[j] > ' ' && s[j] != ':' && s[j] != '"' && s[j] != 0x7f { j++ }
			if j == 0 || j >= len(s) || s[j] != ':' { break }
			key := s[:j]
			s = s[j+1:]

			if len(s) == 0 || s[0] != '"' { break }
			s = s[1:]
			j = 0
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' && j+1 < len(s) { j++ }
				j++
			}
			if j >= len(s) { break }
			val := s[:j]
			s = s[j+1:]

			kOff, kLen := add(key)
			vOff, vLen := add(val)

			h := uint64(kOff & 0x7FFF)
			h |= uint64(kLen & 0x7FFF) << 15
			h |= uint64(vOff & 0x7FFF) << 30
			h |= uint64(vLen & 0x7FFF) << 45

			vk := computeValueKind(val, vLen)
			h |= uint64(vk&0x3) << 60

			handles = append(handles, h)
		}

		fields = append(fields, fieldMetadata{
			name:    f.Name,
			handles: handles,
		})
	}

	byKey := make(map[string][]tagRef)
	for i := range fields {
		fld := &fields[i]
		tagIdx := make(map[string]int, len(fld.handles))
		for hi, h := range fld.handles {
			tg := Tag{handle: h, slab: slab}
			key := tg.Key()
			tagIdx[key] = hi
			byKey[key] = append(byKey[key], tagRef{field: fld.name, handle: h})
		}
		fieldIndex.Set(t, fld.name, fieldEntry{
			slab:     slab,
			handles:  fld.handles,
			tagIndex: tagIdx,
		})
	}

	return &structMetadata{
		slab:   slab,
		fields: fields,
		byKey:  byKey,
	}
}

// ---------- Public Read & Query API ----------
//
// Every query function comes in two forms:
//
//   - An "Of" form taking reflect.Type explicitly (FieldTagOf, FieldTagsOf,
//     TagsOf, KeyTagsOf). Use this when the struct type is only known at
//     runtime -- e.g. a library that receives values as `any` and recovers
//     their type via reflect.TypeOf, the way document.DocumentModel does
//     after its parent reference has been type-erased into `any`.
//   - A generic form (FieldTag[T], FieldTags[T], Tags[T], KeyTags[T]) for
//     callers who know the struct type at the call site. Each is a thin
//     wrapper that resolves T to a reflect.Type and delegates to the "Of"
//     form -- there is exactly one implementation per query, not two to
//     keep in sync.
//
// This mirrors the split already used internally for buildMetadata/
// inspectType, which have always taken reflect.Type directly.

// FieldTagOf fetches a single specific tag for a named field on t.
func FieldTagOf(t reflect.Type, fieldName, tagKey string) (Tag, bool) {
	entry, ok := fieldIndex.Get(t, fieldName)
	if !ok {
		inspectType(t) // populates fieldIndex as a side effect, cached per type
		entry, ok = fieldIndex.Get(t, fieldName)
		if !ok {
			return Tag{}, false
		}
	}

	idx, ok := entry.tagIndex[tagKey]
	if !ok {
		return Tag{}, false
	}
	return Tag{handle: entry.handles[idx], slab: entry.slab}, true
}

// FieldTag fetches a single specific tag for a named field.
func FieldTag[T any](fieldName, tagKey string) (Tag, bool) {
	return FieldTagOf(reflect.TypeOf((*T)(nil)).Elem(), fieldName, tagKey)
}

// @note #5nhvz2 status resolved issue : Inefficient field access
//
// Resolved: FieldTag now does a single fieldIndex.Get(type, fieldName)
// lookup (composite-keyed, bucket+verify -- see map.go) followed by a
// map lookup on that field's tagIndex for the tag key. Both are O(1)
// average instead of the original linear scan over every field followed
// by a linear scan over that field's tags.

// FieldTagsOf yields all tags assigned to a specific field on t.
func FieldTagsOf(t reflect.Type, fieldName string) iter.Seq[Tag] {
	return func(yield func(Tag) bool) {
		entry, ok := fieldIndex.Get(t, fieldName)
		if !ok {
			inspectType(t)
			entry, ok = fieldIndex.Get(t, fieldName)
			if !ok {
				return
			}
		}

		for _, h := range entry.handles {
			if !yield(Tag{handle: h, slab: entry.slab}) {
				return
			}
		}
	}
}

// FieldTags yields all tags assigned to a specific field.
func FieldTags[T any](fieldName string) iter.Seq[Tag] {
	return FieldTagsOf(reflect.TypeOf((*T)(nil)).Elem(), fieldName)
}

// @note #0ar5g0 status resolved issue : Inefficient access
//
// Resolved: same fieldIndex lookup as #5nhvz2 -- a single O(1) access to
// the field's handles instead of scanning meta.fields for a name match.

// TagsOf yields all field-name and tag pairs across the struct type t.
func TagsOf(t reflect.Type) iter.Seq2[string, Tag] {
	return func(yield func(string, Tag) bool) {
		meta := inspectType(t)
		for i := range meta.fields {
			f := &meta.fields[i]
			for _, h := range f.handles {
				if !yield(f.name, Tag{handle: h, slab: meta.slab}) {
					return
				}
			}
		}
	}
}

// Tags yields all field-name and tag pairs across the type T.
func Tags[T any]() iter.Seq2[string, Tag] {
	return TagsOf(reflect.TypeOf((*T)(nil)).Elem())
}

// @note #g23h8y status resolved issue : Inefficient access
//
// Resolved as "already optimal": Tags yields every field/tag pair by
// definition, so visiting each one once is O(n) in the size of the output,
// which is the best any implementation can do here -- there is no name or
// key to look up that could turn this into a sub-linear operation. The
// actual per-call overhead #g23h8y was pointing at (repeatedly re-deriving
// field/tag structure) is what #5nhvz2 and #0ar5g0 address for the
// single-field lookup paths; this function was never doing that redundant
// work in the first place, since it already reads directly from the
// type's cached structMetadata built once by buildMetadata.

// KeyTagsOf yields all fields of t matching a given tag key.
func KeyTagsOf(t reflect.Type, tagKey string) iter.Seq2[string, Tag] {
	return func(yield func(string, Tag) bool) {
		meta := inspectType(t)
		for _, ref := range meta.byKey[tagKey] {
			if !yield(ref.field, Tag{handle: ref.handle, slab: meta.slab}) {
				return
			}
		}
	}
}

// KeyTags yields all fields matching a given tag key.
func KeyTags[T any](tagKey string) iter.Seq2[string, Tag] {
	return KeyTagsOf(reflect.TypeOf((*T)(nil)).Elem(), tagKey)
}

// @note #a23h8y status resolved issue : Inefficient access
//
// Resolved: structMetadata now carries byKey, a map[string][]tagRef built
// once in buildMetadata alongside the rest of a type's metadata. KeyTags
// reads meta.byKey[tagKey] directly, so its cost is proportional to the
// number of matching tags, not to the total number of tags on the type.

// ---------- Idempotent Materialization Caching ----------

type typePairKey struct {
	targetType reflect.Type
	schemaType reflect.Type
}

var materializationCache atomic.Pointer[map[typePairKey]any]
var materializationMu sync.Mutex

func init() {
	m := make(map[typePairKey]any)
	materializationCache.Store(&m)
}

// ParseInto compiles and materializes the tag metadata of struct type t into
// arbitrary struct K once, caching the result by (t, K) pair.
//
// This is the reflect.Type-based counterpart to Read, for callers who only
// know the source struct's type at runtime -- e.g. recovering it via
// reflect.TypeOf from a value that arrived as `any`. K is still a compile-time
// type parameter because ParseInto has to construct a *K to return; only the
// source type T is runtime-supplied.
func ParseInto[K any](t reflect.Type, parsers ...func(tags []Tag) (K, error)) (*K, error) {
	kType := reflect.TypeOf((*K)(nil)).Elem()
	pair := typePairKey{targetType: t, schemaType: kType}

	// Lock-free read path
	curr := materializationCache.Load()
	if cached, ok := (*curr)[pair]; ok {
		if err, isErr := cached.(error); isErr {
			return nil, err
		}
		return cached.(*K), nil
	}

	materializationMu.Lock()
	defer materializationMu.Unlock()

	curr = materializationCache.Load()
	if cached, ok := (*curr)[pair]; ok {
		if err, isErr := cached.(error); isErr {
			return nil, err
		}
		return cached.(*K), nil
	}

	res, err := compileMaterialization[K](t, parsers...)
	next := make(map[typePairKey]any, len(*curr)+1)
	for key, val := range *curr {
		next[key] = val
	}

	if err != nil {
		next[pair] = err
		materializationCache.Store(&next)
		return nil, err
	}

	next[pair] = res
	materializationCache.Store(&next)
	return res, nil
}

// Parse compiles and materializes type metadata into arbitrary struct K once, caching the result.
func Parse[T any, K any](parsers ...func(tags []Tag) (K, error)) (*K, error) {
	return ParseInto[K](reflect.TypeOf((*T)(nil)).Elem(), parsers...)
}

func compileMaterialization[K any](t reflect.Type, parsers ...func(tags []Tag) (K, error)) (*K, error) {
	var result K
	meta := inspectType(t)

	// Strategy 1: Interface implementation on K
	if unmarshaler, ok := any(&result).(TagUnmarshaler); ok {
		for i := range meta.fields {
			f := &meta.fields[i]
			tagSlice := make([]Tag, 0, len(f.handles))
			for _, h := range f.handles {
				tagSlice = append(tagSlice, Tag{handle: h, slab: meta.slab})
			}
			if err := unmarshaler.FromTags(f.name, tagSlice); err != nil {
				return nil, fmt.Errorf("tags: FromTags error on field %s: %w", f.name, err)
			}
		}
		return &result, nil
	}

	// Strategy 2: User-provided parser closure
	if len(parsers) > 0 && parsers[0] != nil {
		var allTags []Tag
		for i := range meta.fields {
			for _, h := range meta.fields[i].handles {
				allTags = append(allTags, Tag{handle: h, slab: meta.slab})
			}
		}
		parsed, err := parsers[0](allTags)
		if err != nil {
			return nil, fmt.Errorf("tags: parser failure: %w", err)
		}
		return &parsed, nil
	}

	return nil, fmt.Errorf("tags: type %s does not implement TagUnmarshaler and no parser was provided", reflect.TypeOf((*K)(nil)).Elem().Name())
}
