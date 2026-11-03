// Package anansi provides idiomatic struct-level serialization on top of the
// Anansi binary wire format. Marshal and Unmarshal operate directly on
// anansi-tagged structs: the compiled schema is derived from the Go type once
// (data.ExtractDTOSchemaDirect with system fields omitted) and cached, so
// steady-state calls never touch the schema pipeline.
//
// The path is deliberately document-free: a binding.Pool compiles the schema
// without the document model's _id_/_metadata_ enrichment, Populate writes the
// struct into a pooled container's typed slots, and the wire codec serializes
// it. Output is deterministic — no generated identity, no checksum, no
// timestamps — and decode runs with WithCopyStrings so bound structs never
// alias pooled container storage and the container is safely reusable.
package anansi

import (
	"context"
	"fmt"
	"reflect"

	canansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
)

// WireVersion is the wire-format version stamped into packets Marshal
// produces (spec 1.0).
const WireVersion uint16 = 0x0100

// Marshal serializes v — an anansi-tagged struct (or pointer to one) — into
// the Anansi binary wire format. The schema is derived from the Go type on
// first use and cached for the process lifetime.
func Marshal(v any) ([]byte, error) {
	pool, err := poolFor(v)
	if err != nil {
		return nil, err
	}
	c := pool.Acquire()
	defer pool.Release(c)
	if err := pool.Populate(c, v); err != nil {
		return nil, err
	}
	return canansi.SerializeAnansi(pool.CS, c, WireVersion)
}

// Unmarshal decodes an Anansi packet into v, which must be a non-nil pointer
// to an anansi-tagged struct whose shape matches the schema Marshal derived.
// Fields absent from the packet keep their zero values. Decoding copies
// strings out of the packet backing (WithCopyStrings), so the pooled container
// can be recycled immediately after the bind.
func Unmarshal(data []byte, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("anansi: unmarshal target must be a non-nil pointer to struct, got %T", v)
	}
	pool, err := poolFor(v)
	if err != nil {
		return err
	}
	c := pool.Acquire()
	defer pool.Release(c)
	if _, err := canansi.DecodeAnansiInto(pool.CS, data, c, pool.Containers, canansi.WithCopyStrings()); err != nil {
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
		return nil, fmt.Errorf("anansi: value must be a struct, got %T", v)
	}
	return binding.PoolForType(t)
}
