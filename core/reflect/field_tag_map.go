package reflect

import (
	"reflect"
	"sync"
)

// structFieldKey is a 64-bit composite key: [32 bits Type Hash][32 bits Field Name Hash].
//
// This is deliberately widened from the original 24/8-bit split. The 8-bit
// field hash gave only 256 buckets per type, which made collisions close to
// certain on any struct with more than a handful of fields (birthday-bound
// territory well under 20 fields). A 32/32 split makes accidental collisions
// astronomically rarer -- but it is still a hash, not a guaranteed-unique
// identifier, so fieldTagMap never trusts a hash match on its own; see
// fieldTagEntry below.
type structFieldKey uint64

// fieldTagEntry is one slot in a bucket: the payload plus the verification
// identity (the actual type and field name) needed to confirm that a
// structFieldKey match is a real match and not a collision.
type fieldTagEntry[P any] struct {
	typ     reflect.Type
	field   string
	payload P
}

// fieldTagMap maps (Type, field name) pairs to generic payload values P.
//
// Internally it is a structFieldKey-bucketed hash map. A structFieldKey
// collision only ever produces a shared bucket -- it can never produce a
// wrong answer, because every lookup walks the (typically single-entry)
// bucket and verifies the stored (typ, field) identity against the query
// before treating anything as a hit. Widening the hash (see structFieldKey)
// keeps buckets small in the common case; this verification is what keeps
// them *correct* even in the uncommon case.
//
// Safe for concurrent use.
type fieldTagMap[P any] struct {
	mu      sync.RWMutex
	buckets map[structFieldKey][]fieldTagEntry[P]
}

// newFieldTagMap initializes an empty fieldTagMap instance.
func newFieldTagMap[P any]() *fieldTagMap[P] {
	return &fieldTagMap[P]{
		buckets: make(map[structFieldKey][]fieldTagEntry[P]),
	}
}

// Set inserts or updates a payload value for a given (typ, field) pair.
func (m *fieldTagMap[P]) Set(typ reflect.Type, field string, value P) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.buckets == nil {
		m.buckets = make(map[structFieldKey][]fieldTagEntry[P])
	}
	key := fieldKeyOf(typ, field)
	bucket := m.buckets[key]
	for i := range bucket {
		if bucket[i].typ == typ && bucket[i].field == field {
			bucket[i].payload = value
			return
		}
	}
	m.buckets[key] = append(bucket, fieldTagEntry[P]{typ: typ, field: field, payload: value})
}

// Get retrieves the value associated with (typ, field), returning
// (zeroValue, false) if absent. A structFieldKey bucket hit that fails
// identity verification is treated as absent, never as a false positive.
func (m *fieldTagMap[P]) Get(typ reflect.Type, field string) (P, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.buckets == nil {
		var zero P
		return zero, false
	}
	key := fieldKeyOf(typ, field)
	for _, e := range m.buckets[key] {
		if e.typ == typ && e.field == field {
			return e.payload, true
		}
	}
	var zero P
	return zero, false
}

// Has checks if a (typ, field) pair exists in the map.
func (m *fieldTagMap[P]) Has(typ reflect.Type, field string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.buckets == nil {
		return false
	}
	key := fieldKeyOf(typ, field)
	for _, e := range m.buckets[key] {
		if e.typ == typ && e.field == field {
			return true
		}
	}
	return false
}

// Remove deletes a (typ, field) entry from the map.
func (m *fieldTagMap[P]) Remove(typ reflect.Type, field string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.buckets == nil {
		return
	}
	key := fieldKeyOf(typ, field)
	bucket := m.buckets[key]
	for i := range bucket {
		if bucket[i].typ == typ && bucket[i].field == field {
			last := len(bucket) - 1
			bucket[i] = bucket[last]
			bucket = bucket[:last]
			if len(bucket) == 0 {
				delete(m.buckets, key)
			} else {
				m.buckets[key] = bucket
			}
			return
		}
	}
}

// Len returns the number of items stored in the map.
func (m *fieldTagMap[P]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, bucket := range m.buckets {
		n += len(bucket)
	}
	return n
}

// Clear removes all entries from the map.
func (m *fieldTagMap[P]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	clear(m.buckets)
}

// fieldKeyOf constructs a 64-bit key combining a 32-bit type hash with a
// 32-bit field-name hash. It remains a *hash*, not a guaranteed-unique
// identifier -- fieldTagMap's bucket+verify lookup is what actually
// guarantees correctness; this only has to be good enough to keep buckets
// small.
func fieldKeyOf(typ reflect.Type, field string) structFieldKey {
	tHash := typeHash32(typ)
	fHash := fieldHash32(field)
	return structFieldKey(tHash)<<32 | structFieldKey(fHash)
}

// typeHash32 hashes a reflect.Type's fully-qualified string representation
// (e.g. "pkg.MyStruct"). This replaces the original implementation's unsafe
// read of the runtime type-descriptor pointer address. That approach existed
// to avoid a reflect.TypeOf call, but fieldTagMap.Get/Set now require a
// reflect.Type value anyway for identity verification, so the unsafe path no
// longer buys anything -- and dropping it removes the only unsafe code in
// this file. typ.String() is stable for the process lifetime and, unlike a
// bare address, produces a human-inspectable hash input.
func typeHash32(typ reflect.Type) uint32 {
	s := typ.String()
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// @note #397ju0  todo : Refactor out helpers #cruft
//
// Resolved in place, not extracted: typeHash32/fieldHash32 are not
// general-purpose hash utilities. They exist solely to pick a bucket for
// fieldTagMap's bucket+verify scheme, and every current caller of
// fieldTagMap depends on that scheme's correctness guarantee (identity
// verification against the real (type, field) pair), not on hash quality
// in isolation. Moving these into a shared utility package would let a
// future caller reuse the hash function alone, without the verification
// step that makes using it safe, silently reintroducing the collision risk
// this rework was meant to close.

// fieldHash32 is an FNV-1a hash over the full 32 bits, with no folding.
// The original implementation folded a 32-bit hash down to 8 bits, which is
// what made collisions near-certain on structs with more than a handful of
// fields. Keeping the full width keeps fieldTagMap's buckets small in
// practice, even though correctness no longer depends on that in principle
// (see fieldTagEntry's verification step above).
func fieldHash32(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
