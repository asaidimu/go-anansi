# Core Operations

# Core Operations with Anansi Collections

Once you have initialized the `persistence.Persistence` service and created a collection, you can perform fundamental data operations (CRUD: Create, Read, Update, Delete) using the `persistence.PersistenceCollectionInterface`.

First, obtain your collection instance:

```go
import (
	"log"

	"github.com/asaidimu/go-anansi/core/persistence"
	"github.com/asaidimu/go-anansi/core/schema"
)

// Assuming persistenceSvc is your initialized *persistence.Persistence
// and userSchema is your schema.SchemaDefinition for the 'users' collection

collection, err := persistenceSvc.Collection("users")
if err != nil {
	log.Fatalf("Failed to get collection 'users': %v", err)
}
fmt.Println("Obtained 'users' collection instance.")
```

## Create (Insert) Documents

The `Create` method allows you to insert single or multiple documents (records) into a collection. Documents are represented as `map[string]any`.

```go
// Insert a single user document
userData := map[string]any{
	"name":      "Alice Smith",
	"email":     "alice@example.com",
	"age":       30,
	"is_active": true,
}

fmt.Println("Inserting single user...")
insertedResult, err := collection.Create(userData)
if err != nil {
	log.Fatalf("Failed to insert Alice: %v", err)
}
// insertedResult.Data will be schema.Document (map[string]any) for single insert, 
// or []schema.Document for batch inserts.
fmt.Printf("Successfully inserted Alice. ID: %v\n", insertedResult.(*query.QueryResult).Data.(schema.Document)["id"])

// Insert multiple user documents (batch insert)
batchUsers := []map[string]any{
	{
		"name":      "Bob Johnson",
		"email":     "bob@example.com",
		"age":       25,
		"is_active": true,
	},
	{
		"name":      "Charlie Brown",
		"email":     "charlie@example.com",
		"age":       35,
		"is_active": false,
	},
}

fmt.Println("Inserting multiple users...")
insertedBatchResult, err := collection.Create(batchUsers)
if err != nil {
	log.Fatalf("Failed to insert batch users: %v", err)
}
fmt.Printf("Successfully inserted %d users in batch.\n", insertedBatchResult.(*query.QueryResult).Count)
```

## Read (Query) Documents

To retrieve documents, use the `Read` method with a `query.QueryDSL` object. The `query.QueryBuilder` provides a fluent API to construct complex queries.

```go
import "github.com/asaidimu/go-anansi/core/query"

// Basic query: Read all documents from the 'users' collection
fmt.Println("Reading all users...")
allUsersQuery := query.NewQueryBuilder().Build()
result, err := collection.Read(&allUsersQuery) // Read expects a pointer to QueryDSL
if err != nil {
	log.Fatalf("Failed to read all users: %v", err)
}

fmt.Printf("Found %d users:\n", result.Count)
for _, user := range result.Data.([]schema.Document) {
	fmt.Printf("  ID: %v, Name: %v, Email: %v, Age: %v, Active: %v\n",
		user["id"], user["name"], user["email"], user["age"], user["is_active"])
}

// Filtered query: Read active users older than 28
fmt.Println("Reading active users older than 28...")
filteredQuery := query.NewQueryBuilder().
	WhereGroup(query.LogicalOperatorAnd). // Start a logical AND group
		Where("is_active").Eq(true).  // Condition 1: is_active = true
		Where("age").Gt(28).         // Condition 2: age > 28
	End(). // End the logical group
	Build()

filteredResult, err := collection.Read(&filteredQuery)
if err != nil {
	log.Fatalf("Failed to read filtered users: %v", err)
}

fmt.Printf("Found %d active users older than 28:\n", filteredResult.Count)
for _, user := range filteredResult.Data.([]schema.Document) {
	fmt.Printf("  ID: %v, Name: %v, Email: %v, Age: %v, Active: %v\n",
		user["id"], user["name"], user["email"], user["age"], user["is_active"])
}
```

## Update Documents

To modify existing documents, use the `Update` method. You provide a `map[string]any` with the fields to update and a `query.QueryFilter` to specify which documents to modify.

```go
import "github.com/asaidimu/go-anansi/core/persistence"

// Update user 'Alice Smith' to age 31 and name 'Alice M. Smith'
updates := map[string]any{
	"age":  31,
	"name": "Alice M. Smith",
}

filterByEmail := query.NewQueryBuilder().Where("email").Eq("alice@example.com").Build().Filters

updateParams := &persistence.CollectionUpdate{
	Data:   updates,
	Filter: filterByEmail,
}

fmt.Println("Updating Alice's profile...")
rowsAffected, err := collection.Update(updateParams)
if err != nil {
	log.Fatalf("Failed to update user: %v", err)
}
fmt.Printf("Updated %d rows.\n", rowsAffected)

// Verify the update
verifyQuery := query.NewQueryBuilder().Where("email").Eq("alice@example.com").Build()
verifiedResult, err := collection.Read(&verifyQuery)
if err != nil {
	log.Fatalf("Failed to verify update: %v", err)
}
if verifiedResult.Count > 0 {
	updatedUser := verifiedResult.Data.([]schema.Document)[0]
	fmt.Printf("Verified update: Name: %v, Age: %v\n", updatedUser["name"], updatedUser["age"])
}
```

