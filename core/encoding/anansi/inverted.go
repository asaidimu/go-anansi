package anansi

import (
	"crypto/rand"
	"fmt"
	"math"
	"slices"
	"sync"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/klauspost/compress/zstd"
)

// This file is the codec's two-pass "inverted reader" encoder. Because the
// wire format is deterministic — every value's encoded width is a pure
// function of its value (varint widths after zigzag, length prefixes, IEEE
// bit patterns) — the writer can be modeled as a reader played backwards:
//
//      Pass 1 (scan):   walk the schema resources and the document's captured
//                       state once, computing the EXACT final body size and a
//                       flat pre-order plan of nested element packets.
//      Pass 2 (populate): allocate the output packet exactly once and fill it
//                       front-to-back with raw values straight from the
//                       document's typed backing slices.
//
// Compared to the previous buffer-accumulating encoder this removes every
// intermediate []byte: no bytes.Buffer growth, no per-value copies into a
// scratch, no per-element scratch round-trips, and one exact allocation per
// packet (plus one zstd frame for compressed transforms). Population reads
// the document through a docView captured in a single DataContainer.Walk —
// the container's sanctioned raw-access hook (encoding materializes a
// container state, so walking internals is safe by design) — and shares one
// rowState per field (tri-state + typed-slice index, ONE positions lookup)
// between scan and populate.

// ---------------------------------------------------------------------------
// binPut — front-advance writer over exactly pre-sized space
// ---------------------------------------------------------------------------

// binPut writes into buf[pos:] without ANY capacity checks: the scan pass
// guarantees the space. A scan undercount surfaces as a loud index panic
// rather than silent corruption or a hidden reallocation; encodeInverted
// additionally panics on an overcount (finished short of the reserved
// region), so size-twin bugs cannot pass unnoticed.
type binPut struct {
	buf []byte
	pos int
}

func (w *binPut) uvarint(v uint64) {
	for v >= 0x80 {
		w.buf[w.pos] = byte(v) | 0x80
		w.pos++
		v >>= 7
	}
	w.buf[w.pos] = byte(v)
	w.pos++
}

func (w *binPut) varint(v int64) { w.uvarint(zigzagEncode(v)) }

func (w *binPut) raw(b []byte) { copy(w.buf[w.pos:], b); w.pos += len(b) }

func (w *binPut) str(s string) { copy(w.buf[w.pos:], s); w.pos += len(s) }

func (w *binPut) float64LE(v float64) {
	x := math.Float64bits(v)
	b := w.buf[w.pos:]
	b[0] = byte(x)
	b[1] = byte(x >> 8)
	b[2] = byte(x >> 16)
	b[3] = byte(x >> 24)
	b[4] = byte(x >> 32)
	b[5] = byte(x >> 40)
	b[6] = byte(x >> 48)
	b[7] = byte(x >> 56)
	w.pos += 8
}

// zero clears n bytes at the cursor (state maps and bool bit-blocks are
// OR-ed bit fields; pooled scratch is dirty between uses).
func (w *binPut) zero(n int) {
	region := w.buf[w.pos : w.pos+n]
	clear(region)
}

// ---------------------------------------------------------------------------
// docView — one-Walk document capture shared by scan and populate
// ---------------------------------------------------------------------------

// rowState is one field's tri-state plus its typed-slice index, read from
// the positions map exactly once per field per packet.
type rowState struct {
	st  fieldState
	idx int32
}

// docView holds everything encoding needs to know about one document: the
// positions map, lazily-cached typed slice pointers (resolved eagerly inside
// the Walk for exactly the types that carry values — a used type's slot
// always exists already, so this allocates nothing) and the per-field row
// states. The Walk's slot accessor is deliberately NOT retained: storing the
// method value would force one heap allocation per document.
type docView struct {
	positions map[int64]int32
	slots     [16]unsafe.Pointer
	rows      []rowState // parallel to the slot's field list
}

// viewDoc captures doc for res's field list in a single Walk. Rows and the
// view struct itself are carved out of the shared plansBuf arena, so
// repeated captures (batch records, nested elements) allocate nothing after
// pool warm-up.
func viewDoc(doc *container.DataContainer, res *slotCodec, arena *plansBuf) *docView {
	return arena.viewAt(doc, res)
}

