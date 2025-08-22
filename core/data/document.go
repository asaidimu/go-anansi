package data

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"sort"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// Document represents a flexible, schema-aware data structure with comprehensive utilities.
type Document map[string]any

// Constants
const (
	MetadataFieldName = "_metadata_"
	SchemaFieldName   = "_schema_"
)

// convertToDocumentMap converts various input types to map[string]any for Must* functions
func convertToDocumentMap(data any) (map[string]any, error) {
	if data == nil {
		return make(map[string]any), nil
	}

	switch v := data.(type) {
	case map[string]any:
		return v, nil
	case Document:
		return map[string]any(v), nil
	default:
		rv := reflect.ValueOf(data)
		if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
			doc := make(map[string]any, rv.Len())
			for _, key := range rv.MapKeys() {
				doc[key.String()] = rv.MapIndex(key).Interface()
			}
			return doc, nil
		}
		return nil, &DocumentError{
		Operation: "convertToDocumentMap",
		Message:   fmt.Sprintf("%s: %T", ErrInvalidTargetType.Error(), data),
		Cause:     ErrInvalidTargetType,
	}
	}
}

// NewDocument creates a new Document from a map[string]any.
func NewDocument(data map[string]any) (Document, error) {
	if data == nil {
		data = make(map[string]any)
	}
	return getFactory().newDocument(context.Background(), data)
}

// MustNewDocument creates a new Document from various map forms, panics on failure.
func MustNewDocument(data any) Document {
	doc, err := convertToDocumentMap(data)
	if err != nil {
		panic(err)
	}

	d, err := getFactory().newDocument(context.Background(), doc)
	if err != nil {
		panic(err)
	}

	return d
}

// Get retrieves a value with detailed error information (direct key lookup only).
func (d Document) Get(key string) (any, error) {
	val, ok := utils.GetValueByPath(d, key);
	if !ok {
		return nil, &DocumentError{
			Operation: "Get",
			Key:       key,
			Message:   "key not found",
			Cause:     ErrKeyNotFound,
		}
	}
	return val, nil
}

// GetOr retrieves a value or returns a default if not found.
func (d Document) GetOr(key string, defaultValue any) any {
	if val, ok := utils.GetValueByPath(d, key); ok {
		return val
	}
	return defaultValue
}

// MustGet retrieves a value, panics if not found.
func (d Document) MustGet(key string) any {
	val, ok := utils.GetValueByPath(d, key)
	if !ok {
		panic(&DocumentError{
			Operation: "MustGet",
			Key:       key,
			Message:   "key not found",
			Cause:     ErrKeyNotFound,
		})
	}
	return val
}

// Set with validation support.
func (d Document) Set(key string, value any) error {
	if key == "" {
		return &DocumentError{
			Operation: "Set",
			Key:       key,
			Message:   ErrKeyEmpty.Error(),
			Cause:     ErrKeyEmpty,
		}
	}
	d[key] = value
	return nil
}

// SetIfNotExists sets a value only if the key doesn't exist.
func (d Document) SetIfNotExists(key string, value any) bool {
	if _, exists := d[key]; !exists {
		d[key] = value
		return true
	}
	return false
}

// GetString with comprehensive type coercion and path support.
func (d Document) GetString(keyOrPath string) (string, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(""), "GetString")
	if err != nil {
		return "", err
	}
	return val.(string), nil
}

// GetInt with comprehensive numeric coercion and path support.
func (d Document) GetInt(keyOrPath string) (int, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(0), "GetInt")
	if err != nil {
		return 0, err
	}
	return val.(int), nil
}

// GetFloat64 with numeric coercion and path support.
func (d Document) GetFloat64(keyOrPath string) (float64, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(0.0), "GetFloat64")
	if err != nil {
		return 0.0, err
	}
	return val.(float64), nil
}

// GetBool with string parsing support and path support.
func (d Document) GetBool(keyOrPath string) (bool, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(false), "GetBool")
	if err != nil {
		return false, err
	}
	return val.(bool), nil
}

// GetTime parses time from various formats with path support.
func (d Document) GetTime(keyOrPath string) (time.Time, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(time.Time{}), "GetTime")
	if err != nil {
		return time.Time{}, err
	}
	return val.(time.Time), nil
}

