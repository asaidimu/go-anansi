package reflect

import (
	"reflect"
	"sync"
)

// structFieldKey is the composite map key for a (type, field name) pair.
//
// The original implementation hashed this pair into a 64-bit integer and
// bucketed entries by that hash, verifying candidates against the real
// (typ, field) identity to defend against collisions. Both reflect.Type and
// string are natively comparable in Go, so the pair is a valid map key on its
// own: the runtime hashes it in O(1) with no allocation (the hash scheme paid
// a typ.String() allocation on every Set/Get/Has/Remove) and collisions are
// impossible by construction, which retired the entire bucket-and-verify
// machinery along with the FNV helpers.
type structFieldKey struct {
	typ   reflect.Type
	field string
}

// fieldTagMap maps (Type, field name) pairs to generic payload values P.
//
// Safe for concurrent use. The zero value is usable (Get/Has/Remove/Len on a
// nil map are no-ops; Set allocates lazily).
type fieldTagMap[P any] struct {
	mu      sync.RWMutex
	entries map[structFieldKey]P
}

// newFieldTagMap initializes an empty fieldTagMap instance.
func newFieldTagMap[P any]() *fieldTagMap[P] {
	return &fieldTagMap[P]{
		entries: make(map[structFieldKey]P),
	}
}

// Set inserts or updates a payload value for a given (typ, field) pair.
func (m *fieldTagMap[P]) Set(typ reflect.Type, field string, value P) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = make(map[structFieldKey]P)
	}
	m.entries[structFieldKey{typ: typ, field: field}] = value
}

// Get retrieves the value associated with (typ, field), returning
// (zeroValue, false) if absent.
func (m *fieldTagMap[P]) Get(typ reflect.Type, field string) (P, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.entries[structFieldKey{typ: typ, field: field}]
	return v, ok
}

// Has checks if a (typ, field) pair exists in the map.
func (m *fieldTagMap[P]) Has(typ reflect.Type, field string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.entries[structFieldKey{typ: typ, field: field}]
	return ok
}

// Remove deletes a (typ, field) entry from the map.
func (m *fieldTagMap[P]) Remove(typ reflect.Type, field string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, structFieldKey{typ: typ, field: field})
}

// Len returns the number of items stored in the map.
func (m *fieldTagMap[P]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// Clear removes all entries from the map.
func (m *fieldTagMap[P]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	clear(m.entries)
}

// @note #397ju0 status resolved todo : Refactor out helpers #cruft
//
// Resolved by removal: typeHash32/fieldHash32/fieldKeyOf and the
// structFieldKey hash-bucket scheme were deleted outright when the map was
// re-keyed on the natively comparable (reflect.Type, string) pair. There are
// no hash helpers left to extract, and the collision-verification caveat they
// existed to mitigate is gone by construction.
