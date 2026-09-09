package main

import (
	"context"
	"fmt"
	"log"

	"github.com/asaidimu/go-anansi/v8"
	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// NOTE: run with the FTS5 build tag, otherwise creating the fulltext index
// fails with "no such module: fts5":
//
//	go run -tags sqlite_fts5 ./example/fts

func articlesSchema() *definition.Schema {
	return &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name: "Articles",
			Fields: map[definition.FieldId]definition.Field{
				"019fb22d-a9a1-7e60-8924-8f09eb81a0b1": {Name: "id", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"019fb22d-a9a1-7e61-8924-8f09eb81a0b2": {Name: "title", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"019fb22d-a9a1-7e62-8924-8f09eb81a0b3": {Name: "body", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
			Indexes: map[definition.IndexID]definition.Index{
				// Index IDs must be UUIDv7 in production mode, just like field IDs.
				"019fb22d-a9a1-7e63-8924-8f09eb81a0c1": {
					Name:   "articles_fts",
					Type:   definition.IndexTypeFullText,
					Fields: []definition.FieldName{"title", "body"},
				},
			},
		},
		Version: common.MustNewVersion("1.0.0"),
	}
}

func printResults(title string, docs data.DocumentSet) {
	fmt.Printf("\n--- %s (%d rows) ---\n", title, len(docs))
	for _, d := range docs {
		m := d.ToMap()
		fmt.Printf("  id=%v title=%q\n", m["id"], m["title"])
	}
}

func must(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %v", msg, err)
	}
}

func main() {
	ctx := context.Background()

	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		DBPath:  ":memory:",
		Schemas: []*definition.Schema{articlesSchema()},
	})
	must(err, "playground setup")
	defer cleanup()

	articles, err := p.Collection(ctx, "Articles")
	must(err, "get Articles collection")

	_, err = articles.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "a1", "title": "Introduction to Go", "body": "Go is a statically typed, compiled programming language designed at Google."}),
		data.MustNewDocument(map[string]any{"id": "a2", "title": "Database design fundamentals", "body": "Relational databases organize data into tables with rows and columns."}),
		data.MustNewDocument(map[string]any{"id": "a3", "title": "Full-text search with SQLite FTS5", "body": "SQLite FTS5 provides fast tokenized search over TEXT columns via MATCH queries."}),
	})
	must(err, "seed articles")

	// -----------------------------------------------------------------
	// 1. Contains: prefix match — "SQLite" matches a3's title.
	// -----------------------------------------------------------------
	fmt.Println("\n=== 1. Contains (prefix match) ===")
	q := query.NewQueryBuilder().From("Articles").TextSearch("title").Contains("SQLite").Build()
	res, err := articles.Read(ctx, &q)
	must(err, "contains search")
	printResults(`Title CONTAINS "SQLite"`, res.Data)

	// -----------------------------------------------------------------
	// 2. Field scoping: the same term scoped to title vs body.
	//    "tokenized" appears only in a3's body.
	// -----------------------------------------------------------------
	fmt.Println("\n=== 2. Field scoping ===")
	qTitle := query.NewQueryBuilder().From("Articles").TextSearch("title").Contains("tokenized").Build()
	res, err = articles.Read(ctx, &qTitle)
	must(err, "title-scoped search")
	fmt.Printf("\nTitle CONTAINS \"tokenized\": %d rows (term lives in body -> no match)\n", res.Count)

	qBody := query.NewQueryBuilder().From("Articles").TextSearch("body").Contains("tokenized").Build()
	res, err = articles.Read(ctx, &qBody)
	must(err, "body-scoped search")
	printResults(`Body CONTAINS "tokenized"`, res.Data)

	// -----------------------------------------------------------------
	// 3. Phrase: contiguous match only. Both a2's title ("Database design
	//    fundamentals") and ... only the contiguous occurrence matches.
	// -----------------------------------------------------------------
	fmt.Println("\n=== 3. Phrase (contiguous match) ===")
	_, err = articles.CreateOne(ctx, data.MustNewDocument(map[string]any{
		"id": "a4", "title": "Design notes", "body": "design database fundamentals reversed",
	}))
	must(err, "seed a4")
	qPhrase := query.NewQueryBuilder().From("Articles").TextSearch("title").Phrase("database design").Build()
	res, err = articles.Read(ctx, &qPhrase)
	must(err, "phrase search")
	printResults(`Title PHRASE "database design"`, res.Data)

	// -----------------------------------------------------------------
	// 4. Exact: the whole query must appear as a single token.
	// -----------------------------------------------------------------
	fmt.Println("\n=== 4. Exact (single-token match) ===")
	qExact := query.NewQueryBuilder().From("Articles").TextSearch("title").Exact("Go").Build()
	res, err = articles.Read(ctx, &qExact)
	must(err, "exact search")
	printResults(`Title EXACT "Go"`, res.Data)

	// -----------------------------------------------------------------
	// 5. Composing FTS with a regular filter (AND).
	// -----------------------------------------------------------------
	fmt.Println("\n=== 5. FTS + regular filter ===")
	qBoth := query.NewQueryBuilder().From("Articles").
		TextSearch("title").Contains("design").
		Where("id").Eq("a2").Build()
	res, err = articles.Read(ctx, &qBoth)
	must(err, "combined search")
	printResults(`Title CONTAINS "design" AND id = "a2"`, res.Data)

	// -----------------------------------------------------------------
	// 6. The index stays in sync: updates re-index, deletes un-index.
	// -----------------------------------------------------------------
	fmt.Println("\n=== 6. Index maintenance ===")
	filter := query.NewQueryBuilder().From("Articles").Where("id").Eq("a1").Build()
	_, err = articles.Update(ctx, &base.CollectionUpdate{
		Filter: filter.Filters,
		Set:    data.MustNewDocument(map[string]any{"title": "Introduction to Go programming"}),
	})
	must(err, "update a1")
	qAfter := query.NewQueryBuilder().From("Articles").TextSearch("title").Contains("programming").Build()
	res, err = articles.Read(ctx, &qAfter)
	must(err, "search after update")
	fmt.Printf("\nAfter retitling a1, Title CONTAINS \"programming\": %d rows (update re-indexed)\n", res.Count)

	delFilter := query.NewQueryBuilder().From("Articles").Where("id").Eq("a3").Build()
	_, err = articles.Delete(ctx, delFilter.Filters, false)
	must(err, "delete a3")
	qGone := query.NewQueryBuilder().From("Articles").TextSearch("title").Contains("SQLite").Build()
	res, err = articles.Read(ctx, &qGone)
	must(err, "search after delete")
	fmt.Printf("After deleting a3, Title CONTAINS \"SQLite\": %d rows (delete un-indexed)\n", res.Count)

	// -----------------------------------------------------------------
	// 7. Relevance ranking: a pure text-search read with no explicit sort
	//    returns best-match-first (bm25), not insertion order.
	// -----------------------------------------------------------------
	fmt.Println("\n=== 7. Relevance ranking (bm25) ===")
	_, err = articles.CreateMany(ctx, []data.Documenter{
		data.MustNewDocument(map[string]any{"id": "g1", "title": "Campfire cooking", "body": "database recipes for campfires"}),
		data.MustNewDocument(map[string]any{"id": "g2", "title": "Kart racing", "body": "go fast, turn left"}),
		// Best match inserted LAST: matches both terms, repeatedly.
		data.MustNewDocument(map[string]any{"id": "g3", "title": "Driver guide", "body": "go database internals and more go database examples"}),
	})
	must(err, "seed ranking docs")
	qRank := query.NewQueryBuilder().From("Articles").TextSearch("body").Contains("go database").Build()
	res, err = articles.Read(ctx, &qRank)
	must(err, "ranked search")
	printResults(`Body CONTAINS "go database" (bm25 ranked)`, res.Data)

	fmt.Println("\nDone.")
}
