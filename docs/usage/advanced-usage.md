# Advanced Usage

# Advanced Usage of Anansi

This section delves into more complex scenarios and powerful features of Anansi, including advanced QueryDSL capabilities, customization, and optimization.

## Advanced Querying with QueryDSL

The `query.QueryBuilder` offers a comprehensive set of methods to construct sophisticated queries beyond basic filters.

### Sorting and Pagination

```go
import "github.com/asaidimu/go-anansi/core/query"
import "github.com/asaidimu/go-anansi/core/schema"

// Query users, ordered by age descending, then by name ascending.
// Limit to 5 results, starting from the 10th record (offset 9).
advancedQuery := query.NewQueryBuilder().
	OrderByDesc("age").
	OrderByAsc("name").
	Limit(5).Offset(9).
	Build()

result, err := collection.Read(&advancedQuery)
if err != nil {
	log.Fatalf("Failed advanced query: %v", err)
}
fmt.Printf("Found %d users with advanced sorting and pagination.\n", result.Count)
for _, user := range result.Data.([]schema.Document) {
	fmt.Printf("  Name: %v, Age: %v\n", user["name"], user["age"])
}
```

### Field Projections (Include/Exclude)

Control which fields are returned from your queries.

```go
// Select only 'name' and 'email' fields
projectedQuery := query.NewQueryBuilder().
	Select().Include("name", "email").End().
	Build()

result, err = collection.Read(&projectedQuery)
if err != nil {
	log.Fatalf("Failed projected query: %v", err)
}
fmt.Println("Projected Results (Name, Email only):")
for _, user := range result.Data.([]schema.Document) {
	fmt.Printf("  Name: %v, Email: %v\n", user["name"], user["email"])
}

// Exclude 'age' and 'is_active' fields
excludedQuery := query.NewQueryBuilder().
	Select().Exclude("age", "is_active").End().
	Build()

result, err = collection.Read(&excludedQuery)
if err != nil {
	log.Fatalf("Failed excluded query: %v", err)
}
fmt.Println("Excluded Results (Age, IsActive excluded):")
for _, user := range result.Data.([]schema.Document) {
	fmt.Printf("  ID: %v, Name: %v, Email: %v\n", user["id"], user["name"], user["email"])
}
```

### Querying Nested JSON Fields (SQLite `json_extract`)

If your schema includes `FieldTypeObject` or `FieldTypeRecord` fields, Anansi's SQLite adapter can query nested properties using dot notation, which translates to `json_extract`.

First, define a schema with a nested object:

```json
const orderSchemaJSON = `{
	"name": "orders",
	"version": "1.0.0",
	"fields": {
		"order_id": {"name": "order_id", "type": "string", "unique": true, "required": true},
		"customer_info": {
			"name": "customer_info",
			"type": "object",
			"schema": {
				"id": "customer_details",
				"name": "customer_details",
				"fields": {
					"name": {"name": "name", "type": "string"},
					"email": {"name": "email", "type": "string"},
					"address": {"name": "address", "type": "object", "schema": {
						"id": "address_details",
						"name": "address_details",
						"fields": {
							"city": {"name": "city", "type": "string"},
							"zip": {"name": "zip", "type": "string"}
						}
					}}
				}
			}
		}
	},
	"indexes": [
		{
			"name": "pk_order_id",
			"fields": ["order_id"],
			"type": "primary"
		}
	]
}`

// ... (initialize persistenceSvc, create 'orders' collection)

// Insert sample order data
orderData := map[string]any{
	"order_id": "ORD-001",
	"customer_info": map[string]any{
		"name": "Jane Doe",
		"email": "jane@example.com",
		"address": map[string]any{
			"city": "New York",
			"zip": "10001",
		},
	},
}
_, err = ordersCollection.Create(orderData)
if err != nil {
	log.Fatalf("Failed to create order: %v", err)
}

// Query by nested field (customer_info.address.city)
queryNested := query.NewQueryBuilder().
	Where("customer_info.address.city").Eq("New York").
	Build()

resultNested, err := ordersCollection.Read(&queryNested)
if err != nil {
	log.Fatalf("Failed to query nested field: %v", err)
}

fmt.Println("Orders from New York:")
for _, order := range resultNested.Data.([]schema.Document) {
	fmt.Printf("  Order ID: %v, Customer Name: %v\n", order["order_id"],
		order["customer_info"].(map[string]any)["name"])
}
```

## Customization: In-memory Go Functions (Computed Fields & Custom Filters)

