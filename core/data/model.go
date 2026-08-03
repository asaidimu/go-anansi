package data

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// UUID Pool
// ============================================================================

var uuidPool = sync.Pool{
	New: func() any {
		return new(uuid.UUID)
	},
}

// ============================================================================
// Interfaces & Models
// ============================================================================

// DocumentModelProvider allows generic constraints to match any struct
// that embeds DocumentModel.
type DocumentModelProvider interface {
	Model() *DocumentModel
}

// ErrModelNotInitialized is returned when a method on DocumentModel is called
// before the struct has been initialized via Model[T].
var ErrModelNotInitialized = fmt.Errorf("data: model not initialized — call data.New[T] before calling this method")

// DocumentModel provides system fields that can be embedded in domain structs.
type DocumentModel struct {
	ID       string         `json:"id,omitempty" anansi:"_id_,required=true,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty" anansi:"_metadata_,required=true,omitempty"`
	parent   any            `anansi:"-"` // *T set by Model(), skipped by schema/binding
}

func (dm *DocumentModel) Model() *DocumentModel {
	return dm
}

// ============================================================================
// Auto-Initialization
// ============================================================================

// New initializes a model with auto-generated ID, metadata, and parent
// reference. The parent reference enables promoted methods like Document()
// and Patch() to access the outer struct's fields.
//
// model must be a non-nil pointer to a struct that embeds DocumentModel.
// Returns the same pointer for chaining.
//
//	product := data.New(&Product{Name: "Widget"})
//	doc, _ := product.Document()  // promoted to DocumentModel.Document()
func New[T any](model *T) *T {
	if model == nil {
		return nil
	}

	// Interface fast-path
	if provider, ok := any(model).(DocumentModelProvider); ok {
		if dm := provider.Model(); dm != nil {
			dm.parent = any(model)
			initDocumentModelFast(dm)
			return model
		}
	}

	// Reflection fallback
	initializeModelReflect(any(model))
	return model
}

func initializeModelReflect(model any) {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}

	// Leverage shared type cache from bind.go
	fields, sysErr := getTypeInfo(v.Type(), AnansiTag)
	if sysErr != nil {
		return
	}

	// Find system embed field path. Only data.DocumentModel is initialized here —
	// the assertion targets *DocumentModel specifically; other registered system
	// models (e.g. document.DocumentModel) have their own New entry point.
	for i := range fields {
		if fields[i].IsSystemEmbed && fields[i].StructField.Type == docModelType {
			fv := v.FieldByIndex(fields[i].Index)
			if fv.CanAddr() {
				dm := fv.Addr().Interface().(*DocumentModel)
				dm.parent = model
				initDocumentModelFast(dm)
				return
			}
		}
	}
}

// ============================================================================
// Fast Initialization Helpers
// ============================================================================

func initDocumentModelFast(dm *DocumentModel) {
	if dm.ID == "" {
		id := uuidPool.Get().(*uuid.UUID)
		*id = uuid.Must(uuid.NewV7())
		dm.ID = formatUUIDNoDashes(id)
		uuidPool.Put(id)
	}

	if dm.Metadata == nil {
		dm.Metadata = make(map[string]any, 3)
	}

	now := time.Now().UnixNano()
	if _, ok := dm.Metadata[MetadataCreated]; !ok {
		dm.Metadata[MetadataCreated] = strconv.FormatInt(now, 10)
	}
	if _, ok := dm.Metadata[MetadataUpdated]; !ok {
		dm.Metadata[MetadataUpdated] = strconv.FormatInt(now, 10)
	}
	if _, ok := dm.Metadata[MetadataVersion]; !ok {
		dm.Metadata[MetadataVersion] = 1
	}
}

// Document converts the outer struct to a full Document via the factory
// pipeline (generates ID, metadata, hash). Requires the model to have been
// initialized via data.New[T]; returns ErrModelNotInitialized otherwise.
func (dm *DocumentModel) Document() (*Document, error) {
	if dm.parent == nil {
		return nil, ErrModelNotInitialized
	}
	return NewDocumentFromStruct(dm.parent)
}

// MustDocument calls Document and panics on error.
func (dm *DocumentModel) MustDocument() *Document {
	doc, err := dm.Document()
	if err != nil {
		panic(fmt.Sprintf("DocumentModel.MustDocument: %v", err))
	}
	return doc
}

// Patch converts the outer struct to a partial Document suitable for partial
// updates. System fields and zero-valued fields are skipped.
func (dm *DocumentModel) Patch() (*Document, error) {
	if dm.parent == nil {
		return nil, ErrModelNotInitialized
	}
	return NewPartialDocumentFromStruct(dm.parent)
}

// Len returns the number of non-zero user data fields set on the outer
// struct, excluding system fields (_id, _metadata_). A Len of 0 indicates
// the model carries no update payload. Requires the model to have been
// initialized via data.New[T]; returns 0 otherwise.
func (dm *DocumentModel) Len() int {
	if dm.parent == nil {
		return 0
	}
	patch, err := dm.Patch()
	if err != nil {
		return 0
	}
	return patch.Len()
}

// formatUUIDNoDashes converts UUID to 32-char hex string in 1 allocation
func formatUUIDNoDashes(id *uuid.UUID) string {
	return hex.EncodeToString(id[:])
}

// ============================================================================
// Accessors & Utilities
// ============================================================================

func (dm *DocumentModel) GetID() string {
	return dm.ID
}

func (dm *DocumentModel) GetMetadata() map[string]any {
	if dm.Metadata == nil {
		return make(map[string]any)
	}
	return dm.Metadata
}

func (dm *DocumentModel) SetMetadata(metadata map[string]any) {
	dm.Metadata = metadata
}

func (dm *DocumentModel) GetMetadataValue(key string) (any, bool) {
	if dm.Metadata == nil {
		return nil, false
	}
	val, ok := dm.Metadata[key]
	return val, ok
}

func (dm *DocumentModel) SetMetadataValue(key string, value any) {
	if dm.Metadata == nil {
		dm.Metadata = make(map[string]any, 4)
	}
	dm.Metadata[key] = value
}

func (dm *DocumentModel) Version() (int, bool) {
	if dm.Metadata == nil {
		return 0, false
	}

	v, ok := dm.Metadata[MetadataVersion]
	if !ok {
		return 0, false
	}

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

func (dm *DocumentModel) CreatedAt() (time.Time, bool) {
	return dm.getTimestamp(MetadataCreated)
}

func (dm *DocumentModel) UpdatedAt() (time.Time, bool) {
	return dm.getTimestamp(MetadataUpdated)
}

func (dm *DocumentModel) getTimestamp(key string) (time.Time, bool) {
	if dm.Metadata == nil {
		return time.Time{}, false
	}

	v, ok := dm.Metadata[key]
	if !ok {
		return time.Time{}, false
	}

	if t, ok := v.(time.Time); ok {
		return t, true
	}

	if ts, ok := v.(string); ok {
		if t, parseOk := parseUnixNanoFast(ts); parseOk {
			return t, true
		}
	}

	if nanos, ok := v.(int64); ok {
		return time.Unix(0, nanos), true
	}

	return time.Time{}, false
}

func parseUnixNanoFast(s string) (time.Time, bool) {
	nanos, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(0, nanos), true
}
