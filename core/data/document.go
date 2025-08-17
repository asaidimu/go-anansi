package data

import (
	"maps"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Document represents a flexible, schema-aware data structure with comprehensive utilities.
type Document map[string]any

// DocumentError represents errors specific to document operations.
type DocumentError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *DocumentError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s operation failed for key '%s': %s (caused by: %v)",
			e.Operation, e.Key, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s operation failed for key '%s': %s", e.Operation, e.Key, e.Message)
}

// QueryBuilder provides a fluent interface for document queries.
type QueryBuilder struct {
	doc     Document
	filters []func(Document) bool
}

// DocumentTransform provides a fluent interface for document transformations.
type DocumentTransform struct {
	doc Document
	ops []func(Document) Document
}

// Constants
const (
	MetadataFieldName = "_metadata_"
	SchemaFieldName   = "_schema_"
	TimestampFormat   = time.RFC3339
)

// Common errors
var (
	ErrKeyNotFound     = errors.New("key not found")
	ErrTypeMismatch    = errors.New("type mismatch")
	ErrInvalidPath     = errors.New("invalid path")
	ErrSchemaViolation = errors.New("schema violation")
	ErrInvalidQuery    = errors.New("invalid query")
)

// Constructor functions

// NewDocument creates a new Document from a map[string]any.
func NewDocument(data map[string]any) Document {
	if data == nil {
		return make(Document)
	}
	return Document(data)
}

// MustNewDocument creates a new Document from various map forms, panics on failure.
func MustNewDocument(data any) Document {
	if data == nil {
		return make(Document)
	}
	switch v := data.(type) {
	case map[string]any:
		return Document(v)
	case Document:
		return v
	default:
		rv := reflect.ValueOf(data)
		if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
			doc := make(Document, rv.Len())
			for _, key := range rv.MapKeys() {
				doc[key.String()] = rv.MapIndex(key).Interface()
			}
			return doc
		}
	}
	panic(fmt.Sprintf("invalid type for document: %T", data))
}

func (d Document) Normalize() Document {
    // Copy the document
    clean := make(Document, len(d))
    for k, v := range d {
        if k == MetadataFieldName {
            clean[k] = v // keep top-level metadata
            continue
        }
        clean[k] = stripNestedMetadata(v)
    }
    return clean
}

func stripNestedMetadata(value any) any {
    switch v := value.(type) {
    case Document:
        // strip metadata completely for nested docs
        clean := make(Document, len(v))
        for k2, v2 := range v {
            if k2 != MetadataFieldName {
                clean[k2] = stripNestedMetadata(v2)
            }
        }
        return clean
    case []Document:
        out := make([]Document, len(v))
        for i, doc := range v {
            out[i] = stripNestedMetadata(doc).(Document)
        }
        return out
    case []any:
        out := make([]any, len(v))
        for i, item := range v {
            out[i] = stripNestedMetadata(item)
        }
        return out
    default:
        return v
    }
}

// FromJSON creates a Document from JSON bytes with enhanced error handling.
func FromJSON(data []byte) (Document, error) {
	if len(data) == 0 {
		return make(Document), nil
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, &DocumentError{
			Operation: "FromJSON",
			Message:   "failed to unmarshal JSON",
			Cause:     err,
		}
	}
	return Document(doc), nil
}

// FromStruct creates a Document from any struct using JSON marshaling.
func FromStruct(s any) (Document, error) {
	if s == nil {
		return make(Document), nil
	}

	data, err := json.Marshal(s)
	if err != nil {
		return nil, &DocumentError{
			Operation: "FromStruct",
			Message:   "failed to marshal struct",
			Cause:     err,
		}
	}

	return FromJSON(data)
}

// Basic operations with enhanced error handling

// Get retrieves a value with detailed error information.
func (d Document) Get(key string) (any, error) {
	val, ok := d[key]
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
	if val, err := d.Get(key); err == nil {
		return val
	}
	return defaultValue
}

// MustGet retrieves a value, panics if not found.
func (d Document) MustGet(key string) any {
	val, err := d.Get(key)
	if err != nil {
		panic(err)
	}
	return val
}

