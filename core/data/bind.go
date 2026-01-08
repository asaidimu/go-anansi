package data

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// ============================================================================
// Predefined Errors
// ============================================================================

var (
	// ErrInvalidDocTag indicates an invalid doc tag format
	ErrInvalidDocTag = common.NewSystemError("ERR_BIND_INVALID_DOC_TAG").
		WithMessage("invalid doc tag format")

	// ErrUnknownDocTagOption indicates an unknown doc tag option
	ErrUnknownDocTagOption = common.NewSystemError("ERR_BIND_UNKNOWN_DOC_TAG_OPTION").
		WithMessage("unknown doc tag option")

	// ErrEmptyFieldName indicates doc tag has empty field name
	ErrEmptyFieldName = common.NewSystemError("ERR_BIND_EMPTY_FIELD_NAME").
		WithMessage("doc tag has empty field name")
)

// ============================================================================
// Binding API - Primary Methods
// ============================================================================

// BindTo binds document data to a struct using 'doc' tags.
// This is the primary ergonomic API for binding.
//
// Usage:
//   var user User
//   err := doc.BindTo(&user)
func (d *Document) BindTo(target any) error {
	return d.BindToWithContext(context.Background(), target)
}

// BindToWithContext binds with context support for cancellation and timeouts.
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

// NewDocumentFromStruct creates a Document from a struct using 'doc' tags.
// It recursively converts nested structs and slices.
// Zero-value fields with omitempty are skipped.
//
// Usage:
//   doc, err := data.NewDocumentFromStruct(user)
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

// NewPartialDocumentFromStruct creates a partial Document from a struct.
// All zero-value fields are skipped, regardless of omitempty tag.
// Useful for updates where you only want to include non-zero fields.
//
// Usage:
//   doc, err := data.NewPartialDocumentFromStruct(user)
func NewPartialDocumentFromStruct(s any, ctx ...context.Context) (*Document, error) {
	docData, err := structToMap(s, true)
	if err != nil {
		return nil, err
	}

	dctx := context.Background()
	if len(ctx) > 0 && ctx[0] != nil {
		dctx = ctx[0]
	}

	return getFactory().newDocument(dctx, docData)
}

// MustNewDocumentFromStruct is like NewDocumentFromStruct but panics on error.
func MustNewDocumentFromStruct(s any, ctx ...context.Context) *Document {
	doc, err := NewDocumentFromStruct(s, ctx...)
	if err != nil {
		panic(fmt.Sprintf("MustNewDocumentFromStruct failed with type %T: %v", s, err))
	}
	return doc
}

// MustNewPartialDocumentFromStruct is like NewPartialDocumentFromStruct but panics on error.
func MustNewPartialDocumentFromStruct(s any, ctx ...context.Context) *Document {
	doc, err := NewPartialDocumentFromStruct(s, ctx...)
	if err != nil {
		panic(fmt.Sprintf("MustNewPartialDocumentFromStruct failed with type %T: %v", s, err))
	}
	return doc
}

// ============================================================================
// Internal Binding Implementation
// ============================================================================

// structBinder handles the actual binding logic with context propagation
type structBinder struct {
	doc *Document
	ctx context.Context
}

func (sb *structBinder) bind(target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return ErrInvalidTargetType.WithOperation("BindTo")
	}

	rv = rv.Elem()
	rt := rv.Type()

	for i := 0; i < rt.NumField(); i++ {
		// Check for context cancellation
		select {
		case <-sb.ctx.Done():
			return common.SystemErrorFrom(sb.ctx.Err()).WithOperation("BindTo")
		default:
		}

		field := rt.Field(i)
		fieldValue := rv.Field(i)

		if !fieldValue.CanSet() {
			continue
		}

		docTag := field.Tag.Get("doc")
		if docTag == "" || docTag == "-" {
			continue
		}

		// Parse and validate tag
		fieldName, options, err := parseDocTag(docTag)
		if err != nil {
			return err.
				WithOperation("BindTo").
				WithPath(field.Name)
		}

		// Check if field is optional
		omitEmpty := options.Has("omitempty")

		// Get value from document
		value, er := sb.doc.Get(fieldName)
		if er != nil {
			if omitEmpty {
				continue
			}

			// Provide helpful error with available fields
			availableFields := sb.doc.Keys()
			suggestion := findClosestMatch(fieldName, availableFields)

			errMsg := fmt.Sprintf("required field '%s' not found for struct field %s", fieldName, field.Name)
			if suggestion != "" {
				errMsg += fmt.Sprintf(". Available fields: %v. Did you mean '%s'?", availableFields, suggestion)
			} else {
				errMsg += fmt.Sprintf(". Available fields: %v", availableFields)
			}

			return ErrRequiredFieldNotFound.
				WithOperation("BindTo").
				WithPath(fieldName).
				WithMessage(errMsg)
		}

		// Set the field value with context propagation
		if err := sb.setFieldValue(fieldValue, value); err != nil {
			return ErrFailedToSetField.
				WithOperation("BindTo").
				WithPath(fieldName).
				WithCause(err).
				WithMessagef("failed to set field %s: %v", field.Name, err)
		}
	}

	return nil
}

