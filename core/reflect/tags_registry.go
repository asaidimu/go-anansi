package reflect

import (
	"iter"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/common"
)

// ValueKind identifies the value payload structure of a tag.
type ValueKind uint8

const (
	KindEmpty  ValueKind = iota // Key exists without value (flag)
	KindString                  // Single string value
	KindSlice                   // Comma-separated slice of strings
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

// Handle bit layout (LSB to MSB), packed mode (ext flag clear):
//
//	bits  0-14 (15): key offset into slab
//	bits 15-29 (15): key length
//	bits 30-44 (15): value offset into slab
//	bits 45-59 (15): value length
//	bits 60-61  (2): ValueKind, precomputed once at build time (see buildMetadata)
//	bit  62     (1): extended-mode flag (see below)
//	bit  63     (1): unused
//
// Extended mode (ext flag set): the tag's key/value did not fit the 32 KiB
// per-type byte slab (or a single string exceeded 32 KiB). Bits 0-44 then hold
// an index into the owning structMetadata's ext table, which stores the key and
// value as ordinary heap strings. This makes oversized tag data impossible to
// corrupt through 15-bit offset masking, at zero cost for the (universal)
// packed case.
//
// Tag wraps a bit-packed 64-bit handle and a reference to the type's byte slab
// (and, for extended-mode tags, the type's ext table).
type Tag struct {
	handle uint64
	slab   []byte
	ext    []extEntry
}

type extEntry struct{ key, val string }

const (
	handleExtFlag      = uint64(1) << 62
	handleExtIndexMask = (uint64(1) << 45) - 1 // bits 0-44
	handleKindShift    = 60
	maxSlabLen         = 0x8000 // exclusive; offsets+lengths must stay < 32 KiB
)

func (t Tag) isExtended() bool { return t.handle&handleExtFlag != 0 }

func (t Tag) extEntryAt() (extEntry, bool) {
	idx := int(t.handle & handleExtIndexMask)
	if t.ext == nil || idx < 0 || idx >= len(t.ext) {
		return extEntry{}, false
	}
	return t.ext[idx], true
}

func (t Tag) IsZero() bool { return t.handle == 0 && t.slab == nil }

// Key returns the tag key (e.g. "json", "validate", "doc").
func (t Tag) Key() string {
	if t.isExtended() {
		if e, ok := t.extEntryAt(); ok {
			return e.key
		}
		return ""
	}
	if t.slab == nil {
		return ""
	}
	off := uint16(t.handle & 0x7FFF)
	length := uint16((t.handle >> 15) & 0x7FFF)
	if length == 0 {
		return ""
	}
	return unsafe.String(&t.slab[off], length)
}

// ValueKind indicates whether the tag is a flag, single string, or slice.
//
// This is a pure bit-mask read: the kind is computed once, at parse time, in
// buildMetadata, and packed into the handle. It cannot change after a Tag is
// constructed, so recomputing it on every call (the original implementation
// re-scanned the value string for a comma each time) was wasted work.
func (t Tag) ValueKind() ValueKind {
	if t.slab == nil && !t.isExtended() {
		return KindEmpty
	}
	return ValueKind((t.handle >> handleKindShift) & 0x3)
}

// @note #cd05t4 status resolved issue : Unneccessary computations
//
// Resolved: ValueKind is now computed once per tag in buildMetadata and
// packed into 2 previously-unused bits of the handle (bits 60-61). Tag.
// ValueKind() is now a bit-mask extraction with no string scan. See
// computeValueKind and the handle layout documented on the Tag struct.

// Value returns the raw single-string value.
//
// Note: values are stored raw (escape sequences such as \" are preserved
// verbatim). This intentionally diverges from reflect.StructTag.Lookup,
// which strconv.Unquotes the value; the divergence is locked in by
// TestTag_SpecialCharacters and keeps the zero-copy slab path allocation
// free. Struct tags in practice never carry escaped quotes.
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
		// @note #1k51l3 status resolved issue : Library Anti pattern. Use common.SystemError
		//
		// Resolved: common.SystemError satisfies error as well as
		// providing means to add richer error context, which is
		// invaluable in tracing issues from low level utilities such
		// as this one, if and when such errors impact higher level
		// implementations.
		return common.NewSystemError("ERR_TAGS_NIL_UNMARSHALER", "tags: target unmarshaler cannot be nil")
	}
	var vals []string
	if val, ok := t.Value(); ok {
		vals = strings.Split(val, ",")
	}
	return target.FromValues(vals...)
}