func (v *docView) slot(dt container.DataType) unsafe.Pointer {
	p := v.slots[dt]
	if p == nil {
		panic(fmt.Sprintf("anansi: docView slot for DataType %d accessed without a captured value", dt))
	}
	return p
}

func (v *docView) intAt(rs rowState) int64 { return (*(*[]int64)(v.slot(container.TypeInt)))[rs.idx] }
func (v *docView) floatAt(rs rowState) float64 {
	return (*(*[]float64)(v.slot(container.TypeFloat)))[rs.idx]
}
func (v *docView) stringAt(rs rowState) string {
	return (*(*[]string)(v.slot(container.TypeString)))[rs.idx]
}
func (v *docView) boolAt(rs rowState) bool { return (*(*[]bool)(v.slot(container.TypeBool)))[rs.idx] }
func (v *docView) bytesAt(rs rowState) []byte {
	return (*(*[][]byte)(v.slot(container.TypeBytes)))[rs.idx]
}
func (v *docView) geomAt(rs rowState) [][]float64 {
	return (*(*[][][]float64)(v.slot(container.TypeGeometry)))[rs.idx]
}
func (v *docView) recordAt(rs rowState) map[string]any {
	return (*(*[]map[string]any)(v.slot(container.TypeRecord)))[rs.idx]
}
func (v *docView) unknownAt(rs rowState) any {
	return (*(*[]any)(v.slot(container.TypeUnknown)))[rs.idx]
}
func (v *docView) arrIntAt(rs rowState) []int64 {
	return (*(*[][]int64)(v.slot(container.TypeArrayInt)))[rs.idx]
}
func (v *docView) arrFloatAt(rs rowState) []float64 {
	return (*(*[][]float64)(v.slot(container.TypeArrayFloat)))[rs.idx]
}
func (v *docView) arrStringAt(rs rowState) []string {
	return (*(*[][]string)(v.slot(container.TypeArrayString)))[rs.idx]
}
func (v *docView) arrBoolAt(rs rowState) []bool {
	return (*(*[][]bool)(v.slot(container.TypeArrayBool)))[rs.idx]
}
func (v *docView) arrBytesAt(rs rowState) [][]byte {
	return (*(*[][][]byte)(v.slot(container.TypeArrayBytes)))[rs.idx]
}
func (v *docView) arrGeomAt(rs rowState) [][][]float64 {
	return (*(*[][][][]float64)(v.slot(container.TypeArrayGeometry)))[rs.idx]
}
func (v *docView) arrUnknownAt(rs rowState) []any {
	return (*(*[][]any)(v.slot(container.TypeArrayUnknown)))[rs.idx]
}
func (v *docView) arrObjectAt(rs rowState) []*container.DataContainer {
	return (*(*[][]*container.DataContainer)(v.slot(container.TypeArrayObject)))[rs.idx]
}

// ---------------------------------------------------------------------------
// plansBuf — pooled plan arena
// ---------------------------------------------------------------------------

// elemPlan is one nested element packet discovered during the scan, in
// pre-order (an element's plan precedes its descendants'). Populate consumes
// plans strictly sequentially, matching the scan's discovery order.
type elemPlan struct {
	res  *slotCodec // element schema slot resources
	v    *docView   // element document capture (reused at populate)
	size int        // full payload: 2-byte header + body
	pt   PacketType // element packet type selected during scan
}

// plansBuf is the encode call's scratch arena: nested element plans, per-doc
// row states, live docViews and the record-key sorting scratch. Pooled;
// every field resets to zero length on put.
type plansBuf struct {
	elems []elemPlan
	rows  []rowState
	live  []*docView
	keys  []string
	next  int   // populate cursor into elems
	err   error // first scan error (unsupported any payloads)
}

func (a *plansBuf) rowsAt(n int) []rowState {
	if cap(a.rows)-len(a.rows) < n {
		grow := make([]rowState, len(a.rows), len(a.rows)+n)
		copy(grow, a.rows)
		a.rows = grow
	}
	r := a.rows[len(a.rows) : len(a.rows)+n]
	a.rows = a.rows[:len(a.rows)+n]
	return r
}

// docViewPool recycles docView structs. Views are handed out as stable
// pointers (a scan holds the root view across nested captures, so arena
// slices must never reallocate under them) and returned to the pool when
// the encode call finishes.
var docViewPool = sync.Pool{
	New: func() any { return new(docView) },
}

