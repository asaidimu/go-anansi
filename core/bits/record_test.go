package bits

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func mustSpec(t *testing.T, lengthBits uint8) HandleSpec {
	t.Helper()
	s, err := NewHandleSpec(lengthBits)
	if err != nil {
		t.Fatalf("NewHandleSpec(%d): %v", lengthBits, err)
	}
	return s
}

// newRecord builds a Record with the given spec. When assignIDs is true, OnSetID
// stamps every new handle with a unique sequentially-increasing ID so that
// Handle(id) lookups behave meaningfully.
func newRecord(t *testing.T, spec HandleSpec, assignIDs bool) *Record {
	t.Helper()
	cfg := Config{HandleSpec: spec}
	if assignIDs {
		next := 0
		cfg.OnSetID = func(h Handle, width uint8) Handle {
			next++
			out, err := spec.SetID(h, uint64(next))
			if err != nil {
				panic(fmt.Sprintf("SetID: %v", err))
			}
			return out
		}
	}
	return NewRecord(cfg)
}

func TestRecordSetGetRoundTrip(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	blobs := [][]byte{
		{},
		{0x01},
		[]byte("hello, world"),
		bytes.Repeat([]byte{0xAB}, 4096),
	}
	for _, b := range blobs {
		h, err := r.Set(b)
		if err != nil {
			t.Fatalf("Set(%d bytes): %v", len(b), err)
		}
		if got := r.Get(h); !bytes.Equal(got, b) {
			t.Errorf("Get(%d bytes) = %q, want %q", len(b), got, b)
		}
		if int(spec.Length(h)) != len(b) {
			t.Errorf("handle length = %d, want %d", spec.Length(h), len(b))
		}
	}
	if r.Length() != len(blobs) {
		t.Errorf("Length = %d, want %d", r.Length(), len(blobs))
	}
	if r.Size() == 0 {
		t.Error("Size() = 0 after storing non-empty data (the empty blob only)")
	}
}

func TestRecordSetTooLong(t *testing.T) {
	spec := mustSpec(t, 2) // max representable length = 3
	r := newRecord(t, spec, false)
	defer r.Close()

	h, err := r.Set([]byte{1, 2, 3, 4})
	if err == nil {
		t.Fatalf("expected error for 4 bytes with 2-bit length spec, got handle %x", h)
	}
	if !errors.Is(err, ErrLengthExceedsBitSpace) {
		t.Errorf("error = %v, want ErrLengthExceedsBitSpace", err)
	}
	// A short blob must still work.
	if _, err := r.Set([]byte{1, 2, 3}); err != nil {
		t.Fatalf("Set(3 bytes): %v", err)
	}
	if r.Length() != 1 {
		t.Errorf("Length = %d, want 1", r.Length())
	}
}

func TestRecordGetNilForMissingOrCorrupt(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	h, err := r.Set([]byte("abc"))
	if err != nil {
		t.Fatal(err)
	}

	// Stale handle pointing at a slot index that was never allocated.
	stale, _ := spec.Handle(9999, 3)
	if got := r.Get(stale); got != nil {
		t.Errorf("Get(never-allocated slot) = %q, want nil", got)
	}

	// Corrupt length that runs past the end of data.
	overrun, _ := spec.Handle(int(h.Offset()), 100)
	if got := r.Get(overrun); got != nil {
		t.Errorf("Get(length overrun) = %q, want nil", got)
	}
}

func TestRecordDelete(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	h1, _ := r.Set([]byte("one"))
	h2, _ := r.Set([]byte("two"))
	h3, _ := r.Set([]byte("three"))

	if !r.Delete(h2) {
		t.Fatal("Delete(h2) = false, want true")
	}
	if r.Length() != 2 {
		t.Errorf("Length after delete = %d, want 2", r.Length())
	}
	if !bytes.Equal(r.Get(h1), []byte("one")) {
		t.Error("h1 damaged by swap-remove delete")
	}
	if !bytes.Equal(r.Get(h3), []byte("three")) {
		t.Error("h3 damaged by swap-remove delete")
	}
	if got := r.Get(h2); got != nil {
		t.Errorf("Get(deleted) = %q, want nil", got)
	}
	// Second delete of the same handle reports false.
	if r.Delete(h2) {
		t.Error("Delete(h2) again = true, want false")
	}
	// Deleting an out-of-range slot reports false.
	ghost, _ := spec.Handle(12345, 0)
	if r.Delete(ghost) {
		t.Error("Delete(never-allocated) = true, want false")
	}
}