func (t Tag) rawVal() string {
	if t.isExtended() {
		if e, ok := t.extEntryAt(); ok {
			return e.val
		}
		return ""
	}
	off := uint16((t.handle >> 30) & 0x7FFF)
	length := uint16((t.handle >> 45) & 0x7FFF)
	if length == 0 {
		return ""
	}
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

	// ext holds key/value pairs for tags that did not fit the slab (see the
	// extended-mode handle layout on Tag). Nil for the overwhelming majority
	// of types.
	ext []extEntry
}

type fieldMetadata struct {
	name    string
	handles []uint64
}

// fieldLoc is the payload stored in the package-level fieldIndex, keyed by
// (struct type, field name). It points into the owning structMetadata instead
// of copying the slab/handles per field, so each type's tag data lives in
// exactly one place; FieldTag/FieldTags resolve their Tag through it.
type fieldLoc struct {
	meta *structMetadata
	idx  int // index into meta.fields
}

// fieldIndex is the package-level (type, field name) -> fieldLoc index.
// It is populated once per type, inside buildMetadata, under registryMu.
var fieldIndex = newFieldTagMap[fieldLoc]()

// @note #dxwlvi todo : Observe Unbounded cache
//
// In a very complex, dynamic application where module use flactuates at
// runtime, a dynamic cache may be more performant memory wise.

// registryCache caches struct metadata per (pointer-stripped) struct type.
//
// The original implementation stored an atomic.Pointer to a map that was
// fully copied on every insert, making registration of T types O(T^2) in
// time and generating a discarded map per insert. A sync.Map gives the same
// lock-free read path with O(1) inserts; the write lock below merely
// serializes duplicate builds of the same type.
var registryCache sync.Map // reflect.Type -> *structMetadata
var registryMu sync.Mutex

