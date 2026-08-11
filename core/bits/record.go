package bits

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const defaultCompactionThreshold = 256

type OnWrite func(h Handle, newStart uint, data []byte) (Handle, []byte)

type Config struct {
	OnSetID               func(h Handle, width uint8) Handle
	OnWrite               OnWrite
	HandleSpec            HandleSpec
	CompactionThreshold   int
	DefragmentationPeriod time.Duration
}

type RecordStats struct {
	Entries        int
	Holes          int
	DataBytes      int64
	WastedBytes    int64
	HeaderBytes    int64
	FreeSlots      int
	IDIndexEntries int
	Tombstones     int
	Compactions    uint64
	Defragments    uint64
}

var (
	ErrTooManyEntries       = errors.New("record: too many live entries; header slot capacity exhausted")
	ErrDataExceedsCapacity = errors.New("record: data would exceed maximum addressable size")
)

// ---------- Structural Sharing Core Constants & Paged Arrays ----------

const (
	pageSize = 256
	pageMask = pageSize - 1
	pageShift = 8
)

// persistentEntries provides structural sharing for dense entries.
type persistentEntries struct {
	pages [][]Handle
	length int
}

func (pe *persistentEntries) get(i int) Handle {
	return pe.pages[i>>pageShift][i&pageMask]
}

func (pe *persistentEntries) set(i int, h Handle) *persistentEntries {
	pIdx := i >> pageShift
	off := i & pageMask

	newPages := append([][]Handle(nil), pe.pages...)
	targetPage := append([]Handle(nil), pe.pages[pIdx]...)
	targetPage[off] = h
	newPages[pIdx] = targetPage

	return &persistentEntries{
		pages:  newPages,
		length: pe.length,
	}
}

func (pe *persistentEntries) append(h Handle) *persistentEntries {
	newLen := pe.length + 1
	pIdx := pe.length >> pageShift
	off := pe.length & pageMask

	newPages := append([][]Handle(nil), pe.pages...)
	if pIdx < len(newPages) {
		targetPage := append([]Handle(nil), newPages[pIdx]...)
		if off < len(targetPage) {
			targetPage[off] = h
		} else {
			targetPage = append(targetPage, h)
		}
		newPages[pIdx] = targetPage
	} else {
		newPages = append(newPages, []Handle{h})
	}

	return &persistentEntries{
		pages:  newPages,
		length: newLen,
	}
}

func (pe *persistentEntries) pop() *persistentEntries {
	if pe.length == 0 {
		return pe
	}
	newLen := pe.length - 1
	pIdx := newLen >> pageShift
	off := newLen & pageMask

	newPages := append([][]Handle(nil), pe.pages...)
	targetPage := append([]Handle(nil), pe.pages[pIdx][:off]...)
	if len(targetPage) == 0 && len(newPages) > 1 {
		newPages = newPages[:pIdx]
	} else {
		newPages[pIdx] = targetPage
	}

	return &persistentEntries{
		pages:  newPages,
		length: newLen,
	}
}

func (pe *persistentEntries) toSlice() []Handle {
	if pe == nil || pe.length == 0 {
		return nil
	}
	res := make([]Handle, pe.length)
	for i := 0; i < pe.length; i++ {
		res[i] = pe.get(i)
	}
	return res
}

// persistentSlotToEntry provides structural sharing for slot lookup.
type persistentSlotToEntry struct {
	pages [][]int32
	length int
}

func (ps *persistentSlotToEntry) get(i int) int32 {
	if ps == nil || i >= ps.length {
		return -1
	}
	return ps.pages[i>>pageShift][i&pageMask]
}

func (ps *persistentSlotToEntry) set(i int, val int32) *persistentSlotToEntry {
	pIdx := i >> pageShift
	off := i & pageMask

	newPages := append([][]int32(nil), ps.pages...)
	for len(newPages) <= pIdx {
		newPages = append(newPages, make([]int32, pageSize))
		for k := range newPages[len(newPages)-1] {
			newPages[len(newPages)-1][k] = -1
		}
	}

	targetPage := append([]int32(nil), newPages[pIdx]...)
	targetPage[off] = val
	newPages[pIdx] = targetPage

	newLen := ps.length
	if i+1 > newLen {
		newLen = i + 1
	}

	return &persistentSlotToEntry{
		pages:  newPages,
		length: newLen,
	}
}

// ---------- Header Format Constants ----------

