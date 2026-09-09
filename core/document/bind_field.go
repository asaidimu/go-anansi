package document

import (
	"fmt"
	"reflect"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

var bindFieldTimeType = reflect.TypeFor[time.Time]()

// BindField implements the document binder's typedFieldBinder interface. It
// copies a schema field's value straight from the container into rv without
// boxing the value through an interface (any). It handles terminal scalars
// (int/float/string/bool/bytes), primitive arrays, nested objects, and
// array-of-object. Field kinds it cannot bind directly (records, geometry,
// time.Time, unknown, non-struct/slice targets) return (false, nil) so the
// binder falls back to the generic FieldSource.Get path with its existing
// coercion and error semantics.
func (d *Document) BindField(name string, rv reflect.Value, tag string) (bool, error) {
	if d == nil || d.cs == nil || d.c == nil || d.isRecord() || reservedBindField(name) {
		return false, nil
	}
	if !rv.IsValid() || !rv.CanSet() {
		return false, nil
	}
	rp, fd, err := d.resolvePath(name)
	if err != nil {
		return false, nil
	}
	return d.bindSlot(rp, fd, rv, tag)
}

func (d *Document) bindSlot(rp definition.ResolvedPath, fd definition.FieldDescriptor, rv reflect.Value, tag string) (bool, error) {
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			// Allocate the pointer only when the slot actually holds a value.
			// Otherwise an absent field binds as a non-nil pointer to zero,
			// which partial-update paths (FromPartialStruct) treat as present
			// and clobber stored data with.
			ok, err := present(d.cs, d.c, fd, rp)
			if err != nil {
				return false, err
			}
			if !ok {
				return true, nil // absence → leave the field nil
			}
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return d.bindSlot(rp, fd, rv.Elem(), tag)
	}
	if !rv.IsValid() || !rv.CanSet() {
		return false, nil
	}

	// Nested documents bind straight from a container-backed child view, so no
	// intermediate map[string]any is materialized.
	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		switch fd.DataType() {
		case container.TypeArrayObject:
			return d.bindArrayObject(rp, fd, rv, tag)
		case container.TypeRecord:
			return false, nil
		default:
			return d.bindNestedObject(rp, fd, rv, tag)
		}
	}

	// Terminal leaf: read the typed slot directly.
	k, err := computeLeafKey(d.cs, fd, rp)
	if err != nil {
		return false, nil
	}
	if d.c.IsNull(k) {
		return true, nil // NULL = absence → leave the field zero
	}
	return d.bindLeaf(fd.DataType(), k, rv)
}

func (d *Document) bindNestedObject(rp definition.ResolvedPath, fd definition.FieldDescriptor, rv reflect.Value, tag string) (bool, error) {
	if rv.Kind() != reflect.Struct || !rv.CanAddr() {
		return false, nil
	}
	child := fd.ChildSchemaIdx()
	childView := d.newNestedView(child, rp)
	if err := bindStruct(childView, rv.Addr().Interface(), d.ctx, tag); err != nil {
		return true, err
	}
	return true, nil
}

func (d *Document) bindArrayObject(rp definition.ResolvedPath, fd definition.FieldDescriptor, rv reflect.Value, tag string) (bool, error) {
	if rv.Kind() != reflect.Slice {
		return false, nil
	}
	k := internalKey(fd)
	children, ok, err := d.c.GetArrayObject(k)
	if err != nil {
		return true, err
	}
	if !ok {
		return true, nil // absent → leave the field zero
	}
	child := fd.ChildSchemaIdx()
	out := reflect.MakeSlice(rv.Type(), len(children), len(children))
	for i, ch := range children {
		childView := d.newNestedViewForChild(ch, child, rp)
		ev := out.Index(i)
		var target any
		if ev.Kind() == reflect.Pointer {
			ev.Set(reflect.New(ev.Type().Elem()))
			target = ev.Interface()
		} else {
			if !ev.CanAddr() {
				return false, nil
			}
			target = ev.Addr().Interface()
		}
		if err := bindStruct(childView, target, d.ctx, tag); err != nil {
			return true, err
		}
	}
	rv.Set(out)
	return true, nil
}

