# Task-Based Guide

# Task-Based Guide for Anansi

This guide provides practical steps for common development tasks using the Anansi framework.

## 1. Defining and Managing Schemas

Anansi's core strength is its schema-driven approach. Schemas define the structure, types, and constraints of your data.

**Task: Define a New Schema**

1.  **Create a `schema.SchemaDefinition` struct**: You can build this programmatically or, more commonly, unmarshal it from a JSON string or file.
    ```go
    package main

    import (
    	"encoding/json"
    	"fmt"
    	"log"
    	"github.com/asaidimu/go-anansi/core/schema"
    )

    func defineProductSchema() schema.SchemaDefinition {
    	productSchemaJSON := `{
    		"name": "products",
    		"version": "1.0.0",
    		"description": "Schema for product catalog",
    		"fields": {
    			"id": {"name": "id", "type": "integer", "required": false, "unique": true},
    			"name": {"name": "name", "type": "string", "required": true},
    			"price": {"name": "price", "type": "number", "required": true, "default": 0.0},
    			"category": {"name": "category", "type": "enum", "values": ["electronics", "books", "clothing"]}
    		}
    	}`
    
    	var productSchema schema.SchemaDefinition
    	if err := json.Unmarshal([]byte(productSchemaJSON), &productSchema); err != nil {
    		log.Fatalf("Failed to unmarshal product schema: %v", err)
    	}
    	fmt.Println("Product schema defined.")
    	return productSchema
    }
    ```

2.  **Add Indexes (Optional)**: Indexes can be part of your schema definition to optimize query performance and enforce uniqueness.
    ```json
    "indexes": [
        {"name": "idx_product_category", "fields": ["category"], "type": "normal"},
        {"name": "idx_product_name_unique", "fields": ["name"], "type": "unique"}
    ]
    ```

**Task: Retrieve an Existing Schema**

```go
// Assuming persistenceSvc is your initialized *persistence.Persistence
existingSchema, err := persistenceSvc.Schema("users")
if err != nil {
	log.Fatalf("Failed to retrieve 'users' schema: %v", err)
}
fmt.Printf("Retrieved schema for '%s', version: %s\n", existingSchema.Name, existingSchema.Version)
```

**Task: List All Available Collections (Schemas)**

```go
collectionNames, err := persistenceSvc.Collections()
if err != nil {
	log.Fatalf("Failed to list collections: %v", err)
}
fmt.Println("Available Collections:", collectionNames)
```

## 2. Managing Data with Events

Anansi emits events for various persistence operations, allowing you to react to data changes.

**Task: Subscribe to Document Creation Events**

1.  **Define a callback function**: This function will be executed when the event occurs.
    ```go
    package main

    import (
    	"context"
    	"fmt"
    	"log"
    	"github.com/asaidimu/go-anansi/core/persistence"
    )

    func main() {
    	// ... (database and persistenceSvc initialization)

    	// Get a collection instance (e.g., "users")
    	collection, err := persistenceSvc.Collection("users")
    	if err != nil {
    		log.Fatalf("Failed to get collection: %v", err)
    	}

    	// Register the subscription
    	subscriptionId := collection.RegisterSubscription(persistence.RegisterSubscriptionOptions{
    		Event: persistence.DocumentCreateSuccess,
    		Label: persistence.StringPtr("user_creation_logger"),
    		Description: persistence.StringPtr("Logs details of successfully created user documents."),
    		Callback: func(ctx context.Context, event persistence.PersistenceEvent) error {
    			fmt.Printf("\n[EVENT] Document Created: Collection '%s', ID: %v, Input: %+v\n", 
    				*event.Collection, event.Output.(*query.QueryResult).Data.(schema.Document)["id"], event.Input)
    			return nil
    		},
    	})
    	fmt.Printf("Registered subscription with ID: %s\n", subscriptionId)

    	// Now, any document created in 'users' collection will trigger this event.
    	_, err = collection.Create(map[string]any{"name": "Event Test", "email": "event@test.com"})
    	if err != nil {
    		log.Fatalf("Create failed: %v", err)
    	}
    	fmt.Println("Document created, check console for event log.")
    }
    ```

**Task: Unsubscribe from Events**

```go
// Call UnregisterSubscription with the ID returned by RegisterSubscription
collection.UnregisterSubscription(subscriptionId)
fmt.Printf("Unregistered subscription with ID: %s.\n", subscriptionId)
```

