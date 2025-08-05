package common

import "fmt"

// Issue represents a detailed validation or operational issue. It is used to provide
// structured, machine-readable feedback about problems encountered during an operation,
// which is particularly useful for form validation or API error responses.
type Issue struct {
	Code        string `json:"code"`                  // Code is a machine-readable identifier for the type of issue (e.g., "validation_error", "not_found").
	Message     string `json:"message"`               // Message is a human-readable description of the issue.
	Path        string `json:"path,omitempty"`        // Path indicates the location of the issue, such as a field name in a JSON document (e.g., "user.address.zipCode").
	Severity    string `json:"severity,omitempty"`    // Severity indicates the seriousness of the issue, typically "error" or "warning".
	Description string `json:"description,omitempty"` // Description provides a more detailed, potentially multi-line explanation of the issue and how to resolve it.
}

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


func (ve Issue) Error() string {
	if ve.Path != "" {
		return fmt.Sprintf("validation error at '%s': %s", ve.Path, ve.Message)
	}
	return fmt.Sprintf("validation error: %s", ve.Message)
}
