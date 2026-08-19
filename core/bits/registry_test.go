package bits

import (
	"bytes"
	"errors"
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

type testView struct {
	Handle Handle
	Data   []byte
}

func testConfig(spec HandleSpec) Config[string, testView] {
	return Config[string, testView]{
		HandleSpec: spec,
		HashKey: func(k string) uint64 {
			var h uint64 = 14695981039346656037
			for i := 0; i < len(k); i++ {
				h ^= uint64(k[i])
				h *= 1099511628211
			}
			return h
		},
		NewView: func(h Handle, data []byte) testView {
			return testView{Handle: h, Data: data}
		},
	}
}

func TestSetGetDelete(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRegistry(testConfig(spec))
	defer r.Close()

	v1, err := r.Set("key1", []byte("hello"))
	if err != nil {
		t.Fatalf("Set key1: %v", err)
	}
	v2, err := r.Set("key2", []byte("world!!"))
	if err != nil {
		t.Fatalf("Set key2: %v", err)
	}

	got1, err := r.Get("key1")
	if err != nil || !bytes.Equal(got1.Data, []byte("hello")) {
		t.Fatalf("Get(key1) = (%q, %v), want %q", got1.Data, err, "hello")
	}
	got2, err := r.Get("key2")
	if err != nil || !bytes.Equal(got2.Data, []byte("world!!")) {
		t.Fatalf("Get(key2) = (%q, %v), want %q", got2.Data, err, "world!!")
	}

	if !r.Delete("key1") {
		t.Fatalf("Delete(key1) = false, want true")
	}
	_, err = r.Get("key1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(key1) after delete = %v, want ErrNotFound", err)
	}
	got2After, err := r.Get("key2")
	if err != nil || !bytes.Equal(got2After.Data, []byte("world!!")) {
		t.Fatalf("Get(key2) after unrelated delete = (%q, %v), want %q", got2After.Data, err, "world!!")
	}

	if r.Length() != 1 {
		t.Fatalf("Length() = %d, want 1", r.Length())
	}

	_ = v1
	_ = v2
}

func TestOverwriteKey(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRegistry(testConfig(spec))
	defer r.Close()

	_, err := r.Set("key1", []byte("v1"))
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	v2, err := r.Set("key1", []byte("v2-updated"))
	if err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}

	if !bytes.Equal(v2.Data, []byte("v2-updated")) {
		t.Fatalf("v2.Data = %q, want %q", v2.Data, "v2-updated")
	}

	got, err := r.Get("key1")
	if err != nil || !bytes.Equal(got.Data, []byte("v2-updated")) {
		t.Fatalf("Get(key1) = (%q, %v), want %q", got.Data, err, "v2-updated")
	}
	if r.Length() != 1 {
		t.Fatalf("Length() = %d, want 1", r.Length())
	}
}

func TestHoleReuse(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRegistry(testConfig(spec))
	defer r.Close()

	_, _ = r.Set("k1", []byte("aaaaaaaaaa")) // 10 bytes
	_, _ = r.Set("k2", []byte("bb"))
	r.Delete("k1")

	stats := r.Stats()
	if stats.Holes != 1 {
		t.Fatalf("Holes = %d, want 1", stats.Holes)
	}

	v3, err := r.Set("k3", []byte("ccccc")) // 5 bytes, should reuse part of the hole
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	got3, err := r.Get("k3")
	if err != nil || !bytes.Equal(got3.Data, []byte("ccccc")) {
		t.Fatalf("Get(k3) = %q, want %q", got3.Data, "ccccc")
	}
	got2, err := r.Get("k2")
	if err != nil || !bytes.Equal(got2.Data, []byte("bb")) {
		t.Fatalf("Get(k2) = %q, want %q (unaffected by hole reuse)", got2.Data, "bb")
	}

	stats = r.Stats()
	if stats.Holes != 1 { // original 10-byte hole minus 5 used = 1 remaining hole of 5
		t.Fatalf("Holes after partial reuse = %d, want 1", stats.Holes)
	}
	if stats.WastedBytes != 5 {
		t.Fatalf("WastedBytes = %d, want 5", stats.WastedBytes)
	}
	_ = v3
}

