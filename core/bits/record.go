package bits

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// defaultCompactionThreshold is the number of tombstoned (deleted) entries
// accumulated in idIndex before it is rebuilt fresh. Go maps never shrink
// their backing bucket array after deletions, so a long-lived Record under
// sustained ID churn would otherwise carry a growing amount of dead bucket
// memory forever.
//
// Earlier versions of this store also kept an `indir` map (for stale-handle
// indirection after a move) and a `posIndex` map (byte-offset -> entry).
// Both are gone: `indir` is unnecessary now that a Handle's Offset() is a
// stable slot index rather than a byte position that moves under it, and
// `posIndex` is a plain slice (slotToEntry) rather than a hash map, so
// neither needs compaction. idIndex is the only map left that does.
const defaultCompactionThreshold = 256

// OnWrite is called when a blob is placed at a given physical offset.
// It receives the handle, the new physical start, and the data slice.
// It may modify the handle's Length/ID bits or the data in-place and
// returns the updated handle and slice, but must not change the handle's
// Offset() — that's the entry's slot index, which is its fixed identity
// and never moves.
type OnWrite func(h Handle, newStart uint, data []byte) (Handle, []byte)

// Config configures the Record data store.
type Config struct {
	OnSetID    func(h Handle, width uint8) Handle
	OnWrite    OnWrite
	HandleSpec HandleSpec

	// CompactionThreshold overrides the number of tombstoned idIndex entries
	// tolerated before it is rebuilt fresh. <= 0 uses defaultCompactionThreshold.
	CompactionThreshold int
	// Cycle at which automatic defragementation
	DefragmentationPeriod time.Duration
}

// RecordStats is a point-in-time snapshot of a Record's internal health,
// suitable for exporting to metrics or for deciding whether a given Record
// has drifted from "mostly fixed" into "churn-heavy" territory.
type RecordStats struct {
	Entries int
	Holes   int

	DataBytes   int64 // len(data)
	WastedBytes int64 // sum of hole lengths within DataBytes
	HeaderBytes int64 // size in bytes of the packed slot-table header

	FreeSlots int // slot indices freed and awaiting reuse before the header must grow

	IDIndexEntries int

	Tombstones  int    // pending idIndex deletions not yet compacted out
	Compactions uint64 // lifetime count of idIndex rebuilds triggered by tombstone accumulation
	Defragments uint64 // lifetime count of completed defragmentation passes
}

var (
	// ErrTooManyEntries is returned when every slot index in the header's
	// addressable range (0..MaxOffset-1) is already in use.
	ErrTooManyEntries = errors.New("record: too many live entries; header slot capacity exhausted")
	// ErrDataExceedsCapacity is returned when appending (Set) or a bulk
	// replacement (Write) would grow `data` past the largest byte offset a
	// header slot can represent.
	ErrDataExceedsCapacity = errors.New("record: data would exceed maximum addressable size")
)

