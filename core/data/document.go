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

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/utils"
)

// Document represents a flexible, schema-aware data structure.
//
// A Document consists of three distinct parts:
//
//  1. ID (string): Immutable system-generated identifier
//     - Access via: ID()
//     - Cannot be modified after creation
//
//  2. Data (map[string]any): User-managed fields
//     - Access via: Get(key), Set(key, value), GetString(key), etc.
//     - This is YOUR data - all Get/Set operations work on this
//
//  3. Metadata (map[string]any): System and custom metadata
//     - Access via: Metadata(), SetMetadataValue(key, value)
//     - Contains: version, timestamps, checksums, signatures
//     - Reserved fields are managed by the system
//
// # Serialization vs. Access
//
// ToMap() returns a complete serializable representation including
// _id and _metadata_ fields for persistence. However, Get/Set operations
// work ONLY on the user data portion:
//
//	doc := MustNewDocument(map[string]any{"name": "John"})
//
//	// Access user data
//	name, _ := doc.Get("name")           // ✓ Works
//	doc.Set("age", 30)                   // ✓ Works
//
//	// Access system fields via dedicated methods
//	id := doc.ID()                       // ✓ Correct way
//	version, _ := doc.Version()          // ✓ Correct way
//
//	// Get/Set do NOT work on system fields
//	id, _ := doc.Get("_id")              // ✗ Error: key not found
//	doc.Set("_id", "custom")             // ✗ Does nothing (not in user data)
//
//	// Serialization includes everything
//	m := doc.ToMap()                     // ✓ Contains _id, _metadata_, name, age
type Document struct {
	id       string
	ctx      context.Context
	data     map[string]any
	metadata map[string]any
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
		return v.ToMap(), nil
	case Document:
		return v.ToMap(), nil
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

// Get retrieves a value from the document's user data with detailed error information.
//
// Only user-defined fields are accessible via Get. To access system fields:
//   - Use ID() for the document identifier
//   - Use Metadata() or specific methods like Version() for metadata
//
// Example:
//
//	doc := MustNewDocument(map[string]any{"name": "John"})
//
//	name, err := doc.Get("name")  // ✓ Works - user data
//	id, err := doc.Get("_id")     // ✗ Error - use ID() instead
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

// Set updates a value in the document's user data.
//
// Only user-defined fields can be set. System fields (_id, _metadata_)
// are not accessible via Set:
//   - _id is immutable after document creation
//   - Use SetMetadataValue() to add custom metadata fields
//
// Example:
//
//	doc.Set("name", "John")              // ✓ Works
//	doc.Set("_id", "custom")             // ✗ Does nothing - not in user data
//	doc.SetMetadataValue("author", "me") // ✓ Correct way for metadata
func (d *Document) Set(key string, value any) error {
	return d.SetNested(key, value)
}

// Unset removes a key from the document's user data.
// System-managed fields (_id_, _metadata_) cannot be removed.
func (d *Document) Unset(key string) {
	if ReservedSystemField(key) {
		return
	}
	delete(d.data, key)
}

// SetIfNotExists sets a value only if the key doesn't exist.
func (d *Document) SetIfNotExists(key string, value any) bool {
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

// Keys returns all user data keys sorted alphabetically.
// System fields (_id, _metadata_) are NOT included.
//
// To access all fields including system fields, use ToMap().
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

// Values returns all user data values in key-sorted order.
// System fields are NOT included.
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
		id:       d.id,
		ctx:      d.ctx,
		data:     deepCloneValue(d.data).(map[string]any),
		metadata: deepCloneValue(d.metadata).(map[string]any),
	}
}

// deepCloneValue recursively clones a value, handling nested Documents, maps (map[string]any),
// and slices ([]any, []Document). Primitive types are returned as is.
func deepCloneValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case *Document:
		return val.Clone()
	case Document:
		return val.Clone()
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

