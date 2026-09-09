package anansi_test

import (
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
)

// TestCompression_RoundTrip exercises every top-level encoder with
// compression alone and combined with the integrity digest (spec 4.1.2
// "with hash" layout: digest over plaintext, wire carries the frame).
func TestCompression_RoundTrip(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)
	want := containerJSON(t, cs, doc)

	batchDocs := columnarDocs(t, cs, columnarPayloads)

	cases := []struct {
		name    string
		encode  func(...anansi.EncodeOption) ([]byte, error)
		isBatch bool
	}{
		{"auto", func(o ...anansi.EncodeOption) ([]byte, error) {
			return anansi.SerializeAnansi(cs, doc, 0, o...)
		}, false},
		{"dense", func(o ...anansi.EncodeOption) ([]byte, error) {
			return anansi.EncodeDense(cs, doc, 0, o...)
		}, false},
		{"sparse", func(o ...anansi.EncodeOption) ([]byte, error) {
			return anansi.EncodeSparse(cs, doc, 0, o...)
		}, false},
		{"batch_row", func(o ...anansi.EncodeOption) ([]byte, error) {
			return anansi.EncodeBatchRows(cs, batchDocs, 0, o...)
		}, true},
		{"batch_columnar", func(o ...anansi.EncodeOption) ([]byte, error) {
			return anansi.EncodeBatchColumnar(cs, batchDocs, 0, o...)
		}, true},
	}

	optionSets := []struct {
		name string
		opts []anansi.EncodeOption
	}{
		{"compressed", []anansi.EncodeOption{anansi.WithCompression()}},
		{"compressed_hashed", []anansi.EncodeOption{anansi.WithCompression(), anansi.WithIntegrity()}},
	}

	for _, c := range cases {
		for _, os := range optionSets {
			t.Run(c.name+"/"+os.name, func(t *testing.T) {
				packet, err := c.encode(os.opts...)
				if err != nil {
					t.Fatalf("encode: %v", err)
				}
				if packet[0]&0x04 == 0 {
					t.Fatal("flags byte missing compression bit")
				}
				if strings.Contains(os.name, "hashed") && packet[0]&0x80 == 0 {
					t.Fatal("flags byte missing hash bit")
				}

				if c.isBatch {
					decoded, _, err := anansi.DecodeBatchRows(cs, packet, nil)
					if err != nil {
						t.Fatalf("DecodeBatchRows: %v", err)
					}
					if got := containerJSON(t, cs, decoded[0]); got != want {
						t.Fatalf("first record mismatch:\n want: %s\n got:  %s", want, got)
					}
					return
				}
				out := container.NewDataContainer()
				if _, err := anansi.DecodeAnansiInto(cs, packet, out, nil); err != nil {
					t.Fatalf("DecodeAnansiInto: %v", err)
				}
				if got := containerJSON(t, cs, out); got != want {
					t.Fatalf("round-trip mismatch:\n want: %s\n got:  %s", want, got)
				}
			})
		}
	}
}

// TestCompression_ShieldsNothingFromDigest pins that tampering anywhere in
// a compressed+hashed packet — frame bytes or digest — is rejected.
func TestCompression_TamperRejected(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)

	packet, err := anansi.SerializeAnansi(cs, doc, 0, anansi.WithCompression(), anansi.WithIntegrity())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Flip a byte inside the compressed region.
	body := append([]byte(nil), packet...)
	body[len(body)-5] ^= 0x01
	if _, err := anansi.DecodeAnansiInto(cs, body, container.NewDataContainer(), nil); err == nil {
		t.Fatal("tampered compressed packet must be rejected")
	}

	// Corrupt the stored digest.
	digest := append([]byte(nil), packet...)
	digest[8] ^= 0x80
	if _, err := anansi.DecodeAnansiInto(cs, digest, container.NewDataContainer(), nil); err == nil {
		t.Fatal("corrupted digest must be rejected")
	}
}

// TestCompression_BombGuard checks the spec 9.2.2 decompression-bomb
// defense: a declared plain_len beyond maxDecompressedSize is rejected
// without attempting decompression.
func TestCompression_BombGuard(t *testing.T) {
	cs := loadCompiledSchema(t)

	// Hand-craft: compressed bit only, epoch 0, version 0, no digest,
	// plain_len = 1 GiB (> the 512 MiB limit), garbage "frame".
	var plainLen []byte
	for v := uint64(1) << 30; v >= 0x80; v >>= 7 {
		plainLen = append(plainLen, byte(v)|0x80)
	}
	plainLen = append(plainLen, byte(uint64(1)<<30>>28)) // final low bits (0x04)
	pkt := append([]byte{0x04, 0x00}, plainLen...)
	pkt = append(pkt, 'j', 'u', 'n', 'k')
	out := container.NewDataContainer()
	if _, err := anansi.DecodeAnansiInto(cs, pkt, out, nil); err == nil {
		t.Fatal("declared oversized plain_len must be rejected")
	} else if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected limit rejection, got: %v", err)
	}
}

// TestCompression_DeclaredLengthMismatch ensures a lying plain_len is
// caught after decoding (defense in depth behind the pre-decode guard).
func TestCompression_DeclaredLengthMismatch(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)

	compressed, err := anansi.SerializeAnansi(cs, doc, 0, anansi.WithCompression())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Truncate so the zstd frame cannot possibly yield the declared size,
	// while keeping the frame structurally decodable enough to fail the
	// length check or decode — either way it must error.
	trunc := compressed[:len(compressed)-10]
	if _, err := anansi.DecodeAnansiInto(cs, trunc, container.NewDataContainer(), nil); err == nil {
		t.Fatal("truncated compressed body must be rejected")
	}
}

// TestCompression_SanityShrinks guards against silently wiring the flag
// without actually compressing: a highly repetitive document must shrink.
func TestCompression_SanityShrinks(t *testing.T) {
	cs := loadCompiledSchema(t)
	const big = "the quick brown fox jumps over the lazy dog "
	payload := `{"title": "` + strings.Repeat(big, 200) + `"}`
	doc := containerFromJSON(t, cs, payload)

	plain, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatalf("plain encode: %v", err)
	}
	zipped, err := anansi.SerializeAnansi(cs, doc, 0, anansi.WithCompression())
	if err != nil {
		t.Fatalf("compressed encode: %v", err)
	}
	if len(zipped) >= len(plain) {
		t.Fatalf("compression did not shrink payload: plain=%d zipped=%d", len(plain), len(zipped))
	}
}
