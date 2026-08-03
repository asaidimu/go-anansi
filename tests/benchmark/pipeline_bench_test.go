// Package benchmark measures the full document pipeline end to end:
//
//	JSON bytes -> parse (custom codec into a pooled container-backed document)
//	-> manipulate (typed Set) -> persist (ephemeral collection)
//	-> fetch (read back by _id_) -> serialize (JSON bytes on the wire)
//
// The documents are ERP-scale orders — header, nested customer object, two
// mounts of a shared address schema, an array of line items — at three sizes,
// mirroring core/encoding/json's ERP benchmarks so stage costs can be compared
// against the raw codec numbers. Persistence uses a real backend: a unique
// in-memory SQLite database per benchmark, pre-seeded with warmDocs documents
// so reads and writes operate on a non-trivial, constant-size store.
//
// Run with the development validator (the registry requires UUIDv7 field IDs
// in production mode):
//
//	ANANSI_ENV=development go test ./tests/benchmark/ -bench=BenchmarkPipeline -benchmem
//
// Parse always goes through DocumentPool.FromJSON (the custom codec): payloads
// carry no _id_, and the document factory generates it, so identity is never
// produced anywhere else in the pipeline.
package benchmark

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/document"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/query/native"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	sqliteExecutor "github.com/asaidimu/go-anansi/v8/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v8/sqlite/query"
)

// warmDocs seeds every store so insert unique-scans and fetch filters operate
// on a non-trivial collection while staying constant across iterations.
const warmDocs = 200

var erpLineCounts = map[string]int{
	"small": 5, "medium": 25, "large": 120,
}

var benchSizes = []string{"small", "medium", "large"}

const erpSchemaJSON = `{
  "version": "1.0.0",
  "name": "erp_order",
  "fields": {
    "id":              { "name": "id",              "type": "string" },
    "customer":        { "name": "customer",        "type": "object", "schema": { "id": "customer" } },
    "billing_address": { "name": "billing_address", "type": "object", "schema": { "id": "address" } },
    "shipping_address":{"name": "shipping_address", "type": "object", "schema": { "id": "address" } },
    "lines":           { "name": "lines",           "type": "array",  "schema": { "id": "order_line" } },
    "status":          { "name": "status",          "type": "string" },
    "currency":        { "name": "currency",        "type": "string" },
    "subtotal":        { "name": "subtotal",        "type": "number" },
    "tax":             { "name": "tax",             "type": "number" },
    "total":           { "name": "total",           "type": "number" },
    "notes":           { "name": "notes",           "type": "array",  "schema": { "type": "string" } }
  },
  "schemas": {
    "customer": {
      "name": "customer",
      "fields": {
        "customer_id": { "name": "id",    "type": "string" },
        "name":        { "name": "name",  "type": "string" },
        "email":       { "name": "email", "type": "string" },
        "tier":        { "name": "tier",  "type": "string" }
      }
    },
    "address": {
      "name": "address",
      "fields": {
        "street": { "name": "street", "type": "string" },
        "city":   { "name": "city",   "type": "string" },
        "region": { "name": "region", "type": "string" },
        "postal": { "name": "postal", "type": "string" },
        "country":{ "name": "country","type": "string" }
      }
    },
    "order_line": {
      "name": "order_line",
      "fields": {
        "sku":         { "name": "sku",         "type": "string" },
        "description": { "name": "description", "type": "string" },
        "qty":         { "name": "qty",         "type": "integer" },
        "unit_price":  { "name": "unit_price",  "type": "number" },
        "discount":    { "name": "discount",    "type": "number" },
        "tax_rate":    { "name": "tax_rate",    "type": "number" },
        "line_total":  { "name": "line_total",  "type": "number" }
      }
    }
  }
}`

