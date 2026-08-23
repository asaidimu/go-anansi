package document

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/data/container"
	canansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/utils"
	"github.com/google/uuid"
)

// Document is a schema-addressed, container-backed implementation of
// data.Documenter.
//
// Unlike the map-backed data.Document, user data lives in a
// *container.DataContainer addressed by the compiled schema: string keys and
// dotted paths resolve through definition.CompiledSchema (ResolvePath +
// Address) into flat DataContainerKeys. The schema carries the system fields
// (_id_, _metadata_) injected by Collection construction, so identity and
// metadata live in the same container as user data.
//
// The three parts mirror data.Document:
//
//  1. ID (string): immutable system identifier — access via ID().
//  2. Data: schema-declared user fields — access via Get/Set/SetNested.
//  3. Metadata: system and custom metadata — access via Metadata(),
//     GetMetadataValue/SetMetadataValue. Custom keys must be declared by a
//     metadata provider schema; undeclared keys error.
//
// Get/Set on keys absent from the schema error; the system fields (_id_,
// _metadata_) are read-only through Set.
//
// Compile-time assertion: the interface data.Documenter is fully implemented.
var _ data.Documenter = (*Document)(nil)

// Document is the schema-backed implementation of the Documenter interface.
type Document struct {
	cs  *definition.CompiledSchema // user-data compiled schema
	c   *container.DataContainer   // user-data storage (includes _id_, _metadata_)
	ctx context.Context

	// pool is the schema pool owning c. Root documents set it so Release can
	// return c to the pool; views (newNestedView, newNestedViewForChild) and
	// record views leave it nil so Release is a no-op.
	pool *container.Pool

	// prefix is the resolved path from the document root to this view's
	// object (empty for a root document). slotIdx is the schema slot against
	// which relative keys are resolved (0 for root).
	prefix  definition.ResolvedPath
	slotIdx uint8

	// record is non-nil for schema-free record views (nested record fields).
	record map[string]any
}

// ============================================================================
// Options
// ============================================================================

// Option configures a Document at construction time.
type Option func(*Document)

// WithContext sets the document's context.
func WithContext(ctx context.Context) Option {
	return func(d *Document) {
		if ctx != nil {
			d.ctx = ctx
		}
	}
}

// WithID overrides the auto-generated document identifier.
func WithID(id string) Option {
	return func(d *Document) {
		if id != "" {
			d.setID(id)
		}
	}
}

// WithMetadata replaces the document's metadata with the given map.
func WithMetadata(m map[string]any) Option {
	return func(d *Document) {
		if d == nil || d.cs == nil || d.c == nil {
			return
		}
		if m == nil {
			_ = d.clearMetadata()
			return
		}
		_ = d.populateMetadata(m)
	}
}

// ============================================================================
// Population helpers
// ============================================================================

// populateFromStruct writes the anansi-tagged fields of s into the container
// (or record view), honoring the reserved "_id_" and "_metadata_" fields.
func (d *Document) populateFromStruct(s any, partial bool) error {
	fields, err := data.StructFieldValues(s, partial)
	if err != nil {
		return err
	}
	for _, f := range fields {
		switch f.Path {
		case data.DocumentIDField:
			if s, ok := f.Value.(string); ok && isValidID(s) {
				d.setID(s)
			}
			continue
		case data.MetadataField:
			if m, ok := f.Value.(map[string]any); ok {
				if err := d.populateMetadata(m); err != nil {
					return err
				}
			}
			continue
		}
		if err := d.set(f.Path, f.Value); err != nil {
			return err
		}
	}
	return nil
}

// newRecordView builds a schema-free document view over a record map.
func newRecordView(m map[string]any, ctx context.Context) *Document {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Document{ctx: ctx, record: m}
}

// NewRecordView builds a schema-free document view over a record map.
//
// Unlike pool-built documents, a record view carries no compiled schema: keys
// never error (unknown paths are stored and returned as-is), typed getters
// coerce their stored values, and identity/metadata are ordinary map entries
// rather than dedicated slots. The persistence layer produces all egress
// documents this way, so rows that do not conform to the collection schema
// (joined projections, mismatched value types) still surface as
// document.Documents.
func NewRecordView(m map[string]any, ctx ...context.Context) *Document {
	var c context.Context
	if len(ctx) > 0 {
		c = ctx[0]
	}
	return newRecordView(m, c)
}

func (d *Document) isRecord() bool { return d.record != nil }

// ============================================================================
// Identity and Context
// ============================================================================

