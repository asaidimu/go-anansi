package ir

// binary.go implements MarshalSchema and UnmarshalSchema — a versioned,
// little-endian binary encoding of Schema that bypasses the compiler
// entirely on load.
//
// Format overview
// ───────────────
//
//   Header  (32 bytes, fixed)
//   Section 1  Core IR          — Descriptors, SchemaOffsets, Variants
//   Section 2  AddressSpace     — sparse encoding of non-zero array entries
//   Section 3  Meta             — per-schema metadata
//   Section 4  ResolvedIndexes  — storage-engine-ready index descriptors
//   Section 5  Constraints      — structural tree; predicate funcs re-bound at load
//   Section 6  Store            — enum value sets and field defaults
//
// AddressSpace sparsity
// ─────────────────────
// CompiledAddressSpace contains six arrays bounded by the IR hard limits
// (128 schemas × 127 fields). Dense encoding wastes ~83 KB per schema because
// real schemas use only a small fraction of slots. Sparse encoding emits only
// non-zero entries as (index, value) pairs. For a typical schema (10 schemas,
// 20 fields each) the address space section shrinks from 83 KB to ~1–2 KB.
//
// Each 2D array [128][127] uses a packed uint16 key: (schemaIdx<<8 | fieldIdx).
// Each 1D array [128] uses a uint8 key (schemaIdx).
//
// What is NOT serialized
// ──────────────────────
//   Schema.PathCache         — reconstructed lazily by DocumentKey()
//   ResolvedConstraint.Predicate     — function pointer; re-bound from the
//                                      PredicateMap supplied to UnmarshalSchema
//   document.Document holes slice    — Store is write-once after Pass 8;
//                                      no freed positions exist on a fresh load
//
// Byte order
// ──────────
// All scalar integers are little-endian. The format is NOT portable across
// endian boundaries. All supported deployment targets (x86, ARM64) are
// little-endian.
//
// Version safety
// ──────────────
// Bump binaryVersion whenever the wire layout changes. UnmarshalSchema returns
// ErrFormatVersion on mismatch so the registry can recompile from source JSON.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"unsafe"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

// ── Public errors ─────────────────────────────────────────────────────────────

// ErrFormatVersion is returned by UnmarshalSchema when the binary was produced
// by an incompatible version of the encoder. The caller should recompile from
// source and store a fresh binary.
var ErrFormatVersion = errors.New("binary: incompatible schema format version")

// ErrFormatCorrupt is returned when the checksum does not match or the binary
// is structurally malformed (truncated, bad tag byte, etc.).
var ErrFormatCorrupt = errors.New("binary: schema binary is corrupt or truncated")

// ── Wire format constants ─────────────────────────────────────────────────────

const (
	binaryMagic   = "ANAS"
	binaryVersion = uint16(1)

	// Header layout (32 bytes):
	//   [0:4]   magic    "ANAS"
	//   [4:6]   version  uint16 LE
	//   [6:8]   flags    uint16 LE  (reserved, must be 0)
	//   [8:12]  checksum uint32 LE  FNV-1a-32 over bytes [32:end]
	//   [12:32] reserved [20]byte   zeros
	headerSize = 32

	// tagged-any type tags used for map[string]any, []any, constraint parameters,
	// index condition values, and enum Values in SchemaMetadata.
	tagNil       = uint8(0x00)
	tagString    = uint8(0x01)
	tagInt64     = uint8(0x02)
	tagFloat64   = uint8(0x03)
	tagBool      = uint8(0x04)
	tagSliceAny  = uint8(0x05)
	tagMapStrAny = uint8(0x06)

	// constraint / condition node kind tags
	condKindLeaf        = uint8(0)
	condKindGroup       = uint8(1)
	constraintKindLeaf  = uint8(0)
	constraintKindGroup = uint8(1)
)

// ── MarshalSchema ─────────────────────────────────────────────────────────────

// MarshalSchema encodes cs into a versioned binary representation.
// The returned slice is self-contained and can be stored directly in the schema
// registry. UnmarshalSchema reconstructs an equivalent *Schema without
// running any compiler passes.
func MarshalSchema(cs *Schema) ([]byte, error) {
	if cs == nil {
		return nil, errors.New("binary: cannot marshal nil Schema")
	}
	if cs.AddressSpace == nil {
		return nil, errors.New("binary: Schema.AddressSpace is nil — schema was not fully compiled")
	}

	e := &encoder{}
	// Reserve header space; checksum is written after the body is complete.
	e.buf = make([]byte, headerSize, 16*1024)

	if err := marshalCoreIR(e, cs); err != nil {
		return nil, fmt.Errorf("binary: section core_ir: %w", err)
	}
	if err := marshalAddressSpace(e, cs.AddressSpace); err != nil {
		return nil, fmt.Errorf("binary: section address_space: %w", err)
	}
	if err := marshalMeta(e, cs); err != nil {
		return nil, fmt.Errorf("binary: section meta: %w", err)
	}
	if err := marshalResolvedIndexes(e, cs); err != nil {
		return nil, fmt.Errorf("binary: section resolved_indexes: %w", err)
	}
	if err := marshalResolvedConstraints(e, cs.ResolvedConstraints); err != nil {
		return nil, fmt.Errorf("binary: section resolved_constraints: %w", err)
	}
	if err := marshalStore(e, cs.Store); err != nil {
		return nil, fmt.Errorf("binary: section store: %w", err)
	}

	body := e.buf[headerSize:]
	h := fnv.New32a()
	h.Write(body)
	checksum := h.Sum32()

	copy(e.buf[0:4], binaryMagic)
	binary.LittleEndian.PutUint16(e.buf[4:6], binaryVersion)
	binary.LittleEndian.PutUint16(e.buf[6:8], 0) // flags — reserved
	binary.LittleEndian.PutUint32(e.buf[8:12], checksum)
	// bytes [12:32] remain zero (reserved)

	return e.buf, nil
}

// ── UnmarshalSchema ───────────────────────────────────────────────────────────