func TestRecordHoleReuse(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	h1, _ := r.Set(bytes.Repeat([]byte{'a'}, 20))
	h2, _ := r.Set(bytes.Repeat([]byte{'b'}, 20))

	// Deleting h2 leaves a 20-byte hole; a fresh 20-byte Set must reuse it
	// (data buffer must not grow).
	if !r.Delete(h2) {
		t.Fatal("Delete(h2) failed")
	}
	before := r.Stats()
	if before.Holes != 1 || before.WastedBytes != 20 {
		t.Fatalf("stats before reuse = %+v, want 1 hole / 20 wasted bytes", before)
	}

	h3, err := r.Set(bytes.Repeat([]byte{'c'}, 20))
	if err != nil {
		t.Fatal(err)
	}
	after := r.Stats()
	if after.Holes != 0 || after.WastedBytes != 0 {
		t.Errorf("stats after reuse = %+v, want 0 holes", after)
	}
	if after.DataBytes != before.DataBytes {
		t.Errorf("DataBytes grew across hole reuse: %d -> %d (should be unchanged)", before.DataBytes, after.DataBytes)
	}
	if !bytes.Equal(r.Get(h3), bytes.Repeat([]byte{'c'}, 20)) {
		t.Error("reused-slot value mismatch")
	}
	_ = h1

	// A Set larger than the hole must append instead (data grows).
	before = r.Stats()
	hBig, err := r.Set(bytes.Repeat([]byte{'d'}, 50))
	if err != nil {
		t.Fatal(err)
	}
	if r.Stats().DataBytes <= before.DataBytes {
		t.Error("larger-than-hole Set did not grow the data buffer")
	}
	if !bytes.Equal(r.Get(hBig), bytes.Repeat([]byte{'d'}, 50)) {
		t.Error("appended value mismatch")
	}
}

func TestRecordKeysAndRead(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	if keys := r.Keys(); keys != nil {
		t.Errorf("Keys() on empty store = %v, want nil", keys)
	}

	h1, _ := r.Set([]byte("first"))
	h2, _ := r.Set([]byte("second"))

	keys := r.Keys()
	if len(keys) != 2 {
		t.Fatalf("Keys() len = %d, want 2", len(keys))
	}
	if !(bytes.Equal(r.Get(keys[0]), []byte("first")) || bytes.Equal(r.Get(keys[1]), []byte("first"))) {
		t.Error("Keys() missing first entry")
	}
	if !(bytes.Equal(r.Get(keys[0]), []byte("second")) || bytes.Equal(r.Get(keys[1]), []byte("second"))) {
		t.Error("Keys() missing second entry")
	}

	var gotEntries []Handle
	var gotData []byte
	r.Read(func(entries []Handle, data []byte) {
		gotEntries = append([]Handle(nil), entries...)
		gotData = append([]byte(nil), data...)
	})
	if len(gotEntries) != 2 {
		t.Errorf("Read entries len = %d, want 2", len(gotEntries))
	}
	if !bytes.Contains(gotData, []byte("first")) || !bytes.Contains(gotData, []byte("second")) {
		t.Errorf("Read data = %q, want it to contain both blobs", gotData)
	}
	_ = h1
	_ = h2
}

func TestRecordIDLookup(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, true)
	defer r.Close()

	h1, _ := r.Set([]byte("one"))
	h2, _ := r.Set([]byte("two"))
	id1 := spec.ID(h1)
	id2 := spec.ID(h2)

	if got, ok := r.Handle(id1); !ok || got != h1 {
		t.Errorf("Handle(%d) = (%x, %v), want (%x, true)", id1, got, ok, h1)
	}
	if got, ok := r.Handle(id2); !ok || got != h2 {
		t.Errorf("Handle(%d) = (%x, %v), want (%x, true)", id2, got, ok, h2)
	}

	// Unknown ID.
	if _, ok := r.Handle(id2 + 1); ok {
		t.Error("Handle(unknown ID) = ok, want false")
	}

	// Delete removes the ID mapping.
	r.Delete(h1)
	if _, ok := r.Handle(id1); ok {
		t.Error("Handle(deleted ID) = ok, want false")
	}
	// The surviving entry's ID still maps.
	if got, ok := r.Handle(id2); !ok || got != h2 {
		t.Errorf("surviving Handle(%d) = (%x, %v), want its handle", id2, got, ok)
	}
}