// ID returns the document's immutable identifier.
func (d *Document) ID() string {
	if d == nil {
		return ""
	}
	if d.isRecord() {
		if id, ok := d.record[data.DocumentIDField]; ok {
			if s, ok := id.(string); ok {
				return s
			}
		}
		return ""
	}
	if d.cs == nil || d.c == nil {
		return ""
	}
	rp, err := d.cs.ResolvePath(data.DocumentIDField)
	if err != nil {
		return ""
	}
	fd, ok := descriptorForStep(d.cs, rp[len(rp)-1])
	if !ok {
		return ""
	}
	k, err := computeLeafKey(d.cs, fd, rp)
	if err != nil {
		return ""
	}
	v, ok, err := d.c.GetString(k)
	if err != nil || !ok {
		return ""
	}
	return v
}

// setID writes the document identity into the container's _id_ field. It
// bypasses Set's system-field guard; empty identities are ignored so patches
// stay bare.
func (d *Document) setID(id string) {
	if d == nil || d.cs == nil || d.c == nil || d.isRecord() || id == "" {
		return
	}
	rp, _, err := d.resolvePath(data.DocumentIDField)
	if err != nil {
		return
	}
	last := rp[len(rp)-1]
	_ = setInto(d.cs, d.c, d.pool, last.SchemaIdx(), uint16(last.FieldIdx()), rp[:len(rp)-1], id)
}

// Context returns the document's context.
func (d *Document) Context() context.Context {
	if d == nil || d.ctx == nil {
		return context.Background()
	}
	return d.ctx
}

// WithContext returns a new Document with the provided context.
func (d *Document) WithContext(ctx context.Context) data.Documenter {
	clone := d.Clone().(*Document)
	if ctx != nil {
		clone.ctx = ctx
	}
	return clone
}

// ============================================================================
// Data Access
// ============================================================================

// Get retrieves a value at a key or dotted path. Keys absent from the schema,
// or schema-declared but unset values, return ErrKeyNotFound.
func (d *Document) Get(key string) (any, error) {
	if d == nil {
		return nil, d.keyErr(key)
	}
	if d.isRecord() {
		val, ok := utils.GetValueByPath(d.record, key)
		if !ok {
			return nil, d.keyErr(key)
		}
		return val, nil
	}
	if key == "" {
		return nil, d.keyEmptyErr()
	}
	rp, fd, err := d.resolvePath(key)
	if err != nil {
		return nil, err
	}

	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		child := fd.ChildSchemaIdx()
		switch fd.DataType() {
		case container.TypeRecord:
			k := internalKey(fd)
			if v, ok, err := d.c.GetRecord(k); err != nil {
				return nil, err
			} else if ok {
				return v, nil
			}
			return nil, d.keyErr(key)
		case container.TypeArrayObject:
			k := internalKey(fd)
			if children, ok, err := d.c.GetArrayObject(k); err != nil {
				return nil, err
			} else if ok {
				arr := make([]any, len(children))
				for i, ch := range children {
					m, err := materializeSlot(d.cs, ch, child, rp)
					if err != nil {
						return nil, err
					}
					arr[i] = m
				}
				return arr, nil
			}
			return nil, d.keyErr(key)
		default:
			present, err := anyDescendantPresent(d.cs, d.c, child, rp)
			if err != nil {
				return nil, err
			}
			if !present {
				return nil, d.keyErr(key)
			}
			return materializeSlot(d.cs, d.c, child, rp)
		}
	}

	k, err := computeLeafKey(d.cs, fd, rp)
	if err != nil {
		return nil, err
	}
	v, ok, err := getByType(d.c, fd.DataType(), k)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, d.keyErr(key)
	}
	if d.c.IsNull(k) {
		return nil, nil
	}
	return v, nil
}

// GetNested is an alias for Get with dotted-path support.
func (d *Document) GetNested(path string) (any, error) {
	return d.Get(path)
}

// GetOr returns a default value if the key is absent.
func (d *Document) GetOr(key string, defaultValue any) any {
	val, err := d.Get(key)
	if err != nil {
		return defaultValue
	}
	return val
}

// MustGet retrieves a value, panicking if absent.
func (d *Document) MustGet(key string) any {
	val, err := d.Get(key)
	if err != nil {
		panic(err)
	}
	return val
}

// Set writes a value at a key or dotted path. The key must resolve against the
// schema; system fields (_id_, _metadata_) are read-only.
func (d *Document) Set(key string, value any) error {
	return d.set(key, value)
}