## 3. Managing Transactions

Anansi provides a transaction mechanism to ensure atomicity for a series of database operations.

**Task: Perform Operations within a Transaction**

```go
package main

import (
    "context"
    "fmt"
    "log"
    "github.com/asaidimu/go-anansi/core/persistence"
)

func main() {
    // ... (database and persistenceSvc initialization)

    fmt.Println("Starting a new transaction...")
    _, err := persistenceSvc.Transact(func(tx persistence.PersistenceTransactionInterface) (any, error) {
        // Obtain a collection instance that operates within this transaction
        txCollection, err := tx.Collection("users")
        if err != nil {
            return nil, fmt.Errorf("failed to get transactional collection: %w", err)
        }

        // Operation 1: Create a user
        _, err = txCollection.Create(map[string]any{
            "name":      "Transaction User A",
            "email":     "txa@example.com",
            "age":       40,
            "is_active": true,
        })
        if err != nil {
            return nil, fmt.Errorf("transaction create 1 failed: %w", err) // This error will cause rollback
        }

        // Operation 2: Create another user
        _, err = txCollection.Create(map[string]any{
            "name":      "Transaction User B",
            "email":     "txb@example.com",
            "age":       45,
            "is_active": true,
        })
        if err != nil {
            return nil, fmt.Errorf("transaction create 2 failed: %w", err) // This error will cause rollback
        }

        fmt.Println("Inside transaction: Both users created (pending commit).")
        // If we return an error here, the transaction will be rolled back.
        // For demonstration of rollback:
        // return nil, fmt.Errorf("simulating a transaction failure")

        return nil, nil // Return nil, nil to commit the transaction
    })

    if err != nil {
        fmt.Printf("Transaction failed and was rolled back: %v\n", err)
    } else {
        fmt.Println("Transaction committed successfully. Users are now persisted.")
    }

    // Verify outside the transaction (users should only be visible if committed)
    // readQuery := query.NewQueryBuilder().Where("email").In("txa@example.com", "txb@example.com").Build()
    // result, _ := persistenceSvc.Collection("users").Read(&readQuery)
    // fmt.Printf("Users after transaction (should be visible if committed): %d\n", result.Count)
}
```

## 4. Validating Data

Anansi integrates schema validation to ensure data integrity before persistence.

**Task: Validate Data Against Schema**

```go
package main

import (
	"fmt"
	"log"
	"github.com/asaidimu/go-anansi/core/schema"
)

func main() {
    // ... (database and persistenceSvc initialization, collection creation)

    collection, err := persistenceSvc.Collection("users")
    if err != nil {
        log.Fatalf("Failed to get collection: %v", err)
    }

    // Scenario 1: Valid data
    validUser := map[string]any{
        "name":      "Valid User",
        "email":     "valid@example.com",
        "age":       25,
        "is_active": true,
    }
    fmt.Println("Validating valid user data...")
    validationResult, err := collection.Validate(validUser, false) // `false` for strict validation
    if err != nil {
        fmt.Printf("Error during validation: %v\n", err)
    }
    if validationResult.Valid {
        fmt.Println("  Validation successful!")
    } else {
        fmt.Println("  Validation failed (unexpected)! Issues:", validationResult.Issues)
    }

    // Scenario 2: Invalid data (missing required field 'email')
    invalidUserMissingEmail := map[string]any{
        "name":      "Invalid User",
        "age":       30,
        "is_active": true,
    }
    fmt.Println("Validating invalid user data (missing email)...")
    validationResult, err = collection.Validate(invalidUserMissingEmail, false)
    if err != nil {
        fmt.Printf("Error during validation: %v\n", err)
    }
    if !validationResult.Valid {
        fmt.Println("  Validation failed as expected. Issues found:")
        for _, issue := range validationResult.Issues {
            fmt.Printf("    Code: %s, Message: %s, Path: %s, Severity: %s\n",
                issue.Code, issue.Message, issue.Path, issue.Severity)
        }
    } else {
        fmt.Println("  Validation unexpectedly successful!")
    }

    // Scenario 3: Invalid data (wrong type for 'age')
    invalidUserWrongType := map[string]any{
        "name":      "Type Error User",
        "email":     "type@example.com",
        "age":       "twenty", // Should be integer
        "is_active": true,
    }
    fmt.Println("Validating invalid user data (wrong age type)...")
    validationResult, err = collection.Validate(invalidUserWrongType, false)
    if err != nil {
        fmt.Printf("Error during validation: %v\n", err)
    }
    if !validationResult.Valid {
        fmt.Println("  Validation failed as expected. Issues found:")
        for _, issue := range validationResult.Issues {
            fmt.Printf("    Code: %s, Message: %s, Path: %s, Severity: %s\n",
                issue.Code, issue.Message, issue.Path, issue.Severity)
        }
    } else {
        fmt.Println("  Validation unexpectedly successful!")
    }

    // Scenario 4: Loose validation (ignores missing required fields)
    fmt.Println("Validating invalid user data with loose=true (missing email)...")
    validationResult, err = collection.Validate(invalidUserMissingEmail, true) 
    if err != nil {
        fmt.Printf("Error during validation: %v\n", err)
    }
    if validationResult.Valid {
        fmt.Println("  Loose validation successful (as expected, missing required fields ignored)!")
    } else {
        fmt.Println("  Loose validation failed (unexpected)! Issues:", validationResult.Issues)
    }
}
```