func TestRecordNoIDsUntracked(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false) // no OnSetID → default ID is 0 for every entry
	defer r.Close()

	if _, ok := r.Handle(0); ok {
		t.Error("Handle(0) on ID-less store = ok, want false")
	}
}

func TestRecordUpdateHandle(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, true)
	defer r.Close()

	h, _ := r.Set([]byte("hello"))
	id := spec.ID(h)

	// Amend the length (same slot, same ID).
	shorter, err := spec.Handle(int(h.Offset()), 3)
	if err != nil {
		t.Fatal(err)
	}
	shorter, err = spec.SetID(shorter, id)
	if err != nil {
		t.Fatal(err)
	}

	prev, ok := r.UpdateHandle(shorter)
	if !ok {
		t.Fatal("UpdateHandle = false, want true")
	}
	if prev != h {
		t.Errorf("previous handle = %x, want %x", prev, h)
	}
	// Get reads the byte range the *passed* handle declares, so the amended
	// handle must surface the shortened length while the stale full-length
	// handle still reads the (still-present) full bytes.
	if got := r.Get(shorter); !bytes.Equal(got, []byte("hel")) {
		t.Errorf("Get via amended handle = %q, want %q", got, "hel")
	}
	if got := r.Get(h); !bytes.Equal(got, []byte("hello")) {
		t.Errorf("Get via stale handle = %q, want %q", got, "hello")
	}
	gotNow, ok := r.Handle(id)
	if !ok || gotNow != shorter {
		t.Errorf("Handle(id) = %x (ok %v), want updated handle %x", gotNow, ok, shorter)
	}

	// Unknown ID.
	if _, ok := r.UpdateHandle(Handle(0xAAAA_BBBB_CCCC_DDDD)); ok {
		t.Error("UpdateHandle(unknown ID) = ok, want false")
	}

	// Same ID but different slot index is refused.
	relocated, _ := spec.Handle(int(h.Offset())+1, 3)
	relocated, _ = spec.SetID(relocated, id)
	if _, ok := r.UpdateHandle(relocated); ok {
		t.Error("UpdateHandle(relocated) = ok, want false (slot is identity)")
	}
}

func TestRecordClear(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, true)
	defer r.Close()

	h1, _ := r.Set([]byte("one"))
	h2, _ := r.Set([]byte("two"))

	r.Clear()
	if r.Length() != 0 {
		t.Errorf("Length after Clear = %d, want 0", r.Length())
	}
	if r.Size() != 0 {
		t.Errorf("Size after Clear = %d, want 0", r.Size())
	}
	if keys := r.Keys(); keys != nil {
		t.Errorf("Keys after Clear = %v, want nil", keys)
	}
	if got := r.Get(h1); got != nil || r.Get(h2) != nil {
		t.Error("handles still resolve after Clear")
	}
	if _, ok := r.Handle(spec.ID(h1)); ok {
		t.Error("ID mapping survived Clear")
	}

	// Store is reusable after clearing.
	if h3, err := r.Set([]byte("three")); err != nil {
		t.Errorf("Set after Clear: %v", err)
	} else if got := r.Get(h3); !bytes.Equal(got, []byte("three")) {
		t.Errorf("Get after Clear+Set = %q", got)
	}
}

func TestRecordWriteBulk(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, true)
	defer r.Close()

	blobs := [][]byte{[]byte("abc"), []byte("de"), []byte("fghi")}
	entries := make([]Handle, len(blobs))
	var data []byte
	for i, b := range blobs {
		h, err := spec.Handle(i, len(b)) // slot index == position in data
		if err != nil {
			t.Fatal(err)
		}
		h, _ = spec.SetID(h, uint64(i+1))
		entries[i] = h
		data = append(data, b...)
	}

	if err := r.Write(func() ([]Handle, []byte) { return entries, data }); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if r.Length() != len(blobs) {
		t.Fatalf("Length after Write = %d, want %d", r.Length(), len(blobs))
	}
	if r.Size() != int64(len(data)) {
		t.Errorf("Size after Write = %d, want %d", r.Size(), len(data))
	}
	for i, h := range entries {
		if got := r.Get(h); !bytes.Equal(got, blobs[i]) {
			t.Errorf("entry %d = %q, want %q", i, got, blobs[i])
		}
		if got, ok := r.Handle(spec.ID(h)); !ok || got != h {
			t.Errorf("Handle(id of entry %d) not found", i)
		}
	}

	// Keys must round-trip through Get after a bulk Write with non-contiguous
	// slot indices.
	keys := r.Keys()
	if len(keys) != len(entries) {
		t.Fatalf("Keys len = %d, want %d", len(keys), len(entries))
	}
	for _, k := range keys {
		if r.Get(k) == nil {
			t.Errorf("Key %x has no bytes after Write", k)
		}
	}
}

