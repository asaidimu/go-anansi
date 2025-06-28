package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/asaidimu/go-anansi/core"
	"github.com/asaidimu/go-anansi/core/persistence"
	"github.com/asaidimu/go-anansi/sqlite"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

const (
	dbFileName     = "user.db"
	userSchemaJSON = `{
		"name": "users",
		"version": "1.0.0",
		"description": "Schema for user profiles",
		"fields": {
			"id": {
				"name": "id",
				"type": "integer",
				"required": false,
				"unique": true,
				"description": "Unique identifier for the user"
			},
			"name": {
				"name": "name",
				"type": "string",
				"required": true,
				"description": "Full name of the user"
			},
			"email": {
				"name": "email",
				"type": "string",
				"required": true,
				"unique": true,
				"description": "Email address, must be unique"
			},
			"age": {
				"name": "age",
				"type": "integer",
				"required": false,
				"description": "Age of the user (optional)"
			},
			"is_active": {
				"name": "is_active",
				"type": "boolean",
				"required": true,
				"default": true,
				"description": "User account active status"
			}
		},
		"indexes": [
			{
				"name": "pk_user_id",
				"fields": ["id"],
				"type": "primary"
			},
			{
				"name": "idx_user_email",
				"fields": ["email"],
				"type": "unique"
			}
		]
	}`
)