// viewAt acquires a docView (zero allocations in steady state — shared by
// top-level documents, batch records and nested elements) and captures doc
// against res in a single Walk.
func (a *plansBuf) viewAt(doc *container.DataContainer, res *slotCodec) *docView {
	v := docViewPool.Get().(*docView)
	a.live = append(a.live, v)
	*v = docView{rows: a.rowsAt(len(res.fields))}
	_, _ = doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		v.positions = positions
		var used [16]bool
		for i := range res.fields {
			idx, ok := positions[int64(res.fields[i].key)]
			switch {
			case !ok:
				v.rows[i] = rowState{st: stateNotSet}
			case idx < 0:
				v.rows[i] = rowState{st: stateNull}
			default:
				v.rows[i] = rowState{st: stateHasValue, idx: idx}
				used[res.fields[i].fd.DataType()] = true
			}
		}
		for dt := 0; dt < 16; dt++ {
			if used[dt] {
				v.slots[dt] = slot(container.DataType(dt))
			}
		}
		return nil, nil
	})
	return v
}

func (a *plansBuf) fail(err error) {
	if a.err == nil {
		a.err = err
	}
}

var plansPool = sync.Pool{
	New: func() any { return &plansBuf{} },
}

func putPlans(a *plansBuf) {
	for _, v := range a.live {
		*v = docView{}
		docViewPool.Put(v)
	}
	a.live = a.live[:0]
	a.elems = a.elems[:0]
	a.rows = a.rows[:0]
	a.keys = a.keys[:0]
	a.next = 0
	a.err = nil
	plansPool.Put(a)
}

// ---------------------------------------------------------------------------
// Size twins (pass 1)
// ---------------------------------------------------------------------------

func sizeString(s string) int { return uvarintLen(uint64(len(s))) + len(s) }

func sizeGeometry(rings [][]float64) int {
	n := uvarintLen(uint64(len(rings)))
	for _, ring := range rings {
		pts := len(ring) / 2
		n += uvarintLen(uint64(pts)) + pts*16
	}
	return n
}

// sizeRecord mirrors writeRecordBody's sorted-key tagged encoding.
func sizeRecord(m map[string]any, arena *plansBuf) int {
	keys := arena.keys[:0]
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	arena.keys = keys
	n := uvarintLen(uint64(len(keys)))
	for _, k := range keys {
		n += sizeString(k) + sizeAny(m[k], arena)
	}
	return n
}

// sizeAny mirrors writeAny's tagged encoding (anyTag byte + payload).
func sizeAny(v any, arena *plansBuf) int {
	switch t := v.(type) {
	case nil:
		return 1
	case bool:
		return 2
	case string:
		return 1 + sizeString(t)
	case int:
		return 1 + uvarintLen(zigzagEncode(int64(t)))
	case int64:
		return 1 + uvarintLen(zigzagEncode(t))
	case float64:
		return 1 + 8
	case []byte:
		return 1 + uvarintLen(uint64(len(t))) + len(t)
	case [][]float64:
		return 1 + sizeGeometry(t)
	case map[string]any:
		return 1 + sizeRecord(t, arena)
	case []any:
		n := 1 + uvarintLen(uint64(len(t)))
		for _, e := range t {
			n += sizeAny(e, arena)
		}
		return n
	case []int64:
		n := 1 + uvarintLen(uint64(len(t)))
		for _, x := range t {
			n += uvarintLen(zigzagEncode(x))
		}
		return n
	case []float64:
		return 1 + uvarintLen(uint64(len(t))) + 8*len(t)
	case []string:
		n := 1 + uvarintLen(uint64(len(t)))
		for _, s := range t {
			n += sizeString(s)
		}
		return n
	case []bool:
		return 1 + uvarintLen(uint64(len(t))) + (len(t)+7)/8
	default:
		arena.fail(fmt.Errorf("anansi: unsupported value type %T for TypeUnknown/TypeRecord slot", v))
		return 0
	}
}

