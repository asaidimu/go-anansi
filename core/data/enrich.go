package data

import (
	"fmt"
	"sort"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// EnrichOption configures how EnrichSchema injects the system fields.
type EnrichOption func(*enrichOptions)

type enrichOptions struct {
	partialSystemFields bool
}

// WithPartialSystemFields marks the injected system fields (notably _id_) as
// optional rather than required. The persistence registry keeps the strict form
// (required _id_), so the storage layer still enforces identity presence; the
// container-backed document layer uses the partial form so documents whose
// identity has not yet been assigned can be decoded, compiled, and patched.
func WithPartialSystemFields() EnrichOption {
	return func(o *enrichOptions) { o.partialSystemFields = true }
}

// EnrichSchema adds the system fields _id_ and _metadata_ to a copy of sc so
// the compiled schema exposes the document's identity and metadata as ordinary
// fields in the user-data container. It is the single enrichment utility — both
// the container-backed document layer and the persistence registry delegate to
// it, guaranteeing they agree on field IDs, field order, and the injection
// mechanics. The metadata schema (defaults + provider fields) and its dependency
// schemas are supplied by the caller from the calling layer's own factory, so
// provider fields configured on that layer are honored.
//
// Static field and schema IDs make the result idempotent: enriching an
// already-enriched schema reproduces the identical output, which is critical
// for migration diff computation. The input schema is not mutated; a deep copy
// is enriched and returned.
//
// By default _id_ is marked required. Partial documents — payloads that arrive
// before the document factory has assigned an identity — can be supported by
// passing WithPartialSystemFields, which drops the required flag so decoding
// and compiling may proceed; the factory still generates _id_ for every
// document that reaches persistence, so uniqueness is preserved.
func EnrichSchema(sc *definition.Schema, meta *definition.NestedSchema, deps []*definition.NestedSchema, opts ...EnrichOption) (*definition.Schema, error) {
	var cfg enrichOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	if sc == nil {
		return nil, nil
	}
	if sc == nil {
		return nil, nil
	}
	s := sc.DeepCopy()

	// Drop any pre-existing _id_/_metadata_ fields and metadata sub-schemas so
	// re-enrichment is stable and a user-declared system field is replaced.
	for fid, f := range s.Fields {
		if f.Name == DocumentIDField || f.Name == MetadataField {
			delete(s.Fields, fid)
		}
	}
	for sid := range s.Schemas {
		if ns, ok := s.Schemas[sid]; ok && ns.Name == MetadataField {
			delete(s.Schemas, sid)
		}
	}
	if s.Schemas == nil {
		s.Schemas = make(map[definition.SchemaId]definition.NestedSchema)
	}

	// Register the metadata schema under its static ID, plus any dependency
	// schemas its fields reference.
	if meta == nil {
		meta = DefaultMetadataSchema()
	}
	s.Schemas[definition.SchemaId(SystemSchemaIDMetadata)] = *meta
	for _, dep := range deps {
		s.Schemas[definition.SchemaId(dep.Name)] = *dep
	}

	if s.Fields == nil {
		s.Fields = make(map[definition.FieldId]definition.Field)
	}
	s.Fields[definition.FieldId(SystemFieldIDDocumentID)] = definition.Field{
		Name:     definition.FieldName(DocumentIDField),
		Required: !cfg.partialSystemFields,
		Unique:   true,
		FieldProperties: definition.FieldProperties{
			Type: definition.FieldTypeString,
		},
	}
	s.Fields[definition.FieldId(SystemFieldIDMetadata)] = definition.Field{
		Name: definition.FieldName(MetadataField),
		FieldProperties: definition.FieldProperties{
			Type: definition.FieldTypeObject,
			Schema: definition.NewSchemaReference(definition.SchemaReference{
				ID: definition.SchemaId(SystemSchemaIDMetadata),
			}),
		},
	}

	if err := ensureSystemFieldsLead(s); err != nil {
		return nil, err
	}
	return s, nil
}

// ensureSystemFieldsLead verifies _id_ and _metadata_ are the two smallest
// field IDs. The compiled schema orders fields lexicographically by FieldId and
// assigns flat storage addresses from that order, so the system fields must
// occupy the first two slots. A user field whose ID sorts before a system ID
// would silently shift every address; fail loudly instead.
func ensureSystemFieldsLead(s *definition.Schema) error {
	ids := make([]string, 0, len(s.Fields))
	for id := range s.Fields {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	if len(ids) < 2 ||
		ids[0] != SystemFieldIDDocumentID ||
		ids[1] != SystemFieldIDMetadata {
		return fmt.Errorf("data: schema %q: system fields must be the first two fields; "+
			"field IDs sort to %v, want first two to be %q and %q",
			s.Name, ids, SystemFieldIDDocumentID, SystemFieldIDMetadata)
	}
	return nil
}
