package document

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
)

// ============================================================================
// SANITIZATION
// ============================================================================

// Sanitize returns a sanitized copy of d. The original is unchanged.
// Scopes are resolved from d's embedded context plus any additional contexts.
func (d *Document) Sanitize(ctx ...context.Context) (data.Documenter, error) {
	if d == nil {
		return nil, ErrNilDocument
	}
	sanitizer, err := d.sanitizerForContexts(ctx...)
	if err != nil {
		return nil, err
	}
	if sanitizer == nil {
		return d.Clone(), nil
	}

	if d.isRecord() {
		return newRecordView(sanitizer.SanitizeDocumentDeep(deepCloneMap(d.record)), d.ctx), nil
	}

	sanitized := sanitizer.SanitizeDocumentDeep(d.ToMap())
	col := &DocumentPool{cs: d.cs, pool: d.pool}
	out, err := col.FromMap(sanitized, WithID(d.ID()), WithContext(d.ctx))
	if err != nil {
		return nil, err
	}
	if err := out.Hash(); err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("document.Sanitize").
			WithMessage("failed to hash sanitized document")
	}
	return out, nil
}

// SafeString returns a sanitized string representation suitable for logging.
func (d *Document) SafeString(ctx ...context.Context) string {
	sanitized, err := d.Sanitize(ctx...)
	if err != nil {
		return fmt.Sprintf("[SANITIZATION_ERROR: %v]", err)
	}
	return sanitized.String()
}

func (d *Document) sanitizerForContexts(ctx ...context.Context) (*data.DocumentSanitizer, error) {
	registry := data.GetSanitizationRegistry()
	if registry == nil {
		return nil, nil
	}
	return registry.GetForContext(d.Context(), ctx...)
}