// scanPacketBodyAs returns the exact encoded body size for v (captured
// against res) assuming the given packet type, appending any nested element
// plans to the arena in pre-order. Top-level callers choose the type first
// (auto-selected or forced); nested element packets always auto-select
// their own type (spec 2.5 TypeContainer).
func scanPacketBodyAs(v *docView, res *slotCodec, arena *plansBuf, pt PacketType) int {
	size := 0

	if pt == PacketDense {
		size += (2*len(res.fields) + 7) / 8 // state map
		for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
			if dt == container.TypeBool {
				n := 0
				for _, wf := range res.byType[dt] {
					if v.rows[wf.idx].st == stateHasValue {
						n++
					}
				}
				size += (n + 7) / 8
				continue
			}
			for _, wf := range res.byType[dt] {
				if rs := v.rows[wf.idx]; rs.st == stateHasValue {
					size += scanValue(v, &wf, rs, arena)
				}
			}
		}
		return size
	}

	// Sparse: field count up front, then per set field [DP varint][value?].
	set := 0
	for i := range v.rows {
		if v.rows[i].st != stateNotSet {
			set++
		}
	}
	size += uvarintLen(uint64(set))
	for i := range res.fields {
		wf := &res.fields[i]
		rs := v.rows[i]
		switch rs.st {
		case stateNotSet:
			continue
		case stateNull:
			size += uvarintLen(uint64(uint32(wf.key.DataPoint()) | 1))
		default:
			size += uvarintLen(uint64(uint32(wf.key.DataPoint()) &^ 1))
			size += scanValue(v, wf, rs, arena)
		}
	}
	return size
}

// scanValue returns the encoded size of one field's value, recursing into
// nested element packets (whose plans are appended to the arena).
func scanValue(v *docView, wf *wireField, rs rowState, arena *plansBuf) int {
	switch wf.fd.DataType() {
	case container.TypeInt:
		return uvarintLen(zigzagEncode(v.intAt(rs)))
	case container.TypeFloat:
		return 8
	case container.TypeString:
		return sizeString(v.stringAt(rs))
	case container.TypeBool:
		return 1 // sparse encoding: one byte
	case container.TypeBytes:
		b := v.bytesAt(rs)
		return uvarintLen(uint64(len(b))) + len(b)
	case container.TypeGeometry:
		return sizeGeometry(v.geomAt(rs))
	case container.TypeRecord:
		return sizeRecord(v.recordAt(rs), arena)
	case container.TypeUnknown:
		return sizeAny(v.unknownAt(rs), arena)
	case container.TypeArrayInt:
		arr := v.arrIntAt(rs)
		n := uvarintLen(uint64(len(arr)))
		for _, x := range arr {
			n += uvarintLen(zigzagEncode(x))
		}
		return n
	case container.TypeArrayFloat:
		arr := v.arrFloatAt(rs)
		return uvarintLen(uint64(len(arr))) + 8*len(arr)
	case container.TypeArrayString:
		arr := v.arrStringAt(rs)
		n := uvarintLen(uint64(len(arr)))
		for _, s := range arr {
			n += sizeString(s)
		}
		return n
	case container.TypeArrayBool:
		arr := v.arrBoolAt(rs)
		return uvarintLen(uint64(len(arr))) + (len(arr)+7)/8
	case container.TypeArrayBytes:
		arr := v.arrBytesAt(rs)
		n := uvarintLen(uint64(len(arr)))
		for _, b := range arr {
			n += uvarintLen(uint64(len(b))) + len(b)
		}
		return n
	case container.TypeArrayGeometry:
		arr := v.arrGeomAt(rs)
		n := uvarintLen(uint64(len(arr)))
		for _, g := range arr {
			n += sizeGeometry(g)
		}
		return n
	case container.TypeArrayUnknown:
		arr := v.arrUnknownAt(rs)
		n := uvarintLen(uint64(len(arr)))
		for _, e := range arr {
			n += sizeAny(e, arena)
		}
		return n
	case container.TypeArrayObject:
		children := v.arrObjectAt(rs)
		cres := wf.child
		n := uvarintLen(uint64(len(children)))
		for i := range children {
			cv := viewDoc(children[i], cres, arena)
			// Pre-order: this element's plan is appended BEFORE its body
			// scan, so descendants land after it and populate consumes
			// plans in the same order it writes packets.
			arena.elems = append(arena.elems, elemPlan{res: cres, v: cv})
			ei := len(arena.elems) - 1
			ept := selectPacketTypeRows(cv.rows, len(cres.fields))
			body := scanPacketBodyAs(cv, cres, arena, ept)
			arena.elems[ei].size = 2 + body
			arena.elems[ei].pt = ept
			n += uvarintLen(uint64(2+body)) + 2 + body
		}
		return n
	default:
		arena.fail(fmt.Errorf("anansi: unsupported data type %d for field %q", wf.fd.DataType(), wf.name))
		return 0
	}
}

