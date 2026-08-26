package document

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/data"
)

// init registers document.DocumentModel with data's system-model registry so
// struct binding, struct extraction, and DTO schema generation treat the embed
// exactly like data.DocumentModel.
func init() {
	data.RegisterSystemModelType(reflect.TypeFor[DocumentModel](), linkDocumentModelParent)
}

func linkDocumentModelParent(embed any, parent any) {
	if dm, ok := embed.(*DocumentModel); ok {
		dm.parent = parent
	}
}

// ErrModelNotInitialized is returned when a method on DocumentModel is called
// before the enclosing struct has been initialized via document.New[T].
var ErrModelNotInitialized = fmt.Errorf("document: model not initialized — call document.New[T] before calling this method")

// DocumentModel provides container-backed system fields that can be embedded in
// domain structs. It is a drop-in alternative to data.DocumentModel for models
// that build documents through document.DocumentPool: promoted Document/Patch
// methods materialize container-backed documents from the DTO schema of the
// enclosing struct's type.
type DocumentModel struct {
	ID       string         `json:"_id_,omitempty" anansi:"_id_,required=true,omitempty"`
	Metadata map[string]any `json:"_metadata_,omitempty" anansi:"_metadata_,required=true,omitempty"`
	parent   any            `anansi:"-"` // *T set by New, restored by the data binder
}

// GetID returns the model's document identifier.
func (dm *DocumentModel) GetID() string {
	return dm.ID
}

// Document builds a fully initialized container-backed document from the
// enclosing struct via the DTO schema of its type. Requires the model to have
// been initialized via document.New[T]; returns ErrModelNotInitialized
// otherwise.
func (dm *DocumentModel) Document() (*Document, error) {
	if dm.parent == nil {
		return nil, ErrModelNotInitialized
	}
	col, err := poolFromTypeOf(dm.parent)
	if err != nil {
		return nil, err
	}
	return col.FromStruct(dm.parent)
}

// MustDocument calls Document and panics on error.
func (dm *DocumentModel) MustDocument() *Document {
	doc, err := dm.Document()
	if err != nil {
		panic(fmt.Sprintf("DocumentModel.MustDocument: %v", err))
	}
	return doc
}

// Patch builds a partial document from the enclosing struct's non-zero user
// fields. System fields and zero-valued fields are skipped. Requires the model
// to have been initialized via document.New[T].
func (dm *DocumentModel) Patch() (*Document, error) {
	if dm.parent == nil {
		return nil, ErrModelNotInitialized
	}
	col, err := poolFromTypeOf(dm.parent)
	if err != nil {
		return nil, err
	}
	return col.FromPartialStruct(dm.parent)
}

// Len returns the number of non-zero user data fields on the enclosing struct,
// excluding system fields. A Len of 0 indicates the model carries no update
// payload. Returns 0 if the model is not initialized.
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

// New initializes a model with an auto-generated ID, metadata defaults, and a
// parent reference. The parent reference enables promoted methods like
// Document() and Patch() to access the outer struct's fields.
//
// model must be a non-nil pointer to a struct that embeds DocumentModel.
// Returns the same pointer for chaining.
//
//	product := document.New(&Product{Name: "Widget"})
//	doc, _ := product.Document() // promoted to DocumentModel.Document()
func New[T any](model *T) *T {
	if model == nil {
		return nil
	}
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return model
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return model
	}
	if dm, ok := findEmbeddedDocumentModel(v); ok {
		dm.parent = any(model)
		initDocumentModel(dm)
	}
	return model
}

// findEmbeddedDocumentModel locates the embedded *DocumentModel within v.
func findEmbeddedDocumentModel(v reflect.Value) (*DocumentModel, bool) {
	idx, ok := findEmbeddedIndex(v.Type(), reflect.TypeFor[DocumentModel]())
	if !ok {
		return nil, false
	}
	fv := v.FieldByIndex(idx)
	if !fv.CanAddr() {
		return nil, false
	}
	dm, ok := fv.Addr().Interface().(*DocumentModel)
	return dm, ok
}

