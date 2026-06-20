// Package transaction provides a robust mechanism for managing database transactions.
// It supports concurrent operations within a single transaction and handles nested
// transaction scopes gracefully.
package transaction

import (
	"context"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const TxKey string = "github.com/asaidimu/go-anansi/__transaction__"

// transaction represents a single database transaction, coordinating multiple
// concurrent operations. It ensures that all operations complete successfully
// before the transaction is committed.
type transaction struct {
	interactor      query.DatabaseInteractor
	wg              sync.WaitGroup
	errChan         chan error
	errOnce         sync.Once
	mu              sync.RWMutex
	committed       bool
	id              string
	logger          *zap.Logger
	onCommitHooks   []func() // Functions to execute after a successful commit
	onRollbackHooks []func() // Functions to execute after a successful commit
}

// Ensures transaction implements the base.Transaction interface.
var _ base.Transaction = (*transaction)(nil)

// newTransaction creates a new transaction instance.
// Each transaction is assigned a unique ID for logging and tracking purposes.
func newTransaction(interactor query.DatabaseInteractor, logger *zap.Logger) *transaction {
	id := uuid.Must(uuid.NewV7())
	return &transaction{
		interactor:    interactor,
		errChan:       make(chan error, 1),
		id:            id.String(),
		logger:        logger,
		onCommitHooks: make([]func(), 0),
	}
}

// OnCommit adds a function to be executed after the transaction successfully commits.
func (tx *transaction) OnCommit(hook func()) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.onCommitHooks = append(tx.onCommitHooks, hook)
}

// OnRollback adds a function to be executed after the transaction successfully commits.
func (tx *transaction) OnRollback(hook func()) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.onRollbackHooks = append(tx.onRollbackHooks, hook)
}

// IsActive returns true if the transaction has not yet been committed or rolled back.
func (tx *transaction) IsActive() bool {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	return !tx.committed
}

// AddOperation registers a new concurrent operation within the transaction.
// It increments a WaitGroup counter and returns a cleanup function that must be
// called when the operation is complete. The cleanup function captures the first
// error that occurs among all concurrent operations.
func (tx *transaction) AddOperation() func(error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	// Do not allow new operations on an already finalized transaction.
	if tx.committed {
		return func(error) {}
	}

	tx.wg.Add(1)
	return func(err error) {
		defer tx.wg.Done()
		if err != nil {
			// Atomically send the first error to the error channel.
			tx.errOnce.Do(func() {
				select {
				case tx.errChan <- err:
				default:
				}
			})
		}
	}
}

// WaitForOperations blocks until all registered operations complete or the context
// is cancelled. It returns the first error reported by any of the operations.
func (tx *transaction) WaitForOperations(ctx context.Context) error {
	done := make(chan struct{})
	var operationErr error

	// Start a goroutine to wait for the WaitGroup. This allows us to race
	// the wait against the context's deadline or cancellation.
	go func() {
		defer close(done)
		tx.wg.Wait()
		close(tx.errChan)           // Close channel to signal no more errors will be sent.
		operationErr = <-tx.errChan // Read the one potential error.
	}()

	select {
	case <-done:
		return operationErr
	case <-ctx.Done():
		return base.ErrTransactionTimeout.WithCause(ctx.Err())
	}
}

// Commit commits the transaction.
func (tx *transaction) Commit(ctx context.Context) error {
	err := tx.finalize(ctx, func(ctx context.Context, ti query.DatabaseInteractor) error {
		return ti.Commit(ctx)
	})
	if err == nil {
		for _, hook := range tx.onCommitHooks {
			hook()
		}
		tx.onCommitHooks = nil // Clear hooks after execution
		tx.onRollbackHooks = nil
	}
	return err
}

