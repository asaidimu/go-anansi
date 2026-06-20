package query

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

// MarshalJSON implements the json.Marshaler interface for QueryDistinctConfig.
// This ensures that when marshaling, it correctly outputs either a boolean or an object.
func (qdc QueryDistinctConfig) MarshalJSON() ([]byte, error) {
	if qdc.IsDistinct != nil && *qdc.IsDistinct { // Only marshal as true if IsDistinct is true
		return json.Marshal(true)
	}
	if qdc.Fields != nil { // Only marshal fields if not nil and not empty
		// Marshal as an object with a "fields" key
		return json.Marshal(struct {
			Fields []string `json:"fields"`
		}{
			Fields: qdc.Fields,
		})
	}
	return []byte("null"), nil // If neither is set, or if IsDistinct is false, or fields is empty, marshal as null
}

// UnmarshalJSON implements the json.Unmarshaler interface for QueryDistinctConfig.
// This ensures that when unmarshaling, it correctly parses either a boolean or an object.
func (qdc *QueryDistinctConfig) UnmarshalJSON(data []byte) error {
	// Handle null or empty input explicitly
	if string(data) == "null" || string(data) == "" || string(data) == "{}" {
		qdc.IsDistinct = nil
		qdc.Fields = nil
		return nil
	}

	// Try unmarshaling as a boolean first
	var boolVal bool
	if err := json.Unmarshal(data, &boolVal); err == nil {
		qdc.IsDistinct = &boolVal
		return nil
	}

	// If it's not a boolean, try unmarshaling as an object with "fields"
	var fieldConfig struct {
		Fields []string `json:"fields"`
	}
	if err := json.Unmarshal(data, &fieldConfig); err == nil {
		// Check if the JSON explicitly contained the "fields" key
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err == nil {
			if _, ok := raw["fields"]; ok { // If "fields" key was present
				if len(fieldConfig.Fields) == 0 {
					qdc.Fields = nil // If it's an empty array, make it nil to match test expectation
				} else {
					qdc.Fields = fieldConfig.Fields
				}
				return nil
			}
		}
	}

	return common.NewSystemError("ERR_QUERY_INVALID_DISTINCT_CONFIG_UNMARSHAL", fmt.Sprintf("invalid QueryDistinctConfig: expected boolean or object with 'fields' array, got %s", string(data))).WithOperation("UnmarshalJSON").WithCause(errors.New("invalid QueryDistinctConfig"))
}

// UnmarshalJSON implements the json.Unmarshaler interface for QueryFilter.
// It attempts to unmarshal the JSON data into one of its possible types:
// FilterCondition, FilterGroup, or TextSearchQuery, based on the presence
// of discriminating fields, including the "condition" wrapper if applicable.
func (qf *QueryFilter) UnmarshalJSON(data []byte) error {
	// Handle null or empty input gracefully for optional fields
	if string(data) == "null" || string(data) == "" {
		return nil
	}

	// Use an anonymous map to peek at the raw JSON data to identify the type
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return common.NewSystemError("ERR_QUERY_INVALID_FILTER_UNMARSHAL_RAW", "invalid QueryFilter: failed to unmarshal into raw map").WithOperation("UnmarshalJSON").WithCause(err)
	}

	// Priority 1: Check for "condition" wrapper, which indicates a FilterCondition
	if conditionRaw, ok := raw["condition"]; ok {
		var condition FilterCondition
		if err := json.Unmarshal(conditionRaw, &condition); err == nil {
			qf.Condition = &condition
			return nil
		}
		// If there's a "condition" key but it failed to unmarshal, it's an error.
		return common.NewSystemError("ERR_QUERY_INVALID_FILTER_UNMARSHAL_CONDITION", "invalid QueryFilter: failed to unmarshal nested FilterCondition").WithOperation("UnmarshalJSON").WithCause(errors.New("failed to unmarshal nested FilterCondition"))
	}

	// Priority 2: Attempt to unmarshal as FilterGroup (has "conditions" field)
	if _, conditionsOk := raw["conditions"]; conditionsOk {
		var group FilterGroup
		if err := json.Unmarshal(data, &group); err == nil {
			qf.Group = &group
			return nil
		}
	}

	// Priority 3: Attempt to unmarshal as TextSearchQuery (has "query" field)
	if _, queryOk := raw["query"]; queryOk {
		var textSearch TextSearchQuery
		if err := json.Unmarshal(data, &textSearch); err == nil {
			if textSearch.Query != "" { // Ensure the 'query' field is actually populated
				qf.TextSearchQuery = &textSearch
				return nil
			}
		}
	}

	// Priority 4: If no specific wrapper or group/text search, try direct FilterCondition
	// This should be the fallback if the data is a flat FilterCondition JSON object.
	if _, fieldOk := raw["field"]; fieldOk {
		if _, opOk := raw["operator"]; opOk {
			if _, valOk := raw["value"]; valOk {
				var condition FilterCondition
				if err := json.Unmarshal(data, &condition); err == nil {
					qf.Condition = &condition
					return nil
				}
			}
		}
	}

	// If none of the above types matched, then it's an invalid structure.
	return common.NewSystemError("ERR_QUERY_INVALID_FILTER_STRUCTURE", fmt.Sprintf("invalid QueryFilter: data does not match FilterCondition, FilterGroup, TextSearchQuery, or wrapped condition: %s", string(data))).WithOperation("UnmarshalJSON").WithCause(errors.New("invalid QueryFilter structure"))
}