Anansi allows you to extend query capabilities by registering custom Go functions. These functions operate on data *after* it's retrieved from the database, making them suitable for complex logic or processing JSON fields that the database might not efficiently query natively. You register these functions via the `schema.FunctionMap` when initializing `persistence.NewPersistence`.

### Computed Fields

Define new fields dynamically by applying Go logic to retrieved data. These fields are not stored in the database but are computed on-the-fly.

```go
package main

import (
	"fmt"
	"log"
	"github.com/asaidimu/go-anansi/core/query"
	"github.com/asaidimu/go-anansi/core/schema"
)

// Define a custom Go function to concatenate name and email
func concatenateNameEmail(row schema.Document, args query.FilterValue) (any, error) {
	name, nameOk := row["name"].(string)
	email, emailOk := row["email"].(string)
	if nameOk && emailOk {
		return fmt.Sprintf("%s <%s>", name, email), nil
	}
	return "", fmt.Errorf("name or email not found or invalid type")
}

func main() {
	// ... (database and persistenceSvc initialization)

	// 1. Prepare the FunctionMap with your custom ComputeFunction
	customFunctions := schema.FunctionMap{
		"full_contact_info": query.ComputeFunction(concatenateNameEmail),
	}

	// Re-initialize persistence with the custom functions
	// (In a real app, you'd register functions *before* initial persistence creation)
	persistenceSvcWithFuncs, err := persistence.NewPersistence(interactor, customFunctions)
	if err != nil {
		log.Fatalf("Failed to re-initialize persistence with functions: %v", err)
	}
	collection, err := persistenceSvcWithFuncs.Collection("users")
	if err != nil {
		log.Fatalf("Failed to get users collection: %v", err)
	}

	// Ensure some data exists
	collection.Create(map[string]any{"name": "Alice", "email": "alice@example.com", "age": 30, "is_active": true})

	// 2. Use AddComputed in your QueryBuilder
	queryWithComputed := query.NewQueryBuilder().
		Select().
			Include("id", "name", "email"). // Ensure base fields are selected from DB
			AddComputed("contact", "full_contact_info"). // 'contact' is the new alias
		End().
		Build()

	result, err := collection.Read(&queryWithComputed)
	if err != nil {
		log.Fatalf("Failed to query with computed field: %v", err)
	}

	fmt.Println("Users with Computed Field 'contact':")
	for _, user := range result.Data.([]schema.Document) {
		fmt.Printf("  ID: %v, Name: %v, Email: %v, Contact: %v\n",
			user["id"], user["name"], user["email"], user["contact"])
	}
}
```

### Custom Filters

Implement complex, non-SQL-standard filtering logic in Go. This is useful for filtering based on computed values or logic that is hard to express in SQL.

```go
// Define a custom Go predicate function for filtering
func isAdultUser(doc schema.Document, field string, args query.FilterValue) (bool, error) {
	// The 'field' argument is typically not used for schema-level predicates like this.
	// 'args' can be used for parameters to the predicate (e.g., minimum_age).
	minAge, ok := args.(int)
	if !ok { minAge = 18 } // Default to 18 if no valid arg

	age, ageOk := doc["age"].(int64) // Assuming age is int64 from DB
	if !ageOk {
		return false, fmt.Errorf("age field not found or invalid type for predicate")
	}
	return age >= int64(minAge), nil
}

func main() {
	// ... (database and persistenceSvc initialization)

	// 1. Prepare the FunctionMap with your custom PredicateFunction
	customFunctions := schema.FunctionMap{
		"is_adult": query.PredicateFunction(isAdultUser),
	}
	// Re-initialize persistence with the custom functions
	persistenceSvcWithFuncs, err := persistence.NewPersistence(interactor, customFunctions)
	if err != nil {
		log.Fatalf("Failed to re-initialize persistence with functions: %v", err)
	}
	collection, err := persistenceSvcWithFuncs.Collection("users")
	if err != nil {
		log.Fatalf("Failed to get users collection: %v", err)
	}

	// Ensure some data exists
	collection.Create(map[string]any{"name": "Child User", "email": "child@example.com", "age": 15, "is_active": true})
	collection.Create(map[string]any{"name": "Adult User", "email": "adult@example.com", "age": 25, "is_active": true})

	// 2. Use Custom in your QueryBuilder with your registered operator
	queryWithCustomFilter := query.NewQueryBuilder().
		Where("age").Custom(query.ComparisonOperator("is_adult"), 18). // Check if user is an adult >= 18
		Build()

	result, err := collection.Read(&queryWithCustomFilter)
	if err != nil {
		log.Fatalf("Failed to query with custom filter: %v", err)
	}

	fmt.Println("Adult Users:")
	for _, user := range result.Data.([]schema.Document) {
		fmt.Printf("  Name: %v, Age: %v\n", user["name"], user["age"])
	}
}
```

