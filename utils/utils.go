package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// StructToMap converts a Go struct into a map[string]any.
//
// This function first marshals the input struct to JSON bytes and then
// unmarshals those bytes into a temporary `map[string]any`. In a subsequent
// pass, any values in this temporary map that are themselves `map[string]any`
// (representing nested JSON objects, typically originating from nested structs)
// are re-marshaled into `json.RawMessage`. This ensures their exact raw JSON
// representation is preserved within the final `map[string]any`.
//
// The input `record` must be a struct or a pointer to a struct. If `record` is
// nil, or not a struct/pointer to a struct, an error is returned.
//
// Example:
//
//	type InnerStruct struct {
//		Detail string `json:"detail"`
//	}
//	type MyStruct struct {
//		ID    string      `json:"id"`
//		Nested InnerStruct `json:"nested_data"`
//	}
//	myInstance := MyStruct{ID: "abc-123", Nested: InnerStruct{Detail: "some info"}}
//	convertedMap, err := StructToMap(myInstance)
//	// convertedMap will be map[string]any{"id": "abc-123", "nested_data": json.RawMessage(`{"detail":"some info"}`)}
func StructToMap[T any](record T) (map[string]any, error) {
	val := reflect.ValueOf(record)

	// Handle nil interface input directly (e.g., if `record` is `nil any`)
	if !val.IsValid() {
		return nil, fmt.Errorf("input record cannot be nil")
	}

	// If the input is a pointer, dereference it to get the underlying value
	if val.Kind() == reflect.Ptr {
		// If it's a nil pointer, return an error
		if val.IsNil() {
			return nil, fmt.Errorf("input record cannot be a nil pointer to a struct")
		}
		val = val.Elem()
	}

	// Validate that the underlying value is a struct
	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("input record must be a struct or a pointer to a struct, got %s", val.Kind())
	}

	// Step 1: Marshal the input struct into JSON bytes.
	// This respects `json:"tag"` annotations, `omitempty`, etc.,
	// and correctly serializes all nested structs into their JSON object forms.
	jsonBytes, err := json.Marshal(record)
	if err != nil {
		// Wrap the error to provide context specific to this function's operation
		return nil, fmt.Errorf("StructToMap: failed to marshal input record to JSON: %w", err)
	}

	// Step 2: Unmarshal these JSON bytes into a temporary map[string]any.
	// At this stage, `encoding/json` will convert nested JSON objects into
	// `map[string]any` and JSON arrays into `[]any`.
	var tempMap map[string]any
	if err := json.Unmarshal(jsonBytes, &tempMap); err != nil {
		// Wrap the error to indicate the point of failure
		return nil, fmt.Errorf("StructToMap: failed to unmarshal JSON to temporary map[string]any: %w", err)
	}

	// Step 3: Iterate through the temporary map and transform any nested maps
	// (which represent original nested structs) into `json.RawMessage`.
	// Pre-allocate map capacity for better performance.
	resultMap := make(map[string]any, len(tempMap))
	for key, val := range tempMap {
		// Check if the current value is a nested map[string]any
		if nestedMap, ok := val.(map[string]any); ok {
			// If it's a nested map, re-marshal it to get its raw JSON bytes.
			// This preserves the exact original JSON structure of the nested object.
			nestedBytes, err := json.Marshal(nestedMap)
			if err != nil {
				// Wrap the error, including the key that caused the issue for debugging
				return nil, fmt.Errorf("StructToMap: error re-marshaling nested map for key '%s': %w", key, err)
			}
			// Assign the raw JSON bytes as `json.RawMessage` to the result map
			resultMap[key] = json.RawMessage(nestedBytes)
		} else {
			// For all other types (e.g., strings, numbers, booleans, arrays, nulls),
			// assign them directly to the result map without transformation.
			resultMap[key] = val
		}
	}

	return resultMap, nil
}

// MapToStruct is a generic function that converts a `map[string]any` into
// a new instance of the specified generic struct type `T`.
//
// This function is designed to be the inverse of `StructToMap`. It correctly
// handles `json.RawMessage` values within the input map by allowing the
// `encoding/json` package to unmarshal them directly into corresponding fields
// in the target struct `T` (whether `T`'s field is a struct, map, or
// `json.RawMessage` itself).
//
// The generic type `T` must be a struct type. If `T` is specified as a pointer
// type (e.g., `*MyStruct`), the function will unmarshal into the dereferenced
// struct and return a pointer to it.
//
// Returns a new instance of `T` populated with data from `input`, or the
// zero value of `T` and an error if conversion fails, if `input` is nil,
// or if `T` is not a struct type.
//
// Example:
//
//	type UserProfile struct {
//		ID   string `json:"id"`
//		Info struct { Name string `json:"name"` } `json:"info"`
//	}
//	inputMap := map[string]any{"id": "user-456", "info": json.RawMessage(`{"name":"Jane Doe"}`)}
//	user, err := MapToStruct[UserProfile](inputMap)
//	// user will be UserProfile{ID: "user-456", Info: struct{ Name string }{Name: "Jane Doe"}}
func MapToStruct[T any](input map[string]any) (T, error) {
	var zero T // Represents the zero value of type T, used for error returns

	if input == nil {
		return zero, fmt.Errorf("MapToStruct: input map cannot be nil")
	}

	// Validate that `T` is a struct type (or a pointer to a struct).
	// reflect.TypeOf(zero) gives the concrete type of T at runtime.
	typ := reflect.TypeOf(zero)
	// If T is a pointer type (e.g., *MyStruct), get the element type to check if it's a struct
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	// Ensure the base type is a struct, as `json.Unmarshal` expects to unmarshal into a struct
	if typ.Kind() != reflect.Struct {
		return zero, fmt.Errorf("MapToStruct: generic type T must be a struct type (or pointer to struct), got %s", typ.Kind())
	}

	// Step 1: Marshal the input `map[string]any` back into JSON bytes.
	// `json.Marshal` will correctly process `json.RawMessage` values, embedding
	// their contained bytes directly into the JSON output without re-encoding.
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return zero, fmt.Errorf("MapToStruct: failed to marshal input map to JSON: %w", err)
	}

	// Step 2: Unmarshal these JSON bytes into a new instance of `T`.
	// `encoding/json` will automatically unmarshal the JSON into the
	// corresponding fields of `T`, handling nested structures and types
	// defined by `json.RawMessage` in the input map correctly.
	var result T // Declare a variable of type T to unmarshal into
	if err := json.Unmarshal(jsonBytes, &result); err != nil { // Pass a pointer to `result` for unmarshaling
		return zero, fmt.Errorf("MapToStruct: failed to unmarshal JSON to target struct: %w", err)
	}

	return result, nil
}