// Set with validation support.
func (d Document) Set(key string, value any) error {
	if key == "" {
		return &DocumentError{
			Operation: "Set",
			Key:       key,
			Message:   "key cannot be empty",
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

// Enhanced type-safe accessors with better error handling

// GetString with comprehensive type coercion.
func (d Document) GetString(key string) (string, error) {
	val, err := d.Get(key)
	if err != nil {
		return "", err
	}

	str, ok := CoerceToString(val)
	if !ok {
		return "", &DocumentError{
			Operation: "GetString",
			Key:       key,
			Message:   fmt.Sprintf("cannot convert %T to string", val),
			Cause:     ErrTypeMismatch,
		}
	}
	return str, nil
}

// GetInt with comprehensive numeric coercion.
func (d Document) GetInt(key string) (int, error) {
	val, err := d.Get(key)
	if err != nil {
		return 0, err
	}

	num, ok := CoerceToInt(val)
	if !ok {
		return 0, &DocumentError{
			Operation: "GetInt",
			Key:       key,
			Message:   fmt.Sprintf("cannot convert %T to int", val),
			Cause:     ErrTypeMismatch,
		}
	}
	return num, nil
}

// GetFloat64 with numeric coercion.
func (d Document) GetFloat64(key string) (float64, error) {
	val, err := d.Get(key)
	if err != nil {
		return 0, err
	}

	num, ok := CoerceToFloat64(val)
	if !ok {
		return 0, &DocumentError{
			Operation: "GetFloat64",
			Key:       key,
			Message:   fmt.Sprintf("cannot convert %T to float64", val),
			Cause:     ErrTypeMismatch,
		}
	}
	return num, nil
}

// GetBool with string parsing support.
func (d Document) GetBool(key string) (bool, error) {
	val, err := d.Get(key)
	if err != nil {
		return false, err
	}

	b, ok := CoerceToBool(val)
	if !ok {
		return false, &DocumentError{
			Operation: "GetBool",
			Key:       key,
			Message:   fmt.Sprintf("cannot convert %T to bool", val),
			Cause:     ErrTypeMismatch,
		}
	}
	return b, nil
}

// GetTime parses time from various formats.
func (d Document) GetTime(key string) (time.Time, error) {
	val, err := d.Get(key)
	if err != nil {
		return time.Time{}, err
	}

	t, ok := CoerceToTime(val)
	if !ok {
		return time.Time{}, &DocumentError{
			Operation: "GetTime",
			Key:       key,
			Message:   fmt.Sprintf("cannot convert %T to time.Time", val),
			Cause:     ErrTypeMismatch,
		}
	}
	return t, nil
}

// GetDocument retrieves a nested document.
func (d Document) GetDocument(key string) (Document, error) {
	val, err := d.Get(key)
	if err != nil {
		return nil, err
	}

	doc, ok := AsDocument(val)
	if !ok {
		return nil, &DocumentError{
			Operation: "GetDocument",
			Key:       key,
			Message:   fmt.Sprintf("cannot convert %T to Document", val),
			Cause:     ErrTypeMismatch,
		}
	}
	return doc, nil
}

// GetDocumentArray retrieves an array of documents.
func (d Document) GetDocumentArray(key string) ([]Document, error) {
	val, err := d.Get(key)
	if err != nil {
		return nil, err
	}

	docs, ok := AsDocumentArray(val)
	if !ok {
		return nil, &DocumentError{
			Operation: "GetDocumentArray",
			Key:       key,
			Message:   fmt.Sprintf("cannot convert %T to []Document", val),
			Cause:     ErrTypeMismatch,
		}
	}
	return docs, nil
}

// Advanced nested operations

// GetNested with enhanced path parsing and error handling.
func (d Document) GetNested(path string) (any, error) {
	if path == "" {
		return nil, &DocumentError{
			Operation: "GetNested",
			Key:       path,
			Message:   "path cannot be empty",
			Cause:     ErrInvalidPath,
		}
	}

	parts := strings.Split(path, ".")
	current := any(d)

	for i, part := range parts {
		switch v := current.(type) {
		case Document:
			val, err := v.Get(part)
			if err != nil {
				return nil, &DocumentError{
					Operation: "GetNested",
					Key:       strings.Join(parts[:i+1], "."),
					Message:   "path segment not found",
					Cause:     err,
				}
			}
			current = val
		case map[string]any:
			val, ok := v[part]
			if !ok {
				return nil, &DocumentError{
					Operation: "GetNested",
					Key:       strings.Join(parts[:i+1], "."),
					Message:   "path segment not found",
					Cause:     ErrKeyNotFound,
				}
			}
			current = val
		default:
			return nil, &DocumentError{
				Operation: "GetNested",
				Key:       strings.Join(parts[:i+1], "."),
				Message:   fmt.Sprintf("cannot traverse into %T", v),
				Cause:     ErrInvalidPath,
			}
		}
	}

	return current, nil
}

// SetNested with path validation and intermediate map creation.
func (d Document) SetNested(path string, value any) error {
	if path == "" {
		return &DocumentError{
			Operation: "SetNested",
			Key:       path,
			Message:   "path cannot be empty",
			Cause:     ErrInvalidPath,
		}
	}

	parts := strings.Split(path, ".")
	current := d

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

		if nextMap, ok := next.(map[string]any); ok {
			current = Document(nextMap)
		} else if nextDoc, ok := next.(Document); ok {
			current = nextDoc
		} else {
			return &DocumentError{
				Operation: "SetNested",
				Key:       strings.Join(parts[:i+1], "."),
				Message:   fmt.Sprintf("cannot traverse into %T", next),
				Cause:     ErrInvalidPath,
			}
		}
	}

	return nil
}

// DeleteNested removes a value at a nested path.
func (d Document) DeleteNested(path string) error {
	if path == "" {
		return &DocumentError{
			Operation: "DeleteNested",
			Key:       path,
			Message:   "path cannot be empty",
			Cause:     ErrInvalidPath,
		}
	}

	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		delete(d, parts[0])
		return nil
	}

	parentPath := strings.Join(parts[:len(parts)-1], ".")
	parent, err := d.GetNested(parentPath)
	if err != nil {
		return err
	}

	switch p := parent.(type) {
	case Document:
		delete(p, parts[len(parts)-1])
	case map[string]any:
		delete(p, parts[len(parts)-1])
	default:
		return &DocumentError{
			Operation: "DeleteNested",
			Key:       path,
			Message:   fmt.Sprintf("parent is not a map: %T", p),
			Cause:     ErrInvalidPath,
		}
	}

	return nil
}

// Transformation operations

// Transform creates a new transformation pipeline.
func (d Document) Transform() *DocumentTransform {
	return &DocumentTransform{doc: d.Clone()}
}

// Map applies a transformation to each value.
func (tp *DocumentTransform) Map(transformer func(key string, value any) any) *DocumentTransform {
	tp.ops = append(tp.ops, func(d Document) Document {
		result := make(Document)
		for k, v := range d {
			result[k] = transformer(k, v)
		}
		return result
	})
	return tp
}

// Filter removes key-value pairs that don't match the predicate.
func (tp *DocumentTransform) Filter(predicate func(key string, value any) bool) *DocumentTransform {
	tp.ops = append(tp.ops, func(d Document) Document {
		result := make(Document)
		for k, v := range d {
			if predicate(k, v) {
				result[k] = v
			}
		}
		return result
	})
	return tp
}

// Pick selects only the specified keys.
func (tp *DocumentTransform) Pick(keys ...string) *DocumentTransform {
	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}

	return tp.Filter(func(key string, value any) bool {
		return keySet[key]
	})
}

