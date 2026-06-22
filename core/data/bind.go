package data

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/utils"
)

// ============================================================================
// Predefined Errors
// ============================================================================

var (
	ErrInvalidDocTag = common.NewSystemError("ERR_BIND_INVALID_DOC_TAG").
		WithMessage("invalid doc tag format")

	ErrUnknownDocTagOption = common.NewSystemError("ERR_BIND_UNKNOWN_DOC_TAG_OPTION").
		WithMessage("unknown doc tag option")

	ErrEmptyFieldName = common.NewSystemError("ERR_BIND_EMPTY_FIELD_NAME").
		WithMessage("doc tag has empty field name")
)

// ============================================================================
// Binding API - Primary Methods
// ============================================================================

func (d *Document) BindTo(target any) error {
	return d.BindToWithContext(context.Background(), target)
}

func (d *Document) BindToWithContext(ctx context.Context, target any) error {
	binder := &structBinder{
		doc: d,
		ctx: ctx,
	}
	return binder.bind(target)
}

// ============================================================================
// Document Creation from Structs
// ============================================================================

func NewDocumentFromStruct(s any, ctx ...context.Context) (*Document, error) {
	docData, err := structToMap(s, false)
	if err != nil {
		return nil, err
	}

	dctx := context.Background()
	if len(ctx) > 0 && ctx[0] != nil {
		dctx = ctx[0]
	}

	return getFactory().newDocument(dctx, docData)
}

func NewPartialDocumentFromStruct(s any, ctx ...context.Context) (*Document, error) {
	docData, err := structToMap(s, true)
	if err != nil {
		return nil, err
	}

	dctx := context.Background()
	if len(ctx) > 0 && ctx[0] != nil {
		dctx = ctx[0]
	}

	return Patch(docData).Document(dctx), nil
}

func MustNewDocumentFromStruct(s any, ctx ...context.Context) *Document {
	doc, err := NewDocumentFromStruct(s, ctx...)
	if err != nil {
		panic(fmt.Sprintf("MustNewDocumentFromStruct failed with type %T: %v", s, err))
	}
	return doc
}

// ============================================================================
// Internal Core Logic (Centralized Reflection)
// ============================================================================

type fieldMetadata struct {
	Name      string
	Options   tagOptions
	Value     reflect.Value
	StructField reflect.StructField
}

// walkFields now takes a 'partial' hint to decide whether to skip system embeddings
func walkFields(v reflect.Value, partial bool, fn func(meta fieldMetadata) error) error {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)

		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if partial && (f.Type == reflect.TypeOf(DocumentModel{}) || ReservedSystemField(f.Name)) {
				continue
			}
			if err := walkFields(fv, partial, fn); err != nil {
				return err
			}
			continue
		}

		docTag := f.Tag.Get("doc")
		if docTag == "" || docTag == "-" {
			continue
		}

		fieldName, options, err := parseDocTag(docTag)
		if err != nil {
			return err.WithPath(f.Name)
		}

		if err := fn(fieldMetadata{
			Name:        fieldName,
			Options:     options,
			Value:       fv,
			StructField: f,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ============================================================================
// Internal Binding Implementation
// ============================================================================

type structBinder struct {
	doc *Document
	ctx context.Context
}

func (sb *structBinder) bind(target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return ErrInvalidTargetType.WithOperation("BindTo")
	}

	return walkFields(rv, false, func(meta fieldMetadata) error {
		// Check for context cancellation
		select {
		case <-sb.ctx.Done():
			return common.SystemErrorFrom(sb.ctx.Err()).WithOperation("BindTo")
		default:
		}

		if !meta.Value.CanSet() {
			return nil
		}

		var value any
		var found bool

		switch meta.Name {
		case DocumentIDField:
			if sb.doc.id != "" {
				value = sb.doc.id
				found = true
			}
		case MetadataField:
			value = sb.doc.Metadata()
			found = true
		default:
			var er error
			value, er = sb.doc.Get(meta.Name)
			found = (er == nil)
		}

		if !found {
			if meta.Options.Has("omitempty") {
				return nil
			}
			return ErrRequiredFieldNotFound.
				WithOperation("BindTo").
				WithPath(meta.Name).
				WithMessagef("required field '%s' not found for struct field %s", meta.Name, meta.StructField.Name)
		}

		if err := sb.setFieldValue(meta.Value, value); err != nil {
			return ErrFailedToSetField.
				WithOperation("BindTo").
				WithPath(meta.Name).
				WithCause(err).
				WithMessagef("failed to set field %s: %v", meta.StructField.Name, err)
		}
		return nil
	})
}

func (sb *structBinder) setFieldValue(field reflect.Value, value any) error {
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
		if fieldType == reflect.TypeOf(time.Time{}) {
			if t, ok := utils.CoerceTime(value); ok {
				field.Set(reflect.ValueOf(t))
				return nil
			}
		} else if valMap, ok := value.(map[string]any); ok {
			nestedDoc := &Document{ctx: sb.ctx, data: valMap}
			newStruct := reflect.New(fieldType).Interface()
			nestedBinder := &structBinder{doc: nestedDoc, ctx: sb.ctx}
			if err := nestedBinder.bind(newStruct); err != nil {
				return err
			}
			field.Set(reflect.ValueOf(newStruct).Elem())
			return nil
		}
	case reflect.Slice:
		if valueSlice, ok := value.([]any); ok {
			return sb.setSliceField(field, valueSlice)
		}
	case reflect.Map:
		if valueMap, ok := value.(map[string]any); ok {
			return sb.setMapField(field, valueMap)
		}
	case reflect.Pointer:
		if field.IsNil() {
			field.Set(reflect.New(fieldType.Elem()))
		}
		return sb.setFieldValue(field.Elem(), value)
	}

	return ErrTypeConversionFailed.WithMessagef("cannot convert %T to %v", value, fieldType)
}