// Rollback rolls back the transaction.
func (tx *transaction) Rollback(ctx context.Context) error {
	err := tx.finalize(ctx, func(ctx context.Context, ti query.DatabaseInteractor) error {
		return ti.Rollback(ctx)
	})
	for _, hook := range tx.onRollbackHooks {
		hook()
	}
	tx.onCommitHooks = nil // Clear hooks after execution
	tx.onRollbackHooks = nil
	return err
}

// GetInteractor returns the underlying transactional database interactor.
func (tx *transaction) GetInteractor() query.DatabaseInteractor {
	return tx.interactor
}

// finalize handles the common logic for committing or rolling back a transaction,
// ensuring the action is performed safely and only once.
func (tx *transaction) finalize(ctx context.Context, op func(context.Context, query.DatabaseInteractor) error) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return base.ErrTransactionAlreadyFinalized
	}
	defer func() { tx.committed = true }()

	if !tx.interactor.HasTransaction(ctx) {
		return base.ErrTransactionNoActive
	}

	return op(ctx, tx.interactor)
}

// Execute wraps a callback function in a database transaction.
// It handles beginning the transaction, and then committing or rolling back based on
// the errors returned by the callback and any concurrent operations.
// If a transaction is already present in the context, it reuses it, enabling
// transaction nesting.
func Execute(
	ctx context.Context,
	interactor query.DatabaseInteractor,
	logger *zap.Logger,
	callback func(ctx context.Context, interactor query.DatabaseInteractor) (any, error),
) (any, error) {

	// If we're already inside a transaction, reuse it.
	if existingTx, inTx := GetCurrentTransaction(ctx); inTx {
		cleanup := existingTx.AddOperation()
		result, err := callback(ctx, existingTx.GetInteractor())
		cleanup(err) // Report operation result to the parent transaction.
		return result, err
	}

	// We are the top-level transaction manager.
	var baseInteractor query.DatabaseInteractor = interactor
	var err error
	managed := false // 'managed' means this 'Execute' call is responsible for commit/rollback.

	if !baseInteractor.HasTransaction(ctx) {
		baseInteractor, err = baseInteractor.StartTransaction(ctx)
		if err != nil {
			return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_FAILED_TO_START_TRANSACTION")
		}
		managed = true
	}

	tx := newTransaction(baseInteractor, logger)
	txCtx := withTransaction(ctx, tx)
	ictx := query.WithInteractor(txCtx, baseInteractor)

	result, callbackErr := callback(ictx, baseInteractor)
	operationErr := tx.WaitForOperations(ictx)

	// If this 'Execute' call did not start the transaction, we must not commit or rollback.
	if !managed {
		if callbackErr != nil {
			return result, callbackErr
		}
		return result, operationErr
	}

	// Determine final transaction outcome based on errors.
	var finalErr error
	if callbackErr != nil {
		finalErr = callbackErr
	} else if operationErr != nil {
		finalErr = base.ErrTransactionAsyncOperationFailed.WithCause(operationErr)
	}

	if finalErr != nil {
		if rollbackErr := tx.Rollback(ictx); rollbackErr != nil {
			return result, base.ErrTransactionFailed.WithCause(rollbackErr).WithCause(finalErr)
		}
		return result, finalErr
	}

	if commitErr := tx.Commit(ictx); commitErr != nil {
		err := base.ErrTransactionCommitFailed.WithCause(commitErr)
		if rollbackErr := tx.Rollback(ictx); rollbackErr != nil {
			return result, err.WithCause(rollbackErr)
		}
		return result, err
	}

	return result, nil
}

func (tx *transaction) ID() string {
	return tx.id
}

// withTransaction embeds the transaction into a new context.
func withTransaction(ctx context.Context, tx base.Transaction) context.Context {
	return context.WithValue(ctx, TxKey, tx)
}

// GetCurrentTransaction retrieves the current transaction from the context, if one exists.
func GetCurrentTransaction(ctx context.Context) (base.Transaction, bool) {
	tx, ok := ctx.Value(TxKey).(base.Transaction)
	return tx, ok
}