// ---------- Header format ----------
//
// header is a single []byte, word size 8 bytes (uint64, little-endian).
// It is the sole source of truth for where an entry's bytes currently live
// in `data` — a Handle's Offset() is a stable slot index into this table,
// never a byte position, so an entry's handle never needs to change when
// its bytes move (e.g. during defragmentation).
//
// Word 0 (always present):
//
//	bits 0-1   prefixLenCode — actual prefix length in words is prefixLenCode+1 (1..4).
//	           This implementation always writes 2 (3 words), leaving room
//	           for a future version to grow the prefix without breaking
//	           readers that only understand word 0.
//	bits 2-9   format version (headerFormatVersion)
//	bits 10-17 the HandleSpec's configured length-bit width, for validating
//	           a header against the spec that's about to read it
//	bits 18-63 reserved
//
// Word 1 (present since prefixLen >= 2):
//
//	bits 0-31  liveCount — number of currently-allocated (live) slots
//	bits 32-63 freeHead  — index of the first free slot, or freeListNil if none
//
// Word 2 (present since prefixLen >= 3):
//
//	bits 0-31  slotCount — number of slot words following the prefix
//	bits 32-63 reserved
//
// Words prefixLen..end: one word per slot index.
//
//	bits 0-31  offset     — current byte position in `data` (slot is alive),
//	           or the index of the next free slot (slot is free;
//	           freeListNil marks the end of the list). Free slots are
//	           threaded into an intrusive linked list through this field
//	           rather than tracked in a separate structure.
//	bits 32-62 generation — bumped every time the slot is allocated or freed.
//	           This guards against corruption of the free list itself (see
//	           allocSlotLocked) but is not a full ABA guard against stale
//	           external handles: Length and ID already occupy every bit a
//	           Handle has left, so there's no room left in a Handle to carry
//	           a generation for Get()/Delete() to check against. A caller
//	           that holds a Handle past its Delete() and it gets reused can
//	           still alias a new entry, exactly as before this change.
//	bit  63    alive — 1 if the slot currently holds a live entry.
const (
	headerWordBytes = 8

	headerFormatVersion = 1
	headerPrefixWords   = 3 // word 0 (meta) + word 1 (liveCount/freeHead) + word 2 (slotCount)

	prefixLenCodeMask  = 0x3
	formatVersionShift = 2
	formatVersionMask  = 0xFF
	specLenBitsShift   = 10
	specLenBitsMask    = 0xFF

	slotGenShift = 32
	slotGenMask  = 0x7FFFFFFF // 31 bits
	slotAliveBit = uint64(1) << 63

	// freeListNil marks "no next free slot". It's outside the valid slot
	// index range (0..MaxOffset-1, MaxOffset == 65536) with enormous
	// headroom, since a slot's offset/next-free field is a full 32 bits
	// even though Handle.Offset() only exposes 16 of them today.
	freeListNil = uint32(0xFFFFFFFF)

	// maxDataOffset is the largest byte offset representable in a header
	// slot's 32-bit offset field, and therefore the largest size `data`
	// can grow to.
	maxDataOffset = uint64(1)<<32 - 1
)

func readWord(h []byte, word int) uint64 {
	off := word * headerWordBytes
	return binary.LittleEndian.Uint64(h[off : off+headerWordBytes])
}

func writeWord(h []byte, word int, v uint64) {
	off := word * headerWordBytes
	binary.LittleEndian.PutUint64(h[off:off+headerWordBytes], v)
}

func headerPrefixLen(h []byte) int {
	return int(readWord(h, 0)&prefixLenCodeMask) + 1
}

func headerLiveCount(h []byte) uint32 { return uint32(readWord(h, 1)) }
func headerFreeHead(h []byte) uint32  { return uint32(readWord(h, 1) >> 32) }
func headerSlotCount(h []byte) uint32 { return uint32(readWord(h, 2)) }

func setHeaderLiveCount(h []byte, v uint32) {
	w := readWord(h, 1)
	w = (w &^ uint64(0xFFFFFFFF)) | uint64(v)
	writeWord(h, 1, w)
}

func setHeaderFreeHead(h []byte, v uint32) {
	w := readWord(h, 1)
	w = (w &^ (uint64(0xFFFFFFFF) << 32)) | (uint64(v) << 32)
	writeWord(h, 1, w)
}

func setHeaderSlotCount(h []byte, v uint32) {
	w := readWord(h, 2)
	w = (w &^ uint64(0xFFFFFFFF)) | uint64(v)
	writeWord(h, 2, w)
}

func slotWordIndex(prefixLen, index int) int { return prefixLen + index }

// readSlot decodes a slot word. When alive is false, offsetOrNext is the
// index of the next free slot (or freeListNil) rather than a byte position.
func readSlot(h []byte, prefixLen, index int) (offsetOrNext uint32, generation uint32, alive bool) {
	w := readWord(h, slotWordIndex(prefixLen, index))
	offsetOrNext = uint32(w)
	generation = uint32((w >> slotGenShift) & slotGenMask)
	alive = w&slotAliveBit != 0
	return
}

func writeSlot(h []byte, prefixLen, index int, offsetOrNext uint32, generation uint32, alive bool) {
	w := uint64(offsetOrNext)
	w |= uint64(generation&slotGenMask) << slotGenShift
	if alive {
		w |= slotAliveBit
	}
	writeWord(h, slotWordIndex(prefixLen, index), w)
}