func TestSnapshotIsolation_ConcurrentReaderSeesConsistentView(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRegistry(testConfig(spec))
	defer r.Close()

	_, _ = r.Set("k1", []byte("original-1"))
	_, _ = r.Set("k2", []byte("original-2"))

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			r.Delete("k1")
			_, _ = r.Set(fmt.Sprintf("churn-%d", i), []byte(fmt.Sprintf("churn-%d", i)))
			r.Defragment()
			_, _ = r.Set("k1", []byte("original-1"))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(200 * time.Millisecond)
		for time.Now().Before(deadline) {
			r.Read(func(entries []Handle, data []byte) {
				snapshotEntries := append([]Handle(nil), entries...)
				snapshotLen := len(data)
				time.Sleep(time.Microsecond)
				if len(entries) != len(snapshotEntries) {
					t.Errorf("entries slice header mutated under an active Read callback")
				}
				if len(data) != snapshotLen {
					t.Errorf("data slice header mutated under an active Read callback")
				}
			})
			got2, err := r.Get("k2")
			if err != nil || !bytes.Equal(got2.Data, []byte("original-2")) {
				t.Errorf("Get(k2) = (%q, %v), want %q (snapshot isolation violated)", got2.Data, err, "original-2")
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestCloneIsIndependent(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRegistry(testConfig(spec))
	defer r.Close()

	_, _ = r.Set("k1", []byte("shared"))
	clone := r.Clone()
	defer clone.Close()

	got, err := clone.Get("k1")
	if err != nil || !bytes.Equal(got.Data, []byte("shared")) {
		t.Fatalf("clone.Get(k1) = (%q, %v), want %q", got.Data, err, "shared")
	}

	r.Delete("k1")
	got, err = clone.Get("k1")
	if err != nil || !bytes.Equal(got.Data, []byte("shared")) {
		t.Fatalf("clone.Get(k1) after original.Delete = (%q, %v), want %q (clone must be independent)", got.Data, err, "shared")
	}

	_, _ = clone.Set("k2", []byte("clone-only"))
	_, err = r.Get("k2")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("original.Get(k2) = %v, want ErrNotFound", err)
	}
}

func TestIDIndexCompactionAndGrowth(t *testing.T) {
	spec := mustSpec(t, 16)
	cfg := testConfig(spec)
	cfg.CompactionThreshold = 4
	r := NewRegistry(cfg)
	defer r.Close()

	var keys []string
	for i := 0; i < 100; i++ {
		k := fmt.Sprintf("key-%d", i)
		_, err := r.Set(k, []byte(fmt.Sprintf("entry-%d", i)))
		if err != nil {
			t.Fatalf("Set: %v", err)
		}
		keys = append(keys, k)
	}

	for i := 0; i < len(keys); i += 2 {
		if !r.Delete(keys[i]) {
			t.Fatalf("Delete(keys[%d]) = false", i)
		}
	}

	stats := r.Stats()
	if stats.Entries != 50 {
		t.Fatalf("Entries = %d, want 50", stats.Entries)
	}
	if stats.Compactions == 0 {
		t.Fatalf("Compactions = 0, want > 0 given CompactionThreshold=4 and 50 deletes")
	}

	for i := 1; i < len(keys); i += 2 {
		got, err := r.Get(keys[i])
		want := fmt.Sprintf("entry-%d", i)
		if err != nil || !bytes.Equal(got.Data, []byte(want)) {
			t.Fatalf("Get(%s) = (%q, %v), want %q", keys[i], got.Data, err, want)
		}
	}
}

func TestDefragmentReclaimsHoles(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRegistry(testConfig(spec))
	defer r.Close()

	_, _ = r.Set("k1", []byte("aaaa"))
	_, _ = r.Set("k2", []byte("bbbb"))
	_, _ = r.Set("k3", []byte("cccc"))
	r.Delete("k2")

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
	if got, err := r.Get("k1"); err != nil || !bytes.Equal(got.Data, []byte("aaaa")) {
		t.Fatalf("Get(k1) after defrag = %q, want %q", got.Data, "aaaa")
	}
	if got, err := r.Get("k3"); err != nil || !bytes.Equal(got.Data, []byte("cccc")) {
		t.Fatalf("Get(k3) after defrag = %q, want %q", got.Data, "cccc")
	}
	if _, err := r.Get("k2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(k2) after defrag (deleted) = %v, want ErrNotFound", err)
	}
}

func TestAutomaticDefragmentation(t *testing.T) {
	spec := mustSpec(t, 16)
	cfg := testConfig(spec)
	cfg.DefragmentationPeriod = 10 * time.Millisecond
	r := NewRegistry(cfg)
	defer r.Close()

	_, _ = r.Set("k1", []byte("aaaa"))
	_, _ = r.Set("k2", []byte("bbbb"))
	r.Delete("k1")

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
	r := NewRegistry(testConfig(spec))
	defer r.Close()

	_, _ = r.Set("k1", []byte("aaaa"))
	_, _ = r.Set("k2", []byte("bbbb"))
	r.Delete("k1")

	time.Sleep(50 * time.Millisecond)

	if r.Stats().Defragments != 0 {
		t.Fatalf("Defragments = %d, want 0 (auto-defrag should be disabled)", r.Stats().Defragments)
	}
}

func TestWriteRebuildsEverything(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRegistry(testConfig(spec))
	defer r.Close()

	_, _ = r.Set("k1", []byte("stale"))

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

	if r.Length() != 2 {
		t.Fatalf("Length() = %d, want 2", r.Length())
	}
}

func TestClear(t *testing.T) {
	spec := mustSpec(t, 16)
	r := NewRegistry(testConfig(spec))
	defer r.Close()

	_, _ = r.Set("k1", []byte("data"))
	r.Clear()

	if r.Length() != 0 {
		t.Fatalf("Length() after Clear = %d, want 0", r.Length())
	}
	if _, err := r.Get("k1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(k1) after Clear = %v, want ErrNotFound", err)
	}
	if r.Stats().Holes != 0 {
		t.Fatalf("Holes after Clear = %d, want 0", r.Stats().Holes)
	}
}