func TestRecordWriteEmpty(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	if err := r.Write(func() ([]Handle, []byte) { return nil, nil }); err != nil {
		t.Fatalf("empty Write: %v", err)
	}
	if r.Length() != 0 || r.Size() != 0 {
		t.Errorf("empty Write: Length=%d Size=%d, want 0/0", r.Length(), r.Size())
	}
}

func TestRecordWriteMaxValidIndex(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	// The largest representable slot index (MaxOffset-1) must round-trip
	// through a bulk Write. Note the 16-bit offset field means index 2^16
	// would wrap to 0, so Write's index guard is defense-in-depth rather
	// than reachable through the public Handle encoding.
	h, err := spec.Handle(MaxOffset-1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Write(func() ([]Handle, []byte) { return []Handle{h}, []byte("xyz") }); err != nil {
		t.Fatalf("Write with max index: %v", err)
	}
	if got := r.Get(h); !bytes.Equal(got, []byte("xyz")) {
		t.Errorf("Get = %q, want %q", got, "xyz")
	}
}

func TestRecordWriteRejectsDataOverflow(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	// Each declared length (3 GiB) is representable alone, but the two
	// together exceed maxDataOffset — without the guard the uint32 cursor
	// would wrap and the second slot would silently point into the first
	// entry's bytes.
	part1, err := spec.Handle(0, 0xC0000000)
	if err != nil {
		t.Fatal(err)
	}
	part2, err := spec.Handle(1, 0xC0000000)
	if err != nil {
		t.Fatal(err)
	}
	got := r.Write(func() ([]Handle, []byte) {
		return []Handle{part1, part2}, make([]byte, 1) // data content is irrelevant to the guard
	})
	if !errors.Is(got, ErrDataExceedsCapacity) {
		t.Errorf("Write with > 4 GiB total length = %v, want %v", got, ErrDataExceedsCapacity)
	}
	if r.Length() != 0 {
		t.Errorf("Length after rejected Write = %d, want 0", r.Length())
	}
}

func TestRecordCloneIndependence(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	h1, _ := r.Set([]byte("alpha"))
	h2, _ := r.Set([]byte("beta"))

	clone := r.Clone()
	defer clone.Close()

	// The clone resolves the same handles at clone time.
	if !bytes.Equal(clone.Get(h1), []byte("alpha")) || !bytes.Equal(clone.Get(h2), []byte("beta")) {
		t.Error("clone does not resolve handles present at clone time")
	}
	if clone.Length() != r.Length() {
		t.Errorf("clone Length = %d, want %d", clone.Length(), r.Length())
	}

	// Mutating one never affects the other.
	hc, _ := clone.Set([]byte("gamma"))
	if r.Length() != 2 {
		t.Errorf("original Length after clone Set = %d, want 2", r.Length())
	}
	if !bytes.Equal(r.Get(hc), nil) {
		t.Error("original resolves clone-only handle")
	}
	if !r.Delete(h1) {
		t.Fatal("original Delete(h1) failed")
	}
	if !bytes.Equal(clone.Get(h1), []byte("alpha")) {
		t.Error("clone affected by original Delete")
	}

	// Clones deep-copy the header/entries/idIndex: a delete on the clone
	// must leave the original fully intact.
	clone.Delete(h2)
	if !bytes.Equal(r.Get(h2), []byte("beta")) {
		t.Error("original affected by clone Delete")
	}
}

func TestRecordStats(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, true) // assign IDs so idIndex tracks one key per entry
	defer r.Close()

	data := []byte("0123456789")
	sets := make([]Handle, 4)
	for i := range sets {
		h, err := r.Set(data)
		if err != nil {
			t.Fatal(err)
		}
		sets[i] = h
	}

	st := r.Stats()
	if st.Entries != 4 {
		t.Errorf("Entries = %d, want 4", st.Entries)
	}
	if st.DataBytes != 4*int64(len(data)) {
		t.Errorf("DataBytes = %d, want %d", st.DataBytes, 4*len(data))
	}
	if st.Holes != 0 || st.WastedBytes != 0 {
		t.Errorf("fresh store shows holes: %+v", st)
	}
	if st.FreeSlots != 0 {
		t.Errorf("FreeSlots = %d, want 0 before any delete", st.FreeSlots)
	}
	if st.IDIndexEntries != 4 {
		t.Errorf("IDIndexEntries = %d, want 4", st.IDIndexEntries)
	}

	// Holes and wasted bytes appear as entries are deleted.
	r.Delete(sets[1])
	st = r.Stats()
	if st.Holes != 1 || st.WastedBytes != int64(len(data)) {
		t.Errorf("after delete: Holes=%d WastedBytes=%d, want 1/%d", st.Holes, st.WastedBytes, len(data))
	}
	if st.FreeSlots != 1 {
		t.Errorf("FreeSlots = %d, want 1 after one delete", st.FreeSlots)
	}
	if st.Entries != 3 {
		t.Errorf("Entries = %d, want 3", st.Entries)
	}
	if st.IDIndexEntries != 3 {
		t.Errorf("IDIndexEntries = %d, want 3 after one delete", st.IDIndexEntries)
	}
}