// Omit excludes the specified keys.
func (tp *DocumentTransform) Omit(keys ...string) *DocumentTransform {
	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}

	return tp.Filter(func(key string, value any) bool {
		return !keySet[key]
	})
}

// Execute applies all transformations and returns the result.
func (tp *DocumentTransform) Execute() Document {
	result := tp.doc
	for _, op := range tp.ops {
		result = op(result)
	}
	return result
}

// Utility functions

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
	result := make(Document)
	for k, v := range d {
		result[k] = deepCloneValue(v)
	}
	return result
}

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

// Serialization

// ToJSON with pretty printing option.
func (d Document) ToJSON(pretty ...bool) ([]byte, error) {
	if len(pretty) > 0 && pretty[0] {
		return json.MarshalIndent(d, "", "  ")
	}
	return json.Marshal(d)
}

// ToStruct converts to a struct with better error handling.
func (d Document) ToStruct(target any) error {
	data, err := d.ToJSON()
	if err != nil {
		return &DocumentError{
			Operation: "ToStruct",
			Message:   "failed to marshal to JSON",
			Cause:     err,
		}
	}

	if err := json.Unmarshal(data, target); err != nil {
		return &DocumentError{
			Operation: "ToStruct",
			Message:   "failed to unmarshal to struct",
			Cause:     err,
		}
	}

	return nil
}