// findEmbeddedIndex returns the field index path of the first anonymous embed
// of type target within t, or (nil, false).
func findEmbeddedIndex(t reflect.Type, target reflect.Type) ([]int, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.Anonymous {
			continue
		}
		if f.Type == target {
			return []int{i}, true
		}
		if f.Type.Kind() == reflect.Struct {
			if sub, ok := findEmbeddedIndex(f.Type, target); ok {
				return append([]int{i}, sub...), true
			}
		}
	}
	return nil, false
}

func initDocumentModel(dm *DocumentModel) {
	// Honor an outer shadow _id_ the caller pre-set on the struct: it becomes
	// the model's document identifier.
	if shadow, ok := outerShadowField(dm.parent, data.DocumentIDField); ok && shadow.Kind() == reflect.String {
		if s := shadow.String(); s != "" {
			dm.ID = s
		}
	}
	if dm.ID == "" {
		dm.ID = newUUID()
	}
	// Sync the (possibly generated) identifier into the outer shadow field so
	// the struct's own ID mirrors the embedded model's _id_.
	if shadow, ok := outerShadowField(dm.parent, data.DocumentIDField); ok && shadow.Kind() == reflect.String {
		shadow.SetString(dm.ID)
	}
	if dm.Metadata == nil {
		dm.Metadata = make(map[string]any, 3)
	}
	now := time.Now().UnixNano()
	if _, ok := dm.Metadata[data.MetadataCreated]; !ok {
		dm.Metadata[data.MetadataCreated] = strconv.FormatInt(now, 10)
	}
	if _, ok := dm.Metadata[data.MetadataUpdated]; !ok {
		dm.Metadata[data.MetadataUpdated] = strconv.FormatInt(now, 10)
	}
	if _, ok := dm.Metadata[data.MetadataVersion]; !ok {
		dm.Metadata[data.MetadataVersion] = 1
	}
}

// outerShadowField returns the settable reflect.Value of the enclosing
// struct's non-anonymous field whose anansi tag names the reserved system
// field name (e.g. "_id_" or "_metadata_"). Generated models declare such
// fields to expose customizable shadow copies of the embedded system model's
// fields; the embedded model keeps them in sync.
func outerShadowField(parent any, name string) (reflect.Value, bool) {
	if parent == nil {
		return reflect.Value{}, false
	}
	v := reflect.ValueOf(parent)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if f.Anonymous {
			continue
		}
		tag := f.Tag.Get(data.AnansiTag)
		tagName := tag
		if idx := strings.IndexByte(tag, ','); idx >= 0 {
			tagName = tag[:idx]
		}
		if tagName == name {
			fv := v.Field(i)
			if fv.CanSet() {
				return fv, true
			}
		}
	}
	return reflect.Value{}, false
}

// ============================================================================
// Schema-Bound DocumentPool per Model Type
// ============================================================================

var modelPoolCache sync.Map // reflect.Type -> *DocumentPool

// poolFromTypeOf returns the lazily built, cached DocumentPool for the
// struct type of ptr, deriving its schema from the type via DTO extraction.
func poolFromTypeOf(ptr any) (*DocumentPool, error) {
	return poolFromType(reflect.TypeOf(ptr))
}

func poolFromType(t reflect.Type) (*DocumentPool, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if v, ok := modelPoolCache.Load(t); ok {
		return v.(*DocumentPool), nil
	}
	schemaBytes, err := data.ExtractDTOSchemaDirect(reflect.New(t).Interface())
	if err != nil {
		return nil, err
	}
	col, err := NewDocumentPoolFromJSON(schemaBytes)
	if err != nil {
		return nil, err
	}
	actual, _ := modelPoolCache.LoadOrStore(t, col)
	return actual.(*DocumentPool), nil
}

// DocumentPoolFromType returns the schema-bound DocumentPool for the given
// model type, deriving its schema from the type via DTO extraction and caching
// it. The pool is shared by every model instance of the type, so documents
// built through it share the container pool.
func DocumentPoolFromType[T any]() (*DocumentPool, error) {
	var zero T
	return poolFromType(reflect.TypeOf(&zero))
}

// MustDocumentPoolFromType is DocumentPoolFromType but panics on error.
func MustDocumentPoolFromType[T any]() *DocumentPool {
	col, err := DocumentPoolFromType[T]()
	if err != nil {
		panic(fmt.Sprintf("document.MustDocumentPoolFromType[%T]: %v", *new(T), err))
	}
	return col
}