// UnmarshalSchema decodes a binary produced by MarshalSchema and re-binds
// predicate functions from predicates. predicates may be nil if the schema
// has no constraints.
//
// Returns ErrFormatVersion if the binary was produced by an incompatible
// version. Returns ErrFormatCorrupt on checksum failure or truncation.
func UnmarshalSchema(data []byte, predicates PredicateMap) (*Schema, error) {
	if len(data) < headerSize {
		return nil, ErrFormatCorrupt
	}
	if string(data[0:4]) != binaryMagic {
		return nil, ErrFormatCorrupt
	}

	ver := binary.LittleEndian.Uint16(data[4:6])
	if ver != binaryVersion {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrFormatVersion, ver, binaryVersion)
	}

	storedChecksum := binary.LittleEndian.Uint32(data[8:12])
	h := fnv.New32a()
	h.Write(data[headerSize:])
	if h.Sum32() != storedChecksum {
		return nil, ErrFormatCorrupt
	}

	if predicates == nil {
		predicates = PredicateMap{}
	}

	d := &decoder{buf: data, pos: headerSize}
	cs := &Schema{
		PathCache: NewPathRegistry(),
	}

	if err := unmarshalCoreIR(d, cs); err != nil {
		return nil, fmt.Errorf("binary: section core_ir: %w", err)
	}
	as, err := unmarshalAddressSpace(d)
	if err != nil {
		return nil, fmt.Errorf("binary: section address_space: %w", err)
	}
	cs.AddressSpace = as

	if err := unmarshalMeta(d, cs); err != nil {
		return nil, fmt.Errorf("binary: section meta: %w", err)
	}
	if err := unmarshalResolvedIndexes(d, cs); err != nil {
		return nil, fmt.Errorf("binary: section resolved_indexes: %w", err)
	}
	if err := unmarshalResolvedConstraints(d, cs, predicates); err != nil {
		return nil, fmt.Errorf("binary: section resolved_constraints: %w", err)
	}
	store, err := unmarshalStore(d)
	if err != nil {
		return nil, fmt.Errorf("binary: section store: %w", err)
	}
	cs.Store = store

	if d.pos != len(d.buf) {
		return nil, fmt.Errorf("%w: %d trailing bytes", ErrFormatCorrupt, len(d.buf)-d.pos)
	}

	return cs, nil
}

// ── Section 1: Core IR ────────────────────────────────────────────────────────

func marshalCoreIR(e *encoder, cs *Schema) error {
	e.uint32(uint32(len(cs.Descriptors)))
	for _, d := range cs.Descriptors {
		e.uint32(d)
	}

	e.uint32(uint32(len(cs.SchemaOffsets)))
	for _, o := range cs.SchemaOffsets {
		e.uint32(o)
	}

	e.uint32(uint32(len(cs.Variants)))
	for fd, indices := range cs.Variants {
		e.uint32(fd)
		e.uint8(uint8(len(indices)))
		for _, idx := range indices {
			e.uint8(idx)
		}
	}

	return nil
}

func unmarshalCoreIR(d *decoder, cs *Schema) error {
	n, err := d.uint32()
	if err != nil {
		return err
	}
	cs.Descriptors = make([]uint32, n)
	for i := range cs.Descriptors {
		if cs.Descriptors[i], err = d.uint32(); err != nil {
			return err
		}
	}

	n, err = d.uint32()
	if err != nil {
		return err
	}
	cs.SchemaOffsets = make([]uint32, n)
	for i := range cs.SchemaOffsets {
		if cs.SchemaOffsets[i], err = d.uint32(); err != nil {
			return err
		}
	}

	n, err = d.uint32()
	if err != nil {
		return err
	}
	if n > 0 {
		cs.Variants = make(map[uint32][]uint8, n)
		for range n {
			fd, err := d.uint32()
			if err != nil {
				return err
			}
			count, err := d.uint8()
			if err != nil {
				return err
			}
			indices := make([]uint8, count)
			for i := range indices {
				if indices[i], err = d.uint8(); err != nil {
					return err
				}
			}
			cs.Variants[fd] = indices
		}
	}

	return nil
}

// ── Section 2: AddressSpace (sparse) ─────────────────────────────────────────
//
// Each array is emitted as a uint32 count followed by non-zero (key, value)
// pairs only. This gives typical savings of ~98% over dense encoding.
//
//   FieldOrdinals [128][127]uint32:
//     count uint32; entries: packed_key uint16 (si<<8|fi), ordinal uint32
//
//   BackEdgeOrdinal [128][127]uint8:
//     count uint32; entries: packed_key uint16, value uint8
//
//   BlockBases, BlockSize, AcyclicSubtreeSize, EntryOrdinal [128]uint32:
//     count uint32; entries: schema_idx uint8, value uint32
//
//   FrontSize uint32  (always written)
//
//   FieldNames: count uint32; per-schema: schema_idx uint8, name_count uint32,
//     name_entries: name string16, field_idx uint8

func marshalAddressSpace(e *encoder, as *CompiledAddressSpace) error {
	// FieldOrdinals
	{
		count := uint32(0)
		for si := range as.FieldOrdinals {
			for fi := range as.FieldOrdinals[si] {
				if as.FieldOrdinals[si][fi] != 0 {
					count++
				}
			}
		}
		e.uint32(count)
		for si := range as.FieldOrdinals {
			for fi := range as.FieldOrdinals[si] {
				if v := as.FieldOrdinals[si][fi]; v != 0 {
					e.uint16(uint16(si)<<8 | uint16(fi))
					e.uint32(v)
				}
			}
		}
	}

	// BackEdgeOrdinal
	{
		count := uint32(0)
		for si := range as.BackEdgeOrdinal {
			for fi := range as.BackEdgeOrdinal[si] {
				if as.BackEdgeOrdinal[si][fi] != 0 {
					count++
				}
			}
		}
		e.uint32(count)
		for si := range as.BackEdgeOrdinal {
			for fi := range as.BackEdgeOrdinal[si] {
				if v := as.BackEdgeOrdinal[si][fi]; v != 0 {
					e.uint16(uint16(si)<<8 | uint16(fi))
					e.uint8(v)
				}
			}
		}
	}

	marshalSparseUint32Array128(e, &as.BlockBases)
	marshalSparseUint32Array128(e, &as.BlockSize)
	marshalSparseUint32Array128(e, &as.AcyclicSubtreeSize)
	marshalSparseUint32Array128(e, &as.EntryOrdinal)

	e.uint32(as.FrontSize)

	// FieldNames: only emit schemas with a non-nil map
	nameSchemaCount := uint32(0)
	for i := range as.FieldNames {
		if as.FieldNames[i] != nil {
			nameSchemaCount++
		}
	}
	e.uint32(nameSchemaCount)
	for i, nm := range as.FieldNames {
		if nm == nil {
			continue
		}
		e.uint8(uint8(i))
		e.uint32(uint32(len(nm)))
		for name, fi := range nm {
			e.str16(name)
			e.uint8(fi)
		}
	}

	return nil
}