// selectPacketTypeRows is selectPacketType over captured row states.
func selectPacketTypeRows(rows []rowState, fieldCount int) PacketType {
	if fieldCount == 0 {
		return PacketDense
	}
	present := 0
	for i := range rows {
		if rows[i].st != stateNotSet {
			present++
		}
	}
	density := float64(present) / float64(fieldCount)
	if fieldCount <= 64 || density > 0.25 {
		return PacketDense
	}
	return PacketSparse
}

// ---------------------------------------------------------------------------
// Populate (pass 2)
// ---------------------------------------------------------------------------

func putPacketBody(w *binPut, v *docView, res *slotCodec, pt PacketType, h header, arena *plansBuf) {
	if pt == PacketDense {
		// State map: 2 bits per field, OR-ed into a zeroed region.
		nBytes := (2*len(res.fields) + 7) / 8
		w.zero(nBytes)
		base := w.pos
		for i := range v.rows {
			var code byte
			switch v.rows[i].st {
			case stateNull:
				code = stateBitsNull
			case stateHasValue:
				code = stateBitsHasValue
			default:
				code = stateBitsNotSet
			}
			bitPos := i * 2
			w.buf[base+bitPos/8] |= code << uint(bitPos%8)
		}
		w.pos += nBytes

		for dt := container.DataType(0); dt <= container.TypeArrayGeometry; dt++ {
			if dt == container.TypeBool {
				// Bit-packed block (spec 2.5 TypeBool Dense Mode).
				n := 0
				for _, wf := range res.byType[dt] {
					if v.rows[wf.idx].st == stateHasValue {
						n++
					}
				}
				nB := (n + 7) / 8
				if nB > 0 {
					w.zero(nB)
					off := w.pos
					i := 0
					for _, wf := range res.byType[dt] {
						rs := v.rows[wf.idx]
						if rs.st != stateHasValue {
							continue
						}
						if v.boolAt(rs) {
							w.buf[off+i/8] |= 1 << uint(i%8)
						}
						i++
					}
					w.pos += nB
				}
				continue
			}
			for _, wf := range res.byType[dt] {
				if rs := v.rows[wf.idx]; rs.st == stateHasValue {
					putValue(w, v, &wf, rs, h, arena)
				}
			}
		}
		return
	}

	// Sparse: field count, then [DP][value?] per set field in wire order.
	set := 0
	for i := range v.rows {
		if v.rows[i].st != stateNotSet {
			set++
		}
	}
	w.uvarint(uint64(set))
	for i := range res.fields {
		wf := &res.fields[i]
		rs := v.rows[i]
		switch rs.st {
		case stateNotSet:
			continue
		case stateNull:
			w.uvarint(uint64(uint32(wf.key.DataPoint()) | 1))
		default:
			w.uvarint(uint64(uint32(wf.key.DataPoint()) &^ 1))
			putValue(w, v, wf, rs, h, arena)
		}
	}
}