const (
	headerWordBytes = 8

	headerFormatVersion = 1
	headerPrefixWords   = 3

	prefixLenCodeMask  = 0x3
	formatVersionShift = 2
	formatVersionMask  = 0xFF
	specLenBitsShift   = 10
	specLenBitsMask    = 0xFF

	slotGenShift = 32
	slotGenMask  = 0x7FFFFFFF
	slotAliveBit = uint64(1) << 63

	freeListNil   = uint32(0xFFFFFFFF)
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

func headerPrefixLen(h []byte) int  { return int(readWord(h, 0)&prefixLenCodeMask) + 1 }
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

func newHeader(lengthBits uint8) []byte {
	h := make([]byte, headerPrefixWords*headerWordBytes)

	word0 := uint64(headerPrefixWords-1) & prefixLenCodeMask
	word0 |= (uint64(headerFormatVersion) & formatVersionMask) << formatVersionShift
	word0 |= (uint64(lengthBits) & specLenBitsMask) << specLenBitsShift
	writeWord(h, 0, word0)

	writeWord(h, 1, uint64(freeListNil)<<32)
	writeWord(h, 2, 0)

	return h
}

func slotOffset(header []byte, index int) (uint32, bool) {
	if index < 0 || index >= int(headerSlotCount(header)) {
		return 0, false
	}
	offset, _, alive := readSlot(header, headerPrefixLen(header), index)
	if !alive {
		return 0, false
	}
	return offset, true
}

func setSlotOffset(header []byte, index int, offset uint32) {
	prefixLen := headerPrefixLen(header)
	_, gen, _ := readSlot(header, prefixLen, index)
	writeSlot(header, prefixLen, index, offset, gen, true)
}

func freeSlot(header []byte, index int) {
	prefixLen := headerPrefixLen(header)
	_, gen, _ := readSlot(header, prefixLen, index)
	nextGen := (gen + 1) & slotGenMask
	freeHead := headerFreeHead(header)
	writeSlot(header, prefixLen, index, freeHead, nextGen, false)
	setHeaderFreeHead(header, uint32(index))
	setHeaderLiveCount(header, headerLiveCount(header)-1)
}

func allocSlot(header []byte, slotToEntry *persistentSlotToEntry) (index int, newHeader []byte, newSlotToEntry *persistentSlotToEntry, err error) {
	freeHead := headerFreeHead(header)

	if freeHead != freeListNil {
		idx := int(freeHead)
		nextFree, _, alive := readSlot(header, headerPrefixLen(header), idx)
		if alive {
			panic("bits: corrupt free list — head slot marked alive")
		}
		setHeaderFreeHead(header, nextFree)
		setHeaderLiveCount(header, headerLiveCount(header)+1)
		return idx, header, slotToEntry, nil
	}

	count := int(headerSlotCount(header))
	if count >= MaxOffset {
		return 0, header, slotToEntry, ErrTooManyEntries
	}
	header = append(header, make([]byte, headerWordBytes)...)
	setHeaderSlotCount(header, uint32(count+1))
	setHeaderLiveCount(header, headerLiveCount(header)+1)
	if slotToEntry == nil {
		slotToEntry = &persistentSlotToEntry{}
	}
	slotToEntry = slotToEntry.set(count, -1)
	return count, header, slotToEntry, nil
}

type hole struct {
	offset uint32
	length uint32
}

// ---------- ID Index ----------

type idTable struct {
	keys                []uint64
	vals                []int32
	state               []uint8
	count               int
	tomb                int
	compactionThreshold int
}

const (
	idStateEmpty uint8 = iota
	idStateOccupied
	idStateTomb
)

const minIDTableCap = 8

func hashUint64(x uint64) uint64 {
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return x
}

func newIDTable(capacityHint, compactionThreshold int) *idTable {
	c := minIDTableCap
	for c < capacityHint {
		c <<= 1
	}
	return &idTable{
		keys:                make([]uint64, c),
		vals:                make([]int32, c),
		state:               make([]uint8, c),
		compactionThreshold: compactionThreshold,
	}
}

func (t *idTable) clone() *idTable {
	return &idTable{
		keys:                append([]uint64(nil), t.keys...),
		vals:                append([]int32(nil), t.vals...),
		state:               append([]uint8(nil), t.state...),
		count:               t.count,
		tomb:                t.tomb,
		compactionThreshold: t.compactionThreshold,
	}
}

func (t *idTable) find(id uint64) (int, bool) {
	mask := uint64(len(t.state) - 1)
	i := hashUint64(id) & mask
	for {
		switch t.state[i] {
		case idStateEmpty:
			return 0, false
		case idStateOccupied:
			if t.keys[i] == id {
				return int(i), true
			}
		}
		i = (i + 1) & mask
	}
}

func (t *idTable) lookup(id uint64) (int32, bool) {
	if t == nil {
		return 0, false
	}
	idx, ok := t.find(id)
	if !ok {
		return 0, false
	}
	return t.vals[idx], true
}

func (t *idTable) capForCount(n int) int {
	c := minIDTableCap
	for c*6/10 < n {
		c <<= 1
	}
	return c
}

func (t *idTable) insertOrUpdateInPlace(id uint64, val int32) {
	mask := uint64(len(t.state) - 1)
	i := hashUint64(id) & mask
	firstTomb := -1
	for {
		switch t.state[i] {
		case idStateEmpty:
			target := i
			if firstTomb >= 0 {
				target = uint64(firstTomb)
				t.tomb--
			}
			t.keys[target] = id
			t.vals[target] = val
			t.state[target] = idStateOccupied
			t.count++
			return
		case idStateTomb:
			if firstTomb < 0 {
				firstTomb = int(i)
			}
		case idStateOccupied:
			if t.keys[i] == id {
				t.vals[i] = val
				return
			}
		}
		i = (i + 1) & mask
	}
}

func (t *idTable) rehashed(newCap int) *idTable {
	nt := newIDTable(newCap, t.compactionThreshold)
	for i, st := range t.state {
		if st == idStateOccupied {
			nt.insertOrUpdateInPlace(t.keys[i], t.vals[i])
		}
	}
	return nt
}

func (t *idTable) insertOrUpdate(id uint64, val int32, compactionThreshold int) *idTable {
	if t == nil {
		nt := newIDTable(minIDTableCap, compactionThreshold)
		nt.insertOrUpdateInPlace(id, val)
		return nt
	}
	if (t.count+t.tomb+1)*10 >= len(t.state)*7 {
		nt := t.rehashed(t.capForCount(t.count + 1))
		nt.insertOrUpdateInPlace(id, val)
		return nt
	}
	nt := t.clone()
	nt.insertOrUpdateInPlace(id, val)
	return nt
}

func (t *idTable) delete(id uint64, compactionThreshold int) (*idTable, bool) {
	if t == nil {
		return t, false
	}
	idx, ok := t.find(id)
	if !ok {
		return t, false
	}
	nt := t.clone()
	nt.state[idx] = idStateTomb
	nt.count--
	nt.tomb++
	if nt.tomb >= compactionThreshold {
		return nt.rehashed(len(nt.state)), true
	}
	return nt, false
}

func appendEntry(entries *persistentEntries, slotToEntry *persistentSlotToEntry, idIndex *idTable, index int, h Handle, spec HandleSpec, compactionThreshold int) (*persistentEntries, *persistentSlotToEntry, *idTable) {
	if entries == nil {
		entries = &persistentEntries{}
	}
	pos := entries.length
	entries = entries.append(h)
	slotToEntry = slotToEntry.set(index, int32(pos))
	idIndex = idIndex.insertOrUpdate(spec.ID(h), int32(pos), compactionThreshold)
	return entries, slotToEntry, idIndex
}

// ---------- Snapshot ----------

type snapshot struct {
	data        []byte
	header      []byte
	entries     *persistentEntries
	slotToEntry *persistentSlotToEntry
	holes       []hole
	idIndex     *idTable

	tombstones  int
	compactions uint64
	defragments uint64
}

type Record struct {
	snap atomic.Pointer[snapshot]

	OnSetID func(h Handle, width uint8) Handle
	onWrite OnWrite

	writeMu sync.Mutex

	defragReq  chan struct{}
	stopDefrag chan struct{}
	defragDone chan struct{}

	defragDelay atomic.Int64

	compactionThreshold int

	defragOnce    sync.Once
	defragStarted atomic.Bool

	spec HandleSpec
}

func NewRecord(cfg Config) *Record {
	ct := cfg.CompactionThreshold
	if ct <= 0 {
		ct = defaultCompactionThreshold
	}
	r := &Record{
		OnSetID:             cfg.OnSetID,
		onWrite:             cfg.OnWrite,
		spec:                cfg.HandleSpec,
		compactionThreshold: ct,
	}
	r.defragDelay.Store(int64(cfg.DefragmentationPeriod))
	r.snap.Store(&snapshot{
		header:      newHeader(cfg.HandleSpec.ConfiguredLengthBits()),
		entries:     &persistentEntries{},
		slotToEntry: &persistentSlotToEntry{},
	})
	return r
}

func (r *Record) Read(cb func(entries []Handle, data []byte)) {
	snap := r.snap.Load()
	cb(snap.entries.toSlice(), snap.data)
}

func (r *Record) Write(cb func() ([]Handle, []byte)) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	newEntries, newData := cb()

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

	header := newHeader(r.spec.ConfiguredLengthBits())
	slotToEntry := &persistentSlotToEntry{}
	entries := &persistentEntries{}
	var idIndex *idTable

	if len(newEntries) > 0 {
		slotCount := maxIndex + 1
		header = append(header, make([]byte, headerWordBytes*slotCount)...)
		setHeaderSlotCount(header, uint32(slotCount))
		setHeaderLiveCount(header, uint32(len(newEntries)))

		prefixLen := headerPrefixLen(header)
		cursor := uint32(0)
		for pos, h := range newEntries {
			index := int(h.Offset())
			length := uint32(r.spec.Length(h))
			writeSlot(header, prefixLen, index, cursor, 0, true)
			entries = entries.append(h)
			slotToEntry = slotToEntry.set(index, int32(pos))
			idIndex = idIndex.insertOrUpdate(r.spec.ID(h), int32(pos), r.compactionThreshold)
			cursor += length
		}
	}

	old := r.snap.Load()
	r.snap.Store(&snapshot{
		data:        newData,
		header:      header,
		entries:     entries,
		slotToEntry: slotToEntry,
		holes:       nil,
		idIndex:     idIndex,
		tombstones:  0,
		compactions: old.compactions,
		defragments: old.defragments,
	})

	r.cancelPendingDefrag()
	return nil
}

