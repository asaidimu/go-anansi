package document

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/common"
	creflect "github.com/asaidimu/go-anansi/v8/core/reflect"
	"github.com/asaidimu/go-anansi/v8/core/utils"
)

// ============================================================================
// STRUCT BINDING ENGINE (document-backed)
// ============================================================================
//
// This file implements the struct binder for container-backed documents. It
// replaces the machinery the package used to borrow from core/data
// (data.BindSourced + its structBinder), so the binding path no longer
// depends on the data package. Field/tag metadata is read through the
// core/reflect tag registry instead of reflect.StructTag.Get, and the
// per-(type, tag) field cache is rebuilt here. The deliberate duplication of
// data's parsing and coercion logic is temporary: it disappears when the data
// package is retired.
//
// Behavioural contract (preserved from the data implementation):
//
//   - Tag chain: [customTag, "anansi"]; the first tag whose value is
//     non-empty and not "-" names the field.
//   - Tag value grammar: "name, opt, ..." with omitempty as the only
//     recognized bare option and key=value options ignored (schema-only);
//     unknown bare options are build errors.
//   - Anonymous struct embeds are flattened recursively; a field whose
//     resolved name is _id_/_metadata_ is sourced from the document's
//     identity/metadata rather than its data.
//   - Fields bind through BindField (typed slot copy, no boxing) when the
//     source supports it, falling back to Get + coercion otherwise.
//   - Embedded system models get their parent reference restored after a
//     successful bind so promoted methods keep working.

const bindAnansiTag = "anansi"

const bindDocumentIDField = "_id_"
const bindMetadataField = "_metadata_"

// reservedBindField reports whether key is a system-managed field name.
func reservedBindField(key string) bool {
	return key == bindDocumentIDField || key == bindMetadataField
}

// ---------- Errors ----------

var (
	ErrBindInvalidTargetType    = common.NewSystemError("ERR_DOCUMENT_BIND_INVALID_TARGET_TYPE", "invalid target type")
	ErrBindFailedToSetField     = common.NewSystemError("ERR_DOCUMENT_BIND_FAILED_TO_SET_FIELD", "failed to set field")
	ErrBindTypeConversionFailed = common.NewSystemError("ERR_DOCUMENT_BIND_TYPE_CONVERSION_FAILED", "type conversion failed")
	ErrBindEmptyFieldName       = common.NewSystemError("ERR_DOCUMENT_BIND_EMPTY_FIELD_NAME", "doc tag has empty field name")
	ErrBindUnknownDocTagOption  = common.NewSystemError("ERR_DOCUMENT_BIND_UNKNOWN_DOC_TAG_OPTION", "unknown doc tag option")
	ErrBindKeyNotFound          = common.NewSystemError("ERR_DOCUMENT_BIND_KEY_NOT_FOUND", "key not found")
	ErrBindCannotTraverse       = common.NewSystemError("ERR_DOCUMENT_BIND_CANNOT_TRAVERSE", "cannot traverse into non-object value")
)

// ---------- Field Sources ----------

// fieldSource is the minimal per-field view the binder needs. *Document
// satisfies it directly (its data lives in typed container slots); mapSource
// adapts a plain map for nested-object binding.
type fieldSource interface {
	Get(key string) (any, error)
	ID() string
	Metadata() map[string]any
}

// typedFieldBinder is implemented by sources that can copy a field's value
// straight into a struct field without boxing it through any. Returning
// (false, nil) makes the caller fall back to the generic fieldSource.Get
// path, preserving coercion and error semantics for field kinds the source
// cannot bind directly.
type typedFieldBinder interface {
	BindField(name string, rv reflect.Value, tag string) (bool, error)
}

// mapSource adapts a nested map[string]any into a fieldSource, mirroring the
// semantics the data binder had for ad-hoc nested documents: no identity and
// an empty (non-nil) metadata map.
type mapSource struct {
	data map[string]any
}

func (m mapSource) Get(key string) (any, error) {
	v, ok := m.data[key]
	if !ok {
		return nil, ErrBindKeyNotFound
	}
	return v, nil
}

func (m mapSource) ID() string { return "" }

func (m mapSource) Metadata() map[string]any { return make(map[string]any) }

// ---------- Cached Field Metadata ----------

// bindFieldInfo is the binder's per-field record: the flattened index path
// into the target struct, the field's schema name (from the anansi tag), and
// the original StructField for error context. omitEmpty and isSystemEmbed
// are only consumed by the struct walker (bind_walk.go); the reader ignores
// them.
type bindFieldInfo struct {
	index         []int
	name          string
	structField   reflect.StructField
	omitEmpty     bool
	isSystemEmbed bool
}

type bindFieldsKey struct {
	t   reflect.Type
	tag string
}

type cachedBindFields struct {
	fields []bindFieldInfo
	err    error
}

// bindFieldsCache caches flattened field metadata per (type, tag). Like the
// data implementation it replaces, parse errors are cached too so a
// malformed struct fails fast on every subsequent bind.
var bindFieldsCache sync.Map // bindFieldsKey -> *cachedBindFields

func resolveBindTagChain(customTag string) []string {
	if customTag == "" {
		return []string{bindAnansiTag}
	}
	return []string{customTag, bindAnansiTag}
}

func bindFieldsFor(t reflect.Type, tag string) ([]bindFieldInfo, error) {
	key := bindFieldsKey{t: t, tag: tag}
	if v, ok := bindFieldsCache.Load(key); ok {
		info := v.(*cachedBindFields)
		return info.fields, info.err
	}

	fields, err := buildBindFields(t, nil, resolveBindTagChain(tag), false)
	info := &cachedBindFields{fields: fields, err: err}
	bindFieldsCache.Store(key, info)
	return fields, err
}