// setFieldValue sets a reflect.Value with context propagation for nested structs
func (sb *structBinder) setFieldValue(field reflect.Value, value any) error {
	if value == nil {
		return nil
	}

	fieldType := field.Type()
	valueType := reflect.TypeOf(value)

	// Direct assignment if types match
	if valueType.AssignableTo(fieldType) {
		field.Set(reflect.ValueOf(value))
		return nil
	}

	// Type coercion for common cases
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
		} else {
			// Handle nested structs with context propagation
			if valMap, ok := value.(map[string]any); ok {
				nestedDoc := &Document{ctx: sb.ctx, data: valMap}
				newStruct := reflect.New(fieldType).Interface()

				// Use the same binder to propagate context
				nestedBinder := &structBinder{
					doc: nestedDoc,
					ctx: sb.ctx,
				}
				if err := nestedBinder.bind(newStruct); err != nil {
					return err
				}
				field.Set(reflect.ValueOf(newStruct).Elem())
				return nil
			}
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
			// Only create pointer if value is not nil
			field.Set(reflect.New(fieldType.Elem()))
		}
		return sb.setFieldValue(field.Elem(), value)
	}

	return ErrTypeConversionFailed.
		WithOperation("setFieldValue").
		WithMessagef("cannot convert %T to %v", value, fieldType)
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
		keyVal := reflect.ValueOf(k)
		valueVal := reflect.New(mapType.Elem()).Elem()

		if err := sb.setFieldValue(valueVal, v); err != nil {
			return err
		}

		newMap.SetMapIndex(keyVal, valueVal)
	}

	field.Set(newMap)
	return nil
}

// ============================================================================
// Struct to Map Conversion
// ============================================================================

// structToMap converts a struct to map[string]any using doc tags
func structToMap(s any, partial bool) (map[string]any, error) {
	rv := reflect.ValueOf(s)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return make(map[string]any), nil
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return nil, ErrInvalidTargetType.
			WithOperation("structToMap").
			WithMessagef("expected struct, got %T", s)
	}

	// Handle time.Time as special case
	if _, ok := rv.Interface().(time.Time); ok {
		return nil, ErrTypeConversionFailed.
			WithOperation("structToMap").
			WithMessage("cannot convert time.Time to Document directly")
	}

	rt := rv.Type()
	docData := make(map[string]any)

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fieldValue := rv.Field(i)

		docTag := field.Tag.Get("doc")
		if docTag == "" || docTag == "-" {
			continue
		}

		fieldName, options, err := parseDocTag(docTag)
		if err != nil {
			return nil, err.WithPath(field.Name)
		}

		omitEmpty := options.Has("omitempty")

		// Skip logic:
		// - In partial mode: skip all zero-value fields
		// - In full mode: only skip zero-value fields with omitempty
		if (partial && fieldValue.IsZero()) || (!partial && omitEmpty && fieldValue.IsZero()) {
			continue
		}

		value, er := convertInterface(fieldValue.Interface())
		if er != nil {
			return nil, err.WithPath(field.Name)
		}
		docData[fieldName] = value
	}

	return docData, nil
}

// convertInterface recursively converts values to their generic representation
func convertInterface(v any) (any, error) {
	if v == nil {
		return nil, nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, nil
		}
		rv = rv.Elem()
	}

	v = rv.Interface()

	// time.Time is a struct but should be treated as primitive
	if _, ok := v.(time.Time); ok {
		return v, nil
	}

	switch rv.Kind() {
	case reflect.Struct:
		// Recursively convert nested structs
		nestedMap, err := structToMap(v, false)
		if err != nil {
			return nil, err
		}
		return nestedMap, nil

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
// Tag Parsing with Validation
// ============================================================================

type tagOptions map[string]bool

func (opts tagOptions) Has(option string) bool {
	return opts[option]
}

// parseDocTag parses and validates doc tag, returning field name and options
func parseDocTag(tag string) (fieldName string, options tagOptions, err *common.SystemError) {
	if tag == "" || tag == "-" {
		return "", nil, nil
	}

	parts := strings.Split(tag, ",")
	fieldName = strings.TrimSpace(parts[0])

	if fieldName == "" {
		return "", nil, ErrEmptyFieldName
	}

	options = make(tagOptions)
	for _, opt := range parts[1:] {
		opt = strings.TrimSpace(opt)
		if opt == "" {
			continue
		}

		switch opt {
		case "omitempty":
			options[opt] = true
		default:
			return "", nil, ErrUnknownDocTagOption.
				WithMessagef("unknown doc tag option: %q. Valid options: omitempty", opt)
		}
	}

	return fieldName, options, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// findClosestMatch finds the closest string match using simple edit distance
func findClosestMatch(target string, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}

	minDistance := len(target)
	closest := ""

	for _, candidate := range candidates {
		distance := levenshteinDistance(target, candidate)
		if distance < minDistance {
			minDistance = distance
			closest = candidate
		}
	}

	// Only suggest if distance is reasonable
	if minDistance <= len(target)/2 {
		return closest
	}

	return ""
}

// levenshteinDistance calculates edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}

			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