func (r *Record) Keys() []Handle {
	snap := r.snap.Load()
	return snap.entries.toSlice()
}

func (r *Record) Length() int {
	snap := r.snap.Load()
	if snap.entries == nil {
		return 0
	}
	return snap.entries.length
}

func (r *Record) Size() int64 {
	return int64(len(r.snap.Load().data))
}

func (r *Record) Stats() RecordStats {
	snap := r.snap.Load()

	var wasted int64
	for _, hl := range snap.holes {
		wasted += int64(hl.length)
	}

	var idEntries, tomb int
	if snap.idIndex != nil {
		idEntries = snap.idIndex.count
		tomb = snap.idIndex.tomb
	}

	eLen := 0
	if snap.entries != nil {
		eLen = snap.entries.length
	}

	return RecordStats{
		Entries:        eLen,
		Holes:          len(snap.holes),
		DataBytes:      int64(len(snap.data)),
		WastedBytes:    wasted,
		HeaderBytes:    int64(len(snap.header)),
		FreeSlots:      int(headerSlotCount(snap.header)) - int(headerLiveCount(snap.header)),
		IDIndexEntries: idEntries,
		Tombstones:     tomb,
		Compactions:    snap.compactions,
		Defragments:    snap.defragments,
	}
}

func (r *Record) Clone() *Record {
	snap := r.snap.Load()

	clone := &Record{
		OnSetID:             r.OnSetID,
		onWrite:             r.onWrite,
		spec:                r.spec,
		compactionThreshold: r.compactionThreshold,
	}
	clone.defragDelay.Store(r.defragDelay.Load())
	clone.snap.Store(snap)

	return clone
}