// MarshalJSON implements the json.Marshaler interface for QueryFilter.
// It marshals only the populated field that is currently set.
// If it's a FilterCondition, it will be wrapped in a "condition" key as per test expectations.
func (qf QueryFilter) MarshalJSON() ([]byte, error) {
	if qf.Condition != nil {
		return json.Marshal(qf.Condition)
	}
	if qf.Group != nil {
		return json.Marshal(qf.Group) // Marshal FilterGroup directly as a flat JSON object
	}
	if qf.TextSearchQuery != nil {
		return json.Marshal(qf.TextSearchQuery) // Marshal TextSearchQuery directly as a flat JSON object
	}
	return []byte("null"), nil // If no field is set, marshal as JSON null
}

// UnmarshalJSON implements the json.Unmarshaler interface for FilterValue.
// It attempts to unmarshal the JSON data into one of its possible underlying types.
func (fv *FilterValue) UnmarshalJSON(data []byte) error {
	// Handle null explicitly
	if string(data) == "null" || string(data) == "" {
		return nil
	}

	// 1. Try unmarshaling as primitive types directly (e.g., "foo", 123, true)
	// Order matters: string, then bool, then number, then object.
	// Object should be last as it's the most generic and could consume other types if placed first.
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		fv.StringVal = &str
		return nil
	}

	var boolean bool
	if err := json.Unmarshal(data, &boolean); err == nil {
		fv.BoolVal = &boolean
		return nil
	}

	var num float64 // Using float64 to cover both integers and floats
	if err := json.Unmarshal(data, &num); err == nil {
		fv.NumberVal = &num
		return nil
	}

	// 2. Try unmarshaling as an array of FilterValue (direct array)
	// This must come before trying a generic object, as arrays are objects in JSON.
	var arr []FilterValue
	if err := json.Unmarshal(data, &arr); err == nil {
		fv.ArrayVal = arr
		return nil
	}

	// 3. Try unmarshaling as a generic JSON object (map[string]any) for object_value or other complex types
	var obj map[string]json.RawMessage // Use RawMessage to peek without full unmarshal
	if err := json.Unmarshal(data, &obj); err == nil {
		// Check for specific type discriminators first
		if typeVal, ok := obj["type"]; ok {
			var typeStr string
			if err := json.Unmarshal(typeVal, &typeStr); err == nil {
				switch typeStr {
				case "field":
					var fieldRef FieldReference
					if err := json.Unmarshal(data, &fieldRef); err == nil {
						fv.FieldRefVal = &fieldRef
						return nil
					}
				case "subquery":
					var subqueryVal SubqueryValue
					if err := json.Unmarshal(data, &subqueryVal); err == nil {
						fv.SubqueryVal = &subqueryVal
						return nil
					}
				}
			}
		}

		// Check for "function" for FunctionCall
		if _, ok := obj["function"]; ok {
			var funcCall FunctionCall
			if err := json.Unmarshal(data, &funcCall); err == nil {
				fv.FunctionCallVal = &funcCall
				return nil
			}
		}

		// If it's an object but not any of the special types, unmarshal as a generic object
		var genericObj map[string]any
		if err := json.Unmarshal(data, &genericObj); err == nil {
			fv.ObjectVal = genericObj
			return nil
		}
	}

	// If none of the above succeeded, the data is not a recognized FilterValue type.
	return common.NewSystemError("ERR_QUERY_UNSUPPORTED_FILTER_VALUE_TYPE", fmt.Sprintf("unsupported FilterValue type or invalid JSON: %s", string(data))).WithOperation("UnmarshalJSON").WithCause(errors.New("unsupported FilterValue type or invalid JSON"))
}

// MarshalJSON implements the json.Marshaler interface for FilterValue.
// It marshals only the single non-nil underlying value, matching the TypeScript union output.
// It explicitly marshals to the expected JSON field names (e.g., "string_value").
func (fv FilterValue) MarshalJSON() ([]byte, error) {
	if fv.StringVal != nil {
		return json.Marshal(*fv.StringVal) // Marshal the string directly
	}
	if fv.NumberVal != nil {
		return json.Marshal(*fv.NumberVal) // Marshal the number directly
	}
	if fv.BoolVal != nil {
		return json.Marshal(*fv.BoolVal) // Marshal the boolean directly
	}
	if fv.ObjectVal != nil {
		return json.Marshal(fv.ObjectVal) // Marshal the object directly
	}
	if fv.ArrayVal != nil {
		return json.Marshal(fv.ArrayVal) // Marshal the array directly
	}
	if fv.FieldRefVal != nil {
		return json.Marshal(fv.FieldRefVal)
	}
	if fv.SubqueryVal != nil {
		return json.Marshal(fv.SubqueryVal)
	}
	if fv.FunctionCallVal != nil {
		return json.Marshal(fv.FunctionCallVal)
	}
	return []byte("null"), nil
}

