package anansi_test

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	"github.com/asaidimu/go-anansi/v8/core/schema"
	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
)

// TestContainerDispatchRoot pins the data-structure contract for a root
// container: Marshal(c) is byte-identical to Marshal(struct) after Populate,
// and Unmarshal back into a pooled container reproduces the struct.
func TestContainerDispatchRoot(t *testing.T) {
	in := sampleOrder(1)
	want, err := anansi.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal(struct): %v", err)
	}

	pool, err := binding.PoolForType(reflect.TypeOf(Order{}))
	if err != nil {
		t.Fatalf("PoolForType: %v", err)
	}
	c := pool.Acquire()
	defer pool.Release(c)
	if err := pool.Populate(c, in); err != nil {
		t.Fatalf("Populate: %v", err)
	}

	got, err := anansi.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal(container): %v", err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("container packet differs from struct packet:\nstruct: %x\ncontainer: %x", want, got)
	}

	out := pool.Acquire()
	defer pool.Release(out)
	if err := anansi.Unmarshal(got, out); err != nil {
		t.Fatalf("Unmarshal(container): %v", err)
	}
	var bound Order
	if err := pool.Bind(out, &bound, context.Background()); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if !reflect.DeepEqual(in, bound) {
		t.Fatalf("round trip mismatch:\nwant: %+v\ngot:  %+v", in, bound)
	}
}

// TestContainerDispatchUnbound pins that a container without a schema stamp is
// rejected instead of silently serializing garbage.
func TestContainerDispatchUnbound(t *testing.T) {
	c := container.NewDataContainer()
	if _, err := anansi.Marshal(c); err == nil {
		t.Fatal("Marshal(unbound container) succeeded, want error")
	}
	if err := anansi.Unmarshal([]byte{0}, c); err == nil {
		t.Fatal("Unmarshal(unbound container) succeeded, want error")
	}
	var nilC *container.DataContainer
	if _, err := anansi.Marshal(nilC); err == nil {
		t.Fatal("Marshal(nil container) succeeded, want error")
	}
}

// TestContainerDispatchNonRoot pins that only root containers are standalone
// packets: a stamped child must be rejected rather than encoded as a root.
func TestContainerDispatchNonRoot(t *testing.T) {
	pool, err := binding.PoolForType(reflect.TypeOf(Order{}))
	if err != nil {
		t.Fatalf("PoolForType: %v", err)
	}
	c := pool.Acquire()
	defer pool.Release(c)
	c.SetRef(unsafe.Pointer(schema.NewRef(pool.CS, 1, nil, nil)))
	if _, err := anansi.Marshal(c); err == nil {
		t.Fatal("Marshal(non-root container) succeeded, want error")
	}
}
