package schema

import (
	"reflect"
	"sync"
)

// AnansiTag is the canonical struct-tag key understood by the anansi binding
// and schema-extraction machinery. The canonical value lives here so the
// extractor, both binders, and downstream packages share one definition.
const AnansiTag = "anansi"

// ============================================================================
// System Model Registry (canonical home)
// ============================================================================
//
// This is the single registry of embedded system-model types (structs that
// carry the system document fields _id_ and _metadata_). It was previously
// duplicated between core/data (map-backed binder) and core/schema/binding
// (struct binder); both now delegate here, as does the DTO schema extractor
// in this package.

// systemModelRegistration describes an embedded model type that carries the
// system document fields (_id_, _metadata_) and must be treated specially by
// binding and extraction: its parent reference is restored after each
// successful bind so promoted methods can reach the enclosing struct.
type systemModelRegistration struct {
	typ reflect.Type
	// linkParent restores the embedded model's parent-struct reference after
	// binding. It is provided by the registering package, keeping parent
	// manipulation internal to that package.
	linkParent func(embed any, parent any)
}

var (
	systemModelRegistrationsMu sync.RWMutex
	systemModelRegistrations   []systemModelRegistration
)

// RegisterSystemModelType registers an embedded model type that carries
// system document fields (_id_, _metadata_) and must be treated like
// DocumentModel by struct binding, struct extraction, and DTO schema
// generation. Register at package init, before any binds or walks run.
//
// linkParent restores the embedded model's parent-struct reference after
// binding, enabling promoted methods (e.g. Document, Patch) on materialized
// read results. It lives in the registering package so parent manipulation
// stays internal; pass nil to skip parent linking.
func RegisterSystemModelType(t reflect.Type, linkParent func(embed any, parent any)) {
	systemModelRegistrationsMu.Lock()
	defer systemModelRegistrationsMu.Unlock()
	for _, existing := range systemModelRegistrations {
		if existing.typ == t {
			return
		}
	}
	systemModelRegistrations = append(systemModelRegistrations, systemModelRegistration{typ: t, linkParent: linkParent})
}

// IsSystemModelType reports whether t is a registered system-model embed.
func IsSystemModelType(t reflect.Type) bool {
	systemModelRegistrationsMu.RLock()
	defer systemModelRegistrationsMu.RUnlock()
	for _, reg := range systemModelRegistrations {
		if reg.typ == t {
			return true
		}
	}
	return false
}

// LinkSystemModelParents restores the parent reference on every embedded
// registered system model within outer, so promoted methods can access the
// outer struct on read-back results.
func LinkSystemModelParents(outer reflect.Value, parent any) {
	systemModelRegistrationsMu.RLock()
	defer systemModelRegistrationsMu.RUnlock()
	for _, reg := range systemModelRegistrations {
		if reg.linkParent == nil {
			continue
		}
		if idx, ok := findEmbeddedTypeIndex(outer.Type(), reg.typ); ok {
			fv := outer.FieldByIndex(idx)
			if fv.Kind() == reflect.Struct && fv.CanAddr() {
				reg.linkParent(fv.Addr().Interface(), parent)
			}
		}
	}
}

// findEmbeddedTypeIndex returns the field index path of the first anonymous
// embed of type target within t, or (nil, false).
func findEmbeddedTypeIndex(t reflect.Type, target reflect.Type) ([]int, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.Anonymous {
			continue
		}
		if f.Type == target {
			return []int{i}, true
		}
		if f.Type.Kind() == reflect.Struct {
			if sub, ok := findEmbeddedTypeIndex(f.Type, target); ok {
				return append([]int{i}, sub...), true
			}
		}
	}
	return nil, false
}
