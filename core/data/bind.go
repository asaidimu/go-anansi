package data

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/utils"
)

// ============================================================================
// Predefined Errors & Constants
// ============================================================================

var (
	ErrInvalidDocTag = common.NewSystemError("ERR_BIND_INVALID_DOC_TAG").
				WithMessage("invalid doc tag format")

	ErrUnknownDocTagOption = common.NewSystemError("ERR_BIND_UNKNOWN_DOC_TAG_OPTION").
				WithMessage("unknown doc tag option")

	ErrEmptyFieldName = common.NewSystemError("ERR_BIND_EMPTY_FIELD_NAME").
				WithMessage("doc tag has empty field name")
)

const AnansiDocTag = "doc"
const AnansiTag = "anansi"

var (
	docModelType = reflect.TypeFor[DocumentModel]()
	timeType     = reflect.TypeFor[time.Time]()
	typeCache    sync.Map // map[reflect.Type]*cachedTypeInfo
)

type cachedTypeInfo struct {
	fields []parsedField
	err    *common.SystemError
}

type parsedField struct {
	Index         []int
	Name          string
	Options       tagOptions
	StructField   reflect.StructField
	IsSystemEmbed bool
}

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

	return getFactory().newDocument(extractContext(ctx), docData)
}

func NewPartialDocumentFromStruct(s any, ctx ...context.Context) (*Document, error) {
	docData, err := structToMap(s, true)
	if err != nil {
		return nil, err
	}

	return Patch(docData).Document(extractContext(ctx)), nil
}

func MustNewDocumentFromStruct(s any, ctx ...context.Context) *Document {
	doc, err := NewDocumentFromStruct(s, ctx...)
	if err != nil {
		panic(fmt.Sprintf("MustNewDocumentFromStruct failed with type %T: %v", s, err))
	}
	return doc
}

func extractContext(ctx []context.Context) context.Context {
	if len(ctx) > 0 && ctx[0] != nil {
		return ctx[0]
	}
	return context.Background()
}

// ============================================================================
// Internal Core Logic (Cached Reflection & Field Walking)
// ============================================================================

type fieldMetadata struct {
	Name        string
	Options     tagOptions
	Value       reflect.Value
	StructField reflect.StructField
}

func getTypeInfo(t reflect.Type) ([]parsedField, *common.SystemError) {
	if val, ok := typeCache.Load(t); ok {
		info := val.(*cachedTypeInfo)
		return info.fields, info.err
	}

	fields, sysErr := buildTypeFields(t, nil, false)
	info := &cachedTypeInfo{fields: fields, err: sysErr}
	typeCache.Store(t, info)
	return fields, sysErr
}

func buildTypeFields(t reflect.Type, indexPrefix []int, isSystemEmbed bool) ([]parsedField, *common.SystemError) {
	var fields []parsedField

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		indexPath := append(append([]int(nil), indexPrefix...), i)

		sysEmbed := isSystemEmbed
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if f.Type == docModelType || ReservedSystemField(f.Name) {
				sysEmbed = true
			}
			subFields, err := buildTypeFields(f.Type, indexPath, sysEmbed)
			if err != nil {
				return nil, err
			}
			fields = append(fields, subFields...)
			continue
		}

		docTag := f.Tag.Get(AnansiTag)
		if docTag == "" || docTag == "-" {
			docTag = f.Tag.Get(AnansiDocTag)
			if docTag == "" || docTag == "-" {
				continue
			}
		}

		fieldName, options, sysErr := parseDocTag(docTag)
		if sysErr != nil {
			return nil, sysErr.WithPath(f.Name)
		}

		fields = append(fields, parsedField{
			Index:         indexPath,
			Name:          fieldName,
			Options:       options,
			StructField:   f,
			IsSystemEmbed: sysEmbed,
		})
	}

	return fields, nil
}

