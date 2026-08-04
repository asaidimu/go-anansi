package document

import (
	"context"

	"github.com/asaidimu/go-anansi/v8/core/data"
)

// ============================================================================
// STRUCT BINDING
// ============================================================================
//
// Binding is lazy: values are read from the container's typed slots one field
// at a time via data.BindSourced, so a bind never materializes the whole
// document into a map. Only generated fast-path unmarshalers (which require a
// map-backed data.Document) trigger the materializing fallback.

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
	return data.BindSourced(d, d.asDataDocument, target, ctx, tag)
}

// ToStruct is an alias for BindTo.
func (d *Document) ToStruct(target any) error {
	return d.BindTo(target)
}

func (d *Document) asDataDocument() (*data.Document, error) {
	if d == nil {
		return nil, ErrNilDocument
	}
	return data.NewDocument(d.ToMap(), d.Context())
}