// marshalSparseUint32Array128 writes non-zero entries of a [128]uint32 as
// count uint32 + (schema_idx uint8, value uint32) pairs.
func marshalSparseUint32Array128(e *encoder, arr *[128]uint32) {
	count := uint32(0)
	for _, v := range arr {
		if v != 0 {
			count++
		}
	}
	e.uint32(count)
	for i, v := range arr {
		if v != 0 {
			e.uint8(uint8(i))
			e.uint32(v)
		}
	}
}

func unmarshalAddressSpace(d *decoder) (*CompiledAddressSpace, error) {
	as := &CompiledAddressSpace{}

	// FieldOrdinals
	{
		count, err := d.uint32()
		if err != nil {
			return nil, err
		}
		for range count {
			pk, err := d.uint16()
			if err != nil {
				return nil, err
			}
			v, err := d.uint32()
			if err != nil {
				return nil, err
			}
			as.FieldOrdinals[pk>>8][pk&0x7F] = v
		}
	}

	// BackEdgeOrdinal
	{
		count, err := d.uint32()
		if err != nil {
			return nil, err
		}
		for range count {
			pk, err := d.uint16()
			if err != nil {
				return nil, err
			}
			v, err := d.uint8()
			if err != nil {
				return nil, err
			}
			as.BackEdgeOrdinal[pk>>8][pk&0x7F] = v
		}
	}

	if err := unmarshalSparseUint32Array128(d, &as.BlockBases); err != nil {
		return nil, err
	}
	if err := unmarshalSparseUint32Array128(d, &as.BlockSize); err != nil {
		return nil, err
	}
	if err := unmarshalSparseUint32Array128(d, &as.AcyclicSubtreeSize); err != nil {
		return nil, err
	}
	if err := unmarshalSparseUint32Array128(d, &as.EntryOrdinal); err != nil {
		return nil, err
	}

	frontSize, err := d.uint32()
	if err != nil {
		return nil, err
	}
	as.FrontSize = frontSize

	schemaCount, err := d.uint32()
	if err != nil {
		return nil, err
	}
	for range schemaCount {
		si, err := d.uint8()
		if err != nil {
			return nil, err
		}
		nameCount, err := d.uint32()
		if err != nil {
			return nil, err
		}
		nm := make(map[string]uint8, nameCount)
		for range nameCount {
			name, err := d.str16()
			if err != nil {
				return nil, err
			}
			fi, err := d.uint8()
			if err != nil {
				return nil, err
			}
			nm[name] = fi
		}
		as.FieldNames[si] = nm
	}

	return as, nil
}

// unmarshalSparseUint32Array128 reads (schema_idx, value) pairs back into arr.
func unmarshalSparseUint32Array128(d *decoder, arr *[128]uint32) error {
	count, err := d.uint32()
	if err != nil {
		return err
	}
	for range count {
		si, err := d.uint8()
		if err != nil {
			return err
		}
		v, err := d.uint32()
		if err != nil {
			return err
		}
		arr[si] = v
	}
	return nil
}

// ── Section 3: Meta ───────────────────────────────────────────────────────────

func marshalMeta(e *encoder, cs *Schema) error {
	// Count non-nil entries only — nil slots are skipped during iteration.
	nonNilCount := uint32(0)
	for _, m := range cs.Meta {
		if m != nil {
			nonNilCount++
		}
	}
	e.uint32(nonNilCount)

	for idx, m := range cs.Meta {
		if m == nil {
			continue
		}
		e.uint8(idx)
		e.str16(m.UUID)
		e.str16(m.Name)
		e.str16(m.Version)
		e.str16(m.Description)
		e.uint8(boolByte(m.Concrete))
		e.uint8(uint8(m.Type))
		e.uint8(m.TargetSchema)

		e.uint8(uint8(len(m.Variants)))
		for _, v := range m.Variants {
			e.uint8(v)
		}

		e.uint32(uint32(len(m.Fields)))
		for fd, fm := range m.Fields {
			e.uint32(fd)
			e.str16(fm.UUID)
			e.str16(fm.Name)
			e.str16(fm.Description)
		}

		e.uint32(uint32(len(m.Indexes)))
		for uuid, idx := range m.Indexes {
			e.str16(uuid)
			ordinal, _ := m.IndexOrdinals[uuid]
			e.uint8(ordinal)
			if err := marshalIndexDescriptor(e, idx); err != nil {
				return err
			}
		}

		e.uint32(uint32(len(m.Values)))
		for _, v := range m.Values {
			if err := marshalAny(e, v); err != nil {
				return err
			}
		}

		e.uint32(uint32(len(m.Metadata)))
		for k, v := range m.Metadata {
			e.str16(k)
			if err := marshalAny(e, v); err != nil {
				return err
			}
		}
	}

	return nil
}

func unmarshalMeta(d *decoder, cs *Schema) error {
	count, err := d.uint32()
	if err != nil {
		return err
	}
	cs.Meta = make(map[uint8]*SchemaMetadata, count)

	for range count {
		idx, err := d.uint8()
		if err != nil {
			return err
		}
		m := &SchemaMetadata{}

		if m.UUID, err = d.str16(); err != nil {
			return err
		}
		if m.Name, err = d.str16(); err != nil {
			return err
		}
		if m.Version, err = d.str16(); err != nil {
			return err
		}
		if m.Description, err = d.str16(); err != nil {
			return err
		}
		concreteByte, err := d.uint8()
		if err != nil {
			return err
		}
		m.Concrete = concreteByte != 0

		typeByte, err := d.uint8()
		if err != nil {
			return err
		}
		m.Type = FieldTypeEnum(typeByte)

		if m.TargetSchema, err = d.uint8(); err != nil {
			return err
		}

		varCount, err := d.uint8()
		if err != nil {
			return err
		}
		if varCount > 0 {
			m.Variants = make([]uint8, varCount)
			for i := range m.Variants {
				if m.Variants[i], err = d.uint8(); err != nil {
					return err
				}
			}
		}

		fieldCount, err := d.uint32()
		if err != nil {
			return err
		}
		m.Fields = make(map[uint32]FieldMeta, fieldCount)
		for range fieldCount {
			fd, err := d.uint32()
			if err != nil {
				return err
			}
			var fm FieldMeta
			if fm.UUID, err = d.str16(); err != nil {
				return err
			}
			if fm.Name, err = d.str16(); err != nil {
				return err
			}
			if fm.Description, err = d.str16(); err != nil {
				return err
			}
			m.Fields[fd] = fm
		}

		indexCount, err := d.uint32()
		if err != nil {
			return err
		}
		m.Indexes = make(map[string]IndexDescriptor, indexCount)
		m.IndexOrdinals = make(map[string]uint8, indexCount)
		for range indexCount {
			uuid, err := d.str16()
			if err != nil {
				return err
			}
			ordinal, err := d.uint8()
			if err != nil {
				return err
			}
			m.IndexOrdinals[uuid] = ordinal
			idx, err := unmarshalIndexDescriptor(d)
			if err != nil {
				return err
			}
			m.Indexes[uuid] = idx
		}

		valCount, err := d.uint32()
		if err != nil {
			return err
		}
		if valCount > 0 {
			m.Values = make([]any, valCount)
			for i := range m.Values {
				if m.Values[i], err = unmarshalAny(d); err != nil {
					return err
				}
			}
		}

		metaCount, err := d.uint32()
		if err != nil {
			return err
		}
		if metaCount > 0 {
			m.Metadata = make(map[string]any, metaCount)
			for range metaCount {
				key, err := d.str16()
				if err != nil {
					return err
				}
				val, err := unmarshalAny(d)
				if err != nil {
					return err
				}
				m.Metadata[key] = val
			}
		}

		cs.Meta[idx] = m
	}

	return nil
}

