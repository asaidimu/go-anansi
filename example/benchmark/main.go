// Command benchmark demonstrates the generated-code document lifecycle:
//
//	schema JSON on disk -> anansi migrate generate -> migrations.Apply
//	-> schemas.InitOrdersModel (generated full-mode wrapper)
//	-> JSON decode straight into a pooled container (orders.FromJSON)
//	-> bind container -> typed struct (doc.BindTo)
//	-> bind struct -> container (orders.FromStruct)
//	-> persist (orders.CreateOne)
//
// The container pool is the collection's own: the generated Orders wrapper
// embeds *document.DocumentPool, which the base collection compiles from the
// active schema (itself sourced from the on-disk JSON via migrations), so the
// generated path never touches an intermediate JSON struct.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/asaidimu/go-anansi/v8"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"go.uber.org/zap"

	"github.com/asaidimu/go-anansi/v8/example/benchmark/migrations"
	"github.com/asaidimu/go-anansi/v8/example/benchmark/schemas"
)

func main() {
	ctx := context.Background()

	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "playground:", err)
		os.Exit(1)
	}
	defer cleanup()

	if err := migrations.Apply(ctx, p); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}

	orders, err := schemas.InitOrdersModel(p, zap.NewNop())
	if err != nil {
		fmt.Fprintln(os.Stderr, "init model:", err)
		os.Exit(1)
	}

	// The payload carries no _id_: the document factory generates identity,
	// and users do not declare their own id field.
	raw := []byte(`{
		"customer": {"name": "Acme Widgets GmbH", "email": "billing@acme.example", "tier": "gold"},
		"billing_address": {"street": "1 Market St", "city": "Berlin", "region": "BE", "postal": "10115", "country": "DE"},
		"shipping_address": {"street": "42 Harbor Way", "city": "Hamburg", "region": "HH", "postal": "20095", "country": "DE"},
		"lines": [
			{"sku": "SKU-00001", "description": "Widget brass 8 mm", "qty": 2, "unit_price": 10.99, "tax_rate": 0.19},
			{"sku": "SKU-00002", "description": "Widget brass 9 mm", "qty": 1, "unit_price": 12.99, "tax_rate": 0.19}
		],
		"status": "processing",
		"currency": "EUR",
		"subtotal": 34.97,
		"tax": 6.64,
		"total": 41.61,
		"notes": ["Priority customer", "Fragile items"]
	}`)

	doc, err := orders.FromJSON(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}
	defer orders.Release(doc)

	var order schemas.Order
	if err := doc.BindTo(&order); err != nil {
		fmt.Fprintln(os.Stderr, "bind container->struct:", err)
		os.Exit(1)
	}

	doc2, err := orders.FromStruct(&order)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bind struct->container:", err)
		os.Exit(1)
	}
	res, err := orders.CreateOne(ctx, doc2)
	orders.Release(doc2)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create:", err)
		os.Exit(1)
	}

	total := 0.0
	if order.Total != nil {
		total = *order.Total
	}
	fmt.Printf("created order %s (%d line item(s)) total=%.2f\n", res.Data.ID(), len(order.Lines), total)

	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(res.Data.ID()).Build()
	readOrders, err := orders.Read(ctx, &q)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	fmt.Printf("read back by _id_: %d document(s)\n", len(readOrders))
}
