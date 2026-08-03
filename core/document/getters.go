package document

import (
	"time"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/utils"
)

// getLeaf reads a terminal leaf field directly from its container slot, without
// boxing the value through any. Type safety is enforced by construction: the
// schema compiles every field to a fixed DataType and the storage key embeds
// that type (DataContainerKey.Type), so reading a slot with a non-matching
// accessor fails at the container. A mismatched getter is a call-site error and
// returns a type error immediately — no coercion, no reflection.
//
// Record views are map-backed and necessarily box; they fall back to the
// provided coercion. Schema defaults are applied only by the persistence
// layer, so an unset leaf returns ErrKeyNotFound.
func getLeaf[T any](d *Document, keyOrPath string, want container.DataType, op string,
	read func(container.DataContainerKey) (T, bool, error),
	coerce func(any) (T, bool)) (T, error) {
	var zero T
	if d == nil {
		return zero, d.keyErr(keyOrPath)
	}
	if d.isRecord() {
		val, ok := utils.GetValueByPath(d.record, keyOrPath)
		if !ok {
			return zero, d.keyErr(keyOrPath)
		}
		if c, ok := coerce(val); ok {
			return c, nil
		}
		return zero, d.typeErr(keyOrPath, dtName(want), val)
	}
	if keyOrPath == "" {
		return zero, d.keyEmptyErr()
	}
	rp, fd, err := d.resolvePath(keyOrPath)
	if err != nil {
		return zero, err
	}
	if fd.DataType() != want {
		return zero, d.typeErr(keyOrPath, dtName(want), dtName(fd.DataType()))
	}
	if !fd.Terminal() || fd.ChildSchemaIdx() != definition.FdNoChild {
		return zero, d.typeErr(keyOrPath, dtName(want), "non-terminal field")
	}
	k, err := computeLeafKey(d.cs, fd, rp)
	if err != nil {
		return zero, err
	}
	v, ok, err := read(k)
	if err != nil {
		return zero, err
	}
	if !ok {
		return zero, d.keyErr(keyOrPath)
	}
	if d.c.IsNull(k) {
		return zero, nil
	}
	return v, nil
}

// GetString retrieves a string value. The field must be schema-declared as a
// string (enforced by construction); record views coerce.
func (d *Document) GetString(keyOrPath string) (string, error) {
	return getLeaf(d, keyOrPath, container.TypeString, "GetString",
		func(k container.DataContainerKey) (string, bool, error) { return d.c.GetString(k) },
		func(v any) (string, bool) { return utils.CoerceToPrimitiveValue[string](v) })
}

// GetInt retrieves an integer value. The field must be schema-declared as an
// integer (enforced by construction).
func (d *Document) GetInt(keyOrPath string) (int, error) {
	return getLeaf(d, keyOrPath, container.TypeInt, "GetInt",
		func(k container.DataContainerKey) (int, bool, error) {
			n, ok, err := d.c.GetInt(k)
			return int(n), ok, err
		},
		func(v any) (int, bool) { return utils.CoerceToPrimitiveValue[int](v) })
}

// GetFloat64 retrieves a number value. The field must be schema-declared as a
// number (enforced by construction).
func (d *Document) GetFloat64(keyOrPath string) (float64, error) {
	return getLeaf(d, keyOrPath, container.TypeFloat, "GetFloat64",
		func(k container.DataContainerKey) (float64, bool, error) { return d.c.GetFloat(k) },
		func(v any) (float64, bool) { return utils.CoerceToPrimitiveValue[float64](v) })
}

// GetBool retrieves a boolean value. The field must be schema-declared as a
// boolean (enforced by construction).
func (d *Document) GetBool(keyOrPath string) (bool, error) {
	return getLeaf(d, keyOrPath, container.TypeBool, "GetBool",
		func(k container.DataContainerKey) (bool, bool, error) { return d.c.GetBool(k) },
		func(v any) (bool, bool) { return utils.CoerceToPrimitiveValue[bool](v) })
}

// GetStringArray retrieves an array of strings. The field must be a
// schema-declared array of strings (enforced by construction).
func (d *Document) GetStringArray(keyOrPath string) ([]string, error) {
	return getLeaf(d, keyOrPath, container.TypeArrayString, "GetStringArray",
		func(k container.DataContainerKey) ([]string, bool, error) { return d.c.GetArrayString(k) },
		func(v any) ([]string, bool) { return utils.CoerceToSlice[string](v) })
}

