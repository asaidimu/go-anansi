package document

import (
	"reflect"

	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
)

// ============================================================================
// STRUCT BINDING (document-backed shim)
// ============================================================================
//
// The struct binder itself — field walking, tag parsing, value coercion, the
// system-model registry, and the bind loop — moved to core/schema/binding so
// serializers can bind structs to schema-bound containers without the
// document model. This file keeps the document-level surface that still
// matters here: *Document satisfies the binder's interfaces (Get, ID,
// Metadata, BindField) and the error values keep their historical names.

// ErrBind* live in core/schema/binding; these aliases keep the document
// package's exported error surface stable.
var (
	ErrBindInvalidTargetType    = binding.ErrBindInvalidTargetType
	ErrBindFailedToSetField     = binding.ErrBindFailedToSetField
	ErrBindTypeConversionFailed = binding.ErrBindTypeConversionFailed
	ErrBindEmptyFieldName       = binding.ErrBindEmptyFieldName
	ErrBindUnknownDocTagOption  = binding.ErrBindUnknownDocTagOption
	ErrBindKeyNotFound          = binding.ErrBindKeyNotFound
	ErrBindCannotTraverse       = binding.ErrBindCannotTraverse
)

const bindDocumentIDField = binding.DocumentIDField
const bindMetadataField = binding.MetadataField

// reservedBindField reports whether key is a system-managed field name.
func reservedBindField(key string) bool {
	return binding.ReservedBindName(key)
}

// RegisterSystemModelType registers an embedded model type that carries
// system document fields (_id_, _metadata_) and must be treated like
// DocumentModel by the binder. Register at package init, before any
// documents are built. It registers with the shared binder registry in
// core/schema/binding (and thereby also affects the document binder).
func RegisterSystemModelType(t reflect.Type, linkParent func(embed any, parent any)) {
	binding.RegisterSystemModelType(t, linkParent)
}

// IsSystemModelType reports whether t is a registered system-model embed.
func IsSystemModelType(t reflect.Type) bool {
	return binding.IsSystemModelType(t)
}
