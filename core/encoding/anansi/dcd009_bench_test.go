package anansi_test

// BenchmarkBatchRows_IntOnly pins #dcd009's resolved snapshot skip: a schema
// with no aliasable string carriers (integer-only records) decodes without
// the zero-copy body snapshot, so DecodeBatchRows never bulk-copies the
// caller's wire buffer for bytes nothing aliases.

import (
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const benchIntOnlySchemaJSON = `{
  "version": "1.0.0",
  "name": "intonly",
  "fields": {
    "a": { "name": "a", "type": "integer" },
    "b": { "name": "b", "type": "integer" },
    "c": { "name": "c", "type": "integer" }
  }
}`

func benchIntOnlyDocs(b *testing.B, cs *definition.CompiledSchema, n int) []*container.DataContainer {
	b.Helper()
	docs := make([]*container.DataContainer, n)
	for i := 0; i < n; i++ {
		docs[i] = container.NewDataContainer()
		payload := fmt.Sprintf(`{"a": %d, "b": %d, "c": %d}`, i, i*2, i*3)
		if err := cjson.DecodeJSONInto(cs, []byte(payload), docs[i], nil); err != nil {
			b.Fatalf("build doc: %v", err)
		}
	}
	return docs
}

func BenchmarkBatchRows_IntOnly(b *testing.B) {
	s, err := definition.FromJSON([]byte(benchIntOnlySchemaJSON))
	if err != nil {
		b.Fatalf("schema parse: %v", err)
	}
	rs, err := definition.Compile(s)
	if err != nil {
		b.Fatalf("schema compile: %v", err)
	}
	cs, err := definition.Link(rs)
	if err != nil {
		b.Fatalf("schema link: %v", err)
	}

	docs := benchIntOnlyDocs(b, cs, 100)
	wire, err := anansi.EncodeBatchRows(cs, docs, 1)
	if err != nil {
		b.Fatalf("batch encode: %v", err)
	}
	b.SetBytes(int64(len(wire)))

	b.Run("decode/fresh", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := anansi.DecodeBatchRows(cs, wire, nil); err != nil {
				b.Fatalf("batch decode: %v", err)
			}
		}
	})
	b.Run("decode/pooled", func(b *testing.B) {
		pool := container.NewPool()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			got, _, err := anansi.DecodeBatchRows(cs, wire, pool)
			if err != nil {
				b.Fatalf("batch decode: %v", err)
			}
			for _, d := range got {
				pool.Put(d)
			}
		}
	})
}
