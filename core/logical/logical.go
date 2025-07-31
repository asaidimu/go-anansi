package logical

import "errors"

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

// Pre-defined errors for logical operator evaluation.
var (
	ErrEmptyResults      = errors.New("results slice cannot be empty")
	ErrInvalidOperator   = errors.New("unknown logical operator")
	ErrInvalidNotOperand = errors.New("logical NOT requires exactly one result")
)

// Evaluate evaluates a logical operator against a slice of boolean results.
func (o LogicalOperator) Evaluate(conditions []bool) (bool, error) {
	if len(conditions) == 0 {
		return false, ErrEmptyResults
	}

	switch o {
	case LogicalAnd:
		for _, r := range conditions {
			if !r {
				return false, nil
			}
		}
		return true, nil
	case LogicalOr:
		for _, r := range conditions {
			if r {
				return true, nil
			}
		}
		return false, nil
	case LogicalNot:
		if len(conditions) != 1 {
			return false, ErrInvalidNotOperand
		}
		return !conditions[0], nil
	case LogicalNor:
		for _, r := range conditions {
			if r {
				return false, nil
			}
		}
		return true, nil
	case LogicalXor:
		trueCount := 0
		for _, r := range conditions {
			if r {
				trueCount++
			}
		}
		return trueCount == 1, nil
	case LogicalNand:
		for _, r := range conditions {
			if !r {
				return true, nil
			}
		}
		return false, nil
	case LogicalXnor:
		trueCount := 0
		for _, r := range conditions {
			if r {
				trueCount++
			}
		}
		return trueCount%2 == 0, nil
	default:
		return false, ErrInvalidOperator
	}
}
