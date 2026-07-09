package definition

import (
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/common"
)

// IndexID represents the id of an index.
type IndexID string

type IndexConditionKind byte

const (
	IndexConditionKindSingle IndexConditionKind = iota + 1
	IndexConditionKindGroup
)

type Index struct {
	Name        string              `json:"name"` // should be IndexName but I got lazy
	Description string              `json:"description,omitempty"`
	Order       string              `json:"order,omitempty"`
	Condition   IndexConditionUnion `json:"condition"`
	Fields      []FieldName         `json:"fields"`
	Type        IndexType           `json:"type"`
	Unique      bool                `json:"unique,omitempty"`
}

func (i Index) MarshalJSON() ([]byte, error) {
	type Alias Index
	proxy := struct {
		Alias
		Condition *IndexConditionUnion `json:"condition,omitempty"`
	}{
		Alias: Alias(i),
	}

	if !i.Condition.IsZero() {
		proxy.Condition = &i.Condition
	}

	return json.Marshal(proxy)
}

type IndexCondition struct {
	Field    FieldName                 `json:"field"`
	Value    LiteralValue              `json:"value"`
	Operator common.ComparisonOperator `json:"operator"`
}

type IndexConditionGroup struct {
	Conditions []IndexConditionUnion  `json:"conditions,omitempty"`
	Operator   common.LogicalOperator `json:"operator"`
}

// IndexConditionUnion embeds the pointers.
type IndexConditionUnion struct {
	kind    IndexConditionKind
	payload any // *IndexCondition or *IndexConditionGroup
}

// IsCondition returns true if this union holds a single condition
func (icu IndexConditionUnion) IsCondition() bool {
	return icu.kind == IndexConditionKindSingle
}
func (icu *IndexConditionUnion) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	var checker struct {
		Field      string `json:"field"`
		Conditions any    `json:"conditions"`
	}
	if err := json.Unmarshal(data, &checker); err != nil {
		return ErrUnmarshalFailed.WithCause(err).WithOperation("IndexConditionUnion.UnmarshalJSON")
	}

	if checker.Conditions != nil {
		var group IndexConditionGroup
		if err := json.Unmarshal(data, &group); err != nil {
			return ErrUnmarshalFailed.WithOperation("IndexConditionUnion.UnmarshalJSON").WithCause(err)
		}
		icu.kind = IndexConditionKindGroup
		icu.payload = &group
	} else if checker.Field != "" {
		var cond IndexCondition
		if err := json.Unmarshal(data, &cond); err != nil {
			return ErrUnmarshalFailed.WithOperation("IndexConditionUnion.UnmarshalJSON").WithCause(err)
		}
		icu.kind = IndexConditionKindSingle
		icu.payload = &cond
	} else {
		return ErrUnmarshalFailed.WithOperation("IndexConditionUnion.UnmarshalJSON").WithMessage("invalid IndexConditionUnion: missing 'field' or 'conditions'")
	}
	return nil
}

func (icu IndexConditionUnion) MarshalJSON() ([]byte, error) {
	if icu.payload == nil {
		return []byte("null"), nil
	}

	val, err := json.Marshal(icu.payload)
	if err != nil {
		return nil, ErrMarshalFailed.WithCause(err).WithOperation("IndexConditionUnion.MarshalJSON")
	}
	return val, nil
}

// IsConditionGroup returns true if this union holds a condition group
func (icu IndexConditionUnion) IsConditionGroup() bool {
	return icu.kind == IndexConditionKindGroup
}

// IsZero returns true if neither is set
func (icu IndexConditionUnion) IsZero() bool {
	return icu.payload == nil
}

type IndexConditionType interface {
	*IndexCondition | *IndexConditionGroup
}

func IndexConditionAs[T IndexConditionType](icu IndexConditionUnion) (T, error) {
	var zero T
	val, ok := icu.payload.(T)
	if !ok {
		return zero, ErrTypeMismatch.WithMessage(
			fmt.Sprintf("type mismatch: requested %T but union contains kind %d", zero, icu.kind),
		)
	}
	return val, nil
}

func (icu IndexConditionUnion) Value() any {
	return icu.payload
}

func NewIndexConditionUnion[T IndexConditionType](payload T) IndexConditionUnion {
	icu := IndexConditionUnion{
		payload: payload,
	}

	// Internal type switch to set the correct kind automatically
	switch any(payload).(type) {
	case *IndexCondition:
		icu.kind = IndexConditionKindSingle
	case *IndexConditionGroup:
		icu.kind = IndexConditionKindGroup
	}

	return icu
}
