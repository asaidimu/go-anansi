package utils

/** Logical operators for combining conditions. */
type LogicalOperator string

// Logical operators for combining filter conditions.
const (
	LogicalOperatorAnd LogicalOperator = "and"
	LogicalOperatorOr  LogicalOperator = "or"
	LogicalOperatorNot LogicalOperator = "not"
	LogicalOperatorNor LogicalOperator = "nor"
	LogicalOperatorXor LogicalOperator = "xor"
)

