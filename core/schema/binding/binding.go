// Package binding implements the schema-driven struct ↔ container binding
// substrate shared by the document system and lightweight serializers.
//
// Everything here operates on a compiled schema (definition.CompiledSchema)
// and a data container (container.DataContainer): dotted-path resolution,
// typed slot reads and writes, struct field walking (write path) and struct
// binding (read path). It sits below the document model: identity (_id_),
// metadata (_metadata_), checksums and generated defaults are NOT handled —
// core/document consumes this package and layers those concerns on top by
// enriching its schemas with the system fields (data.EnrichSchema).
// Serializers that want clean, deterministic output compile a non-enriched
// schema (Pool, PoolForType) and use Populate and Bind directly.
package binding

import (
	"reflect"
	"sync"

	"github.com/asaidimu/go-anansi/v8/core/common"
)

const AnansiTag = "anansi"

const DocumentIDField = "_id_"
const MetadataField = "_metadata_"

// ReservedBindName reports whether key is a system-managed field name whose
// routing is decided by the source, not by generic Get.
func ReservedBindName(key string) bool {
	return key == DocumentIDField || key == MetadataField
}

// ---------- Errors ----------

var (
	ErrBindInvalidTargetType    = common.NewSystemError("ERR_DOCUMENT_BIND_INVALID_TARGET_TYPE", "invalid target type")
	ErrBindFailedToSetField     = common.NewSystemError("ERR_DOCUMENT_BIND_FAILED_TO_SET_FIELD", "failed to set field")
	ErrBindTypeConversionFailed = common.NewSystemError("ERR_DOCUMENT_BIND_TYPE_CONVERSION_FAILED", "type conversion failed")
	ErrBindEmptyFieldName       = common.NewSystemError("ERR_DOCUMENT_BIND_EMPTY_FIELD_NAME", "doc tag has empty field name")
	ErrBindUnknownDocTagOption  = common.NewSystemError("ERR_DOCUMENT_BIND_UNKNOWN_DOC_TAG_OPTION", "unknown doc tag option")
	ErrBindKeyNotFound          = common.NewSystemError("ERR_DOCUMENT_BIND_KEY_NOT_FOUND", "key not found")
	ErrBindCannotTraverse       = common.NewSystemError("ERR_DOCUMENT_BIND_CANNOT_TRAVERSE", "cannot traverse into non-object value")

	// Mirrors of the shared data-layer error values, so path resolution inside
	// a binding view reports the same error codes as the data package without
	// importing it.
	errKeyNotFound = common.NewSystemError("ERR_DATA_KEY_NOT_FOUND", "key not found")
	errInvalidPath = common.NewSystemError("ERR_DATA_INVALID_PATH", "invalid path")
)

// ---------- Sources ----------

// Source is the minimal per-field view the binder needs: values fetched by
// schema field name. Container-backed sources normally also implement
// TypedBinder for box-free slot copies.
type Source interface {
	Get(key string) (any, error)
}

// systemSource is optionally implemented by sources that own document
// identity and metadata. When the target struct declares _id_/_metadata_
// fields, the binder prefers these over Source.Get. Plain container sources
// (no document model) do not implement it; their _id_/_metadata_ fields, if
// any, resolve as ordinary schema fields.
type systemSource interface {
	ID() string
	Metadata() map[string]any
}

// TypedBinder is implemented by sources that can copy a field's value
// straight into a struct field without boxing it through any. Returning
// (false, nil) makes the caller fall back to the generic Source.Get path,
// preserving coercion and error semantics for field kinds the source cannot
// bind directly.
type TypedBinder interface {
	BindField(name string, rv reflect.Value, tag string) (bool, error)
}

// mapSource adapts a nested map[string]any into a Source, mirroring the
// semantics the binder has always had for ad-hoc nested documents: no
// identity and an empty (non-nil) metadata map.
type mapSource struct {
	data map[string]any
}

func (m mapSource) Get(key string) (any, error) {
	v, ok := m.data[key]
	if !ok {
		return nil, ErrBindKeyNotFound
	}
	return v, nil
}

func (m mapSource) ID() string { return "" }

func (m mapSource) Metadata() map[string]any { return make(map[string]any) }

// ---------- System Model Registry ----------

// systemModelRegistration describes an embedded model type that carries the
// system document fields (_id_, _metadata_) and must be treated specially by
// binding: its parent reference is restored after each successful bind so
// promoted methods can reach the enclosing struct.
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
// DocumentModel by the binder. Register at package init, before any binds or
// walks run. The document package registers its own model this way.
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

// linkSystemModelParents restores the parent reference on every embedded
// registered system model within outer, so promoted methods can access the
// outer struct on read-back results.
func linkSystemModelParents(outer reflect.Value, parent any) {
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
