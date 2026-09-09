package document

import (
	"github.com/asaidimu/go-anansi/v8/core/data"
)

// ============================================================================
// MUST HELPER
// ============================================================================

// Must returns a MustHelper bound to a materialized map view of d. The helper
// performs panic-on-error lookups over the document data.
func (d *Document) Must() *data.MustHelper {
	return data.Patch(d.ToMap()).Document(d.Context()).Must()
}