// putValue writes one field's value bytes (spec 2.5, per DataType) straight
// from the captured typed slices.
func putValue(w *binPut, v *docView, wf *wireField, rs rowState, h header, arena *plansBuf) {
	switch wf.fd.DataType() {
	case container.TypeInt:
		w.varint(v.intAt(rs))
	case container.TypeFloat:
		w.float64LE(v.floatAt(rs))
	case container.TypeString:
		s := v.stringAt(rs)
		w.uvarint(uint64(len(s)))
		w.str(s)
	case container.TypeBool:
		if v.boolAt(rs) {
			w.buf[w.pos] = 1
		} else {
			w.buf[w.pos] = 0
		}
		w.pos++
	case container.TypeBytes:
		b := v.bytesAt(rs)
		w.uvarint(uint64(len(b)))
		w.raw(b)
	case container.TypeGeometry:
		putGeometry(w, v.geomAt(rs))
	case container.TypeRecord:
		putRecord(w, v.recordAt(rs), arena)
	case container.TypeUnknown:
		putAny(w, v.unknownAt(rs), arena)
	case container.TypeArrayInt:
		arr := v.arrIntAt(rs)
		w.uvarint(uint64(len(arr)))
		for _, x := range arr {
			w.varint(x)
		}
	case container.TypeArrayFloat:
		arr := v.arrFloatAt(rs)
		w.uvarint(uint64(len(arr)))
		for _, x := range arr {
			w.float64LE(x)
		}
	case container.TypeArrayString:
		arr := v.arrStringAt(rs)
		w.uvarint(uint64(len(arr)))
		for _, s := range arr {
			w.uvarint(uint64(len(s)))
			w.str(s)
		}
	case container.TypeArrayBool:
		arr := v.arrBoolAt(rs)
		w.uvarint(uint64(len(arr)))
		putBoolBits(w, arr)
	case container.TypeArrayBytes:
		arr := v.arrBytesAt(rs)
		w.uvarint(uint64(len(arr)))
		for _, b := range arr {
			w.uvarint(uint64(len(b)))
			w.raw(b)
		}
	case container.TypeArrayGeometry:
		arr := v.arrGeomAt(rs)
		w.uvarint(uint64(len(arr)))
		for _, g := range arr {
			putGeometry(w, g)
		}
	case container.TypeArrayUnknown:
		arr := v.arrUnknownAt(rs)
		w.uvarint(uint64(len(arr)))
		for _, e := range arr {
			putAny(w, e, arena)
		}
	case container.TypeArrayObject:
		children := v.arrObjectAt(rs)
		w.uvarint(uint64(len(children)))
		for range children {
			e := &arena.elems[arena.next]
			arena.next++
			w.uvarint(uint64(e.size))
			// Nested packet header: parent's epoch/version, re-selected
			// type bits, all transform bits stripped (spec 2.5 TypeContainer).
			eflags := (h.Flags &^ (flagPacketTypeMask | flagCompressed | flagEncrypted | flagHashPresent)) | Flags(e.pt)
			w.buf[w.pos] = byte(eflags)
			w.buf[w.pos+1] = h.Version
			w.pos += 2
			putPacketBody(w, e.v, e.res, e.pt, h, arena)
		}
	default:
		arena.fail(fmt.Errorf("anansi: unsupported data type %d for field %q", wf.fd.DataType(), wf.name))
	}
}

func putGeometry(w *binPut, rings [][]float64) {
	w.uvarint(uint64(len(rings)))
	for _, ring := range rings {
		npoints := len(ring) / 2
		w.uvarint(uint64(npoints))
		for i := 0; i+1 < len(ring); i += 2 {
			w.float64LE(ring[i])
			w.float64LE(ring[i+1])
		}
	}
}

func putRecord(w *binPut, m map[string]any, arena *plansBuf) {
	keys := arena.keys[:0]
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	arena.keys = keys
	w.uvarint(uint64(len(keys)))
	for _, k := range keys {
		w.uvarint(uint64(len(k)))
		w.str(k)
		putAny(w, m[k], arena)
	}
}

func putAny(w *binPut, v any, arena *plansBuf) {
	switch t := v.(type) {
	case nil:
		w.buf[w.pos] = tagNull
		w.pos++
	case bool:
		w.buf[w.pos] = tagBool
		w.pos++
		if t {
			w.buf[w.pos] = 1
		} else {
			w.buf[w.pos] = 0
		}
		w.pos++
	case string:
		w.buf[w.pos] = tagString
		w.pos++
		w.uvarint(uint64(len(t)))
		w.str(t)
	case int:
		w.buf[w.pos] = tagInt
		w.pos++
		w.varint(int64(t))
	case int64:
		w.buf[w.pos] = tagInt
		w.pos++
		w.varint(t)
	case float64:
		w.buf[w.pos] = tagFloat
		w.pos++
		w.float64LE(t)
	case []byte:
		w.buf[w.pos] = tagBytes
		w.pos++
		w.uvarint(uint64(len(t)))
		w.raw(t)
	case [][]float64:
		w.buf[w.pos] = tagGeom
		w.pos++
		putGeometry(w, t)
	case map[string]any:
		w.buf[w.pos] = tagRecord
		w.pos++
		putRecord(w, t, arena)
	case []any:
		w.buf[w.pos] = tagArrAny
		w.pos++
		w.uvarint(uint64(len(t)))
		for _, e := range t {
			putAny(w, e, arena)
		}
	case []int64:
		w.buf[w.pos] = tagArrInt
		w.pos++
		w.uvarint(uint64(len(t)))
		for _, x := range t {
			w.varint(x)
		}
	case []float64:
		w.buf[w.pos] = tagArrFlt
		w.pos++
		w.uvarint(uint64(len(t)))
		for _, x := range t {
			w.float64LE(x)
		}
	case []string:
		w.buf[w.pos] = tagArrStr
		w.pos++
		w.uvarint(uint64(len(t)))
		for _, s := range t {
			w.uvarint(uint64(len(s)))
			w.str(s)
		}
	case []bool:
		w.buf[w.pos] = tagArrBool
		w.pos++
		w.uvarint(uint64(len(t)))
		putBoolBits(w, t)
	default:
		// Unreachable when the scan succeeded (same type switch), but fail
		// loudly rather than misalign the stream.
		arena.fail(fmt.Errorf("anansi: unsupported value type %T for TypeUnknown/TypeRecord slot", v))
	}
}