// GetIntArray retrieves an array of integers. The field must be a
// schema-declared array of integers (enforced by construction).
func (d *Document) GetIntArray(keyOrPath string) ([]int, error) {
	return getLeaf(d, keyOrPath, container.TypeArrayInt, "GetIntArray",
		func(k container.DataContainerKey) ([]int, bool, error) {
			arr, ok, err := d.c.GetArrayInt(k)
			if err != nil || !ok {
				return nil, ok, err
			}
			out := make([]int, len(arr))
			for i, n := range arr {
				out[i] = int(n)
			}
			return out, true, nil
		},
		func(v any) ([]int, bool) { return utils.CoerceToSlice[int](v) })
}

// GetArray retrieves a generic array value. The field must be a
// schema-declared array of unknown/unresolvable elements (enforced by
// construction); typed arrays use GetStringArray/GetIntArray/….
func (d *Document) GetArray(keyOrPath string) ([]any, error) {
	return getLeaf(d, keyOrPath, container.TypeArrayUnknown, "GetArray",
		func(k container.DataContainerKey) ([]any, bool, error) { return d.c.GetArrayUnknown(k) },
		func(v any) ([]any, bool) { return toAnySlice(v) })
}

// GetTime retrieves a time value. Times are stored either as epoch nanoseconds
// in an integer slot or as a parseable string; the schema-declared type must be
// one of the two (record views parse any stored representation).
func (d *Document) GetTime(keyOrPath string) (time.Time, error) {
	if d == nil {
		return time.Time{}, d.keyErr(keyOrPath)
	}
	if d.isRecord() {
		val, ok := utils.GetValueByPath(d.record, keyOrPath)
		if !ok {
			return time.Time{}, d.keyErr(keyOrPath)
		}
		if t, ok := utils.CoerceTime(val); ok {
			return t, nil
		}
		return time.Time{}, d.typeErr(keyOrPath, "time", val)
	}
	if keyOrPath == "" {
		return time.Time{}, d.keyEmptyErr()
	}
	rp, fd, err := d.resolvePath(keyOrPath)
	if err != nil {
		return time.Time{}, err
	}
	if !fd.Terminal() || fd.ChildSchemaIdx() != definition.FdNoChild {
		return time.Time{}, d.typeErr(keyOrPath, "time", "non-terminal field")
	}
	k, err := computeLeafKey(d.cs, fd, rp)
	if err != nil {
		return time.Time{}, err
	}
	switch fd.DataType() {
	case container.TypeInt: // epoch nanoseconds
		n, ok, err := d.c.GetInt(k)
		if err != nil {
			return time.Time{}, err
		}
		if !ok {
			return time.Time{}, d.keyErr(keyOrPath)
		}
		if d.c.IsNull(k) {
			return time.Time{}, nil
		}
		return time.Unix(0, n), nil
	case container.TypeString: // RFC3339 / ISO8601 / etc.
		s, ok, err := d.c.GetString(k)
		if err != nil {
			return time.Time{}, err
		}
		if !ok {
			return time.Time{}, d.keyErr(keyOrPath)
		}
		if d.c.IsNull(k) {
			return time.Time{}, nil
		}
		if t, ok := utils.CoerceTime(s); ok {
			return t, nil
		}
		return time.Time{}, d.typeErr(keyOrPath, "time", s)
	default:
		return time.Time{}, d.typeErr(keyOrPath, "time", dtName(fd.DataType()))
	}
}

