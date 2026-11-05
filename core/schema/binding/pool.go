package binding

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/container"
	"github.com/asaidimu/go-anansi/v8/core/schema"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ============================================================================
// SCHEMA-BOUND CONTAINER POOLS
// ============================================================================
//
// Pool pairs a compiled schema with a container pool. Unlike the document
// pool (core/document), the schema is compiled AS PROVIDED — no _id_/_metadata_
// enrichment, no identity defaults, no checksums. This is the substrate
// serializers and other non-document consumers bind structs against.

// Pool binds a compiled schema to a container pool.
type Pool struct {
	CS *definition.CompiledSchema
	// Containers supplies pooled child containers for array-of-object fields
	// and pooled views for Acquire/Release.
	Containers *container.Pool

	// ref is the immutable schema context stamped onto every acquired
	// container so encoders can serialize it without being handed CS again.
	ref     *schema.Ref
	refOnce sync.Once
}

// Ref returns the pool's immutable schema context, creating it lazily for
// pools built as literals.
func (p *Pool) Ref() *schema.Ref {
	if p == nil {
		return nil
	}
	p.refOnce.Do(func() {
		if p.CS == nil {
			return
		}
		var factory container.Factory
		if p.Containers != nil {
			factory = p.Containers
		}
		p.ref = schema.RootRef(p.CS, factory)
	})
	return p.ref
}

// Compile compiles a schema JSON document into a Pool. The schema is used as
// provided: to bind DTO structs, pass a schema produced by
// schema.ExtractDTOSchemaDirect(target, true) (system fields omitted) so the
// container stays free of document-model bookkeeping.
func Compile(schemaJSON []byte) (*Pool, error) {
	s, err := definition.FromJSON(schemaJSON)
	if err != nil {
		return nil, fmt.Errorf("binding: parsing schema: %w", err)
	}
	return NewPool(s)
}

// NewPool compiles and links s without any system-field enrichment.
func NewPool(s *definition.Schema) (*Pool, error) {
	if s == nil {
		return nil, fmt.Errorf("binding: nil schema")
	}
	compiled, err := definition.Compile(s)
	if err != nil {
		return nil, fmt.Errorf("binding: compiling schema: %w", err)
	}
	cs, err := definition.Link(compiled)
	if err != nil {
		return nil, fmt.Errorf("binding: linking schema: %w", err)
	}
	return &Pool{CS: cs, Containers: container.NewPool()}, nil
}

var poolCache sync.Map // reflect.Type -> *Pool

// PoolForType derives (and caches per type) a Pool whose schema is extracted
// from t's anansi-tagged struct shape with system fields omitted. Equivalent
// to document.poolFromTypeOf, minus the document-model enrichment.
func PoolForType(t reflect.Type) (*Pool, error) {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("binding: pool target must be a struct, got %v", t)
	}
	if v, ok := poolCache.Load(t); ok {
		return v.(*Pool), nil
	}
	schemaBytes, err := schema.ExtractDTOSchemaDirect(reflect.New(t).Interface(), true)
	if err != nil {
		return nil, fmt.Errorf("binding: deriving schema for %s: %w", t, err)
	}
	p, err := Compile(schemaBytes)
	if err != nil {
		return nil, fmt.Errorf("binding: compiling schema for %s: %w", t, err)
	}
	actual, _ := poolCache.LoadOrStore(t, p)
	return actual.(*Pool), nil
}

// Acquire returns an empty pooled container stamped with the pool's schema
// context — populate it via Populate or decode into it with the codecs, then
// Release it.
func (p *Pool) Acquire() *container.DataContainer {
	c := p.Containers.Get()
	if r := p.Ref(); r != nil {
		c.SetRef(unsafe.Pointer(r))
	}
	return c
}

// Release returns a container to the pool. Callers must not retain references
// into the container after Release (strings decoded without WithCopyStrings
// alias the container backing).
func (p *Pool) Release(c *container.DataContainer) {
	if c == nil {
		return
	}
	p.Containers.Put(c)
}

// Populate writes the anansi-tagged fields of s into c's typed slots, without
// generating an ID, metadata defaults, or a checksum. Fields must resolve
// against the pool's schema.
func (p *Pool) Populate(c *container.DataContainer, s any) error {
	if p == nil || p.CS == nil {
		return fmt.Errorf("binding: pool has no schema")
	}
	fields, err := WalkStructFields(s, false)
	if err != nil {
		return err
	}
	for _, f := range fields {
		rp, _, err := ResolveViewPath(p.CS, nil, 0, f.Path)
		if err != nil {
			return err
		}
		last := rp[len(rp)-1]
		if err := SetInto(p.CS, c, p.Containers, last.SchemaIdx(), uint16(last.FieldIdx()), rp[:len(rp)-1], f.Value); err != nil {
			return err
		}
	}
	return nil
}

// Bind copies the values in c into target's anansi-tagged fields. The bind
// never materializes c into a map; nested objects bind from container-backed
// child views.
func (p *Pool) Bind(c *container.DataContainer, target any, ctx context.Context) error {
	if p == nil || p.CS == nil {
		return fmt.Errorf("binding: pool has no schema")
	}
	return Bind(&ContainerSource{CS: p.CS, C: c, Ctx: ctx}, target, ctx, "")
}
