package document

import (
	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
)

// ============================================================================
// STRUCT WALKER (shim)
// ============================================================================
//
// The walker moved to core/schema/binding (binding.WalkStructFields) so
// serializers can walk structs without the document model. This shim keeps
// the document-internal names used by populateFromStruct and tests.

// structFieldValue pairs a tagged struct field's document path with its
// normalized value.
type structFieldValue struct {
	path  string
	value any
}

// walkStructFields walks s and returns each anansi-tagged field as a
// (path, value) pair without materializing a map.
func walkStructFields(s any, partial bool) ([]structFieldValue, error) {
	fields, err := binding.WalkStructFields(s, partial)
	if err != nil || fields == nil {
		return nil, err
	}
	out := make([]structFieldValue, len(fields))
	for i, f := range fields {
		out[i] = structFieldValue{path: f.Path, value: f.Value}
	}
	return out, nil
}
