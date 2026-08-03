package document

import (
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// enrichSchema adds the system fields _id_ and _metadata_ to s so the compiled
// schema exposes the document's identity and metadata as ordinary fields in the
// user-data container. Without this the custom codec — which serializes a
// container keyed by schema field names — could not emit them, and the separate
// metadata container would be required.
//
// It delegates to data.EnrichSchema, the single shared enrichment utility, so
// the container-backed document layer and the persistence registry always
// inject identical field IDs, field order, and metadata schemas. Unlike the
// registry it uses WithPartialSystemFields: at the document level _id_ is
// optional so partial documents (payloads without an identity) decode and
// compile, with the document factory generating _id_ before persistence.
func enrichSchema(s *definition.Schema) (*definition.Schema, error) {
	if s == nil {
		return nil, ErrNoSchema
	}
	meta, deps := data.GetMetadataSchema()
	return data.EnrichSchema(s, meta, deps, data.WithPartialSystemFields())
}
