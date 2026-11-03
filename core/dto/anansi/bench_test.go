package anansi_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/document"
	anansi "github.com/asaidimu/go-anansi/v8/core/dto/anansi"
	"github.com/asaidimu/go-anansi/v8/core/dto/anansijson"
	cschema "github.com/asaidimu/go-anansi/v8/core/schema"
)

type Address struct {
	Street string `anansi:"street" json:"street"`
	City   string `anansi:"city" json:"city"`
	Zip    string `anansi:"zip" json:"zip"`
}

type OrderLine struct {
	SKU       string  `anansi:"sku" json:"sku"`
	Quantity  int     `anansi:"qty" json:"qty"`
	UnitPrice float64 `anansi:"unit_price" json:"unit_price"`
}

type Order struct {
	ID       string      `anansi:"id" json:"id"`
	Customer string      `anansi:"customer" json:"customer"`
	Status   string      `anansi:"status" json:"status"`
	Priority int         `anansi:"priority" json:"priority"`
	Total    float64     `anansi:"total" json:"total"`
	Paid     bool        `anansi:"paid" json:"paid"`
	Tags     []string    `anansi:"tags" json:"tags"`
	Billing  Address     `anansi:"billing" json:"billing"`
	Lines    []OrderLine `anansi:"lines" json:"lines"`
}

func sampleAddress(i int) Address {
	return Address{
		Street: "123 Main Street",
		City:   "Springfield",
		Zip:    "12345",
	}
}

func sampleOrder(i int) Order {
	return Order{
		ID:       "ord-000123",
		Customer: "Acme Corporation",
		Status:   "shipped",
		Priority: 2,
		Total:    199.99,
		Paid:     true,
		Tags:     []string{"priority", "gift", "export"},
		Billing: Address{
			Street: "1 Billing Way",
			City:   "Springfield",
			Zip:    "10001",
		},
		Lines: []OrderLine{
			{SKU: "SKU-001", Quantity: 3, UnitPrice: 19.99},
			{SKU: "SKU-002", Quantity: 1, UnitPrice: 139.99},
			{SKU: "SKU-003", Quantity: 2, UnitPrice: 9.99},
		},
	}
}

func TestJSONRoundTrip(t *testing.T) {
	for _, in := range []any{sampleAddress(1), sampleOrder(1)} {
		b, err := anansijson.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal(%T): %v", in, err)
		}
		out := reflect.New(reflect.TypeOf(in)).Interface()
		if err := anansijson.Unmarshal(b, out); err != nil {
			t.Fatalf("Unmarshal(%T): %v", in, err)
		}
		if got := reflect.ValueOf(out).Elem().Interface(); !reflect.DeepEqual(in, got) {
			t.Fatalf("%T round trip mismatch:\n in=%+v\nout=%+v", in, in, got)
		}
	}
}

func TestAnansiRoundTrip(t *testing.T) {
	for _, in := range []any{sampleAddress(1), sampleOrder(1)} {
		b, err := anansi.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal(%T): %v", in, err)
		}
		out := reflect.New(reflect.TypeOf(in)).Interface()
		if err := anansi.Unmarshal(b, out); err != nil {
			t.Fatalf("Unmarshal(%T): %v", in, err)
		}
		if got := reflect.ValueOf(out).Elem().Interface(); !reflect.DeepEqual(in, got) {
			t.Fatalf("%T round trip mismatch:\n in=%+v\nout=%+v", in, in, got)
		}
	}
}

