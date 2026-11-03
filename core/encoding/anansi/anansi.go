// Package anansi implements the Anansi Binary Wire Format (spec version
// 1.0, see todo/anansi_encoding.md in the project root) for
// core/document.Document instances: a compact, schema-aware binary
// serialization built directly on the same *definition.CompiledSchema and
// *container.DataContainer primitives the JSON codec (core/encoding/json)
// uses, so that a value encoded here and decoded back is indistinguishable
// — via Document.Get, JSON serialization, etc. — from one that was only
// ever built through the ordinary Document API.
//
// This implementation covers Level 1 (Basic) conformance from the spec
// plus Level 2 minus Stream packets: Dense packets (section 3.1), Sparse
// packets (section 3.2), and Batch packets — row-oriented (sections
// 3.3.1/3.3.2) and columnar (section 3.3.3) — for all 16 DataTypes; varint
// integers; native null-state semantics; ZSTD body compression (section
// 4.1, WithCompression); BLAKE3-truncated-to-128-bit integrity hashing
// (section 4.3, WithIntegrity / WithIntegrityHash); and AES-256-GCM
// encryption (section 4.2, WithEncryption / WithDecryptionKey). Scalar
// bools are bit-packed in Dense blocks and columnar value arrays (section
// 2.5). Transforms
// compose per the spec: compress, then encrypt; the digest always covers
// plaintext and is verified after decrypt+decompress. String decoding is
// zero-copy by default — values view one bulk-copied, container-owned
// backing (WithCopyStrings opts out). Stream packets
// (section 3.4) remain unimplemented.
//
// Encoding is two-pass ("inverted reader", see inverted.go): a size scan
// computes the exact final body size, then the body is populated directly
// into one exactly-sized packet. Decoding is single-pass streaming.
package anansi

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// rootSlot is the schema slot index of a Document's top-level (root)
// schema, matching the convention used throughout core/document and
// core/encoding/json.
const rootSlot uint8 = 0

// @note #hol8mc todo : Implement struct serialization/deserialization
//
// Because schemas can be derived from tagged structs, I would think that
// it would be prudent to implement an anansi codec and register it with
// codecs. For example
//
// type AnansiCodec interface {
//     Marshal(v any) ([]byte, error) // v is a struct with anansi tags
//     Unmarshal(data []byte, v any) error // data is a an anansi frame
//     Name() string
// }
// this native loop is not easily discoverable:
// pool := document.NewPool(cs, nil)  // check NewPool signature
// doc := pool.FromStruct(msg)        // struct → container
// wire := doc.ToAnansi(0)            // container → anansi binary
//
// doc2 := pool.FromAnansi(wire)      // anansi binary → container (or pool.New + UnmarshalAnansi)
// doc2.BindTo(&msg)                  // container → struct

// SerializeAnansi encodes doc (bound to schema slot 0 of cs) as a complete
// Anansi packet, automatically selecting Dense or Sparse framing per the
// spec's encoder selection logic (6.1). fullVersion is the schema version
// to embed in the header (spec 2.2); callers that don't maintain a schema
// version registry can pass 0. Optional transforms (compression, integrity
// digest) are requested via EncodeOption.
func SerializeAnansi(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16, opts ...EncodeOption) ([]byte, error) {
	return encodeInverted(cs, doc, fullVersion, PacketDense, false, opts...)
}

// DecodeAnansi decodes a complete Anansi packet into a freshly allocated,
// unpooled *container.DataContainer bound to schema slot 0 of cs.
func DecodeAnansi(cs *definition.CompiledSchema, data []byte, opts ...DecodeOption) (*container.DataContainer, uint16, error) {
	doc := container.NewDataContainer()
	v, err := DecodeAnansiInto(cs, data, doc, nil, opts...)
	if err != nil {
		return nil, 0, err
	}
	return doc, v, nil
}

// DecodeAnansiInto decodes a complete Anansi packet into doc (schema slot 0
// of cs), using pool for any nested TypeArrayObject child containers if
// non-nil. It returns the schema's full version as read from the header.
// Encrypted packets require WithDecryptionKey; compression and integrity
// digests are handled automatically. Strings decode zero-copy by default
// into a container-owned backing buffer (one bulk copy, no per-string
// allocations; the caller's input is never retained) — pass
// WithCopyStrings to opt out.
func DecodeAnansiInto(cs *definition.CompiledSchema, data []byte, doc *container.DataContainer, pool *container.Pool, opts ...DecodeOption) (uint16, error) {
	r := newByteReader(data)
	h, err := readHeader(r)
	if err != nil {
		return 0, err
	}
	cfg := newDecodeConfig(opts)
	// Spec 4.1.4/6.4.1: decrypt if needed, decompress if needed, verify the
	// digest over plaintext, then parse the body.
	body, fresh, err := openFrame(r, h.Flags, cfg)
	if err != nil {
		return 0, err
	}

	if !cfg.copyStrings {
		if !fresh {
			// The caller's wire is not ours to retain: snapshot it into the
			// container's pooled backing so aliased strings stay valid.
			buf := doc.AcquireBacking(body.remaining())
			copy(buf, body.data[body.pos:])
			body = newByteReader(buf)
		}
		body.alias = true
		doc.OwnBacking(body.data)
	}

	res, err := resourcesFor(cs)
	if err != nil {
		return 0, err
	}

	switch h.Flags.PacketType() {
	case PacketDense:
		if err := decodeDenseBody(body, res.cs, h, doc, res, pool); err != nil {
			return 0, err
		}
	case PacketSparse:
		if err := decodeSparseBody(body, res.cs, h, doc, res, pool); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("anansi: unsupported top-level packet type %s (Batch/Stream have dedicated entry points)", h.Flags.PacketType())
	}
	return h.FullVersion(), nil
}

