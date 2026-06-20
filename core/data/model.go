package data

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// UUID Pool for reduced allocations
// ============================================================================

var uuidPool = sync.Pool{
	New: func() any {
		return new(uuid.UUID)
	},
}

// ============================================================================
// DocumentModelProvider
// ============================================================================

// DocumentModelProvider allows generic constraints to match any struct
// that embeds DocumentModel.
type DocumentModelProvider interface {
	Model() *DocumentModel
}

// ============================================================================
// DocumentModel
// ============================================================================

// DocumentModel provides system fields that can be embedded in domain structs.
//
// Usage:
//
//	type Product struct {
//	    data.DocumentModel
//	    Name  string  `doc:"name"`
//	    Price float64 `doc:"price"`
//	}
//
//	// Auto-initialize with New()
//	product := data.New(Product{Name: "Laptop", Price: 999.99})
//	fmt.Println(product.ID)  // Auto-generated ID
//
//	// Or use pointer
//	product := data.New(&Product{Name: "Laptop", Price: 999.99})
type DocumentModel struct {
	ID       string         `json:"id,omitempty" doc:"_id_,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty" doc:"_metadata_,omitempty"`
}

// ============================================================================
// Auto-Initialization (Optimized)
// ============================================================================

// New initializes a model with auto-generated ID and metadata.
// Returns the same type that was passed in (value or pointer).
//
// Example:
//
//	product := data.New(Product{Name: "Laptop", Price: 999.99})
//	fmt.Println(product.ID) // Auto-generated
//
//	// Works with pointers too
//	product := data.New(&Product{Name: "Laptop", Price: 999.99})
func New[T any](model T) T {
	// Use reflection to find and initialize DocumentModel
	initializeModel(&model)
	return model
}

// initializeModel finds the embedded DocumentModel and initializes it
func initializeModel(model any) {
	v := reflect.ValueOf(model)

	// Handle pointer
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	// Use cached type metadata if available
	t := v.Type()

	// Fast path: check cache for DocumentModel field location
	if fieldIndex, ok := getDocumentModelFieldIndex(t); ok {
		if fieldIndex >= 0 {
			field := v.Field(fieldIndex)
			if field.CanSet() {
				dm := field.Addr().Interface().(*DocumentModel)
				initDocumentModelFast(dm)
			}
		}
		return
	}

	// Slow path: find and cache DocumentModel field
	fieldIndex := findAndCacheDocumentModelField(t, v)
	if fieldIndex >= 0 {
		field := v.Field(fieldIndex)
		if field.CanSet() {
			dm := field.Addr().Interface().(*DocumentModel)
			initDocumentModelFast(dm)
		}
	}
}

// ============================================================================
// DocumentModel Field Cache
// ============================================================================

var (
	dmFieldCacheMu sync.RWMutex
	dmFieldCache   = make(map[reflect.Type]int, 32)
)

func getDocumentModelFieldIndex(t reflect.Type) (int, bool) {
	dmFieldCacheMu.RLock()
	idx, ok := dmFieldCache[t]
	dmFieldCacheMu.RUnlock()
	return idx, ok
}

func cacheDocumentModelFieldIndex(t reflect.Type, idx int) {
	dmFieldCacheMu.Lock()
	if len(dmFieldCache) < 1024 { // Prevent unbounded growth
		dmFieldCache[t] = idx
	}
	dmFieldCacheMu.Unlock()
}

func findAndCacheDocumentModelField(t reflect.Type, v reflect.Value) int {
	documentModelType := reflect.TypeOf(DocumentModel{})

	for i := 0; i < t.NumField(); i++ {
		fieldType := t.Field(i)

		// Direct match or anonymous embedding
		if fieldType.Type == documentModelType {
			cacheDocumentModelFieldIndex(t, i)
			return i
		}

		if fieldType.Anonymous && fieldType.Type == documentModelType {
			cacheDocumentModelFieldIndex(t, i)
			return i
		}
	}

	// Not found - cache -1 to avoid future searches
	cacheDocumentModelFieldIndex(t, -1)
	return -1
}

// ============================================================================
// Fast Initialization
// ============================================================================

// Pre-allocated string builders for timestamp formatting
var tsBuilderPool = sync.Pool{
	New: func() any {
		return new(strings.Builder)
	},
}

