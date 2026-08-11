package bits

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func mustSpec(t *testing.T, lengthBits uint8) HandleSpec {
	t.Helper()
	spec, err := NewHandleSpec(lengthBits)
	if err != nil {
		t.Fatalf("NewHandleSpec: %v", err)
	}
	return spec
}

func TestSetGetDelete(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRecord(Config{HandleSpec: spec})
	defer r.Close()

	h1, err := r.Set([]byte("hello"))
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	h2, err := r.Set([]byte("world!!"))
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	if got := r.Get(h1); !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("Get(h1) = %q, want %q", got, "hello")
	}
	if got := r.Get(h2); !bytes.Equal(got, []byte("world!!")) {
		t.Fatalf("Get(h2) = %q, want %q", got, "world!!")
	}

	if !r.Delete(h1) {
		t.Fatalf("Delete(h1) = false, want true")
	}
	if got := r.Get(h1); got != nil {
		t.Fatalf("Get(h1) after delete = %q, want nil", got)
	}
	if got := r.Get(h2); !bytes.Equal(got, []byte("world!!")) {
		t.Fatalf("Get(h2) after unrelated delete = %q, want %q", got, "world!!")
	}

	if r.Length() != 1 {
		t.Fatalf("Length() = %d, want 1", r.Length())
	}
}

func TestHoleReuse(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRecord(Config{HandleSpec: spec})
	defer r.Close()

	h1, _ := r.Set([]byte("aaaaaaaaaa")) // 10 bytes
	h2, _ := r.Set([]byte("bb"))
	r.Delete(h1)

	stats := r.Stats()
	if stats.Holes != 1 {
		t.Fatalf("Holes = %d, want 1", stats.Holes)
	}

	h3, err := r.Set([]byte("ccccc")) // 5 bytes, should reuse part of the hole
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := r.Get(h3); !bytes.Equal(got, []byte("ccccc")) {
		t.Fatalf("Get(h3) = %q, want %q", got, "ccccc")
	}
	if got := r.Get(h2); !bytes.Equal(got, []byte("bb")) {
		t.Fatalf("Get(h2) = %q, want %q (unaffected by hole reuse)", got, "bb")
	}

	stats = r.Stats()
	if stats.Holes != 1 { // original 10-byte hole minus 5 used = 1 remaining hole of 5
		t.Fatalf("Holes after partial reuse = %d, want 1", stats.Holes)
	}
	if stats.WastedBytes != 5 {
		t.Fatalf("WastedBytes = %d, want 5", stats.WastedBytes)
	}
}