func main() {
	// --- Original Code Block: Database Initialization ---
	// Remove the database file if it already exists to start fresh
	if err := os.Remove(dbFileName); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to remove existing database file %s: %v", dbFileName, err)
	}
	fmt.Printf("Starting fresh: removed existing %s (if any).\n", dbFileName)

	// Open SQLite database connection to a file
	db, err := sql.Open("sqlite3", dbFileName)
	if err != nil {
		log.Fatalf("Failed to open database connection: %v", err)
	}

	defer func() {
		if cErr := db.Close(); cErr != nil {
			log.Printf("Error closing database connection: %v", cErr)
		}
		fmt.Println("Database connection closed.")
	}()

	interactor := persistence.DatabaseInteractor(sqlite.NewSQLiteInteractor(db, nil, nil, nil))
	// Initialize the persistence service
	persistenceSvc, err := persistence.NewPersistence(interactor, core.FunctionMap{})
	if err != nil {
		log.Fatalf("Failed to initialize persistence : %v", err)
	}
	fmt.Println("Initialized persistence.")
	// --- End Original Code Block: Database Initialization ---

	// --- Original Code Block: Schema Definition and Collection Creation ---
	// Define the User schema as a JSON string
	fmt.Println("Defining User schema from JSON string...")

	// Unmarshal the JSON string into a core.SchemaDefinition struct
	var userSchema core.SchemaDefinition
	err = json.Unmarshal([]byte(userSchemaJSON), &userSchema)
	if err != nil {
		log.Fatalf("Failed to unmarshal user schema JSON: %v", err)
	}
	fmt.Println("User schema unmarshaled successfully from JSON.")

	// Create the "users" table in the database
	fmt.Println("Creating 'users' table...")
	_, err = persistenceSvc.Create(userSchema)
	if err != nil {
		log.Fatalf("Failed to create collection 'users': %v", err)
	}
	fmt.Println("'users' table created successfully.")

	// Get the collection instance.
	// As per the original code, 'collection' will hold the type returned by persistence.Collection().
	collection, err := persistenceSvc.Collection("users")
	if err != nil {
		log.Fatalf("Failed to get collection 'users': %v", err)
	}
	// --- End Original Code Block: Schema Definition and Collection Creation ---

	collection.RegisterSubscription(core.RegisterSubscriptionOptions{
		Event: core.DocumentCreateSuccess,
		Callback: func(ctx context.Context, event core.PersistenceEvent) error {
			fmt.Printf("Document added to collection '%s', %v \n", *event.Collection, event)
			return nil
		},
	})

	collection.RegisterSubscription(core.RegisterSubscriptionOptions{
		Event: core.DocumentDeleteSuccess,
		Callback: func(ctx context.Context, event core.PersistenceEvent) error {
			fmt.Printf("Document deleted from collection '%s', %v \n", *event.Collection, event)
			return nil
		},
	})
	// --- Original Code Block: Data Operations ---
	user := map[string]any{
		"name":      "Alice Smith",
		"email":     "alice@example.com",
		"age":       30,
		"is_active": true,
	}

	// Directly call methods on 'collection' as done in the original code,
	// without any explicit type assertions for `collection`.
	validationResult, err := collection.Validate(user, false)
	if err != nil {
		fmt.Printf("Error during validation: %v\n", err)
	}

	if !validationResult.Valid {
		fmt.Println("Validation failed! Issues found:")
		for _, issue := range validationResult.Issues {
			fmt.Printf("  Code: %s, Message: %s, Path: %s, Severity: %s, Description: %s\n",
				issue.Code, issue.Message, issue.Path, issue.Severity, issue.Description)
		}
	} else {
		fmt.Println("Validation successful!")
	}

	fmt.Println("Inserting sample data...")
	_, err = collection.Create(user)
	if err != nil {
		log.Fatalf("Failed to insert Alice 1: %v", err)
	}

	_, err = collection.Create(
		map[string]any{
			"name":      "Alice Smith",
			"email":     "alice2@example.com",
			"age":       27,
			"is_active": true,
		},
	)
	if err != nil {
		log.Fatalf("Failed to insert Alice 2: %v", err)
	}

	_, err = collection.Create(
		map[string]any{
			"name":      "Alice Smith",
			"email":     "alice3@example.com",
			"age":       28,
			"is_active": false,
		},
	)
	if err != nil {
		log.Fatalf("Failed to insert Alice 3: %v", err)
	}

	q := core.NewQueryBuilder().Where("age").Lt(28).Build()
	_, err = collection.Delete(q.Filters, false)
	if err != nil {
		log.Fatalf("Failed to delete Alice: %v", err)
	}

	fmt.Println("Sample data inserted successfully.")

	q = core.NewQueryBuilder().Where("age").Eq(28).Build()
	_, err = collection.Update(&core.CollectionUpdate{
		Data: map[string]any{
			"name": "Alex Smith",
		},
		Filter: q.Filters,
	})

	if err != nil {
		log.Fatalf("Failed to update to Alex: %v", err)
	}
	// --- End Original Code Block: Data Operations ---

	// --- Original Code Block: Query and Print ---
	fmt.Println("\nQuerying data from 'users' table:")
	q = core.NewQueryBuilder().Build()

	out, err := collection.Read(q)
	if err != nil {
		log.Fatalf("Failed to read database: %v", err)
	}

	fmt.Println("-------------------------------------------------------------------")
	fmt.Printf("%-10s %-20s %-25s %-5s %-10s\n", "ID", "Name", "Email", "Age", "Active")
	fmt.Println("-------------------------------------------------------------------")

	result, _ := out.(*core.QueryResult)
	rows := result.Data.([]core.Document)

	for _, row := range rows {
		id := row["id"].(int64)
		name := row["name"].(string)
		email := row["email"].(string)
		isActive := row["is_active"].(bool)
		age := row["age"].(int64)

		fmt.Printf("%-10d %-20s %-25s %-5d %-10t\n", id, name, email, age, isActive)
	}

	fmt.Println("-------------------------------------------------------------------")


	// --- End Original Code Block: Query and Print ---

	// --- Original Code Block: Instructions ---
	fmt.Printf("\nDatabase created successfully at: %s\n", dbFileName)
	fmt.Println("You can inspect this database file using the 'sqlite3' command-line tool:")
	fmt.Printf("1. Open your terminal.\n")
	fmt.Printf("2. Navigate to the directory where 'main.go' and 'user.db' are located.\n")
	fmt.Printf("3. Run: sqlite3 %s\n", dbFileName)
	fmt.Printf("4. Inside the sqlite3 prompt, you can run SQL commands:\n")
	fmt.Printf("    - .tables (to list tables)\n")
	fmt.Printf("    - .schema users (to view table schema)\n")
	fmt.Printf("    - SELECT * FROM users; (to view data)\n")
	fmt.Printf("    - .quit (to exit)\n")
	// --- End Original Code Block: Instructions ---

	_, err = persistenceSvc.Transact(func(tx core.PersistenceTransactionInterface) (any, error) {
		collection, err := tx.Collection("users")
		if err != nil {
			return nil, err
		}

		q = core.NewQueryBuilder().Build()

		out, err := collection.Read(q)
		if err != nil {
			log.Fatalf("Failed to read database: %v", err)
		}

		fmt.Println("-------------------------------------------------------------------")
		fmt.Printf("%-10s %-20s %-25s %-5s %-10s\n", "ID", "Name", "Email", "Age", "Active")
		fmt.Println("-------------------------------------------------------------------")

		result, _ := out.(*core.QueryResult)
		rows := result.Data.([]core.Document)

		for _, row := range rows {
			id := row["id"].(int64)
			name := row["name"].(string)
			email := row["email"].(string)
			isActive := row["is_active"].(bool)
			age := row["age"].(int64)

			fmt.Printf("%-10d %-20s %-25s %-5d %-10t\n", id, name, email, age, isActive)
		}

		fmt.Println("-------------------------------------------------------------------")

		return nil, nil
	})

	if err != nil {
		log.Fatalf("Transaction failed: %v", err)
	}

	// delete collection
	fmt.Println("Droping 'users' table...")
	_, err = persistenceSvc.Delete("users")
	if err != nil {
		log.Fatalf("Failed to drop table: %v", err)
	}

	fmt.Println("Dropped 'users' table...")
}