// newHeader builds a fresh, empty header: a 3-word prefix (format/version
// and the spec's configured length-bit width; live-count and free-list
// head; slot count) followed by zero slot words. Slot words are appended
// one at a time as new indices are needed (see allocSlotLocked) and are
// never removed — freed slots are threaded onto the intrusive free list
// instead, so the header only grows past its previous high-water mark of
// concurrently-live entries.
func newHeader(lengthBits uint8) []byte {
	h := make([]byte, headerPrefixWords*headerWordBytes)

	word0 := uint64(headerPrefixWords-1) & prefixLenCodeMask
	word0 |= (uint64(headerFormatVersion) & formatVersionMask) << formatVersionShift
	word0 |= (uint64(lengthBits) & specLenBitsMask) << specLenBitsShift
	writeWord(h, 0, word0)

	writeWord(h, 1, uint64(freeListNil)<<32) // liveCount = 0, freeHead = nil
	writeWord(h, 2, 0)                       // slotCount = 0

	return h
}

// hole describes a free byte range within `data`, available for reuse by a
// future Set(). Unlike in earlier versions of this store, holes are no
// longer packed into a Handle-shaped value — a Handle's Offset() is now a
// slot index, not a byte position, so it can't double as a hole descriptor.
type hole struct {
	offset uint32
	length uint32
}

// Record is a zero-copy blob store keyed by packed Handles.
//
// Concurrency model: all reads take an RLock and all mutations take a full
// Lock, so concurrent reads never block each other and never race with the
// internal state below. Mutations are rare relative to reads for the
// primary use case (mostly-fixed, built-once schemas), so a plain RWMutex
// gives lock-free-among-readers behavior without paying a copy-on-write tax
// on every write — which would be actively harmful for the churn-heavy
// subset of usage (query-derived schemas), since safely publishing a new
// immutable data buffer on every hole-reuse write would require copying the
// entire buffer, not just the touched region.
//
// A Handle's Offset() is a slot index into header, not a byte position.
// header is the sole source of truth for where that slot's bytes currently
// live in `data`; moving an entry (hole-reuse, defragmentation) only ever
// updates header, so a handle is never invalidated by its bytes moving.
// entries is a dense, live-only slice of every active Handle (swap-remove
// on delete, exactly as in earlier versions) so that Read()/Keys()/Clone()
// keep working as a compact zero-copy view. slotToEntry is a plain slice,
// parallel to header, mapping a slot index to its position within entries
// (or -1 if that slot is currently free) — a cheaper, compaction-free
// replacement for the byte-offset-keyed hash map this used to require.
// idIndex remains a hash map, since caller-assigned IDs are arbitrary and
// not necessarily dense, and is the only structure here that still needs
// periodic compaction (see compactIDIndexLocked).
type Record struct {
	data        []byte
	header      []byte   // packed slot table; see header format doc above
	entries     []Handle // dense, live-only entries (swap-remove on delete)
	slotToEntry []int32  // slot index -> position in entries; -1 if free
	holes       []hole   // free byte ranges within data

	idIndex map[uint64]int // caller ID -> position in entries

	OnSetID func(h Handle, width uint8) Handle
	onWrite OnWrite
	spec    HandleSpec

	mu          sync.RWMutex
	defragDelay atomic.Int64

	defragStarted bool // protected by mu; guards lazy goroutine start and Close()
	defragReq     chan struct{}
	stopDefrag    chan struct{}
	defragDone    chan struct{}

	compactionThreshold int // resolved at construction; always > 0
	tombstones          int
	compactions         uint64
	defragments         uint64
}

// NewRecord creates a new Record instance configured with the provided options.
// The background defrag goroutine is NOT started here — it starts lazily on
// the first Delete() that actually creates a hole, so a Record that is only
// ever written to via Write/Set and never deleted-from never pays for it.
func NewRecord(cfg Config) *Record {
	ct := cfg.CompactionThreshold
	if ct <= 0 {
		ct = defaultCompactionThreshold
	}
	r := &Record{
		OnSetID:             cfg.OnSetID,
		onWrite:             cfg.OnWrite,
		spec:                cfg.HandleSpec,
		header:              newHeader(cfg.HandleSpec.ConfiguredLengthBits()),
		defragReq:           make(chan struct{}, 1),
		stopDefrag:          make(chan struct{}),
		defragDone:          make(chan struct{}),
		compactionThreshold: ct,
	}
	if cfg.DefragmentationPeriod == 0 {
		cfg.DefragmentationPeriod = 0
	}
	r.defragDelay.Store(int64(cfg.DefragmentationPeriod * time.Millisecond))
	return r
}

