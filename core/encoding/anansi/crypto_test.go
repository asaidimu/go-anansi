package anansi_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
)

var testKey = bytes.Repeat([]byte{0x42}, 32)  // valid AES-256 key
var otherKey = bytes.Repeat([]byte{0x77}, 32) // different valid key
var shortKey = bytes.Repeat([]byte{0x11}, 16) // invalid length

// TestEncryption_RoundTrip covers the spec transform matrix end to end:
// encryption alone, encryption+compression (4.2.2 order), and the full
// stack including the plaintext digest (4.3.2).
func TestEncryption_RoundTrip(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)
	want := containerJSON(t, cs, doc)

	batchDocs := columnarDocs(t, cs, columnarPayloads)

	cases := []struct {
		name    string
		opts    []anansi.EncodeOption
		isBatch bool
	}{
		{"enc", []anansi.EncodeOption{anansi.WithEncryption(testKey)}, false},
		{"enc_comp", []anansi.EncodeOption{anansi.WithEncryption(testKey), anansi.WithCompression()}, false},
		{"enc_comp_hash", []anansi.EncodeOption{
			anansi.WithEncryption(testKey), anansi.WithCompression(), anansi.WithIntegrity(),
		}, false},
		{"batch_enc", []anansi.EncodeOption{anansi.WithEncryption(testKey)}, true},
		{"batch_enc_comp_hash", []anansi.EncodeOption{
			anansi.WithEncryption(testKey), anansi.WithCompression(), anansi.WithIntegrity(),
		}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var packet []byte
			var err error
			if c.isBatch {
				packet, err = anansi.EncodeBatchColumnar(cs, batchDocs, 300, c.opts...)
			} else {
				packet, err = anansi.SerializeAnansi(cs, doc, 300, c.opts...)
			}
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if packet[0]&0x40 == 0 {
				t.Fatal("flags byte missing encryption bit")
			}

			if c.isBatch {
				decoded, version, err := anansi.DecodeBatchRows(cs, packet, nil, anansi.WithDecryptionKey(testKey))
				if err != nil {
					t.Fatalf("DecodeBatchRows: %v", err)
				}
				if version != 300 {
					t.Fatalf("version = %d, want 300", version)
				}
				if got := containerJSON(t, cs, decoded[0]); got != want {
					t.Fatalf("first record mismatch:\n want: %s\n got:  %s", want, got)
				}
				return
			}
			out := container.NewDataContainer()
			version, err := anansi.DecodeAnansiInto(cs, packet, out, nil, anansi.WithDecryptionKey(testKey))
			if err != nil {
				t.Fatalf("DecodeAnansiInto: %v", err)
			}
			if version != 300 {
				t.Fatalf("version = %d, want 300", version)
			}
			if got := containerJSON(t, cs, out); got != want {
				t.Fatalf("round-trip mismatch:\n want: %s\n got:  %s", want, got)
			}
		})
	}
}

func TestEncryption_KeyHandling(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)

	packet, err := anansi.SerializeAnansi(cs, doc, 0, anansi.WithEncryption(testKey))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Missing key must fail with a clear message.
	out := container.NewDataContainer()
	if _, err := anansi.DecodeAnansiInto(cs, packet, out, nil); err == nil {
		t.Fatal("decoding encrypted packet without a key must fail")
	} else if !strings.Contains(err.Error(), "WithDecryptionKey") {
		t.Fatalf("expected missing-key guidance, got: %v", err)
	}

	// Wrong key must fail AEAD authentication.
	if _, err := anansi.DecodeAnansiInto(cs, packet, container.NewDataContainer(), nil, anansi.WithDecryptionKey(otherKey)); err == nil {
		t.Fatal("wrong key must be rejected")
	}

	// Invalid key size must be rejected at encode time.
	if _, err := anansi.SerializeAnansi(cs, doc, 0, anansi.WithEncryption(shortKey)); err == nil {
		t.Fatal("short key must be rejected at encode")
	}
}

func TestEncryption_TamperAndNonceUniqueness(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)

	first, err := anansi.SerializeAnansi(cs, doc, 0, anansi.WithEncryption(testKey))
	if err != nil {
		t.Fatalf("first encode: %v", err)
	}
	second, err := anansi.SerializeAnansi(cs, doc, 0, anansi.WithEncryption(testKey))
	if err != nil {
		t.Fatalf("second encode: %v", err)
	}
	// Random nonces mean identical plaintext never encrypts to identical
	// wire bytes (spec 4.2.2 nonce-reuse warning).
	if bytes.Equal(first, second) {
		t.Fatal("two encryptions of the same document produced identical packets")
	}

	// Flip a ciphertext byte; AEAD authentication must reject it before any
	// parsing happens.
	tampered := append([]byte(nil), first...)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := anansi.DecodeAnansiInto(cs, tampered, container.NewDataContainer(), nil, anansi.WithDecryptionKey(testKey)); err == nil {
		t.Fatal("tampered ciphertext must be rejected")
	}
}