// MarshalJSON customizes the JSON marshalling for ProjectionComputedItem.
func (p ProjectionComputedItem) MarshalJSON() ([]byte, error) {
	if p.ComputedFieldExpression != nil {
		return json.Marshal(p.ComputedFieldExpression)
	}
	if p.CaseExpression != nil {
		return json.Marshal(p.CaseExpression)
	}
	return []byte("{}"), nil // Return an empty object if neither is set
}

// UnmarshalJSON customizes the JSON unmarshalling for ProjectionComputedItem.
func (p *ProjectionComputedItem) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	// Try to determine the actual type based on the 'type' field within the JSON.
	var itemType string
	if typeVal, ok := raw["type"]; ok {
		if err := json.Unmarshal(typeVal, &itemType); err != nil {
			return common.NewSystemError("ERR_QUERY_PROJECTION_COMPUTED_ITEM_UNMARSHAL_TYPE", "failed to unmarshal 'type' field in ProjectionComputedItem").WithOperation("UnmarshalJSON").WithCause(err)
		}
	} else {
		// If 'type' field is missing, it's an invalid ProjectionComputedItem
		return common.NewSystemError("ERR_QUERY_PROJECTION_COMPUTED_ITEM_MISSING_TYPE", "missing 'type' field for ProjectionComputedItem").WithOperation("UnmarshalJSON").WithCause(errors.New("missing 'type' field"))
	}

	switch itemType {
	case "computed":
		var cfe ComputedFieldExpression
		if err := json.Unmarshal(b, &cfe); err != nil {
			return common.NewSystemError("ERR_QUERY_PROJECTION_COMPUTED_ITEM_UNMARSHAL_COMPUTED", "failed to unmarshal as ComputedFieldExpression").WithOperation("UnmarshalJSON").WithCause(err)
		}
		p.ComputedFieldExpression = &cfe
	case "case":
		var ce CaseExpression
		if err := json.Unmarshal(b, &ce); err != nil {
			return common.NewSystemError("ERR_QUERY_PROJECTION_COMPUTED_ITEM_UNMARSHAL_CASE", "failed to unmarshal as CaseExpression").WithOperation("UnmarshalJSON").WithCause(err)
		}
		p.CaseExpression = &ce
	default:
		return common.NewSystemError("ERR_QUERY_PROJECTION_COMPUTED_ITEM_UNKNOWN_TYPE", fmt.Sprintf("unknown 'type' field for ProjectionComputedItem: %s", itemType)).WithOperation("UnmarshalJSON").WithCause(errors.New("unknown 'type' field"))
	}

	return nil
}

// MarshalJSON customizes the JSON marshalling for PaginationOptions.
func (p PaginationOptions) MarshalJSON() ([]byte, error) {
	// Use an anonymous struct to control which fields are marshalled based on the 'Type'.
	// This ensures only relevant fields for the specific pagination type are included.
	switch p.Type {
	case "offset":
		// For "offset" type, only include Type, Limit, and Offset.
		aux := struct {
			Type   PaginationType `json:"type"`
			Limit  int            `json:"limit"`
			Offset *int           `json:"offset,omitempty"`
		}{
			Type:   p.Type,
			Limit:  p.Limit,
			Offset: p.Offset,
		}
		return json.Marshal(aux)
	default:
		// Handle unknown or unsupported pagination types.
		return nil, common.NewSystemError("ERR_QUERY_UNKNOWN_PAGINATION_TYPE_MARSHAL", fmt.Sprintf("unknown pagination type: %s", p.Type)).WithOperation("MarshalJSON").WithCause(errors.New("unknown pagination type"))
	}
}

// UnmarshalJSON customizes the JSON unmarshalling for PaginationOptions.
func (p *PaginationOptions) UnmarshalJSON(b []byte) error {
	// First, unmarshal enough to get the 'type' field.
	var raw struct {
		Type PaginationType `json:"type"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	p.Type = raw.Type // Set the type in the main struct

	// Then, unmarshal based on the detected type.
	switch p.Type {
	case "offset":
		var aux struct {
			Limit  int  `json:"limit"`
			Offset *int `json:"offset,omitempty"`
		}
		if err := json.Unmarshal(b, &aux); err != nil {
			return common.NewSystemError("ERR_QUERY_PAGINATION_UNMARSHAL_OFFSET_FAILED", "failed to unmarshal offset pagination options").WithOperation("UnmarshalJSON").WithCause(err)
		}
		p.Limit = aux.Limit
		p.Offset = aux.Offset
	default:
		return common.NewSystemError("ERR_QUERY_UNKNOWN_PAGINATION_TYPE_UNMARSHAL", fmt.Sprintf("unknown or missing pagination type '%s' in JSON", p.Type)).WithOperation("UnmarshalJSON").WithCause(errors.New("unknown or missing pagination type"))
	}

	return nil
}