// ---------- Zero‑copy bulk I/O ----------

// Read provides read‑only access to the entire store's entries and data.
// The callback is invoked while the read lock is held. Do not retain the slices.
func (r *Record) Read(cb func(entries []Handle, data []byte)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cb(r.entries, r.data)
}

// Write atomically replaces the entire store with new entries and data.
// The callback must return the new entries and data slices.
//
// entries must be given in the same order their bytes appear in data —
// entries[0]'s bytes start at offset 0, entries[1]'s bytes start where
// entries[0]'s end, and so on with no gaps. Write derives each entry's
// physical byte position from this ordering plus its encoded Length; it
// does not accept arbitrary/non-contiguous layouts the way byte-offset-keyed
// handles used to allow.
//
// Each handle's own Offset() bits ARE trusted as-is and used as that
// entry's slot index (its identity going forward) — Write does not
// reassign them. Indices must be distinct and below MaxOffset; an index at
// or past MaxOffset yields ErrTooManyEntries. The total of all declared
// Lengths must not exceed maxDataOffset; exceeding it yields
// ErrDataExceedsCapacity instead of letting the uint32 byte cursor wrap and
// corrupt every slot's offset. Both limits are validated before any state
// changes, so a failing Write leaves the store untouched. After a successful
// replacement, header, slotToEntry, and idIndex are rebuilt from scratch to
// match, since the callback bypasses the normal Set/Delete bookkeeping and
// has no relation to any prior handle history.
func (r *Record) Write(cb func() ([]Handle, []byte)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	newEntries, newData := cb()

	// Validate before touching any state: an over-limit Write leaves the
	// store exactly as it was, not half-rebuilt.
	var maxIndex int
	var total uint64
	for _, h := range newEntries {
		if idx := int(h.Offset()); idx > maxIndex {
			maxIndex = idx
		}
		total += uint64(r.spec.Length(h))
	}
	if maxIndex >= MaxOffset {
		return ErrTooManyEntries
	}
	if total > maxDataOffset {
		return ErrDataExceedsCapacity
	}

	r.data = newData
	r.holes = r.holes[:0]

	r.header = newHeader(r.spec.ConfiguredLengthBits())
	r.slotToEntry = nil
	r.idIndex = nil
	r.entries = newEntries

	if len(newEntries) > 0 {
		slotCount := maxIndex + 1
		for i := r.slotCount(); i < slotCount; i++ {
			r.header = append(r.header, make([]byte, headerWordBytes)...)
		}
		setHeaderSlotCount(r.header, uint32(slotCount))
		setHeaderLiveCount(r.header, uint32(len(newEntries)))

		r.slotToEntry = make([]int32, slotCount)
		for i := range r.slotToEntry {
			r.slotToEntry[i] = -1
		}
		r.idIndex = make(map[uint64]int, len(newEntries))

		prefixLen := r.prefixLen()
		cursor := uint32(0)
		for pos, h := range newEntries {
			index := int(h.Offset())
			length := uint32(r.spec.Length(h))
			writeSlot(r.header, prefixLen, index, cursor, 0, true)
			r.slotToEntry[index] = int32(pos)
			r.idIndex[r.spec.ID(h)] = pos
			cursor += length
		}
	}

	r.tombstones = 0

	select {
	case <-r.defragReq:
	default:
	}
	return nil
}

// Keys returns a copy of all active handles.
func (r *Record) Keys() []Handle {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
		return nil
	}
	keys := make([]Handle, len(r.entries))
	copy(keys, r.entries)
	return keys
}

// Length returns the number of active entries in the store.
func (r *Record) Length() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// Size returns the current size in bytes of the underlying data buffer.
func (r *Record) Size() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.data))
}