## Delete Documents

The `Delete` method removes documents from a collection based on a `query.QueryFilter`. For safety, a filter is required by default. To delete all records (use with extreme caution!), you must set the `unsafe` parameter to `true`.

```go
// Delete inactive users
filterInactive := query.NewQueryBuilder().Where("is_active").Eq(false).Build().Filters

fmt.Println("Deleting inactive users...")
rowsAffected, err = collection.Delete(filterInactive, false) // 'false' ensures a filter is required
if err != nil {
	log.Fatalf("Failed to delete inactive users: %v", err)
}
fmt.Printf("Deleted %d inactive users.\n", rowsAffected)

// WARNING: Deleting all records (UNSAFE OPERATION)
// fmt.Println("WARNING: Deleting all remaining users (unsafe operation)...")
// allUsersFilter := query.NewQueryBuilder().Build().Filters // An empty filter usually means all records
// rowsAffected, err = collection.Delete(allUsersFilter, true) // 'true' allows deletion without a specific filter
// if err != nil {
// 	log.Fatalf("Failed to delete all users: %v", err)
// }
// fmt.Printf("Deleted %d remaining users (unsafe).\n", rowsAffected)
```

---
### 🤖 AI Agent Guidance

```json
{
  "decisionPoints": [
    "IF operation_is_create THEN validate_data_against_schema ELSE proceed_to_insert",
    "IF operation_is_read THEN construct_query_dsl_based_on_requirements ELSE fetch_all_documents",
    "IF operation_is_update THEN ensure_update_data_and_filter_are_provided ELSE throw_error",
    "IF operation_is_delete AND filter_is_empty AND unsafe_is_false THEN reject_delete_operation ELSE proceed_with_delete"
  ],
  "verificationSteps": [
    "Check: `result.Count` after `Read` to confirm number of documents retrieved matches expectation",
    "Check: `rowsAffected` after `Update` or `Delete` to confirm the correct number of records were modified/removed",
    "Check: `insertedResult.Data` or `insertedBatchResult.Count` after `Create` for successful insertion details",
    "Check: `err == nil` after every database operation to confirm success"
  ],
  "quickPatterns": [
    "Pattern: Create Single Document\n```go\nuser := map[string]any{\"name\": \"Test\", \"email\": \"test@example.com\"}\nresult, err := collection.Create(user)\n// Handle result and error\n```",
    "Pattern: Read All Documents\n```go\nq := query.NewQueryBuilder().Build()\nresult, err := collection.Read(&q)\n// Process result.Data.([]schema.Document)\n```",
    "Pattern: Read with Filter\n```go\nq := query.NewQueryBuilder().Where(\"field\").Eq(\"value\").Build()\nresult, err := collection.Read(&q)\n```",
    "Pattern: Update Documents\n```go\nupdates := map[string]any{\"field\": \"new_value\"}\nfilter := query.NewQueryBuilder().Where(\"id\").Eq(1).Build().Filters\nupdateParams := &persistence.CollectionUpdate{Data: updates, Filter: filter}\nrowsAffected, err := collection.Update(updateParams)\n```",
    "Pattern: Delete Documents with Filter\n```go\nfilter := query.NewQueryBuilder().Where(\"status\").Eq(\"inactive\").Build().Filters\nrowsAffected, err := collection.Delete(filter, false)\n```"
  ],
  "diagnosticPaths": [
    "Error: InvalidDataTypeForCreate -> Symptom: `invalid data type for Create` -> Check: Input to `Create` must be `map[string]any` or `[]map[string]any` -> Fix: Convert input data to the correct type.",
    "Error: DataValidationFailed -> Symptom: `Provided data does not conform to the collections schema` -> Check: Review `ValidationResult.Issues` for specific schema violations (missing required fields, type mismatches, constraint violations) -> Fix: Correct data to match schema.",
    "Error: QueryExecutionFailed -> Symptom: `failed to read data from collection` / `failed to insert data` / `failed to update data` / `failed to delete data` -> Check: Review SQL query generated (if logged), parameters, database connection, database logs for specific errors (e.g., syntax, constraint violation) -> Fix: Adjust QueryDSL, ensure data consistency, verify database health.",
    "Error: UnsafeDeleteAttempt -> Symptom: `DELETE without WHERE clause is not allowed for safety` -> Check: `Delete` method called without a filter or with `unsafeDelete` set to `false` -> Fix: Provide a filter or explicitly set `unsafeDelete` to `true` (use with caution)."
  ]
}
```

---
*Generated using Gemini AI on 6/28/2025, 10:32:05 PM. Review and refine as needed.*