func (d *Document) set(key string, value any) error {
	if d == nil {
		return d.keyErr(key)
	}
	if data.ReservedSystemField(key) {
		return d.readonlyErr(key)
	}
	if key == "" {
		return d.keyEmptyErr()
	}
	if d.isRecord() {
		return setValueByPath(d.record, key, value)
	}
	rp, _, err := d.resolvePath(key)
	if err != nil {
		return err
	}
	last := rp[len(rp)-1]
	return setInto(d.cs, d.c, d.pool, last.SchemaIdx(), uint16(last.FieldIdx()), rp[:len(rp)-1], value)
}

// SetNested writes a value at a dotted path.
func (d *Document) SetNested(path string, value any) error {
	return d.set(path, value)
}

// SetIfNotExists sets a value only if the key is absent. Returns true if set.
func (d *Document) SetIfNotExists(key string, value any) bool {
	if d.isRecord() {
		if _, ok := utils.GetValueByPath(d.record, key); ok {
			return false
		}
		_ = setValueByPath(d.record, key, value)
		return true
	}
	if d.HasKey(key) {
		return false
	}
	if err := d.Set(key, value); err != nil {
		return false
	}
	return true
}

// Unset removes a key or path. Unknown keys are a no-op.
func (d *Document) Unset(key string) {
	if d == nil || d.isRecord() {
		return
	}
	if data.ReservedSystemField(key) || key == "" {
		return
	}
	rp, fd, err := d.resolvePath(key)
	if err != nil {
		return
	}
	_ = unsetPath(d.cs, d.c, fd, rp)
}

// Delete removes a value at a path, returning an error for invalid paths.
func (d *Document) Delete(path string) error {
	if d.isRecord() {
		if path == "" {
			return d.keyEmptyErr()
		}
		return deleteValueByPath(d.record, path)
	}
	if data.ReservedSystemField(path) {
		return d.readonlyErr(path)
	}
	if path == "" {
		return d.keyEmptyErr()
	}
	rp, fd, err := d.resolvePath(path)
	if err != nil {
		return err
	}
	return unsetPath(d.cs, d.c, fd, rp)
}

