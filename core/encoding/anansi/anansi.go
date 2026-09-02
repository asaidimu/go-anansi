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
