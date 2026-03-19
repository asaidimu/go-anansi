package ir_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func TestSerialize_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		src  []byte
	}{
		{"Flat", flatSchema},
		{"Nested", nestedObjectSchema},
		{"Union", unionSchema},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cs := mustCompile(c.src, nil)
			out, err := ir.Serialize(cs)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}

			ss := mustParse(out)
			cs2, err := ir.Compile(ss, nil)
			if err != nil {
				t.Fatalf("Compile of serialized: %v", err)
			}

			out2, _ := ir.Serialize(cs2)
			if string(out) != string(out2) {
				t.Error("Serialization round-trip mismatch")
			}
		})
	}
}