// Keys returns the document's present top-level user-data keys, sorted.
// System fields (_id_, _metadata_) are excluded.
func (d *Document) Keys() []string {
	if d == nil {
		return []string{}
	}
	if d.isRecord() {
		keys := sortedMapKeys(d.record)
		filtered := keys[:0]
		for _, k := range keys {
			if data.ReservedSystemField(k) {
				continue
			}
			filtered = append(filtered, k)
		}
		return filtered
	}
	m, err := materializeSlot(d.cs, d.c, d.slotIdx, d.prefix)
	if err != nil {
		return []string{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if data.ReservedSystemField(k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Values returns the document's user-data values in key-sorted order.
func (d *Document) Values() []any {
	keys := d.Keys()
	values := make([]any, len(keys))
	for i, k := range keys {
		values[i] = d.GetOr(k, nil)
	}
	return values
}

// Len returns the number of present top-level user-data fields.
func (d *Document) Len() int {
	return len(d.Keys())
}

// IsEmpty reports whether the document has no user-data fields.
func (d *Document) IsEmpty() bool {
	return d == nil || d.Len() == 0
}

// HasKey reports whether a key exists in user data (system fields excluded).
func (d *Document) HasKey(key string) bool {
	if d == nil {
		return false
	}
	if d.isRecord() {
		_, ok := utils.GetValueByPath(d.record, key)
		return ok
	}
	if data.ReservedSystemField(key) || key == "" {
		return false
	}
	rp, fd, err := d.resolvePath(key)
	if err != nil {
		return false
	}
	ok, err := present(d.cs, d.c, fd, rp)
	if err != nil {
		return false
	}
	return ok
}

// HasPath reports whether a dotted path resolves to a present value.
func (d *Document) HasPath(keyOrPath string) bool {
	if d == nil {
		return false
	}
	if d.isRecord() {
		_, ok := utils.GetValueByPath(d.record, keyOrPath)
		return ok
	}
	_, err := d.Get(keyOrPath)
	return err == nil
}

// ============================================================================
// Serialization
// ============================================================================

// ToMap returns the full serializable representation: _id_, _metadata_, and
// user data. For root documents the system fields are ordinary schema fields
// in the container; views materialize their subtree, which excludes the
// root-level system fields.
func (d *Document) ToMap() map[string]any {
	if d == nil {
		return nil
	}
	if d.isRecord() {
		return deepCloneMap(d.record)
	}
	m, err := materializeSlot(d.cs, d.c, d.slotIdx, d.prefix)
	if err != nil {
		return map[string]any{}
	}
	return m
}

// Data returns the user-data map only, excluding system fields.
func (d *Document) Data() map[string]any {
	if d == nil {
		return map[string]any{}
	}
	if d.isRecord() {
		m := deepCloneMap(d.record)
		for k := range m {
			if data.ReservedSystemField(k) {
				delete(m, k)
			}
		}
		return m
	}
	m, err := materializeSlot(d.cs, d.c, d.slotIdx, d.prefix)
	if err != nil {
		return map[string]any{}
	}
	for k := range m {
		if data.ReservedSystemField(k) {
			delete(m, k)
		}
	}
	return m
}

// ============================================================================
// Serialization
// ============================================================================

// MarshalJSON implements json.Marshaler via the custom schema-driven codec.
// Root documents serialize directly from the container (keys ordered by field
// name); views and record views fall back to map materialization.
func (d *Document) MarshalJSON() ([]byte, error) {
	if d == nil {
		return []byte("null"), nil
	}
	if d.isRecord() {
		return json.Marshal(d.record)
	}
	if len(d.prefix) > 0 {
		return json.Marshal(d.ToMap())
	}
	return cjson.SerializeJSON(d.cs, d.c)
}

// SerializeField serializes the value at a dotted path directly from the
// container's typed slots — no intermediate map materialization — using the
// schema-driven stream serializer. Record views fall back to marshaling the
// materialized value. An absent path serializes as "null".
func (d *Document) SerializeField(path string) (string, error) {
	if d == nil {
		return "null", nil
	}
	if d.isRecord() || len(d.prefix) > 0 {
		v, err := d.Get(path)
		if err != nil {
			return "null", nil
		}
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return cjson.SerializeJSONPrefixString(d.cs, d.c, path)
}

// SerializeFieldString serializes the value at a dotted path directly from the
// container, reporting present=false when the field is absent so callers can
// store SQL NULL instead of a serialized "null".
func (d *Document) SerializeFieldString(path string) (string, bool, error) {
	if d == nil {
		return "null", false, nil
	}
	if d.isRecord() || len(d.prefix) > 0 {
		v, err := d.Get(path)
		if err != nil {
			return "", false, nil
		}
		b, err := json.Marshal(v)
		if err != nil {
			return "", false, err
		}
		return string(b), true, nil
	}
	rp, fd, err := d.resolvePath(path)
	if err != nil {
		return "", false, nil
	}
	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		present, err := present(d.cs, d.c, fd, rp)
		if err != nil {
			return "", false, err
		}
		if !present {
			return "", false, nil
		}
	}
	s, err := cjson.SerializeJSONPrefixString(d.cs, d.c, path)
	if err != nil {
		return "", false, err
	}
	if s == "null" {
		return "", false, nil
	}
	return s, true, nil
}

// UnmarshalJSON implements json.Unmarshaler. The receiver must already be
// bound to a Collection — documents are only ever constructed through a
// Collection, which supplies the compiled schema — so the input is decoded
// into its container via the custom codec, then identity metadata defaults are
// completed and the checksum computed.
func (d *Document) UnmarshalJSON(data []byte) error {
	if d == nil {
		return fmt.Errorf("document: cannot unmarshal into a nil document")
	}
	if d.cs == nil || d.c == nil {
		return fmt.Errorf("document: document is not bound to a schema")
	}
	if err := cjson.DecodeJSONInto(d.cs, data, d.c, d.pool); err != nil {
		return err
	}
	d.ensureMetadataDefaults()
	return d.finalizeMetadata()
}

// ToAnansi serializes the document via the Anansi binary wire format codec
// (core/encoding/anansi), mirroring MarshalJSON. Root documents encode
// directly from the container; views and record views are not backed by a
// single addressable container, so ToAnansi is unsupported for them (use
// MarshalJSON/ToMap instead). fullVersion is the schema version to embed in
// the packet header (see core/encoding/anansi's package doc); callers
// without a schema version registry can pass 0.
func (d *Document) ToAnansi(fullVersion uint16) ([]byte, error) {
	if d == nil {
		return nil, fmt.Errorf("document: cannot serialize a nil document")
	}
	if d.isRecord() || len(d.prefix) > 0 {
		return nil, fmt.Errorf("document: ToAnansi is only supported on root documents, not record or nested views")
	}
	return canansi.SerializeAnansi(d.cs, d.c, fullVersion)
}

// UnmarshalAnansi decodes an Anansi binary wire format packet (as produced
// by ToAnansi) into the receiver, mirroring UnmarshalJSON: the receiver must
// already be bound to a Collection (constructed via a DocumentPool), after
// which identity metadata defaults are completed and the checksum
// recomputed.
func (d *Document) UnmarshalAnansi(data []byte) error {
	if d == nil {
		return fmt.Errorf("document: cannot unmarshal into a nil document")
	}
	if d.cs == nil || d.c == nil {
		return fmt.Errorf("document: document is not bound to a schema")
	}
	if _, err := canansi.DecodeAnansiInto(d.cs, data, d.c, d.pool); err != nil {
		return err
	}
	d.ensureMetadataDefaults()
	return d.finalizeMetadata()
}

// String renders the document as pretty JSON.
func (d *Document) String() string {
	if d == nil {
		return "Document{nil}"
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "Document{error: " + err.Error() + "}"
	}
	return string(data)
}

// ============================================================================
// Clone
// ============================================================================

// Clone returns a deep copy of the document. The copy owns its containers and
// can be released independently.
func (d *Document) Clone() data.Documenter {
	if d == nil {
		return nil
	}
	if d.isRecord() {
		return newRecordView(deepCloneMap(d.record), d.ctx)
	}
	return &Document{
		cs:      d.cs,
		c:       cloneUserContainer(d.c, d.pool),
		pool:    d.pool,
		ctx:     d.ctx,
		prefix:  append(definition.ResolvedPath(nil), d.prefix...),
		slotIdx: d.slotIdx,
	}
}

// StripMetadata returns a clean copy without metadata.
func (d *Document) StripMetadata() data.Documenter {
	if d == nil {
		return nil
	}
	if d.isRecord() {
		return newRecordView(deepCloneMap(d.record), d.ctx)
	}
	out := &Document{
		cs:      d.cs,
		c:       cloneUserContainer(d.c, d.pool),
		pool:    d.pool,
		ctx:     d.ctx,
		prefix:  append(definition.ResolvedPath(nil), d.prefix...),
		slotIdx: d.slotIdx,
	}
	if err := out.clearMetadata(); err != nil {
		return out
	}
	return out
}

// Release returns the document's pooled containers to their pools. Only root
// documents own pooled containers; views (container-backed or record) and
// already-released documents are no-ops. After Release the document must not
// be used.
func (d *Document) Release() {
	if d == nil || d.pool == nil {
		return
	}

	// Date: 2026-08-07
	// Subject: Releasing nested document. This ties to decoders
	// as well which materialize data into containers.
	// Should we choose to give subdocuments inside arrays or records
	// There own pools. We might want to release them here
	d.pool.Put(d.c)
	d.c = nil
	d.pool = nil
}

// ============================================================================
// ID / Metadata helpers
// ============================================================================

func newUUID() string {
	return strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
}

func isValidID(id string) bool {
	if len(id) != 32 {
		return false
	}
	var b strings.Builder
	b.Grow(36)
	b.WriteString(id[0:8])
	b.WriteByte('-')
	b.WriteString(id[8:12])
	b.WriteByte('-')
	b.WriteString(id[12:16])
	b.WriteByte('-')
	b.WriteString(id[16:20])
	b.WriteByte('-')
	b.WriteString(id[20:])
	u, err := uuid.Parse(b.String())
	if err != nil {
		return false
	}
	return u.Version() == 7
}

// ensureMetadataDefaults injects created/updated/version defaults when absent,
// mirroring the data factory's extractOrCreateMetadata.
func (d *Document) ensureMetadataDefaults() {
	if d == nil || d.cs == nil || d.c == nil {
		return
	}
	now := time.Now().UnixNano()
	if !d.metadataKeySet(data.MetadataCreated) {
		_ = d.setMetadataValue(data.MetadataCreated, now, true)
	}
	if !d.metadataKeySet(data.MetadataUpdated) {
		_ = d.setMetadataValue(data.MetadataUpdated, now, true)
	}
	if !d.metadataKeySet(data.MetadataVersion) {
		_ = d.setMetadataValue(data.MetadataVersion, 1, true)
	}
}

// finalizeMetadata runs the configured metadata providers and computes the
// document checksum, completing the document's system metadata the same way
// the data factory's newDocument pipeline does.
func (d *Document) finalizeMetadata() error {
	if err := data.ApplyMetadataProviders(d.ctx, d); err != nil {
		return err
	}
	return d.Hash()
}
