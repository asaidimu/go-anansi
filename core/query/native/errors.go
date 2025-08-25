package native

import (
	"errors"
	"strings"
)

// NativeError represents errors specific to native query operations.
type NativeError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *NativeError) Error() string {
	var b strings.Builder
	b.WriteString(e.Operation)
	b.WriteString(" operation failed")

	if e.Key != "" {
		b.WriteString(" for key '")
		b.WriteString(e.Key)
		b.WriteString("': ")
	} else {
		b.WriteString(": ")
	}
	b.WriteString(e.Message)

	if e.Cause != nil {
		b.WriteString(" (caused by: ")
		b.WriteString(e.Cause.Error())
		b.WriteString(")")
	}
	return b.String()
}

func (e *NativeError) Unwrap() error {
	return e.Cause
}

// Pre-defined errors for the native package.
var (
	ErrCouldNotGetQuery                 = errors.New("could not get a query")
	ErrCouldNotGetResultSchema          = errors.New("could not get result schema")
	ErrCouldNotDeleteWithoutFilters     = errors.New("could not delete without filters")
	ErrCouldNotBuildCreateCollectionQuery = errors.New("could not build create collection query")
	ErrCouldNotCreateCollection         = errors.New("could not create collection")
	ErrCouldNotBuildCreateIndexQuery    = errors.New("could not build create index query")
	ErrCouldNotCreateIndex              = errors.New("could not create index")
	ErrCannotNestTransactions           = errors.New("cannot nest transactions")
	ErrFailedToBeginTransaction         = errors.New("failed to begin transaction")
	ErrOperationFailed                  = errors.New("operation failed")
	ErrRollbackFailed                   = errors.New("rollback failed")
	ErrFailedToCommitTransaction        = errors.New("failed to commit transaction")
	ErrCommitNotApplicable              = errors.New("commit not applicable")
	ErrRollbackNotApplicable            = errors.New("rollback not applicable")
)