func inspectType(t reflect.Type) *structMetadata {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return &structMetadata{}
	}

	if meta, ok := registryCache.Load(t); ok {
		return meta.(*structMetadata)
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	if meta, ok := registryCache.Load(t); ok {
		return meta.(*structMetadata)
	}

	meta := buildMetadata(t)
	registryCache.Store(t, meta)
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

// scanTags parses a struct tag string into packed handles, appending any
// oversized strings to meta.ext. The control flow deliberately mirrors
// reflect.StructTag.Lookup (the stdlib parser used by the rest of the
// repository via f.Tag.Get): on a malformed segment it stops parsing, so
// behaviour never diverges from the stdlib for malformed input. Values are
// stored raw — escape sequences are preserved verbatim (see Tag.Value).
func scanTags(raw string, meta *structMetadata) []uint64 {
	var handles []uint64
	s := raw
	for s != "" {
		j := 0
		for j < len(s) && s[j] == ' ' {
			j++
		}
		s = s[j:]
		if s == "" {
			break
		}

		j = 0
		for j < len(s) && s[j] > ' ' && s[j] != ':' && s[j] != '"' && s[j] != 0x7f {
			j++
		}
		if j == 0 || j+1 >= len(s) || s[j] != ':' || s[j+1] != '"' {
			break
		}
		key := s[:j]
		s = s[j+1:]

		j = 1
		for j < len(s) && s[j] != '"' {
			if s[j] == '\\' {
				j++
			}
			j++
		}
		if j >= len(s) {
			break
		}
		val := s[1:j]
		s = s[j+1:]

		vk := computeValueKind(val, uint16(len(val)))
		handles = append(handles, meta.packTag(key, val, vk))
	}
	return handles
}

// packTag packs one key/value pair into a handle. Strings that fit the
// per-type slab are referenced by 15-bit offset/length; anything else falls
// back to the ext table so oversized input can never corrupt through
// masking.
func (meta *structMetadata) packTag(key, val string, vk ValueKind) uint64 {
	fitsSlab := func(s string) bool {
		return len(s) <= 0x7FFF && len(meta.slab)+len(s) < maxSlabLen
	}
	if fitsSlab(key) && fitsSlab(val) {
		kOff := uint16(len(meta.slab))
		meta.slab = append(meta.slab, key...)
		vOff := uint16(len(meta.slab))
		meta.slab = append(meta.slab, val...)

		var h uint64
		h |= uint64(kOff & 0x7FFF)
		h |= uint64(uint16(len(key))&0x7FFF) << 15
		h |= uint64(vOff&0x7FFF) << 30
		h |= uint64(uint16(len(val))&0x7FFF) << 45
		h |= uint64(vk&0x3) << handleKindShift
		return h
	}

	idx := uint64(len(meta.ext))
	meta.ext = append(meta.ext, extEntry{key: key, val: val})

	var h uint64
	h |= idx & handleExtIndexMask
	h |= handleExtFlag
	h |= uint64(vk&0x3) << handleKindShift
	return h
}

func buildMetadata(t reflect.Type) *structMetadata {
	meta := &structMetadata{
		slab:   make([]byte, 0, 256),
		fields: make([]fieldMetadata, 0, t.NumField()),
		byKey:  make(map[string][]tagRef),
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" && !f.Anonymous {
			continue
		}
		handles := scanTags(string(f.Tag), meta)
		meta.fields = append(meta.fields, fieldMetadata{
			name:    f.Name,
			handles: handles,
		})
	}

	for i := range meta.fields {
		fld := &meta.fields[i]
		for _, h := range fld.handles {
			tg := Tag{handle: h, slab: meta.slab, ext: meta.ext}
			meta.byKey[tg.Key()] = append(meta.byKey[tg.Key()], tagRef{field: fld.name, handle: h})
		}
		fieldIndex.Set(t, fld.name, fieldLoc{meta: meta, idx: i})
	}

	return meta
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
//
// All "Of" forms normalize their input the same way inspectType does:
// pointers are stripped, so FieldTagOf(reflect.TypeOf(&S{}), ...) and
// FieldTagOf(reflect.TypeOf(S{}), ...) are equivalent. (The original
// implementation stripped pointers only inside inspectType, so a pointer
// type missed the fieldIndex twice and silently returned no tag after
// paying for a full build.)

// normalizeStructType strips pointers and rejects non-struct types, matching
// inspectType's cache-key semantics.
func normalizeStructType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	return t
}

// FieldTagOf fetches a single specific tag for a named field on t.
func FieldTagOf(t reflect.Type, fieldName, tagKey string) (Tag, bool) {
	t = normalizeStructType(t)
	if t == nil {
		return Tag{}, false
	}

	entry, ok := fieldIndex.Get(t, fieldName)
	if !ok {
		inspectType(t) // populates fieldIndex as a side effect, cached per type
		entry, ok = fieldIndex.Get(t, fieldName)
		if !ok {
			return Tag{}, false
		}
	}

	// Fields carry only a handful of tags, so a linear scan over slab-backed
	// keys beats a per-field map index (which cost one map allocation per
	// field at build time and an extra hash per lookup).
	fld := &entry.meta.fields[entry.idx]
	for _, h := range fld.handles {
		tg := Tag{handle: h, slab: entry.meta.slab, ext: entry.meta.ext}
		if tg.Key() == tagKey {
			return tg, true
		}
	}
	return Tag{}, false
}

