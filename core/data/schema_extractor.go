package data

import "github.com/asaidimu/go-anansi/v8/core/schema"

// SchemaFrom extracts a meta-schema JSON document for any struct type using
// the default "anansi" tag for field names (and all other metadata).
//
// Deprecated: the schema extractor moved to core/schema (its dedicated home,
// see the #cwzt7j note); use schema.SchemaFrom. This shim remains only for
// packages that cannot import core/schema directly (e.g. utils, which
// core/schema/definition itself depends on).
func SchemaFrom[T any](omitSystemField ...bool) ([]byte, error) {
	return schema.SchemaFrom[T](omitSystemField...)
}
