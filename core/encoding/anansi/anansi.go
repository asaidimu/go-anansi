// Package anansi implements the Anansi Binary Wire Format (spec version
// 1.0, see todo/anansi_encoding.md in the project root) for
// core/document.Document instances: a compact, schema-aware binary
// serialization built directly on the same *definition.CompiledSchema and
// *container.DataContainer primitives the JSON codec (core/encoding/json)
// uses, so that a value encoded here and decoded back is indistinguishable
// — via Document.Get, JSON serialization, etc. — from one that was only
// ever built through the ordinary Document API.
//
// This implementation covers Level 1 (Basic) conformance from the spec:
// Dense packets (section 3.1), Sparse packets (section 3.2), and Batch
// row-oriented packets (section 3.3.1/3.3.2) for all 16 DataTypes, varint
// integers, and native null-state semantics — uncompressed and
// unencrypted. Columnar batch, Stream packets, compression, and encryption
// (spec sections 3.3.3, 3.4, and 8) are not implemented; see the CHANGELOG
// note at the bottom of this file for the reasoning.
package anansi

import (
	"bytes"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// rootSlot is the schema slot index of a Document's top-level (root)
// schema, matching the convention used throughout core/document and
// core/encoding/json.
const rootSlot uint8 = 0

// SerializeAnansi encodes doc (bound to schema slot 0 of cs) as a complete
// Anansi packet, automatically selecting Dense or Sparse framing per the
// spec's encoder selection logic (6.1). fullVersion is the schema version
// to embed in the header (spec 2.2); callers that don't maintain a schema
// version registry can pass 0.
func SerializeAnansi(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16) ([]byte, error) {
	epoch, version, err := schemaVersionByte(fullVersion)
	if err != nil {
		return nil, err
	}
	h := header{Version: version}
	h.Flags = Flags(epoch) << flagEpochShift
	return encodePacket(cs, rootSlot, doc, h, nil)
}

// DecodeAnansi decodes a complete Anansi packet into a freshly allocated,
// unpooled *container.DataContainer bound to schema slot 0 of cs.
func DecodeAnansi(cs *definition.CompiledSchema, data []byte) (*container.DataContainer, uint16, error) {
	doc := container.NewDataContainer()
	v, err := DecodeAnansiInto(cs, data, doc, nil)
	if err != nil {
		return nil, 0, err
	}
	return doc, v, nil
}

// DecodeAnansiInto decodes a complete Anansi packet into doc (schema slot 0
// of cs), using pool for any nested TypeArrayObject child containers if
// non-nil. It returns the schema's full version as read from the header.
func DecodeAnansiInto(cs *definition.CompiledSchema, data []byte, doc *container.DataContainer, pool *container.Pool) (uint16, error) {
	r := newByteReader(data)
	h, err := readHeader(r)
	if err != nil {
		return 0, err
	}
	if h.Flags.Compressed() {
		return 0, fmt.Errorf("anansi: compressed packets are not supported by this codec")
	}
	if h.Flags.Encrypted() {
		return 0, fmt.Errorf("anansi: encrypted packets are not supported by this codec")
	}
	if h.Flags.HashPresent() {
		// Spec 2.1.1: 16 bytes immediately after byte 1. We don't verify
		// it (no hash algorithm is specified in the spec beyond "present"),
		// but we must still skip it to reach the payload.
		if _, err := r.readN(16); err != nil {
			return 0, fmt.Errorf("anansi: read packet hash: %w", err)
		}
	}

	fields, err := collectWireFields(cs, rootSlot, nil)
	if err != nil {
		return 0, err
	}

	switch h.Flags.PacketType() {
	case PacketDense:
		if err := decodeDenseBody(r, cs, h, doc, fields, pool); err != nil {
			return 0, err
		}
	case PacketSparse:
		if err := decodeSparseBody(r, cs, h, doc, fields, pool); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("anansi: unsupported top-level packet type %s (Batch/Stream have dedicated entry points)", h.Flags.PacketType())
	}
	return h.FullVersion(), nil
}

// encodePacket auto-selects Dense vs Sparse for one schema slot (root or a
// TypeArrayObject child) and produces a complete packet: header + body.
// version.Version/Epoch are reused verbatim for nested packets (spec 2.5
// TypeArrayObject: "the nested packet uses the same schema version as the
// parent unless the schema defines an override" — this codec never
// overrides).
func encodePacket(cs *definition.CompiledSchema, schemaIdx uint8, doc *container.DataContainer, version header, prefix definition.ResolvedPath) ([]byte, error) {
	fields, err := collectWireFields(cs, schemaIdx, prefix)
	if err != nil {
		return nil, err
	}

	pt := selectPacketType(doc, fields)
	h := version
	h.Flags = (h.Flags &^ flagPacketTypeMask) | Flags(pt)

	buf := bytes.NewBuffer(make([]byte, 0, 64+len(fields)*4))
	buf.WriteByte(byte(h.Flags))
	buf.WriteByte(h.Version)

	switch pt {
	case PacketDense:
		if err := encodeDenseBody(buf, cs, h, doc, fields); err != nil {
			return nil, err
		}
	case PacketSparse:
		if err := encodeSparseBody(buf, cs, h, doc, fields); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// decodePacketInto is encodePacket's decode counterpart, used both by the
// top-level DecodeAnansiInto (schemaIdx == rootSlot) and recursively for
// TypeArrayObject children (schemaIdx == that field's child slot).
func decodePacketInto(cs *definition.CompiledSchema, schemaIdx uint8, data []byte, doc *container.DataContainer, pool *container.Pool, prefix definition.ResolvedPath) error {
	r := newByteReader(data)
	h, err := readHeader(r)
	if err != nil {
		return err
	}
	if h.Flags.HashPresent() {
		if _, err := r.readN(16); err != nil {
			return fmt.Errorf("anansi: read nested packet hash: %w", err)
		}
	}
	fields, err := collectWireFields(cs, schemaIdx, prefix)
	if err != nil {
		return err
	}
	switch h.Flags.PacketType() {
	case PacketDense:
		return decodeDenseBody(r, cs, h, doc, fields, pool)
	case PacketSparse:
		return decodeSparseBody(r, cs, h, doc, fields, pool)
	default:
		return fmt.Errorf("anansi: unsupported nested packet type %s", h.Flags.PacketType())
	}
}

// selectPacketType implements the spec's encoder selection logic (6.1):
// dense when the schema is small (<=64 fields) or densely populated
// (>25% of fields set), sparse otherwise.
//
// The spec additionally forces Sparse whenever the schema is recursive.
// This codec omits that check deliberately: in this engine, a recursive
// schema reference is represented as a single terminal TypeUnknown field
// (see classifyField in core/schema/definition/link.go) rather than an
// unbounded structural cycle, so the field list collectWireFields produces
// is always finite and fixed-size regardless of recursion — Dense encoding
// remains well-defined. schemaContainsRecursiveField (wirefields.go) is
// available for callers that want to inspect this, but it does not gate
// packet selection here.
func selectPacketType(doc *container.DataContainer, fields []wireField) PacketType {
	fieldCount := len(fields)
	if fieldCount == 0 {
		return PacketDense
	}
	presentCount := 0
	for _, wf := range fields {
		if fieldStateOf(doc, wf.key) != stateNotSet {
			presentCount++
		}
	}
	density := float64(presentCount) / float64(fieldCount)
	if fieldCount <= 64 || density > 0.25 {
		return PacketDense
	}
	return PacketSparse
}

// EncodeDense forces Dense-packet encoding of doc, bypassing the density
// heuristic. Returns an error if any field's current state cannot be
// represented (it never can't be — Dense supports every state — this
// exists purely so callers can force the format explicitly).
func EncodeDense(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16) ([]byte, error) {
	return encodeForced(cs, doc, fullVersion, PacketDense)
}

// EncodeSparse forces Sparse-packet encoding of doc, bypassing the density
// heuristic.
func EncodeSparse(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16) ([]byte, error) {
	return encodeForced(cs, doc, fullVersion, PacketSparse)
}

func encodeForced(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16, pt PacketType) ([]byte, error) {
	epoch, version, err := schemaVersionByte(fullVersion)
	if err != nil {
		return nil, err
	}
	h := header{Version: version}
	h.Flags = Flags(epoch)<<flagEpochShift | Flags(pt)

	fields, err := collectWireFields(cs, rootSlot, nil)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(make([]byte, 0, 64+len(fields)*4))
	buf.WriteByte(byte(h.Flags))
	buf.WriteByte(h.Version)
	switch pt {
	case PacketDense:
		if err := encodeDenseBody(buf, cs, h, doc, fields); err != nil {
			return nil, err
		}
	case PacketSparse:
		if err := encodeSparseBody(buf, cs, h, doc, fields); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("anansi: encodeForced: unsupported packet type %s", pt)
	}
	return buf.Bytes(), nil
}
