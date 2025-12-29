package common

// ContextKey is a type for context keys to avoid collisions
type ContextKey string

const CollectionNameContextKey ContextKey = "anansi.collection.name"

// LogicalOperator defines the logical operators that can be used to combine conditions
// in constraints and filters.
type LogicalOperator string

// Supported logical operators.
const (
	LogicalAnd  LogicalOperator = "and"  // Represents a logical AND.
	LogicalOr   LogicalOperator = "or"   // Represents a logical OR.
	LogicalNot  LogicalOperator = "not"  // Represents a logical NOT.
	LogicalNor  LogicalOperator = "nor"  // Represents a logical NOR.
	LogicalXor  LogicalOperator = "xor"  // Represents a logical XOR.
	LogicalNand LogicalOperator = "nand" // Represents a logical NAND.
	LogicalXnor LogicalOperator = "xnor" // Represents a logical XNOR.
)

// FunctionMap is a map of function names to generic functions.
type FunctionMap map[string]any



// Future represents the result of an asynchronous operation.
type Future[T any] interface {
	// Await blocks until the operation is complete and returns the result and any error that occurred.
	Await() (T, error)
}
