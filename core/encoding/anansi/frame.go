package anansi

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/zeebo/blake3"
)

// Wire framing for the implemented transforms (spec sections 4.1–4.3).
// The full top-level packet layout is:
//
//      [flags: u8]                    bit2 = compressed, bit6 = encrypted,
//                                     bit7 = hash present
//      [schema_version: u8]
//      [digest: 16 bytes]             if bit7: BLAKE3(plaintext body)[0..16)
//      [nonce: 12 bytes]              if bit6
//      [payload...]                   see below
//
// The payload depends on the transform flags:
//
//      plain:                          body
//      compressed (bit2):              [plain_len: uvarint][zstd frame]
//      encrypted  (bit6):              AEAD ciphertext+tag over the payload
//                                      the unencrypted framing would carry
//      encrypted + compressed:         AEAD([plain_len][zstd frame])
//
// Ordering follows the spec: encode is plaintext → compress → encrypt
// (4.2.2); decode is decrypt → decompress → verify digest (6.4.1 steps
// 10–12). The digest always covers the fully decoded PLAINTEXT body, never
// compressed or encrypted bytes.
//
// Algorithm documentation per spec 4.1.2/4.2.1 ("Implementations MUST
// document which algorithm they use"):
//   - Compression: ZSTD as implemented by github.com/klauspost/compress,
//     default encoder level, standard frames. The uvarint plain_len
//     realizes the spec's optional "uncompressed_size" field and doubles
//     as the decompression-bomb guard input (9.2.2).
//   - Encryption: AES-256-GCM (stdlib crypto/aes + crypto/cipher), 12-byte
//     random nonce per packet drawn from crypto/rand, 16-byte auth tag
//     appended by Seal. Nonces are never reused deterministically; callers
//     re-encrypting many packets under one key should rotate keys well
//     before the ~2^32-packet birthday bound for 96-bit random nonces.
//     Key management is out of scope (spec 4.2.1) — callers own key
//     storage and rotation.

// hashSize is the truncated digest length in bytes (spec 4.3.1).
const hashSize = 16

// maxDecompressedSize bounds the plain_len any decoder will honor (spec
// 9.2.2): packets declaring more than this are rejected before allocation.
// zstd.DecodeTo additionally enforces its own 1 GiB ceiling regardless of
// what the header claims.
const maxDecompressedSize = 512 << 20 // 512 MiB

// aesKeySize is the required AES-256 key length in bytes.
const aesKeySize = 32

// gcmNonceSize is the standard GCM nonce length in bytes (spec 4.2.2:
// "12 bytes for ChaCha20/AES-GCM").
const gcmNonceSize = 12

// EncodeOption customizes top-level packet encoding.
type EncodeOption func(*encodeConfig)

type encodeConfig struct {
	compressed bool
	integrity  bool
	key        []byte // AES-256 key; nil disables encryption
}

// WithCompression compresses the packet body (flags bit 2). See the frame
// comment above for the exact layout.
func WithCompression() EncodeOption {
	return func(c *encodeConfig) { c.compressed = true }
}

// WithIntegrity embeds the BLAKE3-truncated integrity digest over the
// plaintext body (flags bit 7). Composable with WithCompression and
// WithEncryption.
func WithIntegrity() EncodeOption {
	return func(c *encodeConfig) { c.integrity = true }
}

// WithEncryption seals the packet payload with AES-256-GCM under the given
// 32-byte key (flags bit 6). Composable with the other options; per spec
// 4.2.2 compression happens before encryption.
func WithEncryption(key []byte) EncodeOption {
	return func(c *encodeConfig) { c.key = key }
}

