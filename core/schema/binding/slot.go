package binding

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

var bindFieldTimeType = reflect.TypeFor[time.Time]()

// ContainerSource is a Source over a schema-bound container view: a compiled
// schema, a container, and the (prefix, slotIdx) address of the object view
// within it. It is the decoupled counterpart of a container-backed document
// view — no identity, no metadata, no record mode — and implements TypedBinder
// so structs bind box-free from the container's typed slots.
type ContainerSource struct {
	CS      *definition.CompiledSchema
	C       *container.DataContainer
	Prefix  definition.ResolvedPath
	SlotIdx uint8
	Ctx     context.Context
}

// resolve resolves a dotted path relative to this view.
func (s *ContainerSource) resolve(path string) (definition.ResolvedPath, definition.FieldDescriptor, error) {
	return ResolveViewPath(s.CS, s.Prefix, s.SlotIdx, path)
}

// Get returns the value at a schema field name as a boxed any, materializing
// nested objects as maps. It is the generic fallback behind Bind for field
// kinds the typed slot path cannot bind directly (time.Time, unknown, ...).
func (s *ContainerSource) Get(key string) (any, error) {
	if s == nil || s.CS == nil || s.C == nil {
		return nil, sourceKeyErr(key)
	}
	rp, fd, err := s.resolve(key)
	if err != nil {
		return nil, err
	}

	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		child := fd.ChildSchemaIdx()
		switch fd.DataType() {
		case container.TypeRecord:
			k := InternalKey(fd)
			if v, ok, err := s.C.GetRecord(k); err != nil {
				return nil, err
			} else if ok {
				return v, nil
			}
			return nil, sourceKeyErr(key)
		case container.TypeArrayObject:
			k := InternalKey(fd)
			if children, ok, err := s.C.GetArrayObject(k); err != nil {
				return nil, err
			} else if ok {
				arr := make([]any, len(children))
				for i, ch := range children {
					m, err := MaterializeSlot(s.CS, ch, child, rp)
					if err != nil {
						return nil, err
					}
					arr[i] = m
				}
				return arr, nil
			}
			return nil, sourceKeyErr(key)
		default:
			present, err := AnyDescendantPresent(s.CS, s.C, child, rp)
			if err != nil {
				return nil, err
			}
			if !present {
				return nil, sourceKeyErr(key)
			}
			return MaterializeSlot(s.CS, s.C, child, rp)
		}
	}

	k, err := ComputeLeafKey(s.CS, fd, rp)
	if err != nil {
		return nil, err
	}
	v, ok, err := GetByType(s.C, fd.DataType(), k)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, sourceKeyErr(key)
	}
	if s.C.IsNull(k) {
		return nil, nil
	}
	return v, nil
}

// BindField copies a schema field's value straight from the container into rv
// without boxing the value through an interface (any). It handles terminal
// scalars (int/float/string/bool/bytes), primitive arrays, nested objects, and
// array-of-object. Field kinds it cannot bind directly (records, geometry,
// time.Time, unknown, non-struct/slice targets) return (false, nil) so the
// binder falls back to the generic Source.Get path with its existing coercion
// and error semantics.
func (s *ContainerSource) BindField(name string, rv reflect.Value, tag string) (bool, error) {
	if s == nil || s.CS == nil || s.C == nil {
		return false, nil
	}
	if !rv.IsValid() || !rv.CanSet() {
		return false, nil
	}
	rp, fd, err := s.resolve(name)
	if err != nil {
		return false, nil
	}
	return s.bindSlot(rp, fd, rv, tag)
}

