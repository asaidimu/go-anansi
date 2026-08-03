package document

import (
	"errors"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
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

// resolveSchemaPath resolves a dotted path against a schema from the root slot,
// delegating the step walk to CompiledSchema.ResolvePath. The leaf
// FieldDescriptor is recovered from the final step of the resolved path.
func resolveSchemaPath(cs *definition.CompiledSchema, path string) (definition.ResolvedPath, definition.FieldDescriptor, error) {
	rp, err := cs.ResolvePath(path)
	if err != nil {
		return nil, 0, err
	}
	fd, ok := descriptorForStep(cs, rp[len(rp)-1])
	if !ok {
		return nil, 0, fmt.Errorf("document: cannot resolve path %q", path)
	}
	return rp, fd, nil
}

// descriptorForStep returns the FieldDescriptor for the final step of a
// resolved path.
func descriptorForStep(cs *definition.CompiledSchema, step definition.ResolvedStep) (definition.FieldDescriptor, bool) {
	if cs == nil || int(step.SchemaIdx()) >= len(cs.Schemas) {
		return 0, false
	}
	slot := cs.Schemas[step.SchemaIdx()]
	abs := int(slot.FieldStart) + int(step.FieldIdx())
	if abs < 0 || abs >= len(cs.Descriptors) {
		return 0, false
	}
	return cs.Descriptors[abs], true
}

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
	addr := cs.Address(path)
	if addr == 0 {
		return internalKey(fd), nil
	}
	dp, err := container.NewDataPoint(fd.DataType(), int32(addr))
	if err != nil {
		return 0, err
	}
	return container.NewDataContainerKey(dp, uint32(fd)), nil
}

// internalKey builds the structural key used for non-flattened fields
// (records, array-of-object), whose DataPoint ID derives from the descriptor
// rather than a flat address.
func internalKey(fd definition.FieldDescriptor) container.DataContainerKey {
	return container.NewDataContainerKey(container.DataPoint(fd.DataPoint()), uint32(fd))
}

// ============================================================================
// TYPED READ / WRITE
// ============================================================================

// getByType reads a value from a container slot matching the descriptor type.
func getByType(c *container.DataContainer, dt container.DataType, key container.DataContainerKey) (any, bool, error) {
	switch dt {
	case container.TypeInt:
		return c.GetInt(key)
	case container.TypeFloat:
		return c.GetFloat(key)
	case container.TypeString:
		return c.GetString(key)
	case container.TypeBool:
		return c.GetBool(key)
	case container.TypeBytes:
		return c.GetBytes(key)
	case container.TypeGeometry:
		return c.GetGeometry(key)
	case container.TypeRecord:
		return c.GetRecord(key)
	case container.TypeArrayInt:
		return c.GetArrayInt(key)
	case container.TypeArrayFloat:
		return c.GetArrayFloat(key)
	case container.TypeArrayString:
		return c.GetArrayString(key)
	case container.TypeArrayBool:
		return c.GetArrayBool(key)
	case container.TypeArrayBytes:
		return c.GetArrayBytes(key)
	case container.TypeArrayGeometry:
		return c.GetArrayGeometry(key)
	case container.TypeArrayObject:
		return c.GetArrayObject(key)
	case container.TypeArrayUnknown:
		return c.GetArrayUnknown(key)
	case container.TypeUnknown:
		return c.GetUnknown(key)
	}
	return nil, false, fmt.Errorf("document: unsupported data type %d", dt)
}