func newEncodeConfig(opts []EncodeOption) encodeConfig {
	var c encodeConfig
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// DecodeOption customizes top-level packet decoding.
type DecodeOption func(*decodeConfig)

type decodeConfig struct {
	key         []byte // AES-256 key for encrypted packets
	copyStrings bool   // opt-out of the default zero-copy string decoding
}

// WithDecryptionKey supplies the AES-256 key needed to open an encrypted
// packet. Decoding a packet with flags bit 6 set without this option fails.
func WithDecryptionKey(key []byte) DecodeOption {
	return func(c *decodeConfig) { c.key = key }
}

// WithCopyStrings opts out of the default zero-copy string decoding,
// materializing each string as its own allocation instead of a view into
// the container's backing buffer. Choose this when decoded documents are
// retained far longer than their wire size justifies (e.g. long-lived
// caches keeping one hot field alive would otherwise pin the whole
// packet's backing). Steady-state decode cost: one memmove and zero string
// allocations by default; nested child containers share the root's backing
// so extracted children remain valid; equal values are not deduplicated.
func WithCopyStrings() DecodeOption {
	return func(c *decodeConfig) { c.copyStrings = true }
}

func newDecodeConfig(opts []DecodeOption) decodeConfig {
	var c decodeConfig
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// ---------------------------------------------------------------------------
// AEAD cache
// ---------------------------------------------------------------------------

// aeadCacheCap bounds the number of distinct keys whose AES-256-GCM
// instances are retained. A full key schedule per encrypt/decrypt call
// dominated encrypt-path profiles; callers overwhelmingly reuse one (or a
// few) keys, and a rotated-out key merely costs one rebuilt instance.
const aeadCacheCap = 64

// aeadCache reuses AES-256-GCM instances per key. Keys are fixed-width
// arrays, so the hot lookup copies the caller's key onto the stack and
// hashes it with zero allocations.
type aeadCacheT struct {
	mu sync.RWMutex
	m  map[[aesKeySize]byte]cipher.AEAD
}

var aeadCache = &aeadCacheT{m: make(map[[aesKeySize]byte]cipher.AEAD, 4)}

// cachedAEAD returns the AEAD for key, building and caching it on first
// use. Keys longer/shorter than 32 bytes are rejected here so every caller
// shares one validation path.
func cachedAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != aesKeySize {
		return nil, fmt.Errorf("anansi: AES-256-GCM requires a %d-byte key, got %d bytes", aesKeySize, len(key))
	}
	var ak [aesKeySize]byte
	copy(ak[:], key)

	aeadCache.mu.RLock()
	aead, ok := aeadCache.m[ak]
	aeadCache.mu.RUnlock()
	if ok {
		return aead, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("anansi: init cipher: %w", err)
	}
	aead, err = cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("anansi: init GCM: %w", err)
	}

	aeadCache.mu.Lock()
	if len(aeadCache.m) >= aeadCacheCap {
		// Pathological key rotation: reset rather than grow without bound.
		aeadCache.m = make(map[[aesKeySize]byte]cipher.AEAD, 4)
	}
	aeadCache.m[ak] = aead
	aeadCache.mu.Unlock()
	return aead, nil
}

