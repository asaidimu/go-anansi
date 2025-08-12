package query

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// From creates a new Query instance from JSON data.
// It accepts either a JSON string or byte slice and returns a populated Query struct.
// This method leverages all the custom UnmarshalJSON implementations for proper deserialization.
//
// Parameters:
//   - data: Can be either string or []byte containing valid JSON
//
// Returns:
//   - *Query: A new Query instance populated from the JSON data
//   - error: Any error encountered during parsing or validation
//
// Example usage:
//   query, err := query.From(`{"filters": {"field": "name", "operator": "eq", "value": "John"}}`)
//   if err != nil {
//       log.Fatal(err)
//   }
func From(data any) (*Query, error) {
	if data == nil {
		return nil, fmt.Errorf("input data cannot be nil")
	}

	var jsonBytes []byte
	var err error

	// Handle different input types
	switch v := data.(type) {
	case string:
		if v == "" {
			return &Query{}, nil // Return empty query for empty string
		}
		jsonBytes = []byte(v)
	case []byte:
		if len(v) == 0 {
			return &Query{}, nil // Return empty query for empty byte slice
		}
		jsonBytes = v
	default:
		return nil, fmt.Errorf("unsupported input type: %T, expected string or []byte", data)
	}

	// Check for null JSON
	if string(jsonBytes) == "null" {
		return &Query{}, nil
	}

	// Create a new Query instance
	var query Query

	// Unmarshal the JSON data using the custom UnmarshalJSON methods
	if err = utils.FromJSON(jsonBytes, &query); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON into Query: %w", err)
	}

	return &query, nil
}

// FromString is a convenience method that specifically handles string input.
// It's a wrapper around From() for better type safety when you know you're working with strings.
func FromString(jsonStr string) (*Query, error) {
	return From(jsonStr)
}

// FromBytes is a convenience method that specifically handles byte slice input.
// It's a wrapper around From() for better type safety when you know you're working with byte slices.
func FromBytes(jsonBytes []byte) (*Query, error) {
	return From(jsonBytes)
}

// MustFrom is like From but panics if there's an error.
// This is useful in scenarios where you're confident the JSON is valid (e.g., hardcoded queries).
func MustFrom(data any) *Query {
	query, err := From(data)
	if err != nil {
		panic(fmt.Sprintf("MustFrom failed: %v", err))
	}
	return query
}

// Validate performs basic validation on the Query struct after deserialization.
// This method can be called after From() to ensure the query is logically consistent.
func (q *Query) Validate() error {
	// Validate pagination if present
	if q.Pagination != nil {
		if q.Pagination.Limit < 0 {
			return fmt.Errorf("pagination limit cannot be negative: %d", q.Pagination.Limit)
		}
		if q.Pagination.Offset != nil && *q.Pagination.Offset < 0 {
			return fmt.Errorf("pagination offset cannot be negative: %d", *q.Pagination.Offset)
		}
	}

	// Validate aggregations if present
	for i, agg := range q.Aggregations {
		if agg.Field == "" && agg.Type != AggregationTypeCount {
			return fmt.Errorf("aggregation at index %d: field is required for %s aggregation", i, agg.Type)
		}
	}

	// Validate joins if present
	for i, join := range q.Joins {
		if join.Target.Name == "" {
			return fmt.Errorf("join at index %d: target name is required", i)
		}
	}

	// Validate sort configurations if present
	for i, sort := range q.Sort {
		if sort.Field == "" {
			return fmt.Errorf("sort at index %d: field is required", i)
		}
		if sort.Direction != SortDirectionAsc && sort.Direction != SortDirectionDesc {
			return fmt.Errorf("sort at index %d: invalid direction '%s', must be 'asc' or 'desc'", i, sort.Direction)
		}
	}

	return nil
}

// Clone creates a deep copy of the Query struct.
// This is useful when you want to modify a query without affecting the original.
func (q *Query) Clone() (*Query, error) {
	var newQuery Query
	if err := utils.Clone(*q, &newQuery); err != nil {
		return nil, fmt.Errorf("failed to clone query: %w", err)
	}
	return &newQuery, nil
}

// MustClone is like Clone but panics if there's an error.
func (q *Query) MustClone() *Query {
	clone, err := q.Clone()
	if err != nil {
		panic(fmt.Sprintf("MustClone failed: %v", err))
	}
	return clone
}

// ToJSON serializes the Query to a JSON string.
// This is the inverse operation of From().
func (q *Query) ToJSON() (string, error) {
	return utils.ToJSON(q)
}

// ToJSONBytes serializes the Query to JSON bytes.
func (q *Query) ToJSONBytes() ([]byte, error) {
	return utils.ToJSONBytes(q)
}

// MustToJSON is like ToJSON but panics if there's an error.
func (q *Query) MustToJSON() string {
	jsonStr, err := q.ToJSON()
	if err != nil {
		panic(fmt.Sprintf("MustToJSON failed: %v", err))
	}
	return jsonStr
}

// Merge combines this query with another query, with the other query taking precedence.
// This is useful for applying default queries or overriding specific parts of a query.
func (q *Query) Merge(other *Query) *Query {
	if other == nil {
		return q
	}

	merged := &Query{}

	// Use reflection to merge fields, giving precedence to the 'other' query
	qVal := reflect.ValueOf(q).Elem()
	otherVal := reflect.ValueOf(other).Elem()
	mergedVal := reflect.ValueOf(merged).Elem()

	for i := 0; i < qVal.NumField(); i++ {
		qField := qVal.Field(i)
		otherField := otherVal.Field(i)
		mergedField := mergedVal.Field(i)

		if !mergedField.CanSet() {
			continue
		}

		// If other field is not nil/zero, use it; otherwise use the current field
		if !otherField.IsZero() {
			mergedField.Set(otherField)
		} else {
			mergedField.Set(qField)
		}
	}

	return merged
}
