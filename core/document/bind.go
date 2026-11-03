package document

import (
	"context"

	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
)

// ============================================================================
// STRUCT BINDING
// ============================================================================
//
// Binding walks the target struct's cached field metadata (built from the
// core/reflect tag registry) and fills each field straight from this
// document's typed container slots via BindField, falling back to Get +
// coercion for field kinds the container cannot bind directly. A bind
// therefore never materializes the document into a map — nested binds recurse
// over container-backed child views instead of maps.
//
// The binder itself lives in core/schema/binding — the schema↔struct↔container
// substrate below the document model, shared with lightweight serializers.
// *Document satisfies that package's Source (Get), systemSource (ID,
// Metadata) and TypedBinder (BindField, bind_field.go) interfaces, so system
// fields (_id_, _metadata_) route through the document's identity/metadata
// rather than generic field lookup.

// BindTo binds the document data into a target struct.
func (d *Document) BindTo(target any) error {
	return d.bindTo(context.Background(), target, "")
}

// BindToWithContext binds the document data into a target struct with a context.
func (d *Document) BindToWithContext(ctx context.Context, target any) error {
	return d.bindTo(ctx, target, "")
}

// BindToTag binds the document data into a target struct using the given tag.
func (d *Document) BindToTag(target any, tag string) error {
	return d.bindTo(context.Background(), target, tag)
}

// BindToTagWithContext binds the document data into a target struct using the
// given tag and context.
func (d *Document) BindToTagWithContext(ctx context.Context, target any, tag string) error {
	return d.bindTo(ctx, target, tag)
}

func (d *Document) bindTo(ctx context.Context, target any, tag string) error {
	if d == nil {
		return ErrNilDocument
	}
	return binding.Bind(d, target, ctx, tag)
}

// ToStruct is an alias for BindTo.
func (d *Document) ToStruct(target any) error {
	return d.BindTo(target)
}
