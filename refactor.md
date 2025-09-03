# Transaction Refactoring Plan

This document outlines a plan to refactor the transaction handling in the persistence layer to address a critical flaw in the current implementation and to make the API more ergonomic and safe for concurrent use.

## 1. The Core Problem: Unergonomic and Unsafe Transactions

The current transaction implementation has a fundamental flaw that makes it unsafe and difficult to use correctly. When a transaction is initiated with `s.persistence.Transact`, the transactional persistence object created for the transaction does not share its transactional context with the collections that are used within the transaction's callback function.

### Problematic Code

Here’s the execution flow that demonstrates the problem:

1.  A transaction is started, and a new `basePersistence` object is created for the transaction. However, its `collections` cache is empty, and its `interactor` is `nil`.

    ```go
    // In basePersistence.Transact()
    tx, err := (*p.interactor).StartTransaction(ctx)  // Creates DB transaction
    engine := query.NewQueryEngine(tx, p.logger)     // New engine with tx interactor

    tp := &basePersistence{
        // ... other fields
        interactor:  nil,  // ⚠️ Set to nil!
        collections: make(map[string]base.Collection), // ⚠️ Empty cache!
    }
    ```

2.  The user's callback receives the transactional `base.BasePersistence` object, but it's likely that the collections used inside the callback are retrieved from the original, non-transactional persistence layer.

3.  When a method like `CreateOne` is called on a collection, it uses the **non-transactional** interactor from the original persistence layer, not the transactional one.

    ```go
    result, err := p.Transact(ctx, func(b base.BasePersistence) {

    // This collection is using the NON-transactional interactor
    collection.CreateOne(ctx, doc)
    return nil, nil
    })
    ```

This means that any database operations performed on collections within the `Transact` callback are **not** part of the transaction. They are executed with the original, non-transactional database connection and are auto-committed immediately. This completely defeats the purpose of using a transaction and can lead to data inconsistency.

## 2. Initial Proposed Solution

To solve this, the initial idea was to introduce a context-aware transaction model. This model would use a `Transaction` struct to manage the transaction state and pass it through the `context`. This would ensure that all database operations within the transaction's scope are correctly associated with the transaction.

Here is the initial proposed implementation:

```go
package persistence

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
)

// Transaction represents a single database transaction with coordination
type Transaction struct {
	interactor query.BaseDatabaseInteractor
	wg         sync.WaitGroup
	errChan    chan error
	errOnce    sync.Once
	mu         sync.RWMutex
	committed  bool
	id         string
}

type contextKey string

const txKey contextKey = "transaction"

// newTransaction creates a new transaction instance
func newTransaction(interactor query.BaseDatabaseInteractor, id string) *Transaction {
	return &Transaction{
		interactor: interactor,
		errChan:    make(chan error, 1),
		id:         id,
	}
}

// IsActive returns whether this transaction is still active
func (tx *Transaction) IsActive() bool {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	return !tx.committed
}

// AddOperation increments the operation counter and returns a cleanup function
func (tx *Transaction) AddOperation() func(error) {
	if !tx.IsActive() {
		// Transaction already committed/rolled back - return no-op
		return func(error) {}
	}
	
	tx.wg.Add(1)
	return func(err error) {
		defer tx.wg.Done()
		if err != nil {
			// Send first error to error channel
			tx.errOnce.Do(func() {
				select {
				case tx.errChan <- err:
				default:
					// Channel full or closed, ignore
				}
			})
		}
	}
}

// WaitForOperations waits for all operations to complete and returns any error
func (tx *Transaction) WaitForOperations(ctx context.Context) error {
	done := make(chan struct{})
	var operationErr error
	
	go func() {
		tx.wg.Wait()
		// Check for operation errors
		select {
		case operationErr = <-tx.errChan:
		default:
			// No errors
		}
		close(done)
	}()
	
	// Wait with context timeout
	select {
	case <-done:
		return operationErr
	case <-ctx.Done():
		return fmt.Errorf("transaction operations timeout: %w", ctx.Err())
	}
}

// Commit commits the transaction and marks it as completed
func (tx *Transaction) Commit(ctx context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	
	if tx.committed {
		return fmt.Errorf("transaction already committed")
	}
	
	err := tx.interactor.Commit(ctx)
	tx.committed = true
	close(tx.errChan)
	return err
}

// Rollback rolls back the transaction and marks it as completed
func (tx *Transaction) Rollback(ctx context.Context) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	
	if tx.committed {
		return fmt.Errorf("transaction already committed")
	}
	
	err := tx.interactor.Rollback(ctx)
	tx.committed = true
	close(tx.errChan)
	return err
}

// GetCurrentTransaction retrieves the current transaction from context
func GetCurrentTransaction(ctx context.Context) (*Transaction, bool) {
	tx, ok := ctx.Value(txKey).(*Transaction)
	return tx, ok
}

// WithTransaction creates a context with the given transaction
func WithTransaction(ctx context.Context, tx *Transaction) context.Context {
	return context.WithValue(ctx, txKey, tx)
}

// getCurrentInteractor returns the appropriate interactor for the operation
func (c *baseCollection) getCurrentInteractor(ctx context.Context) (query.BaseDatabaseInteractor, func(error)) {
	if tx, inTx := GetCurrentTransaction(ctx); inTx && tx.IsActive() {
		// We're in an active transaction - use transaction interactor and coordination
		cleanup := tx.AddOperation()
		return tx.interactor, cleanup
	}
	
	// Not in a transaction - use base interactor with no-op cleanup
	return c.interactor, func(error) {}
}

// Transact executes a function within a database transaction
// This works regardless of which goroutine it's called from
func (p *basePersistence) Transact(ctx context.Context, callback func(ctx context.Context) (any, error)) (any, error) {
	// Check if we're already in a transaction (nested)
	if existingTx, inTx := GetCurrentTransaction(ctx); inTx {
		// Nested transaction - just execute in existing transaction
		return callback(ctx)
	}
	
	// Start new database transaction
	dbTx, err := (*p.interactor).StartTransaction(ctx)
	if err != nil {
		return nil, err
	}
	
	// Create our transaction coordinator
	tx := newTransaction(dbTx, fmt.Sprintf("tx-%d", time.Now().UnixNano()))
	
	// Create transaction context
	txCtx := WithTransaction(ctx, tx)
	
	// Execute callback
	result, callbackErr := callback(txCtx)
	
	// Wait for all operations spawned during callback to complete
	operationErr := tx.WaitForOperations(ctx)
	
	// Determine if we should commit or rollback
	if callbackErr != nil {
		tx.Rollback(ctx)
		return result, fmt.Errorf("transaction failed (callback): %w", callbackErr)
	}
	
	if operationErr != nil {
		tx.Rollback(ctx)
		return result, fmt.Errorf("transaction failed (operation): %w", operationErr)
	}
	
	// Commit transaction
	if commitErr := tx.Commit(ctx); commitErr != nil {
		tx.Rollback(ctx)
		return result, fmt.Errorf("failed to commit transaction: %w", commitErr)
	}
	
	return result, nil
}

// Modified collection methods - these work from any goroutine
func (c *baseCollection) CreateOne(ctx context.Context, doc data.Document) (base.CreateResult, error) {
	interactor, cleanup := c.getCurrentInteractor(ctx)
	
	inserted, err := interactor.InsertDocuments(ctx, c.schema, []data.Document{doc})
	
	// Always call cleanup with error status
	cleanup(err)
	
	if err != nil {
		return base.CreateResult{Status: base.StatusFailedPersistence, Data: doc, Error: err.Error()}, err
	}

	insertedDocs := inserted.([]data.Document)
	if len(insertedDocs) > 0 {
		doc := insertedDocs[0]
		doc.MustVerifyHash()
		return base.CreateResult{Status: base.StatusCreated, Data: doc}, nil
	}

	insertErr := fmt.Errorf("no document inserted")
	cleanup(insertErr)
	return base.CreateResult{Status: base.StatusFailedPersistence, Data: doc}, insertErr
}

func (c *baseCollection) Update(ctx context.Context, params *base.CollectionUpdate) (int, error) {
	if params == nil || params.Filter == nil {
		return 0, base.NewPersistenceError(base.ErrInvalidUpdateParams.Error(), base.ErrInvalidUpdateParams)
	}

	interactor, cleanup := c.getCurrentInteractor(ctx)
	
	result, err := interactor.UpdateDocuments(ctx, c.schema, params.Data, params.Filter)
	cleanup(err)
	
	if err != nil {
		return 0, base.NewPersistenceError(fmt.Sprintf("%s: %v", base.ErrUpdateDocuments.Error(), err), base.ErrUpdateDocuments)
	}

	rowsAffected := result.(int64)
	return int(rowsAffected), nil
}

func (c *baseCollection) Delete(ctx context.Context, q *query.QueryFilter, unsafe bool) (int, error) {
	if q == nil && !unsafe {
		return 0, base.NewPersistenceError(base.ErrDeleteRequiresFilter.Error(), base.ErrDeleteRequiresFilter)
	}

	interactor, cleanup := c.getCurrentInteractor(ctx)
	
	result, err := interactor.DeleteDocuments(ctx, c.schema, q, unsafe)
	cleanup(err)
	
	if err != nil {
		return 0, base.NewPersistenceError(fmt.Sprintf("%s: %v", base.ErrDeleteDocuments.Error(), err), base.ErrDeleteDocuments)
	}

	rowsAffected := result.(int64)
	return int(rowsAffected), nil
}
```