// Stats returns a point-in-time snapshot of the store's internal health.
func (r *Record) Stats() RecordStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var wasted int64
	for _, hl := range r.holes {
		wasted += int64(hl.length)
	}

	return RecordStats{
		Entries:        len(r.entries),
		Holes:          len(r.holes),
		DataBytes:      int64(len(r.data)),
		WastedBytes:    wasted,
		HeaderBytes:    int64(len(r.header)),
		FreeSlots:      r.slotCount() - int(headerLiveCount(r.header)),
		IDIndexEntries: len(r.idIndex),
		Tombstones:     r.tombstones,
		Compactions:    r.compactions,
		Defragments:    r.defragments,
	}
}

// Clone returns a deep copy of the Record with entirely independent
// underlying storage (data, header, entries, slotToEntry, holes, and
// idIndex). Any handle that resolves correctly against the original at the
// moment of cloning resolves identically against the clone. The two
// Records then evolve completely independently: mutating one never affects
// the other.
//
// The clone does not inherit the original's background defrag goroutine or
// its lifetime stats (Stats().Defragments/Compactions start at zero) — it
// lazily starts its own on its own first real hole, exactly like a Record
// built via NewRecord.
func (r *Record) Clone() *Record {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clone := &Record{
		OnSetID:             r.OnSetID,
		onWrite:             r.onWrite,
		spec:                r.spec,
		defragReq:           make(chan struct{}, 1),
		stopDefrag:          make(chan struct{}),
		defragDone:          make(chan struct{}),
		compactionThreshold: r.compactionThreshold,
	}
	clone.defragDelay.Store(r.defragDelay.Load())

	if len(r.data) > 0 {
		clone.data = append([]byte(nil), r.data...)
	}
	if len(r.header) > 0 {
		clone.header = append([]byte(nil), r.header...)
	}
	if len(r.entries) > 0 {
		clone.entries = append([]Handle(nil), r.entries...)
	}
	if len(r.slotToEntry) > 0 {
		clone.slotToEntry = append([]int32(nil), r.slotToEntry...)
	}
	if len(r.holes) > 0 {
		clone.holes = append([]hole(nil), r.holes...)
	}
	if r.idIndex != nil {
		clone.idIndex = make(map[uint64]int, len(r.idIndex))
		for k, v := range r.idIndex {
			clone.idIndex[k] = v
		}
	}

	return clone
}

// ---------- Configuration ----------

func (r *Record) SetDefragDelay(d time.Duration) {
	r.defragDelay.Store(int64(d))
}

// Close stops the background defrag goroutine if it was ever started.
// It is always safe to call, including on a Record that was never
// deleted-from and therefore never started a background goroutine.
func (r *Record) Close() {
	r.mu.Lock()
	started := r.defragStarted
	r.mu.Unlock()
	if !started {
		return
	}
	close(r.stopDefrag)
	<-r.defragDone
}

// ---------- Public API ----------

func (r *Record) Set(b []byte) (Handle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	need := len(b)
	maxLen := r.spec.MaxLength()
	if uint64(need) > maxLen {
		return 0, fmt.Errorf("%w: requested %d, max allowed for %d bits is %d",
			ErrLengthExceedsBitSpace, need, r.spec.ConfiguredLengthBits(), maxLen)
	}

	index, err := r.allocSlotLocked()
	if err != nil {
		return 0, err
	}

	// 1. Check existing holes (linear scan: hole lists are expected to stay
	// small and transient, so a sorted/indexed structure isn't worth the
	// added complexity here).
	for i := 0; i < len(r.holes); i++ {
		hl := r.holes[i]
		if uint64(hl.length) < uint64(need) {
			continue
		}

		lastIdx := len(r.holes) - 1
		r.holes[i] = r.holes[lastIdx]
		r.holes = r.holes[:lastIdx]

		h, err := r.spec.Handle(index, need)
		if err != nil {
			r.freeSlotLocked(index)
			return 0, err
		}
		if r.OnSetID != nil {
			h = r.OnSetID(h, r.spec.BitSpace())
		}

		dataToWrite := b
		if r.onWrite != nil {
			h, dataToWrite = r.onWrite(h, uint(hl.offset), b)
			if len(dataToWrite) != need {
				r.freeSlotLocked(index)
				return 0, errors.New("onWrite returned different length")
			}
		}
		copy(r.data[hl.offset:hl.offset+uint32(need)], dataToWrite)
		r.setSlotOffsetLocked(index, hl.offset)

		if hl.length > uint32(need) {
			r.holes = append(r.holes, hole{
				offset: hl.offset + uint32(need),
				length: hl.length - uint32(need),
			})
		}

		r.pushEntryLocked(index, h)
		return h, nil
	}

	// 2. Append to end of data store.
	newStart := uint32(len(r.data))
	if uint64(newStart)+uint64(need) > maxDataOffset {
		r.freeSlotLocked(index)
		return 0, ErrDataExceedsCapacity
	}

	h, err := r.spec.Handle(index, need)
	if err != nil {
		r.freeSlotLocked(index)
		return 0, err
	}
	if r.OnSetID != nil {
		h = r.OnSetID(h, r.spec.BitSpace())
	}

	dataToWrite := b
	if r.onWrite != nil {
		h, dataToWrite = r.onWrite(h, uint(newStart), b)
		if len(dataToWrite) != need {
			r.freeSlotLocked(index)
			return 0, errors.New("onWrite returned different length")
		}
	}

	r.data = append(r.data, dataToWrite...)
	r.setSlotOffsetLocked(index, newStart)
	r.pushEntryLocked(index, h)
	return h, nil
}

