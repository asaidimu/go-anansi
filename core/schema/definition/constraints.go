package definition

import (
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

// PredicateParams is a generic struct that holds the parameters for a predicate function.
type PredicateParams struct {
	Data       any          `json:"data"`       // The data being validated.
	Keys       []FieldName  `json:"keys"`       // The specific field being validated.
	Parameters LiteralValue `json:"parameters"` // Any extra arguments for the predicate.
}

// Predicate defines a function for data validation.
type Predicate func(params PredicateParams) []common.Issue

// PredicateName represents the name of a supported predicate.
type PredicateName string

// PredicateMap is a map of predicate names to their validation functions.
type PredicateMap map[PredicateName]Predicate

// Constraint defines a validation rule for fields in a schema
type ConstraintRule struct {
	Fields     []FieldName   `json:"fields,omitempty"`
	Predicate  PredicateName `json:"predicate"`
	Parameters LiteralValue  `json:"parameters"`
}

// ConstraintGroup defines a group of multiple constraints with a logical operator.
type ConstraintGroup struct {
	Rules    []ConstraintUnion      `json:"rules"`
	Operator common.LogicalOperator `json:"operator"`
}

type ConstraintKind byte

const (
	ConstraintKindRule ConstraintKind = iota + 1
	ConstraintKindGroup
)

type ConstraintUnion struct {
	kind    ConstraintKind `json:"-"`
	payload any            `json:"-"`
}

// ConstraintRule represents a discriminated union of Constraint, ConstraintGroup, or ResourceReference.
// Exactly one field should be non-nil at any time.
type Constraint struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ConstraintUnion
}

func (c *Constraint) UnmarshalJSON(data []byte) error {
	var meta struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}

	if err := json.Unmarshal(data, &meta); err != nil {
		return ErrUnmarshalFailed.WithCause(err).WithOperation("Constraint.UnmarshalJSON (base fields)")
	}
	var union ConstraintUnion
	if err := json.Unmarshal(data, &union); err != nil {
		return ErrUnmarshalFailed.WithCause(err).WithOperation("Constraint.UnmarshalJSON (union part)")
	}

	c.Name = meta.Name
	c.Description = meta.Description
	c.ConstraintUnion = union
	return nil
}

func (cu *ConstraintUnion) UnmarshalJSON(data []byte) error {
	var checker struct {
		Operator  common.LogicalOperator `json:"operator"`
		Predicate string                 `json:"predicate"`
	}
	if err := json.Unmarshal(data, &checker); err != nil {
		return ErrUnmarshalFailed.WithCause(err).WithOperation("ConstraintUnion.UnmarshalJSON")
	}

	// 2. Decide and Unmarshal exactly once, enforcing mutual exclusivity
	hasOperator := checker.Operator != 0
	hasPredicate := checker.Predicate != ""

	if hasOperator && hasPredicate {
		return ErrInvalidContraint.WithMessage("constraint cannot have both 'operator' and 'predicate'").WithOperation("ConstraintUnion.UnmarshalJSON")
	}

	if hasOperator {
		var g ConstraintGroup
		if err := json.Unmarshal(data, &g); err != nil {
			return ErrUnmarshalFailed.WithCause(err).WithOperation("ConstraintUnion.UnmarshalJSON")
		}
		cu.kind = ConstraintKindGroup
		cu.payload = &g
		return nil
	}

	if hasPredicate {
		var r ConstraintRule
		if err := json.Unmarshal(data, &r); err != nil {
			return ErrUnmarshalFailed.WithCause(err).WithOperation("ConstraintUnion.UnmarshalJSON")
		}
		cu.kind = ConstraintKindRule
		cu.payload = &r
		return nil
	}

	return ErrInvalidContraint.WithMessage("constraint must have either 'operator' or 'predicate'").WithOperation("ConstraintUnion.UnmarshalJSON")
}

func (c Constraint) MarshalJSON() ([]byte, error) {
	type Meta struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	meta := Meta{
		Name:        c.Name,
		Description: c.Description,
	}

	switch c.kind {
	case ConstraintKindRule:
		rule := c.payload.(*ConstraintRule)

		// Create a flat proxy that combines Meta and Rule fields
		// and overrides Parameters with a pointer for omitempty support.
		return json.Marshal(struct {
			Meta
			Fields     []FieldName   `json:"fields,omitempty"`
			Predicate  PredicateName `json:"predicate"`
			Parameters *LiteralValue `json:"parameters,omitempty"`
		}{
			Meta:      meta,
			Fields:    rule.Fields,
			Predicate: rule.Predicate,
			// Apply manual "omit empty" logic using your fixed IsZero()
			Parameters: func() *LiteralValue {
				if rule.Parameters.IsZero() || rule.Parameters.IsNull() {
					return nil
				}
				return &rule.Parameters
			}(),
		})

	case ConstraintKindGroup:
		return json.Marshal(struct {
			Meta
			*ConstraintGroup
		}{
			Meta:            meta,
			ConstraintGroup: c.payload.(*ConstraintGroup),
		})

	default:
		return nil, ErrInvalidContraint.WithMessage("constraint must have either rule or group").WithOperation("Constraint.MarshalJSON")
	}
}

func (cu ConstraintUnion) MarshalJSON() ([]byte, error) {
	if cu.payload == nil {
		return []byte("null"), nil
	}

	switch cu.kind {
	case ConstraintKindRule:
		rule := cu.payload.(*ConstraintRule)
		// We use a flat proxy here just like we did in Constraint.MarshalJSON
		// to ensure Parameters is omitted if it is Zero or Null.
		return json.Marshal(struct {
			Fields     []FieldName     `json:"fields,omitempty"`
			Predicate  PredicateName `json:"predicate"`
			Parameters *LiteralValue `json:"parameters,omitempty"`
		}{
			Fields:    rule.Fields,
			Predicate: rule.Predicate,
			Parameters: func() *LiteralValue {
				if rule.Parameters.IsZero() || rule.Parameters.IsNull() {
					return nil
				}
				return &rule.Parameters
			}(),
		})

	default:
		// For ConstraintGroup or other types, standard marshaling is fine
		return json.Marshal(cu.payload)
	}
}

// Kind returns the discriminator for the union.
func (cu ConstraintUnion) Kind() ConstraintKind {
	return cu.kind
}

// Value returns the underlying payload (either *ConstraintRule or *ConstraintGroup).
func (cu ConstraintUnion) Value() any {
	return cu.payload
}

// ConstraintUnionType is a type constraint for the generic accessor.
type ConstraintUnionType interface {
	*ConstraintRule | *ConstraintGroup
}

// ConstraintAs attempts to extract the value as the specified type T.
// It provides a type-safe way to access the union without manual type assertions.
func ConstraintAs[T ConstraintUnionType](cu ConstraintUnion) (T, error) {
	var zero T

	val, ok := cu.payload.(T)
	if !ok {
		return zero, ErrTypeMismatch.WithMessage(fmt.Sprintf("type mismatch: requested %T but union contains kind %d", zero, cu.kind))
	}

	return val, nil
}

func NewConstrainUnion[T ConstraintUnionType](payload T) ConstraintUnion {
	if payload == nil {
		return ConstraintUnion{}
	}

	r := ConstraintUnion{
		payload: payload,
	}
	switch any(r.payload).(type) {
	case *ConstraintRule:
		r.kind = ConstraintKindRule
	case *ConstraintGroup:
		r.kind = ConstraintKindGroup
	}
	return r
}
