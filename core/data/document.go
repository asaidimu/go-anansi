package data

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"sort"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// Constants
const (
	SchemaField = "_schema_"
)

// Document represents a flexible, schema-aware data structure.
type Document struct {
	ctx  context.Context
	data map[string]any
}

// Patch represents a set of fields to be updated.
// It is a map[string]any but signifies that it is a partial document.
type Patch map[string]any

// Document converts the Patch into a Document.
// It uses the same logic as your existing Patch function:
// bypassing auto-IDs and Metadata.
func (p Patch) Document(ctx ...context.Context) *Document {
	data := map[string]any(p)
	if data == nil {
		data = make(map[string]any)
	}

	dctx := context.Background()
	if len(ctx) > 0 && ctx[0] != nil {
		dctx = ctx[0]
	}
	return &Document{
		ctx:  dctx,
		data: data,
	}
}

// convertToDocumentMap converts various input types to map[string]any for Must* functions
func convertToDocumentMap(data any) (map[string]any, error) {
	if data == nil {
		return make(map[string]any), nil
	}

	switch v := data.(type) {
	case map[string]any:
		return v, nil
	case *Document:
		if v == nil {
			return make(map[string]any), nil
		}
		if v.data == nil {
			return make(map[string]any), nil
		}
		return v.data, nil
	case Document:
		if v.data == nil {
			return make(map[string]any), nil
		}
		return v.data, nil
	default:
		rv := reflect.ValueOf(data)
		if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
			doc := make(map[string]any, rv.Len())
			for _, key := range rv.MapKeys() {
				doc[key.String()] = rv.MapIndex(key).Interface()
			}
			return doc, nil
		}
		return nil, common.SystemErrorFrom(ErrInvalidTargetType).WithOperation("data.convertToDocumentMap").WithMessage(fmt.Sprintf("cannot convert %T to Document", data))
	}
}

// NewDocument creates a new Document from a map[string]any.
func NewDocument(data any, ctx ...context.Context) (*Document, error) {
	docMap, err := convertToDocumentMap(data)
	if err != nil {
		return nil, err
	}
	dctx := context.Background()
	if len(ctx) > 0 && ctx[0] != nil {
		dctx = ctx[0]
	}
	return getFactory().newDocument(dctx, docMap)
}

// MustNewDocument creates a new Document from various map forms, panics on failure.
func MustNewDocument(data any, ctx ...context.Context) *Document {
	docMap, err := convertToDocumentMap(data)
	if err != nil {
		panic(err)
	}

	dctx := context.Background()
	if len(ctx) > 0 && ctx[0] != nil {
		dctx = ctx[0]
	}

	d, err := getFactory().newDocument(dctx, docMap)
	if err != nil {
		panic(err)
	}

	return d
}

// Context returns the document's context.
func (d *Document) Context() context.Context {
	if d.ctx == nil {
		return context.Background()
	}
	return d.ctx
}

// WithContext returns a new Document with the provided context.
func (d *Document) WithContext(ctx context.Context) *Document {
	newDoc := d.Clone()
	newDoc.ctx = ctx
	return newDoc
}

// Get retrieves a value with detailed error information.
func (d *Document) Get(key string) (any, error) {
	val, ok := utils.GetValueByPath(d.data, key)
	if !ok {
		return nil, common.SystemErrorFrom(ErrKeyNotFound).WithOperation("data.Document.Get").WithPath(key)
	}
	return val, nil
}

// GetOr retrieves a value or returns a default if not found.
func (d *Document) GetOr(key string, defaultValue any) any {
	if val, ok := utils.GetValueByPath(d.data, key); ok {
		return val
	}
	return defaultValue
}

// MustGet retrieves a value, panics if not found.
func (d *Document) MustGet(key string) any {
	val, ok := utils.GetValueByPath(d.data, key)
	if !ok {
		panic(common.SystemErrorFrom(ErrKeyNotFound).WithOperation("data.Document.MustGet").WithPath(key))
	}
	return val
}