// GetDocument retrieves a nested document with path support.
func (d Document) GetDocument(keyOrPath string) (Document, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(Document{}), "GetDocument")
	if err != nil {
		return nil, err
	}
	return val.(Document), nil
}

// GetDocumentArray retrieves an array of documents with path support.
func (d Document) GetDocumentArray(keyOrPath string) ([]Document, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf([]Document{}), "GetDocumentArray")
	if err != nil {
		return nil, err
	}
	return val.([]Document), nil
}

// getAndCoerce is a private helper function to retrieve a value by path and coerce it to a target type.
func (d Document) getAndCoerce(keyOrPath string, targetType reflect.Type, operation string) (any, error) {
	val, ok := utils.GetValueByPath(d, keyOrPath)
	if !ok {
		return nil, &DocumentError{
			Operation: operation,
			Key:       keyOrPath,
			Message:   "key not found",
			Cause:     ErrKeyNotFound,
		}
	}

	var coercedVal any
	var conversionOk bool

	switch targetType {
	case reflect.TypeOf(""):
		coercedVal, conversionOk = utils.CoerceToPrimitiveValue[string](val)
	case reflect.TypeOf(0):
		coercedVal, conversionOk = utils.CoerceToPrimitiveValue[int](val)
	case reflect.TypeOf(0.0):
		coercedVal, conversionOk = utils.CoerceToPrimitiveValue[float64](val)
	case reflect.TypeOf(false):
		coercedVal, conversionOk = utils.CoerceToPrimitiveValue[bool](val)
	case reflect.TypeOf(time.Time{}):
		coercedVal, conversionOk = utils.CoerceTime(val)
	case reflect.TypeOf(Document{}):
		coercedVal, conversionOk = AsDocument(val)
	case reflect.TypeOf([]Document{}):
		coercedVal, conversionOk = AsDocumentArray(val)
	default:
		return nil, &DocumentError{
			Operation: operation,
			Key:       keyOrPath,
			Message:   fmt.Sprintf("unsupported target type for coercion: %s", targetType.String()),
			Cause:     fmt.Errorf("%w: unsupported type", ErrTypeConversion),
		}
	}

	if !conversionOk {
		return nil, &DocumentError{
			Operation: operation,
			Key:       keyOrPath,
			Message:   fmt.Sprintf("%s: cannot convert %T to %s", ErrTypeConversion.Error(), val, targetType.String()),
			Cause:     fmt.Errorf("%w: %w", ErrTypeConversion, ErrTypeMismatch),
		}
	}

	return coercedVal, nil
}

// Keys returns all keys sorted alphabetically.
func (d Document) Keys() []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Values returns all values in key-sorted order.
func (d Document) Values() []any {
	keys := d.Keys()
	values := make([]any, len(keys))
	for i, k := range keys {
		values[i] = d[k]
	}
	return values
}

// Clone creates a deep copy with better nested handling.
func (d Document) Clone() Document {
	return d.deepClone().(Document)
}

func (d Document) deepClone() any {
	// deepClone recursively clones the Document and its nested structures (maps, slices, and other Documents).
	// It ensures that modifications to the cloned document do not affect the original, providing
	// a truly independent copy.
	result := make(Document)
	for k, v := range d {
		result[k] = deepCloneValue(v)
	}
	return result
}

// deepCloneValue recursively clones a value, handling nested Documents, maps (map[string]any),
// and slices ([]any, []Document). Primitive types are returned as is. This function is
// crucial for creating deep copies of document structures to prevent unintended side effects.
func deepCloneValue(v any) any {
	switch val := v.(type) {
	case Document:
		return val.deepClone()
	case map[string]any:
		result := make(map[string]any)
		for k, subV := range val {
			result[k] = deepCloneValue(subV)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = deepCloneValue(item)
		}
		return result
	case []Document:
		result := make([]Document, len(val))
		for i, doc := range val {
			result[i] = doc.Clone()
		}
		return result
	default:
		return v
	}
}

// Merge combines multiple documents with conflict resolution.
func (d Document) Merge(others ...Document) Document {
	result := d.Clone()
	for _, other := range others {
		maps.Copy(result, other)
	}
	return result
}

// DeepMerge performs recursive merging of nested objects.
func (d Document) DeepMerge(others ...Document) Document {
	result := d.Clone()
	for _, other := range others {
		result.deepMergeInto(other)
	}
	return result
}

