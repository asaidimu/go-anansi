package anansi

import (
	"context"
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v8/core/container"
	"github.com/asaidimu/go-anansi/v8/core/schema"
	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
)

// WireVersion is the wire-format version stamped into packets Marshal
// produces (spec 1.0).
const WireVersion uint16 = 0x0100

// Marshal serializes v to the Anansi binary wire format. v is any data
// structure:
//
//   - a root *container.DataContainer stamped with a schema ref (slot 0, no
//     path) serializes its typed slots; the packet version defaults to
//     WireVersion and can be overridden with WithVersion. Nested children and
//     views are not standalone packets, so they are rejected;
//   - an anansi-tagged struct (or pointer to one) derives its schema from the
//     Go type on first use and caches it for the process lifetime.
func Marshal(v any, opts ...EncodeOption) ([]byte, error) {
	if c, ok := v.(*container.DataContainer); ok {
		return marshalContainer(c, opts...)
	}
	pool, err := poolFor(v)
	if err != nil {
		return nil, err
	}
	c := pool.Acquire()
	defer pool.Release(c)
	if err := pool.Populate(c, v); err != nil {
		return nil, err
	}
	return SerializeAnansi(pool.CS, c, WireVersion, opts...)
}

// Unmarshal decodes an Anansi packet into v. v is any data structure:
//
//   - a root *container.DataContainer stamped with a schema ref (slot 0, no
//     path) decodes into its typed slots, sourcing nested element containers
//     from the ref's factory;
//   - a non-nil pointer to an anansi-tagged struct whose shape matches the
//     schema Marshal derived. Fields absent from the packet keep their zero
//     values. Decoding copies strings out of the packet backing
//     (WithCopyStrings), so the pooled container can be recycled immediately
//     after the bind.
func Unmarshal(data []byte, v any, opts ...DecodeOption) error {
	if c, ok := v.(*container.DataContainer); ok {
		return unmarshalContainer(data, c, opts...)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("anansi: unmarshal target must be a non-nil pointer to struct or *container.DataContainer, got %T", v)
	}
	pool, err := poolFor(v)
	if err != nil {
		return err
	}
	c := pool.Acquire()
	defer pool.Release(c)
	if _, err := DecodeAnansiInto(pool.CS, data, c, pool.Containers, append([]DecodeOption{WithCopyStrings()}, opts...)...); err != nil {
		return err
	}
	return pool.Bind(c, v, context.Background())
}

func containerRef(c *container.DataContainer) (*schema.Ref, error) {
	if c == nil {
		return nil, fmt.Errorf("anansi: cannot serialize a nil container")
	}
	p := c.Ref()
	if p == nil {
		return nil, fmt.Errorf("anansi: container is not schema-bound; acquire it from a schema-bound pool")
	}
	r := (*schema.Ref)(p)
	if r == nil || r.CS == nil {
		return nil, fmt.Errorf("anansi: container carries an empty schema ref")
	}
	return r, nil
}

func marshalContainer(c *container.DataContainer, opts ...EncodeOption) ([]byte, error) {
	r, err := containerRef(c)
	if err != nil {
		return nil, err
	}
	if r.Slot != 0 || len(r.Path) != 0 {
		return nil, fmt.Errorf("anansi: only root containers are standalone packets (slot %d, path %q)", r.Slot, r.DottedPath())
	}
	return SerializeAnansi(r.CS, c, WireVersion, opts...)
}

func unmarshalContainer(data []byte, c *container.DataContainer, opts ...DecodeOption) error {
	r, err := containerRef(c)
	if err != nil {
		return err
	}
	if r.Slot != 0 || len(r.Path) != 0 {
		return fmt.Errorf("anansi: only root containers are standalone packets (slot %d, path %q)", r.Slot, r.DottedPath())
	}
	_, err = DecodeAnansiInto(r.CS, data, c, r.Containers, opts...)
	return err
}

func poolFor(v any) (*binding.Pool, error) {
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("anansi: value must be a struct, got %T", v)
	}
	return binding.PoolForType(t)
}
