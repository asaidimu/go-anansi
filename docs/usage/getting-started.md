# Getting Started

# Getting Started with Anansi

This section provides an overview of Anansi, its core concepts, and a quick guide to setting up your first project. Anansi aims to simplify data persistence in Go applications by leveraging declarative schemas and a flexible architecture.

## Core Concepts

*   **Schema-Driven**: Data models are defined using `schema.SchemaDefinition` (often JSON), which dictates table structure, field types, validation rules, and indexing.
*   **Collection**: An abstraction over a database table, representing a logical grouping of documents conforming to a `SchemaDefinition`.
*   **Document**: A single record or entry within a Collection, represented as a `map[string]any`.
*   **QueryDSL**: A declarative, fluent API (`query.QueryBuilder`) for constructing complex database queries, which are then translated to SQL by specific database adapters.
*   **DatabaseInteractor**: An interface (`persistence.DatabaseInteractor`) that abstracts low-level database operations (CRUD, DDL, transactions), allowing Anansi to support multiple database backends.
*   **Persistence Service**: The top-level service (`persistence.Persistence`) that orchestrates schema management, collection creation, and transactional operations.

## Architecture Overview

Anansi's architecture is layered, promoting separation of concerns:

1.  **Core (Interfaces)**: Defines the fundamental interfaces for persistence (`persistence`), querying (`query`), and schema management (`schema`). These are generic and database-agnostic.
2.  **Database Adapters**: Concrete implementations of the core interfaces for specific database systems (e.g., `sqlite`). These handle database-specific SQL generation, type mapping, and connection management.
3.  **Application Layer**: Your Go application interacts with Anansi through the top-level `persistence.Persistence` and `persistence.PersistenceCollectionInterface` APIs.

## Quick Setup Guide

To quickly get started, you'll need Go and a SQLite environment. Follow these steps to initialize Anansi and create your first collection.

### Prerequisites