// ── Section 4: ResolvedIndexes ────────────────────────────────────────────────

func marshalResolvedIndexes(e *encoder, cs *Schema) error {
	e.uint32(uint32(len(cs.ResolvedIndexes)))
	for key, ri := range cs.ResolvedIndexes {
		e.uint16(key)
		e.uint8(uint8(ri.Type))
		e.uint8(uint8(ri.Order))
		e.uint8(boolByte(ri.Unique))
		e.uint32(uint32(len(ri.Fields)))
		for _, dk := range ri.Fields {
			e.int64(int64(dk))
		}
		if ri.Condition != nil {
			e.uint8(1)
			if err := marshalResolvedCondition(e, ri.Condition); err != nil {
				return err
			}
		} else {
			e.uint8(0)
		}
	}
	return nil
}

func unmarshalResolvedIndexes(d *decoder, cs *Schema) error {
	count, err := d.uint32()
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	cs.ResolvedIndexes = make(map[uint16]ResolvedIndex, count)
	for range count {
		key, err := d.uint16()
		if err != nil {
			return err
		}
		ri := ResolvedIndex{}

		it, err := d.uint8()
		if err != nil {
			return err
		}
		ri.Type = IndexType(it)

		io, err := d.uint8()
		if err != nil {
			return err
		}
		ri.Order = IndexOrder(io)

		ub, err := d.uint8()
		if err != nil {
			return err
		}
		ri.Unique = ub != 0

		fieldCount, err := d.uint32()
		if err != nil {
			return err
		}
		ri.Fields = make([]document.DocumentKey, fieldCount)
		for i := range ri.Fields {
			v, err := d.int64()
			if err != nil {
				return err
			}
			ri.Fields[i] = document.DocumentKey(v)
		}

		hasCond, err := d.uint8()
		if err != nil {
			return err
		}
		if hasCond != 0 {
			ri.Condition, err = unmarshalResolvedCondition(d)
			if err != nil {
				return err
			}
		}

		cs.ResolvedIndexes[key] = ri
	}
	return nil
}

// ── Section 5: Constraints ────────────────────────────────────────────────────

func marshalResolvedConstraints(e *encoder, rt *ResolvedConstraintTree) error {
	if rt == nil {
		e.uint8(0)
		return nil
	}
	e.uint8(1)

	e.uint32(uint32(len(rt.Roots)))
	for _, root := range rt.Roots {
		if err := marshalResolvedConstraintNode(e, root); err != nil {
			return err
		}
	}

	e.uint32(uint32(len(rt.Index)))
	for ordinal, node := range rt.Index {
		e.uint16(ordinal)
		if err := marshalResolvedConstraintNode(e, node); err != nil {
			return err
		}
	}

	return nil
}

func unmarshalResolvedConstraints(d *decoder, cs *Schema, predicates PredicateMap) error {
	present, err := d.uint8()
	if err != nil {
		return err
	}
	if present == 0 {
		return nil
	}

	rt := &ResolvedConstraintTree{}

	rootCount, err := d.uint32()
	if err != nil {
		return err
	}
	rt.Roots = make([]ResolvedConstraintNode, rootCount)
	for i := range rt.Roots {
		if rt.Roots[i], err = unmarshalResolvedConstraintNode(d, predicates); err != nil {
			return err
		}
	}

	indexCount, err := d.uint32()
	if err != nil {
		return err
	}
	rt.Index = make(map[uint16]ResolvedConstraintNode, indexCount)
	for range indexCount {
		ordinal, err := d.uint16()
		if err != nil {
			return err
		}
		node, err := unmarshalResolvedConstraintNode(d, predicates)
		if err != nil {
			return err
		}
		rt.Index[ordinal] = node
	}

	cs.ResolvedConstraints = rt
	return nil
}

// ── Section 6: Store ──────────────────────────────────────────────────────────
//
// document.Document exposes Walk() for zero-copy access to its internal
// positions map and typed slices. We use this on both sides:
//   - Marshal: Walk reads positions and slice contents directly.
//   - Unmarshal: Walk writes positions and slice contents in-place without
//     going through the public API's type-check overhead.
// The holes slice is not persisted — Store is write-once after Pass 8.

func marshalStore(e *encoder, store *document.Document) error {
	if store == nil {
		e.uint8(0)
		return nil
	}
	e.uint8(1)
	return marshalDocumentBody(e, store)
}

func unmarshalStore(d *decoder) (*document.Document, error) {
	present, err := d.uint8()
	if err != nil {
		return nil, err
	}
	if present == 0 {
		return nil, nil
	}
	store := document.NewDocument()
	if err := unmarshalDocumentBody(d, store); err != nil {
		return nil, err
	}
	return store, nil
}

// marshalDocumentBody / unmarshalDocumentBody handle positions + all 16 typed
// slots for any Document (root Store or nested Document in TypeRecord / TypeArrayObject).

func marshalDocumentBody(e *encoder, doc *document.Document) error {
	var marshalErr error
	doc.Walk(func(
		positions map[int64]int32,
		slot func(t document.DataType, initialSize ...int) unsafe.Pointer,
	) (any, error) {
		e.uint32(uint32(len(positions)))
		for k, idx := range positions {
			e.int64(k)
			e.int32(idx)
		}
		for typ := document.DataType(0); typ < 16; typ++ {
			if err := marshalTypedSlot(e, typ, slot(typ)); err != nil {
				marshalErr = err
				return nil, err
			}
		}
		return nil, nil
	})
	return marshalErr
}

