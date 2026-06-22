package main

import (
	"fmt"
	"log"
	"math"
	"sort"

	"github.com/asaidimu/go-anansi/v7/core/data"
)

func main() {
	data.ConfigureDocumentFactory(data.DocumentFactoryConfig{}, nil)

	// 1. Build a document using the fluent builder
	db := data.NewDocumentBuilder()
	var err error

	db.Set("username", "alice").
		Set("email", "alice@example.com").
		Set("age", 30)

	db, err = db.SetNested("profile.bio", "Software engineer")
	if err != nil {
		log.Fatal(err)
	}

	db, err = db.SetNested("profile.skills", []string{"Go", "TypeScript", "Kubernetes"})
	if err != nil {
		log.Fatal(err)
	}

	db, err = db.SetNested("address.street", "123 Main St")
	if err != nil {
		log.Fatal(err)
	}

	db, err = db.SetNested("address.city", "Berlin")
	if err != nil {
		log.Fatal(err)
	}

	db.WithMetadata(map[string]any{ // problematic
		"customTag": "premium-user",
	})

	doc := db.Build()

	fmt.Println("Original document:")
	pretty, _ := doc.ToJSON(true)
	fmt.Println(string(pretty))

	// 2. Use Must helpers for quick access (panics on error – great for examples/scripts)
	fmt.Println("\nQuick access with Must:")
	fmt.Println("Username:", doc.Must().GetString("username"))
	fmt.Println("Age:", doc.Must().GetInt("age"))
	fmt.Println("First skill:", doc.Must().GetStringArray("profile.skills")[0])

	// 3. Nested access with safe methods
	bio, err := doc.GetString("profile.bio")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\nBio:", bio)

	// 6. In-memory query on a set of documents
	// Build three documents correctly handling SetNested errors
	buildUser := func(username, email string, age int, bio string) *data.Document {
		db := data.NewDocumentBuilder().
			Set("username", username).
			Set("email", email).
			Set("age", age)

		db, err = db.SetNested("profile.bio", bio)
		if err != nil {
			log.Fatal(err)
		}

		return db.Build()
	}

	docs := data.DocumentSet{
		doc,
		buildUser("bob", "bob@example.com", 25, "Designer"),
		buildUser("carol", "carol@example.com", 35, "Product manager"),
	}

	// Fluent query: users older than 28
	results := data.Query(docs).
		WhereFunc(func(d *data.Document) bool {
			age, _ := d.GetInt("age")
			return age > 28
		}).
		SortBy("age"). // descending
		Execute()

	fmt.Println("\nUsers older than 28 (sorted by age desc):")
	for _, r := range results {
		fmt.Printf("- %s (age %d)\n", r.Must().GetString("username"), r.Must().GetInt("age"))
	}

	// 7. Manual aggregation on ages (since AggregateNumeric does not exist in the provided code)
	var ages []float64
	for _, d := range docs {
		age, _ := d.GetFloat64("age")
		ages = append(ages, age)
	}

	sort.Float64s(ages)
	count := len(ages)
	sum := 0.0
	for _, a := range ages {
		sum += a
	}
	avg := sum / float64(count)
	min := ages[0]
	max := ages[count-1]
	var median float64
	if count%2 == 0 {
		median = (ages[count/2-1] + ages[count/2]) / 2
	} else {
		median = ages[count/2]
	}

	// Standard deviation
	var variance float64
	for _, a := range ages {
		diff := a - avg
		variance += diff * diff
	}
	stdDev := variance / float64(count)
	if count > 1 {
		stdDev = stdDev / float64(count-1) // sample std dev
	}
	stdDev = math.Sqrt(stdDev) // assuming math.Sqrt is imported or use data's utils if available

	fmt.Println("\nAge statistics (manual):")
	fmt.Printf("Count: %d\nSum: %.1f\nAverage: %.2f\nMin: %.0f\nMax: %.0f\nMedian: %.2f\nStdDev: %.3f\n",
		count, sum, avg, min, max, median, stdDev)

	// 8. JSONPath query – find all usernames
	usernames, err := docs[0].JSONPathQuery("$.username")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\nJSONPath $.username result:", usernames)

	skills, _ := doc.JSONPathQuery("$.profile.skills[*]")
	fmt.Println("Skills:", skills)

	// 9. Bind to struct
	type UserProfile struct {
		Username string   `doc:"username"`
		Email    string   `doc:"email"`
		Age      int      `doc:"age"`
		Bio      string   `doc:"profile.bio"`
		Skills   []string `doc:"profile.skills"`
		City     string   `doc:"address.city"`
	}

	var profile UserProfile
	if err := doc.BindTo(&profile); err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nBound to struct:")
	fmt.Printf("%+v\n", profile)

	// 10. Normalization (strips nested metadata – useful before persistence)
	clean := doc.Normalize()
	fmt.Println("\nNormalized (nested metadata stripped):")
	pretty, _ = clean.ToJSON(true)
	fmt.Println(string(pretty))
}
