package anansi_test

import (
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
)

// TestIntegrityHash_RoundTrip wraps every top-level packet type with an
// integrity digest (spec 4.3.2) and verifies each decodes identically.
func TestIntegrityHash_RoundTrip(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)
	want := containerJSON(t, cs, doc)

	batchDocs := columnarDocs(t, cs, columnarPayloads)

	cases := []struct {
		name    string
		encode  func() ([]byte, error)
		version uint16
	}{
		{"auto", func() ([]byte, error) { return anansi.SerializeAnansi(cs, doc, 0) }, 0},
		{"dense", func() ([]byte, error) { return anansi.EncodeDense(cs, doc, 0) }, 0},
		{"sparse", func() ([]byte, error) { return anansi.EncodeSparse(cs, doc, 0) }, 0},
		{"batch_row", func() ([]byte, error) {
			return anansi.EncodeBatchRows(cs, batchDocs, 0)
		}, 0},
		{"batch_columnar", func() ([]byte, error) {
			return anansi.EncodeBatchColumnar(cs, batchDocs, 0)
		}, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			packet, err := c.encode()
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if packet[0]&0x80 != 0 {
				t.Fatal("encoder set the hash flag before wrapping")
			}

			hashed, err := anansi.WithIntegrityHash(packet)
			if err != nil {
				t.Fatalf("WithIntegrityHash: %v", err)
			}
			if hashed[0]&0x80 == 0 {
				t.Fatal("wrapped packet must carry flags bit 7")
			}
			if len(hashed) != len(packet)+16 {
				t.Fatalf("wrapped length = %d, want %d (original packet + 16-byte digest inserted after byte 1)", len(hashed), len(packet)+16)
			}

			switch c.name {
			case "batch_row", "batch_columnar":
				decoded, version, err := anansi.DecodeBatchRows(cs, hashed, nil)
				if err != nil {
					t.Fatalf("DecodeBatchRows: %v", err)
				}
				if version != c.version {
					t.Fatalf("version = %d, want %d", version, c.version)
				}
				if got := containerJSON(t, cs, decoded[0]); got != want {
					t.Fatalf("first record mismatch:\n want: %s\n got:  %s", want, got)
				}
			default:
				out := container.NewDataContainer()
				version, err := anansi.DecodeAnansiInto(cs, hashed, out, nil)
				if err != nil {
					t.Fatalf("DecodeAnansiInto: %v", err)
				}
				if version != c.version {
					t.Fatalf("version = %d, want %d", version, c.version)
				}
				if got := containerJSON(t, cs, out); got != want {
					t.Fatalf("round-trip mismatch:\n want: %s\n got:  %s", want, got)
				}
			}
		})
	}
}

func TestIntegrityHash_TamperDetection(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)

	packet, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatalf("SerializeAnansi: %v", err)
	}
	hashed, err := anansi.WithIntegrityHash(packet)
	if err != nil {
		t.Fatalf("WithIntegrityHash: %v", err)
	}

	// Flip a payload byte deep inside the document body.
	body := append([]byte(nil), hashed...)
	body[len(body)-3] ^= 0xFF
	out := container.NewDataContainer()
	if _, err := anansi.DecodeAnansiInto(cs, body, out, nil); err == nil {
		t.Fatal("expected integrity failure for tampered payload")
	} else if !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("expected integrity error, got: %v", err)
	}

	// Corrupt the stored digest itself.
	digest := append([]byte(nil), hashed...)
	digest[5] ^= 0x01
	if _, err := anansi.DecodeAnansiInto(cs, digest, container.NewDataContainer(), nil); err == nil {
		t.Fatal("expected integrity failure for corrupted digest")
	}

	// A packet flagged as hashed but truncated before the full digest.
	short := hashed[:10]
	if _, err := anansi.DecodeAnansiInto(cs, short, container.NewDataContainer(), nil); err == nil {
		t.Fatal("expected error for truncated digest")
	}
}

func TestIntegrityHash_PreservesEpochAndNestedPackets(t *testing.T) {
	cs := loadCompiledSchema(t)
	// fullPayload contains an items array-of-object, so the payload embeds
	// nested sub-packets; their headers must not inherit the hash bit
	// (they are covered by the parent digest instead).
	doc := containerFromJSON(t, cs, fullPayload)

	packet, err := anansi.SerializeAnansi(cs, doc, 300) // epoch 1
	if err != nil {
		t.Fatalf("SerializeAnansi(300): %v", err)
	}
	hashed, err := anansi.WithIntegrityHash(packet)
	if err != nil {
		t.Fatalf("WithIntegrityHash: %v", err)
	}

	version, err := anansi.DecodeAnansiInto(cs, hashed, container.NewDataContainer(), nil)
	if err != nil {
		t.Fatalf("DecodeAnansiInto: %v", err)
	}
	if version != 300 {
		t.Fatalf("fullVersion = %d, want 300 (epoch bits must survive hashing)", version)
	}
}

func TestIntegrityHash_RejectsHashingCompressedFlag(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, `{"count": 1}`)

	packet, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatalf("SerializeAnansi: %v", err)
	}
	flagged := append([]byte(nil), packet...)
	flagged[0] |= 0x04 // pretend compressed
	if _, err := anansi.WithIntegrityHash(flagged); err == nil {
		t.Fatal("hashing a packet marked compressed must fail: spec hashes plaintext only")
	}
}

func TestIntegrityHash_UnhashedStillDecodes(t *testing.T) {
	cs := loadCompiledSchema(t)
	doc := containerFromJSON(t, cs, fullPayload)
	want := containerJSON(t, cs, doc)

	packet, err := anansi.SerializeAnansi(cs, doc, 0)
	if err != nil {
		t.Fatalf("SerializeAnansi: %v", err)
	}
	out := container.NewDataContainer()
	if _, err := anansi.DecodeAnansiInto(cs, packet, out, nil); err != nil {
		t.Fatalf("DecodeAnansiInto: %v", err)
	}
	if got := containerJSON(t, cs, out); got != want {
		t.Fatal("unhashed decode mismatch")
	}
}
