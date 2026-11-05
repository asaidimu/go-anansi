// Package json implements the schema-driven JSON codec over
// *container.DataContainer typed slots, plus a typed-struct facade. Marshal
// and Unmarshal accept either data structure:
//
//   - a *container.DataContainer stamped with a schema ref encodes and decodes
//     its typed slots directly (no intermediate maps, keys ordered by field
//     name);
//   - an anansi-tagged struct derives and caches a compiled schema per Go
//     type. A compiled per-type plan (plan.go) gives the struct path a direct
//     struct↔JSON path — no per-call reflection, boxing, or container traffic —
//     for the shapes it can model exactly. Types the plan refuses (time.Time
//     fields, maps, pointer fields, system-model embeds, …) fall back to the
//     container path, which remains authoritative; the two paths are pinned
//     byte-identical by the differential tests.
//
// The struct path is document-free: a binding.Pool compiles the schema without
// the document model's _id_/_metadata_ enrichment, so output carries no
// generated identity, no checksum and no timestamps.
package json

import (
	"context"
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v8/core/container"
	"github.com/asaidimu/go-anansi/v8/core/schema"
	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
)

// Marshal serializes v to schema-driven JSON. v is any data structure:
//
//   - a *container.DataContainer stamped with a schema ref (by a schema-bound
//     pool) serializes its typed slots directly; a root container emits the
//     whole object, a view emits the subtree at its path, and a nested child
//     emits its local fields;
//   - an anansi-tagged struct (or pointer to one) derives its schema from the
//     Go type on first use and caches it for the process lifetime. Shapes
//     covered by the compiled type plan encode without touching the container
//     path; everything else falls back to it with identical output.
func Marshal(v any) ([]byte, error) {
	if c, ok := v.(*container.DataContainer); ok {
		return marshalContainer(c)
	}
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t != nil && t.Kind() == reflect.Struct {
		if e := planFor(t); e.err == nil {
			return encodeWithPlan(e.root, v)
		}
	}
	return marshalLegacy(v)
}

// MarshalAppend appends the JSON encoding of v to dst and returns the
// extended buffer. The bytes are exactly Marshal's output. It exists for
// long-running callers that serialize in a loop: reusing one buffer across
// calls — buf = json.MarshalAppend(buf[:0], v) — makes the steady state
// allocation-free, so a hot endpoint generates no serialization garbage for
// the GC to collect (Marshal's returned buffer, however sized, is per-call
// garbage by contract since the caller owns it).
//
// The returned slice may share dst's array and is only valid until dst is
// reused; copy it if the bytes must outlive the next call. Types the plan
// refuses fall back to the container path with identical output. On error,
// dst is returned unchanged.
func MarshalAppend(dst []byte, v any) ([]byte, error) {
	if c, ok := v.(*container.DataContainer); ok {
		out, err := marshalContainer(c)
		if err != nil {
			return dst, err
		}
		return append(dst, out...), nil
	}
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t != nil && t.Kind() == reflect.Struct {
		if e := planFor(t); e.err == nil {
			return marshalAppendPlan(dst, e.root, v)
		}
	}
	out, err := marshalLegacy(v)
	if err != nil {
		return dst, err
	}
	return append(dst, out...), nil
}

// Unmarshal parses schema-driven JSON into v. v is any data structure:
//
//   - a *container.DataContainer stamped with a schema ref decodes into its
//     typed slots (a root container validates required fields; a nested child
//     decodes its local fields);
//   - a non-nil pointer to an anansi-tagged struct whose shape matches the
//     schema Marshal derived. Fields absent from the payload keep their zero
//     values. Decoding never aliases the input buffer: strings are built as
//     fresh copies on both paths.
func Unmarshal(data []byte, v any) error {
	if c, ok := v.(*container.DataContainer); ok {
		return unmarshalContainer(data, c)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("json: unmarshal target must be a non-nil pointer to struct or *container.DataContainer, got %T", v)
	}
	if e, err := planForDecode(rv.Elem().Type()); err == nil {
		return decodeWithPlan(e.root, data, v)
	}
	return unmarshalLegacy(data, v)
}

// containerRef returns the immutable schema context attached to c.
func containerRef(c *container.DataContainer) (*schema.Ref, error) {
	if c == nil {
		return nil, fmt.Errorf("json: cannot serialize a nil container")
	}
	p := c.Ref()
	if p == nil {
		return nil, fmt.Errorf("json: container is not schema-bound; acquire it from a schema-bound pool")
	}
	r := (*schema.Ref)(p)
	if r == nil || r.CS == nil {
		return nil, fmt.Errorf("json: container carries an empty schema ref")
	}
	return r, nil
}

// marshalContainer serializes a schema-stamped container. A root container
// (slot 0, no path) uses the full-document path; a view (slot 0 with a path)
// serializes the subtree at that path; a nested child (nonzero slot) emits its
// local fields.
func marshalContainer(c *container.DataContainer) ([]byte, error) {
	r, err := containerRef(c)
	if err != nil {
		return nil, err
	}
	switch {
	case r.Slot == 0 && len(r.Path) == 0:
		return serializeJSONFiltered(r.CS, c, "", nil)
	case r.Slot == 0:
		return SerializeJSONPrefix(r.CS, c, r.DottedPath())
	default:
		return serializeContainerAt(r.CS, c, r.Slot)
	}
}

func unmarshalContainer(data []byte, c *container.DataContainer) error {
	r, err := containerRef(c)
	if err != nil {
		return err
	}
	switch {
	case r.Slot == 0 && len(r.Path) == 0:
		return DecodeJSONInto(r.CS, data, c, r.Containers)
	case r.Slot == 0:
		return fmt.Errorf("json: cannot decode into a nested view container; decode a root container")
	default:
		return decodeContainerAt(r.CS, data, c, r.Slot, r.Path, r.Containers)
	}
}

// ----------------------------------------------------------------------------
// Legacy container path (semantic reference and fallback)
// ----------------------------------------------------------------------------

func marshalLegacy(v any) ([]byte, error) {
	pool, err := poolFor(v)
	if err != nil {
		return nil, err
	}
	c := pool.Acquire()
	defer pool.Release(c)
	if err := pool.Populate(c, v); err != nil {
		return nil, err
	}
	return SerializeJSON(pool.CS, c)
}

func unmarshalLegacy(data []byte, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("json: unmarshal target must be a non-nil pointer to struct, got %T", v)
	}
	pool, err := poolFor(v)
	if err != nil {
		return err
	}
	c := pool.Acquire()
	defer pool.Release(c)
	if err := DecodeJSONInto(pool.CS, data, c, pool.Containers); err != nil {
		return err
	}
	return pool.Bind(c, v, context.Background())
}

func poolFor(v any) (*binding.Pool, error) {
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("json: value must be a struct, got %T", v)
	}
	return binding.PoolForType(t)
}
