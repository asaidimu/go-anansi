package anansi

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/cache"
	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// This file holds the codec's per-schema resources: the slotCodec resource
// tree built once per compiled schema and shared by every subsequent
// encode/decode call.
//
// A *definition.CompiledSchema is immutable after Link — descriptors,
// addresses and field counts never change — so everything derivable from it
// (the flattened wire-field list, the DataPoint lookup index, the per-DataType
// groupings, the child resource trees for TypeArrayObject mounts) is a pure
// function of the schema and can be cached without invalidation logic. Cache
// entries are therefore always safe to evict; a bounded LRU (the repo's
// general-purpose core/cache) simply pins at most MaxEntries schemas'
// resources at a time.
//
// Before this file existed the same walks were memoised in two process-wide
// unbounded sync.Maps keyed by {schema pointer, slot, path string} — they
// never evicted, pinned every schema (and every nested path-key string
// allocated per sub-packet) forever, and one of the two caches was dead code.
// The tree below replaces both.

// slotCodec is one schema slot's pre-digested wire view: the canonical
// wire-ordered field list plus the indices every hot path needs.
type slotCodec struct {
	// cs is the compiled schema this tree was built from. The tree pins the
	// schema for as long as it is cached (values are pure functions of it).
	cs *definition.CompiledSchema

	// fields is the canonical wire order for this slot (declaration order,
	// flattened objects inlined) — the codec's Dense state-map order and
	// Sparse field order.
	fields []wireField

	// byDP indexes fields by canonical DataPoint (null bit masked off) for
	// Sparse decode, which must match wire DataPoints to fields without
	// relying on encode order.
	byDP map[int32]*wireField

	// byType groups fields by container.DataType in wire order so Dense and
	// columnar value blocks iterate only the fields that actually belong to
	// a block instead of rescanning all fields once per DataType.
	byType [16][]wireField
}

func (sc *slotCodec) add(wf wireField) {
	wf.idx = len(sc.fields)
	sc.fields = append(sc.fields, wf)
	last := &sc.fields[len(sc.fields)-1]
	sc.byType[wf.fd.DataType()] = append(sc.byType[wf.fd.DataType()], *last)
	sc.byDP[int32(wf.key.DataPoint())&^1] = last
}

// buildSlotCodec walks schemaIdx (recursing through flattened object slots
// and prebuilding child trees for TypeArrayObject mounts) and returns the
// slot's resources. childSlotCodec prebuilt trees own a reference to cs,
// keeping the schema live for as long as the cached resources are.
func buildSlotCodec(cs *definition.CompiledSchema, schemaIdx uint8, prefix definition.ResolvedPath) (*slotCodec, error) {
	sc := &slotCodec{cs: cs, byDP: make(map[int32]*wireField, 16)}
	if err := sc.collectInto(cs, schemaIdx, prefix); err != nil {
		return nil, err
	}
	return sc, nil
}

