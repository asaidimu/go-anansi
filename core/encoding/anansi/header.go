package anansi

import "fmt"

// PacketType identifies which of the four Anansi packet layouts a payload
// uses (spec 2.1.1).
type PacketType uint8

const (
	PacketDense  PacketType = 0
	PacketSparse PacketType = 1
	PacketBatch  PacketType = 2
	PacketStream PacketType = 3
)

func (t PacketType) String() string {
	switch t {
	case PacketDense:
		return "Dense"
	case PacketSparse:
		return "Sparse"
	case PacketBatch:
		return "Batch"
	case PacketStream:
		return "Stream"
	default:
		return fmt.Sprintf("PacketType(%d)", uint8(t))
	}
}

// Flags is the single-byte header flags field (spec 2.1.1):
//
//	bits 0-1: packet type
//	bit  2  : compression
//	bit  3  : encoding mode (batch: 0=row, 1=columnar)
//	bits 4-5: version epoch
//	bit  6  : encryption
//	bit  7  : hash present
type Flags uint8

const (
	flagPacketTypeMask = 0x03
	flagCompressed     = 0x04
	flagBatchColumnar  = 0x08
	flagEpochShift     = 4
	flagEpochMask      = 0x03
	flagEncrypted      = 0x40
	flagHashPresent    = 0x80
)

// PacketType extracts the packet type from the flags byte.
func (f Flags) PacketType() PacketType { return PacketType(f & flagPacketTypeMask) }

// Compressed reports whether bit 2 (compression) is set.
func (f Flags) Compressed() bool { return f&flagCompressed != 0 }

// BatchColumnar reports whether bit 3 (batch encoding mode) is set. Only
// meaningful when PacketType() == PacketBatch.
func (f Flags) BatchColumnar() bool { return f&flagBatchColumnar != 0 }

// Epoch extracts the 2-bit version epoch (bits 4-5).
func (f Flags) Epoch() uint8 { return uint8(f>>flagEpochShift) & flagEpochMask }

// Encrypted reports whether bit 6 (encryption) is set.
func (f Flags) Encrypted() bool { return f&flagEncrypted != 0 }

// HashPresent reports whether bit 7 (hash present) is set.
func (f Flags) HashPresent() bool { return f&flagHashPresent != 0 }

// newFlags builds a flags byte from its components (spec 2.1.1). This
// implementation only ever produces uncompressed, unencrypted, unhashed
// payloads with epoch 0 — compression/encryption/hashing are advanced
// features (spec section 4/8) outside the scope of this codec, but the bit
// layout is honored so any future extension composes cleanly and so
// well-formed packets from other implementations decode correctly.
func newFlags(pt PacketType, columnar bool) Flags {
	f := Flags(pt) & flagPacketTypeMask
	if columnar {
		f |= flagBatchColumnar
	}
	return f
}

// header is the common 2-byte packet prefix (spec 2.1): flags + schema
// version. fullVersion combines the version byte with the flags' epoch bits
// (spec 2.2) to address up to 1024 distinct schema versions.
type header struct {
	Flags   Flags
	Version uint8
}

// FullVersion computes (epoch << 8) | version (spec 2.2).
func (h header) FullVersion() uint16 {
	return uint16(h.Flags.Epoch())<<8 | uint16(h.Version)
}

// writeHeader appends the 2-byte header to buf.
func writeHeader(buf []byte, h header) []byte {
	return append(buf, byte(h.Flags), h.Version)
}

// readHeader reads the 2-byte header from the front of r.
func readHeader(r *byteReader) (header, error) {
	fb, err := r.readByte()
	if err != nil {
		return header{}, fmt.Errorf("anansi: read header flags: %w", err)
	}
	vb, err := r.readByte()
	if err != nil {
		return header{}, fmt.Errorf("anansi: read header version: %w", err)
	}
	return header{Flags: Flags(fb), Version: vb}, nil
}

// schemaVersionByte splits a full 10-bit version into its epoch (bits 4-5 of
// the flags byte) and version byte (byte 1) components.
func schemaVersionByte(fullVersion uint16) (epoch uint8, version uint8, err error) {
	if fullVersion > 1023 {
		return 0, 0, fmt.Errorf("anansi: schema version %d exceeds maximum of 1023", fullVersion)
	}
	return uint8(fullVersion >> 8), uint8(fullVersion & 0xFF), nil
}