func (sb *structBinder) setSliceField(field reflect.Value, values []any) error {
	elementType := field.Type().Elem()
	slice := reflect.MakeSlice(field.Type(), len(values), len(values))
	for i, val := range values {
		elem := slice.Index(i)
		if elementType.Kind() == reflect.Pointer {
			elem.Set(reflect.New(elementType.Elem()))
			elem = elem.Elem()
		}
		if err := sb.setFieldValue(elem, val); err != nil {
			return err
		}
	}
	field.Set(slice)
	return nil
}

func (sb *structBinder) setMapField(field reflect.Value, values map[string]any) error {
	mapType := field.Type()
	newMap := reflect.MakeMap(mapType)
	for k, v := range values {
		valueVal := reflect.New(mapType.Elem()).Elem()
		if err := sb.setFieldValue(valueVal, v); err != nil {
			return err
		}
		newMap.SetMapIndex(reflect.ValueOf(k), valueVal)
	}
	field.Set(newMap)
	return nil
}

// ============================================================================
// Struct to Map Conversion
// ============================================================================

func structToMap(s any, partial bool) (map[string]any, error) {
	rv := reflect.ValueOf(s)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return make(map[string]any), nil
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return nil, ErrInvalidTargetType.WithMessagef("expected struct, got %T", s)
	}

	docData := make(map[string]any)
	err := walkFields(rv, partial, func(meta fieldMetadata) error {
		omitEmpty := meta.Options.Has("omitempty")
		if (partial && meta.Value.IsZero()) || (!partial && omitEmpty && meta.Value.IsZero()) {
			return nil
		}

		value, err := convertInterface(meta.Value.Interface())
		if err != nil {
			return err
		}
		docData[meta.Name] = value
		return nil
	})

	return docData, err
}

func convertInterface(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() { return nil, nil }
		rv = rv.Elem()
	}

	// 1. Handle standard non-primitive types
	v = rv.Interface()
	if _, ok := v.(time.Time); ok { return v, nil }

	switch rv.Kind() {
	// 2. Normalize Custom Types to Primitives
	// This fixes the Enum validation error
	case reflect.String:
		return rv.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint(), nil
	case reflect.Float32, reflect.Float64:
		return rv.Float(), nil
	case reflect.Bool:
		return rv.Bool(), nil

	// 3. Normalize Maps
	// This fixes the [OBJECT_TYPE_MISMATCH] error
	case reflect.Map:
		ret := make(map[string]any)
		for _, key := range rv.MapKeys() {
			// Convert the key to string (Go maps in our context use string keys)
			k := fmt.Sprintf("%v", key.Interface())
			// Recursively normalize the map value
			elem, err := convertInterface(rv.MapIndex(key).Interface())
			if err != nil { return nil, err }
			ret[k] = elem
		}
		return ret, nil

	case reflect.Struct:
		return structToMap(v, false)

	case reflect.Slice:
		ret := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			elem, err := convertInterface(rv.Index(i).Interface())
			if err != nil { return nil, err }
			ret[i] = elem
		}
		return ret, nil

	default:
		return v, nil
	}
}

// ============================================================================
// Tag Parsing & Helpers
// ============================================================================

type tagOptions map[string]bool
func (opts tagOptions) Has(option string) bool { return opts[option] }

func parseDocTag(tag string) (string, tagOptions, *common.SystemError) {
	parts := strings.Split(tag, ",")
	name := strings.TrimSpace(parts[0])
	if name == "" { return "", nil, ErrEmptyFieldName }

	opts := make(tagOptions)
	for _, opt := range parts[1:] {
		opt = strings.TrimSpace(opt)
		if opt == "omitempty" { opts[opt] = true } else if opt != "" {
			return "", nil, ErrUnknownDocTagOption.WithMessagef("unknown option: %q", opt)
		}
	}
	return name, opts, nil
}
