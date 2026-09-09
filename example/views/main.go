package main

import (
	"context"
	"fmt"
	"log"

	"github.com/asaidimu/go-anansi/v8"
	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

func usersSchema() *definition.Schema {
	return &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "Users",
			Fields: map[definition.FieldId]definition.Field{
				"019fb22d-a9a1-7e3e-8924-8f09eb81a096": {Name: "id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"019fb22d-a9a1-7e3f-8e2c-3427aaf6b775": {Name: "name", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"019fb22d-a9a1-7e40-b7e2-8ca46b4c788a": {Name: "email", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"019fb22d-a9a1-7e41-b7e2-8ca46b4c788b": {Name: "active", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeBoolean}},
			},
		},
		Version: common.MustNewVersion("1.0.0"),
	}
}

func ordersSchema() *definition.Schema {
	return &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "Orders",
			Fields: map[definition.FieldId]definition.Field{
				"019fb22d-a9a1-7e42-8924-8f09eb81a097": {Name: "id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"019fb22d-a9a1-7e43-8924-8f09eb81a098": {Name: "user_id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"019fb22d-a9a1-7e44-8924-8f09eb81a099": {Name: "total", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeNumber}},
				"019fb22d-a9a1-7e45-8924-8f09eb81a09a": {Name: "status", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
		Version: common.MustNewVersion("1.0.0"),
	}
}

func printResults(title string, docs data.DocumentSet) {
	fmt.Printf("\n--- %s (%d rows) ---\n", title, len(docs))
	for _, d := range docs {
		fmt.Printf("  %v\n", d.ToMap())
	}
}

func must(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %v", msg, err)
	}
}

