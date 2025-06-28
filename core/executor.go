package core

import ()

// Row represents a single record/row of data retrieved from the database.
// This is the input/output type for your pure Go functions.
type Document map[string]any

// GoComputeFunction is a pure Go function that computes a new value for a row.
// It takes a Row (representing the current data) and returns the computed value
// for a new field, and an error if computation fails.
type ComputeFunction func(row Document, args FilterValue) (any, error)

// GoFilterFunction is a pure Go function that performs custom filtering logic on a row.
// It takes a Row and returns true if the row passes the filter, false otherwise,
// and an error if evaluation fails.
type PredicateFunction func(doc Document, field string, args FilterValue) (bool, error)

