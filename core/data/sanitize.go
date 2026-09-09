package data

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/sanitize"
)

// ============================================================================
// Document Integration
// ============================================================================

// Sanitize returns a sanitized copy of the document using context-aware sanitization.
// The sanitization scope is determined by the context value at common.SanitizationScopeContextKey.
//
// Multiple contexts can be provided for multi-scope composition. When multiple scopes
// are specified, the most restrictive policy wins for each field.
//
// Returns error if:
//   - Sanitization scope not found and fail-fast is enabled
//   - Sanitization operation fails
//
// The returned document is a new instance; the original is unchanged.
//
// Deprecated: Use document.Document.Sanitize instead.
func (d *Document) Sanitize(ctx ...context.Context) (Documenter, error) {
	sanitizer, err := sanitize.Registry().GetForContext(d.ctx, ctx...)
	if err != nil {
		return nil, err
	}

	if sanitizer == nil {
		// No sanitization configured - return clone to maintain immutability
		return d.Clone(), nil
	}

	// Sanitize user data
	sanitizedData := sanitizer.SanitizeDocumentDeep(d.data)

	// Sanitize metadata (preserving system fields)
	sanitizedMetadata := sanitizer.SanitizeMetadata(d.metadata)

	// Create new document with sanitized data
	doc := &Document{
		id:       d.id,              // ID is never sanitized
		ctx:      d.ctx,             // Preserve original context
		data:     sanitizedData,     // Sanitized user data
		metadata: sanitizedMetadata, // Sanitized metadata
	}

	// Recalculate hash for sanitized document
	if err := doc.Hash(); err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("data.Document.Sanitize").
			WithMessage("failed to hash sanitized document")
	}

	return doc, nil
}

// SafeString returns a sanitized string representation suitable for logging.
// Uses context to determine appropriate sanitization rules.
// If sanitization fails, returns error string representation.
//
// Deprecated: Use document.Document.SafeString instead.
func (d *Document) SafeString(ctx ...context.Context) string {
	sanitized, err := d.Sanitize(ctx...)
	if err != nil {
		return fmt.Sprintf("[SANITIZATION_ERROR: %v]", err)
	}
	return sanitized.String()
}

// ============================================================================
// Helper Functions
// ============================================================================

// SanitizeDocumentArray sanitizes an array of documents.
// Each document uses its own embedded context for scope resolution.
//
// Deprecated: Use document.Document.Sanitize instead.
func SanitizeDocumentArray(docs []*Document) ([]Documenter, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	sanitized := make([]Documenter, len(docs))
	for i, doc := range docs {
		res, err := doc.Sanitize()
		if err != nil {
			return nil, common.SystemErrorFrom(err).
				WithOperation("data.SanitizeDocumentArray").
				WithMessagef("failed to sanitize document at index %d", i).
				WithIssue(common.Issue{
					Code:    "ERR_SANITIZATION_FAILED",
					Message: err.Error(),
					Index:   &i,
				})
		}
		sanitized[i] = res
	}
	return sanitized, nil
}

// SanitizeDocumentArrayWithContexts sanitizes documents with per-document contexts.
//
// Deprecated: Use document.Document.Sanitize instead.
func SanitizeDocumentArrayWithContexts(docs []*Document, contexts []context.Context) ([]Documenter, error) {
	if len(docs) != len(contexts) {
		return nil, common.NewSystemError("ERR_SANITIZATION_CONFIG_INVALID").
			WithMessage("docs and contexts length mismatch").
			WithMessagef("expected %d contexts for %d documents", len(docs), len(contexts))
	}

	sanitized := make([]Documenter, len(docs))
	for i, doc := range docs {
		res, err := doc.Sanitize(contexts[i])
		if err != nil {
			return nil, common.SystemErrorFrom(err).
				WithOperation("data.SanitizeDocumentArrayWithContexts").
				WithMessagef("failed to sanitize document at index %d", i).
				WithIssue(common.Issue{
					Code:    "ERR_SANITIZATION_FAILED",
					Message: err.Error(),
					Index:   &i,
				})
		}
		sanitized[i] = res
	}
	return sanitized, nil
}

// SanitizeValue sanitizes any value that might contain documents.
// For Documents, uses their embedded context. For raw maps, uses provided context.
//
// Deprecated: Use document.Document.Sanitize instead.
func SanitizeValue(ctx context.Context, value any) (any, error) {
	if value == nil {
		return nil, nil
	}

	switch v := value.(type) {
	case *Document:
		return v.Sanitize()

	case Document:
		return v.Sanitize()

	case []*Document:
		return SanitizeDocumentArray(v)

	case []Document:
		// Convert to slice of pointers before sanitizing
		docs := make([]*Document, len(v))
		for i := range v {
			docs[i] = &v[i]
		}
		return SanitizeDocumentArray(docs)

	case map[string]any:
		// Treat as raw document data - create temporary document
		tempDoc := &Document{
			ctx:  ctx,
			data: v,
		}
		sanitized, err := tempDoc.Sanitize(ctx)
		if err != nil {
			return nil, err
		}
		return sanitized.Data(), nil

	case []map[string]any:
		sanitized := make([]map[string]any, len(v))
		for i, m := range v {
			tempDoc := &Document{
				ctx:  ctx,
				data: m,
			}
			sanitizedDoc, err := tempDoc.Sanitize(ctx)
			if err != nil {
				return nil, common.SystemErrorFrom(err).
					WithOperation("data.SanitizeValue").
					WithMessagef("failed to sanitize map at index %d", i)
			}
			sanitized[i] = sanitizedDoc.Data()
		}
		return sanitized, nil

	case []any:
		// Recurse on array elements
		sanitized := make([]any, len(v))
		for i, item := range v {
			var err error
			sanitized[i], err = SanitizeValue(ctx, item)
			if err != nil {
				return nil, common.SystemErrorFrom(err).
					WithOperation("data.SanitizeValue").
					WithMessagef("failed to sanitize array element at index %d", i)
			}
		}
		return sanitized, nil

	default:
		// Scalar or unknown type - preserve as-is
		return value, nil
	}
}