func (r *Record) Get(h Handle) []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	index := int(h.Offset())
	length := uint32(r.spec.Length(h))

	byteOffset, ok := r.slotOffsetLocked(index)
	if !ok {
		return nil
	}
	if uint64(byteOffset)+uint64(length) > uint64(len(r.data)) {
		return nil
	}
	return r.data[byteOffset : byteOffset+length]
}

func (r *Record) Delete(h Handle) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := int(h.Offset())
	if index < 0 || index >= len(r.slotToEntry) {
		return false
	}
	pos := int(r.slotToEntry[index])
	if pos < 0 {
		return false
	}

	e := r.entries[pos]
	lastPos := len(r.entries) - 1
	if pos != lastPos {
		moved := r.entries[lastPos]
		r.entries[pos] = moved
		r.slotToEntry[int(moved.Offset())] = int32(pos)
		r.setIDIndex(r.spec.ID(moved), pos)
	}
	r.entries = r.entries[:lastPos]
	r.slotToEntry[index] = -1

	r.deleteIDIndex(r.spec.ID(e))

	byteOffset, _ := r.slotOffsetLocked(index)
	length := uint32(r.spec.Length(e))
	r.freeSlotLocked(index)

	r.holes = append(r.holes, hole{offset: byteOffset, length: length})
	r.triggerDefragLocked()
	return true
}

// Handle looks up the current handle for a previously-assigned ID.
// Only meaningful when entries carry unique IDs (typically via OnSetID or
// explicit HandleSpec.SetID) — if IDs are left at their zero default, only
// one entry is tracked by ID at a time.
func (r *Record) Handle(id uint64) (Handle, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	idx, ok := r.lookupID(id)
	if !ok {
		return 0, false
	}
	return r.entries[idx], true
}

// UpdateHandle overwrites the stored handle for the entry with the ID
// embedded in handle, and returns the previous handle for that entry.
// Reports false if no entry with that ID exists.
//
// A slot index (Offset()) is now an entry's fixed identity — the header is
// the sole source of truth for where an entry's bytes physically live, so
// relocation is no longer expressed through handles at all, and
// UpdateHandle refuses (returns false) if handle's Offset() doesn't match
// the existing entry's. What's left for this method to do is narrower than
// before: amend an entry's declared Length (its ID can't change, since ID
// is how the existing entry is found in the first place).
func (r *Record) UpdateHandle(handle Handle) (Handle, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.spec.ID(handle)
	pos, ok := r.lookupID(id)
	if !ok {
		return 0, false
	}

	old := r.entries[pos]
	if handle.Offset() != old.Offset() {
		return 0, false
	}

	r.entries[pos] = handle
	return old, true
}

func (r *Record) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data = r.data[:0]
	r.header = newHeader(r.spec.ConfiguredLengthBits())
	r.entries = r.entries[:0]
	r.slotToEntry = r.slotToEntry[:0]
	r.holes = r.holes[:0]
	r.idIndex = nil
	r.tombstones = 0

	select {
	case <-r.defragReq:
	default:
	}
}

func (r *Record) Defragment() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defragmentLocked()
}

