package binding

import (
	"context"
	"reflect"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/utils"
)

// bindStruct binds struct fields from a Source. Fields bind lazily, one at a
// time, via BindField (typed slot copy) or src.Get + coercion; a bind never
// materializes the whole source into a map.
//
// System fields (_id_, _metadata_) bind from a systemSource's ID()/Metadata()
// when the source owns document identity; other sources resolve them as
// ordinary fields. After a successful bind, embedded registered system models
// get their parent reference restored so promoted methods keep working.
func Bind(src Source, target any, ctx context.Context, tag string) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return ErrBindInvalidTargetType.WithOperation("BindTo")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	v := rv.Elem()
	fields, sysErr := bindFieldsFor(v.Type(), tag)
	if sysErr != nil {
		return sysErr
	}

	sys, isSys := src.(systemSource)
	ctxDone := ctx.Done()

	for i := range fields {
		fInfo := &fields[i]

		if ctxDone != nil {
			select {
			case <-ctxDone:
				return common.SystemErrorFrom(ctx.Err()).WithOperation("BindTo")
			default:
			}
		}

		var value any
		var found bool

		switch {
		case isSys && fInfo.name == DocumentIDField:
			if id := sys.ID(); id != "" {
				value = id
				found = true
			}
		case isSys && fInfo.name == MetadataField:
			value = sys.Metadata()
			found = true
		default:
			// Fields bind box-free straight from a container-backed source
			// when it supports it; other kinds fall back to Get + coercion.
			if tf, ok := src.(TypedBinder); ok {
				if fv := v.FieldByIndex(fInfo.index); fv.CanSet() {
					if handled, err := tf.BindField(fInfo.name, fv, tag); handled {
						if err != nil {
							return ErrBindFailedToSetField.
								WithOperation("BindTo").
								WithPath(fInfo.name).
								WithCause(err).
								WithMessagef("failed to set field %s: %v", fInfo.structField.Name, err)
						}
						continue
					}
				}
			}
			var er error
			value, er = src.Get(fInfo.name)
			found = (er == nil)
		}

		if !found || value == nil {
			continue
		}

		fv := v.FieldByIndex(fInfo.index)
		if !fv.CanSet() {
			continue
		}
		if err := setBindField(fv, value, ctx, tag); err != nil {
			return ErrBindFailedToSetField.
				WithOperation("BindTo").
				WithPath(fInfo.name).
				WithCause(err).
				WithMessagef("failed to set field %s: %v", fInfo.structField.Name, err)
		}
	}

	// Restore the parent reference on any embedded registered system model so
	// promoted methods (Document, Patch) can access the outer struct on
	// read-back results.
	linkSystemModelParents(v, target)

	return nil
}

// ---------- Value Coercion ----------

var bindTimeType = reflect.TypeFor[time.Time]()

// setBindField assigns value into field, coercing when necessary. Direct
// assignment when assignable, primitive coercion for scalars, time coercion,
// nested struct binding from maps, and recursive slice/map/pointer handling.
func setBindField(field reflect.Value, value any, ctx context.Context, tag string) error {
	if value == nil {
		return nil
	}

	fieldType := field.Type()
	valueType := reflect.TypeOf(value)

	if valueType.AssignableTo(fieldType) {
		field.Set(reflect.ValueOf(value))
		return nil
	}

	switch fieldType.Kind() {
	case reflect.String:
		if str, ok := utils.CoerceToPrimitiveValue[string](value); ok {
			field.SetString(str)
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if num, ok := utils.CoerceToPrimitiveValue[int](value); ok {
			field.SetInt(int64(num))
			return nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if num, ok := utils.CoerceToPrimitiveValue[uint](value); ok {
			field.SetUint(uint64(num))
			return nil
		}
	case reflect.Float32, reflect.Float64:
		if num, ok := utils.CoerceToPrimitiveValue[float64](value); ok {
			field.SetFloat(num)
			return nil
		}
	case reflect.Bool:
		if b, ok := utils.CoerceToPrimitiveValue[bool](value); ok {
			field.SetBool(b)
			return nil
		}
	case reflect.Struct:
		if fieldType == bindTimeType {
			if t, ok := utils.CoerceTime(value); ok {
				field.Set(reflect.ValueOf(t))
				return nil
			}
		} else if valMap, ok := value.(map[string]any); ok {
			newStruct := reflect.New(fieldType).Interface()
			nestedSrc := mapSource{data: valMap}
			if err := Bind(nestedSrc, newStruct, ctx, tag); err != nil {
				return err
			}
			field.Set(reflect.ValueOf(newStruct).Elem())
			return nil
		}
	case reflect.Slice:
		if valueSlice, ok := value.([]any); ok {
			return setBindSliceField(field, valueSlice, ctx, tag)
		}
	case reflect.Map:
		if valueMap, ok := value.(map[string]any); ok {
			return setBindMapField(field, valueMap, ctx, tag)
		}
	case reflect.Pointer:
		if field.IsNil() {
			field.Set(reflect.New(fieldType.Elem()))
		}
		return setBindField(field.Elem(), value, ctx, tag)
	}

	return ErrBindTypeConversionFailed.WithMessagef("cannot convert %T to %v", value, fieldType)
}

func setBindSliceField(field reflect.Value, values []any, ctx context.Context, tag string) error {
	elementType := field.Type().Elem()
	slice := reflect.MakeSlice(field.Type(), len(values), len(values))
	for i, val := range values {
		elem := slice.Index(i)
		if elementType.Kind() == reflect.Pointer {
			elem.Set(reflect.New(elementType.Elem()))
			elem = elem.Elem()
		}
		if err := setBindField(elem, val, ctx, tag); err != nil {
			return err
		}
	}
	field.Set(slice)
	return nil
}

func setBindMapField(field reflect.Value, values map[string]any, ctx context.Context, tag string) error {
	mapType := field.Type()
	newMap := reflect.MakeMapWithSize(mapType, len(values))
	for k, v := range values {
		valueVal := reflect.New(mapType.Elem()).Elem()
		if err := setBindField(valueVal, v, ctx, tag); err != nil {
			return err
		}
		newMap.SetMapIndex(reflect.ValueOf(k), valueVal)
	}
	field.Set(newMap)
	return nil
}