// FieldTag fetches a single specific tag for a named field.
func FieldTag[T any](fieldName, tagKey string) (Tag, bool) {
	return FieldTagOf(reflect.TypeOf((*T)(nil)).Elem(), fieldName, tagKey)
}

// @note #5nhvz2 status resolved issue : Inefficient field access
//
// Resolved: FieldTag now does a single fieldIndex.Get(type, fieldName)
// lookup (composite-keyed, see field_tag_map.go) followed by a linear scan
// of that field's handles for the tag key. Both are O(1)/O(k) with k = tags
// per field (single digits) instead of the original linear scan over every
// field followed by a linear scan over that field's tags.

// FieldTagsOf yields all tags assigned to a specific field on t.
func FieldTagsOf(t reflect.Type, fieldName string) iter.Seq[Tag] {
	return func(yield func(Tag) bool) {
		t = normalizeStructType(t)
		if t == nil {
			return
		}
		entry, ok := fieldIndex.Get(t, fieldName)
		if !ok {
			inspectType(t)
			entry, ok = fieldIndex.Get(t, fieldName)
			if !ok {
				return
			}
		}

		fld := &entry.meta.fields[entry.idx]
		for _, h := range fld.handles {
			if !yield(Tag{handle: h, slab: entry.meta.slab, ext: entry.meta.ext}) {
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
				if !yield(f.name, Tag{handle: h, slab: meta.slab, ext: meta.ext}) {
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
			if !yield(ref.field, Tag{handle: ref.handle, slab: meta.slab, ext: meta.ext}) {
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

// materializationCache caches compiled tag materializations by (source type,
// target type). Values are either *K or error (failures are cached too, so a
// failing parse is not retried). The original implementation copied the whole
// map on every insert (O(T^2) across registrations); sync.Map gives O(1)
// inserts with the same lock-free read path.
var materializationCache sync.Map // typePairKey -> any
var materializationMu sync.Mutex

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
	if cached, ok := materializationCache.Load(pair); ok {
		if err, isErr := cached.(error); isErr {
			return nil, err
		}
		return cached.(*K), nil
	}

	materializationMu.Lock()
	defer materializationMu.Unlock()

	if cached, ok := materializationCache.Load(pair); ok {
		if err, isErr := cached.(error); isErr {
			return nil, err
		}
		return cached.(*K), nil
	}

	res, err := compileMaterialization[K](t, parsers...)
	if err != nil {
		materializationCache.Store(pair, err)
		return nil, err
	}

	materializationCache.Store(pair, res)
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
				tagSlice = append(tagSlice, Tag{handle: h, slab: meta.slab, ext: meta.ext})
			}
			if err := unmarshaler.FromTags(f.name, tagSlice); err != nil {
				return nil, &materializationError{field: f.name, err: err}
			}
		}
		return &result, nil
	}

	// Strategy 2: User-provided parser closure
	if len(parsers) > 0 && parsers[0] != nil {
		var allTags []Tag
		for i := range meta.fields {
			for _, h := range meta.fields[i].handles {
				allTags = append(allTags, Tag{handle: h, slab: meta.slab, ext: meta.ext})
			}
		}
		parsed, err := parsers[0](allTags)
		if err != nil {
			return nil, &materializationError{err: err}
		}
		return &parsed, nil
	}

	return nil, &materializationError{target: reflect.TypeOf((*K)(nil)).Elem().Name()}
}

// materializationError renders the same messages the previous fmt.Errorf
// calls produced, while remaining a plain error for callers.
type materializationError struct {
	field  string
	target string
	err    error
}

func (e *materializationError) Error() string {
	switch {
	case e.err != nil && e.field != "":
		return "tags: FromTags error on field " + e.field + ": " + e.err.Error()
	case e.err != nil:
		return "tags: parser failure: " + e.err.Error()
	default:
		return "tags: type " + e.target + " does not implement TagUnmarshaler and no parser was provided"
	}
}

func (e *materializationError) Unwrap() error { return e.err }