## 3. Critique of the Initial Proposal

The initial proposal was a good starting point, but it had several subtle bugs and design issues.

### 3.1. WaitGroup Race Condition (TOCTOU Bug)

**Problem:** There is a race condition between checking if the transaction is active (`!tx.IsActive()`) and adding to the `WaitGroup` (`tx.wg.Add(1)`). A goroutine could check the status, find it active, but before it can add to the `WaitGroup`, the main goroutine could commit or rollback the transaction, leading to a deadlock.

**Problematic Code:**

```go
func (tx *Transaction) AddOperation() func(error) {
	if !tx.IsActive() { // Check
		return func(error) {}
	}
	
	tx.wg.Add(1) // Use
	// ...
}
```

### 3.2. Error Channel Race Condition

**Problem:** The `Commit` and `Rollback` methods close the error channel. If a goroutine tries to send an error to the channel after it's closed, the application will panic.

**Problematic Code:**

```go
func (tx *Transaction) Commit(ctx context.Context) error {
	// ...
	tx.committed = true
	close(tx.errChan) // This can race with a goroutine trying to send an error
	return err
}
```

### 3.3. API Design for Concurrency

**Problem:** The initial API design implicitly manages goroutines, which is not idiomatic Go. It's better to provide an explicit and safe way for users to run asynchronous operations within the transaction.

**Problematic Design:** The user would have to manually create goroutines and then somehow get the transaction from the context, which is not clean.

### 3.4. Missing Timeout in `WaitForOperations`

**Problem:** The `WaitForOperations` method could hang indefinitely if a goroutine gets stuck and never calls `wg.Done()`.

**Problematic Code:**

```go
func (tx *Transaction) WaitForOperations() error {
	tx.wg.Wait() // This will block forever if a goroutine is stuck
	// ...
}
```

## 4. Final Proposed Solution

Based on the critique of the initial proposal, here is the final, improved solution. It uses an interface-based approach to provide a clean and safe API for transactions.

### Interface Definitions

```go
package base

// ... imports

// TransactionPersistence defines the set of operations that can be performed
// within a database transaction. It extends the BasePersistence interface with
// an Async method for running concurrent operations within the transaction.
type TransactionPersistence interface {
	BasePersistence

	// Async executes a function in a separate goroutine, ensuring that the
	// transaction will wait for it to complete before committing or rolling back.
	Async(f func(ctx context.Context) error)
}

// Persistence defines the core contract for the persistence layer.
// ...
type Persistence interface {
	BasePersistence

	// ... other methods

	// Transact executes a series of operations within a single atomic transaction.
	// The provided callback function receives a TransactionPersistence object, and if the callback
	// returns an error, the transaction is rolled back.
	Transact(ctx context.Context, callback func(tx TransactionPersistence) (any, error)) (any, error)

	// ... other methods
}
```

### Implementation

The implementation will involve:

1.  A `transactionCoordinator` struct to manage the transaction state (WaitGroup, error channel, etc.).
2.  A `transaction` struct that implements the `TransactionPersistence` interface and holds a reference to the `transactionCoordinator` and a transactional `BasePersistence` instance.
3.  The `Transact` method will create a new `transactionCoordinator` and a new `transaction` object for each transaction.
4.  The `Async` method on the `transaction` struct will use the `transactionCoordinator` to manage the goroutines.

This design provides a clean separation of concerns and a safe, explicit API for concurrent operations within a transaction.
