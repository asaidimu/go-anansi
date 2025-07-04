// Package persistence provides utility functions for the persistence layer.
package persistence

import (
	"fmt"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/utils"
)

// createEvent is a helper function that constructs a PersistenceEvent. It populates
// the event with details about the operation, such as its type, the collection it
// belongs to, input and output data, and timing information. This function is used
// by the event-emitting wrappers to ensure that all events are created consistently.
func createEvent(
	eventType PersistenceEventType,
	operation string,
	collectionName string,
	input any,
	output any,
	query any,
	err *string,
	issues []Issue,
	startTime time.Time,
) PersistenceEvent {
	var duration *int64
	if !startTime.IsZero() {
		d := time.Since(startTime).Milliseconds()
		duration = &d
	}

	return PersistenceEvent{
		Type:       eventType,
		Timestamp:  time.Now().UnixMilli(),
		Operation:  operation,
		Collection: &collectionName,
		Input:      input,
		Output:     output,
		Error:      err,
		Issues:     issues,
		Query:      query,
		Duration:   duration,
	}
}

// DocumentToStruct converts a document represented as `any` (which is expected
// to be a `map[string]any`) into a new instance of the specified generic
// struct type `T`.
//
// This function acts as a convenient wrapper around `MapToStruct`. It first
// performs a type assertion to ensure that the input `doc` is indeed a
// `map[string]any`.
//
// The generic type `T` must be a struct type, as required by `MapToStruct`.
//
// Returns a new instance of `T` populated with data from `doc`, or the
// zero value of `T` and an error if conversion fails, if `doc` is not
// a `map[string]any`, or if `T` is not a struct type.
//
// Example:
//
//	type Product struct {
//		Name  string  `json:"name"`
//		Price float64 `json:"price"`
//	}
//	documentData := map[string]any{"name": "Laptop", "price": 1200.50}
//	product, err := DocumentToStruct[Product](documentData)
//	// product will be Product{Name: "Laptop", Price: 1200.50}
func DocumentToStruct[T any](doc any) (T, error) {
	var zero T
	data, ok := doc.(schema.Document)
	if !ok {
		return zero, fmt.Errorf("DocumentToStruct: input 'doc' must be a map[string]any, got %T", doc)
	}
	return utils.MapToStruct[T](data)
}

// StructToDocument converts a Go struct into a `map[string]any` representation,
// where nested structs are transformed into `json.RawMessage`.
//
// This function is an alias for `StructToMap`, providing a semantic name
// when the target `map[string]any` is intended to represent a "document"
// (e.g., for storage in a NoSQL database or for generic API payloads).
//
// The input `record` must be a struct or a pointer to a struct. If `record` is
// nil, or not a struct/pointer to a struct, an error is returned.
//
// Example:
//
//	type UserData struct {
//		Name    string `json:"name"`
//		Address struct { City string `json:"city"` } `json:"address"`
//	}
//	user := UserData{Name: "Alice", Address: struct{ City string }{"New York"}}
//	doc, err := StructToDocument(user)
//	// doc will be map[string]any{"name": "Alice", "address": json.RawMessage(`{"city":"New York"}`)}
func StructToDocument[T any](record T) (schema.Document, error) {
	return utils.StructToMap(record)
}
