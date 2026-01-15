package common

import (
	"encoding/json"
	"fmt"
)

// ComparisonOperator defines the set of operators used in a filter or index condition.
type ComparisonOperator byte

const (
	Equal ComparisonOperator = iota + 1
	NotEqual
	LessThan
	LessThanOrEqual
	GreaterThan
	GreaterThanOrEqual
	In
	NotIn
	Contains
	NotContains
	NotExists
	Exists
)

var compToString = map[ComparisonOperator]string{
	Equal:              "eq",
	NotEqual:           "neq",
	LessThan:           "lt",
	LessThanOrEqual:    "lte",
	GreaterThan:        "gt",
	GreaterThanOrEqual: "gte",
	In:                 "in",
	NotIn:              "nin",
	Contains:           "contains",
	NotContains:        "ncontains",
	Exists:             "exists",
	NotExists:          "nexists",
}

var stringToComp = map[string]ComparisonOperator{
	"eq":        Equal,
	"neq":       NotEqual,
	"lt":        LessThan,
	"lte":       LessThanOrEqual,
	"gt":        GreaterThan,
	"gte":       GreaterThanOrEqual,
	"in":        In,
	"nin":       NotIn,
	"contains":  Contains,
	"ncontains": NotContains,
	"exists":    Exists,
	"nexists":   NotExists,
}

func (o ComparisonOperator) String() string {
	if s, ok := compToString[o]; ok {
		return s
	}
	return ""
}

func (o ComparisonOperator) MarshalJSON() ([]byte, error) {
	val, err := json.Marshal(o.String())
	if err != nil {
		return nil, ErrMarshalFailed.WithCause(err)
	}
	return val, nil
}

func (o *ComparisonOperator) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if val, ok := stringToComp[s]; ok {
		*o = val
		return nil
	}
	return ErrInvalidOperator.WithMessage(fmt.Sprintf("invalid comparison operator: %s", s))
}
