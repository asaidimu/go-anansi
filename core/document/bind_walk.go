package document

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// ============================================================================
// STRUCT WALKER (struct -> (path, value) pairs)
// ============================================================================
//
// walkStructFields is the write-path counterpart to the bindStruct read path:
// it walks an anansi-tagged struct once and returns each field as a
// (document path, normalized value) pair. DocumentPool.FromStruct and
// FromPartialStruct consume the pairs in populateFromStruct, writing them
// straight into the container's typed slots with no map intermediate.
//
// It replaces data.StructFieldValues, which the write path used to borrow:
// layout metadata comes from the same bindFieldsFor cache the reader uses
// (tag values read through the core/reflect registry), and value
// normalization mirrors data's struct-to-value conversion. Behavioural parity
// with data.StructFieldValues is pinned by TestWalkStructFieldsParity.
//
// Rules (mirroring the data implementation):
//   - Default tag chain [anansi]; the first non-empty, non-"-" value names
//     the field.
//   - Anonymous struct embeds are flattened; unexported fields are skipped.
//   - partial=true: system-embedded fields and zero-valued fields are skipped.
//   - partial=false: zero-valued fields carrying `omitempty` are skipped.
//   - Nested structs normalize to map[string]any (recursively, non-partial);
//     dotted tag paths nest inside those maps.

// structFieldValue pairs a tagged struct field's document path with its
// normalized value.
type structFieldValue struct {
	path  string
	value any
}

// walkStructFields walks s and returns each anansi-tagged field as a
// (path, value) pair without materializing a map.
func walkStructFields(s any, partial bool) ([]structFieldValue, error) {
	rv := reflect.ValueOf(s)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, nil
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return nil, ErrBindInvalidTargetType.WithMessagef("expected struct, got %T", s)
	}

	fields, err := bindFieldsFor(rv.Type(), "")
	if err != nil {
		return nil, err
	}

	out := make([]structFieldValue, 0, len(fields))
	for i := range fields {
		fInfo := &fields[i]
		if partial && fInfo.isSystemEmbed {
			continue
		}

		fv := rv.FieldByIndex(fInfo.index)
		if !fv.CanInterface() {
			// Reachable only through unexported embeds; the data walker
			// would panic on Interface() here — skipping is strictly safer.
			continue
		}
		if (partial && fv.IsZero()) || (!partial && fInfo.omitEmpty && fv.IsZero()) {
			continue
		}

		value, err := convertWalkValue(fv.Interface())
		if err != nil {
			return nil, err
		}
		out = append(out, structFieldValue{path: fInfo.name, value: value})
	}
	return out, nil
}

// convertWalkValue normalizes a struct field value into its document form:
// scalars and times pass through, pointers unwrap, nested structs become
// maps, and slices/maps convert element-wise.
func convertWalkValue(v any) (any, error) {
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
			elem, err := convertWalkValue(rv.MapIndex(key).Interface())
			if err != nil {
				return nil, err
			}
			ret[k] = elem
		}
		return ret, nil

	case reflect.Struct:
		return walkStructToMap(v)

	case reflect.Slice:
		ret := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			elem, err := convertWalkValue(rv.Index(i).Interface())
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

// walkStructToMap materializes an anansi-tagged struct as a map, splitting
// dotted tag paths into nested maps. Used for nested struct values, which
// always walk non-partial (system fields preserved for the caller to route).
func walkStructToMap(s any) (map[string]any, error) {
	values, err := walkStructFields(s, false)
	if err != nil {
		return nil, err
	}

	docData := make(map[string]any, len(values))
	for _, fv := range values {
		if err := setWalkNestedMap(docData, fv.path, fv.value); err != nil {
			return nil, err
		}
	}
	return docData, nil
}

func setWalkNestedMap(data map[string]any, path string, value any) error {
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
			return ErrBindCannotTraverse.WithPath(path)
		}
		current = m
	}
	return nil
}