*   **Go**: Version `1.24.4` or newer. Download from [golang.org/dl](https://golang.org/dl/).
*   **SQLite3**: The `github.com/mattn/go-sqlite3` driver requires the SQLite C library to be present on your system. Most Linux distributions and macOS come with it pre-installed. For Windows, you might need to install it manually.

### Installation

1.  **Initialize your Go module** (if you haven't already):
    ```bash
    go mod init your-module-name
    ```
2.  **Add Anansi as a dependency**:
    ```bash
    go get github.com/asaidimu/go-anansi
    go mod tidy
    ```
3.  **Import the SQLite driver** in your `main.go` or relevant file:
    ```go
    import _ "github.com/mattn/go-sqlite3"
    ```

### First Tasks: Initializing Persistence and Creating a Collection

This example demonstrates how to set up the database, initialize the Anansi `Persistence` service, and define/create your first data collection using a JSON schema.

```go
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/asaidimu/go-anansi/core/persistence"
	"github.com/asaidimu/go-anansi/core/schema"
	"github.com/asaidimu/go-anansi/sqlite"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

const dbFileName = "my_first_anansi.db"

// Define your schema as a JSON string for easy externalization.
const userSchemaJSON = `{
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

func main() {
	// Clean up previous database file to start fresh
	if err := os.Remove(dbFileName); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to remove existing database file %s: %v", dbFileName, err)
	}
	fmt.Printf("Starting fresh: removed existing %s (if any).\n", dbFileName)

	// 1. Open SQLite database connection
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

	// 2. Initialize the SQLite Interactor
	// (Logger and options are optional; nil defaults to a no-op logger and default options)
	interactor := sqlite.NewSQLiteInteractor(db, nil, nil, nil)

	// 3. Initialize the core Anansi Persistence service
	// (Empty FunctionMap for now; see 'In-memory Go Functions' section for details)
	persistenceSvc, err := persistence.NewPersistence(interactor, schema.FunctionMap{})
	if err != nil {
		log.Fatalf("Failed to initialize persistence: %v", err)
	}
	fmt.Println("Initialized persistence service.")

	// 4. Unmarshal your schema JSON into a SchemaDefinition struct
	var userSchema schema.SchemaDefinition
	err = json.Unmarshal([]byte(userSchemaJSON), &userSchema)
	if err != nil {
		log.Fatalf("Failed to unmarshal user schema JSON: %v", err)
	}
	fmt.Println("User schema unmarshaled successfully.")

	// 5. Create the collection (table) in the database based on the schema
	_, err = persistenceSvc.Create(userSchema)
	if err != nil {
		log.Fatalf("Failed to create collection 'users': %v", err)
	}
	fmt.Println("'users' collection created successfully.")

	// You now have a 'users' table ready for CRUD operations!
}
```

---
### 🤖 AI Agent Guidance

```json
{
  "decisionPoints": [
    "IF database_file_exists THEN delete_old_database_file ELSE proceed",
    "IF database_connection_fails THEN log_fatal_error ELSE proceed",
    "IF persistence_initialization_fails THEN log_fatal_error ELSE proceed",
    "IF schema_unmarshal_fails THEN log_fatal_error ELSE proceed",
    "IF collection_creation_fails THEN log_fatal_error ELSE proceed"
  ],
  "verificationSteps": [
    "Check: `os.IsNotExist(err)` to safely remove old DB file",
    "Check: `err == nil` after `sql.Open` for successful DB connection",
    "Check: `err == nil` after `persistence.NewPersistence` for successful service initialization",
    "Check: `err == nil` after `json.Unmarshal` for valid schema definition",
    "Check: `err == nil` after `persistenceSvc.Create(userSchema)` for table creation"
  ],
  "quickPatterns": [
    "Pattern: Go Database Setup\n```go\ndb, err := sql.Open(\"sqlite3\", dbFileName)\nif err != nil {\n    log.Fatalf(\"Failed to open database: %v\", err)\n}\ndefer db.Close()\n```",
    "Pattern: Anansi Persistence Initialization\n```go\ninteractor := sqlite.NewSQLiteInteractor(db, nil, nil, nil) // or custom logger/options\npersistenceSvc, err := persistence.NewPersistence(interactor, schema.FunctionMap{})\nif err != nil {\n    log.Fatalf(\"Failed to initialize persistence: %v\", err)\n}\n```",
    "Pattern: Schema Unmarshaling and Collection Creation\n```go\nvar mySchema schema.SchemaDefinition\nerr = json.Unmarshal([]byte(mySchemaJSON), &mySchema)\nif err != nil {\n    log.Fatalf(\"Failed to unmarshal schema: %v\", err)\n}\n_, err = persistenceSvc.Create(mySchema)\nif err != nil {\n    log.Fatalf(\"Failed to create collection: %v\", err)\n}\n```"
  ],
  "diagnosticPaths": [
    "Error: DatabaseConnectionFailed -> Symptom: `Failed to open database connection` log message -> Check: Database file path, permissions, SQLite3 C library installation -> Fix: Correct file path, grant permissions, install SQLite3 library",
    "Error: PersistenceInitializationFailed -> Symptom: `Failed to initialize persistence` log message -> Check: `DatabaseInteractor` instance, underlying database status -> Fix: Ensure database is reachable and `DatabaseInteractor` is correctly instantiated",
    "Error: SchemaUnmarshalFailed -> Symptom: `Failed to unmarshal user schema JSON` log message -> Check: `userSchemaJSON` string for valid JSON syntax, adherence to `schema.SchemaDefinition` structure -> Fix: Correct JSON syntax, adjust schema definition fields",
    "Error: CollectionCreationFailed -> Symptom: `Failed to create collection 'users'` log message -> Check: Schema name uniqueness, database permissions, `CreateCollection` logs for SQL errors -> Fix: Use a unique collection name, grant appropriate database privileges, review generated SQL if logged"
  ]
}
```

---
*Generated using Gemini AI on 6/28/2025, 10:32:05 PM. Review and refine as needed.*