// Set with validation support.
func (d *Document) Set(key string, value any) error {
	if key == DocumentIDField {
		return common.SystemErrorFrom(ErrReadOnlyField).WithOperation("data.Document.Set").WithPath(key).WithMessage(fmt.Sprintf("field '%s' is managed by the library and cannot be set manually", DocumentIDField))
	}
	if key == "" {
		return common.SystemErrorFrom(ErrKeyEmpty).WithOperation("data.Document.Set").WithPath(key)
	}
	if d.data == nil {
		d.data = make(map[string]any)
	}
	d.data[key] = value
	return nil
}

// SetIfNotExists sets a value only if the key doesn't exist.
func (d *Document) SetIfNotExists(key string, value any) bool {
	if key == DocumentIDField {
		return false
	}
	if d.data == nil {
		d.data = make(map[string]any)
	}
	if _, exists := d.data[key]; !exists {
		d.data[key] = value
		return true
	}
	return false
}

// GetString with comprehensive type coercion and path support.
func (d *Document) GetString(keyOrPath string) (string, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(""), "GetString")
	if err != nil {
		return "", err
	}
	return val.(string), nil
}

// GetInt with comprehensive numeric coercion and path support.
func (d *Document) GetInt(keyOrPath string) (int, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(0), "GetInt")
	if err != nil {
		return 0, err
	}
	return val.(int), nil
}

// GetFloat64 with numeric coercion and path support.
func (d *Document) GetFloat64(keyOrPath string) (float64, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(0.0), "GetFloat64")
	if err != nil {
		return 0.0, err
	}
	return val.(float64), nil
}

// GetBool with string parsing support and path support.
func (d *Document) GetBool(keyOrPath string) (bool, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(false), "GetBool")
	if err != nil {
		return false, err
	}
	return val.(bool), nil
}

// GetTime parses time from various formats with path support.
func (d *Document) GetTime(keyOrPath string) (time.Time, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(time.Time{}), "GetTime")
	if err != nil {
		return time.Time{}, err
	}
	return val.(time.Time), nil
}

// GetDocument retrieves a nested document with path support.
func (d *Document) GetDocument(keyOrPath string) (*Document, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf(&Document{}), "GetDocument")
	if err != nil {
		return nil, err
	}
	return val.(*Document), nil
}

// GetDocumentArray retrieves an array of documents with path support.
func (d *Document) GetDocumentArray(keyOrPath string) ([]*Document, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf([]*Document{}), "GetDocumentArray")
	if err != nil {
		return nil, err
	}
	return val.([]*Document), nil
}

// GetStringArray retrieves a slice of strings with path support.
func (d *Document) GetStringArray(keyOrPath string) ([]string, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf([]string{}), "GetStringArray")
	if err != nil {
		return nil, err
	}
	return val.([]string), nil
}

// GetIntArray retrieves a slice of integers with path support.
func (d *Document) GetIntArray(keyOrPath string) ([]int, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf([]int{}), "GetIntArray")
	if err != nil {
		return nil, err
	}
	return val.([]int), nil
}

// GetArray retrieves a generic slice ([]any) with path support.
func (d *Document) GetArray(keyOrPath string) ([]any, error) {
	val, err := d.getAndCoerce(keyOrPath, reflect.TypeOf([]any{}), "GetArray")
	if err != nil {
		return nil, err
	}
	return val.([]any), nil
}