func (s *ContainerSource) bindSlot(rp definition.ResolvedPath, fd definition.FieldDescriptor, rv reflect.Value, tag string) (bool, error) {
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			// Allocate the pointer only when the slot actually holds a value.
			// Otherwise an absent field binds as a non-nil pointer to zero,
			// which partial-update paths treat as present and clobber stored
			// data with.
			ok, err := Present(s.CS, s.C, fd, rp)
			if err != nil {
				return false, err
			}
			if !ok {
				return true, nil // absence → leave the field nil
			}
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return s.bindSlot(rp, fd, rv.Elem(), tag)
	}
	if !rv.IsValid() || !rv.CanSet() {
		return false, nil
	}

	// Nested documents bind straight from a container-backed child view, so no
	// intermediate map[string]any is materialized.
	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		switch fd.DataType() {
		case container.TypeArrayObject:
			return s.bindArrayObject(rp, fd, rv, tag)
		case container.TypeRecord:
			return false, nil
		default:
			return s.bindNestedObject(rp, fd, rv, tag)
		}
	}

	// Terminal leaf: read the typed slot directly.
	k, err := ComputeLeafKey(s.CS, fd, rp)
	if err != nil {
		return false, nil
	}
	if s.C.IsNull(k) {
		return true, nil // NULL = absence → leave the field zero
	}
	return s.bindLeaf(fd.DataType(), k, rv)
}

func (s *ContainerSource) bindNestedObject(rp definition.ResolvedPath, fd definition.FieldDescriptor, rv reflect.Value, tag string) (bool, error) {
	if rv.Kind() != reflect.Struct || !rv.CanAddr() {
		return false, nil
	}
	child := fd.ChildSchemaIdx()
	childView := &ContainerSource{
		CS:      s.CS,
		C:       s.C,
		Ctx:     s.Ctx,
		Prefix:  append(definition.ResolvedPath(nil), rp...),
		SlotIdx: child,
	}
	if err := Bind(childView, rv.Addr().Interface(), s.Ctx, tag); err != nil {
		return true, err
	}
	return true, nil
}

func (s *ContainerSource) bindArrayObject(rp definition.ResolvedPath, fd definition.FieldDescriptor, rv reflect.Value, tag string) (bool, error) {
	if rv.Kind() != reflect.Slice {
		return false, nil
	}
	k := InternalKey(fd)
	children, ok, err := s.C.GetArrayObject(k)
	if err != nil {
		return true, err
	}
	if !ok {
		return true, nil // absent → leave the field zero
	}
	child := fd.ChildSchemaIdx()
	out := reflect.MakeSlice(rv.Type(), len(children), len(children))
	for i, ch := range children {
		childView := &ContainerSource{
			CS:      s.CS,
			C:       ch,
			Ctx:     s.Ctx,
			Prefix:  append(definition.ResolvedPath(nil), rp...),
			SlotIdx: child,
		}
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
		if err := Bind(childView, target, s.Ctx, tag); err != nil {
			return true, err
		}
	}
	rv.Set(out)
	return true, nil
}

func (s *ContainerSource) bindLeaf(dt container.DataType, k container.DataContainerKey, rv reflect.Value) (bool, error) {
	if rv.Type() == bindFieldTimeType {
		return false, nil // time fields use the generic CoerceTime path
	}
	switch dt {
	case container.TypeInt:
		n, ok, err := s.C.GetInt(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		return true, setIntLike(rv, n)
	case container.TypeFloat:
		f, ok, err := s.C.GetFloat(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		return true, setFloatLike(rv, f)
	case container.TypeString:
		str, ok, err := s.C.GetString(k)
		if err != nil {
			return true, err
		}
		if !ok {
			return true, nil
		}
		if rv.Kind() != reflect.String {
			return false, nil // e.g. time.Time from string → generic path
		}
		rv.SetString(str)
		return true, nil
	case container.TypeBool:
		b, ok, err := s.C.GetBool(k)
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
		b, ok, err := s.C.GetBytes(k)
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
		vals, ok, err := s.C.GetArrayInt(k)
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
		vals, ok, err := s.C.GetArrayFloat(k)
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
		vals, ok, err := s.C.GetArrayString(k)
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
		vals, ok, err := s.C.GetArrayBool(k)
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
		vals, ok, err := s.C.GetArrayBytes(k)
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
	return fmt.Errorf("binding: cannot bind container value into %v", t)
}

func bindFieldOverflowErr(v int64, t reflect.Type) error {
	return fmt.Errorf("binding: value %d overflows %v", v, t)
}
