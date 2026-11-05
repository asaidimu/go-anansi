package schema

import (
	"github.com/asaidimu/go-anansi/v8/core/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// Ref is the immutable schema context a data structure carries. For a
// DataContainer it records which compiled schema the container belongs to,
// which schema slot scopes it, and where it sits inside that schema.
//
// Root containers use Slot 0 and a nil Path. A view onto a flattened nested
// object carries the slot and the resolved path from the root; an
// array-of-object child carries the child slot and the path of the field that
// owns it. Keeping the context here (rather than a bare *CompiledSchema) is
// what lets a container serialize itself without the owner passing the schema
// at every call site.
//
// A Ref is shared by every container stamped with it and must be treated as
// immutable after construction.
type Ref struct {
	CS   *definition.CompiledSchema
	Slot uint8
	Path definition.ResolvedPath

	// Containers is the optional allocation seam used when materializing
	// nested containers (array-of-object elements). Codecs call Get through
	// this interface and stay ignorant of whether the owner pools, arenas, or
	// freshly allocates them. It is never required for serialization.
	Containers container.Factory
}

// RootRef returns the schema context for a root-level container.
func RootRef(cs *definition.CompiledSchema, factory container.Factory) *Ref {
	return &Ref{CS: cs, Containers: factory}
}

// NewRef returns a schema context for a nested container. path is copied so
// later mutation of the caller's slice cannot corrupt the immutable ref.
func NewRef(cs *definition.CompiledSchema, slot uint8, path definition.ResolvedPath, factory container.Factory) *Ref {
	var owned definition.ResolvedPath
	if len(path) > 0 {
		owned = make(definition.ResolvedPath, len(path))
		copy(owned, path)
	}
	return &Ref{CS: cs, Slot: slot, Path: owned, Containers: factory}
}

// DottedPath renders the Ref's path as the dotted field-name prefix the
// container codecs expect. It returns "" for a root Ref. The returned string
// is only ever used for naming during serialization, never as a key.
func (r *Ref) DottedPath() string {
	if r == nil || len(r.Path) == 0 || r.CS == nil {
		return ""
	}
	parts := make([]string, 0, len(r.Path))
	for _, step := range r.Path {
		schemaIdx := int(step.SchemaIdx())
		if schemaIdx >= len(r.CS.Schemas) {
			continue
		}
		slot := r.CS.Schemas[schemaIdx]
		abs := int(slot.FieldStart) + int(step.FieldIdx())
		if abs < 0 || abs >= len(r.CS.FieldsMeta) {
			continue
		}
		parts = append(parts, r.CS.FieldsMeta[abs].Name)
	}
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "." + p
	}
	return out
}