// getAndCoerce is a private helper function to retrieve a value by path and coerce it to a target type.
func (d *Document) getAndCoerce(keyOrPath string, targetType reflect.Type, operation string) (any, error) {
	val, ok := utils.GetValueByPath(d.data, keyOrPath)
	if !ok {
		return nil, common.SystemErrorFrom(ErrKeyNotFound).WithOperation("data.Document." + operation).WithPath(keyOrPath)
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
	case reflect.TypeOf(&Document{}):
		coercedVal, conversionOk = DocumentFrom(val)
	case reflect.TypeOf([]*Document{}):
		coercedVal, conversionOk = DocumentSlice(val)
	case reflect.TypeOf([]string{}):
		coercedVal, conversionOk = utils.CoerceToSlice[string](val)
	case reflect.TypeOf([]int{}):
		coercedVal, conversionOk = utils.CoerceToSlice[int](val)
	case reflect.TypeOf([]any{}):
		if slice, ok := val.([]any); ok {
			coercedVal, conversionOk = slice, true
		}
	default:
		return nil, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document." + operation).WithPath(keyOrPath).WithMessage(fmt.Sprintf("unsupported target type for coercion: %s", targetType.String()))
	}

	if !conversionOk {
		return nil, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document." + operation).WithPath(keyOrPath).WithMessage(fmt.Sprintf("cannot convert %T to %s", val, targetType.String())).WithCause(errors.Join(ErrTypeConversion, ErrTypeMismatch))
	}

	return coercedVal, nil
}