func unmarshalDocumentBody(d *decoder, doc *document.Document) error {
	var unmarshalErr error
	doc.Walk(func(
		positions map[int64]int32,
		slot func(t document.DataType, initialSize ...int) unsafe.Pointer,
	) (any, error) {
		posCount, err := d.uint32()
		if err != nil {
			unmarshalErr = err
			return nil, err
		}
		for range posCount {
			k, err := d.int64()
			if err != nil {
				unmarshalErr = err
				return nil, err
			}
			idx, err := d.int32()
			if err != nil {
				unmarshalErr = err
				return nil, err
			}
			positions[k] = idx
		}
		for typ := document.DataType(0); typ < 16; typ++ {
			if err := unmarshalTypedSlot(d, typ, slot(typ)); err != nil {
				unmarshalErr = err
				return nil, err
			}
		}
		return nil, nil
	})
	return unmarshalErr
}

// ── Typed slot marshal / unmarshal ────────────────────────────────────────────
//
// Each slot: uint32 count + count × element.
// ptr is *[]T cast to unsafe.Pointer, matching Document.slot()'s contract.

func marshalTypedSlot(e *encoder, typ document.DataType, ptr unsafe.Pointer) error {
	if ptr == nil {
		e.uint32(0)
		return nil
	}
	switch typ {
	case document.TypeUnknown:
		s := *(*[]any)(ptr)
		e.uint32(uint32(len(s)))
		for _, v := range s {
			if err := marshalAny(e, v); err != nil {
				return err
			}
		}
	case document.TypeInt:
		s := *(*[]int64)(ptr)
		e.uint32(uint32(len(s)))
		for _, v := range s {
			e.int64(v)
		}
	case document.TypeFloat:
		s := *(*[]float64)(ptr)
		e.uint32(uint32(len(s)))
		for _, v := range s {
			e.float64(v)
		}
	case document.TypeString:
		s := *(*[]string)(ptr)
		e.uint32(uint32(len(s)))
		for _, v := range s {
			e.str16(v)
		}
	case document.TypeBool:
		s := *(*[]bool)(ptr)
		e.uint32(uint32(len(s)))
		for _, v := range s {
			e.uint8(boolByte(v))
		}
	case document.TypeBytes:
		s := *(*[][]byte)(ptr)
		e.uint32(uint32(len(s)))
		for _, v := range s {
			e.uint32(uint32(len(v)))
			e.buf = append(e.buf, v...)
		}
	case document.TypeGeometry:
		s := *(*[][][]float64)(ptr)
		e.uint32(uint32(len(s)))
		for _, rings := range s {
			e.uint32(uint32(len(rings)))
			for _, pt := range rings {
				e.uint32(uint32(len(pt)))
				for _, coord := range pt {
					e.float64(coord)
				}
			}
		}
	case document.TypeRecord:
		s := *(*[]map[string]*document.Document)(ptr)
		e.uint32(uint32(len(s)))
		for _, rec := range s {
			if err := marshalDocumentRecord(e, rec); err != nil {
				return err
			}
		}
	case document.TypeArrayUnknown:
		s := *(*[][]any)(ptr)
		e.uint32(uint32(len(s)))
		for _, arr := range s {
			e.uint32(uint32(len(arr)))
			for _, v := range arr {
				if err := marshalAny(e, v); err != nil {
					return err
				}
			}
		}
	case document.TypeArrayInt:
		s := *(*[][]int64)(ptr)
		e.uint32(uint32(len(s)))
		for _, arr := range s {
			e.uint32(uint32(len(arr)))
			for _, v := range arr {
				e.int64(v)
			}
		}
	case document.TypeArrayFloat:
		s := *(*[][]float64)(ptr)
		e.uint32(uint32(len(s)))
		for _, arr := range s {
			e.uint32(uint32(len(arr)))
			for _, v := range arr {
				e.float64(v)
			}
		}
	case document.TypeArrayString:
		s := *(*[][]string)(ptr)
		e.uint32(uint32(len(s)))
		for _, arr := range s {
			e.uint32(uint32(len(arr)))
			for _, v := range arr {
				e.str16(v)
			}
		}
	case document.TypeArrayBool:
		s := *(*[][]bool)(ptr)
		e.uint32(uint32(len(s)))
		for _, arr := range s {
			e.uint32(uint32(len(arr)))
			for _, v := range arr {
				e.uint8(boolByte(v))
			}
		}
	case document.TypeArrayBytes:
		s := *(*[][][]byte)(ptr)
		e.uint32(uint32(len(s)))
		for _, arr := range s {
			e.uint32(uint32(len(arr)))
			for _, v := range arr {
				e.uint32(uint32(len(v)))
				e.buf = append(e.buf, v...)
			}
		}
	case document.TypeArrayObject:
		s := *(*[][]*document.Document)(ptr)
		e.uint32(uint32(len(s)))
		for _, arr := range s {
			e.uint32(uint32(len(arr)))
			for _, doc := range arr {
				if err := marshalDocumentInline(e, doc); err != nil {
					return err
				}
			}
		}
	case document.TypeArrayGeometry:
		s := *(*[][][][]float64)(ptr)
		e.uint32(uint32(len(s)))
		for _, arr := range s {
			e.uint32(uint32(len(arr)))
			for _, rings := range arr {
				e.uint32(uint32(len(rings)))
				for _, pt := range rings {
					e.uint32(uint32(len(pt)))
					for _, coord := range pt {
						e.float64(coord)
					}
				}
			}
		}
	}
	return nil
}