// Len returns the number of user data fields in the document.
// System fields (_id, _metadata_) are NOT counted.
func (d *Document) Len() int {
	if d == nil || d.data == nil {
		return 0
	}
	return len(d.data)
}

// IsEmpty checks if the document has no user data fields.
// System fields (_id, _metadata_) are NOT considered.
func (d *Document) IsEmpty() bool {
	return d == nil || len(d.data) == 0
}

// HasKey checks if a key exists in the document's user data.
// System fields (_id, _metadata_) will return false.
// Use ID() and Metadata() to check for system fields.
func (d *Document) HasKey(key string) bool {
	if d == nil || d.data == nil {
		return false
	}
	_, ok := d.data[key]
	return ok
}

// HasPath checks if a path exists in the document's user data (supports dot notation).
// System fields are NOT accessible via paths.
func (d *Document) HasPath(keyOrPath string) bool {
	if d == nil || d.data == nil {
		return false
	}
	_, ok := utils.GetValueByPath(d.data, keyOrPath)
	return ok
}

// Is performs deep equality comparison on two documents, including ID and metadata.
func (d *Document) Is(other *Document) bool {
	if d == nil && other == nil {
		return true
	}
	if d == nil || other == nil {
		return false
	}
	return d.id == other.id &&
		reflect.DeepEqual(d.data, other.data) &&
		reflect.DeepEqual(d.metadata, other.metadata)
}

// Equals performs content-only deep equality comparison on the user data,
// ignoring auto-generated IDs and metadata.
func (d *Document) Equals(other *Document) bool {
	if d == nil && other == nil {
		return true
	}
	if d == nil || other == nil {
		return false
	}
	return reflect.DeepEqual(d.data, other.data)
}

// ToMap returns a complete map representation of the document suitable
// for serialization and persistence.
//
// The returned map includes:
//   - "_id": the document's unique identifier
//   - "_metadata_": system and custom metadata
//   - All user-defined fields
//
// Note: This method is for serialization. To work with document data,
// use Get/Set for user fields and dedicated methods for system fields.
//
// Example:
//
//	doc := MustNewDocument(map[string]any{"name": "John"})
//
//	m := doc.ToMap()
//	// m = {
//	//   "_id": "abc123...",
//	//   "_metadata_": {...},
//	//   "name": "John"
//	// }
//
//	json.Marshal(m)  // Serialize complete document
func (d *Document) ToMap() map[string]any {
	if d == nil {
		return nil
	}

	result := make(map[string]any, len(d.data)+2)
	if len(d.id) > 0 {
		result[DocumentIDField] = d.id
	}
	if d.metadata != nil {
		result[MetadataField] = d.metadata
	}

	for k, v := range d.data {
		result[k] = asMapValue(v)
	}
	return result
}

// Data returns a copy of the document's user data only, excluding
// system fields (_id and _metadata_).
//
// Use this when you need just the business data without system fields.
//
// Example:
//
//	doc := MustNewDocument(map[string]any{"name": "John", "age": 30})
//
//	data := doc.Data()
//	// data = {"name": "John", "age": 30}
//	// No _id or _metadata_ included
func (d *Document) Data() map[string]any {
	if d == nil || d.data == nil {
		return make(map[string]any)
	}
	result := make(map[string]any, len(d.data))
	for k, v := range d.data {
		result[k] = asMapValue(v)
	}
	return result
}

