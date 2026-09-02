package data

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/utils"
)

// ============================================================================
// Zero-Reflection Interfaces (Codegen Hooks)
// ============================================================================

// DocumentUnmarshaler allows generated structs to bypass reflection completely.
type DocumentUnmarshaler interface {
	UnmarshalDocument(doc *Document) error
}

// FieldSource is the minimal per-field view the struct binder needs. Binding
// against this interface (rather than a materialized *Document) lets
// container-backed documents bind lazily from their typed slots, one field at
// a time, without materializing the whole document into a map.
type FieldSource interface {
	Get(key string) (any, error)
	ID() string
	Metadata() map[string]any
}

// TypedFieldBinder is implemented by container-backed FieldSource
// implementations (e.g. *document.Document) that can copy a field's value
// straight into a struct field without boxing it through an interface (any).
// Returning (false, nil) makes the caller fall back to the generic
// FieldSource.Get path, preserving coercion and error semantics for field
// kinds the source cannot bind directly.
type TypedFieldBinder interface {
	BindField(name string, rv reflect.Value, tag string) (bool, error)
}

// DocumentTagUnmarshaler allows tag-aware custom unmarshaling without reflection.
type DocumentTagUnmarshaler interface {
	UnmarshalDocumentTag(doc *Document, tag string) error
}

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

const AnansiTag = "anansi"

var (
	docModelType = reflect.TypeFor[DocumentModel]()
	timeType     = reflect.TypeFor[time.Time]()
	typeCache    sync.Map // map[typeCacheKey]*cachedTypeInfo
)

// Registered system-model embed types beyond data.DocumentModel. Packages that
// provide their own embedded model (e.g. document.DocumentModel) register it so
// struct binding, struct extraction, and DTO schema generation treat it exactly
// like data.DocumentModel.
type systemModelRegistration struct {
	typ reflect.Type
	// linkParent restores the embedded model's parent-struct reference after
	// binding so promoted methods (Document, Patch) work on materialized
	// results. It is provided by the registering package, keeping parent
	// manipulation internal to that package.
	linkParent func(embed any, parent any)
}

var (
	systemModelRegistrationsMu sync.RWMutex
	systemModelRegistrations   = []systemModelRegistration{
		{typ: docModelType, linkParent: linkDocumentModelParent},
	}
)

func linkDocumentModelParent(embed any, parent any) {
	if dm, ok := embed.(*DocumentModel); ok {
		dm.parent = parent
	}
}

// RegisterSystemModelType registers an embedded model type that carries system
// document fields (_id_, _metadata_) and must be treated like data.DocumentModel
// by struct binding, struct extraction, and DTO schema generation. Register at
// package init, before any documents are built.
//
// linkParent restores the embedded model's parent-struct reference after
// binding, enabling promoted methods (e.g. Document, Patch) on materialized read
// results. It lives in the registering package so parent manipulation stays
// internal; pass nil to skip parent linking.
func RegisterSystemModelType(t reflect.Type, linkParent func(embed any, parent any)) {
	systemModelRegistrationsMu.Lock()
	defer systemModelRegistrationsMu.Unlock()
	for _, existing := range systemModelRegistrations {
		if existing.typ == t {
			return
		}
	}
	systemModelRegistrations = append(systemModelRegistrations, systemModelRegistration{typ: t, linkParent: linkParent})
}

// IsSystemModelType reports whether t is a registered system-model embed.
func IsSystemModelType(t reflect.Type) bool {
	systemModelRegistrationsMu.RLock()
	defer systemModelRegistrationsMu.RUnlock()
	for _, reg := range systemModelRegistrations {
		if reg.typ == t {
			return true
		}
	}
	return false
}

// linkSystemModelParents restores the parent reference on every embedded
// registered system model within outer, so promoted methods can access the
// outer struct on read-back results.
func linkSystemModelParents(outer reflect.Value, parent any) {
	systemModelRegistrationsMu.RLock()
	defer systemModelRegistrationsMu.RUnlock()
	for _, reg := range systemModelRegistrations {
		if reg.linkParent == nil {
			continue
		}
		if idx, ok := findEmbeddedTypeIndex(outer.Type(), reg.typ); ok {
			fv := outer.FieldByIndex(idx)
			if fv.Kind() == reflect.Struct && fv.CanAddr() {
				reg.linkParent(fv.Addr().Interface(), parent)
			}
		}
	}
}

// findEmbeddedTypeIndex returns the field index path of the first anonymous
// embed of type target within t, or (nil, false).
func findEmbeddedTypeIndex(t reflect.Type, target reflect.Type) ([]int, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.Anonymous {
			continue
		}
		if f.Type == target {
			return []int{i}, true
		}
		if f.Type.Kind() == reflect.Struct {
			if sub, ok := findEmbeddedTypeIndex(f.Type, target); ok {
				return append([]int{i}, sub...), true
			}
		}
	}
	return nil, false
}

