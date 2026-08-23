package anansi

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"

	"github.com/klauspost/compress/zstd"
)

// Wire framing for the implemented transforms (spec sections 4.1–4.3).
// The full top-level packet layout is:
//
//	[flags: u8]                    bit2 = compressed, bit6 = encrypted,
//	                               bit7 = hash present
//	[schema_version: u8]
//	[digest: 16 bytes]             if bit7: BLAKE3(plaintext body)[0..16)
//	[nonce: 12 bytes]              if bit6
//	[payload...]                   see below
//
// The payload depends on the transform flags:
//
//	plain:                          body
//	compressed (bit2):              [plain_len: uvarint][zstd frame]
//	encrypted  (bit6):              AEAD ciphertext+tag over the payload
//	                                the unencrypted framing would carry
//	encrypted + compressed:         AEAD([plain_len][zstd frame])
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

// newAEAD builds the AES-256-GCM AEAD for key.
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

// finishFrame assembles the final top-level packet from h (whose Flags
// carry only epoch/type bits) and the plaintext body, applying the
// configured transforms in spec order: compress, then encrypt.
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

	out := make([]byte, 2, len(body)+len(body)/2+64)
	out[0], out[1] = byte(flags), h.Version
	if cfg.integrity {
		d := packetDigest(body)
		out = append(out, d[:]...)
	}

	inner := body
	if cfg.compressed {
		var lb bytes.Buffer
		writeUvarintTo(&lb, uint64(len(body)))
		lb.Write(zstd.EncodeTo(nil, body))
		inner = lb.Bytes()
	}

	if cfg.key == nil {
		return append(out, inner...), nil
	}

	aead, err := newAEAD(cfg.key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("anansi: generate nonce: %w", err)
	}
	out = append(out, nonce...)
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
		aead, err := newAEAD(cfg.key)
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