// buildOrder renders a deterministic ERP order payload with nLines line items.
// It deliberately omits _id_: the document factory is the only component that
// generates identity, so the parse stage produces it.
func buildOrder(ordID string, nLines int) []byte {
	var buf bytes.Buffer
	buf.WriteString(`{"id":`)
	buf.WriteString(fmt.Sprintf("%q", ordID))
	buf.WriteString(`,"customer":{"id":"CUST-1234","name":"Acme Widgets GmbH","email":"billing@acme.example","tier":"gold"}`)
	buf.WriteString(`,"billing_address":{"street":"1 Market St","city":"Berlin","region":"BE","postal":"10115","country":"DE"}`)
	buf.WriteString(`,"shipping_address":{"street":"42 Harbor Way","city":"Hamburg","region":"HH","postal":"20095","country":"DE"}`)
	buf.WriteString(`,"lines":[`)
	var subtotal float64
	for i := 0; i < nLines; i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		qty := i%9 + 1
		unit := 10.0 + float64(i%50) + 0.99
		discount := 0.0
		if i%3 == 0 {
			discount = 0.10
		}
		lineTotal := float64(qty) * unit * (1 - discount)
		subtotal += lineTotal
		buf.WriteString(`{"sku":"SKU-`)
		buf.WriteString(fmt.Sprintf("%05d", i+1))
		buf.WriteString(`","description":"Widget brass `)
		buf.WriteString(fmt.Sprintf("%.0f", 8+float64(i%40)))
		buf.WriteString(` mm","qty":`)
		buf.WriteString(fmt.Sprintf("%d", qty))
		buf.WriteString(`,"unit_price":`)
		buf.WriteString(fmt.Sprintf("%.2f", unit))
		buf.WriteString(`,"discount":`)
		buf.WriteString(fmt.Sprintf("%.2f", discount))
		buf.WriteString(`,"tax_rate":0.19,"line_total":`)
		buf.WriteString(fmt.Sprintf("%.2f", lineTotal))
		buf.WriteString(`}`)
	}
	tax := subtotal * 0.19
	buf.WriteString(`],"status":"processing","currency":"EUR","subtotal":`)
	buf.WriteString(fmt.Sprintf("%.2f", subtotal))
	buf.WriteString(`,"tax":`)
	buf.WriteString(fmt.Sprintf("%.2f", tax))
	buf.WriteString(`,"total":`)
	buf.WriteString(fmt.Sprintf("%.2f", subtotal+tax))
	buf.WriteString(`,"notes":["Priority customer","Fragile items"]}`)
	return buf.Bytes()
}

type benchEnv struct {
	ctx  context.Context
	pool *document.DocumentPool
	coll base.Collection
	ids  []string
}

var envSeq atomic.Uint64

// newEnv configures the (empty-provider) document factory, builds the schema
// pool, creates an in-memory SQLite persistence with a collection for the
// schema, and seeds warmDocs documents so the store is non-trivial.
func newEnv(b *testing.B, nLines, warm int) *benchEnv {
	b.Helper()
	if err := data.ConfigureDocumentFactory(data.DocumentFactoryConfig{}, nil); err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	sc, err := definition.FromJSON([]byte(erpSchemaJSON))
	if err != nil {
		b.Fatal(err)
	}
	pool, err := document.NewDocumentPool(sc)
	if err != nil {
		b.Fatal(err)
	}

	// Unique named in-memory database per env; cache=shared lets the sql.DB
	// connection pool share one database.
	dsn := fmt.Sprintf("file:bench_%d?mode=memory&cache=shared", envSeq.Add(1))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { db.Close() })

	logger := zap.NewNop()
	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil {
		b.Fatal(err)
	}
	queryFactory := sqliteQuery.NewSQLiteFactory(nil)
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil {
		b.Fatal(err)
	}
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	if err != nil {
		b.Fatal(err)
	}
	coll, err := p.CreateCollection(ctx, sc)
	if err != nil {
		b.Fatal(err)
	}
	env := &benchEnv{ctx: ctx, pool: pool, coll: coll}
	payload := buildOrder("ORD-SEED", nLines)
	for i := 0; i < warm; i++ {
		doc, err := pool.FromJSON(payload)
		if err != nil {
			b.Fatal(err)
		}
		res, err := coll.CreateMany(ctx, []data.Documenter{doc})
		if err != nil {
			b.Fatal(err)
		}
		if res[0].Status != base.StatusCreated {
			b.Fatalf("seed insert failed: %v", res[0].Error)
		}
		env.ids = append(env.ids, res[0].Data.ID())
		pool.Release(doc)
	}
	return env
}

func (env *benchEnv) parse(b *testing.B, payload []byte) *document.Document {
	b.Helper()
	doc, err := env.pool.FromJSON(payload)
	if err != nil {
		b.Fatal(err)
	}
	return doc
}

func (env *benchEnv) manipulate(b *testing.B, doc *document.Document) {
	b.Helper()
	if err := doc.Set("status", "shipped"); err != nil {
		b.Fatal(err)
	}
	if err := doc.Set("currency", "USD"); err != nil {
		b.Fatal(err)
	}
}

func idQuery(id string) *query.Query {
	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build()
	return &q
}

func idFilter(id string) *query.QueryFilter {
	return query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build().Filters
}