### Optimization: `persistence.InteractorOptions`

When initializing your `SQLiteInteractor`, you can provide `persistence.InteractorOptions` to control DDL generation.

```go
import "github.com/asaidimu/go-anansi/sqlite"
import "github.com/asaidimu/go-anansi/core/persistence"

// Custom options for table creation
customOptions := &persistence.InteractorOptions{
	IfNotExists:   false, // Do not add IF NOT EXISTS to CREATE TABLE
	DropIfExists:  true,  // Drop table if it exists before creating
	CreateIndexes: false, // Do not create indexes automatically with table
	TablePrefix:   "app_", // Prefix all table names (e.g., 'users' becomes 'app_users')
}

interactorWithCustomOpts := sqlite.NewSQLiteInteractor(db, nil, customOptions, nil)
// Then initialize persistence with this interactor:
// persistenceSvc, err := persistence.NewPersistence(interactorWithCustomOpts, schema.FunctionMap{})
```


---
### 🤖 AI Agent Guidance

```json
{
  "decisionPoints": [
    "IF need_complex_sorting THEN use_orderByAsc_or_orderByDesc ELSE use_default_order",
    "IF need_specific_fields THEN use_select_include ELSE use_select_exclude",
    "IF need_computed_fields THEN define_compute_function AND add_computed_projection ELSE no_computed_fields",
    "IF need_custom_filter_logic THEN define_predicate_function AND use_custom_comparison_operator ELSE use_standard_filters",
    "IF table_recreation_strategy_is_destructive THEN set_dropIfExists_true ELSE set_dropIfExists_false"
  ],
  "verificationSteps": [
    "Check: Query results match expected sorting order and pagination limits.",
    "Check: Query results contain only included fields or lack excluded fields.",
    "Check: Computed fields appear in results with correct values.",
    "Check: Custom filter correctly filters rows based on its logic.",
    "Check: `TablePrefix` is correctly applied to table names (inspect database schema)."
  ],
  "quickPatterns": [
    "Pattern: Sorting and Pagination\n```go\nq := query.NewQueryBuilder().OrderBy(\"field\", query.SortDirectionDesc).Limit(10).Offset(0).Build()\n```",
    "Pattern: Select specific fields\n```go\nq := query.NewQueryBuilder().Select().Include(\"field1\", \"field2\").End().Build()\n```",
    "Pattern: Exclude specific fields\n```go\nq := query.NewQueryBuilder().Select().Exclude(\"field_to_hide\").End().Build()\n```",
    "Pattern: Add a Computed Field\n```go\n// Assume 'myComputeFunc' is registered\nq := query.NewQueryBuilder().Select().AddComputed(\"new_field_alias\", \"myComputeFunc\").End().Build()\n```",
    "Pattern: Use a Custom Filter\n```go\n// Assume 'myCustomPredicate' is registered\nq := query.NewQueryBuilder().Where(\"field\").Custom(query.ComparisonOperator(\"myCustomPredicate\"), \"arg_value\").Build()\n```"
  ],
  "diagnosticPaths": [
    "Error: UnsupportedQueryFeature -> Symptom: `unsupported logical operator for Go evaluation` or `unsupported comparison operator for direct SQL` -> Check: Ensure the QueryDSL feature (Joins, Aggregations, specific complex filters) is fully implemented in the current `DatabaseInteractor` and `QueryGenerator` -> Fix: Adapt query to supported features or contribute implementation for the desired feature.",
    "Error: GoFunctionRegistrationMissing -> Symptom: `unregistered Go compute function` or `unregistered Go filter function` -> Check: Ensure the function name/operator is present in the `schema.FunctionMap` passed to `persistence.NewPersistence` -> Fix: Register the function before initializing persistence.",
    "Error: GoFunctionRuntimeError -> Symptom: Error from within your custom `ComputeFunction` or `PredicateFunction` -> Check: Input types for the Go function, logic within the function (e.g., type assertions) -> Fix: Debug the Go function's implementation, handle nil or unexpected types.",
    "Error: IncorrectTablePrefix -> Symptom: `no such table: my_table` when a prefix was expected or vice versa -> Check: `persistence.InteractorOptions.TablePrefix` during `NewSQLiteInteractor` initialization -> Fix: Adjust `TablePrefix` or ensure table name in schema matches expectations."
  ]
}
```

---
*Generated using Gemini AI on 6/28/2025, 10:32:05 PM. Review and refine as needed.*