// ============================================================================
// Cached Type Metadata
// ============================================================================

type typeCacheKey struct {
	t   reflect.Type
	tag string
}

type cachedTypeInfo struct {
	fields []parsedField
	err    *common.SystemError
}

type fieldSetterFunc func(structPtr unsafe.Pointer, value any, ctx context.Context, tag string) error

type parsedField struct {
	Index         []int
	Name          string
	Options       tagOptions
	StructField   reflect.StructField
	IsSystemEmbed bool
	Offset        uintptr
	Setter        fieldSetterFunc
}

// ============================================================================
// Binding API - Primary Methods
// ============================================================================

// BindTo binds document fields into target using the default tag resolution chain.
func (d *Document) BindTo(target any) error {
	return d.BindToWithContext(context.Background(), target)
}

// BindToWithContext is BindTo with an explicit context for cancellation.
func (d *Document) BindToWithContext(ctx context.Context, target any) error {
	return BindSourced(d, func() (*Document, error) { return d, nil }, target, ctx, "")
}

// BindToTag binds document fields into target using a custom struct tag name.
func (d *Document) BindToTag(target any, tag string) error {
	return d.BindToTagWithContext(context.Background(), target, tag)
}

// BindToTagWithContext is BindToTagWithContext with an explicit context for cancellation.
func (d *Document) BindToTagWithContext(ctx context.Context, target any, tag string) error {
	return BindSourced(d, func() (*Document, error) { return d, nil }, target, ctx, tag)
}

// BindSourced binds struct fields from an arbitrary FieldSource. Generated
// fast-path unmarshalers still receive a materialized *Document (via
// materialize, called only for them); every other target binds lazily through
// src.Get/ID/Metadata, one field at a time, so container-backed documents
// materialize at most a single nested subtree instead of the whole document.
func BindSourced(src FieldSource, materialize func() (*Document, error), target any, ctx context.Context, tag string) error {
	if tag == "" {
		if _, ok := target.(DocumentUnmarshaler); ok {
			md, err := materialize()
			if err != nil {
				return err
			}
			return md.BindToWithContext(ctx, target)
		}
	} else {
		if _, ok := target.(DocumentTagUnmarshaler); ok {
			md, err := materialize()
			if err != nil {
				return err
			}
			return md.BindToTagWithContext(ctx, target, tag)
		}
	}
	binder := &structBinder{
		src: src,
		ctx: ctx,
		tag: tag,
	}
	return binder.bind(target)
}

// ============================================================================
// Document Creation from Structs
// ============================================================================

func NewDocumentFromStruct(s any, ctx ...context.Context) (*Document, error) {
	docData, err := structToMap(s, false, "")
	if err != nil {
		return nil, err
	}
	return getFactory().newDocument(extractContext(ctx), docData)
}

func NewDocumentFromStructWithTag(s any, tag string, ctx ...context.Context) (*Document, error) {
	docData, err := structToMap(s, false, tag)
	if err != nil {
		return nil, err
	}
	return getFactory().newDocument(extractContext(ctx), docData)
}

func NewPartialDocumentFromStruct(s any, ctx ...context.Context) (*Document, error) {
	docData, err := structToMap(s, true, "")
	if err != nil {
		return nil, err
	}
	return Patch(docData).Document(extractContext(ctx)), nil
}