// BenchmarkPipeline_Parse decodes the JSON payload into a pooled
// container-backed document (custom codec + identity metadata + checksum),
// releasing the container back to the pool each iteration.
func BenchmarkPipeline_Parse(b *testing.B) {
	for _, name := range benchSizes {
		n := erpLineCounts[name]
		env := newEnv(b, n, 0)
		payload := buildOrder("ORD-2026-000042", n)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				env.pool.Release(env.parse(b, payload))
			}
		})
	}
}

// BenchmarkPipeline_Manipulate measures typed Set mutations on an already
// parsed document.
func BenchmarkPipeline_Manipulate(b *testing.B) {
	for _, name := range benchSizes {
		n := erpLineCounts[name]
		env := newEnv(b, n, 0)
		payload := buildOrder("ORD-2026-000042", n)
		b.Run(name, func(b *testing.B) {
			doc := env.parse(b, payload)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				env.manipulate(b, doc)
			}
		})
	}
}

// BenchmarkPipeline_Persist materializes a parsed document to a map and inserts
// it into the in-memory SQLite collection, then deletes it again so the store
// stays seeded-sized.
func BenchmarkPipeline_Persist(b *testing.B) {
	for _, name := range benchSizes {
		n := erpLineCounts[name]
		env := newEnv(b, n, warmDocs)
		payload := buildOrder("ORD-2026-000042", n)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				doc := env.parse(b, payload)
				res, err := env.coll.CreateMany(env.ctx, []data.Documenter{doc})
				if err != nil {
					b.Fatal(err)
				}
				if res[0].Status != base.StatusCreated {
					b.Fatalf("create failed: %v", res[0].Error)
				}
				id := res[0].Data.ID()
				env.pool.Release(doc)
				if rows, err := env.coll.Delete(env.ctx, idFilter(id), false); err != nil || rows != 1 {
					b.Fatalf("cleanup delete rows=%d err=%v", rows, err)
				}
			}
		})
	}
}

// BenchmarkPipeline_Fetch reads a seeded document back by _id_ from SQLite.
func BenchmarkPipeline_Fetch(b *testing.B) {
	for _, name := range benchSizes {
		n := erpLineCounts[name]
		env := newEnv(b, n, warmDocs)
		b.Run(name, func(b *testing.B) {
			id := env.ids[len(env.ids)-1]
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				res, err := env.coll.Read(env.ctx, idQuery(id))
				if err != nil {
					b.Fatal(err)
				}
				if res.Count != 1 {
					b.Fatalf("expected 1 doc, got %d", res.Count)
				}
			}
		})
	}
}

// BenchmarkPipeline_Serialize marshals an already-fetched record-view document
// (the egress form) back to JSON bytes for the wire.
func BenchmarkPipeline_Serialize(b *testing.B) {
	for _, name := range benchSizes {
		n := erpLineCounts[name]
		env := newEnv(b, n, warmDocs)
		b.Run(name, func(b *testing.B) {
			res, err := env.coll.Read(env.ctx, idQuery(env.ids[len(env.ids)-1]))
			if err != nil {
				b.Fatal(err)
			}
			doc := res.Data[0]
			if _, err := json.Marshal(doc); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := json.Marshal(doc); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPipeline_FullCycle runs the entire pipeline per iteration — parse,
// manipulate, persist, fetch, serialize, cleanup — on a store kept at a
// constant size by inserting and deleting each document.
func BenchmarkPipeline_FullCycle(b *testing.B) {
	for _, name := range benchSizes {
		n := erpLineCounts[name]
		env := newEnv(b, n, warmDocs)
		payload := buildOrder("ORD-2026-000042", n)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				doc := env.parse(b, payload)
				env.manipulate(b, doc)
				res, err := env.coll.CreateMany(env.ctx, []data.Documenter{doc})
				if err != nil {
					b.Fatal(err)
				}
				if res[0].Status != base.StatusCreated {
					b.Fatalf("create failed: %v", res[0].Error)
				}
				id := res[0].Data.ID()
				env.pool.Release(doc)
				readRes, err := env.coll.Read(env.ctx, idQuery(id))
				if err != nil {
					b.Fatal(err)
				}
				if readRes.Count != 1 {
					b.Fatalf("read failed: count=%d", readRes.Count)
				}
				if _, err := json.Marshal(readRes.Data[0]); err != nil {
					b.Fatal(err)
				}
				if rows, err := env.coll.Delete(env.ctx, idFilter(id), false); err != nil || rows != 1 {
					b.Fatalf("cleanup delete rows=%d err=%v", rows, err)
				}
			}
		})
	}
}
