package common

import (
	"encoding/json"
	"fmt"
)

// LogicalOperator defines the logical operators used to combine conditions.
type LogicalOperator byte

const (
	// LogicalAnd represents a logical AND (all conditions must be true).
	LogicalAnd LogicalOperator = iota + 1
	// LogicalOr represents a logical OR (at least one condition must be true).
	LogicalOr
	// LogicalNot represents a logical NOT (the condition must be false).
	LogicalNot
	// LogicalNor represents a logical NOR (all conditions must be false).
	LogicalNor
	// LogicalXor represents a logical XOR (exactly one condition must be true).
	LogicalXor
	// LogicalNand represents a logical NAND (not all conditions can be true).
	LogicalNand
	// LogicalXnor represents a logical XNOR (an even number of conditions must be true).
	LogicalXnor
)

var logicalToString = map[LogicalOperator]string{
	LogicalAnd:  "and",
	LogicalOr:   "or",
	LogicalNot:  "not",
	LogicalNor:  "nor",
	LogicalXor:  "xor",
	LogicalNand: "nand",
	LogicalXnor: "xnor",
}

var stringToLogical = map[string]LogicalOperator{
	"and":  LogicalAnd,
	"or":   LogicalOr,
	"not":  LogicalNot,
	"nor":  LogicalNor,
	"xor":  LogicalXor,
	"nand": LogicalNand,
	"xnor": LogicalXnor,
}

func (o LogicalOperator) String() string {
	if s, ok := logicalToString[o]; ok {
		return s
	}
	return ""
}

func (o LogicalOperator) MarshalJSON() ([]byte, error) {
	val, err := json.Marshal(o.String())
	if err != nil {
		return nil, ErrMarshalFailed.WithCause(err)
	}
	return val, nil
}

func (o *LogicalOperator) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return ErrUnmarshalFailed.WithCause(err)
	}
	if val, ok := stringToLogical[s]; ok {
		*o = val
		return nil
	}
	return ErrInvalidOperator.WithMessage(fmt.Sprintf("invalid logical operator: %s", s))
}


// Evaluate evaluates a logical operator against a slice of boolean results.
func (o LogicalOperator) Evaluate(conditions []bool) (bool, error) {
	if len(conditions) == 0 {
		return false, ErrEmptConditions
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
		return false, ErrInvalidOperator.WithMessage("unknown logical operator")
	}
}
