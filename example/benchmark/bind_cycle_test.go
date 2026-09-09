// Package main: BenchmarkBindCycle measures the generated-code user path
// through the document lifecycle, end to end, on a real backend:
//
//	JSON bytes -> orders.FromJSON (custom codec into a pooled container)
//	-> doc.BindTo(&order)      (bind container -> typed struct)
//	-> orders.FromStruct       (bind struct -> container)
//	-> orders.CreateOne        (persist)
//	-> orders.Delete           (cleanup, keeps the store warm-seeded-sized)
//
// Everything goes through the generated full-mode Orders model wrapper
// (schemas.InitOrdersModel), never hand-rolled raw plumbing. The documents
// are ERP-scale orders at three sizes, mirroring core/encoding/json's ERP
// benchmarks so stage costs compare against the raw codec numbers.
//
// The schema is UUIDv7-normalized, so it also runs in production mode; for a
// sibling benchmark the same invocation is documented as:
//
//	ANANSI_ENV=development go test ./example/benchmark/ -bench=BindCycle -benchmem
package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/query/native"
	"github.com/asaidimu/go-anansi/v8/example/benchmark/migrations"
	"github.com/asaidimu/go-anansi/v8/example/benchmark/schemas"
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

// buildOrderPayload renders a deterministic ERP order payload with nLines line
// items. It deliberately omits _id_: the document factory is the only
// component that generates identity. Users do not declare their own id field.
func buildOrderPayload(nLines int) []byte {
	var buf bytes.Buffer
	buf.WriteString(`{"customer":{"name":"Acme Widgets GmbH","email":"billing@acme.example","tier":"gold"}`)
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

type bindEnv struct {
	ctx    context.Context
	orders *schemas.Orders
	ids    []string
}

var envSeq atomic.Uint64

// newBindEnv configures the (empty-provider) document factory, creates an
// in-memory SQLite persistence, applies the generated migrations, wraps the
// collection in the generated Orders model, and seeds warmDocs documents so
// the store is non-trivial.
func newBindEnv(tb testing.TB, nLines, warm int) *bindEnv {
	tb.Helper()
	if err := data.ConfigureDocumentFactory(data.DocumentFactoryConfig{}, nil); err != nil {
		tb.Fatal(err)
	}
	ctx := context.Background()

	dsn := fmt.Sprintf("file:bench_bind_%d?mode=memory&cache=shared", envSeq.Add(1))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })

	logger := zap.NewNop()
	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil {
		tb.Fatal(err)
	}
	interactor, err := native.NewNativeInteractor(executor, sqliteQuery.NewSQLiteFactory(nil), logger)
	if err != nil {
		tb.Fatal(err)
	}
	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	if err != nil {
		tb.Fatal(err)
	}
	if err := migrations.Apply(ctx, p); err != nil {
		tb.Fatal(err)
	}

	orders, err := schemas.InitOrdersModel(p, logger)
	if err != nil {
		tb.Fatal(err)
	}

	env := &bindEnv{ctx: ctx, orders: orders}
	payload := buildOrderPayload(nLines)
	for i := 0; i < warm; i++ {
		doc, err := orders.FromJSON(payload)
		if err != nil {
			tb.Fatal(err)
		}
		res, err := orders.CreateOne(ctx, doc)
		orders.Release(doc)
		if err != nil {
			tb.Fatal(err)
		}
		env.ids = append(env.ids, res.Data.ID())
	}
	return env
}

func idQuery(id string) *query.Query {
	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build()
	return &q
}

func idFilter(id string) *query.QueryFilter {
	return query.NewQueryBuilder().Where(data.DocumentIDField).Eq(id).Build().Filters
}

// cycle runs one full generated-code bind cycle: decode JSON into a pooled
// container, bind container -> struct, bind struct -> container, persist, and
// delete again so the store stays seeded-sized.
func (env *bindEnv) cycle(b *testing.B, payload []byte) {
	b.Helper()
	doc, err := env.orders.FromJSON(payload)
	if err != nil {
		b.Fatal(err)
	}
	var order schemas.Order
	if err := doc.BindTo(&order); err != nil {
		b.Fatal(err)
	}
	env.orders.Release(doc)
	doc2, err := env.orders.FromStruct(&order)
	if err != nil {
		b.Fatal(err)
	}
	res, err := env.orders.CreateOne(env.ctx, doc2)
	env.orders.Release(doc2)
	if err != nil {
		b.Fatal(err)
	}
	id := res.Data.ID()
	if res.Data != nil {
		res.Data.Release()
	}
	if rows, err := env.orders.Delete(env.ctx, idFilter(id), false); err != nil || rows != 1 {
		b.Fatalf("cleanup delete rows=%d err=%v", rows, err)
	}
}