func unmarshalTypedSlot(d *decoder, typ document.DataType, ptr unsafe.Pointer) error {
	count, err := d.uint32()
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	switch typ {
	case document.TypeUnknown:
		s := (*[]any)(ptr)
		*s = make([]any, count)
		for i := range *s {
			if (*s)[i], err = unmarshalAny(d); err != nil {
				return err
			}
		}
	case document.TypeInt:
		s := (*[]int64)(ptr)
		*s = make([]int64, count)
		for i := range *s {
			if (*s)[i], err = d.int64(); err != nil {
				return err
			}
		}
	case document.TypeFloat:
		s := (*[]float64)(ptr)
		*s = make([]float64, count)
		for i := range *s {
			if (*s)[i], err = d.float64(); err != nil {
				return err
			}
		}
	case document.TypeString:
		s := (*[]string)(ptr)
		*s = make([]string, count)
		for i := range *s {
			if (*s)[i], err = d.str16(); err != nil {
				return err
			}
		}
	case document.TypeBool:
		s := (*[]bool)(ptr)
		*s = make([]bool, count)
		for i := range *s {
			b, err := d.uint8()
			if err != nil {
				return err
			}
			(*s)[i] = b != 0
		}
	case document.TypeBytes:
		s := (*[][]byte)(ptr)
		*s = make([][]byte, count)
		for i := range *s {
			n, err := d.uint32()
			if err != nil {
				return err
			}
			if (*s)[i], err = d.bytes(int(n)); err != nil {
				return err
			}
		}
	case document.TypeGeometry:
		s := (*[][][]float64)(ptr)
		*s = make([][][]float64, count)
		for i := range *s {
			ringCount, err := d.uint32()
			if err != nil {
				return err
			}
			(*s)[i] = make([][]float64, ringCount)
			for j := range (*s)[i] {
				ptCount, err := d.uint32()
				if err != nil {
					return err
				}
				(*s)[i][j] = make([]float64, ptCount)
				for k := range (*s)[i][j] {
					if (*s)[i][j][k], err = d.float64(); err != nil {
						return err
					}
				}
			}
		}
	case document.TypeRecord:
		s := (*[]map[string]*document.Document)(ptr)
		*s = make([]map[string]*document.Document, count)
		for i := range *s {
			if (*s)[i], err = unmarshalDocumentRecord(d); err != nil {
				return err
			}
		}
	case document.TypeArrayUnknown:
		s := (*[][]any)(ptr)
		*s = make([][]any, count)
		for i := range *s {
			n, err := d.uint32()
			if err != nil {
				return err
			}
			(*s)[i] = make([]any, n)
			for j := range (*s)[i] {
				if (*s)[i][j], err = unmarshalAny(d); err != nil {
					return err
				}
			}
		}
	case document.TypeArrayInt:
		s := (*[][]int64)(ptr)
		*s = make([][]int64, count)
		for i := range *s {
			n, err := d.uint32()
			if err != nil {
				return err
			}
			(*s)[i] = make([]int64, n)
			for j := range (*s)[i] {
				if (*s)[i][j], err = d.int64(); err != nil {
					return err
				}
			}
		}
	case document.TypeArrayFloat:
		s := (*[][]float64)(ptr)
		*s = make([][]float64, count)
		for i := range *s {
			n, err := d.uint32()
			if err != nil {
				return err
			}
			(*s)[i] = make([]float64, n)
			for j := range (*s)[i] {
				if (*s)[i][j], err = d.float64(); err != nil {
					return err
				}
			}
		}
	case document.TypeArrayString:
		s := (*[][]string)(ptr)
		*s = make([][]string, count)
		for i := range *s {
			n, err := d.uint32()
			if err != nil {
				return err
			}
			(*s)[i] = make([]string, n)
			for j := range (*s)[i] {
				if (*s)[i][j], err = d.str16(); err != nil {
					return err
				}
			}
		}
	case document.TypeArrayBool:
		s := (*[][]bool)(ptr)
		*s = make([][]bool, count)
		for i := range *s {
			n, err := d.uint32()
			if err != nil {
				return err
			}
			(*s)[i] = make([]bool, n)
			for j := range (*s)[i] {
				b, err := d.uint8()
				if err != nil {
					return err
				}
				(*s)[i][j] = b != 0
			}
		}
	case document.TypeArrayBytes:
		s := (*[][][]byte)(ptr)
		*s = make([][][]byte, count)
		for i := range *s {
			n, err := d.uint32()
			if err != nil {
				return err
			}
			(*s)[i] = make([][]byte, n)
			for j := range (*s)[i] {
				blen, err := d.uint32()
				if err != nil {
					return err
				}
				if (*s)[i][j], err = d.bytes(int(blen)); err != nil {
					return err
				}
			}
		}
	case document.TypeArrayObject:
		s := (*[][]*document.Document)(ptr)
		*s = make([][]*document.Document, count)
		for i := range *s {
			n, err := d.uint32()
			if err != nil {
				return err
			}
			(*s)[i] = make([]*document.Document, n)
			for j := range (*s)[i] {
				if (*s)[i][j], err = unmarshalDocumentInline(d); err != nil {
					return err
				}
			}
		}
	case document.TypeArrayGeometry:
		s := (*[][][][]float64)(ptr)
		*s = make([][][][]float64, count)
		for i := range *s {
			n, err := d.uint32()
			if err != nil {
				return err
			}
			(*s)[i] = make([][][]float64, n)
			for j := range (*s)[i] {
				ringCount, err := d.uint32()
				if err != nil {
					return err
				}
				(*s)[i][j] = make([][]float64, ringCount)
				for k := range (*s)[i][j] {
					ptCount, err := d.uint32()
					if err != nil {
						return err
					}
					(*s)[i][j][k] = make([]float64, ptCount)
					for l := range (*s)[i][j][k] {
						if (*s)[i][j][k][l], err = d.float64(); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

func marshalDocumentInline(e *encoder, doc *document.Document) error {
	if doc == nil {
		e.uint8(0)
		return nil
	}
	e.uint8(1)
	return marshalDocumentBody(e, doc)
}

func unmarshalDocumentInline(d *decoder) (*document.Document, error) {
	present, err := d.uint8()
	if err != nil {
		return nil, err
	}
	if present == 0 {
		return nil, nil
	}
	doc := document.NewDocument()
	if err := unmarshalDocumentBody(d, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func marshalDocumentRecord(e *encoder, rec map[string]*document.Document) error {
	e.uint32(uint32(len(rec)))
	for k, doc := range rec {
		e.str16(k)
		if err := marshalDocumentInline(e, doc); err != nil {
			return err
		}
	}
	return nil
}

func unmarshalDocumentRecord(d *decoder) (map[string]*document.Document, error) {
	n, err := d.uint32()
	if err != nil {
		return nil, err
	}
	rec := make(map[string]*document.Document, n)
	for range n {
		key, err := d.str16()
		if err != nil {
			return nil, err
		}
		doc, err := unmarshalDocumentInline(d)
		if err != nil {
			return nil, err
		}
		rec[key] = doc
	}
	return rec, nil
}

// ── IndexDescriptor ───────────────────────────────────────────────────────────

func marshalIndexDescriptor(e *encoder, idx IndexDescriptor) error {
	e.str16(idx.Name)
	e.str16(idx.Description)
	e.uint8(uint8(idx.Type))
	e.uint8(uint8(idx.Order))
	e.uint8(boolByte(idx.Unique))
	e.uint32(uint32(len(idx.Fields)))
	for _, f := range idx.Fields {
		e.str16(f)
	}
	if idx.Condition != nil {
		e.uint8(1)
		return marshalIndexCondition(e, idx.Condition)
	}
	e.uint8(0)
	return nil
}

func unmarshalIndexDescriptor(d *decoder) (IndexDescriptor, error) {
	idx := IndexDescriptor{}
	var err error
	if idx.Name, err = d.str16(); err != nil {
		return idx, err
	}
	if idx.Description, err = d.str16(); err != nil {
		return idx, err
	}
	it, err := d.uint8()
	if err != nil {
		return idx, err
	}
	idx.Type = IndexType(it)
	io, err := d.uint8()
	if err != nil {
		return idx, err
	}
	idx.Order = IndexOrder(io)
	ub, err := d.uint8()
	if err != nil {
		return idx, err
	}
	idx.Unique = ub != 0
	fieldCount, err := d.uint32()
	if err != nil {
		return idx, err
	}
	idx.Fields = make([]string, fieldCount)
	for i := range idx.Fields {
		if idx.Fields[i], err = d.str16(); err != nil {
			return idx, err
		}
	}
	hasCond, err := d.uint8()
	if err != nil {
		return idx, err
	}
	if hasCond != 0 {
		idx.Condition, err = unmarshalIndexCondition(d)
		if err != nil {
			return idx, err
		}
	}
	return idx, nil
}

// ── IndexCondition ────────────────────────────────────────────────────────────

func marshalIndexCondition(e *encoder, cond IndexCondition) error {
	switch c := cond.(type) {
	case IndexConditionLeaf:
		e.uint8(condKindLeaf)
		e.str16(c.Field)
		e.uint8(uint8(c.Operator))
		return marshalAny(e, c.Value)
	case IndexConditionGroup:
		e.uint8(condKindGroup)
		e.uint8(uint8(c.Operator))
		e.uint32(uint32(len(c.Conditions)))
		for _, child := range c.Conditions {
			if err := marshalIndexCondition(e, child); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("binary: unknown IndexCondition type")
	}
}

func unmarshalIndexCondition(d *decoder) (IndexCondition, error) {
	kind, err := d.uint8()
	if err != nil {
		return nil, err
	}
	switch kind {
	case condKindLeaf:
		leaf := IndexConditionLeaf{}
		if leaf.Field, err = d.str16(); err != nil {
			return nil, err
		}
		op, err := d.uint8()
		if err != nil {
			return nil, err
		}
		leaf.Operator = ComparisonOperator(op)
		leaf.Value, err = unmarshalAny(d)
		return leaf, err
	case condKindGroup:
		group := IndexConditionGroup{}
		op, err := d.uint8()
		if err != nil {
			return nil, err
		}
		group.Operator = LogicalOperator(op)
		count, err := d.uint32()
		if err != nil {
			return nil, err
		}
		group.Conditions = make([]IndexCondition, count)
		for i := range group.Conditions {
			if group.Conditions[i], err = unmarshalIndexCondition(d); err != nil {
				return nil, err
			}
		}
		return group, nil
	default:
		return nil, fmt.Errorf("binary: unknown index condition kind %d", kind)
	}
}

// ── ResolvedCondition ─────────────────────────────────────────────────────────

func marshalResolvedCondition(e *encoder, cond ResolvedCondition) error {
	switch c := cond.(type) {
	case ResolvedConditionLeaf:
		e.uint8(condKindLeaf)
		e.int64(int64(c.Field))
		e.uint8(uint8(c.Operator))
		return marshalAny(e, c.Value)
	case ResolvedConditionGroup:
		e.uint8(condKindGroup)
		e.uint8(uint8(c.Operator))
		e.uint32(uint32(len(c.Conditions)))
		for _, child := range c.Conditions {
			if err := marshalResolvedCondition(e, child); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("binary: unknown ResolvedCondition type")
	}
}

func unmarshalResolvedCondition(d *decoder) (ResolvedCondition, error) {
	kind, err := d.uint8()
	if err != nil {
		return nil, err
	}
	switch kind {
	case condKindLeaf:
		leaf := ResolvedConditionLeaf{}
		k, err := d.int64()
		if err != nil {
			return nil, err
		}
		leaf.Field = document.DocumentKey(k)
		op, err := d.uint8()
		if err != nil {
			return nil, err
		}
		leaf.Operator = ComparisonOperator(op)
		leaf.Value, err = unmarshalAny(d)
		return leaf, err
	case condKindGroup:
		group := ResolvedConditionGroup{}
		op, err := d.uint8()
		if err != nil {
			return nil, err
		}
		group.Operator = LogicalOperator(op)
		count, err := d.uint32()
		if err != nil {
			return nil, err
		}
		group.Conditions = make([]ResolvedCondition, count)
		for i := range group.Conditions {
			if group.Conditions[i], err = unmarshalResolvedCondition(d); err != nil {
				return nil, err
			}
		}
		return group, nil
	default:
		return nil, fmt.Errorf("binary: unknown resolved condition kind %d", kind)
	}
}

// ── ResolvedConstraintNode ────────────────────────────────────────────────────

func marshalResolvedConstraintNode(e *encoder, node ResolvedConstraintNode) error {
	switch n := node.(type) {
	case ResolvedConstraint:
		e.uint8(constraintKindLeaf)
		e.str16(n.UUID)
		e.str16(n.Name)
		e.str16(n.Description)
		e.str16(n.PredicateName)
		e.uint32(uint32(len(n.Fields)))
		for _, dk := range n.Fields {
			e.int64(int64(dk))
		}
		return marshalAny(e, n.Parameters)
	case ResolvedConstraintGroup:
		e.uint8(constraintKindGroup)
		e.str16(n.UUID)
		e.str16(n.Name)
		e.str16(n.Description)
		e.uint8(uint8(n.Operator))
		e.uint32(uint32(len(n.Constraints)))
		for _, child := range n.Constraints {
			if err := marshalResolvedConstraintNode(e, child); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("binary: unknown ResolvedConstraintNode type")
	}
}

func unmarshalResolvedConstraintNode(d *decoder, predicates PredicateMap) (ResolvedConstraintNode, error) {
	kind, err := d.uint8()
	if err != nil {
		return nil, err
	}
	switch kind {
	case constraintKindLeaf:
		rc := ResolvedConstraint{}
		if rc.UUID, err = d.str16(); err != nil {
			return nil, err
		}
		if rc.Name, err = d.str16(); err != nil {
			return nil, err
		}
		if rc.Description, err = d.str16(); err != nil {
			return nil, err
		}
		if rc.PredicateName, err = d.str16(); err != nil {
			return nil, err
		}
		// Re-bind predicate from the supplied map. Error on any missing name so
		// callers discover registration gaps eagerly rather than at validation time.
		if rc.PredicateName != "" {
			pred, ok := predicates[rc.PredicateName]
			if !ok {
				return nil, fmt.Errorf(
					"binary: unknown predicate %q — register it in the PredicateMap before loading",
					rc.PredicateName,
				)
			}
			rc.Predicate = pred
		}
		fieldCount, err := d.uint32()
		if err != nil {
			return nil, err
		}
		rc.Fields = make([]document.DocumentKey, fieldCount)
		for i := range rc.Fields {
			k, err := d.int64()
			if err != nil {
				return nil, err
			}
			rc.Fields[i] = document.DocumentKey(k)
		}
		rc.Parameters, err = unmarshalAny(d)
		return rc, err

	case constraintKindGroup:
		rg := ResolvedConstraintGroup{}
		if rg.UUID, err = d.str16(); err != nil {
			return nil, err
		}
		if rg.Name, err = d.str16(); err != nil {
			return nil, err
		}
		if rg.Description, err = d.str16(); err != nil {
			return nil, err
		}
		op, err := d.uint8()
		if err != nil {
			return nil, err
		}
		rg.Operator = LogicalOperator(op)
		count, err := d.uint32()
		if err != nil {
			return nil, err
		}
		rg.Constraints = make([]ResolvedConstraintNode, count)
		for i := range rg.Constraints {
			if rg.Constraints[i], err = unmarshalResolvedConstraintNode(d, predicates); err != nil {
				return nil, err
			}
		}
		return rg, nil

	default:
		return nil, fmt.Errorf("binary: unknown constraint node kind %d", kind)
	}
}

// ── tagged-any ────────────────────────────────────────────────────────────────
//
// After JSON unmarshalling, any values are always: nil, string, float64, bool,
// []any, map[string]any. store.go's toInt64 may produce int64 from float64.

func marshalAny(e *encoder, v any) error {
	if v == nil {
		e.uint8(tagNil)
		return nil
	}
	switch val := v.(type) {
	case string:
		e.uint8(tagString)
		e.str16(val)
	case int64:
		e.uint8(tagInt64)
		e.int64(val)
	case float64:
		e.uint8(tagFloat64)
		e.float64(val)
	case bool:
		e.uint8(tagBool)
		e.uint8(boolByte(val))
	case []any:
		e.uint8(tagSliceAny)
		e.uint32(uint32(len(val)))
		for _, elem := range val {
			if err := marshalAny(e, elem); err != nil {
				return err
			}
		}
	case map[string]any:
		e.uint8(tagMapStrAny)
		e.uint32(uint32(len(val)))
		for k, mv := range val {
			e.str16(k)
			if err := marshalAny(e, mv); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("binary: unsupported any type %T", v)
	}
	return nil
}

func unmarshalAny(d *decoder) (any, error) {
	tag, err := d.uint8()
	if err != nil {
		return nil, err
	}
	switch tag {
	case tagNil:
		return nil, nil
	case tagString:
		return d.str16()
	case tagInt64:
		return d.int64()
	case tagFloat64:
		return d.float64()
	case tagBool:
		b, err := d.uint8()
		return b != 0, err
	case tagSliceAny:
		n, err := d.uint32()
		if err != nil {
			return nil, err
		}
		s := make([]any, n)
		for i := range s {
			if s[i], err = unmarshalAny(d); err != nil {
				return nil, err
			}
		}
		return s, nil
	case tagMapStrAny:
		n, err := d.uint32()
		if err != nil {
			return nil, err
		}
		m := make(map[string]any, n)
		for range n {
			key, err := d.str16()
			if err != nil {
				return nil, err
			}
			val, err := unmarshalAny(d)
			if err != nil {
				return nil, err
			}
			m[key] = val
		}
		return m, nil
	default:
		return nil, fmt.Errorf("binary: unknown any tag 0x%02x", tag)
	}
}

// ── encoder ───────────────────────────────────────────────────────────────────

type encoder struct {
	buf []byte
}

func (e *encoder) uint8(v uint8) {
	e.buf = append(e.buf, v)
}

func (e *encoder) uint16(v uint16) {
	e.buf = append(e.buf, byte(v), byte(v>>8))
}

func (e *encoder) uint32(v uint32) {
	e.buf = append(e.buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (e *encoder) int32(v int32) {
	e.uint32(uint32(v))
}

func (e *encoder) int64(v int64) {
	u := uint64(v)
	e.buf = append(e.buf,
		byte(u), byte(u>>8), byte(u>>16), byte(u>>24),
		byte(u>>32), byte(u>>40), byte(u>>48), byte(u>>56),
	)
}

func (e *encoder) float64(v float64) {
	e.int64(int64(math.Float64bits(v)))
}

func (e *encoder) str16(s string) {
	if len(s) > 0xFFFF {
		s = s[:0xFFFF]
	}
	e.uint16(uint16(len(s)))
	e.buf = append(e.buf, s...)
}

// ── decoder ───────────────────────────────────────────────────────────────────

type decoder struct {
	buf []byte
	pos int
}

func (d *decoder) need(n int) error {
	if d.pos+n > len(d.buf) {
		return fmt.Errorf("%w: need %d bytes at offset %d, have %d remaining",
			ErrFormatCorrupt, n, d.pos, len(d.buf)-d.pos)
	}
	return nil
}

func (d *decoder) uint8() (uint8, error) {
	if err := d.need(1); err != nil {
		return 0, err
	}
	v := d.buf[d.pos]
	d.pos++
	return v, nil
}

func (d *decoder) uint16() (uint16, error) {
	if err := d.need(2); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint16(d.buf[d.pos:])
	d.pos += 2
	return v, nil
}

func (d *decoder) uint32() (uint32, error) {
	if err := d.need(4); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(d.buf[d.pos:])
	d.pos += 4
	return v, nil
}

func (d *decoder) int32() (int32, error) {
	v, err := d.uint32()
	return int32(v), err
}

func (d *decoder) int64() (int64, error) {
	if err := d.need(8); err != nil {
		return 0, err
	}
	v := int64(binary.LittleEndian.Uint64(d.buf[d.pos:]))
	d.pos += 8
	return v, nil
}

func (d *decoder) float64() (float64, error) {
	v, err := d.int64()
	return math.Float64frombits(uint64(v)), err
}

func (d *decoder) str16() (string, error) {
	n, err := d.uint16()
	if err != nil {
		return "", err
	}
	if err := d.need(int(n)); err != nil {
		return "", err
	}
	s := string(d.buf[d.pos : d.pos+int(n)])
	d.pos += int(n)
	return s, nil
}

func (d *decoder) bytes(n int) ([]byte, error) {
	if err := d.need(n); err != nil {
		return nil, err
	}
	b := make([]byte, n)
	copy(b, d.buf[d.pos:d.pos+n])
	d.pos += n
	return b, nil
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func boolByte(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
