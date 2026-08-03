package document

import (
	"context"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/data/container"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ============================================================================
// Container Pooling
// ============================================================================

// newContainerFor allocates a user-data container for a derived document
// (clone/strip/normalize) from the owning pool when the source document is
// pooled, and falls back to an unpooled container otherwise (schema-free
// views).
func newContainerFor(d *Document) *container.DataContainer {
	if d.pool != nil {
		return d.pool.Get()
	}
	return container.NewDataContainer()
}

// cloneUserContainer deep-copies c. When pool is non-nil the copy and its
// array-object children are allocated from pool so the copy can be released;
// otherwise the copy is unpooled.
func cloneUserContainer(c *container.DataContainer, pool *container.Pool) *container.DataContainer {
	if c == nil {
		return nil
	}
	if pool == nil {
		return cloneContainer(c)
	}
	out, err := pool.Clone(c)
	if err != nil {
		return cloneContainer(c)
	}
	return out
}

// ============================================================================
// DocumentPool
// ============================================================================

// DocumentPool is a schema-bound factory for container-backed documents. It is
// created once per schema — enriching the schema with the system fields
// (_id_, _metadata_), compiling and linking it, and owning the per-schema
// container pool — and is the only way to construct a *Document. Documents
// acquired from the same DocumentPool share its pool, so container slice sizes
// stabilize at the schema's steady-state cardinality.
type DocumentPool struct {
	cs   *definition.CompiledSchema // user-data compiled schema
	pool *container.Pool            // per-schema user-data container pool
}

// DocumentPoolProvider is implemented by types that expose a container-backed
// document pool. The pool is schema-bound and owned by the lowest collection
// level; higher-level wrappers (model collections, decorators) reuse it
// instead of compiling a second pool from the schema.
type DocumentPoolProvider interface {
	// DocumentPool returns the container-backed document pool, rebuilt when the
	// active schema version changes.
	DocumentPool(ctx context.Context) (*DocumentPool, error)
}

// NewDocumentPool enriches s with the system fields and builds a DocumentPool that
// owns its pool.
func NewDocumentPool(s *definition.Schema) (*DocumentPool, error) {
	if s == nil {
		return nil, ErrNoSchema
	}
	rs, err := enrichSchema(s)
	if err != nil {
		return nil, err
	}
	compiled, err := definition.Compile(rs)
	if err != nil {
		return nil, err
	}
	cs, err := definition.Link(compiled)
	if err != nil {
		return nil, err
	}
	return &DocumentPool{cs: cs, pool: container.NewPool()}, nil
}

// NewDocumentPoolFromJSON compiles the schema encoded in data and builds a
// DocumentPool that owns its pool.
func NewDocumentPoolFromJSON(data []byte) (*DocumentPool, error) {
	s, err := definition.FromJSON(data)
	if err != nil {
		return nil, err
	}
	return NewDocumentPool(s)
}

// CompiledSchema returns the compiled, linked user-data schema this pool binds
// its documents to.
func (c *DocumentPool) CompiledSchema() *definition.CompiledSchema { return c.cs }

// newDocument allocates a bare container-backed document: a container from the
// pool, schema binding, and a default context. It carries no ID, no metadata
// and no checksum — full-document builders add those.
func (c *DocumentPool) newDocument() (*Document, error) {
	if c == nil || c.cs == nil {
		return nil, ErrNoSchema
	}
	var user *container.DataContainer
	if c.pool != nil {
		user = c.pool.Get()
	} else {
		user = container.NewDataContainer()
	}
	return &Document{
		cs:   c.cs,
		c:    user,
		pool: c.pool,
		ctx:  context.Background(),
	}, nil
}

// FromJSON decodes a serialized document into a pooled, schema-bound document
// via the custom codec. A valid _id_ decoded from the payload is honored;
// otherwise one is generated — the document factory is the only component that
// generates _id_. Like the other full-document builders, FromJSON completes
// identity metadata defaults and computes the checksum.
func (c *DocumentPool) FromJSON(data []byte, opts ...Option) (*Document, error) {
	d, err := c.newDocument()
	if err != nil {
		return nil, err
	}
	if err := cjson.DecodeJSONInto(c.cs, data, d.c, c.pool); err != nil {
		c.Release(d)
		return nil, err
	}
	for _, o := range opts {
		o(d)
	}
	if d.ID() == "" || !isValidID(d.ID()) {
		d.setID(newUUID())
	}
	d.ensureMetadataDefaults()
	if err := d.finalizeMetadata(); err != nil {
		c.Release(d)
		return nil, err
	}
	return d, nil
}

// MustFromJSON is FromJSON but panics on error.
func (c *DocumentPool) MustFromJSON(data []byte, opts ...Option) *Document {
	d, err := c.FromJSON(data, opts...)
	if err != nil {
		panic(err)
	}
	return d
}

// ============================================================================
// Full Documents
// ============================================================================

// New returns an empty, fully initialized document: a generated ID, metadata
// defaults, configured provider metadata, and a checksum. It mirrors
// data.NewDocument, with the user-data container drawn from the DocumentPool's
// pool.
func (c *DocumentPool) New(opts ...Option) (*Document, error) {
	d, err := c.newDocument()
	if err != nil {
		return nil, err
	}
	d.setID(newUUID())
	for _, o := range opts {
		o(d)
	}
	d.ensureMetadataDefaults()
	if err := d.finalizeMetadata(); err != nil {
		return nil, err
	}
	return d, nil
}

// MustNew is New but panics on error.
func (c *DocumentPool) MustNew(opts ...Option) *Document {
	d, err := c.New(opts...)
	if err != nil {
		panic(err)
	}
	return d
}

// FromMap builds a fully initialized document from a map. The map may include
// the reserved "_id_" and "_metadata_" keys; system fields are extracted and
// stored in their dedicated slots, everything else is validated against the
// schema. A valid "_id_" is honored, otherwise one is generated.
func (c *DocumentPool) FromMap(input map[string]any, opts ...Option) (*Document, error) {
	d, err := c.newDocument()
	if err != nil {
		return nil, err
	}
	if input == nil {
		input = make(map[string]any)
	}

	if idVal, ok := input[data.DocumentIDField]; ok {
		if s, ok := idVal.(string); ok && isValidID(s) {
			d.setID(s)
		}
	}
	if d.ID() == "" {
		d.setID(newUUID())
	}

	if metaVal, ok := input[data.MetadataField]; ok {
		if m, ok := metaVal.(map[string]any); ok {
			if err := d.populateMetadata(m); err != nil {
				return nil, err
			}
		}
	}
	d.ensureMetadataDefaults()

	for k, v := range input {
		if data.ReservedSystemField(k) {
			continue
		}
		if err := d.Set(k, v); err != nil {
			return nil, err
		}
	}

	for _, o := range opts {
		o(d)
	}
	d.ensureMetadataDefaults()

	if err := d.finalizeMetadata(); err != nil {
		return nil, err
	}
	return d, nil
}

// MustFromMap is FromMap but panics on error.
func (c *DocumentPool) MustFromMap(input map[string]any, opts ...Option) *Document {
	d, err := c.FromMap(input, opts...)
	if err != nil {
		panic(err)
	}
	return d
}

// FromStruct builds a fully initialized document from a struct, populating the
// underlying container directly from the struct's anansi-tagged fields — no map
// intermediate. Each field resolves against the compiled schema and is written
// into its typed slot; unknown paths error. The reserved "_id_" field is
// honored when non-empty, "_metadata_" is extracted, and a missing "_id_" is
// generated.
func (c *DocumentPool) FromStruct(s any, opts ...Option) (*Document, error) {
	d, err := c.newDocument()
	if err != nil {
		return nil, err
	}
	if err := d.populateFromStruct(s, false); err != nil {
		return nil, err
	}
	if d.ID() == "" {
		d.setID(newUUID())
	}
	for _, o := range opts {
		o(d)
	}
	d.ensureMetadataDefaults()
	if err := d.finalizeMetadata(); err != nil {
		return nil, err
	}
	return d, nil
}

// MustFromStruct is FromStruct but panics on error.
func (c *DocumentPool) MustFromStruct(s any, opts ...Option) *Document {
	d, err := c.FromStruct(s, opts...)
	if err != nil {
		panic(err)
	}
	return d
}

// ============================================================================
// Patches
// ============================================================================

// Patch returns a completely uninitialized document: empty user and metadata
// containers, no generated ID, no metadata defaults, no provider metadata and
// no checksum. It is the container-backed equivalent of data.Patch — raw
// material to Set fields into for an update, with identity and metadata left
// to the persistence layer.
func (c *DocumentPool) Patch(opts ...Option) (*Document, error) {
	d, err := c.newDocument()
	if err != nil {
		return nil, err
	}
	for _, o := range opts {
		o(d)
	}
	return d, nil
}

// MustPatch is Patch but panics on error.
func (c *DocumentPool) MustPatch(opts ...Option) *Document {
	d, err := c.Patch(opts...)
	if err != nil {
		panic(err)
	}
	return d
}

// FromPartialStruct builds a patch from a struct, writing only the present,
// non-zero anansi-tagged fields. System-embedded fields (data.DocumentModel)
// are skipped; an "_id_" carried by the struct is honored. No ID is generated,
// no metadata defaults or provider metadata are injected, and no checksum is
// computed — identity and metadata are the persistence layer's responsibility.
func (c *DocumentPool) FromPartialStruct(s any, opts ...Option) (*Document, error) {
	d, err := c.newDocument()
	if err != nil {
		return nil, err
	}
	if err := d.populateFromStruct(s, true); err != nil {
		return nil, err
	}
	for _, o := range opts {
		o(d)
	}
	return d, nil
}

// MustFromPartialStruct is FromPartialStruct but panics on error.
func (c *DocumentPool) MustFromPartialStruct(s any, opts ...Option) *Document {
	d, err := c.FromPartialStruct(s, opts...)
	if err != nil {
		panic(err)
	}
	return d
}

// Release returns a document's pooled containers to the DocumentPool's pool.
// Views and already-released documents are no-ops.
func (c *DocumentPool) Release(d *Document) {
	if d == nil {
		return
	}
	d.Release()
}