// buildBindFields walks t's fields, flattening anonymous struct embeds and
// resolving each field's schema name through the tag chain. Tag values are
// read via the core/reflect registry (cached per type, zero-copy) rather
// than reflect.StructTag.Get. isSystemEmbed marks the whole subtree as
// system-embedded (identity fields), propagated from enclosing embeds.
func buildBindFields(t reflect.Type, indexPrefix []int, tagChain []string, isSystemEmbed bool) ([]bindFieldInfo, error) {
	var fields []bindFieldInfo

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		indexPath := append(append([]int(nil), indexPrefix...), i)

		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			// System-model embeds (e.g. DocumentModel) mark their whole
			// subtree so the struct walker can skip identity fields for
			// partial writes.
			sysEmbed := isSystemEmbed || IsSystemModelType(f.Type)
			subFields, err := buildBindFields(f.Type, indexPath, tagChain, sysEmbed)
			if err != nil {
				return nil, err
			}
			fields = append(fields, subFields...)
			continue
		}

		// Unexported fields are excluded: they cannot be set through
		// reflection, and the core/reflect tag registry (the shared source of
		// tag metadata) excludes them as well.
		if f.PkgPath != "" {
			continue
		}

		var docTag string
		for _, tagName := range tagChain {
			tg, ok := creflect.FieldTagOf(t, f.Name, tagName)
			if !ok {
				continue
			}
			v, _ := tg.Value()
			if v != "" && v != "-" {
				docTag = v
				break
			}
		}
		if docTag == "" {
			continue
		}

		fieldName, omitEmpty, sysErr := parseBindTag(docTag)
		if sysErr != nil {
			return nil, sysErr.WithPath(f.Name)
		}

		fields = append(fields, bindFieldInfo{
			index:         indexPath,
			name:          fieldName,
			structField:   f,
			omitEmpty:     omitEmpty,
			isSystemEmbed: isSystemEmbed,
		})
	}

	return fields, nil
}

// parseBindTag parses the anansi tag value grammar used for binding:
// the first comma-delimited token is the field name, bare options are flags
// (only "omitempty" is meaningful to binding), and key=value options are
// schema-only and silently ignored.
func parseBindTag(tag string) (string, bool, *common.SystemError) {
	commaIdx := strings.IndexByte(tag, ',')
	if commaIdx == -1 {
		name := strings.TrimSpace(tag)
		if name == "" {
			return "", false, ErrBindEmptyFieldName
		}
		return name, false, nil
	}

	name := strings.TrimSpace(tag[:commaIdx])
	if name == "" {
		return "", false, ErrBindEmptyFieldName
	}

	var omitempty bool
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
			omitempty = true
		} else if strings.Contains(opt, "=") {
			// schema-only options silently ignored by binding layer
		} else if opt != "" {
			return "", false, ErrBindUnknownDocTagOption.WithMessagef("unknown option: %q", opt)
		}
	}

	return name, omitempty, nil
}

// ---------- System Model Registry ----------

// systemModelRegistration describes an embedded model type that carries the
// system document fields (_id_, _metadata_) and must be treated specially by
// binding: its parent reference is restored after each successful bind so
// promoted methods can reach the enclosing struct.
type systemModelRegistration struct {
	typ reflect.Type
	// linkParent restores the embedded model's parent-struct reference after
	// binding. It is provided by the registering package, keeping parent
	// manipulation internal to that package.
	linkParent func(embed any, parent any)
}

var (
	systemModelRegistrationsMu sync.RWMutex
	systemModelRegistrations   = []systemModelRegistration{
		{typ: reflect.TypeFor[DocumentModel](), linkParent: linkDocumentModelParent},
	}
)

// RegisterSystemModelType registers an embedded model type that carries
// system document fields (_id_, _metadata_) and must be treated like
// DocumentModel by the document binder. Register at package init, before any
// documents are built.
//
// This registry is consulted by the document package's own binder (the one
// reached through Documenter.BindTo and friends). It deliberately mirrors —
// but is separate from — the registry the data package keeps for its
// map-backed binder.
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

// ---------- The Binder ----------

// bindStruct binds struct fields from a fieldSource. Fields bind lazily, one
// at a time, via BindField (typed slot copy) or src.Get + coercion; a bind
// never materializes the whole source into a map.
func bindStruct(src fieldSource, target any, ctx context.Context, tag string) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return ErrBindInvalidTargetType.WithOperation("BindTo")
	}

	v := rv.Elem()
	fields, sysErr := bindFieldsFor(v.Type(), tag)
	if sysErr != nil {
		return sysErr
	}

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

		switch fInfo.name {
		case bindDocumentIDField:
			if id := src.ID(); id != "" {
				value = id
				found = true
			}
		case bindMetadataField:
			value = src.Metadata()
			found = true
		default:
			// Fields bind box-free straight from a container-backed source
			// when it supports it; other kinds fall back to Get + coercion.
			if tf, ok := src.(typedFieldBinder); ok {
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

// setBindField assigns value into field, coercing when necessary. It
// replicates the data binder's semantics: direct assignment when assignable,
// primitive coercion for scalars, time coercion, nested struct binding from
// maps, and recursive slice/map/pointer handling.
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
			if err := bindStruct(nestedSrc, newStruct, ctx, tag); err != nil {
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
