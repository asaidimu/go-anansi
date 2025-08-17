package data

import "maps"

// Add these imports to the existing import block
import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// 1. GENERIC TYPE SUPPORT
// =============================================================================

// Get with generic type support
func Get[T any](doc Document, key string) (T, error) {
	var zero T
	val, err := doc.Get(key)
	if err != nil {
		return zero, err
	}

	result, ok := val.(T)
	if !ok {
		return zero, &DocumentError{
			Operation: "Get[T]",
			Key:       key,
			Message:   fmt.Sprintf("%s: cannot convert %T to %T", ErrTypeConversion.Error(), val, zero),
			Cause:     fmt.Errorf("%w: %w", ErrTypeConversion, ErrTypeMismatch),
		}
	}
	return result, nil
}

// GetWithCoercion attempts type coercion for common types
func GetWithCoercion[T any](doc Document, key string) (T, error) {
	var zero T
	val, err := doc.Get(key)
	if err != nil {
		return zero, err
	}

	// Try direct type assertion first
	if result, ok := val.(T); ok {
		return result, nil
	}

	// Try coercion for common types
	switch any(zero).(type) {
	case string:
		if str, ok := CoerceToString(val); ok {
			if result, ok := any(str).(T); ok {
				return result, nil
			}
		}
	case int:
		if num, ok := CoerceToInt(val); ok {
			if result, ok := any(num).(T); ok {
				return result, nil
			}
		}
	case float64:
		if num, ok := CoerceToFloat64(val); ok {
			if result, ok := any(num).(T); ok {
				return result, nil
			}
		}
	case bool:
		if b, ok := CoerceToBool(val); ok {
			if result, ok := any(b).(T); ok {
				return result, nil
			}
		}
	case time.Time:
		if t, ok := CoerceToTime(val); ok {
			if result, ok := any(t).(T); ok {
				return result, nil
			}
		}
	}

	return zero, &DocumentError{
		Operation: "GetWithCoercion[T]",
		Key:       key,
		Message:   fmt.Sprintf("%s: cannot convert %T to %T", ErrTypeConversion.Error(), val, zero),
		Cause:     fmt.Errorf("%w: %w", ErrTypeConversion, ErrTypeMismatch),
	}
}

// GetNested with generic type support
func GetNested[T any](doc Document, path string) (T, error) {
	var zero T
	val, err := doc.GetNested(path)
	if err != nil {
		return zero, err
	}

	result, ok := val.(T)
	if !ok {
		return zero, &DocumentError{
			Operation: "GetNested[T]",
			Key:       path,
			Message:   fmt.Sprintf("%s: cannot convert %T to %T", ErrTypeConversion.Error(), val, zero),
			Cause:     fmt.Errorf("%w: %w", ErrTypeConversion, ErrTypeMismatch),
		}
	}
	return result, nil
}

// =============================================================================
// 2. MUST VARIANTS (PANIC ON ERROR)
// =============================================================================

// MustHelper provides panic-based operations
type MustHelper struct {
	doc Document
}

// Must returns a helper for panic-based operations
func (d Document) Must() *MustHelper {
	return &MustHelper{doc: d}
}