// Keys returns all keys sorted alphabetically.
func (d *Document) Keys() []string {
	if d.data == nil {
		return []string{}
	}
	keys := make([]string, 0, len(d.data))
	for k := range d.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Values returns all values in key-sorted order.
func (d *Document) Values() []any {
	keys := d.Keys()
	values := make([]any, len(keys))
	for i, k := range keys {
		values[i] = d.data[k]
	}
	return values
}

// Clone creates a deep copy of the document.
func (d *Document) Clone() *Document {
	if d == nil {
		return nil
	}
	return &Document{
		ctx:  d.ctx,
		data: d.deepClone().(map[string]any),
	}
}

func (d *Document) deepClone() any {
	// deepClone recursively clones the Document's data map and its nested structures.
	if d.data == nil {
		return make(map[string]any)
	}
	result := make(map[string]any)
	for k, v := range d.data {
		result[k] = deepCloneValue(v)
	}
	return result
}

// deepCloneValue recursively clones a value, handling nested Documents, maps (map[string]any),
// and slices ([]any, []Document). Primitive types are returned as is.
func deepCloneValue(v any) any {
	switch val := v.(type) {
	case *Document:
		// When a Document is nested, we clone its data map.
		return val.deepClone()
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
	case []*Document:
		arr := make([]*Document, len(val))
		for i, doc := range val {
			arr[i] = doc.Clone()
		}
		return arr
	default:
		return v
	}
}

// Merge combines multiple documents. The receiving document is modified in place.
func (d *Document) Merge(others ...*Document) {
	for _, other := range others {
		if other != nil && other.data != nil {
			if d.data == nil {
				d.data = make(map[string]any)
			}
			maps.Copy(d.data, other.data)
		}
	}
}

// DeepMerge performs a recursive merge of nested objects. The receiving document is modified in place.
func (d *Document) DeepMerge(others ...*Document) {
	for _, other := range others {
		if other != nil {
			d.deepMergeInto(other)
		}
	}
}

// deepMergeInto recursively merges the content of 'other' Document into 'd'.
func (d *Document) deepMergeInto(other *Document) {
	if d.data == nil {
		d.data = make(map[string]any)
	}
	for k, v := range other.data {
		if ReservedSystemField(k) {
			continue
		}

		if existing, ok := d.data[k]; ok {
			// If existing value is a Document struct, recurse
			if existingDoc, ok := existing.(*Document); ok {
				if otherDoc, ok := DocumentFrom(v); ok {
					existingDoc.deepMergeInto(otherDoc)
					continue
				}
			}
			// If existing value is a map, treat it as a document and recurse
			if existingMap, ok := existing.(map[string]any); ok {
				if otherMap, ok := v.(map[string]any); ok {
					docToMerge := &Document{ctx: context.Background(), data: existingMap}
					docToMerge.deepMergeInto(&Document{ctx: context.Background(), data: otherMap})
					d.data[k] = docToMerge.data // Put the merged map back
					continue
				}
			}
		}
		// Assign new or overwrite non-mergeable values
		d.data[k] = deepCloneValue(v)
	}
}

// String provides a readable JSON representation of the document's data.
func (d *Document) String() string {
	if d == nil {
		return "Document{nil}"
	}
	data, err := d.ToJSON(true)
	if err != nil {
		return fmt.Sprintf("Document{error: %v}", err)
	}
	return string(data)
}

// Len returns the number of key-value pairs in the document's data.
func (d *Document) Len() int {
	if d == nil || d.data == nil {
		return 0
	}
	l := len(d.data)
	if _, ok := d.data[MetadataField]; ok {
		l--
	}
	return l
}

// IsEmpty checks if the document's data map is empty.
func (d *Document) IsEmpty() bool {
	if d == nil {
		return true
	}

	count := 0
	for key := range d.data {
		if ReservedSystemField(key) {
			continue
		}
		count += 1
	}
	return count == 0
}

// HasKey checks if a key exists in the document's data.
func (d *Document) HasKey(key string) bool {
	if d == nil || d.data == nil {
		return false
	}
	_, ok := d.data[key]
	return ok
}

// HasPath checks if a path exists in the document's data (supports dot notation).
func (d *Document) HasPath(keyOrPath string) bool {
	if d == nil || d.data == nil {
		return false
	}
	_, ok := utils.GetValueByPath(d.data, keyOrPath)
	return ok
}

// Is performs deep equality comparison on the data maps of two documents, including ID and metadata.
func (d *Document) Is(other *Document) bool {
	if d == nil && other == nil {
		return true
	}
	if d == nil || other == nil {
		return false
	}
	return reflect.DeepEqual(d.data, other.data)
}

// Equals performs content-only deep equality comparison on the data maps of two documents,
// ignoring auto-generated IDs and dynamic metadata fields.
func (d *Document) Equals(other *Document) bool {
	if d == nil && other == nil {
		return true
	}
	if d == nil || other == nil {
		return false
	}

	// Clone documents to avoid modifying originals
	dClone := d.Clone()
	otherClone := other.Clone()

	// Strip metadata from both documents
	dClone = dClone.StripMetadata()
	otherClone = otherClone.StripMetadata()

	// Remove ID field from both documents for content-only comparison
	if dClone.data != nil {
		delete(dClone.data, DocumentIDField)
	}
	if otherClone.data != nil {
		delete(otherClone.data, DocumentIDField)
	}

	return reflect.DeepEqual(dClone.data, otherClone.data)
}

// ToMap returns a deep map[string]any representation of the document's data.
func (d *Document) ToMap() map[string]any {
	if d == nil || d.data == nil {
		return nil
	}
	out := make(map[string]any, len(d.data))
	for k, v := range d.data {
		out[k] = asMapValue(v)
	}
	return out
}


// asMapValue recursively converts a value to its map[string]any representation.
func asMapValue(v any) any {
	switch val := v.(type) {
	case *Document:
		return val.ToMap() // Recursively call AsMap on nested Document structs
	case Document:
		return val.ToMap()
	case map[string]any:
		nested := make(map[string]any, len(val))
		for nk, nv := range val {
			nested[nk] = asMapValue(nv)
		}
		return nested
	case []any:
		arr := make([]any, len(val))
		for i, item := range val {
			arr[i] = asMapValue(item)
		}
		return arr
	case []*Document:
		arr := make([]map[string]any, len(val))
		for i, doc := range val {
			arr[i] = doc.ToMap() // Convert each Document in the slice to a map
		}
		return arr
	default:
		return v
	}
}

func (d *Document) ID() string {
	if d == nil || d.data == nil {
		return ""
	}
	if val, ok := d.data[DocumentIDField].(string); ok {
		return val
	}
	return ""
}

// Metadata access with enhanced functionality.
func (d *Document) Metadata() (map[string]any, bool) {
	if d == nil || d.data == nil {
		return nil, false
	}
	val, ok := d.data[MetadataField]
	if !ok {
		return nil, false
	}

	if meta, ok := val.(map[string]any); ok {
		newMeta := make(map[string]any, len(meta))
		maps.Copy(newMeta, meta)
		return newMeta, true
	}
	return nil, false
}

func (d *Document) SetMetadata(metadata map[string]any) {
	if d.data == nil {
		d.data = make(map[string]any)
	}
	d.data[MetadataField] = metadata
}

// StripMetadata removes metadata and returns a clean copy.
func (d *Document) StripMetadata() *Document {
	cleanDoc := d.Clone()
	if cleanDoc.data != nil {
		cleanDoc.data = stripNestedMetadata(cleanDoc.data).(map[string]any)
	}
	return cleanDoc
}

// --- Data Integrity ---

// Hash computes and sets the HMAC-SHA256 hash of the metadata block.
func (d *Document) Hash() error {
	meta, ok := d.Metadata()
	if !ok {
		return common.SystemErrorFrom(ErrNoMetadata).WithOperation("data.Document.Hash")
	}

	hash, err := getFactory().calculateHash(d)
	if err != nil {
		return err
	}

	meta[MetadataChecksum] = hash
	d.SetMetadata(meta)
	return nil
}

// VerifyHash checks the integrity of the metadata block against its hash.
func (d *Document) VerifyHash() (bool, error) {
	meta, ok := d.Metadata()
	if !ok {
		return false, nil
	}

	providedHash, ok := meta[MetadataChecksum].(string)
	if !ok {
		return false, nil
	}

	calculatedHash, err := getFactory().calculateHash(d)
	if err != nil {
		return false, err
	}

	return providedHash == calculatedHash, nil
}

// Sign computes and sets the RSA signature for the entire document.
func (d *Document) Sign(privateKey *rsa.PrivateKey) error {
	signature, err := getFactory().signDocument(d, privateKey)
	if err != nil {
		return err
	}

	meta, ok := d.Metadata()
	if !ok {
		meta = make(map[string]any)
	}

	meta[MetadataSignature] = signature
	d.SetMetadata(meta)
	return nil
}

// Verify checks the RSA signature of the document.
func (d *Document) Verify(publicKey *rsa.PublicKey) error {
	meta, ok := d.Metadata()
	if !ok {
		return common.SystemErrorFrom(ErrNoMetadata).WithOperation("data.Document.Verify")
	}

	signature, ok := meta[MetadataSignature].(string)
	if !ok {
		return common.SystemErrorFrom(ErrSignatureInvalid).WithOperation("data.Document.Verify").WithMessage("no signature found in metadata")
	}

	return getFactory().verifySignature(d, publicKey, signature)
}

// --- Metadata Accessors ---

// GetMetadataValue retrieves a value from the document's metadata map.
func (d *Document) GetMetadataValue(key string) (any, error) {
	meta, ok := d.Metadata()
	if !ok {
		return nil, common.SystemErrorFrom(ErrNoMetadata).WithOperation("data.Document.GetMetadataValue").WithPath(key)
	}
	val, ok := meta[key]
	if !ok {
		return nil, common.SystemErrorFrom(ErrKeyNotFound).WithOperation("data.Document.GetMetadataValue").WithPath(key)
	}
	return val, nil
}

// SetMetadataValue sets a value in the document's metadata map.
func (d *Document) SetMetadataValue(key string, value any) error {
	switch key {
	case MetadataChecksum, MetadataSignature, MetadataVersion, MetadataCreated, MetadataUpdated:
		return common.SystemErrorFrom(ErrReadOnlyField).WithOperation("data.Document.SetMetadataValue").WithPath(key).WithMessage("cannot overwrite internal metadata field")
	}

	meta, ok := d.Metadata()
	if !ok {
		meta = make(map[string]any)
	}
	meta[key] = value
	d.SetMetadata(meta)
	return nil
}

// Version returns the document's version number.
func (d *Document) Version() (int, error) {
	val, err := d.GetMetadataValue(MetadataVersion)
	if err != nil {
		return 0, err
	}
	version, ok := utils.CoerceToPrimitiveValue[int](val)
	if !ok {
		return 0, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document.Version").WithPath(MetadataVersion).WithMessage(fmt.Sprintf("cannot convert %T to int", val))
	}
	return version, nil
}

// Checksum returns the document's checksum string.
func (d *Document) Checksum() (string, error) {
	val, err := d.GetMetadataValue(MetadataChecksum)
	if err != nil {
		return "", err
	}
	checksum, ok := val.(string)
	if !ok {
		return "", common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document.Checksum").WithPath(MetadataChecksum).WithMessage(fmt.Sprintf("cannot convert %T to string", val))
	}
	return checksum, nil
}

// Signature returns the document's signature string.
func (d *Document) Signature() (string, error) {
	val, err := d.GetMetadataValue(MetadataSignature)
	if err != nil {
		return "", err
	}
	signature, ok := val.(string)
	if !ok {
		return "", common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document.Signature").WithPath(MetadataSignature).WithMessage(fmt.Sprintf("cannot convert %T to string", val))
	}
	return signature, nil
}

// CreatedAt returns the document's creation timestamp.
func (d *Document) CreatedAt() (time.Time, error) {
	val, err := d.GetMetadataValue(MetadataCreated)
	if err != nil {
		return time.Time{}, err
	}
	createdAt, ok := utils.CoerceTime(val)
	if !ok {
		return time.Time{}, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document.CreatedAt").WithPath(MetadataCreated).WithMessage(fmt.Sprintf("cannot convert %T to time.Time", val))
	}
	return createdAt, nil
}

// UpdatedAt returns the document's last update timestamp.
func (d *Document) UpdatedAt() (time.Time, error) {
	val, err := d.GetMetadataValue(MetadataUpdated)
	if err != nil {
		return time.Time{}, err
	}
	updatedAt, ok := utils.CoerceTime(val)
	if !ok {
		return time.Time{}, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document.UpdatedAt").WithPath(MetadataUpdated).WithMessage(fmt.Sprintf("cannot convert %T to time.Time", val))
	}
	return updatedAt, nil
}

// GetMetadataString returns a metadata value as a string.
func (d *Document) GetMetadataString(key string) (string, error) {
	val, err := d.GetMetadataValue(key)
	if err != nil {
		return "", err
	}
	str, ok := utils.CoerceToPrimitiveValue[string](val)
	if !ok {
		return "", common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document.GetMetadataString").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to string", val))
	}
	return str, nil
}

