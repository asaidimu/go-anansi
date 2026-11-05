package json

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

func orderPool(t *testing.T) *binding.Pool {
	t.Helper()
	pool, err := binding.PoolForType(reflect.TypeOf(Order{}))
	if err != nil {
		t.Fatalf("PoolForType: %v", err)
	}
	return pool
}

func orderLines(t *testing.T, cs *definition.CompiledSchema, c *container.DataContainer) []*container.DataContainer {
	t.Helper()
	abs, _, ok := findRootField(cs, "lines")
	if !ok {
		t.Fatal("Order schema has no lines field")
	}
	children, ok, err := c.GetArrayObject(internalKey(cs.Descriptors[abs]))
	if err != nil {
		t.Fatalf("GetArrayObject: %v", err)
	}
	if !ok || len(children) == 0 {
		t.Fatal("no lines children")
	}
	return children
}

// TestContainerDispatchRoot pins the data-structure contract for a root
// container: Marshal(c) is byte-identical to Marshal(struct) after Populate,
// and Unmarshal back into a pooled container reproduces the struct.
func TestContainerDispatchRoot(t *testing.T) {
	in := sampleOrder()
	want, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal(struct): %v", err)
	}

	pool := orderPool(t)
	c := pool.Acquire()
	defer pool.Release(c)
	if err := pool.Populate(c, in); err != nil {
		t.Fatalf("Populate: %v", err)
	}

	got, err := Marshal(c)
	if err != nil {
		t.Fatalf("Marshal(container): %v", err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("container bytes differ from struct bytes:\nstruct: %s\ncontainer: %s", want, got)
	}

	appended, err := MarshalAppend(nil, c)
	if err != nil {
		t.Fatalf("MarshalAppend(container): %v", err)
	}
	if !bytes.Equal(want, appended) {
		t.Fatalf("MarshalAppend(container) differs:\n%s\n%s", want, appended)
	}

	out := pool.Acquire()
	defer pool.Release(out)
	if err := Unmarshal(got, out); err != nil {
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
	if _, err := Marshal(c); err == nil {
		t.Fatal("Marshal(unbound container) succeeded, want error")
	}
	if err := Unmarshal([]byte(`{}`), c); err == nil {
		t.Fatal("Unmarshal(unbound container) succeeded, want error")
	}
	var nilC *container.DataContainer
	if _, err := Marshal(nilC); err == nil {
		t.Fatal("Marshal(nil container) succeeded, want error")
	}
}

// TestContainerDispatchChild pins that an array-of-object element is itself a
// self-describing data structure: it serializes to its local JSON object
// (matching the struct shape) and can be decoded into another stamped child.
func TestContainerDispatchChild(t *testing.T) {
	in := sampleOrder()
	pool := orderPool(t)

	c := pool.Acquire()
	defer pool.Release(c)
	if err := pool.Populate(c, in); err != nil {
		t.Fatalf("Populate: %v", err)
	}
	// Children created by Populate carry their own schema ref.
	populated := orderLines(t, pool.CS, c)
	got, err := Marshal(populated[0])
	if err != nil {
		t.Fatalf("Marshal(child): %v", err)
	}

	var line OrderLine
	if err := Unmarshal(got, &line); err != nil {
		t.Fatalf("Unmarshal(struct): %v", err)
	}
	if !reflect.DeepEqual(in.Lines[0], line) {
		t.Fatalf("child round trip mismatch:\nwant: %+v\ngot:  %+v", in.Lines[0], line)
	}

	// Children produced by decode are stamped too.
	doc, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal(struct): %v", err)
	}
	out := pool.Acquire()
	defer pool.Release(out)
	if err := Unmarshal(doc, out); err != nil {
		t.Fatalf("Unmarshal(container): %v", err)
	}
	decoded := orderLines(t, pool.CS, out)
	if err := Unmarshal(got, decoded[0]); err != nil {
		t.Fatalf("Unmarshal(child): %v", err)
	}
	again, err := Marshal(decoded[0])
	if err != nil {
		t.Fatalf("Marshal(decoded child): %v", err)
	}
	if !bytes.Equal(got, again) {
		t.Fatalf("decoded child differs:\nwant: %s\ngot:  %s", got, again)
	}
}
