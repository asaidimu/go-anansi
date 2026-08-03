package document

import (
	"github.com/asaidimu/go-anansi/v8/core/data"
)

// ============================================================================
// JSONPATH QUERY
// ============================================================================

// JSONPathQuery evaluates a JSONPath expression against the document data.
func (d *Document) JSONPathQuery(path string) ([]any, error) {
	if d == nil {
		return nil, ErrNilDocument
	}
	md := data.Patch(d.Data()).Document(d.Context())
	return md.JSONPathQuery(path)
}