func (r *Record) SetDefragDelay(d time.Duration) {
	r.defragDelay.Store(int64(d))
}

func (r *Record) Close() {
	if !r.defragStarted.Load() {
		return
	}
	close(r.stopDefrag)
	<-r.defragDone
}

func (r *Record) Set(b []byte) (Handle, error) {
	need := len(b)
	maxLen := r.spec.MaxLength()
	if uint64(need) > maxLen {
		return 0, fmt.Errorf("%w: requested %d, max allowed for %d bits is %d",
			ErrLengthExceedsBitSpace, need, r.spec.ConfiguredLengthBits(), maxLen)
	}

	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	old := r.snap.Load()

	header := append([]byte(nil), old.header...)
	slotToEntry := old.slotToEntry

	index, header, slotToEntry, err := allocSlot(header, slotToEntry)
	if err != nil {
		return 0, err
	}

	entries := old.entries
	idIndex := old.idIndex
	holes := append([]hole(nil), old.holes...)

	for i := 0; i < len(holes); i++ {
		hl := holes[i]
		if uint64(hl.length) < uint64(need) {
			continue
		}

		lastIdx := len(holes) - 1
		holes[i] = holes[lastIdx]
		holes = holes[:lastIdx]

		h, err := r.spec.Handle(index, need)
		if err != nil {
			freeSlot(header, index)
			return 0, err
		}
		if r.OnSetID != nil {
			h = r.OnSetID(h, r.spec.BitSpace())
		}

		data := append([]byte(nil), old.data...)

		dataToWrite := b
		if r.onWrite != nil {
			h, dataToWrite = r.onWrite(h, uint(hl.offset), b)
			if len(dataToWrite) != need {
				freeSlot(header, index)
				return 0, errors.New("onWrite returned different length")
			}
		}
		copy(data[hl.offset:hl.offset+uint32(need)], dataToWrite)
		setSlotOffset(header, index, hl.offset)

		if hl.length > uint32(need) {
			holes = append(holes, hole{
				offset: hl.offset + uint32(need),
				length: hl.length - uint32(need),
			})
		}

		entries, slotToEntry, idIndex = appendEntry(entries, slotToEntry, idIndex, index, h, r.spec, r.compactionThreshold)

		r.snap.Store(&snapshot{
			data:        data,
			header:      header,
			entries:     entries,
			slotToEntry: slotToEntry,
			holes:       holes,
			idIndex:     idIndex,
			tombstones:  old.tombstones,
			compactions: old.compactions,
			defragments: old.defragments,
		})
		return h, nil
	}

	newStart := uint32(len(old.data))
	if uint64(newStart)+uint64(need) > maxDataOffset {
		freeSlot(header, index)
		return 0, ErrDataExceedsCapacity
	}

	h, err := r.spec.Handle(index, need)
	if err != nil {
		freeSlot(header, index)
		return 0, err
	}
	if r.OnSetID != nil {
		h = r.OnSetID(h, r.spec.BitSpace())
	}

	dataToWrite := b
	if r.onWrite != nil {
		h, dataToWrite = r.onWrite(h, uint(newStart), b)
		if len(dataToWrite) != need {
			freeSlot(header, index)
			return 0, errors.New("onWrite returned different length")
		}
	}

	data := append(old.data, dataToWrite...)
	setSlotOffset(header, index, newStart)
	entries, slotToEntry, idIndex = appendEntry(entries, slotToEntry, idIndex, index, h, r.spec, r.compactionThreshold)

	r.snap.Store(&snapshot{
		data:        data,
		header:      header,
		entries:     entries,
		slotToEntry: slotToEntry,
		holes:       holes,
		idIndex:     idIndex,
		tombstones:  old.tombstones,
		compactions: old.compactions,
		defragments: old.defragments,
	})
	return h, nil
}

