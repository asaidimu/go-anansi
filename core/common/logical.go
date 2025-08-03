package common

import "errors"

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
