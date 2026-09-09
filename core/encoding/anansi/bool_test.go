package anansi

import (
	"bytes"
	"testing"
)

func TestBoolBitsRoundTrip(t *testing.T) {
	cases := [][]bool{
		{},
		{true},
		{false},
		{true, false, true, true, false, false, true, false},
		{false, false, false, false, false, false, false, false, true}, // 9 values
		make([]bool, 64), // exactly 8 bytes
		func() []bool {
			v := make([]bool, 100)
			for i := range v {
				if i%3 == 0 {
					v[i] = true
				}
			}
			return v
		}(),
	}
	for _, vals := range cases {
		var buf bytes.Buffer
		w := binPut{buf: make([]byte, (len(vals)+7)/8)}
		putBoolBits(&w, vals)
		buf.Write(w.buf[:w.pos])
		if got := buf.Len(); got != (len(vals)+7)/8 {
			t.Fatalf("packed %d bools into %d bytes, want %d", len(vals), got, (len(vals)+7)/8)
		}
		back, err := readBoolBits(newByteReader(buf.Bytes()), len(vals))
		if err != nil {
			t.Fatalf("readBoolBits(%d): %v", len(vals), err)
		}
		for i := range vals {
			if back[i] != vals[i] {
				t.Fatalf("%d bools: index %d = %v, want %v", len(vals), i, back[i], vals[i])
			}
		}
	}
}

func TestReadBoolBits_Underflow(t *testing.T) {
	if _, err := readBoolBits(newByteReader([]byte{}), 9); err == nil {
		t.Fatal("expected underflow error")
	}
}