// ---------- Slot table helpers ----------
// header is never nil: NewRecord, Write, and Clear all initialize it via
// newHeader, since every entry — not just ones that survive a move — needs
// it to find its bytes.

func (r *Record) prefixLen() int { return headerPrefixLen(r.header) }
func (r *Record) slotCount() int { return int(headerSlotCount(r.header)) }

// slotOffsetLocked returns the current byte offset for a live slot index,
// or false if the index is out of range or currently free.
func (r *Record) slotOffsetLocked(index int) (uint32, bool) {
	if index < 0 || index >= r.slotCount() {
		return 0, false
	}
	offset, _, alive := readSlot(r.header, r.prefixLen(), index)
	if !alive {
		return 0, false
	}
	return offset, true
}

func (r *Record) setSlotOffsetLocked(index int, offset uint32) {
	prefixLen := r.prefixLen()
	_, gen, _ := readSlot(r.header, prefixLen, index)
	writeSlot(r.header, prefixLen, index, offset, gen, true)
}

// allocSlotLocked reserves a slot index for a new entry: it reuses a freed
// index from the intrusive free list when one is available (O(1), no
// header growth), or grows the header by one word when the free list is
// empty. Must be called with mu held.
func (r *Record) allocSlotLocked() (int, error) {
	prefixLen := r.prefixLen()
	freeHead := headerFreeHead(r.header)

	if freeHead != freeListNil {
		idx := int(freeHead)
		nextFree, _, alive := readSlot(r.header, prefixLen, idx)
		if alive {
			panic("bits: corrupt free list — head slot marked alive")
		}
		setHeaderFreeHead(r.header, nextFree)
		setHeaderLiveCount(r.header, headerLiveCount(r.header)+1)
		return idx, nil
	}

	count := r.slotCount()
	if count >= MaxOffset {
		return 0, ErrTooManyEntries
	}
	r.header = append(r.header, make([]byte, headerWordBytes)...)
	setHeaderSlotCount(r.header, uint32(count+1))
	setHeaderLiveCount(r.header, headerLiveCount(r.header)+1)
	r.slotToEntry = append(r.slotToEntry, -1)
	return count, nil
}

// freeSlotLocked returns a slot to the free list, threading it onto the
// head via the slot's own offset field, and bumps its generation. Must be
// called with mu held; the caller is responsible for having already
// removed the slot's entry from entries/slotToEntry/idIndex.
func (r *Record) freeSlotLocked(index int) {
	prefixLen := r.prefixLen()
	_, gen, _ := readSlot(r.header, prefixLen, index)
	nextGen := (gen + 1) & slotGenMask
	freeHead := headerFreeHead(r.header)
	writeSlot(r.header, prefixLen, index, freeHead, nextGen, false)
	setHeaderFreeHead(r.header, uint32(index))
	setHeaderLiveCount(r.header, headerLiveCount(r.header)-1)
}

// pushEntryLocked appends a newly-allocated entry to the dense entries
// slice and records its slot index's position within it.
func (r *Record) pushEntryLocked(index int, h Handle) {
	pos := len(r.entries)
	r.entries = append(r.entries, h)
	r.slotToEntry[index] = int32(pos)
	r.setIDIndex(r.spec.ID(h), pos)
}

// ---------- ID index helpers ----------
// idIndex is lazily allocated: a Record that is built once (via Write, or
// via Set calls with no intervening Delete) and then only ever read pays
// nothing beyond a nil check for it.

func (r *Record) lookupID(id uint64) (int, bool) {
	if r.idIndex == nil {
		return 0, false
	}
	idx, ok := r.idIndex[id]
	return idx, ok
}

func (r *Record) setIDIndex(id uint64, pos int) {
	if r.idIndex == nil {
		r.idIndex = make(map[uint64]int)
	}
	r.idIndex[id] = pos
}

// deleteIDIndex removes an entry from idIndex if present, counting it as a
// tombstone (triggering compaction once the threshold is crossed).
func (r *Record) deleteIDIndex(id uint64) {
	if r.idIndex == nil {
		return
	}
	if _, ok := r.idIndex[id]; ok {
		delete(r.idIndex, id)
		r.noteTombstoneLocked()
	}
}