func walkFields(v reflect.Value, partial bool, fn func(meta fieldMetadata) error) error {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	fields, sysErr := getTypeInfo(v.Type())
	if sysErr != nil {
		return sysErr
	}

	for i := range fields {
		fInfo := &fields[i]
		if partial && fInfo.IsSystemEmbed {
			continue
		}

		fv := v.FieldByIndex(fInfo.Index)
		if err := fn(fieldMetadata{
			Name:        fInfo.Name,
			Options:     fInfo.Options,
			Value:       fv,
			StructField: fInfo.StructField,
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

	v := rv.Elem()
	fields, sysErr := getTypeInfo(v.Type())
	if sysErr != nil {
		return sysErr
	}

	ctxDone := sb.ctx.Done()

	for i := range fields {
		fInfo := &fields[i]

		if ctxDone != nil {
			select {
			case <-ctxDone:
				return common.SystemErrorFrom(sb.ctx.Err()).WithOperation("BindTo")
			default:
			}
		}

		fv := v.FieldByIndex(fInfo.Index)
		if !fv.CanSet() {
			continue
		}

		var value any
		var found bool

		switch fInfo.Name {
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
			value, er = sb.doc.Get(fInfo.Name)
			found = (er == nil)
		}

		if !found {
			continue
		}

		if err := sb.setFieldValue(fv, value); err != nil {
			return ErrFailedToSetField.
				WithOperation("BindTo").
				WithPath(fInfo.Name).
				WithCause(err).
				WithMessagef("failed to set field %s: %v", fInfo.StructField.Name, err)
		}
	}

	// Set parent reference so promoted methods (Document(), Patch()) work
	if provider, ok := target.(DocumentModelProvider); ok {
		if dm := provider.Model(); dm != nil {
			dm.parent = target
		}
	}

	return nil
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
		if fieldType == timeType {
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
	newMap := reflect.MakeMapWithSize(mapType, len(values))
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

	fields, sysErr := getTypeInfo(rv.Type())
	if sysErr != nil {
		return nil, sysErr
	}

	docData := make(map[string]any, len(fields))
	for i := range fields {
		fInfo := &fields[i]
		if partial && fInfo.IsSystemEmbed {
			continue
		}

		fv := rv.FieldByIndex(fInfo.Index)
		if (partial && fv.IsZero()) || (!partial && fInfo.Options.OmitEmpty && fv.IsZero()) {
			continue
		}

		value, err := convertInterface(fv.Interface())
		if err != nil {
			return nil, err
		}
		if err := setNestedMap(docData, fInfo.Name, value); err != nil {
			return nil, err
		}
	}

	return docData, nil
}

func setNestedMap(data map[string]any, path string, value any) error {
	parts := strings.Split(path, ".")
	current := data
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return nil
		}
		next, ok := current[part]
		if !ok {
			next = make(map[string]any)
			current[part] = next
		}
		m, ok := next.(map[string]any)
		if !ok {
			return ErrCannotTraverse.WithPath(path)
		}
		current = m
	}
	return nil
}

func convertInterface(v any) (any, error) {
	if v == nil {
		return nil, nil
	}

	// Fast path for primitives and standard types without reflection overhead
	switch val := v.(type) {
	case string, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, bool, time.Time:
		return val, nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, nil
		}
		rv = rv.Elem()
		v = rv.Interface()
	}

	if _, ok := v.(time.Time); ok {
		return v, nil
	}

	switch rv.Kind() {
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

	case reflect.Map:
		ret := make(map[string]any, rv.Len())
		for _, key := range rv.MapKeys() {
			k := fmt.Sprintf("%v", key.Interface())
			elem, err := convertInterface(rv.MapIndex(key).Interface())
			if err != nil {
				return nil, err
			}
			ret[k] = elem
		}
		return ret, nil

	case reflect.Struct:
		return structToMap(v, false)

	case reflect.Slice:
		ret := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			elem, err := convertInterface(rv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
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

type tagOptions struct {
	OmitEmpty bool
}

func (opts tagOptions) Has(option string) bool {
	return option == "omitempty" && opts.OmitEmpty
}

func parseDocTag(tag string) (string, tagOptions, *common.SystemError) {
	commaIdx := strings.IndexByte(tag, ',')
	if commaIdx == -1 {
		name := strings.TrimSpace(tag)
		if name == "" {
			return "", tagOptions{}, ErrEmptyFieldName
		}
		return name, tagOptions{}, nil
	}

	name := strings.TrimSpace(tag[:commaIdx])
	if name == "" {
		return "", tagOptions{}, ErrEmptyFieldName
	}

	var opts tagOptions
	rest := tag[commaIdx+1:]
	for len(rest) > 0 {
		var opt string
		if idx := strings.IndexByte(rest, ','); idx != -1 {
			opt = strings.TrimSpace(rest[:idx])
			rest = rest[idx+1:]
		} else {
			opt = strings.TrimSpace(rest)
			rest = ""
		}

		if opt == "omitempty" {
			opts.OmitEmpty = true
		} else if strings.Contains(opt, "=") {
			// schema-only options (e.g. required=true, type=enum, values=...)
			// silently ignored by the binding layer
		} else if opt != "" {
			return "", tagOptions{}, ErrUnknownDocTagOption.WithMessagef("unknown option: %q", opt)
		}
	}

	return name, opts, nil
}
