# Integration Guide

## Environment Requirements

Go Runtime: Version 1.24.4 or newer.
SQLite C Library: Required for `github.com/mattn/go-sqlite3`. Typically pre-installed on Linux/macOS. Windows users may need to install via MSYS2 or pre-compiled binaries.

## Initialization Patterns

### Standard setup for initializing Anansi Persistence with SQLite.
```[DETECTED_LANGUAGE]
package main

import (
	"database/sql"
	"log"

	"github.com/asaidimu/go-anansi/core/persistence"
	"github.com/asaidimu/go-anansi/core/schema"
	"github.com/asaidimu/go-anansi/sqlite"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

func main() {
	// Open database connection
	db, err := sql.Open("sqlite3", "./my_app.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Initialize SQLite Interactor with default options
	// A zap.Logger instance can be passed instead of nil for logging
	interactor := sqlite.NewSQLiteInteractor(db, nil, sqlite.DefaultInteractorOptions(), nil)

	// Initialize the Anansi Persistence service
	// Pass an empty schema.FunctionMap if no custom Go functions are needed initially
	persistenceSvc, err := persistence.NewPersistence(interactor, schema.FunctionMap{})
	if err != nil {
		log.Fatalf("Failed to initialize persistence: %v", err)
	}

	// Example: Create a collection
	// var mySchema schema.SchemaDefinition = ... // Your schema definition
	// _, err = persistenceSvc.Create(mySchema)
	// if err != nil { log.Fatalf("Failed to create collection: %v", err) }
}

```

## Common Integration Pitfalls

- **Issue**: Forgetting to import the SQLite driver with `_`.
  - **Solution**: Ensure `_ "github.com/mattn/go-sqlite3"` is present in your import statements.

- **Issue**: Not handling `sql.DB` connection closing with `defer db.Close()`.
  - **Solution**: Always `defer db.Close()` immediately after `sql.Open` to ensure resources are released.

- **Issue**: Attempting to create a collection with a name that already exists without `DropIfExists` or `IfNotExists` configured.
  - **Solution**: Set `persistence.InteractorOptions.DropIfExists = true` for development/testing, or `IfNotExists = true` for idempotent creation, or check `CollectionExists` before `Create`.

- **Issue**: Passing a nil `schema.FunctionMap` when custom Go functions are expected.
  - **Solution**: Always pass a populated `schema.FunctionMap` if you plan to use custom computed fields or filter predicates.

## Lifecycle Dependencies

The `persistence.Persistence` service depends on an initialized `persistence.DatabaseInteractor`. The `DatabaseInteractor` itself requires an active `*sql.DB` connection. Therefore, `*sql.DB` must be opened and `DatabaseInteractor` initialized before `persistence.NewPersistence` can be called. All database connections should be gracefully closed (e.g., via `defer db.Close()`). Transactions initiated via `StartTransaction` create new, isolated `DatabaseInteractor` instances which must be explicitly `Commit`ted or `Rollback`ed.



---
*Generated using Gemini AI on 6/28/2025, 10:32:05 PM. Review and refine as needed.*