// putBoolBits packs values LSB-first into ceil(len/8) bytes at the cursor.
func putBoolBits(w *binPut, values []bool) {
	nB := (len(values) + 7) / 8
	w.zero(nB)
	for i, b := range values {
		if b {
			w.buf[w.pos+i/8] |= 1 << uint(i%8)
		}
	}
	w.pos += nB
}

// ---------------------------------------------------------------------------
// Frame assembly
// ---------------------------------------------------------------------------

// zstdEncoder is the shared streaming encoder used for EncodeAll-into-packet
// compression (safe for concurrent use).
var zstdEncoder = func() *zstd.Encoder {
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		panic(fmt.Sprintf("anansi: init zstd encoder: %v", err))
	}
	return enc
}()

// plainBufPool holds the pooled plaintext scratch used by compressed
// transforms (body region + zstd frame region).
type plainBuf struct {
	b []byte
}

var plainBufPool = sync.Pool{
	New: func() any { return &plainBuf{} },
}

func getPlain(n int) *plainBuf {
	pb := plainBufPool.Get().(*plainBuf)
	if cap(pb.b) < n {
		pb.b = make([]byte, n)
	}
	return pb
}

func putPlain(pb *plainBuf) {
	if cap(pb.b) > 1<<20 {
		pb.b = nil // release oversized scratch
	}
	plainBufPool.Put(pb)
}

// encodeInverted is the shared core of SerializeAnansi/EncodeDense/
// EncodeSparse: one scan pass, one exact allocation, one populate pass.
// When forceType is false the packet type is auto-selected per spec 6.1.
func encodeInverted(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16, pt PacketType, forceType bool, opts ...EncodeOption) ([]byte, error) {
	epoch, version, err := schemaVersionByte(fullVersion)
	if err != nil {
		return nil, err
	}
	res, err := resourcesFor(cs)
	if err != nil {
		return nil, err
	}
	cfg := newEncodeConfig(opts)

	h := header{Version: version}
	h.Flags = Flags(epoch) << flagEpochShift

	arena := plansPool.Get().(*plansBuf)
	defer putPlans(arena)

	v := viewDoc(doc, res, arena)
	rootPT := selectPacketTypeRows(v.rows, len(res.fields))
	if forceType {
		rootPT = pt
	}
	h.Flags = h.Flags&^flagPacketTypeMask | Flags(rootPT)
	bodySize := scanPacketBodyAs(v, res, arena, rootPT)
	if arena.err != nil {
		return nil, arena.err
	}

	populate := func(buf []byte, bodyStart int) {
		w := binPut{buf: buf, pos: bodyStart}
		putPacketBody(&w, v, res, rootPT, h, arena)
		if w.pos != bodyStart+bodySize {
			panic(fmt.Sprintf("anansi: inverted encoder size mismatch: scanned %d body bytes, wrote %d", bodySize, w.pos-bodyStart))
		}
	}
	return finishInverted(h, bodySize, cfg, arena, populate)
}