// Type conversion utilities

// CoerceToString attempts to convert any value to string.
func CoerceToString(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case fmt.Stringer:
		return val.String(), true
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", val), true
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val), true
	case float32, float64:
		return fmt.Sprintf("%g", val), true
	case bool:
		return fmt.Sprintf("%t", val), true
	case nil:
		return "", true
	default:
		if s := fmt.Sprintf("%v", val); s != "" {
			return s, true
		}
		return "", false
	}
}

// CoerceToInt attempts to convert any value to int.
func CoerceToInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int8:
		return int(val), true
	case int16:
		return int(val), true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	case uint:
		return int(val), true
	case uint8:
		return int(val), true
	case uint16:
		return int(val), true
	case uint32:
		return int(val), true
	case uint64:
		return int(val), true
	case float32:
		return int(val), true
	case float64:
		return int(val), true
	case string:
		if i, err := strconv.Atoi(val); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return int(f), true
		}
		return 0, false
	case bool:
		if val {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

// CoerceToFloat64 attempts to convert any value to float64.
func CoerceToFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int, int8, int16, int32, int64:
		return float64(reflect.ValueOf(val).Int()), true
	case uint, uint8, uint16, uint32, uint64:
		return float64(reflect.ValueOf(val).Uint()), true
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, true
		}
		return 0, false
	case bool:
		if val {
			return 1.0, true
		}
		return 0.0, true
	default:
		return 0, false
	}
}

// CoerceToBool attempts to convert any value to bool.
func CoerceToBool(v any) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case string:
		lower := strings.ToLower(val)
		if lower == "true" || lower == "1" || lower == "yes" || lower == "on" {
			return true, true
		}
		if lower == "false" || lower == "0" || lower == "no" || lower == "off" || lower == "" {
			return false, true
		}
		return false, false
	case int, int8, int16, int32, int64:
		return reflect.ValueOf(val).Int() != 0, true
	case uint, uint8, uint16, uint32, uint64:
		return reflect.ValueOf(val).Uint() != 0, true
	case float32, float64:
		return reflect.ValueOf(val).Float() != 0, true
	case nil:
		return false, true
	default:
		return false, false
	}
}

// CoerceToTime attempts to convert any value to time.Time.
func CoerceToTime(v any) (time.Time, bool) {
	switch val := v.(type) {
	case time.Time:
		return val, true
	case string:
		// Try common time formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			time.RFC822,
			time.RFC822Z,
			time.RFC1123,
			time.RFC1123Z,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05.000000000",
			"2006-01-02",
			"15:04:05",
		}

		for _, format := range formats {
			if t, err := time.Parse(format, val); err == nil {
				return t, true
			}
		}
		return time.Time{}, false
	case int64:
		return time.Unix(val, 0), true
	case float64:
		return time.Unix(int64(val), 0), true
	default:
		return time.Time{}, false
	}
}