// GetMetadataInt returns a metadata value as an int.
func (d *Document) GetMetadataInt(key string) (int, error) {
	val, err := d.GetMetadataValue(key)
	if err != nil {
		return 0, err
	}
	num, ok := utils.CoerceToPrimitiveValue[int](val)
	if !ok {
		return 0, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document.GetMetadataInt").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to int", val))
	}
	return num, nil
}

// GetMetadataFloat returns a metadata value as a float64.
func (d *Document) GetMetadataFloat(key string) (float64, error) {
	val, err := d.GetMetadataValue(key)
	if err != nil {
		return 0, err
	}
	num, ok := utils.CoerceToPrimitiveValue[float64](val)
	if !ok {
		return 0, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document.GetMetadataFloat").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to float64", val))
	}
	return num, nil
}

// GetMetadataBool returns a metadata value as a bool.
func (d *Document) GetMetadataBool(key string) (bool, error) {
	val, err := d.GetMetadataValue(key)
	if err != nil {
		return false, err
	}
	boolean, ok := utils.CoerceToPrimitiveValue[bool](val)
	if !ok {
		return false, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document.GetMetadataBool").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to bool", val))
	}
	return boolean, nil
}

// GetMetadataTime returns a metadata value as a time.Time.
func (d *Document) GetMetadataTime(key string) (time.Time, error) {
	val, err := d.GetMetadataValue(key)
	if err != nil {
		return time.Time{}, err
	}
	t, ok := utils.CoerceTime(val)
	if !ok {
		return time.Time{}, common.SystemErrorFrom(ErrTypeConversion).WithOperation("data.Document.GetMetadataTime").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to time.Time", val))
	}
	return t, nil
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
