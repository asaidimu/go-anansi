// Package anansijson provides idiomatic struct-level JSON serialization:
// Marshal and Unmarshal operate directly on anansi-tagged structs, deriving
// and caching a compiled schema per type. The semantic reference remains the
// schema-driven JSON codec (core/encoding/json) over typed container slots,
// so output is deterministic and the schema's types are enforced on decode.
//
// A compiled per-type plan (plan.go) gives Marshal and Unmarshal a direct
// struct↔JSON path — no per-call reflection, boxing, or container traffic —
// for the shapes it can model exactly. Types the plan refuses (time.Time
// fields, maps, pointer fields, system-model embeds, …) fall back to the
// container path, which remains authoritative; the two paths are pinned
// byte-identical by the differential tests.
//
// Like the binary flavor (core/dto/anansi), the path is document-free: a
// binding.Pool compiles the schema without the document model's
// _id_/_metadata_ enrichment, so output carries no generated identity, no
// checksum and no timestamps.
package anansijson

import (
	"context"
	"fmt"
	"reflect"

	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
)

// Marshal serializes v — an anansi-tagged struct (or pointer to one) — to
// schema-driven JSON. The schema is derived from the Go type on first use and
// cached for the process lifetime. Shapes covered by the compiled type plan
// encode without touching the container path; everything else falls back to
// it with identical output.
func Marshal(v any) ([]byte, error) {
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

// Unmarshal parses schema-driven JSON into v, which must be a non-nil pointer
// to an anansi-tagged struct whose shape matches the schema Marshal derived.
// Fields absent from the payload keep their zero values. Decoding never
// aliases the input buffer: strings are built as fresh copies on both paths.
func Unmarshal(data []byte, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("anansijson: unmarshal target must be a non-nil pointer to struct, got %T", v)
	}
	if e, err := planForDecode(rv.Elem().Type()); err == nil {
		return decodeWithPlan(e.root, data, v)
	}
	return unmarshalLegacy(data, v)
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
	return cjson.SerializeJSON(pool.CS, c)
}

func unmarshalLegacy(data []byte, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("anansijson: unmarshal target must be a non-nil pointer to struct, got %T", v)
	}
	pool, err := poolFor(v)
	if err != nil {
		return err
	}
	c := pool.Acquire()
	defer pool.Release(c)
	if err := cjson.DecodeJSONInto(pool.CS, data, c, pool.Containers); err != nil {
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
		return nil, fmt.Errorf("anansijson: value must be a struct, got %T", v)
	}
	return binding.PoolForType(t)
}