// newAEAD builds a fresh AES-256-GCM AEAD for key. Retained for callers
// that deliberately want an uncached instance; hot paths use cachedAEAD.
func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != aesKeySize {
		return nil, fmt.Errorf("anansi: AES-256-GCM requires a %d-byte key, got %d bytes", aesKeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("anansi: init cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("anansi: init GCM: %w", err)
	}
	return aead, nil
}

// ---------------------------------------------------------------------------
// Integrity hashing (spec 4.3)
// ---------------------------------------------------------------------------

// blake3Pool reuses BLAKE3 hashers across packets. Sum writes into a
// caller-provided 32-byte stack buffer (the digest's full width) so the
// hot path allocates nothing.
var blake3Pool = sync.Pool{
	New: func() any { return blake3.New() },
}

// packetDigest computes BLAKE3(payload)[0..hashSize) (spec 4.3.2).
func packetDigest(payload []byte) [hashSize]byte {
	var out [hashSize]byte
	h := blake3Pool.Get().(*blake3.Hasher)
	h.Reset()
	// hash.Hash's Write never returns an error for the in-memory hasher.
	_, _ = h.Write(payload)
	var sum [32]byte // full BLAKE3 digest width — a 16-byte buffer cannot hold it
	copy(out[:], h.Sum(sum[:0])[:hashSize])
	blake3Pool.Put(h)
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

// ---------------------------------------------------------------------------
// Frame assembly / opening
// ---------------------------------------------------------------------------

// finishFrame assembles the final top-level packet from h (whose Flags
// carry only epoch/type bits) and the plaintext body, applying the
// configured transforms in spec order: compress, then encrypt.
//
// The output is one exactly-sized allocation owned by the caller — nothing
// that is returned ever comes from a pool. (A previous revision returned
// pooled buffers while re-queuing them, aliasing two consecutive packets'
// bytes; the second encode silently overwrote the first packet — the bug
// TestEncryption_TamperAndNonceUniqueness now guards.)
func finishFrame(h header, body []byte, cfg encodeConfig) ([]byte, error) {
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

	inner := body
	if cfg.compressed {
		// zstd.EncodeTo allocates its exact output; the plain_len varint
		// (spec 4.1) fronts it inside the payload area.
		z := zstd.EncodeTo(nil, body)
		inner = make([]byte, uvarintLen(uint64(len(body))), uvarintLen(uint64(len(body)))+len(z))
		writeUvarintTo(bytes.NewBuffer(inner[:0]), uint64(len(body)))
		inner = append(inner, z...)
	}

	total := 2 + len(inner)
	if cfg.integrity {
		total += hashSize
	}
	if cfg.key != nil {
		total += gcmNonceSize + 16 // nonce + GCM tag
	}
	out := make([]byte, 0, total)

	out = append(out, byte(flags), h.Version)
	if cfg.integrity {
		d := packetDigest(body)
		out = append(out, d[:]...)
	}

	if cfg.key == nil {
		return append(out, inner...), nil
	}

	aead, err := cachedAEAD(cfg.key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("anansi: generate nonce: %w", err)
	}
	out = append(out, nonce...)
	// Seal appends ciphertext+tag; total reserved room for it exactly.
	return aead.Seal(out, nonce, inner, nil), nil
}

// openFrame consumes everything between the header and the body — optional
// digest, optional nonce+ciphertext, optional compression envelope —
// verifies integrity when flagged, and returns a reader positioned at the
// start of the decodable body (decrypt → decompress per 6.4.1). owned
// reports whether the returned reader's buffer is freshly allocated by this
// call (compressed/encrypted paths) and therefore already private to the
// decoded containers; callers can attach it as backing without copying.
func openFrame(r *byteReader, flags Flags, cfg decodeConfig) (_ *byteReader, owned bool, _ error) {
	fresh := false
	var stored []byte
	if flags.HashPresent() {
		b, err := r.readN(hashSize)
		if err != nil {
			return nil, false, fmt.Errorf("anansi: read packet hash: %w", err)
		}
		stored = b
	}

	if flags.Encrypted() {
		if cfg.key == nil {
			return nil, false, fmt.Errorf("anansi: encrypted packet but no decryption key provided (use anansi.WithDecryptionKey)")
		}
		aead, err := cachedAEAD(cfg.key)
		if err != nil {
			return nil, false, err
		}
		nonce, err := r.readN(gcmNonceSize)
		if err != nil {
			return nil, false, fmt.Errorf("anansi: read nonce: %w", err)
		}
		inner, err := aead.Open(nil, nonce, r.data[r.pos:], nil)
		if err != nil {
			return nil, false, fmt.Errorf("anansi: decrypt payload: %w", err)
		}
		r = newByteReader(inner)
		fresh = true
	}

	var plainLen uint64 = uint64(r.remaining())
	if flags.Compressed() {
		n, err := r.readUvarint()
		if err != nil {
			return nil, false, fmt.Errorf("anansi: read uncompressed size: %w", err)
		}
		if n > maxDecompressedSize {
			return nil, false, fmt.Errorf("anansi: declared uncompressed size %d exceeds limit %d", n, uint64(maxDecompressedSize))
		}
		plainLen = n
		plain, err := zstd.DecodeTo(make([]byte, 0, plainLen), r.data[r.pos:])
		if err != nil {
			return nil, false, fmt.Errorf("anansi: decompress body: %w", err)
		}
		if uint64(len(plain)) != plainLen {
			return nil, false, fmt.Errorf("anansi: decompressed %d bytes, header declared %d", uint64(len(plain)), plainLen)
		}
		r = newByteReader(plain)
		fresh = true
	}

	if stored != nil {
		if err := verifyIntegrity(stored, r.data[r.pos:]); err != nil {
			return nil, false, err
		}
	}
	return r, fresh, nil
}