// setByType writes a value into a container slot matching the descriptor type.
func setByType(c *container.DataContainer, dt container.DataType, key container.DataContainerKey, value any) error {
	if value == nil {
		c.SetNull(key)
		return nil
	}
	switch dt {
	case container.TypeInt:
		n, err := asInt64(value)
		if err != nil {
			return err
		}
		return c.SetInt(key, n)
	case container.TypeFloat:
		f, err := asFloat64(value)
		if err != nil {
			return err
		}
		return c.SetFloat(key, f)
	case container.TypeString:
		return c.SetString(key, asString(value))
	case container.TypeBool:
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("document: expected boolean, got %T", value)
		}
		return c.SetBool(key, b)
	case container.TypeBytes:
		return c.SetBytes(key, []byte(asString(value)))
	case container.TypeGeometry:
		g, err := asGeometry(value)
		if err != nil {
			return err
		}
		return c.SetGeometry(key, g)
	case container.TypeRecord:
		m, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("document: expected record (map), got %T", value)
		}
		return c.SetRecord(key, m)
	case container.TypeArrayInt:
		v, err := asInt64Slice(value)
		if err != nil {
			return err
		}
		return c.SetArrayInt(key, v)
	case container.TypeArrayFloat:
		v, err := asFloat64Slice(value)
		if err != nil {
			return err
		}
		return c.SetArrayFloat(key, v)
	case container.TypeArrayString:
		return c.SetArrayString(key, asStringSlice(value))
	case container.TypeArrayBool:
		v, err := asBoolSlice(value)
		if err != nil {
			return err
		}
		return c.SetArrayBool(key, v)
	case container.TypeArrayBytes:
		return c.SetArrayBytes(key, asBytesSlice(value))
	case container.TypeArrayGeometry:
		v, err := asGeometrySlice(value)
		if err != nil {
			return err
		}
		return c.SetArrayGeometry(key, v)
	case container.TypeArrayUnknown:
		v, ok := value.([]any)
		if !ok {
			return fmt.Errorf("document: expected array, got %T", value)
		}
		return c.SetArrayUnknown(key, v)
	case container.TypeUnknown:
		return c.SetUnknown(key, value)
	}
	return fmt.Errorf("document: unsupported data type %d", dt)
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
	if cs == nil || int(slotIdx) >= len(cs.Schemas) {
		return nil, fmt.Errorf("document: schema slot %d out of range", slotIdx)
	}
	slot := cs.Schemas[slotIdx]
	out := make(map[string]any, slot.FieldCount)
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		name := cs.FieldsMeta[abs].Name
		fp := appendPath(path, definition.NewResolvedStep(slotIdx, uint8(j)))

		if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
			childIdx := fd.ChildSchemaIdx()
			switch fd.DataType() {
			case container.TypeRecord:
				k := internalKey(fd)
				if v, ok, err := c.GetRecord(k); err != nil {
					return nil, err
				} else if ok {
					out[name] = v
				}
			case container.TypeArrayObject:
				k := internalKey(fd)
				if children, ok, err := c.GetArrayObject(k); err != nil {
					return nil, err
				} else if ok {
					arr := make([]any, len(children))
					for i, childC := range children {
						m, err := materializeSlot(cs, childC, childIdx, fp)
						if err != nil {
							return nil, err
						}
						arr[i] = m
					}
					out[name] = arr
				}
			default:
				present, err := anyDescendantPresent(cs, c, childIdx, fp)
				if err != nil {
					return nil, err
				}
				if !present {
					continue
				}
				m, err := materializeSlot(cs, c, childIdx, fp)
				if err != nil {
					return nil, err
				}
				out[name] = m
			}
			continue
		}

		k, err := computeLeafKey(cs, fd, fp)
		if err != nil {
			return nil, err
		}
		v, ok, err := getByType(c, fd.DataType(), k)
		if err != nil {
			return nil, err
		}
		if ok {
			if c.IsNull(k) {
				out[name] = nil
			} else {
				out[name] = v
			}
		}
	}
	return out, nil
}

// present reports whether the field at the given resolved path holds a value
// (including an explicit null) in the container.
func present(cs *definition.CompiledSchema, c *container.DataContainer, fd definition.FieldDescriptor, rp definition.ResolvedPath) (bool, error) {
	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		child := fd.ChildSchemaIdx()
		switch fd.DataType() {
		case container.TypeRecord:
			return c.IsSet(internalKey(fd)), nil
		case container.TypeArrayObject:
			return c.IsSet(internalKey(fd)), nil
		default:
			return anyDescendantPresent(cs, c, child, rp)
		}
	}
	k, err := computeLeafKey(cs, fd, rp)
	if err != nil {
		return false, err
	}
	if c.IsSet(k) {
		return true, nil
	}
	// A schema default is applied only by the persistence layer, so an unset
	// field is absent.
	return false, nil
}

// anyDescendantPresent reports whether any leaf under the given object slot is
// set in the container.
func anyDescendantPresent(cs *definition.CompiledSchema, c *container.DataContainer, slotIdx uint8, path definition.ResolvedPath) (bool, error) {
	if cs == nil || int(slotIdx) >= len(cs.Schemas) {
		return false, nil
	}
	slot := cs.Schemas[slotIdx]
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		fp := appendPath(path, definition.NewResolvedStep(slotIdx, uint8(j)))
		ok, err := present(cs, c, fd, fp)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// unsetPath removes the value at the given resolved path from the container,
// including the entire subtree of a flattened object.
func unsetPath(cs *definition.CompiledSchema, c *container.DataContainer, fd definition.FieldDescriptor, rp definition.ResolvedPath) error {
	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		child := fd.ChildSchemaIdx()
		switch fd.DataType() {
		case container.TypeRecord:
			c.Unset(internalKey(fd))
			return nil
		case container.TypeArrayObject:
			if children, ok, err := c.GetArrayObject(internalKey(fd)); err == nil && ok {
				for _, ch := range children {
					if ch != nil {
						ch.Clear()
					}
				}
			}
			c.Unset(internalKey(fd))
			return nil
		default:
			return unsetSubtree(cs, c, child, rp)
		}
	}
	k, err := computeLeafKey(cs, fd, rp)
	if err != nil {
		return err
	}
	c.Unset(k)
	return nil
}