// TestJSONOutputShape checks that the document-model bookkeeping (_id_,
// _metadata_) does not leak into the wire output.
func TestJSONOutputShape(t *testing.T) {
	b, err := anansijson.Marshal(sampleOrder(1))
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err := json.Unmarshal(b, &keys); err != nil {
		t.Fatal(err)
	}
	for k := range keys {
		if k == "_id_" || k == "_metadata_" {
			t.Errorf("output contains system field %q; keys: %v", k, keysOf(keys))
		}
	}
	t.Logf("json payload (%d B): %s", len(b), b)
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// TestUnmarshalReleaseSafety poisons the pool: repeated unmarshals through
// the same cached pool, checking that earlier results stay intact. If the
// JSON decoder aliased container storage, the first struct's strings would
// mutate when the backing is refilled.
func TestUnmarshalReleaseSafety(t *testing.T) {
	var firstJSON, firstAnansi Order
	bJSON, _ := json.Marshal(sampleOrder(1))
	bAJSON, _ := anansijson.Marshal(sampleOrder(1))
	bAnansi, _ := anansi.Marshal(sampleOrder(1))

	if err := anansijson.Unmarshal(bAJSON, &firstJSON); err != nil {
		t.Fatal(err)
	}
	if err := anansi.Unmarshal(bAnansi, &firstAnansi); err != nil {
		t.Fatal(err)
	}

	for i := 2; i < 8; i++ {
		var x, y Order
		bx, _ := anansijson.Marshal(sampleOrder(i))
		if err := anansijson.Unmarshal(bx, &x); err != nil {
			t.Fatal(err)
		}
		if err := anansi.Unmarshal(bAnansi, &y); err != nil {
			t.Fatal(err)
		}
		_ = bx
		_ = bJSON
	}
	if !reflect.DeepEqual(firstJSON, sampleOrder(1)) {
		t.Fatalf("pooled JSON unmarshal corrupted earlier result: %+v", firstJSON)
	}
	if !reflect.DeepEqual(firstAnansi, sampleOrder(1)) {
		t.Fatalf("anansi unmarshal corrupted earlier result: %+v", firstAnansi)
	}
}

// TestDeterministicOutput pins the property the document-free path unlocks:
// identical input must produce byte-identical output (no generated UUID,
// timestamps, or checksum). Document-backed marshaling could never promise
// this.
func TestDeterministicOutput(t *testing.T) {
	for _, m := range []struct {
		name string
		fn   func(any) ([]byte, error)
	}{
		{"json", anansijson.Marshal},
		{"anansi", anansi.Marshal},
	} {
		first, err := m.fn(sampleOrder(1))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 5; i++ {
			again, err := m.fn(sampleOrder(1))
			if err != nil {
				t.Fatal(err)
			}
			if string(first) != string(again) {
				t.Fatalf("%s marshal is nondeterministic", m.name)
			}
		}
	}
}

// --- benchmarks ---

func benchPayloadJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func BenchmarkJSONMarshal(b *testing.B) {
	small := sampleAddress(1)
	large := sampleOrder(1)
	stdSmall, _ := json.Marshal(small)
	stdLarge, _ := json.Marshal(large)

	b.Run("stdlib/small", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(len(stdSmall)), "bytes")
		for i := 0; i < b.N; i++ {
			if _, err := json.Marshal(&small); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("anansijson/small", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(len(stdSmall)), "bytes")
		for i := 0; i < b.N; i++ {
			if _, err := anansijson.Marshal(&small); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("anansijson-append/small", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(len(stdSmall)), "bytes")
		buf := make([]byte, 0, 1024)
		for i := 0; i < b.N; i++ {
			var err error
			buf, err = anansijson.MarshalAppend(buf[:0], &small)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("stdlib/large", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(len(stdLarge)), "bytes")
		for i := 0; i < b.N; i++ {
			if _, err := json.Marshal(&large); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("anansijson/large", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(len(stdLarge)), "bytes")
		for i := 0; i < b.N; i++ {
			if _, err := anansijson.Marshal(&large); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("anansijson-append/large", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(len(stdLarge)), "bytes")
		buf := make([]byte, 0, 1024)
		for i := 0; i < b.N; i++ {
			var err error
			buf, err = anansijson.MarshalAppend(buf[:0], &large)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkJSONUnmarshal(b *testing.B) {
	small := sampleAddress(1)
	large := sampleOrder(1)
	smallIn := benchPayloadJSON(&small)
	largeIn := benchPayloadJSON(&large)

	b.Run("stdlib/small", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(smallIn)))
		for i := 0; i < b.N; i++ {
			var v Address
			if err := json.Unmarshal(smallIn, &v); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("anansijson/small", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(smallIn)))
		for i := 0; i < b.N; i++ {
			var v Address
			if err := anansijson.Unmarshal(smallIn, &v); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("stdlib/large", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(largeIn)))
		for i := 0; i < b.N; i++ {
			var v Order
			if err := json.Unmarshal(largeIn, &v); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("anansijson/large", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(largeIn)))
		for i := 0; i < b.N; i++ {
			var v Order
			if err := anansijson.Unmarshal(largeIn, &v); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkDocumentPathMarshal measures the pre-refactor flow for comparison:
// a document pool whose schema is enriched with _id_/_metadata_, FromStruct's
// identity/metadata/checksum bookkeeping, and ToAnansi serialization. The gap
// to BenchmarkAnansi/marshal is the cost of the document model on a pure
// serialization path.
func BenchmarkDocumentPathMarshal(b *testing.B) {
	small := sampleAddress(1)
	large := sampleOrder(1)

	newPool := func(v any) *document.DocumentPool {
		t := reflect.TypeOf(v)
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		schema, err := cschema.ExtractDTOSchemaDirect(reflect.New(t).Interface(), true)
		if err != nil {
			b.Fatal(err)
		}
		pool, err := document.NewDocumentPoolFromJSON(schema)
		if err != nil {
			b.Fatal(err)
		}
		return pool
	}
	smallPool := newPool(&small)
	largePool := newPool(&large)

	b.Run("small", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			doc, err := smallPool.FromStruct(&small)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := doc.ToAnansi(0x0100); err != nil {
				b.Fatal(err)
			}
			smallPool.Release(doc)
		}
	})
	b.Run("large", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			doc, err := largePool.FromStruct(&large)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := doc.ToAnansi(0x0100); err != nil {
				b.Fatal(err)
			}
			largePool.Release(doc)
		}
	})
}

func BenchmarkAnansi(b *testing.B) {
	small := sampleAddress(1)
	large := sampleOrder(1)
	smallBin, err := anansi.Marshal(&small)
	if err != nil {
		b.Fatal(err)
	}
	largeBin, err := anansi.Marshal(&large)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("marshal/small", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(len(smallBin)), "bytes")
		for i := 0; i < b.N; i++ {
			if _, err := anansi.Marshal(&small); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("marshal/large", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(len(largeBin)), "bytes")
		for i := 0; i < b.N; i++ {
			if _, err := anansi.Marshal(&large); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("unmarshal/small", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(smallBin)))
		for i := 0; i < b.N; i++ {
			var v Address
			if err := anansi.Unmarshal(smallBin, &v); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("unmarshal/large", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(largeBin)))
		for i := 0; i < b.N; i++ {
			var v Order
			if err := anansi.Unmarshal(largeBin, &v); err != nil {
				b.Fatal(err)
			}
		}
	})
}