func (m *MustHelper) Get(key string) any {
	val, err := m.doc.Get(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetString(key string) string {
	val, err := m.doc.GetString(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetInt(key string) int {
	val, err := m.doc.GetInt(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetFloat64(key string) float64 {
	val, err := m.doc.GetFloat64(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetBool(key string) bool {
	val, err := m.doc.GetBool(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetTime(key string) time.Time {
	val, err := m.doc.GetTime(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetDocument(key string) Document {
	val, err := m.doc.GetDocument(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetNested(path string) any {
	val, err := m.doc.GetNested(path)
	if err != nil {
		panic(err)
	}
	return val
}

// Generic Must getter
func MustGet[T any](doc Document, key string) T {
	val, err := Get[T](doc, key)
	if err != nil {
		panic(err)
	}
	return val
}

// =============================================================================
// 3. ENHANCED FLUENT QUERY INTERFACE
// =============================================================================

// Query creates a new fluent query interface
func Query(docs DocumentSet) *FluentQuery {
	return &FluentQuery{
		docs:    docs,
		filters: make([]func(Document) bool, 0),
	}
}

// FluentQuery provides a fluent interface for querying documents
type FluentQuery struct {
	docs    DocumentSet
	filters []func(Document) bool
	sorters []SortCriteria
	limit   int
	offset  int
}

type SortCriteria struct {
	Key  string
	Desc bool
}

// Where adds an equality filter
func (fq *FluentQuery) Where(key string, value any) *FluentQuery {
	fq.filters = append(fq.filters, func(d Document) bool {
		val, err := d.Get(key)
		if err != nil {
			return false
		}
		return reflect.DeepEqual(val, value)
	})
	return fq
}

// WhereFunc adds a custom filter function
func (fq *FluentQuery) WhereFunc(predicate func(Document) bool) *FluentQuery {
	fq.filters = append(fq.filters, predicate)
	return fq
}

// Comparison helpers for fluent queries
type FieldComparison struct {
	query *FluentQuery
	key   string
}

// Where returns a field comparison helper
func (fq *FluentQuery) WhereField(key string) *FieldComparison {
	return &FieldComparison{query: fq, key: key}
}

func (fc *FieldComparison) Equals(value any) *FluentQuery {
	return fc.query.Where(fc.key, value)
}

func (fc *FieldComparison) GreaterThan(value any) *FluentQuery {
	fc.query.filters = append(fc.query.filters, func(d Document) bool {
		val, err := d.Get(fc.key)
		if err != nil {
			return false
		}
		return compareValues(val, value) > 0
	})
	return fc.query
}

func (fc *FieldComparison) LessThan(value any) *FluentQuery {
	fc.query.filters = append(fc.query.filters, func(d Document) bool {
		val, err := d.Get(fc.key)
		if err != nil {
			return false
		}
		return compareValues(val, value) < 0
	})
	return fc.query
}

func (fc *FieldComparison) Contains(substr string) *FluentQuery {
	fc.query.filters = append(fc.query.filters, func(d Document) bool {
		val, err := d.GetString(fc.key)
		if err != nil {
			return false
		}
		return strings.Contains(strings.ToLower(val), strings.ToLower(substr))
	})
	return fc.query
}

func (fc *FieldComparison) In(values ...any) *FluentQuery {
	valueSet := make(map[any]bool)
	for _, v := range values {
		valueSet[v] = true
	}

	fc.query.filters = append(fc.query.filters, func(d Document) bool {
		val, err := d.Get(fc.key)
		if err != nil {
			return false
		}
		return valueSet[val]
	})
	return fc.query
}

// Sorting
func (fq *FluentQuery) SortBy(key string) *FluentQuery {
	fq.sorters = append(fq.sorters, SortCriteria{Key: key, Desc: false})
	return fq
}

func (fq *FluentQuery) SortByDesc(key string) *FluentQuery {
	fq.sorters = append(fq.sorters, SortCriteria{Key: key, Desc: true})
	return fq
}

// Pagination
func (fq *FluentQuery) Limit(n int) *FluentQuery {
	fq.limit = n
	return fq
}

func (fq *FluentQuery) Offset(n int) *FluentQuery {
	fq.offset = n
	return fq
}

func (fq *FluentQuery) Skip(n int) *FluentQuery {
	return fq.Offset(n)
}

// Aggregation helpers
func (fq *FluentQuery) Count() int {
	result := fq.Execute()
	return len(result)
}

func (fq *FluentQuery) First() (Document, bool) {
	result := fq.Limit(1).Execute()
	if len(result) == 0 {
		return nil, false
	}
	return result[0], true
}

func (fq *FluentQuery) Exists() bool {
	return fq.Limit(1).Count() > 0
}

// Execute applies all filters, sorts, and pagination
func (fq *FluentQuery) Execute() DocumentSet {
	result := make(DocumentSet, 0, len(fq.docs))

	// Apply filters
	for _, doc := range fq.docs {
		include := true
		for _, filter := range fq.filters {
			if !filter(doc) {
				include = false
				break
			}
		}
		if include {
			result = append(result, doc)
		}
	}

	// Apply sorting
	if len(fq.sorters) > 0 {
		sort.Slice(result, func(i, j int) bool {
			for _, criteria := range fq.sorters {
				val1, err1 := result[i].Get(criteria.Key)
				val2, err2 := result[j].Get(criteria.Key)

				if err1 != nil && err2 != nil {
					continue
				}
				if err1 != nil {
					return !criteria.Desc
				}
				if err2 != nil {
					return criteria.Desc
				}

				cmp := compareValues(val1, val2)
				if cmp != 0 {
					if criteria.Desc {
						return cmp > 0
					}
					return cmp < 0
				}
			}
			return false
		})
	}

	// Apply pagination
	if fq.offset > 0 && fq.offset < len(result) {
		result = result[fq.offset:]
	} else if fq.offset >= len(result) {
		return DocumentSet{}
	}

	if fq.limit > 0 && fq.limit < len(result) {
		result = result[:fq.limit]
	}

	return result
}

// =============================================================================
// 4. STRUCT TAG SUPPORT
// =============================================================================

// StructBinder handles automatic struct binding
type StructBinder struct {
	doc Document
}

// Bind returns a struct binder helper
func (d Document) Bind() *StructBinder {
	return &StructBinder{doc: d}
}

// To binds document data to a struct using 'doc' tags
func (sb *StructBinder) To(target any) error {
	return sb.ToWithContext(context.Background(), target)
}

// ToWithContext binds with context support
func (sb *StructBinder) ToWithContext(ctx context.Context, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return &DocumentError{
			Operation: "BindTo",
			Message:   ErrInvalidTargetType.Error(),
			Cause:     ErrInvalidTargetType,
		}
	}

	rv = rv.Elem()
	rt := rv.Type()

	for i := 0; i < rt.NumField(); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
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

		// Parse tag options
		tagParts := strings.Split(docTag, ",")
		fieldName := tagParts[0]
		options := tagParts[1:]

		// Check if field is optional
		omitEmpty := false
		for _, opt := range options {
			if opt == "omitempty" {
				omitEmpty = true
			}
		}

		// Get value from document
		value, err := sb.doc.Get(fieldName)
		if err != nil {
			if omitEmpty {
				continue
			}
			return &DocumentError{
				Operation: "BindTo",
				Key:       fieldName,
				Message:   fmt.Sprintf("%s for struct field %s", ErrRequiredFieldNotFound.Error(), field.Name),
				Cause:     fmt.Errorf("%w: %w", ErrRequiredFieldNotFound, err),
			}
		}

		// Set the field value
		if err := setFieldValue(fieldValue, value); err != nil {
			return &DocumentError{
				Operation: "BindTo",
				Key:       fieldName,
				Message:   fmt.Sprintf("%s for field %s", ErrFailedToSetField.Error(), field.Name),
				Cause:     fmt.Errorf("%w: %w", ErrFailedToSetField, err),
			}
		}
	}

	return nil
}

// BindTo is a generic version that returns the bound struct
func BindTo[T any](doc Document) (T, error) {
	var result T
	err := doc.Bind().To(&result)
	return result, err
}

// setFieldValue sets a reflect.Value (representing a struct field) from an arbitrary
// 'any' value. It attempts direct assignment if types are compatible, otherwise, it
// performs type coercion for common primitive types (string, int, float, bool, time.Time).
// It also recursively handles nested slices and maps. Returns an error if the value
// cannot be set or coerced to the field's type.
func setFieldValue(field reflect.Value, value any) error {
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
		if str, ok := CoerceToString(value); ok {
			field.SetString(str)
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if num, ok := CoerceToInt(value); ok {
			field.SetInt(int64(num))
			return nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if num, ok := CoerceToInt(value); ok {
			field.SetUint(uint64(num))
			return nil
		}
	case reflect.Float32, reflect.Float64:
		if num, ok := CoerceToFloat64(value); ok {
			field.SetFloat(num)
			return nil
		}
	case reflect.Bool:
		if b, ok := CoerceToBool(value); ok {
			field.SetBool(b)
			return nil
		}
	case reflect.Struct:
		if fieldType == reflect.TypeOf(time.Time{}) {
			if t, ok := CoerceToTime(value); ok {
				field.Set(reflect.ValueOf(t))
				return nil
			}
		} else { // Handle nested structs
			if valMap, ok := value.(map[string]any); ok {
				nestedDoc := Document(valMap)
				// Create a new instance of the nested struct type
				newStruct := reflect.New(fieldType).Interface()
				// Recursively bind the nested document to the new struct
				if err := nestedDoc.Bind().To(newStruct); err != nil {
					return err
				}
				field.Set(reflect.ValueOf(newStruct).Elem())
				return nil
			}
		}
	case reflect.Slice:
		if valueSlice, ok := value.([]any); ok {
			return setSliceField(field, valueSlice)
		}
	case reflect.Map:
		if valueMap, ok := value.(map[string]any); ok {
			return setMapField(field, valueMap)
		}
	case reflect.Ptr:
		if field.IsNil() {
			field.Set(reflect.New(fieldType.Elem()))
		}
		return setFieldValue(field.Elem(), value)
	}

	return fmt.Errorf("%w: cannot convert %T to %v", ErrTypeConversionFailed, value, fieldType)
}

// setSliceField handles assigning a slice of 'any' values to a reflect.Value
// representing a slice field in a struct. It iterates through the input values,
// recursively calls setFieldValue for each element to handle nested types and
// type coercion, and constructs a new slice of the correct element type.
func setSliceField(field reflect.Value, values []any) error {
	elementType := field.Type().Elem()
	slice := reflect.MakeSlice(field.Type(), len(values), len(values))

	for i, val := range values {
		elem := slice.Index(i)
		if elementType.Kind() == reflect.Ptr {
			elem.Set(reflect.New(elementType.Elem()))
			elem = elem.Elem()
		}
		if err := setFieldValue(elem, val); err != nil {
			return err
		}
	}

	field.Set(slice)
	return nil
}

// setMapField handles assigning a map[string]any to a reflect.Value representing
// a map field in a struct. It iterates through the input map, recursively calls
// setFieldValue for each value to handle nested types and type coercion, and
// constructs a new map of the correct key and element types.
func setMapField(field reflect.Value, values map[string]any) error {
	mapType := field.Type()
	newMap := reflect.MakeMap(mapType)

	for k, v := range values {
		keyVal := reflect.ValueOf(k)
		valueVal := reflect.New(mapType.Elem()).Elem()

		if err := setFieldValue(valueVal, v); err != nil {
			return err
		}

		newMap.SetMapIndex(keyVal, valueVal)
	}

	field.Set(newMap)
	return nil
}

// FromStruct creates a Document from a struct using 'doc' tags
func FromStructWithTags(s any) (Document, error) {
	rv := reflect.ValueOf(s)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return FromStruct(s) // Fallback to JSON marshaling
	}

	rt := rv.Type()
	doc := make(Document)

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fieldValue := rv.Field(i)

		docTag := field.Tag.Get("doc")
		if docTag == "" || docTag == "-" {
			continue
		}

		// Parse tag options
		tagParts := strings.Split(docTag, ",")
		fieldName := tagParts[0]
		options := tagParts[1:]

		// Check omitempty option
		omitEmpty := false
		for _, opt := range options {
			if opt == "omitempty" {
				omitEmpty = true
			}
		}

		// Skip zero values if omitempty
		if omitEmpty && fieldValue.IsZero() {
			continue
		}

		// Convert field value
		value := fieldValue.Interface()
		doc[fieldName] = value
	}

	return doc, nil
}

// =============================================================================
// 5. ENHANCED DOCUMENT BUILDER
// =============================================================================

func (db *DocumentBuilder) SetFromStruct(s any) *DocumentBuilder {
	if doc, err := FromStructWithTags(s); err == nil {
		maps.Copy(db.doc, doc)
	}
	return db
}

func (db *DocumentBuilder) Validate(validator func(Document) error) *DocumentBuilder {
	// Store validator for execution during Build()
	if err := validator(db.doc); err != nil {
		panic(&DocumentError{
			Operation: "Build",
			Message:   ErrSchemaViolation.Error(),
			Cause:     fmt.Errorf("%w: %w", ErrSchemaViolation, err),
		})
	}
	return db
}