// unsetSubtree removes every leaf under the object slot path.
func unsetSubtree(cs *definition.CompiledSchema, c *container.DataContainer, slotIdx uint8, path definition.ResolvedPath) error {
	if cs == nil || int(slotIdx) >= len(cs.Schemas) {
		return nil
	}
	slot := cs.Schemas[slotIdx]
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		fp := appendPath(path, definition.NewResolvedStep(slotIdx, uint8(j)))
		if err := unsetPath(cs, c, fd, fp); err != nil {
			return err
		}
	}
	return nil
}

// ============================================================================
// MAP → CONTAINER POPULATION
// ============================================================================

// setInto writes value into container c for the field (slotIdx, fieldIdx),
// where base is the resolved path from the document root to the object that
// contains this field. For the root container and flattened objects, base is
// the object's own path; for array-of-object children, base is the array
// field's path.
func setInto(cs *definition.CompiledSchema, c *container.DataContainer, slotIdx uint8, fieldIdx uint16, base definition.ResolvedPath, value any) error {
	if cs == nil || int(slotIdx) >= len(cs.Schemas) {
		return fmt.Errorf("document: schema slot %d out of range", slotIdx)
	}
	slot := cs.Schemas[slotIdx]
	abs := int(slot.FieldStart) + int(fieldIdx)
	fd := cs.Descriptors[abs]
	name := cs.FieldsMeta[abs].Name
	fp := appendPath(base, definition.NewResolvedStep(slotIdx, uint8(fieldIdx)))

	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		child := fd.ChildSchemaIdx()
		switch fd.DataType() {
		case container.TypeRecord:
			m, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("document: field %q expects a record (map), got %T", name, value)
			}
			return c.SetRecord(internalKey(fd), m)
		case container.TypeArrayObject:
			return setArrayObject(cs, c, fd, child, fp, value)
		default:
			m, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("document: field %q expects an object (map), got %T", name, value)
			}
			return setMapInto(cs, c, child, fp, m)
		}
	}

	k, err := computeLeafKey(cs, fd, fp)
	if err != nil {
		return err
	}
	return setByType(c, fd.DataType(), k, value)
}

// setMapInto populates every field of the object at slotIdx from m. Values
// live in container c, addressed under base (the object's own resolved path).
func setMapInto(cs *definition.CompiledSchema, c *container.DataContainer, slotIdx uint8, base definition.ResolvedPath, m map[string]any) error {
	if cs == nil || int(slotIdx) >= len(cs.Schemas) {
		return fmt.Errorf("document: schema slot %d out of range", slotIdx)
	}
	slot := cs.Schemas[slotIdx]
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		name := cs.FieldsMeta[abs].Name
		val, ok := m[name]
		if !ok {
			continue
		}
		if err := setInto(cs, c, slotIdx, j, base, val); err != nil {
			return err
		}
	}
	return nil
}

// setArrayObject populates an array-of-object field. Each item becomes a child
// container whose leaves are addressed under the array field's own path.
func setArrayObject(cs *definition.CompiledSchema, c *container.DataContainer, fd definition.FieldDescriptor, childIdx uint8, base definition.ResolvedPath, value any) error {
	items, err := toMapSlice(value)
	if err != nil {
		return err
	}
	children := make([]*container.DataContainer, len(items))
	for i, item := range items {
		child := container.NewDataContainer()
		if err := setMapInto(cs, child, childIdx, base, item); err != nil {
			return err
		}
		children[i] = child
	}
	return c.SetArrayObject(internalKey(fd), children)
}

// toMapSlice coerces an array-of-object value into []map[string]any, accepting
// []any, []map[string]any, and slices of Documenters.
func toMapSlice(value any) ([]map[string]any, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []map[string]any:
		return v, nil
	case []any:
		out := make([]map[string]any, len(v))
		for i, e := range v {
			m, ok := e.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("document: array element %d is not an object (map), got %T", i, e)
			}
			out[i] = m
		}
		return out, nil
	default:
		return nil, fmt.Errorf("document: expected array of objects, got %T", value)
	}
}

// appendPath returns a new ResolvedPath with step appended (copy-on-write).
func appendPath(path definition.ResolvedPath, step definition.ResolvedStep) definition.ResolvedPath {
	out := make(definition.ResolvedPath, len(path), len(path)+1)
	copy(out, path)
	return append(out, step)
}

var errRecordMode = errors.New("document: operation not supported on a record view")
