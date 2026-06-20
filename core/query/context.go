package query

import "context"

type contextKey string

const (
	// InteractorContextKey is the context key for the DatabaseInteractor.
	InteractorContextKey contextKey = "__database_interactor__"
)

// WithInteractor returns a new context with the given DatabaseInteractor.
func WithInteractor(ctx context.Context, interactor DatabaseInteractor) context.Context {
	if _, ok := GetInteractor(ctx); !ok {
		return context.WithValue(ctx, InteractorContextKey, interactor)
	}
	return ctx
}

// GetInteractor returns the DatabaseInteractor from the context, if any.
func GetInteractor(ctx context.Context) (DatabaseInteractor, bool) {
	interactor, ok := ctx.Value(InteractorContextKey).(DatabaseInteractor)
	return interactor, ok
}
