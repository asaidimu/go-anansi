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
// encryption (section 4.2, WithEncryption / WithDecryptionKey). Transforms
// compose per the spec: compress, then encrypt; the digest always covers
// plaintext and is verified after decrypt+decompress. Stream packets
// (section 3.4) remain unimplemented; see the CHANGELOG note at the bottom
// of this file for the reasoning.
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
// version registry can pass 0. Optional transforms (compression, integrity
// digest) are requested via EncodeOption.
func SerializeAnansi(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16, opts ...EncodeOption) ([]byte, error) {
	epoch, version, err := schemaVersionByte(fullVersion)
	if err != nil {
		return nil, err
	}
	h := header{Version: version}
	h.Flags = Flags(epoch) << flagEpochShift
	body, _, err := encodePacketBody(cs, rootSlot, doc, h, nil)
	if err != nil {
		return nil, err
	}
	return finishFrame(h, body, newEncodeConfig(opts))
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
// digests are handled automatically.
func DecodeAnansiInto(cs *definition.CompiledSchema, data []byte, doc *container.DataContainer, pool *container.Pool, opts ...DecodeOption) (uint16, error) {
	r := newByteReader(data)
	h, err := readHeader(r)
	if err != nil {
		return 0, err
	}
	// Spec 4.1.4/6.4.1: decrypt if needed, decompress if needed, verify the
	// digest over plaintext, then parse the body.
	r, err = openFrame(r, h.Flags, newDecodeConfig(opts))
	if err != nil {
		return 0, err
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

// encodePacketBody auto-selects Dense vs Sparse for one schema slot (root
// or a TypeArrayObject child) and returns the raw body bytes plus the
// packet-local header describing them. The returned header preserves the
// caller's epoch/version but re-selects the type bits for doc and strips
// all transform bits: nested sub-packets are part of their parent's
// payload — already covered by the parent's integrity digest, with any
// compression/encryption applying to the outer message only — so they must
// not claim transforms of their own (spec 2.5 TypeContainer: "the nested
// packet uses the same schema version as the parent").
func encodePacketBody(cs *definition.CompiledSchema, schemaIdx uint8, doc *container.DataContainer, version header, prefix definition.ResolvedPath) ([]byte, header, error) {
	fields, err := collectWireFields(cs, schemaIdx, prefix)
	if err != nil {
		return nil, header{}, err
	}

	pt := selectPacketType(doc, fields)
	h := header{Version: version.Version}
	h.Flags = (version.Flags &^ (flagPacketTypeMask | flagCompressed | flagEncrypted | flagHashPresent)) | Flags(pt)

	buf := bytes.NewBuffer(make([]byte, 0, 64+len(fields)*4))
	switch pt {
	case PacketDense:
		if err := encodeDenseBody(buf, cs, h, doc, fields); err != nil {
			return nil, header{}, err
		}
	case PacketSparse:
		if err := encodeSparseBody(buf, cs, h, doc, fields); err != nil {
			return nil, header{}, err
		}
	}
	return buf.Bytes(), h, nil
}

// encodePacket builds a complete raw nested sub-packet (header + body).
func encodePacket(cs *definition.CompiledSchema, schemaIdx uint8, doc *container.DataContainer, version header, prefix definition.ResolvedPath) ([]byte, error) {
	body, h, err := encodePacketBody(cs, schemaIdx, doc, version, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 2, len(body)+2)
	out[0], out[1] = byte(h.Flags), h.Version
	return append(out, body...), nil
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
	if h.Flags.Compressed() || h.Flags.Encrypted() {
		return fmt.Errorf("anansi: compressed/encrypted nested packets are not supported by this codec")
	}
	if err := readAndVerifyNestedHash(r, h.Flags); err != nil {
		return err
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
// heuristic. Optional transforms via EncodeOption.
func EncodeDense(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16, opts ...EncodeOption) ([]byte, error) {
	return encodeForced(cs, doc, fullVersion, PacketDense, opts...)
}

// EncodeSparse forces Sparse-packet encoding of doc, bypassing the density
// heuristic. Optional transforms via EncodeOption.
func EncodeSparse(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16, opts ...EncodeOption) ([]byte, error) {
	return encodeForced(cs, doc, fullVersion, PacketSparse, opts...)
}

func encodeForced(cs *definition.CompiledSchema, doc *container.DataContainer, fullVersion uint16, pt PacketType, opts ...EncodeOption) ([]byte, error) {
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
	return finishFrame(h, buf.Bytes(), newEncodeConfig(opts))
}