// ---------- idIndex compaction ----------
// Go's map implementation never shrinks its backing bucket array after
// deletions, so a Record under sustained ID churn (repeated Set/Delete
// cycles) would otherwise carry a growing amount of dead bucket memory in
// idIndex indefinitely. noteTombstoneLocked tracks how many deletions have
// accumulated since the last compaction and rebuilds it fresh once the
// threshold is crossed.

// noteTombstoneLocked must be called with mu held, once for every key
// actually removed from idIndex.
func (r *Record) noteTombstoneLocked() {
	r.tombstones++
	if r.tombstones >= r.compactionThreshold {
		r.compactIDIndexLocked()
	}
}

func (r *Record) compactIDIndexLocked() {
	if r.idIndex != nil {
		rebuilt := make(map[uint64]int, len(r.idIndex))
		for k, v := range r.idIndex {
			rebuilt[k] = v
		}
		r.idIndex = rebuilt
	}
	r.tombstones = 0
	r.compactions++
}

// ---------- Background defrag loop ----------

func (r *Record) defragLoop() {
	var timer *time.Timer
	defer close(r.defragDone)

	for {
		select {
		case <-r.stopDefrag:
			if timer != nil {
				timer.Stop()
			}
			return
		case <-r.defragReq:
			delay := time.Duration(r.defragDelay.Load())
			if delay <= 0 {
				r.mu.Lock()
				r.defragmentLocked()
				r.mu.Unlock()
				continue
			}
			if timer == nil {
				timer = time.NewTimer(delay)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(delay)
			}
			select {
			case <-r.stopDefrag:
				timer.Stop()
				return
			case <-timer.C:
				r.mu.Lock()
				r.defragmentLocked()
				r.mu.Unlock()
				timer = nil
			}
		}
	}
}

// triggerDefragLocked requests a background defrag pass, starting the
// background goroutine on first use. Must be called with mu held.
func (r *Record) triggerDefragLocked() {
	if time.Duration(r.defragDelay.Load()) <= 0 {
		return
	}
	if !r.defragStarted {
		r.defragStarted = true
		go r.defragLoop()
	}
	select {
	case r.defragReq <- struct{}{}:
	default:
	}
}

// ---------- Defragmentation ----------

// defragmentLocked compacts `data` by walking entries (order is whatever
// swap-remove has left it in — not necessarily byte-layout order) and
// packing each one's bytes toward the front, in the order entries happens
// to hold them. Only header's per-slot offset needs updating when an entry
// moves: a Handle's Offset() (its slot index) never changes, so entries
// and slotToEntry are untouched by movement — this is the direct payoff of
// making slot index the entry's identity instead of its byte position.
func (r *Record) defragmentLocked() {
	if len(r.holes) == 0 {
		return
	}
	r.defragments++

	if len(r.entries) == 0 {
		r.data = r.data[:0]
		r.holes = r.holes[:0]
		return
	}

	prefixLen := r.prefixLen()
	writePos := uint32(0)

	for pos := 0; pos < len(r.entries); pos++ {
		h := r.entries[pos]
		index := int(h.Offset())
		length := uint32(r.spec.Length(h))

		offset, gen, alive := readSlot(r.header, prefixLen, index)
		if !alive {
			panic("bits: live entry references a free header slot")
		}

		if writePos != offset {
			dataSlice := r.data[offset : offset+length]
			newHandle := h
			if r.onWrite != nil {
				var newData []byte
				newHandle, newData = r.onWrite(h, uint(writePos), dataSlice)
				if uint32(len(newData)) != length {
					panic("onWrite returned different length during defrag")
				}
				if int(newHandle.Offset()) != index {
					panic("bits: onWrite must not change a handle's slot index")
				}
				dataSlice = newData
			}
			copy(r.data[writePos:writePos+length], dataSlice)
			writeSlot(r.header, prefixLen, index, writePos, gen, true)

			if newHandle != h {
				r.entries[pos] = newHandle
				if newID, oldID := r.spec.ID(newHandle), r.spec.ID(h); newID != oldID {
					delete(r.idIndex, oldID)
					r.setIDIndex(newID, pos)
				}
			}
		}

		writePos += length
	}

	r.data = r.data[:writePos]
	r.holes = r.holes[:0]
}
