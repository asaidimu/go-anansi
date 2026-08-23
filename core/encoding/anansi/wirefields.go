package anansi

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// wireField is one directly-addressable slot in a schema's flattened wire
// representation: a value that has its own DataContainerKey in the
// container, as opposed to a purely structural (flattened) object field that
// owns no storage of its own.
//
// Two kinds of fields are addressable this way:
//   - Terminal fields (fd.Terminal() == true): ordinary scalars, records,
//     and simple arrays. Their key is path-dependent, computed exactly as
//     Document.Get/Set and the JSON codec compute it (computeLeafKey).
//   - TypeArrayObject fields, which are non-terminal (they own a child
//     schema slot describing each element) but still store their own value
//     — a []*container.DataContainer — directly in the parent container,
//     addressed by the field descriptor alone (internalKey), independent of
//     path.
//
// Plain "object" fields (Union/Composite/Recursive-flattened, DataType ==
// TypeUnknown, non-terminal) own no storage: their descendants are
// enumerated in their place by recursing into the child schema slot with
// the same path prefix, exactly as decodeObjectInto does.
type wireField struct {
	fd   definition.FieldDescriptor
	key  container.DataContainerKey
	name string

	// childIdx is the schema slot describing each element of a
	// TypeArrayObject field. Zero value is meaningless unless
	// fd.DataType() == container.TypeArrayObject.
	childIdx uint8
	// childPath is the resolved path prefix under which each array
	// element's own fields are addressed (spec: this codec mirrors the
	// JSON codec's convention of reusing the array field's own path as
	// the starting prefix for its elements, since elements live in
	// separate DataContainers and the path is only used to derive a
	// stable, collision-free address via the compiled schema).
	childPath definition.ResolvedPath
}

// collectWireFields walks schemaIdx (and, recursively, any flattened object
// child slots reachable from it) and returns every wire-addressable field in
// a stable, deterministic order.
//
// This order is this codec's canonical "field order" for both the Dense
// state map and the Sparse field list. It is a function purely of the
// compiled schema (declaration order, with flattened object fields inlined
// at their declaration point), so it is guaranteed stable across repeated
// calls for the same *definition.CompiledSchema and reproducible between
// independent encode/decode calls — the property the wire format actually
// needs.
//
// Note on deviation from the abstract spec's "ascending DataPoint" field
// order (spec 2.3): in this concrete engine, a terminal field's DataPoint id
// is a path-derived address (definition.CompiledSchema.Address), not a
// small sequential per-schema counter, so sorting by raw DataPoint value
// does not group fields by DataType the way the spec's illustrative example
// assumes. This codec instead defines field order as schema declaration
// order (a fixed, version-stable order given a compiled schema) and groups
// Dense value blocks by DataType per section 3.1.3 independently of that
// order, which satisfies the same structural requirements (fixed-size state
// map, self-delineating per-type value blocks) without depending on the
// specific bit layout of one engine's internal identifiers.
func collectWireFields(cs *definition.CompiledSchema, schemaIdx uint8, prefix definition.ResolvedPath) ([]wireField, error) {
	var out []wireField
	if err := collectWireFieldsInto(cs, schemaIdx, prefix, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func collectWireFieldsInto(cs *definition.CompiledSchema, schemaIdx uint8, prefix definition.ResolvedPath, out *[]wireField) error {
	if int(schemaIdx) >= len(cs.Schemas) {
		return fmt.Errorf("anansi: schema slot %d out of range", schemaIdx)
	}
	slot := cs.Schemas[schemaIdx]
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		name := cs.FieldsMeta[abs].Name
		step := definition.NewResolvedStep(schemaIdx, uint8(j))
		fieldPath := append(append(definition.ResolvedPath{}, prefix...), step)

		if fd.Terminal() {
			key, err := computeLeafKey(cs, fd, fieldPath)
			if err != nil {
				return err
			}
			*out = append(*out, wireField{fd: fd, key: key, name: name})
			continue
		}

		if fd.ChildSchemaIdx() == definition.FdNoChild {
			// Non-terminal with no child schema: nothing to encode (should
			// not normally occur in a well-formed compiled schema).
			continue
		}

		if fd.DataType() == container.TypeArrayObject {
			*out = append(*out, wireField{
				fd:        fd,
				key:       internalKey(fd),
				name:      name,
				childIdx:  fd.ChildSchemaIdx(),
				childPath: fieldPath,
			})
			continue
		}

		// Flattened object/union/composite/recursive-container field: it
		// owns no storage itself; its descendants live at this same path
		// prefix, one schema slot deeper.
		if err := collectWireFieldsInto(cs, fd.ChildSchemaIdx(), fieldPath, out); err != nil {
			return err
		}
	}
	return nil
}

// computeLeafKey resolves a terminal field's path to its DataContainerKey,
// mirroring core/encoding/json's identically-named unexported helper so
// that keys computed here always agree with keys the rest of the engine
// (Document.Get/Set, the JSON codec) would compute for the same field. Both
// call the same underlying *definition.CompiledSchema.Address, which is a
// pure, memoised function of the schema and path — so identical paths
// always yield identical keys regardless of which codec computed them.
func computeLeafKey(cs *definition.CompiledSchema, fd definition.FieldDescriptor, path definition.ResolvedPath) (container.DataContainerKey, error) {
	if len(path) == 0 {
		return internalKey(fd), nil
	}
	addr := cs.Address(path)
	if addr == 0 {
		return internalKey(fd), nil
	}
	dp, err := container.NewDataPoint(fd.DataType(), int32(addr))
	if err != nil {
		return 0, fmt.Errorf("anansi: build data point: %w", err)
	}
	return container.NewDataContainerKey(dp, uint32(fd)), nil
}

// internalKey builds the path-independent, descriptor-derived key used for
// structural fields (TypeArrayObject) whose address does not depend on
// nesting path — mirroring core/encoding/json's identically-named helper.
func internalKey(fd definition.FieldDescriptor) container.DataContainerKey {
	return container.NewDataContainerKey(container.DataPoint(fd.DataPoint()), uint32(fd))
}

// schemaContainsRecursiveField reports whether any field reachable from
// schemaIdx (including through flattened object descendants) has its
// Recursive() bit set. In this engine, recursive fields are always terminal
// (stored as an opaque TypeUnknown value — see classifyField in
// core/schema/definition/link.go), so this walk always terminates without
// needing a visited-slot guard: it never recurses through a recursive
// field itself, only through non-terminal flattened-object fields, which
// are acyclic by construction. This is exposed for diagnostic/informational
// use; unlike the abstract spec (3.1.4), this codec does not need it to
// decide Dense eligibility (see selectPacketType in anansi.go for why).
func schemaContainsRecursiveField(cs *definition.CompiledSchema, schemaIdx uint8) (bool, error) {
	if int(schemaIdx) >= len(cs.Schemas) {
		return false, fmt.Errorf("anansi: schema slot %d out of range", schemaIdx)
	}
	slot := cs.Schemas[schemaIdx]
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		if fd.Recursive() {
			return true, nil
		}
		if fd.Terminal() {
			continue
		}
		if fd.ChildSchemaIdx() == definition.FdNoChild || fd.DataType() == container.TypeArrayObject {
			continue
		}
		rec, err := schemaContainsRecursiveField(cs, fd.ChildSchemaIdx())
		if err != nil {
			return false, err
		}
		if rec {
			return true, nil
		}
	}
	return false, nil
}