func main() {
	ctx := context.Background()
	users := usersSchema()
	orders := ordersSchema()

	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		DBPath:  ":memory:",
		Schemas: []*definition.Schema{users, orders},
	})
	must(err, "playground setup")
	defer cleanup()

	usersColl, err := p.Collection(ctx, "Users")
	must(err, "get Users collection")
	ordersColl, err := p.Collection(ctx, "Orders")
	must(err, "get Orders collection")

	// Seed data: 3 users (2 active), 3 orders.
	_, err = usersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "u1", "name": "Alice", "email": "alice@example.com", "active": true}),
		data.MustNewDocument(map[string]any{"id": "u2", "name": "Bob", "email": "bob@example.com", "active": true}),
		data.MustNewDocument(map[string]any{"id": "u3", "name": "Cara", "email": "cara@example.com", "active": false}),
	})
	must(err, "seed users")
	_, err = ordersColl.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "o1", "user_id": "u1", "total": 250.0, "status": "paid"}),
		data.MustNewDocument(map[string]any{"id": "o2", "user_id": "u2", "total": 20.0, "status": "paid"}),
		data.MustNewDocument(map[string]any{"id": "o3", "user_id": "u3", "total": 500.0, "status": "paid"}),
	})
	must(err, "seed orders")

	// -----------------------------------------------------------------
	// 1. VIRTUAL VIEW: ActiveUsers = Users WHERE active=true, projected
	//    to name+email. No table is created; every Read re-runs the
	//    stored query composed with the caller's query.
	// -----------------------------------------------------------------
	fmt.Println("\n=== 1. Virtual view ===")
	activeUsersQuery := query.NewQueryBuilder().
		From("Users").
		Where("active").Eq(true).
		Select().Include("name", "email").End().
		Build()
	activeUsersQuery.Target = &query.QueryTarget{Name: "Users", Schema: users}

	_, err = p.CreateView(ctx, "ActiveUsers", &activeUsersQuery, false)
	must(err, "create virtual view ActiveUsers")

	view, err := p.Collection(ctx, "ActiveUsers")
	must(err, "get ActiveUsers view")

	empty := query.NewQueryBuilder().Build()
	res, err := view.Read(ctx, &empty)
	must(err, "read ActiveUsers")
	printResults("ActiveUsers (virtual, live)", res.Data)
	// Expect 2 rows: Alice + Bob, with only name+email projected.

	// Composition: caller's filter is AND-merged with the view's filter.
	aliceOnly := query.NewQueryBuilder().Where("name").Eq("Alice").Build()
	res, err = view.Read(ctx, &aliceOnly)
	must(err, "read ActiveUsers with user filter")
	printResults("ActiveUsers WHERE name=Alice (view filter AND user filter)", res.Data)
	// Expect 1 row.

	// Views are read-only.
	_, err = view.CreateOne(ctx, data.MustNewDocument(map[string]any{
		"id": "x1", "name": "X", "email": "x@x.com", "active": true,
	}))
	fmt.Printf("\nWrite to virtual view -> err: %v (expect ERR_PERSISTENCE_READ_ONLY)\n", err)

	// Virtual views show new data immediately (no refresh).
	_, err = usersColl.CreateOne(ctx, data.MustNewDocument(map[string]any{
		"id": "u4", "name": "Dan", "email": "dan@example.com", "active": true,
	}))
	must(err, "insert Dan")
	res, err = view.Read(ctx, &empty)
	must(err, "re-read ActiveUsers")
	fmt.Printf("\nAfter inserting active Dan, ActiveUsers count=%d (virtual = always fresh)\n", res.Count)

	// -----------------------------------------------------------------
	// 2. MATERIALIZED VIEW: BigOrders = Users JOIN Orders ON
	//    Orders.user_id = Users.id WHERE Orders.total > 100.
	//    Backed by a physical table (CREATE TABLE AS SELECT).
	//    Reads hit the snapshot; Refresh() re-runs the SELECT.
	// -----------------------------------------------------------------
	fmt.Println("\n=== 2. Materialized view ===")
	bigOrdersQuery := query.NewQueryBuilder().
		From("Users").
		InnerJoin("Orders").End().
		Where("Orders.total").Gt(100.0).
		Build()
	bigOrdersQuery.Joins[0].On = &query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "Orders.user_id",
			Operator: query.ComparisonOperatorEq,
			Value:    query.FilterValue{FieldRefVal: &query.FieldReference{Field: "Users.id"}},
		},
	}
	bigOrdersQuery.Target = &query.QueryTarget{Name: "Users", Schema: users}
	bigOrdersQuery.Joins[0].Target = query.QueryTarget{Name: "Orders", Schema: orders}

	_, err = p.CreateView(ctx, "BigOrders", &bigOrdersQuery, true)
	must(err, "create materialized view BigOrders")

	mv, err := p.Collection(ctx, "BigOrders")
	must(err, "get BigOrders view")

	res, err = mv.Read(ctx, &empty)
	must(err, "read BigOrders")
	printResults("BigOrders (materialized snapshot)", res.Data)
	// Expect 2 rows: Alice/o1 (250) + Cara/o3 (500).

	// Insert a new big order -> snapshot is stale until Refresh().
	_, err = ordersColl.CreateOne(ctx, data.MustNewDocument(map[string]any{
		"id": "o4", "user_id": "u2", "total": 999.0, "status": "paid",
	}))
	must(err, "insert o4")
	res, err = mv.Read(ctx, &empty)
	must(err, "re-read BigOrders before refresh")
	fmt.Printf("\nBefore Refresh, BigOrders count=%d (stale snapshot)\n", res.Count)

	must(mv.Refresh(ctx), "refresh BigOrders")
	res, err = mv.Read(ctx, &empty)
	must(err, "re-read BigOrders after refresh")
	fmt.Printf("After Refresh, BigOrders count=%d (snapshot rebuilt)\n", res.Count)
	printResults("BigOrders after Refresh", res.Data)

	// Refresh on a virtual view fails.
	err = view.Refresh(ctx)
	fmt.Printf("\nRefresh(virtual ActiveUsers) -> err: %v (expect ERR_PERSISTENCE_NOT_MATERIALIZED)\n", err)

	fmt.Println("\nDone.")
}