// AsDocument attempts to convert any value to a Document.
func AsDocument(v any) (Document, bool) {
	switch val := v.(type) {
	case Document:
		return val, true
	case map[string]any:
		return Document(val), true
	case nil:
		return make(Document), true
	default:
		return nil, false
	}
}

// AsDocumentArray attempts to convert any value to []Document.
func AsDocumentArray(v any) ([]Document, bool) {
	switch val := v.(type) {
	case []Document:
		return val, true
	case []any:
		docs := make([]Document, 0, len(val))
		for _, item := range val {
			if doc, ok := AsDocument(item); ok {
				docs = append(docs, doc)
			} else {
				return nil, false
			}
		}
		return docs, true
	case []map[string]any:
		docs := make([]Document, len(val))
		for i, m := range val {
			docs[i] = Document(m)
		}
		return docs, true
	default:
		return nil, false
	}
}

// Batch operations

// DocumentSet represents a collection of documents with batch operations.
type DocumentSet []Document

// NewDocumentSet creates a new document set.
func NewDocumentSet(docs ...Document) DocumentSet {
	return DocumentSet(docs)
}

// Filter applies a filter to all documents in the set.
func (ds DocumentSet) Filter(predicate func(Document) bool) DocumentSet {
	result := make(DocumentSet, 0)
	for _, doc := range ds {
		if predicate(doc) {
			result = append(result, doc)
		}
	}
	return result
}

// Map applies a transformation to all documents in the set.
func (ds DocumentSet) Map(transformer func(Document) Document) DocumentSet {
	result := make(DocumentSet, len(ds))
	for i, doc := range ds {
		result[i] = transformer(doc)
	}
	return result
}

// Find returns the first document matching the predicate.
func (ds DocumentSet) Find(predicate func(Document) bool) (Document, bool) {
	for _, doc := range ds {
		if predicate(doc) {
			return doc, true
		}
	}
	return nil, false
}

// Where returns documents where the specified key equals the value.
func (ds DocumentSet) Where(key string, value any) DocumentSet {
	return ds.Filter(func(d Document) bool {
		val, err := d.Get(key)
		if err != nil {
			return false
		}
		return reflect.DeepEqual(val, value)
	})
}

// SortBy sorts documents by a key in ascending order.
func (ds DocumentSet) SortBy(key string) DocumentSet {
	result := make(DocumentSet, len(ds))
	copy(result, ds)

	sort.Slice(result, func(i, j int) bool {
		val1, err1 := result[i].Get(key)
		val2, err2 := result[j].Get(key)

		if err1 != nil && err2 != nil {
			return false
		}
		if err1 != nil {
			return true
		}
		if err2 != nil {
			return false
		}

		return compareValues(val1, val2) < 0
	})

	return result
}