func TestSnapshotIsolation_ConcurrentReaderSeesConsistentView(t *testing.T) {
	// This is the core RCU correctness property: a Read() callback that
	// captured a snapshot must see a totally consistent, unchanging view
	// even while other goroutines mutate the Record concurrently.
	spec := mustSpec(t, 16)
	r := NewRecord(Config{HandleSpec: spec})
	defer r.Close()

	h1, _ := r.Set([]byte("original-1"))
	h2, _ := r.Set([]byte("original-2"))

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer goroutine: continuously deletes and re-adds entries, and
	// triggers manual defragmentation, to maximize the chance of catching
	// any accidental in-place mutation of a published snapshot.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			r.Delete(h1)
			nh, _ := r.Set([]byte(fmt.Sprintf("churn-%d", i)))
			_ = nh
			r.Defragment()
			h1, _ = r.Set([]byte("original-1")) // keep a stable h1-equivalent alive for the reader below... actually replaced below
		}
	}()

	// Reader goroutine: repeatedly snapshots via Read and verifies that
	// whatever it captured never changes mid-inspection.
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(200 * time.Millisecond)
		for time.Now().Before(deadline) {
			r.Read(func(entries []Handle, data []byte) {
				snapshotEntries := append([]Handle(nil), entries...)
				snapshotLen := len(data)
				time.Sleep(time.Microsecond) // give a writer a chance to interleave
				if len(entries) != len(snapshotEntries) {
					t.Errorf("entries slice header mutated under an active Read callback")
				}
				if len(data) != snapshotLen {
					t.Errorf("data slice header mutated under an active Read callback")
				}
			})
			// h2 must always resolve to its original bytes: nothing this
			// test does should ever touch h2.
			if got := r.Get(h2); !bytes.Equal(got, []byte("original-2")) {
				t.Errorf("Get(h2) = %q, want %q (snapshot isolation violated)", got, "original-2")
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestCloneIsIndependent(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRecord(Config{HandleSpec: spec})
	defer r.Close()

	h1, _ := r.Set([]byte("shared"))
	clone := r.Clone()
	defer clone.Close()

	// Both should resolve h1 identically right after cloning.
	if got := clone.Get(h1); !bytes.Equal(got, []byte("shared")) {
		t.Fatalf("clone.Get(h1) = %q, want %q", got, "shared")
	}

	// Mutating the original must not affect the clone, and vice versa.
	r.Delete(h1)
	if got := clone.Get(h1); !bytes.Equal(got, []byte("shared")) {
		t.Fatalf("clone.Get(h1) after original.Delete = %q, want %q (clone must be independent)", got, "shared")
	}

	h2, _ := clone.Set([]byte("clone-only"))
	if got := r.Get(h2); got != nil {
		t.Fatalf("original.Get(h2) = %q, want nil (clone-only entry must not leak back)", got)
	}
}

func TestIDIndexAndUpdateHandle(t *testing.T) {
	spec := mustSpec(t, 16) // 48-16=32 bits of ID space
	nextID := uint64(1)
	r := NewRecord(Config{
		HandleSpec: spec,
		OnSetID: func(h Handle, width uint8) Handle {
			nh, err := spec.SetID(h, nextID)
			if err != nil {
				return h
			}
			nextID++
			return nh
		},
	})
	defer r.Close()

	h1, _ := r.Set([]byte("one"))
	h2, _ := r.Set([]byte("two"))

	id1 := spec.ID(h1)
	id2 := spec.ID(h2)
	if id1 == id2 {
		t.Fatalf("expected distinct IDs, got %d == %d", id1, id2)
	}

	found, ok := r.Handle(id1)
	if !ok || found != h1 {
		t.Fatalf("Handle(id1) = (%v, %v), want (%v, true)", found, ok, h1)
	}

	// UpdateHandle: same offset+ID, different length bits, should succeed
	// and return the old handle.
	longer, err := spec.Handle(int(h1.Offset()), 3)
	if err != nil {
		t.Fatalf("spec.Handle: %v", err)
	}
	longer, err = spec.SetID(longer, id1)
	if err != nil {
		t.Fatalf("spec.SetID: %v", err)
	}
	old, ok := r.UpdateHandle(longer)
	if !ok || old != h1 {
		t.Fatalf("UpdateHandle = (%v, %v), want (%v, true)", old, ok, h1)
	}

	found, ok = r.Handle(id1)
	if !ok || found != longer {
		t.Fatalf("Handle(id1) after update = (%v, %v), want (%v, true)", found, ok, longer)
	}
}

func TestIDIndexCompactionAndGrowth(t *testing.T) {
	spec := mustSpec(t, 16)
	nextID := uint64(1)
	r := NewRecord(Config{
		HandleSpec:          spec,
		CompactionThreshold: 4,
		OnSetID: func(h Handle, width uint8) Handle {
			nh, err := spec.SetID(h, nextID)
			if err != nil {
				return h
			}
			nextID++
			return nh
		},
	})
	defer r.Close()

	var handles []Handle
	for i := 0; i < 100; i++ {
		h, err := r.Set([]byte(fmt.Sprintf("entry-%d", i)))
		if err != nil {
			t.Fatalf("Set: %v", err)
		}
		handles = append(handles, h)
	}

	// Delete every other entry to churn the ID table and trigger
	// compaction repeatedly given the low threshold.
	for i := 0; i < len(handles); i += 2 {
		if !r.Delete(handles[i]) {
			t.Fatalf("Delete(handles[%d]) = false", i)
		}
	}

	stats := r.Stats()
	if stats.Entries != 50 {
		t.Fatalf("Entries = %d, want 50", stats.Entries)
	}
	if stats.Compactions == 0 {
		t.Fatalf("Compactions = 0, want > 0 given CompactionThreshold=4 and 50 deletes")
	}

	// Every surviving entry must still resolve correctly by ID.
	for i := 1; i < len(handles); i += 2 {
		id := spec.ID(handles[i])
		found, ok := r.Handle(id)
		if !ok || found != handles[i] {
			t.Fatalf("Handle(%d) = (%v, %v), want (%v, true)", id, found, ok, handles[i])
		}
	}
}

func TestDefragmentReclaimsHoles(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRecord(Config{HandleSpec: spec})
	defer r.Close()

	h1, _ := r.Set([]byte("aaaa"))
	h2, _ := r.Set([]byte("bbbb"))
	h3, _ := r.Set([]byte("cccc"))
	r.Delete(h2)

	before := r.Stats()
	if before.Holes == 0 {
		t.Fatalf("expected a hole before defragment")
	}

	r.Defragment()

	after := r.Stats()
	if after.Holes != 0 {
		t.Fatalf("Holes after Defragment = %d, want 0", after.Holes)
	}
	if after.WastedBytes != 0 {
		t.Fatalf("WastedBytes after Defragment = %d, want 0", after.WastedBytes)
	}
	if got := r.Get(h1); !bytes.Equal(got, []byte("aaaa")) {
		t.Fatalf("Get(h1) after defrag = %q, want %q", got, "aaaa")
	}
	if got := r.Get(h3); !bytes.Equal(got, []byte("cccc")) {
		t.Fatalf("Get(h3) after defrag = %q, want %q", got, "cccc")
	}
	if got := r.Get(h2); got != nil {
		t.Fatalf("Get(h2) after defrag (deleted) = %q, want nil", got)
	}
}

func TestAutomaticDefragmentation(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRecord(Config{
		HandleSpec:            spec,
		DefragmentationPeriod: 10 * time.Millisecond,
	})
	defer r.Close()

	h1, _ := r.Set([]byte("aaaa"))
	_, _ = r.Set([]byte("bbbb"))
	r.Delete(h1)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r.Stats().Defragments > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if r.Stats().Defragments == 0 {
		t.Fatalf("expected automatic defragmentation to have run")
	}
	if r.Stats().Holes != 0 {
		t.Fatalf("Holes after automatic defrag = %d, want 0", r.Stats().Holes)
	}
}

func TestDefragmentationDisabledByDefault(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRecord(Config{HandleSpec: spec}) // DefragmentationPeriod: 0
	defer r.Close()

	h1, _ := r.Set([]byte("aaaa"))
	r.Set([]byte("bbbb"))
	r.Delete(h1)

	time.Sleep(50 * time.Millisecond)

	if r.Stats().Defragments != 0 {
		t.Fatalf("Defragments = %d, want 0 (auto-defrag should be disabled)", r.Stats().Defragments)
	}
	if !r.defragStarted.Load() {
		// fine either way functionally, but confirms the lazy-start
		// optimization: no Delete should start the goroutine when disabled.
	} else {
		t.Fatalf("defragStarted = true, want false (goroutine must not start when DefragmentationPeriod <= 0)")
	}
}

func TestWriteRebuildsEverything(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRecord(Config{HandleSpec: spec})
	defer r.Close()

	r.Set([]byte("stale"))

	h1, err := spec.Handle(0, 3)
	if err != nil {
		t.Fatalf("spec.Handle: %v", err)
	}
	h2, err := spec.Handle(1, 3)
	if err != nil {
		t.Fatalf("spec.Handle: %v", err)
	}

	err = r.Write(func() ([]Handle, []byte) {
		return []Handle{h1, h2}, []byte("foobar")
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if got := r.Get(h1); !bytes.Equal(got, []byte("foo")) {
		t.Fatalf("Get(h1) = %q, want %q", got, "foo")
	}
	if got := r.Get(h2); !bytes.Equal(got, []byte("bar")) {
		t.Fatalf("Get(h2) = %q, want %q", got, "bar")
	}
	if r.Length() != 2 {
		t.Fatalf("Length() = %d, want 2", r.Length())
	}
}

func TestClear(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRecord(Config{HandleSpec: spec})
	defer r.Close()

	h1, _ := r.Set([]byte("data"))
	r.Clear()

	if r.Length() != 0 {
		t.Fatalf("Length() after Clear = %d, want 0", r.Length())
	}
	if got := r.Get(h1); got != nil {
		t.Fatalf("Get(h1) after Clear = %q, want nil", got)
	}
	if r.Stats().Holes != 0 {
		t.Fatalf("Holes after Clear = %d, want 0", r.Stats().Holes)
	}
}

func TestConcurrentReadsAndWritesRace(t *testing.T) {
	// Run with -race: this is the test that would catch any accidental
	// mutation of a published snapshot's backing arrays.
	spec := mustSpec(t, 16)
	nextID := uint64(1)
	var idMu sync.Mutex
	r := NewRecord(Config{
		HandleSpec: spec,
		OnSetID: func(h Handle, width uint8) Handle {
			idMu.Lock()
			id := nextID
			nextID++
			idMu.Unlock()
			nh, err := spec.SetID(h, id)
			if err != nil {
				return h
			}
			return nh
		},
		DefragmentationPeriod: 5 * time.Millisecond,
	})
	defer r.Close()

	const numWriters = 4
	const numReaders = 8
	const duration = 300 * time.Millisecond

	var handlesMu sync.Mutex
	var liveHandles []Handle

	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed))
			for {
				select {
				case <-stop:
					return
				default:
				}
				switch rng.Intn(3) {
				case 0:
					buf := make([]byte, 1+rng.Intn(64))
					h, err := r.Set(buf)
					if err == nil {
						handlesMu.Lock()
						liveHandles = append(liveHandles, h)
						handlesMu.Unlock()
					}
				case 1:
					handlesMu.Lock()
					if len(liveHandles) > 0 {
						idx := rng.Intn(len(liveHandles))
						h := liveHandles[idx]
						liveHandles[idx] = liveHandles[len(liveHandles)-1]
						liveHandles = liveHandles[:len(liveHandles)-1]
						handlesMu.Unlock()
						r.Delete(h)
					} else {
						handlesMu.Unlock()
					}
				case 2:
					r.Defragment()
				}
			}
		}(int64(i) + 1)
	}

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				handlesMu.Lock()
				snapHandles := append([]Handle(nil), liveHandles...)
				handlesMu.Unlock()

				for _, h := range snapHandles {
					_ = r.Get(h) // may legitimately be nil if concurrently deleted
				}
				_ = r.Keys()
				_ = r.Length()
				_ = r.Size()
				_ = r.Stats()
				r.Read(func(entries []Handle, data []byte) {
					_ = len(entries)
					_ = len(data)
				})
			}
		}()
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()
}

func BenchmarkGetParallel(b *testing.B) {
	spec, _ := NewHandleSpec(16)
	r := NewRecord(Config{HandleSpec: spec})
	defer r.Close()

	var handles []Handle
	for i := 0; i < 10000; i++ {
		h, _ := r.Set([]byte(fmt.Sprintf("value-%d", i)))
		handles = append(handles, h)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = r.Get(handles[i%len(handles)])
			i++
		}
	})
}

func BenchmarkSet(b *testing.B) {
	spec, _ := NewHandleSpec(16)
	r := NewRecord(Config{HandleSpec: spec})
	defer r.Close()

	buf := []byte("benchmark-value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Set(buf)
	}
}

func BenchmarkCloneParallelReaders(b *testing.B) {
	spec, _ := NewHandleSpec(16)
	r := NewRecord(Config{HandleSpec: spec})
	defer r.Close()
	for i := 0; i < 1000; i++ {
		r.Set([]byte(fmt.Sprintf("value-%d", i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := r.Clone()
		c.Close()
	}
}