// EncodeDense forces Dense-packet encoding of doc, bypassing the density
// heuristic. Optional transforms via EncodeOption.
func EncodeDense(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16, opts ...EncodeOption) ([]byte, error) {
	return encodeInverted(cs, doc, fullVersion, PacketDense, true, opts...)
}

// EncodeSparse forces Sparse-packet encoding of doc, bypassing the density
// heuristic. Optional transforms via EncodeOption.
func EncodeSparse(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16, opts ...EncodeOption) ([]byte, error) {
	return encodeInverted(cs, doc, fullVersion, PacketSparse, true, opts...)
}

// @note #dcd001 issue : Dense decoder doesn't mirror the encoder's docView fast path
//
// inverted.go's encoder is SBE-class on the populate path: viewDoc captures
// the document in one Walk (one positions lookup per field), caches slot
// pointers per used DataType, and reads values via (*(*[]T)(v.slots[dt]))[idx]
// with zero map access on the hot path. scanPacketBodyAs + putPacketBody
// then walk res.byType[dt] — a pre-computed per-DataType field list — so the
// outer loop is a fixed 16-iter type walk and the inner switch is jump-table
// predictable.
//
// dense.go's decodeDenseBody was not built to the same standard. Per field it
// calls doc.SetBool / doc.SetNull / readFieldPayload, and SetXxx does, in order:
//   1. type check (key.Type() != TypeX) — redundant, decode already dispatched
//   2. positions[int64(key)] lookup
//   3. claimHole(TypeX) — backward scan of d.holes
//   4. write at [idx] (or AppendXxx if no hole)
//   5. positions[int64(key)] = idx
// So per field decode pays ~2 map ops + 1 hole scan + 1 append, where encode
// pays 1 map op (amortized into viewDoc) + 1 array read.
//
// This is the umbrella for #dcd002-#dcd006. The fix pattern already exists in
// inverted.go: decoder should use doc.Walk to capture slot pointers and
// per-field indices once, pre-size each used type slice from the state map
// count (mirroring how the encoder sizes from state map counts), and write
// directly through slot+idx — one positions write per field, no hole claim,
// no type check, no growth.
//
// Expected: decode-wide64 drops from ~2-4x the encoder's latency to ~1.5-2x,
// and pooled decode hits 0 allocs/op.
//
// @see #dcd002
// @see #dcd003
// @see #dcd004

// @note #dcd002 issue : decodeDenseStateMap allocates []fieldState; Columnar already shows the fix
//
// Confirmed by columnar.go inspection: decodeColumnarBlock does NOT allocate
// a []fieldState. It reads the packed field-major state column in place via
// stateAtBit(packed, bit), a single array-index + shift + mask. Dense should
// do the same for its state map — the encoder's putPacketBody writes it with
// the same bit-shift pattern, so the decoder can read it with the same.
//
// This is now a local comparison: two decoders in the same package, one reads
// the state bits in place (Columnar) and one materializes a slice (Dense).
// Following Columnar's lead here removes the allocation with no new design.
//
// Same applies to any future state-map-like decoder (e.g. Stream packets if
// they land): read the bits, don't materialize the slice.
//
// @see #dcd001

// @note #dcd003 issue : Decoder doesn't pre-size typed slices; AppendXxx grows from cap 8
//
// DataContainer.initSlice defaults to cap 8. For a fresh (unpooled) container
// decoding 64 values of the same type, AppendXxx grows 8 -> 16 -> 32 -> 64 ->
// 128: ~5 reallocations plus copies per type. This dominates the "fresh"
// variant of BenchmarkDecodeDense/wide64.
//
// The encoder sizes its output exactly by scanning the state map first
// (inverted.go:scanPacketBodyAs). The decoder has the same information: after
// reading the state map it knows exact counts per DataType (nInt, nFloat,
// nString, ...), so it can pre-size each used slot with slot(dt, nDt) before
// the value loop. One allocation per used type, zero reallocations.
//
// @see #dcd001

