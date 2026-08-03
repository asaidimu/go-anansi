package document

import (
	"context"

	"github.com/asaidimu/go-anansi/v8/core/data"
)

// ============================================================================
// STRUCT BINDING
// ============================================================================
//
// Binding delegates to a materialized data.Document so tag/field/struct logic
// is identical. Requires the data factory to be configured at startup.

// BindTo binds the document data into a target struct.
func (d *Document) BindTo(target any) error {
	md, err := d.asDataDocument()
	if err != nil {
		return err
	}
	return md.BindTo(target)
}

// BindToWithContext binds the document data into a target struct with a context.
func (d *Document) BindToWithContext(ctx context.Context, target any) error {
	md, err := d.asDataDocument()
	if err != nil {
		return err
	}
	return md.BindToWithContext(ctx, target)
}

// BindToTag binds the document data into a target struct using the given tag.
func (d *Document) BindToTag(target any, tag string) error {
	md, err := d.asDataDocument()
	if err != nil {
		return err
	}
	return md.BindToTag(target, tag)
}

// BindToTagWithContext binds the document data into a target struct using the
// given tag and context.
func (d *Document) BindToTagWithContext(ctx context.Context, target any, tag string) error {
	md, err := d.asDataDocument()
	if err != nil {
		return err
	}
	return md.BindToTagWithContext(ctx, target, tag)
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