func NewPartialDocumentFromStructWithTag(s any, tag string, ctx ...context.Context) (*Document, error) {
	docData, err := structToMap(s, true, tag)
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

func MustNewDocumentFromStructWithTag(s any, tag string, ctx ...context.Context) *Document {
	doc, err := NewDocumentFromStructWithTag(s, tag, ctx...)
	if err != nil {
		panic(fmt.Sprintf("MustNewDocumentFromStructWithTag failed with type %T: %v", s, err))
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

func resolveTagChain(customTag string) []string {
	if customTag == "" {
		return []string{AnansiTag}
	}
	return []string{customTag, AnansiTag}
}

func getTypeInfo(t reflect.Type, tag string) ([]parsedField, *common.SystemError) {
	key := typeCacheKey{t: t, tag: tag}
	if val, ok := typeCache.Load(key); ok {
		info := val.(*cachedTypeInfo)
		return info.fields, info.err
	}

	tagChain := resolveTagChain(tag)
	fields, sysErr := buildTypeFields(t, nil, 0, false, tagChain)
	info := &cachedTypeInfo{fields: fields, err: sysErr}
	typeCache.Store(key, info)
	return fields, sysErr
}

func buildTypeFields(t reflect.Type, indexPrefix []int, baseOffset uintptr, isSystemEmbed bool, tagChain []string) ([]parsedField, *common.SystemError) {
	var fields []parsedField

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		indexPath := append(append([]int(nil), indexPrefix...), i)
		fieldOffset := baseOffset + f.Offset

		sysEmbed := isSystemEmbed
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if IsSystemModelType(f.Type) || ReservedSystemField(f.Name) {
				sysEmbed = true
			}
			subFields, err := buildTypeFields(f.Type, indexPath, fieldOffset, sysEmbed, tagChain)
			if err != nil {
				return nil, err
			}
			fields = append(fields, subFields...)
			continue
		}

		var docTag string
		for _, tagName := range tagChain {
			v := f.Tag.Get(tagName)
			if v != "" && v != "-" {
				docTag = v
				break
			}
		}
		if docTag == "" {
			continue
		}

		fieldName, options, sysErr := parseDocTag(docTag)
		if sysErr != nil {
			return nil, sysErr.WithPath(f.Name)
		}

		setter := compileSetter(f.Type, fieldOffset)

		fields = append(fields, parsedField{
			Index:         indexPath,
			Name:          fieldName,
			Options:       options,
			StructField:   f,
			IsSystemEmbed: sysEmbed,
			Offset:        fieldOffset,
			Setter:        setter,
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

	fields, sysErr := getTypeInfo(v.Type(), "")
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
	src FieldSource
	ctx context.Context
	tag string
}

func (sb *structBinder) bind(target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return ErrInvalidTargetType.WithOperation("BindTo")
	}

	v := rv.Elem()
	fields, sysErr := getTypeInfo(v.Type(), sb.tag)
	if sysErr != nil {
		return sysErr
	}

	structPtr := rv.UnsafePointer()
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

		var value any
		var found bool

		switch fInfo.Name {
		case DocumentIDField:
			if id := sb.src.ID(); id != "" {
				value = id
				found = true
			}
		case MetadataField:
			value = sb.src.Metadata()
			found = true
		default:
			// Fields without a compiled setter bind box-free straight from a
			// container-backed source when it supports it. Setter fields keep
			// their unsafe-pointer fast path; system fields keep Get/ID/Metadata.
			if fInfo.Setter == nil {
				if tf, ok := sb.src.(TypedFieldBinder); ok {
					if fv := v.FieldByIndex(fInfo.Index); fv.CanSet() {
						if handled, err := tf.BindField(fInfo.Name, fv, sb.tag); handled {
							if err != nil {
								return ErrFailedToSetField.
									WithOperation("BindTo").
									WithPath(fInfo.Name).
									WithCause(err).
									WithMessagef("failed to set field %s: %v", fInfo.StructField.Name, err)
							}
							continue
						}
					}
				}
			}
			var er error
			value, er = sb.src.Get(fInfo.Name)
			found = (er == nil)
		}

		if !found || value == nil {
			continue
		}

		if fInfo.Setter != nil {
			if err := fInfo.Setter(structPtr, value, sb.ctx, sb.tag); err != nil {
				return ErrFailedToSetField.
					WithOperation("BindTo").
					WithPath(fInfo.Name).
					WithCause(err).
					WithMessagef("failed to set field %s: %v", fInfo.StructField.Name, err)
			}
		} else {
			fv := v.FieldByIndex(fInfo.Index)
			if !fv.CanSet() {
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
	}

	// Restore the parent reference on any embedded registered system model so
	// promoted methods (Document, Patch) can access the outer struct on read-back
	// results.
	if rv.Kind() == reflect.Pointer && rv.Elem().Kind() == reflect.Struct {
		linkSystemModelParents(rv.Elem(), target)
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
			nestedBinder := &structBinder{src: nestedDoc, ctx: sb.ctx, tag: sb.tag}
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
// Direct Pointer Setters Compilation
// ============================================================================

func compileSetter(fieldType reflect.Type, offset uintptr) fieldSetterFunc {
	switch fieldType.Kind() {
	case reflect.String:
		return func(ptr unsafe.Pointer, val any, ctx context.Context, tag string) error {
			targetPtr := (*string)(unsafe.Pointer(uintptr(ptr) + offset))
			if str, ok := val.(string); ok {
				*targetPtr = str
				return nil
			}
			if str, ok := utils.CoerceToPrimitiveValue[string](val); ok {
				*targetPtr = str
				return nil
			}
			return ErrTypeConversionFailed
		}

	case reflect.Int:
		return func(ptr unsafe.Pointer, val any, ctx context.Context, tag string) error {
			targetPtr := (*int)(unsafe.Pointer(uintptr(ptr) + offset))
			if num, ok := utils.CoerceToPrimitiveValue[int](val); ok {
				*targetPtr = num
				return nil
			}
			return ErrTypeConversionFailed
		}

	case reflect.Int64:
		return func(ptr unsafe.Pointer, val any, ctx context.Context, tag string) error {
			targetPtr := (*int64)(unsafe.Pointer(uintptr(ptr) + offset))
			if num, ok := utils.CoerceToPrimitiveValue[int64](val); ok {
				*targetPtr = num
				return nil
			}
			return ErrTypeConversionFailed
		}

	case reflect.Float64:
		return func(ptr unsafe.Pointer, val any, ctx context.Context, tag string) error {
			targetPtr := (*float64)(unsafe.Pointer(uintptr(ptr) + offset))
			if num, ok := utils.CoerceToPrimitiveValue[float64](val); ok {
				*targetPtr = num
				return nil
			}
			return ErrTypeConversionFailed
		}

	case reflect.Bool:
		return func(ptr unsafe.Pointer, val any, ctx context.Context, tag string) error {
			targetPtr := (*bool)(unsafe.Pointer(uintptr(ptr) + offset))
			if b, ok := utils.CoerceToPrimitiveValue[bool](val); ok {
				*targetPtr = b
				return nil
			}
			return ErrTypeConversionFailed
		}

	case reflect.Struct:
		if fieldType == timeType {
			return func(ptr unsafe.Pointer, val any, ctx context.Context, tag string) error {
				targetPtr := (*time.Time)(unsafe.Pointer(uintptr(ptr) + offset))
				if t, ok := utils.CoerceTime(val); ok {
					*targetPtr = t
					return nil
				}
				return ErrTypeConversionFailed
			}
		}
		return nil

	default:
		return nil
	}
}

// ============================================================================
// Struct to Map Conversion
// ============================================================================

// StructFieldValue pairs a tagged struct field's document path with its
// normalized value.
type StructFieldValue struct {
	Path  string
	Value any
}

// StructFieldValues walks s and returns each anansi-tagged field as a
// (path, value) pair without materializing a map. When partial is true,
// system-embedded fields (data.DocumentModel) and zero-valued fields are
// skipped. This is the field-walking core shared with NewDocumentFromStruct;
// callers can use it to populate other document implementations (e.g. a
// container-backed document.Document) field-by-field.
func StructFieldValues(s any, partial bool) ([]StructFieldValue, error) {
	return structFieldValues(s, partial, "")
}

// StructFieldValuesWithTag is StructFieldValues with a custom struct tag name.
func StructFieldValuesWithTag(s any, partial bool, tag string) ([]StructFieldValue, error) {
	return structFieldValues(s, partial, tag)
}

func structFieldValues(s any, partial bool, tag string) ([]StructFieldValue, error) {
	rv := reflect.ValueOf(s)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, nil
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return nil, ErrInvalidTargetType.WithMessagef("expected struct, got %T", s)
	}

	fields, sysErr := getTypeInfo(rv.Type(), tag)
	if sysErr != nil {
		return nil, sysErr
	}

	out := make([]StructFieldValue, 0, len(fields))
	for i := range fields {
		fInfo := &fields[i]
		if partial && fInfo.IsSystemEmbed {
			continue
		}

		fv := rv.FieldByIndex(fInfo.Index)
		if (partial && fv.IsZero()) || (!partial && fInfo.Options.OmitEmpty && fv.IsZero()) {
			continue
		}

		value, err := convertInterface(fv.Interface(), tag)
		if err != nil {
			return nil, err
		}
		out = append(out, StructFieldValue{Path: fInfo.Name, Value: value})
	}
	return out, nil
}

func structToMap(s any, partial bool, tag string) (map[string]any, error) {
	values, err := structFieldValues(s, partial, tag)
	if err != nil {
		return nil, err
	}

	docData := make(map[string]any, len(values))
	for _, fv := range values {
		if err := setNestedMap(docData, fv.Path, fv.Value); err != nil {
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

func convertInterface(v any, tag string) (any, error) {
	if v == nil {
		return nil, nil
	}

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
			elem, err := convertInterface(rv.MapIndex(key).Interface(), tag)
			if err != nil {
				return nil, err
			}
			ret[k] = elem
		}
		return ret, nil

	case reflect.Struct:
		return structToMap(v, false, tag)

	case reflect.Slice:
		ret := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			elem, err := convertInterface(rv.Index(i).Interface(), tag)
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
			// schema-only options silently ignored by binding layer
		} else if opt != "" {
			return "", tagOptions{}, ErrUnknownDocTagOption.WithMessagef("unknown option: %q", opt)
		}
	}

	return name, opts, nil
}