func (d *Document) bindLeaf(dt container.DataType, k container.DataContainerKey, rv reflect.Value) (bool, error) {
	if rv.Type() == bindFieldTimeType {
		return false, nil // time fields use the generic CoerceTime path
	}
	switch dt {
	case container.TypeInt:
		n, ok, err := d.c.GetInt(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		return true, setIntLike(rv, n)
	case container.TypeFloat:
		f, ok, err := d.c.GetFloat(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		return true, setFloatLike(rv, f)
	case container.TypeString:
		s, ok, err := d.c.GetString(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		if rv.Kind() != reflect.String {
			return false, nil // e.g. time.Time from string → generic path
		}
		rv.SetString(s)
		return true, nil
	case container.TypeBool:
		b, ok, err := d.c.GetBool(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		if rv.Kind() != reflect.Bool {
			return false, nil
		}
		rv.SetBool(b)
		return true, nil
	case container.TypeBytes:
		b, ok, err := d.c.GetBytes(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		if rv.Kind() != reflect.Slice || rv.Type().Elem().Kind() != reflect.Uint8 {
			return false, nil
		}
		rv.SetBytes(b)
		return true, nil
	case container.TypeArrayInt:
		vals, ok, err := d.c.GetArrayInt(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		return true, setSlice(rv, len(vals), func(i int, ev reflect.Value) error {
			return setIntLike(ev, vals[i])
		})
	case container.TypeArrayFloat:
		vals, ok, err := d.c.GetArrayFloat(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		return true, setSlice(rv, len(vals), func(i int, ev reflect.Value) error {
			return setFloatLike(ev, vals[i])
		})
	case container.TypeArrayString:
		vals, ok, err := d.c.GetArrayString(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		return true, setSlice(rv, len(vals), func(i int, ev reflect.Value) error {
			if ev.Kind() != reflect.String {
				return bindFieldTypeConvErr(ev.Type())
			}
			ev.SetString(vals[i])
			return nil
		})
	case container.TypeArrayBool:
		vals, ok, err := d.c.GetArrayBool(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		return true, setSlice(rv, len(vals), func(i int, ev reflect.Value) error {
			if ev.Kind() != reflect.Bool {
				return bindFieldTypeConvErr(ev.Type())
			}
			ev.SetBool(vals[i])
			return nil
		})
	case container.TypeArrayBytes:
		vals, ok, err := d.c.GetArrayBytes(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		return true, setSlice(rv, len(vals), func(i int, ev reflect.Value) error {
			if ev.Kind() != reflect.Slice || ev.Type().Elem().Kind() != reflect.Uint8 {
				return bindFieldTypeConvErr(ev.Type())
			}
			ev.SetBytes(vals[i])
			return nil
		})
	default:
		return false, nil
	}
}

// setSlice allocates a slice of len n and fills it via at.
func setSlice(rv reflect.Value, n int, at func(i int, ev reflect.Value) error) error {
	if rv.Kind() != reflect.Slice {
		return bindFieldTypeConvErr(rv.Type())
	}
	out := reflect.MakeSlice(rv.Type(), n, n)
	for i := 0; i < n; i++ {
		if err := at(i, out.Index(i)); err != nil {
			return err
		}
	}
	rv.Set(out)
	return nil
}

func setIntLike(rv reflect.Value, n int64) error {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if rv.OverflowInt(n) {
			return bindFieldOverflowErr(n, rv.Type())
		}
		rv.SetInt(n)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n < 0 || rv.OverflowUint(uint64(n)) {
			return bindFieldOverflowErr(n, rv.Type())
		}
		rv.SetUint(uint64(n))
		return nil
	case reflect.Float32, reflect.Float64:
		rv.SetFloat(float64(n))
		return nil
	}
	return bindFieldTypeConvErr(rv.Type())
}

func setFloatLike(rv reflect.Value, f float64) error {
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		rv.SetFloat(f)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		rv.SetInt(int64(f))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		rv.SetUint(uint64(f))
		return nil
	}
	return bindFieldTypeConvErr(rv.Type())
}

func bindFieldTypeConvErr(t reflect.Type) error {
	return fmt.Errorf("document: cannot bind container value into %v", t)
}

func bindFieldOverflowErr(v int64, t reflect.Type) error {
	return fmt.Errorf("document: value %d overflows %v", v, t)
}
