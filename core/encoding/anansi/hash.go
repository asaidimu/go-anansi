package anansi

import (
	"crypto/subtle"
	"fmt"

	"github.com/zeebo/blake3"
)

// Integrity hashing (spec 4.3): every packet may carry a 16-byte BLAKE3
// digest of its payload, signalled by bit 7 of the flags byte. The digest
// sits immediately after the 2-byte header (bytes 2..18) and covers all
// remaining bytes — the plaintext payload (spec 4.1: hashing happens on
// uncompressed plaintext; this codec never produces compressed or
// encrypted packets, so the payload is always plaintext here).
//
// The spec mandates BLAKE3 truncated to 128 bits. This implementation uses
// github.com/zeebo/blake3 (AVX2/SSE4.1/NEON-accelerated, hash.Hash-style API).

// hashSize is the truncated digest length in bytes (spec 4.3.1).
const hashSize = 16

// packetDigest computes BLAKE3(payload)[0..hashSize) (spec 4.3.2).
func packetDigest(payload []byte) [hashSize]byte {
	var out [hashSize]byte
	h := blake3.New()
	// hash.Hash's Write never returns an error for the in-memory hasher.
	_, _ = h.Write(payload)
	copy(out[:], h.Sum(nil))
	return out
}

// WithIntegrityHash takes a complete packet produced by this package
// (SerializeAnansi, EncodeDense, EncodeSparse, EncodeBatchRows,
// EncodeBatchColumnar — including any nested sub-packets in its payload)
// and returns a copy with flags bit 7 set and the packet's integrity
// digest inserted after byte 1 (spec 4.3.2). The input is not modified.
//
// Every decode entry point verifies the digest when present and rejects
// tampered payloads (spec 9.1: "Hash mismatch | Reject").
func WithIntegrityHash(packet []byte) ([]byte, error) {
	if len(packet) < 2 {
		return nil, ErrBufferUnderflow
	}
	if Flags(packet[0]).Compressed() || Flags(packet[0]).Encrypted() {
		return nil, fmt.Errorf("anansi: cannot hash a compressed/encrypted packet (spec 4.3 hashes plaintext)")
	}
	if len(packet) < 2+hashSize {
		return nil, fmt.Errorf("anansi: packet too short to carry an integrity hash")
	}

	out := make([]byte, 0, len(packet)+hashSize)
	out = append(out, packet[0]|byte(flagHashPresent), packet[1])
	digest := packetDigest(packet[2:])
	out = append(out, digest[:]...)
	out = append(out, packet[2:]...)
	return out, nil
}

// verifyIntegrity compares stored against the digest of payload,
// constant-time (spec 6.4.1 step 12, 9.1).
func verifyIntegrity(stored, payload []byte) error {
	want := packetDigest(payload)
	if subtle.ConstantTimeCompare(stored, want[:]) != 1 {
		return fmt.Errorf("anansi: integrity check failed (packet digest mismatch)")
	}
	return nil
}

// readAndVerifyNestedHash verifies the optional digest of a raw nested
// sub-packet. Unlike top-level packets, nested packets never carry
// transforms (encodePacketBody strips those bits), so there is no
// compression envelope to open — only hash-or-not.
func readAndVerifyNestedHash(r *byteReader, flags Flags) error {
	if !flags.HashPresent() {
		return nil
	}
	stored, err := r.readN(hashSize)
	if err != nil {
		return fmt.Errorf("anansi: read nested packet hash: %w", err)
	}
	return verifyIntegrity(stored, r.data[r.pos:])
}
