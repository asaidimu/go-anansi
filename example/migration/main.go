package main

import (
	"context"
	"fmt"
	"log"

	anansi "github.com/asaidimu/go-anansi/v7"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/asaidimu/go-anansi/v7/example/migration/migrations"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	ctx := context.Background()

	// ── Phase 1: Simulate the first deployment ─────────────────────
	// Schema v1.0.0: name, price, stock only (no category, no
	// description). The generated migration will later add them.
	fmt.Println("=== Phase 1: Deploy initial schema (v1.0.0) ===")

	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		DBPath: "/tmp/migration-demo.db",
	})
	if err != nil {
		log.Fatalf("playground: %v", err)
	}
	defer cleanup()

	// Create collection at v1.0.0 directly — name, price, stock, category.
	// (simulates the first deploy before "description" was added).
	j := `{"version":"1.0.0","name":"Products","fields":{"019f405a-e686-75b6-95da-737a99188596":{"name":"name","required":true,"unique":true,"type":"string"},"019f405a-e686-764c-839f-a217470c321c":{"name":"price","required":true,"type":"number"},"019f405a-e686-7658-b3c8-fd15c3db2889":{"name":"stock","required":true,"type":"integer"},"019f405a-e686-7663-b32a-ccbccfc18c52":{"name":"category","type":"string"}}}`
	v1, _ := definition.FromJSON([]byte(j))
	if err := p.CreateCollections(ctx, []*definition.Schema{v1}); err != nil {
		log.Fatalf("create v1: %v", err)
	}

	sc, _ := p.Schema(ctx, "Products")
	fmt.Printf("Collection created at version: %s\n", sc.Version)

	// Seed data
	coll, _ := p.Collection(ctx, "Products")
	docs, _ := coll.CreateMany(ctx, []*data.Document{
		data.MustNewDocument(map[string]any{"name": "Widget", "price": 9.99, "stock": 100}),
		data.MustNewDocument(map[string]any{"name": "Gadget", "price": 24.99, "stock": 50}),
	})
	fmt.Printf("Seeded %d documents\n\n", len(docs))

	// ── Phase 2: Schema evolution ──────────────────────────────────
	// A new developer edits products.schema.json (adds "category" and
	// "description"), runs "anansi schema migrate", which generates the
	// 1.0.0→1.1.0 migration. On next app start, Apply detects the
	// version gap and migrates in place.

	fmt.Println("=== Phase 2: migrations.Apply discovers pending migration ===")

	if err := migrations.Apply(ctx, p); err != nil {
		log.Fatalf("apply: %v", err)
	}

	sc, _ = p.Schema(ctx, "Products")
	fmt.Printf("Collection at version: %s\n", sc.Version)
	fmt.Print("Fields: ")
	first := true
	for _, f := range sc.Fields {
		if !first {
			fmt.Print(", ")
		}
		fmt.Printf("%s (%s)", f.Name, f.FieldProperties.Type.String())
		first = false
	}
	fmt.Println()

	// Verify existing data survived the migration
	result, _ := p.Collection(ctx, "Products")
	r, _ := result.Read(ctx, &query.Query{})
	fmt.Printf("Documents still accessible: %d\n", len(r.Data))
	for _, doc := range r.Data {
		n, _ := doc.Get("name")
		p, _ := doc.Get("price")
		fmt.Printf("  %s ($%.2f)\n", n, p)
	}

	// ── Phase 3: Idempotent re-run ─────────────────────────────────
	// A subsequent call is a no-op.
	if err := migrations.Apply(ctx, p); err != nil {
		log.Fatalf("apply (re-run): %v", err)
	}
	fmt.Println("\n✓ Re-run is a no-op — data and schema intact.")
}
