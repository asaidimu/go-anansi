package common


// FunctionMap is a map of function names to generic functions.
type FunctionMap map[string]any


// Future represents the result of an asynchronous operation.
type Future[T any] interface {
	// Await blocks until the operation is complete and returns the result and any error that occurred.
	Await() (T, error)
}

// Validatable represents any type that can validate its own state and return
// structured validation issues.
type Validatable interface {
	Validate() Issues
}