// GetDocument retrieves a nested document view. Object fields yield a
// container-backed view over the same flat container; record fields yield a
// schema-free record view.
func (d *Document) GetDocument(keyOrPath string) (data.Documenter, error) {
	if d == nil {
		return nil, d.keyErr(keyOrPath)
	}
	if d.isRecord() {
		val, err := d.Get(keyOrPath)
		if err != nil {
			return nil, err
		}
		if m, ok := val.(map[string]any); ok {
			return newRecordView(m, d.ctx), nil
		}
		if val == nil {
			return newRecordView(map[string]any{}, d.ctx), nil
		}
		return nil, d.typeErr(keyOrPath, "object/record", val)
	}
	rp, fd, err := d.resolvePath(keyOrPath)
	if err != nil {
		return nil, err
	}
	if fd.Terminal() || fd.ChildSchemaIdx() == definition.FdNoChild {
		return nil, d.typeErr(keyOrPath, "object/record", "scalar")
	}
	child := fd.ChildSchemaIdx()
	switch fd.DataType() {
	case container.TypeRecord:
		k := internalKey(fd)
		v, ok, err := d.c.GetRecord(k)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, d.keyErr(keyOrPath)
		}
		return newRecordView(v, d.ctx), nil
	case container.TypeArrayObject:
		return nil, d.typeErr(keyOrPath, "a single document", "array of documents")
	default:
		return d.newNestedView(child, rp), nil
	}
}

// GetDocumentArray retrieves an array of document views for array-of-object
// fields.
func (d *Document) GetDocumentArray(keyOrPath string) ([]data.Documenter, error) {
	if d == nil {
		return nil, d.keyErr(keyOrPath)
	}
	if d.isRecord() {
		val, err := d.Get(keyOrPath)
		if err != nil {
			return nil, err
		}
		switch v := val.(type) {
		case []map[string]any:
			out := make([]data.Documenter, len(v))
			for i, m := range v {
				out[i] = newRecordView(m, d.ctx)
			}
			return out, nil
		case []any:
			out := make([]data.Documenter, 0, len(v))
			for _, e := range v {
				m, ok := e.(map[string]any)
				if !ok {
					return nil, d.typeErr(keyOrPath, "array of documents", val)
				}
				out = append(out, newRecordView(m, d.ctx))
			}
			return out, nil
		}
		return nil, d.typeErr(keyOrPath, "array of documents", val)
	}
	rp, fd, err := d.resolvePath(keyOrPath)
	if err != nil {
		return nil, err
	}
	if fd.Terminal() || fd.ChildSchemaIdx() == definition.FdNoChild || fd.DataType() != container.TypeArrayObject {
		return nil, d.typeErr(keyOrPath, "array of documents", "not an array of objects")
	}
	child := fd.ChildSchemaIdx()
	k := internalKey(fd)
	children, ok, err := d.c.GetArrayObject(k)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, d.keyErr(keyOrPath)
	}
	out := make([]data.Documenter, len(children))
	for i, ch := range children {
		out[i] = d.newNestedViewForChild(ch, child, rp)
	}
	return out, nil
}

// newNestedView builds a container-backed view sharing the root container.
func (d *Document) newNestedView(child uint8, rp definition.ResolvedPath) *Document {
	return &Document{
		cs:      d.cs,
		c:       d.c,
		ctx:     d.ctx,
		prefix:  append(definition.ResolvedPath(nil), rp...),
		slotIdx: child,
	}
}

// newNestedViewForChild builds a container-backed view over an array element's
// own container.
func (d *Document) newNestedViewForChild(child *container.DataContainer, childSlot uint8, rp definition.ResolvedPath) *Document {
	return &Document{
		cs:      d.cs,
		c:       child,
		ctx:     d.ctx,
		prefix:  append(definition.ResolvedPath(nil), rp...),
		slotIdx: childSlot,
	}
}

// dtName renders a DataType for error messages.
func dtName(dt container.DataType) string {
	switch dt {
	case container.TypeInt:
		return "integer"
	case container.TypeFloat:
		return "number"
	case container.TypeString:
		return "string"
	case container.TypeBool:
		return "boolean"
	case container.TypeBytes:
		return "bytes"
	case container.TypeGeometry:
		return "geometry"
	case container.TypeRecord:
		return "record"
	case container.TypeUnknown:
		return "any"
	case container.TypeArrayInt:
		return "array of integers"
	case container.TypeArrayFloat:
		return "array of numbers"
	case container.TypeArrayString:
		return "array of strings"
	case container.TypeArrayBool:
		return "array of booleans"
	case container.TypeArrayBytes:
		return "array of bytes"
	case container.TypeArrayObject:
		return "array of objects"
	case container.TypeArrayGeometry:
		return "array of geometry"
	case container.TypeArrayUnknown:
		return "array"
	}
	return "unknown"
}
