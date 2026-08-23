package anansi

import (
	"bytes"
	"fmt"
	"math/bits"
)

// This file implements the Anansi Binary Wire Format's variable-length
// integer encodings (spec section 2.4): unsigned LEB128 varints and
// zigzag-encoded signed varints, plus the shared error type raised on
// malformed or truncated varint data.

// maxVarintBytes is the maximum number of bytes a 64-bit varint may occupy
// (spec 2.4.3): ceil(64/7) = 10, but the spec caps it at 9 bytes for 64-bit
// values assuming the top byte only needs 1 extra bit; we accept up to 10 to
// be strictly safe against all-ones inputs while still rejecting runaway
// streams (security 9.2.1).
const maxVarintBytes = 10

// ErrBufferUnderflow is returned when a decode operation runs out of input
// bytes before completing a field (spec 9.1).
var ErrBufferUnderflow = fmt.Errorf("anansi: buffer underflow")

// ErrVarintTooLong is returned when a varint exceeds the maximum encodable
// size, guarding against malicious or corrupt streams (spec 9.2.1).
var ErrVarintTooLong = fmt.Errorf("anansi: varint exceeds maximum size")

// putUvarint appends the unsigned LEB128 encoding of v to buf and returns the
// extended buffer.
func putUvarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

// putVarint appends the zigzag+LEB128 encoding of the signed value v.
func putVarint(buf []byte, v int64) []byte {
	return putUvarint(buf, zigzagEncode(v))
}

// zigzagEncode maps signed n to an unsigned value so small-magnitude negative
// numbers stay small on the wire (spec 2.4.2).
func zigzagEncode(n int64) uint64 {
	return uint64((n << 1) ^ (n >> 63))
}

// zigzagDecode reverses zigzagEncode.
func zigzagDecode(u uint64) int64 {
	return int64(u>>1) ^ -int64(u&1)
}

// byteReader is a minimal cursor over an in-memory buffer used by the Anansi
// decoders. It never allocates on the hot path and reports buffer underflow
// as a normal error rather than panicking, per spec 9.2.1.
type byteReader struct {
	data []byte
	pos  int
}

func newByteReader(data []byte) *byteReader {
	return &byteReader{data: data}
}

// remaining returns the number of unread bytes.
func (r *byteReader) remaining() int { return len(r.data) - r.pos }

// eof reports whether the cursor has consumed all input.
func (r *byteReader) eof() bool { return r.pos >= len(r.data) }

// readByte consumes and returns a single byte.
func (r *byteReader) readByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, ErrBufferUnderflow
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

// readN consumes and returns the next n bytes as a sub-slice of the
// underlying buffer (no copy). Callers that need to retain the bytes beyond
// the lifetime of the input buffer must copy them explicitly.
func (r *byteReader) readN(n int) ([]byte, error) {
	if n < 0 || r.remaining() < n {
		return nil, ErrBufferUnderflow
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

// readUvarint reads an unsigned LEB128 varint (spec 2.4.1).
func (r *byteReader) readUvarint() (uint64, error) {
	var x uint64
	var s uint
	for i := 0; i < maxVarintBytes; i++ {
		b, err := r.readByte()
		if err != nil {
			return 0, err
		}
		if b < 0x80 {
			if i == maxVarintBytes-1 && b > 1 {
				return 0, ErrVarintTooLong
			}
			return x | uint64(b)<<s, nil
		}
		x |= uint64(b&0x7F) << s
		s += 7
	}
	return 0, ErrVarintTooLong
}

// readVarint reads a zigzag+LEB128 encoded signed varint (spec 2.4.2).
func (r *byteReader) readVarint() (int64, error) {
	u, err := r.readUvarint()
	if err != nil {
		return 0, err
	}
	return zigzagDecode(u), nil
}

// uvarintLen returns the number of bytes putUvarint would emit for v,
// without allocating — used for pre-sizing buffers.
func uvarintLen(v uint64) int {
	if v == 0 {
		return 1
	}
	return (bits.Len64(v) + 6) / 7
}

// writeUvarintTo appends the varint encoding of v directly to a bytes.Buffer,
// avoiding an intermediate slice allocation on the hot encode path.
func writeUvarintTo(buf *bytes.Buffer, v uint64) {
	var tmp [maxVarintBytes]byte
	n := 0
	for v >= 0x80 {
		tmp[n] = byte(v) | 0x80
		v >>= 7
		n++
	}
	tmp[n] = byte(v)
	n++
	buf.Write(tmp[:n])
}

// writeVarintTo appends the zigzag varint encoding of v to buf.
func writeVarintTo(buf *bytes.Buffer, v int64) {
	writeUvarintTo(buf, zigzagEncode(v))
}
