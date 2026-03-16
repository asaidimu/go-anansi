package ir

import "encoding/json"

// parse.go implements Pass 1: unmarshal raw JSON into the source model.
// All structural JSON errors are caught here. No IR construction occurs.

// parseSource unmarshals raw JSON into a sourceSchema. It returns a
// CompileError with PassParse if the JSON is malformed or missing required
// top-level fields (name, version).
func parseSource(src []byte) (*sourceSchema, []CompileError) {
	var s sourceSchema
	if err := json.Unmarshal(src, &s); err != nil {
		return nil, []CompileError{{
			Pass:    PassParse,
			Message: "invalid JSON: " + err.Error(),
		}}
	}

	var errs []CompileError

	if s.Name == "" {
		errs = append(errs, CompileError{
			Pass:    PassParse,
			Message: "schema missing required field: name",
		})
	}
	if s.Version == "" {
		errs = append(errs, CompileError{
			Pass:    PassParse,
			Message: "schema missing required field: version",
		})
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return &s, nil
}