func (r *Record) Get(h Handle) []byte {
	snap := r.snap.Load()

	index := int(h.Offset())
	length := uint32(r.spec.Length(h))

	byteOffset, ok := slotOffset(snap.header, index)
	if !ok {
		return nil
	}
	if uint64(byteOffset)+uint64(length) > uint64(len(snap.data)) {
		return nil
	}
	return snap.data[byteOffset : byteOffset+length]
}

func (r *Record) Delete(h Handle) bool {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	old := r.snap.Load()

	index := int(h.Offset())
	pos := int(old.slotToEntry.get(index))
	if pos < 0 {
		return false
	}

	entries := old.entries
	slotToEntry := old.slotToEntry
	header := append([]byte(nil), old.header...)
	holes := append([]hole(nil), old.holes...)
	idIndex := old.idIndex

	e := entries.get(pos)
	lastPos := entries.length - 1
	if pos != lastPos {
		moved := entries.get(lastPos)
		entries = entries.set(pos, moved)
		slotToEntry = slotToEntry.set(int(moved.Offset()), int32(pos))
		idIndex = idIndex.insertOrUpdate(r.spec.ID(moved), int32(pos), r.compactionThreshold)
	}
	entries = entries.pop()
	slotToEntry = slotToEntry.set(index, -1)

	var compacted bool
	idIndex, compacted = idIndex.delete(r.spec.ID(e), r.compactionThreshold)

	byteOffset, _ := slotOffset(header, index)
	length := uint32(r.spec.Length(e))
	freeSlot(header, index)

	holes = append(holes, hole{offset: byteOffset, length: length})

	newSnap := &snapshot{
		data:        old.data,
		header:      header,
		entries:     entries,
		slotToEntry: slotToEntry,
		holes:       holes,
		idIndex:     idIndex,
		tombstones:  old.tombstones,
		compactions: old.compactions,
		defragments: old.defragments,
	}
	if compacted {
		newSnap.compactions++
	}
	r.snap.Store(newSnap)

	r.triggerDefrag()
	return true
}

