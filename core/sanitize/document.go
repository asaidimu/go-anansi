package sanitize

import (
	"slices"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"go.uber.org/zap"
)

// ============================================================================
// DocumentSanitizer - Document-Aware Operations
// ============================================================================

// DocumentSanitizer extends Sanitizer with Document-aware operations
type DocumentSanitizer struct {
	*Sanitizer
	scopeID string // For logging and debugging
}

// NewDocumentSanitizer creates a sanitizer that understands Document structure
func NewDocumentSanitizer(config FieldMaskConfig, logger *zap.Logger, scopeID string) *DocumentSanitizer {
	return &DocumentSanitizer{
		Sanitizer: NewSanitizer(config, logger),
		scopeID:   scopeID,
	}
}

// SanitizeDocumentDeep performs deep sanitization including nested documents
// This handles the Document type's recursive structure properly
func (ds *DocumentSanitizer) SanitizeDocumentDeep(doc map[string]any) map[string]any {
	if doc == nil {
		return nil
	}

	sanitized := make(map[string]any, len(doc))

	for fieldName, value := range doc {
		// Special handling for metadata field - sanitize recursively but preserve structure
		if fieldName == common.MetadataField {
			if metaMap, ok := value.(map[string]any); ok {
				sanitized[fieldName] = ds.SanitizeMetadata(metaMap)
				continue
			}
		}

		// Apply policy and recurse for nested structures
		sanitized[fieldName] = ds.sanitizeValueDeep(fieldName, value)
	}

	return sanitized
}

// Reserved metadata field names that should never be sanitized
var reservedMetadataFields = []string{
	common.MetadataVersion,
	common.MetadataCreated,
	common.MetadataUpdated,
	common.MetadataChecksum,
	common.MetadataSignature,
}

// isSystemMetadataField checks if a field is a reserved system metadata field
func isSystemMetadataField(key string) bool {
	return slices.Contains(reservedMetadataFields, key)
}

// SanitizeMetadata handles metadata specially - preserve system fields, sanitize user fields
func (ds *DocumentSanitizer) SanitizeMetadata(metadata map[string]any) map[string]any {
	sanitized := make(map[string]any, len(metadata))

	for key, value := range metadata {
		if isSystemMetadataField(key) {
			// Preserve system metadata as-is
			sanitized[key] = value
		} else {
			// Sanitize user-defined metadata fields
			sanitized[key] = ds.sanitizeValueDeep(key, value)
		}
	}

	return sanitized
}

// sanitizeValueDeep recursively sanitizes values including nested structures
func (ds *DocumentSanitizer) sanitizeValueDeep(fieldName string, value any) any {
	if value == nil {
		return nil
	}

	// Get policy for this field
	policy := ds.getPolicyForField(fieldName)

	// Handle nested structures before applying policy
	switch v := value.(type) {
	case map[string]any:
		// Nested document - recurse with full sanitization
		nested := make(map[string]any, len(v))
		for nestedKey, nestedValue := range v {
			nested[nestedKey] = ds.sanitizeValueDeep(nestedKey, nestedValue)
		}
		return nested

	case []map[string]any:
		// Array of documents
		sanitizedArray := make([]map[string]any, len(v))
		for i, item := range v {
			sanitizedItem := make(map[string]any, len(item))
			for itemKey, itemValue := range item {
				sanitizedItem[itemKey] = ds.sanitizeValueDeep(itemKey, itemValue)
			}
			sanitizedArray[i] = sanitizedItem
		}
		return sanitizedArray

	case []any:
		// Generic array - recurse on each element
		sanitizedArray := make([]any, len(v))
		for i, item := range v {
			// For array items, we can't determine field name, use array context
			sanitizedArray[i] = ds.sanitizeValueDeep(fieldName+"[]", item)
		}
		return sanitizedArray

	default:
		// Scalar value - apply policy
		return ds.applyPolicy(fieldName, value, policy)
	}
}

// SanitizeNestedMap sanitizes a nested map value (record-typed fields,
// unknown objects) the same way SanitizeDocumentDeep handles nested values,
// without the top-level metadata special-casing. A new map is allocated; the
// input is unchanged. Reserved-metadata preservation is the caller's concern.
func (ds *DocumentSanitizer) SanitizeNestedMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = ds.sanitizeValueDeep(k, v)
	}
	return out
}

// PolicyForField returns the masking policy that applies to a field name.
func (s *Sanitizer) PolicyForField(fieldName string) MaskedFieldPolicy {
	return s.getPolicyForField(fieldName)
}

// ApplyPolicyValue applies a masking policy to a value, returning the masked
// value (MaskPreserve returns value unchanged).
func (s *Sanitizer) ApplyPolicyValue(fieldName string, value any, policy MaskedFieldPolicy) any {
	return s.applyPolicy(fieldName, value, policy)
}

// applyPolicy overrides parent to add scope context to warnings
func (ds *DocumentSanitizer) applyPolicy(fieldName string, value any, policy MaskedFieldPolicy) any {
	if value == nil {
		return nil
	}

	switch policy {
	case MaskRedact, MaskHash, MaskObscure, MaskPreserve:
		return ds.Sanitizer.applyPolicy(fieldName, value, policy)
	default:
		ds.logger.Warn("Unknown masking policy, preserving value",
			zap.String("scope", ds.scopeID),
			zap.String("field", fieldName),
			zap.String("policy", string(policy)))
		return value
	}
}
