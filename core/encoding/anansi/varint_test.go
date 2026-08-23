package anansi

import (
	"bytes"
	"testing"
)

func TestUvarintRoundTrip(t *testing.T) {
	cases := []uint64{0, 1, 127, 128, 16383, 16384, 1 << 20, 1 << 40, 1<<63 - 1, ^uint64(0)}
	for _, v := range cases {
		buf := putUvarint(nil, v)
		if len(buf) != uvarintLen(v) {
			t.Fatalf("uvarintLen(%d) = %d, actual encoding is %d bytes", v, uvarintLen(v), len(buf))
		}
		r := newByteReader(buf)
		got, err := r.readUvarint()
		if err != nil {
			t.Fatalf("readUvarint(%d): %v", v, err)
		}
		if got != v {
			t.Fatalf("uvarint round-trip: want %d got %d", v, got)
		}
		if !r.eof() {
			t.Fatalf("uvarint(%d): expected buffer fully consumed, %d bytes remain", v, r.remaining())
		}
	}
}

func TestVarintRoundTrip(t *testing.T) {
	cases := []int64{0, -1, 1, -2, 2, 1 << 40, -(1 << 40), -9223372036854775808, 9223372036854775807}
	for _, v := range cases {
		buf := putVarint(nil, v)
		r := newByteReader(buf)
		got, err := r.readVarint()
		if err != nil {
			t.Fatalf("readVarint(%d): %v", v, err)
		}
		if got != v {
			t.Fatalf("varint round-trip: want %d got %d", v, got)
		}
	}
}

func TestWriteUvarintToMatchesPutUvarint(t *testing.T) {
	cases := []uint64{0, 1, 127, 128, 300, 1 << 30, ^uint64(0)}
	for _, v := range cases {
		var buf bytes.Buffer
		writeUvarintTo(&buf, v)
		want := putUvarint(nil, v)
		if !bytes.Equal(buf.Bytes(), want) {
			t.Fatalf("writeUvarintTo(%d) = %x, want %x", v, buf.Bytes(), want)
		}
	}
}

func TestReadUvarintBufferUnderflow(t *testing.T) {
	// A continuation byte (high bit set) with nothing following must error,
	// not panic or read out of bounds.
	r := newByteReader([]byte{0x80})
	if _, err := r.readUvarint(); err != ErrBufferUnderflow {
		t.Fatalf("expected ErrBufferUnderflow, got %v", err)
	}
}

func TestReadUvarintTooLong(t *testing.T) {
	// 10 continuation bytes with a non-terminal final byte should be
	// rejected rather than silently overflowing.
	buf := bytes.Repeat([]byte{0xFF}, 10)
	r := newByteReader(buf)
	if _, err := r.readUvarint(); err == nil {
		t.Fatalf("expected an error for an over-long varint")
	}
}

func TestByteReaderReadNUnderflow(t *testing.T) {
	r := newByteReader([]byte{1, 2, 3})
	if _, err := r.readN(4); err != ErrBufferUnderflow {
		t.Fatalf("expected ErrBufferUnderflow, got %v", err)
	}
	// A valid, smaller read should still succeed afterward from the start.
	r2 := newByteReader([]byte{1, 2, 3})
	got, err := r2.readN(2)
	if err != nil {
		t.Fatalf("readN(2): %v", err)
	}
	if !bytes.Equal(got, []byte{1, 2}) {
		t.Fatalf("readN(2) = %v, want [1 2]", got)
	}
}