func (sc *slotCodec) collectInto(cs *definition.CompiledSchema, schemaIdx uint8, prefix definition.ResolvedPath) error {
	if int(schemaIdx) >= len(cs.Schemas) {
		return fmt.Errorf("anansi: schema slot %d out of range", schemaIdx)
	}
	slot := cs.Schemas[schemaIdx]
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		name := cs.FieldsMeta[abs].Name
		step := definition.NewResolvedStep(schemaIdx, uint8(j))
		fieldPath := append(append(definition.ResolvedPath{}, prefix...), step)

		if fd.Terminal() {
			key, err := computeLeafKey(cs, fd, fieldPath)
			if err != nil {
				return err
			}
			sc.add(wireField{fd: fd, key: key, name: name})
			continue
		}

		if fd.ChildSchemaIdx() == definition.FdNoChild {
			// Non-terminal with no child schema: nothing to encode (should
			// not normally occur in a well-formed compiled schema).
			continue
		}

		if fd.DataType() == container.TypeArrayObject {
			child, err := buildSlotCodec(cs, fd.ChildSchemaIdx(), fieldPath)
			if err != nil {
				return err
			}
			sc.add(wireField{
				fd:        fd,
				key:       internalKey(fd),
				name:      name,
				childIdx:  fd.ChildSchemaIdx(),
				childPath: fieldPath,
				child:     child,
			})
			continue
		}

		// Flattened object/union/composite/recursive-container field: it
		// owns no storage itself; its descendants live at this same path
		// prefix, one schema slot deeper.
		if err := sc.collectInto(cs, fd.ChildSchemaIdx(), fieldPath); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Bounded cache + direct-mapped fast path
// ---------------------------------------------------------------------------

// resourceCache bounds retained per-schema resources (256 schemas). Values
// are pure functions of immutable schemas, so eviction — including the
// watermark evictor dropping cold entries — is always safe; the next lookup
// simply rebuilds.
var resourceCache = func() cache.RepositoryCache[*slotCodec] {
	cfg := cache.DefaultCacheConfig()
	cfg.MaxEntries = 256
	cfg.ShardCount = 4
	cfg.PositiveTTL = 0 // resource entries never expire by TTL
	cfg.NegativeTTL = 0
	cfg.JanitorInterval = -1 // no background janitor; pure-function values
	return cache.NewManagedCache[*slotCodec](cfg, nil)
}()

// fastSlots is a 16-slot direct-mapped front over resourceCache: one atomic
// load + pointer compare hits on the hot path, and a collision (two schemas
// hashing to the same slot) merely evicts the other slot's fast entry —
// correctness never depends on the fast path, only on resourceCache.
var fastSlots [16]atomic.Pointer[fastEntry]

type fastEntry struct {
	cs  *definition.CompiledSchema
	res *slotCodec
}

// resourcesFor returns cs's root slot resources, consulting the direct-mapped
// fast path, then the bounded cache, then building fresh.
func resourcesFor(cs *definition.CompiledSchema) (*slotCodec, error) {
	if cs == nil {
		return nil, fmt.Errorf("anansi: nil compiled schema")
	}
	ptr := reflect.ValueOf(cs).Pointer()
	slot := &fastSlots[ptr&15]
	if e := slot.Load(); e != nil && e.cs == cs {
		return e.res, nil
	}
	key := strconv.FormatUint(uint64(ptr), 16)
	if res, ok := resourceCache.Get(key); ok {
		slot.Store(&fastEntry{cs: cs, res: res})
		return res, nil
	}
	res, err := buildSlotCodec(cs, rootSlot, nil)
	if err != nil {
		return nil, err
	}
	resourceCache.Set(key, res)
	slot.Store(&fastEntry{cs: cs, res: res})
	return res, nil
}

// ---------------------------------------------------------------------------
// Per-document state capture
// ---------------------------------------------------------------------------

// positionsOf returns the container's positions map through a single Walk —
// the one sanctioned raw-access hook for encode/decode paths (the container
// is being materialized for encode, so walking its internals is safe by
// design). Every encoder uses this ONCE per document and shares the result
// across state-map building, packet-type selection and value writing, instead
// of issuing IsSet/IsNull (two map lookups) or re-Walking per DataType.
func positionsOf(doc *container.DataContainer) map[int64]int32 {
	var p map[int64]int32
	_, _ = doc.Walk(func(positions map[int64]int32, _ func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		p = positions
		return nil, nil
	})
	return p
}

// stateAt evaluates a field's tri-state straight from a captured positions
// map (spec 2.7) — no container calls at all.
func stateAt(positions map[int64]int32, key container.DataContainerKey) fieldState {
	idx, ok := positions[int64(key)]
	if !ok {
		return stateNotSet
	}
	if idx < 0 {
		return stateNull
	}
	return stateHasValue
}

// ---------------------------------------------------------------------------
// Encode scratch pool
// ---------------------------------------------------------------------------

// encodeScratch carries the reusable buffers one encode pass needs (dense
// state maps wider than the stack buffer, bool gather, bit packing). Buffers
// are pooled; each use must re-slice to zero length.
type encodeScratch struct {
	state  []byte
	bools  []bool
	packed []byte
}

var scratchPool = sync.Pool{
	New: func() any {
		return &encodeScratch{
			state:  make([]byte, 0, 64),
			bools:  make([]bool, 0, 16),
			packed: make([]byte, 0, 64),
		}
	},
}

func getScratch() *encodeScratch {
	return scratchPool.Get().(*encodeScratch)
}

func putScratch(s *encodeScratch) {
	s.state = s.state[:0]
	s.bools = s.bools[:0]
	s.packed = s.packed[:0]
	scratchPool.Put(s)
}