---
### 🤖 AI Agent Guidance

```json
{
  "decisionPoints": [
    "IF schema_retrieval_fails THEN report_error ELSE proceed",
    "IF collection_listing_fails THEN report_error ELSE display_collections",
    "IF subscription_registration_fails THEN log_error ELSE confirm_subscription_id",
    "IF transaction_callback_returns_error THEN rollback_transaction ELSE commit_transaction",
    "IF data_validation_fails THEN log_issues_and_reject_operation ELSE proceed_with_data"
  ],
  "verificationSteps": [
    "Check: `persistenceSvc.Schema` returns a non-nil `SchemaDefinition` and `err == nil`.",
    "Check: `persistenceSvc.Collections` returns a non-empty `[]string` for existing collections.",
    "Check: `collection.RegisterSubscription` returns a valid `subscriptionId` string.",
    "Check: The `Transact` function's return `err` to confirm transaction outcome (nil for commit, non-nil for rollback).",
    "Check: `validationResult.Valid` is `true` for valid data, `false` otherwise. Examine `validationResult.Issues` for details."
  ],
  "quickPatterns": [
    "Pattern: Retrieve Collection Schema\n```go\nmySchema, err := persistenceSvc.Schema(\"my_collection\")\n// Handle err, use mySchema\n```",
    "Pattern: Register Event Listener\n```go\nsubID := collection.RegisterSubscription(persistence.RegisterSubscriptionOptions{\n    Event: persistence.DocumentCreateSuccess,\n    Callback: func(ctx context.Context, event persistence.PersistenceEvent) error { /* your logic */ return nil },\n})\n```",
    "Pattern: Execute Transaction\n```go\n_, err := persistenceSvc.Transact(func(tx persistence.PersistenceTransactionInterface) (any, error) {\n    // Operations using tx.Collection(...)\n    return nil, nil // Or return error to rollback\n})\n```",
    "Pattern: Validate Data\n```go\nvalidationResult, err := collection.Validate(myData, false)\nif !validationResult.Valid { /* handle issues */ }\n```"
  ],
  "diagnosticPaths": [
    "Error: SchemaNotFound -> Symptom: `Collection <name> does not exist` -> Check: Ensure the collection name matches an existing schema in the internal `_schemas` collection -> Fix: Use the correct collection name or create the collection first.",
    "Error: SubscriptionCallbackFailure -> Symptom: Error message logged from within your event callback function -> Check: Review callback logic, ensure it handles `PersistenceEvent` payload correctly -> Fix: Debug and correct logic within the callback function.",
    "Error: TransactionIsolationFailure -> Symptom: Data appears/disappears unexpectedly mid-transaction from other connections -> Check: Ensure your `DatabaseInteractor` correctly implements transaction isolation levels (usually database-dependent) -> Fix: Consult database documentation for isolation best practices or re-evaluate transaction scope.",
    "Error: ValidationCoercionIssue -> Symptom: `TYPE_MISMATCH` or unexpected validation issues for seemingly correct data -> Check: Review `schema.FieldType` definitions for the fields in question, ensure implicit type coercions (e.g., string '1' to int) are intended and correctly handled by Anansi's validator -> Fix: Adjust schema `FieldType` or pre-process data to explicit types if automatic coercion is problematic."
  ]
}
```

---
*Generated using Gemini AI on 6/28/2025, 10:32:05 PM. Review and refine as needed.*