// SortByDesc sorts documents by a key in descending order.
func (ds DocumentSet) SortByDesc(key string) DocumentSet {
	result := ds.SortBy(key)
	// Reverse the slice
	for i := 0; i < len(result)/2; i++ {
		j := len(result) - 1 - i
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// GroupBy groups documents by a key value.
func (ds DocumentSet) GroupBy(key string) map[string]DocumentSet {
	groups := make(map[string]DocumentSet)

	for _, doc := range ds {
		val, err := doc.GetString(key)
		if err != nil {
			val = "undefined"
		}

		if _, exists := groups[val]; !exists {
			groups[val] = make(DocumentSet, 0)
		}
		groups[val] = append(groups[val], doc)
	}

	return groups
}

// Reduce applies a reducer function to all documents.
func (ds DocumentSet) Reduce(reducer func(acc, current Document) Document, initial Document) Document {
	result := initial.Clone()
	for _, doc := range ds {
		result = reducer(result, doc)
	}
	return result
}

// Aggregate performs common aggregation operations.
func (ds DocumentSet) Aggregate(key string) AggregationResult {
	var sum float64
	var count int
	var min, max float64
	var values []float64

	for _, doc := range ds {
		if val, err := doc.GetFloat64(key); err == nil {
			values = append(values, val)
			sum += val
			count++

			if count == 1 {
				min = val
				max = val
			} else {
				if val < min {
					min = val
				}
				if val > max {
					max = val
				}
			}
		}
	}

	result := AggregationResult{
		Count: count,
		Sum:   sum,
		Min:   min,
		Max:   max,
	}

	if count > 0 {
		result.Average = sum / float64(count)

		// Calculate median
		sort.Float64s(values)
		if count%2 == 0 {
			result.Median = (values[count/2-1] + values[count/2]) / 2
		} else {
			result.Median = values[count/2]
		}

		// Calculate standard deviation
		var variance float64
		for _, val := range values {
			diff := val - result.Average
			variance += diff * diff
		}
		result.StdDev = variance / float64(count)
	}

	return result
}

// AggregationResult contains statistical aggregation results.
type AggregationResult struct {
	Count   int     `json:"count"`
	Sum     float64 `json:"sum"`
	Average float64 `json:"average"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Median  float64 `json:"median"`
	StdDev  float64 `json:"std_dev"`
}

// Utility helper functions

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
	if numA, okA := CoerceToFloat64(a); okA {
		if numB, okB := CoerceToFloat64(b); okB {
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
	strA, _ := CoerceToString(a)
	strB, _ := CoerceToString(b)

	if strA < strB {
		return -1
	}
	if strA > strB {
		return 1
	}
	return 0
}

// Advanced querying with JSONPath-like syntax

// JSONPathQuery executes a JSONPath-like query on the document.
func (d Document) JSONPathQuery(path string) ([]any, error) {
	if path == "" || path == "$" {
		return []any{d}, nil
	}
	path = strings.TrimPrefix(path, "$.")
	return d.executeJSONPath(path)
}

func (d Document) executeJSONPath(path string) ([]any, error) {
	parts := strings.Split(path, ".")
	results := []any{d}

	for _, part := range parts {
		var newResults []any

		for _, result := range results {
			if part == "*" {
				// Wildcard - get all values
				if doc, ok := AsDocument(result); ok {
					for _, val := range doc.Values() {
						newResults = append(newResults, val)
					}
				} else if arr, ok := result.([]any); ok {
					newResults = append(newResults, arr...)
				}
			} else if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
				// Array index or filter
				indexStr := part[1 : len(part)-1]
				if arr, ok := result.([]any); ok {
					if index, err := strconv.Atoi(indexStr); err == nil {
						if index >= 0 && index < len(arr) {
							newResults = append(newResults, arr[index])
						}
					}
				}
			} else {
				// Regular key access
				if doc, ok := AsDocument(result); ok {
					if val, err := doc.Get(part); err == nil {
						newResults = append(newResults, val)
					}
				}
			}
		}

		results = newResults
		if len(results) == 0 {
			break
		}
	}

	return results, nil
}

// Advanced metadata operations

// SetTimestamp adds a timestamp to metadata.
func (d Document) SetTimestamp(key string) {
	metadata, _ := d.Metadata()
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[key] = time.Now().Format(TimestampFormat)
	d.SetMetadata(metadata)
}

// GetCreatedAt retrieves the creation timestamp from metadata.
func (d Document) GetCreatedAt() (time.Time, error) {
	metadata, ok := d.Metadata()
	if !ok {
		return time.Time{}, &DocumentError{
			Operation: "GetCreatedAt",
			Message:   "no metadata found",
			Cause:     ErrKeyNotFound,
		}
	}

	if created, ok := metadata["created_at"]; ok {
		result, ok := CoerceToTime(created)
		if !ok {

			return time.Time{}, &DocumentError{
				Operation: "GetCreatedAt",
				Key:       "created_at",
				Message:   "created_at value cannot be coerced into time",
				Cause:     ErrKeyNotFound,
			}
		}
		return result, nil
	}

	return time.Time{},
		&DocumentError{
			Operation: "GetCreatedAt",
			Key:       "created_at",
			Message:   "created_at not found in metadata",
			Cause:     ErrKeyNotFound,
		}
}

// SetVersion increments the version in metadata.
func (d Document) SetVersion(version int) {
	metadata, _ := d.Metadata()
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["version"] = version
	d.SetMetadata(metadata)
}

// GetVersion retrieves the version from metadata.
func (d Document) GetVersion() (int, error) {
	metadata, ok := d.Metadata()
	if !ok {
		return 0, &DocumentError{
			Operation: "GetVersion",
			Message:   "no metadata found",
			Cause:     ErrKeyNotFound,
		}
	}

	if version, ok := metadata["version"]; ok {
		if v, ok := CoerceToInt(version); ok {
			return v, nil
		}
	}

	return 0, &DocumentError{
		Operation: "GetVersion",
		Key:       "version",
		Message:   "version not found or invalid in metadata",
		Cause:     ErrKeyNotFound,
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
	result := make(Document)
	for k, v := range d {
		if k != MetadataFieldName {
			result[k] = v
		}
	}
	return result
}

// Performance and caching utilities

// DocumentCache provides simple in-memory caching for documents.
type DocumentCache struct {
	cache   map[string]Document
	maxSize int
}

// NewDocumentCache creates a new document cache with specified maximum size.
func NewDocumentCache(maxSize int) *DocumentCache {
	return &DocumentCache{
		cache:   make(map[string]Document),
		maxSize: maxSize,
	}
}

// Get retrieves a document from cache.
func (dc *DocumentCache) Get(key string) (Document, bool) {
	doc, ok := dc.cache[key]
	return doc, ok
}

// Set stores a document in cache.
func (dc *DocumentCache) Set(key string, doc Document) {
	if len(dc.cache) >= dc.maxSize {
		// Simple LRU: remove first key (not truly LRU but simple)
		for k := range dc.cache {
			delete(dc.cache, k)
			break
		}
	}
	dc.cache[key] = doc.Clone()
}

// Clear removes all cached documents.
func (dc *DocumentCache) Clear() {
	dc.cache = make(map[string]Document)
}

// Index provides fast lookups for document collections.
type DocumentIndex struct {
	keyIndex     map[string]map[any][]int // key -> value -> document indices
	documents    []Document
	keyExtractor func(Document) map[string]any
}

// NewDocumentIndex creates a new index for documents.
func NewDocumentIndex(docs []Document, keyExtractor func(Document) map[string]any) *DocumentIndex {
	index := &DocumentIndex{
		keyIndex:     make(map[string]map[any][]int),
		documents:    docs,
		keyExtractor: keyExtractor,
	}

	index.rebuild()
	return index
}

// rebuild recreates the index.
func (di *DocumentIndex) rebuild() {
	di.keyIndex = make(map[string]map[any][]int)

	for i, doc := range di.documents {
		keys := di.keyExtractor(doc)
		for key, value := range keys {
			if di.keyIndex[key] == nil {
				di.keyIndex[key] = make(map[any][]int)
			}
			di.keyIndex[key][value] = append(di.keyIndex[key][value], i)
		}
	}
}

// Find returns documents matching the key-value pair.
func (di *DocumentIndex) Find(key string, value any) []Document {
	if valueMap, ok := di.keyIndex[key]; ok {
		if indices, ok := valueMap[value]; ok {
			result := make([]Document, len(indices))
			for i, idx := range indices {
				result[i] = di.documents[idx]
			}
			return result
		}
	}
	return []Document{}
}

// Document builder pattern

// DocumentBuilder provides a fluent interface for building documents.
type DocumentBuilder struct {
	doc Document
}

// NewDocumentBuilder creates a new document builder.
func NewDocumentBuilder() *DocumentBuilder {
	return &DocumentBuilder{
		doc: make(Document),
	}
}

// Set adds a key-value pair.
func (db *DocumentBuilder) Set(key string, value any) *DocumentBuilder {
	db.doc[key] = value
	return db
}

// SetIf conditionally adds a key-value pair.
func (db *DocumentBuilder) SetIf(condition bool, key string, value any) *DocumentBuilder {
	if condition {
		db.doc[key] = value
	}
	return db
}

// SetNested adds a nested value.
func (db *DocumentBuilder) SetNested(path string, value any) *DocumentBuilder {
	db.doc.SetNested(path, value)
	return db
}

// WithMetadata adds metadata.
func (db *DocumentBuilder) WithMetadata(metadata map[string]any) *DocumentBuilder {
	db.doc.SetMetadata(metadata)
	return db
}

// WithTimestamp adds a creation timestamp.
func (db *DocumentBuilder) WithTimestamp() *DocumentBuilder {
	db.doc.SetTimestamp("created_at")
	return db
}

// Build returns the constructed document.
func (db *DocumentBuilder) Build() Document {
	return db.doc.Clone()
}

// Convenience functions for common operations

// Flatten creates a flat map from nested document structure.
func (d Document) Flatten(separator string) map[string]any {
	if separator == "" {
		separator = "."
	}

	result := make(map[string]any)
	d.flattenInto(result, "", separator)
	return result
}

func (d Document) flattenInto(result map[string]any, prefix, separator string) {
	for k, v := range d {
		key := k
		if prefix != "" {
			key = prefix + separator + k
		}

		if doc, ok := AsDocument(v); ok {
			doc.flattenInto(result, key, separator)
		} else if arr, ok := v.([]any); ok {
			for i, item := range arr {
				itemKey := fmt.Sprintf("%s[%d]", key, i)
				if itemDoc, ok := AsDocument(item); ok {
					itemDoc.flattenInto(result, itemKey, separator)
				} else {
					result[itemKey] = item
				}
			}
		} else {
			result[key] = v
		}
	}
}

// Unflatten reconstructs nested structure from flat map.
func Unflatten(flat map[string]any, separator string) Document {
	if separator == "" {
		separator = "."
	}

	doc := make(Document)

	for key, value := range flat {
		parts := strings.Split(key, separator)
		current := doc

		for i, part := range parts {
			if i == len(parts)-1 {
				current[part] = value
			} else {
				next, ok := current[part]
				if !ok {
					next = make(map[string]any)
					current[part] = next
				}
				if nextDoc, ok := AsDocument(next); ok {
					current = nextDoc
				} else if nextMap, ok := next.(map[string]any); ok {
					current = Document(nextMap)
				}
			}
		}
	}

	return doc
}

// Diff computes differences between two documents.
func (d Document) Diff(other Document) DocumentDiff {
	diff := DocumentDiff{
		Added:    make(map[string]any),
		Removed:  make(map[string]any),
		Modified: make(map[string]DiffValue),
	}

	// Find added and modified
	for k, v := range other {
		if existing, ok := d[k]; ok {
			if !reflect.DeepEqual(existing, v) {
				diff.Modified[k] = DiffValue{Old: existing, New: v}
			}
		} else {
			diff.Added[k] = v
		}
	}

	// Find removed
	for k, v := range d {
		if _, ok := other[k]; !ok {
			diff.Removed[k] = v
		}
	}

	return diff
}

// DocumentDiff represents differences between two documents.
type DocumentDiff struct {
	Added    map[string]any       `json:"added"`
	Removed  map[string]any       `json:"removed"`
	Modified map[string]DiffValue `json:"modified"`
}

// DiffValue represents a changed value.
type DiffValue struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// HasChanges returns true if there are any differences.
func (dd DocumentDiff) HasChanges() bool {
	return len(dd.Added) > 0 || len(dd.Removed) > 0 || len(dd.Modified) > 0
}

// Apply applies the diff to create a new document.
func (d Document) Apply(diff DocumentDiff) Document {
	result := d.Clone()

	// Remove deleted keys
	for k := range diff.Removed {
		delete(result, k)
	}

	// Add new keys
	maps.Copy(result, diff.Added)

	// Modify changed keys
	for k, v := range diff.Modified {
		result[k] = v.New
	}

	return result
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
	return len(d)
}

// IsEmpty checks if the document is empty.
func (d Document) IsEmpty() bool {
	return len(d) == 0
}

// HasKey checks if a key exists.
func (d Document) HasKey(key string) bool {
	_, ok := d[key]
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