// asMapValue recursively converts a value to its map[string]any representation.
func asMapValue(v any) any {
	switch val := v.(type) {
	case *Document:
		return val.ToMap() // Recursively call ToMap on nested Document structs
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

// ID returns the document's unique identifier.
//
// The ID is automatically generated during document creation and is immutable.
// It cannot be changed after the document is created.
func (d *Document) ID() string {
	if d == nil {
		return ""
	}
	return d.id
}

// Metadata returns a copy of the document's metadata map.
//
// The metadata includes both system-managed fields (version, timestamps,
// checksums) and custom fields set via SetMetadataValue().
//
// System-managed fields:
//   - _version: document version number
//   - _created: creation timestamp
//   - _updated: last update timestamp
//   - _checksum: HMAC-SHA256 hash
//   - _signature: RSA signature (if signed)
func (d *Document) Metadata() map[string]any {
	if d == nil || d.metadata == nil {
		return make(map[string]any)
	}
	result := make(map[string]any, len(d.metadata))
	maps.Copy(result, d.metadata)
	return result
}

// SetMetadata replaces the entire metadata map.
// This is primarily used internally by the factory.
// For adding custom metadata, use SetMetadataValue() instead.
func (d *Document) SetMetadata(metadata map[string]any) {
	if d.metadata == nil {
		d.metadata = make(map[string]any)
	} else {
		// Clear existing metadata
		for k := range d.metadata {
			delete(d.metadata, k)
		}
	}
	maps.Copy(d.metadata, metadata)
}

// StripMetadata removes metadata and returns a clean copy.
func (d *Document) StripMetadata() *Document {
	return &Document{
		id:   d.id,
		ctx:  d.ctx,
		data: deepCloneValue(d.data).(map[string]any),
		// metadata intentionally omitted
	}
}

// --- Data Integrity ---

// Hash computes and sets the HMAC-SHA256 hash of the metadata block.
func (d *Document) Hash() error {
	hash, err := getFactory().calculateHash(d)
	if err != nil {
		return err
	}

	if d.metadata == nil {
		d.metadata = make(map[string]any)
	}
	d.metadata[MetadataChecksum] = hash
	return nil
}

// VerifyHash checks the integrity of the metadata block against its hash.
func (d *Document) VerifyHash() (bool, error) {
	if d.metadata == nil {
		return false, nil
	}

	providedHash, ok := d.metadata[MetadataChecksum].(string)
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

	if d.metadata == nil {
		d.metadata = make(map[string]any)
	}
	d.metadata[MetadataSignature] = signature
	return nil
}

// Verify checks the RSA signature of the document.
func (d *Document) Verify(publicKey *rsa.PublicKey) error {
	if d.metadata == nil {
		return common.SystemErrorFrom(ErrNoMetadata).WithOperation("data.Document.Verify")
	}

	signature, ok := d.metadata[MetadataSignature].(string)
	if !ok {
		return common.SystemErrorFrom(ErrSignatureInvalid).WithOperation("data.Document.Verify").WithMessage("no signature found in metadata")
	}

	return getFactory().verifySignature(d, publicKey, signature)
}

// --- Metadata Accessors ---

// GetMetadataValue retrieves a value from the document's metadata map.
func (d *Document) GetMetadataValue(key string) (any, error) {
	if d.metadata == nil {
		return nil, common.SystemErrorFrom(ErrNoMetadata).WithOperation("data.Document.GetMetadataValue").WithPath(key)
	}
	val, ok := d.metadata[key]
	if !ok {
		return nil, common.SystemErrorFrom(ErrKeyNotFound).WithOperation("data.Document.GetMetadataValue").WithPath(key)
	}
	return val, nil
}

// SetMetadataValue sets a custom metadata field.
//
// Reserved system fields (_version, _created, _updated, _checksum, _signature)
// cannot be set manually and will return an error.
//
// Example:
//
//	doc.SetMetadataValue("author", "John Doe")
//	doc.SetMetadataValue("department", "Engineering")
func (d *Document) SetMetadataValue(key string, value any) error {
	switch key {
	case MetadataChecksum, MetadataSignature, MetadataVersion, MetadataCreated, MetadataUpdated:
		return common.SystemErrorFrom(ErrReadOnlyField).WithOperation("data.Document.SetMetadataValue").WithPath(key).WithMessage("cannot overwrite system-managed metadata field")
	}

	if d.metadata == nil {
		d.metadata = make(map[string]any)
	}
	d.metadata[key] = value
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