// BenchmarkBindCycle measures the generated-code document cycle per iteration.
func BenchmarkBindCycle(b *testing.B) {
	for _, name := range benchSizes {
		n := erpLineCounts[name]
		env := newBindEnv(b, n, warmDocs)
		payload := buildOrderPayload(n)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				env.cycle(b, payload)
			}
		})
	}
}

// cycleTyped runs the normal user-facing path through the typed
// ModelCollection API — the code real callers write: decode a request into a
// typed struct, create (struct in, struct out), read back by _id_, update,
// then delete. Unlike cycle it never touches a raw Document after decode, so
// it also exercises the read-back containers the typed API binds and returns
// to the pool.
func (env *bindEnv) cycleTyped(b *testing.B, payload []byte) {
	b.Helper()
	doc, err := env.orders.FromJSON(payload)
	if err != nil {
		b.Fatal(err)
	}
	var order schemas.Order
	if err := doc.BindTo(&order); err != nil {
		b.Fatal(err)
	}
	env.orders.Release(doc)

	created, err := env.orders.Create(env.ctx, &order)
	if err != nil {
		b.Fatal(err)
	}
	id := created.GetID()

	readBack, err := env.orders.FindByID(env.ctx, id)
	if err != nil {
		b.Fatal(err)
	}
	if readBack.GetID() != id {
		b.Fatalf("read back id %q != created %q", readBack.GetID(), id)
	}

	if _, err := env.orders.Update(env.ctx, id, &order); err != nil {
		b.Fatal(err)
	}

	if err := env.orders.DeleteByID(env.ctx, id); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkTypedCycle measures the typed generated-code user path end to end.
func BenchmarkTypedCycle(b *testing.B) {
	for _, name := range benchSizes {
		n := erpLineCounts[name]
		env := newBindEnv(b, n, warmDocs)
		payload := buildOrderPayload(n)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				env.cycleTyped(b, payload)
			}
		})
	}
}

// TestBindCycle sanity-checks the generated-code round trip: identity comes
// from the factory, the typed struct round-trips through container binding,
// and the persisted document reads back by _id_.
func TestBindCycle(t *testing.T) {
	env := newBindEnv(t, 3, 0)
	payload := buildOrderPayload(3)

	doc, err := env.orders.FromJSON(payload)
	if err != nil {
		t.Fatal(err)
	}
	var order schemas.Order
	if err := doc.BindTo(&order); err != nil {
		t.Fatal(err)
	}
	env.orders.Release(doc)

	if order.GetID() == "" {
		t.Fatal("expected generated _id_ after decode")
	}
	if len(order.Lines) != 3 {
		t.Fatalf("expected 3 line items, got %d", len(order.Lines))
	}
	if order.Currency == nil || *order.Currency != "EUR" {
		t.Fatalf("expected currency EUR, got %v", order.Currency)
	}
	if order.Status == nil || *order.Status != "processing" {
		t.Fatalf("expected status processing, got %v", order.Status)
	}

	doc2, err := env.orders.FromStruct(&order)
	if err != nil {
		t.Fatal(err)
	}
	res, err := env.orders.CreateOne(env.ctx, doc2)
	env.orders.Release(doc2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Data.ID() != order.GetID() {
		t.Fatalf("persisted id %q != bound id %q", res.Data.ID(), order.GetID())
	}

	readRes, err := env.orders.Read(env.ctx, idQuery(order.GetID()))
	if err != nil {
		t.Fatal(err)
	}
	if len(readRes) != 1 {
		t.Fatalf("expected 1 document by _id_, got %d", len(readRes))
	}
	readBack := readRes[0]
	if readBack.GetID() != order.GetID() {
		t.Fatalf("read back id %q != bound id %q", readBack.GetID(), order.GetID())
	}
	if len(readBack.Lines) != 3 {
		t.Fatalf("read back expected 3 line items, got %d", len(readBack.Lines))
	}

	if rows, err := env.orders.Delete(env.ctx, idFilter(order.GetID()), false); err != nil || rows != 1 {
		t.Fatalf("cleanup delete rows=%d err=%v", rows, err)
	}
}