func TestRecordDefragment(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	a, _ := r.Set([]byte("aaaaaaaaaa"))
	b, _ := r.Set([]byte("bbbbbbbbbb"))
	c, _ := r.Set([]byte("cccccccccc"))

	r.Delete(b) // creates a 10-byte hole in the middle

	if r.Stats().Holes != 1 {
		t.Fatalf("Holes = %d, want 1", r.Stats().Holes)
	}

	r.Defragment()
	st := r.Stats()
	if st.Holes != 0 || st.WastedBytes != 0 {
		t.Errorf("post-defrag holes: %+v", st)
	}
	if st.DataBytes != 20 {
		t.Errorf("DataBytes after defrag = %d, want 20 (two 10-byte entries)", st.DataBytes)
	}
	if st.Defragments != 1 {
		t.Errorf("Defragments = %d, want 1", st.Defragments)
	}
	// Handles still resolve and their bytes are intact after relocation.
	if !bytes.Equal(r.Get(a), []byte("aaaaaaaaaa")) {
		t.Error("a relocated wrong")
	}
	if !bytes.Equal(r.Get(c), []byte("cccccccccc")) {
		t.Error("c relocated wrong")
	}
	// A second defrag with no holes is a no-op.
	r.Defragment()
	if r.Stats().Defragments != 1 {
		t.Errorf("Defragments = %d, want still 1", r.Stats().Defragments)
	}
}

func TestRecordDefragmentEmpty(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	a, _ := r.Set([]byte("x"))
	b, _ := r.Set([]byte("y"))
	r.Delete(a)
	r.Delete(b)

	r.Defragment()
	if st := r.Stats(); st.Entries != 0 || st.DataBytes != 0 || st.Holes != 0 {
		t.Errorf("post-defrag empty store: %+v", st)
	}
}