// initDocumentModelFast initializes a DocumentModel with optimized allocations
func initDocumentModelFast(dm *DocumentModel) {
	// Generate ID if not set
	if dm.ID == "" {
		// Use pooled UUID for reduced allocations
		id := uuidPool.Get().(*uuid.UUID)
		*id = uuid.Must(uuid.NewV7())
		dm.ID = formatUUIDNoDashes(id)
		uuidPool.Put(id)
	}

	// Initialize metadata if not set
	if dm.Metadata == nil {
		dm.Metadata = make(map[string]any, 3) // Pre-size for 3 standard fields
	}

	// Set timestamps if not present
	now := time.Now().UnixNano()

	// Use string builder pool for timestamp formatting
	if _, ok := dm.Metadata[MetadataCreated]; !ok {
		dm.Metadata[MetadataCreated] = formatUnixNano(now)
	}
	if _, ok := dm.Metadata[MetadataUpdated]; !ok {
		dm.Metadata[MetadataUpdated] = formatUnixNano(now)
	}
	if _, ok := dm.Metadata[MetadataVersion]; !ok {
		dm.Metadata[MetadataVersion] = 1
	}
}

// formatUUIDNoDashes formats UUID without dashes (faster than strings.ReplaceAll)
func formatUUIDNoDashes(id *uuid.UUID) string {
	src := id.String() // 36 chars with dashes
	dst := make([]byte, 32)

	j := 0
	for i := 0; i < len(src); i++ {
		if src[i] != '-' {
			dst[j] = src[i]
			j++
		}
	}

	return string(dst)
}

// formatUnixNano converts nanoseconds to string (faster than fmt.Sprintf)
func formatUnixNano(nanos int64) string {
	return strconv.FormatInt(nanos, 10)
}

// ============================================================================
// DocumentModel Helper Methods
// ============================================================================

// GetID returns the document ID
func (dm *DocumentModel) GetID() string {
	return dm.ID
}

// GetMetadata returns the metadata map
func (dm *DocumentModel) GetMetadata() map[string]any {
	if dm.Metadata == nil {
		return make(map[string]any)
	}
	return dm.Metadata
}

// SetMetadata sets the metadata map
func (dm *DocumentModel) SetMetadata(metadata map[string]any) {
	dm.Metadata = metadata
}

// GetMetadataValue retrieves a value from metadata
func (dm *DocumentModel) GetMetadataValue(key string) (any, bool) {
	if dm.Metadata == nil {
		return nil, false
	}
	val, ok := dm.Metadata[key]
	return val, ok
}

// SetMetadataValue sets a value in metadata
func (dm *DocumentModel) SetMetadataValue(key string, value any) {
	if dm.Metadata == nil {
		dm.Metadata = make(map[string]any, 4)
	}
	dm.Metadata[key] = value
}

// Version returns the document version from metadata
func (dm *DocumentModel) Version() (int, bool) {
	if dm.Metadata == nil {
		return 0, false
	}

	v, ok := dm.Metadata[MetadataVersion]
	if !ok {
		return 0, false
	}

	// Fast path for common types
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case int32:
		return int(val), true
	default:
		return 0, false
	}
}

// CreatedAt returns the creation timestamp from metadata
func (dm *DocumentModel) CreatedAt() (time.Time, bool) {
	return dm.getTimestamp(MetadataCreated)
}

// UpdatedAt returns the last update timestamp from metadata
func (dm *DocumentModel) UpdatedAt() (time.Time, bool) {
	return dm.getTimestamp(MetadataUpdated)
}

// getTimestamp is a helper to extract timestamps from metadata
func (dm *DocumentModel) getTimestamp(key string) (time.Time, bool) {
	if dm.Metadata == nil {
		return time.Time{}, false
	}

	v, ok := dm.Metadata[key]
	if !ok {
		return time.Time{}, false
	}

	// Fast path for time.Time
	if t, ok := v.(time.Time); ok {
		return t, true
	}

	// Fast path for string (Unix nano)
	if ts, ok := v.(string); ok {
		if t, parseOk := parseUnixNanoFast(ts); parseOk {
			return t, true
		}
	}

	// Fallback for int64
	if nanos, ok := v.(int64); ok {
		return time.Unix(0, nanos), true
	}

	return time.Time{}, false
}

// Model returns the underlying document model
func (dm *DocumentModel) Model() *DocumentModel {
	return dm
}

// ============================================================================
// Fast Timestamp Parsing
// ============================================================================

// parseUnixNanoFast parses a Unix nanosecond timestamp string
// Optimized to avoid fmt.Sscanf overhead
func parseUnixNanoFast(s string) (time.Time, bool) {
	nanos, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(0, nanos), true
}
