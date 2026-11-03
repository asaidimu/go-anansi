package document

import (
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ============================================================================
// PATH → ADDRESS RESOLUTION
// ============================================================================
//
// String keys and dotted paths resolve against a CompiledSchema using the same
// two-layer address scheme the schema compiler defines:
//
//   - ResolvePath turns a dotted path into (SchemaIdx, FieldIdx) steps.
//   - Address computes a flat 27-bit user-data address for terminal leaves.
//   - The address combined with the leaf FieldDescriptor forms the
//     DataContainerKey under which the value is stored.
//
// Nested object views carry a prefix (the resolved steps leading to the
// object) so their relative keys resolve against the same flat container.
//
// The address/typed-IO/materialization substrate itself moved to
// core/schema/binding (shared with serializers); the functions below keep the
// document-internal names used across this package as thin delegates.

// resolvePath resolves a dotted path relative to this document's view (prefix +
// slot), returning the full resolved path and the leaf field descriptor.
func (d *Document) resolvePath(path string) (definition.ResolvedPath, definition.FieldDescriptor, error) {
	if path == "" {
		return nil, 0, d.keyErr("")
	}
	if d == nil || d.cs == nil {
		return nil, 0, d.keyErr(path)
	}
	segments := strings.Split(path, ".")
	rp := make(definition.ResolvedPath, 0, len(segments)+len(d.prefix))
	rp = append(rp, d.prefix...)
	slotIdx := d.slotIdx
	for i, segment := range segments {
		step, fd, err := d.cs.ResolveFieldStep(slotIdx, segment)
		if err != nil {
			return nil, 0, d.keyErr(path)
		}
		rp = append(rp, step)
		if i == len(segments)-1 {
			return rp, fd, nil
		}
		if fd.Terminal() || fd.ChildSchemaIdx() == definition.FdNoChild {
			return nil, 0, d.invalidPathErr(path, fmt.Sprintf("field %q is terminal and cannot be descended into", segment))
		}
		slotIdx = fd.ChildSchemaIdx()
	}
	return nil, 0, d.invalidPathErr(path, "cannot resolve path")
}

// computeLeafKey resolves a ResolvedPath to its flat storage key. Non-terminal
// paths fall back to the internal (structural) key, matching the reference
// decoder.
func computeLeafKey(cs *definition.CompiledSchema, fd definition.FieldDescriptor, path definition.ResolvedPath) (container.DataContainerKey, error) {
	return binding.ComputeLeafKey(cs, fd, path)
}

// internalKey builds the structural key used for non-flattened fields
// (records, array-of-object), whose DataPoint ID derives from the descriptor
// rather than a flat address.
func internalKey(fd definition.FieldDescriptor) container.DataContainerKey {
	return binding.InternalKey(fd)
}

// descriptorForStep returns the FieldDescriptor for the final step of a
// resolved path.
func descriptorForStep(cs *definition.CompiledSchema, step definition.ResolvedStep) (definition.FieldDescriptor, bool) {
	return binding.DescriptorForStep(cs, step)
}

// ============================================================================
// TYPED READ / WRITE
// ============================================================================

// getByType reads a value from a container slot matching the descriptor type.
func getByType(c *container.DataContainer, dt container.DataType, key container.DataContainerKey) (any, bool, error) {
	return binding.GetByType(c, dt, key)
}

// setByType writes a value into a container slot matching the descriptor type.
func setByType(c *container.DataContainer, dt container.DataType, key container.DataContainerKey, value any) error {
	return binding.SetByType(c, dt, key, value)
}

// ============================================================================
// SCHEMA-DRIVEN MATERIALIZATION
// ============================================================================

// materializeSlot materializes the object at schema slot slotIdx whose fields
// are stored in container c under the given resolved path. It is the single
// source of truth for map-shaped reads (Get of non-terminal objects, ToMap,
// Data). Schema defaults are not injected here — only the persistence layer
// applies default values — so absent fields stay absent.
func materializeSlot(cs *definition.CompiledSchema, c *container.DataContainer, slotIdx uint8, path definition.ResolvedPath) (map[string]any, error) {
	return binding.MaterializeSlot(cs, c, slotIdx, path)
}

// present reports whether the field at the given resolved path holds a value
// (including an explicit null) in the container.
func present(cs *definition.CompiledSchema, c *container.DataContainer, fd definition.FieldDescriptor, rp definition.ResolvedPath) (bool, error) {
	return binding.Present(cs, c, fd, rp)
}

// anyDescendantPresent reports whether any leaf under the given object slot is
// set in the container.
func anyDescendantPresent(cs *definition.CompiledSchema, c *container.DataContainer, slotIdx uint8, path definition.ResolvedPath) (bool, error) {
	return binding.AnyDescendantPresent(cs, c, slotIdx, path)
}

// unsetPath removes the value at the given resolved path from the container,
// including the entire subtree of a flattened object.
func unsetPath(cs *definition.CompiledSchema, c *container.DataContainer, fd definition.FieldDescriptor, rp definition.ResolvedPath) error {
	return binding.UnsetPath(cs, c, fd, rp)
}

// unsetSubtree removes every leaf under the object slot path.
func unsetSubtree(cs *definition.CompiledSchema, c *container.DataContainer, slotIdx uint8, path definition.ResolvedPath) error {
	return binding.UnsetSubtree(cs, c, slotIdx, path)
}

// setInto writes value into container c for the field (slotIdx, fieldIdx),
// where base is the resolved path from the document root to the object that
// contains this field. For the root container and flattened objects, base is
// the object's own path; for array-of-object children, base is the array
// field's path. pool, when non-nil, supplies child containers for
// array-of-object fields so struct->container binds reuse pooled containers
// instead of allocating fresh ones.
func setInto(cs *definition.CompiledSchema, c *container.DataContainer, pool *container.Pool, slotIdx uint8, fieldIdx uint16, base definition.ResolvedPath, value any) error {
	return binding.SetInto(cs, c, pool, slotIdx, fieldIdx, base, value)
}

// appendPath returns a new ResolvedPath with step appended (copy-on-write).
func appendPath(path definition.ResolvedPath, step definition.ResolvedStep) definition.ResolvedPath {
	return binding.AppendPath(path, step)
}

// ============================================================================
// VALUE COERCION (delegates)
// ============================================================================

func asInt64(v any) (int64, error)   { return binding.AsInt64(v) }
func asFloat64(v any) (float64, error) { return binding.AsFloat64(v) }
func asString(v any) string          { return binding.AsString(v) }

// toAnySlice normalizes a slice-like value into []any for element coercion.
func toAnySlice(v any) ([]any, bool) { return binding.ToAnySlice(v) }