func TestRecordIDIndexCompaction(t *testing.T) {
	spec := mustSpec(t, 32)
	type state struct {
		next int
	}
	st := &state{}
	r := NewRecord(Config{
		HandleSpec:          spec,
		CompactionThreshold: 2,
		OnSetID: func(h Handle, width uint8) Handle {
			st.next++
			out, err := spec.SetID(h, uint64(st.next))
			if err != nil {
				panic(err)
			}
			return out
		},
	})
	defer r.Close()

	if r.compactionThreshold != 2 {
		t.Fatalf("compactionThreshold = %d, want 2", r.compactionThreshold)
	}

	// 6 churn cycles: 2 deletions per compaction, threshold 2 → ≥3 compactions.
	for i := 0; i < 6; i++ {
		hh, err := r.Set([]byte(fmt.Sprintf("v%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		r.Delete(hh)
	}
	if r.compactions == 0 {
		t.Error("idIndex was never compacted")
	}
	if r.tombstones != 0 {
		t.Errorf("tombstones = %d, want 0 after compaction", r.tombstones)
	}
}

func TestRecordDefaultCompactionThreshold(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()
	if r.compactionThreshold != defaultCompactionThreshold {
		t.Errorf("compactionThreshold = %d, want default %d", r.compactionThreshold, defaultCompactionThreshold)
	}
}

func TestRecordOnWriteLengthMismatch(t *testing.T) {
	spec := mustSpec(t, 32)
	r := NewRecord(Config{
		HandleSpec: spec,
		OnWrite: func(h Handle, start uint, data []byte) (Handle, []byte) {
			return h, append(append([]byte(nil), data...), 0xFF) // different length
		},
	})
	defer r.Close()

	h, err := r.Set([]byte("abc"))
	if err == nil {
		t.Fatalf("expected length-mismatch error, got handle %x", h)
	}
	if r.Length() != 0 {
		t.Errorf("Length after failed Set = %d, want 0", r.Length())
	}
	// The reserved slot must have been returned to the free list: live count 0,
	// exactly one free slot awaiting reuse, nothing leaked as a live entry.
	if st := r.Stats(); st.Entries != 0 || st.FreeSlots != 1 {
		t.Errorf("stats after failed Set = %+v, want 0 live entries and 1 free slot", st)
	}
}

func TestRecordOnWriteRewritesEntry(t *testing.T) {
	spec := mustSpec(t, 32)
	next := 0
	r := NewRecord(Config{
		HandleSpec: spec,
		OnWrite: func(h Handle, start uint, data []byte) (Handle, []byte) {
			next++
			nh, err := spec.SetID(h, uint64(next))
			if err != nil {
				panic(err)
			}
			return nh, data
		},
	})
	defer r.Close()

	h, err := r.Set([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Handle(spec.ID(h)); !ok || got != h {
		t.Errorf("Handle(id stamped by onWrite) missing: got %x ok %v", got, ok)
	}
	if !bytes.Equal(r.Get(h), []byte("payload")) {
		t.Error("payload corrupted by onWrite rewrite")
	}
}

// TestRecordBackgroundDefrag verifies that a Delete creating a real hole wakes
// the lazily-started background defrag goroutine, which eventually compacts.
func TestRecordBackgroundDefrag(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	r.SetDefragDelay(1 * time.Millisecond)

	a, _ := r.Set([]byte("aaaaaaaaaa"))
	mid, _ := r.Set([]byte("bbbbbbbbbb"))
	c, _ := r.Set([]byte("cccccccccc"))

	// No Delete yet → background goroutine must not exist.
	if r.defragStarted {
		t.Fatal("defrag goroutine started before any delete")
	}
	r.Delete(mid)

	deadline := time.Now().Add(2 * time.Second)
	for r.Stats().Defragments == 0 {
		if time.Now().After(deadline) {
			t.Fatal("background defrag never ran")
		}
		time.Sleep(1 * time.Millisecond)
	}
	st := r.Stats()
	if st.DataBytes != 20 {
		t.Errorf("DataBytes after background defrag = %d, want 20", st.DataBytes)
	}
	if !bytes.Equal(r.Get(a), []byte("aaaaaaaaaa")) || !bytes.Equal(r.Get(c), []byte("cccccccccc")) {
		t.Error("entry bytes corrupted by background defrag")
	}
	r.Close()
}

func TestRecordCloseSafe(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	// Never deleted-from: Close must return without waiting on a goroutine.
	r.Close()
	// And the store stays usable afterwards.
	if _, err := r.Set([]byte("still works")); err != nil {
		t.Errorf("Set after Close: %v", err)
	}
	r.Close()
}

func TestRecordConcurrentAccess(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, true)
	defer r.Close()

	const goroutines = 8
	const perGoroutine = 100

	var mu sync.Mutex
	var handles []Handle
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				payload := []byte(fmt.Sprintf("g%d-i%d", g, i))
				h, err := r.Set(payload)
				if err != nil {
					t.Errorf("Set: %v", err)
					return
				}
				mu.Lock()
				handles = append(handles, h)
				mu.Unlock()
			}
		}(g)
	}
	wg.Wait()

	if r.Length() != goroutines*perGoroutine {
		t.Errorf("Length = %d, want %d", r.Length(), goroutines*perGoroutine)
	}
	for _, h := range handles {
		if r.Get(h) == nil {
			t.Errorf("handle %x lost its bytes", h)
		}
	}
	// Concurrent readers must not corrupt the store.
	var rwg sync.WaitGroup
	for g := 0; g < 4; g++ {
		rwg.Add(1)
		go func() {
			defer rwg.Done()
			for range handles {
				r.Keys()
				r.Length()
				r.Stats()
			}
		}()
	}
	rwg.Wait()
}

func TestRecordEmptyBlob(t *testing.T) {
	spec := mustSpec(t, 32)
	r := newRecord(t, spec, false)
	defer r.Close()

	h, err := r.Set(nil)
	if err != nil {
		t.Fatalf("Set(nil): %v", err)
	}
	if got := r.Get(h); len(got) != 0 {
		t.Errorf("Get(empty) = %q, want empty", got)
	}
	if r.Size() != 0 {
		t.Errorf("Size = %d, want 0 for empty blob", r.Size())
	}
}