// deepMergeInto recursively merges the content of 'other' Document into 'd'.
// Existing keys in 'd' are overwritten by 'other', except for nested Documents
// and map[string]any types, which are recursively merged. This provides a way
// to combine documents while preserving the structure of nested objects.
func (d Document) deepMergeInto(other Document) {
	for k, v := range other {
		if existing, ok := d[k]; ok {
			if existingDoc, ok := existing.(Document); ok {
				if otherDoc, ok := AsDocument(v); ok {
					existingDoc.deepMergeInto(otherDoc)
					continue
				}
			} else if existingMap, ok := existing.(map[string]any); ok {
				if otherMap, ok := v.(map[string]any); ok {
					Document(existingMap).deepMergeInto(Document(otherMap))
					continue
				}
			}
		}
		d[k] = deepCloneValue(v)
	}
}

// String provides a readable representation.
func (d Document) String() string {
	data, err := d.ToJSON(true)
	if err != nil {
		return fmt.Sprintf("Document{error: %v}", err)
	}
	return string(data)
}

// Len returns the number of key-value pairs.
func (d Document) Len() int {
	l := len(d)
	if _, ok := d[MetadataFieldName]; ok {
		l--
	}
	return l
}

// IsEmpty checks if the document is empty.
func (d Document) IsEmpty() bool {
	return len(d) == 0
}

// HasKey checks if a key exists (direct key only, not path).
func (d Document) HasKey(key string) bool {
	_, ok := d[key]
	return ok
}

// HasPath checks if a path exists (supports dot notation).
func (d Document) HasPath(keyOrPath string) bool {
	_, ok := utils.GetValueByPath(d, keyOrPath)
	return ok
}

// Equals performs deep equality comparison.
func (d Document) Equals(other Document) bool {
	if len(d) != len(other) {
		return false
	}

	for k, v := range d {
		otherV, ok := other[k]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(v, otherV) {
			return false
		}
	}

	return true
}

// AsMap returns a deep map[string]any representation of the document.
// Nested Documents and slices are recursively converted.
func (d Document) AsMap() map[string]any {
	out := make(map[string]any, len(d))
	for k, v := range d {
		out[k] = asMapValue(v)
	}
	return out
}

// asMapValue recursively converts a value to its map[string]any representation.
// It handles Document, map[string]any, and slices of these types, ensuring that
// the returned map is a standard Go map, not a Document type.
func asMapValue(v any) any {
	switch val := v.(type) {
	case Document:
		return val.AsMap()
	case map[string]any:
		// Convert arbitrary map[string]any (not typed as Document)
		nested := make(map[string]any, len(val))
		for nk, nv := range val {
			nested[nk] = asMapValue(nv)
		}
		return nested
	case []Document:
		arr := make([]any, len(val))
		for i, doc := range val {
			arr[i] = doc.AsMap()
		}
		return arr
	case []any:
		arr := make([]any, len(val))
		for i, item := range val {
			arr[i] = asMapValue(item)
		}
		return arr
	default:
		return val // primitive type (string, int, etc.)
	}
}

// Metadata access with enhanced functionality
func (d Document) Metadata() (map[string]any, bool) {
	val, ok := d[MetadataFieldName]
	if !ok {
		return nil, false
	}

	if meta, ok := val.(map[string]any); ok {
		return meta, true
	}

	return nil, false
}

func (d Document) SetMetadata(metadata map[string]any) {
	d[MetadataFieldName] = metadata
}

// StripMetadata removes metadata and returns a clean copy.
func (d Document) StripMetadata() Document {
	doc := stripNestedMetadata(d)
	result := doc.(Document)
	return result
}

// compareValues compares two values for sorting.
func compareValues(a, b any) int {
	// Handle nil values
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Try numeric comparison first
	if numA, okA := utils.CoerceToPrimitiveValue[float64](a); okA {
		if numB, okB := utils.CoerceToPrimitiveValue[float64](b); okB {
			if numA < numB {
				return -1
			}
			if numA > numB {
				return 1
			}
			return 0
		}
	}

	// String comparison
	strA, _ := utils.CoerceToPrimitiveValue[string](a)
	strB, _ := utils.CoerceToPrimitiveValue[string](b)

	if strA < strB {
		return -1
	}
	if strA > strB {
		return 1
	}
	return 0
}
