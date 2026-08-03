package document

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/data/container"
)

func TestDebugID(t *testing.T) {
	col := newTestCollection(t)
	d, err := col.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after New: ID=%q len=%d", d.ID(), d.c.Length())

	_, fd, err := d.cs.ResolveFieldStep(0, data.DocumentIDField)
	if err != nil {
		t.Fatalf("ResolveFieldStep: %v", err)
	}
	t.Logf("fd.DataPoint=%d", int64(fd.DataPoint()))
	rp, _, err := d.resolvePath(data.DocumentIDField)
	if err != nil {
		t.Fatalf("resolvePath: %v", err)
	}
	addr := d.cs.Address(rp)
	t.Logf("cs.Address(%v)=%d internalKey fd.DataPoint=%d", rp, int64(addr), int64(fd.DataPoint()))
	v, ok, err := d.c.GetString(container.NewDataContainerKey(container.DataPoint(fd.DataPoint()), uint32(fd)))
	t.Logf("read via internalKey: %q ok=%v err=%v", v, ok, err)
}