func TestConcurrentReadsAndWritesRace(t *testing.T) {
	spec := mustSpec(t, 16)
	cfg := testConfig(spec)
	cfg.DefragmentationPeriod = 5 * time.Millisecond
	r := NewRegistry(cfg)
	defer r.Close()

	const numWriters = 4
	const numReaders = 8
	const duration = 300 * time.Millisecond

	var keysMu sync.Mutex
	var liveKeys []string

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
					k := fmt.Sprintf("key-%d", rng.Intn(100))
					_, err := r.Set(k, buf)
					if err == nil {
						keysMu.Lock()
						liveKeys = append(liveKeys, k)
						keysMu.Unlock()
					}
				case 1:
					keysMu.Lock()
					if len(liveKeys) > 0 {
						idx := rng.Intn(len(liveKeys))
						k := liveKeys[idx]
						liveKeys[idx] = liveKeys[len(liveKeys)-1]
						liveKeys = liveKeys[:len(liveKeys)-1]
						keysMu.Unlock()
						r.Delete(k)
					} else {
						keysMu.Unlock()
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
				keysMu.Lock()
				snapKeys := append([]string(nil), liveKeys...)
				keysMu.Unlock()

				for _, k := range snapKeys {
					_, _ = r.Get(k)
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

func TestExportImport(t *testing.T) {
	spec := mustSpec(t, 16)
	r1 := NewRegistry(testConfig(spec))
	defer r1.Close()

	_, err := r1.Set("k1", []byte("hello"))
	if err != nil {
		t.Fatalf("Set k1: %v", err)
	}
	_, err = r1.Set("k2", []byte("world"))
	if err != nil {
		t.Fatalf("Set k2: %v", err)
	}

	exported := r1.Export()
	if len(exported) < 12 {
		t.Fatalf("exported data too short: %d bytes", len(exported))
	}

	r2 := NewRegistry(testConfig(spec))
	defer r2.Close()

	err = r2.Import(exported)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	got1, err := r2.Get("k1")
	if err != nil || !bytes.Equal(got1.Data, []byte("hello")) {
		t.Fatalf("r2.Get(k1) = (%q, %v), want %q", got1.Data, err, "hello")
	}
	got2, err := r2.Get("k2")
	if err != nil || !bytes.Equal(got2.Data, []byte("world")) {
		t.Fatalf("r2.Get(k2) = (%q, %v), want %q", got2.Data, err, "world")
	}

	// Invalid import data tests
	if err := r2.Import([]byte("too-short")); err == nil {
		t.Fatalf("Import short data = nil, want error")
	}
	badMagic := append([]byte(nil), exported...)
	badMagic[0] = 'X'
	if err := r2.Import(badMagic); err == nil {
		t.Fatalf("Import bad magic = nil, want error")
	}
}