// finishInverted assembles the final packet: exact allocation, populate
// writes the plaintext body directly into its final position (packet or
// pooled scratch), then the transform envelope is applied in spec order
// (digest over plaintext, compress, encrypt). The GCM seal reuses the
// plaintext's own storage via the documented plaintext[:0] dst pattern.
func finishInverted(h header, bodySize int, cfg encodeConfig, arena *plansBuf, populate func(buf []byte, bodyStart int)) ([]byte, error) {
	flags := h.Flags
	if cfg.integrity {
		flags |= flagHashPresent
	}
	if cfg.compressed {
		flags |= flagCompressed
	}
	if cfg.key != nil {
		flags |= flagEncrypted
	}

	prefix := 2
	if cfg.integrity {
		prefix += hashSize
	}

	switch {
	case !cfg.compressed && cfg.key == nil:
		// Plain: the body is written once, directly into the packet.
		total := prefix + bodySize
		packet := make([]byte, total)
		packet[0], packet[1] = byte(flags), h.Version
		populate(packet, prefix)
		if cfg.integrity {
			d := packetDigest(packet[prefix : prefix+bodySize])
			copy(packet[2:], d[:])
		}
		return packet, nil

	case !cfg.compressed && cfg.key != nil:
		// Encrypted only: [hdr][digest?][nonce][ciphertext+tag]; the
		// plaintext body is populated in place, then sealed into itself.
		bodyStart := prefix + gcmNonceSize
		total := bodyStart + bodySize + 16
		packet := make([]byte, total)
		packet[0], packet[1] = byte(flags), h.Version
		populate(packet, bodyStart)
		if cfg.integrity {
			d := packetDigest(packet[bodyStart : bodyStart+bodySize])
			copy(packet[2:], d[:])
		}
		if _, err := rand.Read(packet[prefix : prefix+gcmNonceSize]); err != nil {
			return nil, fmt.Errorf("anansi: generate nonce: %w", err)
		}
		aead, err := cachedAEAD(cfg.key)
		if err != nil {
			return nil, err
		}
		out := aead.Seal(packet[bodyStart:bodyStart], packet[prefix:prefix+gcmNonceSize], packet[bodyStart:bodyStart+bodySize], nil)
		return packet[:bodyStart+len(out)], nil

	case cfg.compressed && cfg.key == nil:
		// Compressed only: [hdr][digest?][plain_len varint][zstd frame].
		// The body is populated into pooled scratch, digested, then zstd
		// EncodeAll writes the frame straight into the packet's payload
		// area (reserved with zstd's documented worst-case bound).
		pb := getPlain(bodySize)
		defer putPlain(pb)
		body := pb.b[:bodySize]
		populate(body, 0)

		vl := uvarintLen(uint64(bodySize))
		total := prefix + vl + bodySize + bodySize/255 + 64
		packet := make([]byte, 0, total)
		packet = append(packet, byte(flags), h.Version)
		if cfg.integrity {
			d := packetDigest(body)
			packet = append(packet, d[:]...)
		}
		packet = putUvarint(packet, uint64(bodySize))
		packet = zstdEncoder.EncodeAll(body, packet)
		return packet, nil

	default:
		// Encrypted + compressed (spec 4.2.2): the AEAD plaintext is
		// [plain_len varint][zstd frame] — both inside the seal.
		vl := uvarintLen(uint64(bodySize))
		zbound := bodySize + bodySize/255 + 64
		pb := getPlain(bodySize + vl + zbound)
		defer putPlain(pb)
		body := pb.b[:bodySize]
		populate(body, 0)

		plainStart := bodySize
		plain := putUvarint(pb.b[plainStart:plainStart], uint64(bodySize))
		plain = zstdEncoder.EncodeAll(body, plain)

		bodyStart := prefix + gcmNonceSize
		total := bodyStart + len(plain) + 16
		packet := make([]byte, total)
		packet[0], packet[1] = byte(flags), h.Version
		if cfg.integrity {
			d := packetDigest(body)
			copy(packet[2:], d[:])
		}
		if _, err := rand.Read(packet[prefix : prefix+gcmNonceSize]); err != nil {
			return nil, fmt.Errorf("anansi: generate nonce: %w", err)
		}
		aead, err := cachedAEAD(cfg.key)
		if err != nil {
			return nil, err
		}
		out := aead.Seal(packet[bodyStart:bodyStart], packet[prefix:prefix+gcmNonceSize], plain, nil)
		return packet[:bodyStart+len(out)], nil
	}
}