// @note #dcd004 issue : SetXxx does redundant type check + unnecessary hole scan on decode path
//
// data_container.go: every SetInt/SetFloat/SetString/... starts with
//     if key.Type() != TypeX { return ErrTypeMismatch }
// and then calls claimHole(TypeX), a backward scan of d.holes for a matching
// type. On the decode path both are wasted work:
//
//   - The type comes from wf.fd.DataType(); the decoder dispatched on it a few
//     instructions earlier. The check cannot fail on valid input, and on
//     invalid input the decode already produced garbage.
//
//   - doc was either freshly allocated (holes empty) or Clear()'d by the pool
//     (Clear resets holes to [:0]). claimHole scans, finds nothing, returns -1,
//     and AppendXxx runs. Decode pays for a scan that cannot succeed.
//
// The direct-write path from #dcd001 skips both: the write is typed by
// construction and a fresh decode never reuses freed slots. If the decoder
// can't adopt Walk for some reason, a fallback is a Container API variant
// "SetFromEmpty" that skips type check + hole claim for known-typed fills.
//
// @see #dcd001

// @note #dcd005 issue : Sparse and Batch inherit every Dense issue; confirmed across all decoders
//
// Inspected sparse.go, batch.go, columnar.go. Status per issue:
//
//   #dcd001 (Walk fast path)  — ALL FOUR decoders miss it. Dense, Sparse,
//                                Batch-Row (via dispatch), Columnar all use
//                                doc.SetXxx per field.
//   #dcd002 ([]fieldState)    — Dense only. Columnar reads bits in place.
//                                Sparse has no state map. Batch inherits
//                                whichever decoder it dispatches to.
//   #dcd003 (no pre-size)     — All four. Sparse too: many present fields
//                                still grows via AppendXxx.
//   #dcd004 (redundant checks)— All four. Every SetXxx pays type check +
//                                hole scan on a fresh/pooled container.
//   #dcd006 (readFieldPayload)— All four call it. Single lever, four wins.
//
// Fix pattern: one shared "fillFromReader" helper that walks the target
// container once, pre-sizes from field count (sparse) or state-map counts
// (dense / batch-row / batch-columnar), and writes through slot+idx.
// One helper, four decoders.
//
// @see #dcd001
// @see #dcd006

// @note #dcd006 todo : Verify readFieldPayload isn't re-dispatching per field
//
// decodeDenseBody iterates res.byType[dt] — the same pre-computed per-DataType
// grouping the encoder uses — which is correct. But then it calls
// readFieldPayload per field. If readFieldPayload switches on wf.fd.DataType()
// again, the grouping's benefit is thrown away: dispatch is paid once per field
// instead of once per type.
//
// Confirm by reading readFieldPayload's body. If it does switch, inline the
// per-type read into the outer type loop so the dispatch cost is paid once per
// type (mirroring inverted.go:putValue's structure, where the switch is on
// wf.fd.DataType() but the caller has already grouped by type).
//
// @see #dcd001

// @note #dcd007 issue : readBoolBits allocates []bool per bool block; caller iterates and discards
//
// dense.go:decodeDenseBody and columnar.go:decodeColumnarBlock both call
// readBoolBits, which returns a fresh []bool per bool block. The caller then
// iterates the slice and calls doc.SetBool on each element, then discards the
// slice. Two costs compound:
//
//   1. The []bool allocation itself (one per Dense bool block; one per bool
//      FIELD per Columnar block — Columnar pays it N times where Dense pays
//      it once).
//   2. The intermediate slice prevents direct writes into the target
//      container's bool slice via Walk.
//
// The encoder's mirror (putPacketBody's TypeBool branch and
// EncodeBatchColumnar's bool loop) writes bits directly into the output
// buffer with no intermediate. The decoder should do the same: read bits
// in place and set the target bool value per bit, no []bool in between.
//
// @see #dcd001
// @see #dcd002

// @note #dcd008 todo : Batch decode allocates the return []*DataContainer per call
//
// batch.go:DecodeBatchRows does docs := make([]*container.DataContainer,
// recordCount) unconditionally — even when the caller will consume and
// discard in the same scope. For a 1000-record batch that's 8KB of
// pointers per call. Smaller than the per-field issues but constant, and it
// sits on the same hot path.
//
// Options: (a) accept it — the slice IS the return value, and callers can't
// avoid needing the pointers; (b) pool the slice; (c) offer an iterator API
// (DecodeBatchEach(pool, fn)) that avoids the slice entirely for streaming
// consumers. (c) is the most useful for large batches — the caller often
// wants to process-and-release one record at a time anyway.
//
// @see #dcd005

// @note #dcd009 todo : Zero-copy string path copies the entire remaining body unconditionally
//
// anansi.go:DecodeAnansiInto and batch.go:DecodeBatchRows both do:
//
//     buf := doc.AcquireBacking(body.remaining())
//     copy(buf, body.data[body.pos:])
//
// This is correct — the caller's wire buffer must not be retained, so a
// snapshot is needed if any string aliases it. But the copy happens even
// when the schema has NO string fields, in which case there's nothing to
// alias and the copy is pure waste. For a batch of integer-only records the
// entire body is copied for no reason.
//
// Precomputed fix: res knows whether any field of the schema is string-typed
// (or array-of-string). If not, skip the snapshot. Cheap to check at
// resourcesFor time, stored on the slotCodec.
//
// @see #dcd005