func (r *Record) Handle(id uint64) (Handle, bool) {
	snap := r.snap.Load()
	pos, ok := snap.idIndex.lookup(id)
	if !ok {
		return 0, false
	}
	return snap.entries.get(int(pos)), true
}

func (r *Record) UpdateHandle(handle Handle) (Handle, bool) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	old := r.snap.Load()

	id := r.spec.ID(handle)
	pos, ok := old.idIndex.lookup(id)
	if !ok {
		return 0, false
	}

	oldHandle := old.entries.get(int(pos))
	if handle.Offset() != oldHandle.Offset() {
		return 0, false
	}

	entries := old.entries.set(int(pos), handle)

	r.snap.Store(&snapshot{
		data:        old.data,
		header:      old.header,
		entries:     entries,
		slotToEntry: old.slotToEntry,
		holes:       old.holes,
		idIndex:     old.idIndex,
		tombstones:  old.tombstones,
		compactions: old.compactions,
		defragments: old.defragments,
	})
	return oldHandle, true
}

func (r *Record) Clear() {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	r.snap.Store(&snapshot{
		header:      newHeader(r.spec.ConfiguredLengthBits()),
		entries:     &persistentEntries{},
		slotToEntry: &persistentSlotToEntry{},
	})
	r.cancelPendingDefrag()
}

func (r *Record) Defragment() {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.defragmentLocked()
}

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
				r.writeMu.Lock()
				r.defragmentLocked()
				r.writeMu.Unlock()
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
				r.writeMu.Lock()
				r.defragmentLocked()
				r.writeMu.Unlock()
				timer = nil
			}
		}
	}
}

func (r *Record) triggerDefrag() {
	if time.Duration(r.defragDelay.Load()) <= 0 {
		return
	}
	r.defragOnce.Do(func() {
		r.defragReq = make(chan struct{}, 1)
		r.stopDefrag = make(chan struct{})
		r.defragDone = make(chan struct{})
		r.defragStarted.Store(true)
		go r.defragLoop()
	})
	select {
	case r.defragReq <- struct{}{}:
	default:
	}
}

func (r *Record) cancelPendingDefrag() {
	if !r.defragStarted.Load() {
		return
	}
	select {
	case <-r.defragReq:
	default:
	}
}

func (r *Record) defragmentLocked() {
	old := r.snap.Load()
	if len(old.holes) == 0 {
		return
	}

	if old.entries == nil || old.entries.length == 0 {
		r.snap.Store(&snapshot{
			header:      old.header,
			entries:     old.entries,
			slotToEntry: old.slotToEntry,
			idIndex:     old.idIndex,
			data:        old.data[:0],
			holes:       nil,
			tombstones:  old.tombstones,
			compactions: old.compactions,
			defragments: old.defragments + 1,
		})
		return
	}

	header := append([]byte(nil), old.header...)
	entries := old.entries
	idIndex := old.idIndex
	data := make([]byte, 0, len(old.data))

	prefixLen := headerPrefixLen(header)
	writePos := uint32(0)

	for pos := 0; pos < entries.length; pos++ {
		h := entries.get(pos)
		index := int(h.Offset())
		length := uint32(r.spec.Length(h))

		offset, gen, alive := readSlot(header, prefixLen, index)
		if !alive {
			panic("bits: live entry references a free header slot")
		}

		dataSlice := old.data[offset : offset+length]
		newHandle := h
		if writePos != offset {
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
			writeSlot(header, prefixLen, index, writePos, gen, true)

			if newHandle != h {
				entries = entries.set(pos, newHandle)
				if newID, oldID := r.spec.ID(newHandle), r.spec.ID(h); newID != oldID {
					idIndex, _ = idIndex.delete(oldID, r.compactionThreshold)
					idIndex = idIndex.insertOrUpdate(newID, int32(pos), r.compactionThreshold)
				}
			}
		}

		data = append(data, dataSlice...)
		writePos += length
	}

	r.snap.Store(&snapshot{
		data:        data,
		header:      header,
		entries:     entries,
		slotToEntry: old.slotToEntry,
		holes:       nil,
		idIndex:     idIndex,
		tombstones:  old.tombstones,
		compactions: old.compactions,
		defragments: old.defragments + 1,
	})
}
