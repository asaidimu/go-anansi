package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

// TODO convert plain errors to system errors
// MaskedFieldPolicy defines how a field should be treated during sanitization
type MaskedFieldPolicy string

const (
	MaskRedact   MaskedFieldPolicy = "redact"   // Replace with "[REDACTED]"
	MaskHash     MaskedFieldPolicy = "hash"     // Replace with short hash of value
	MaskPreserve MaskedFieldPolicy = "preserve" // Keep original (for safe fields)
	MaskObscure  MaskedFieldPolicy = "obscure"  // e.g., show first/last few chars
)

// FieldMaskPattern allows regex matching on field names
type FieldMaskPattern struct {
	Regex  *regexp.Regexp
	Policy MaskedFieldPolicy
}

// FieldMaskConfig defines per-field masking rules for a collection or schema
type FieldMaskConfig struct {
	// Fields maps field name (case-sensitive) to masking policy
	Fields map[string]MaskedFieldPolicy

	// Patterns allows regex-based field matching for dynamic rules
	Patterns []FieldMaskPattern

	// DefaultPolicy is applied when no explicit rule matches (default: preserve)
	DefaultPolicy MaskedFieldPolicy

	// ObscureConfig controls behavior of MaskObscure policy
	ObscureConfig ObscureConfig
}

// ObscureConfig defines how obscuring should work
type ObscureConfig struct {
	// PrefixLength: how many characters to show at the start
	PrefixLength int

	// SuffixLength: how many characters to show at the end
	SuffixLength int

	// Replacement: character to use for obscured portion (default: "*")
	Replacement string

	// MinLength: minimum length before obscuring applies (show full value if shorter)
	MinLength int
}

// DefaultObscureConfig provides sensible defaults for obscuring
func DefaultObscureConfig() ObscureConfig {
	return ObscureConfig{
		PrefixLength: 2,
		SuffixLength: 2,
		Replacement:  "*",
		MinLength:    6,
	}
}

// Sanitizer handles field masking based on configuration
type Sanitizer struct {
	config FieldMaskConfig
	logger *zap.Logger
}

// NewSanitizer creates a new field sanitizer with the given configuration
func NewSanitizer(config FieldMaskConfig, logger *zap.Logger) *Sanitizer {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Set default policy if not specified
	if config.DefaultPolicy == "" {
		config.DefaultPolicy = MaskPreserve
	}

	// Set default obscure config if not specified
	if config.ObscureConfig.Replacement == "" {
		config.ObscureConfig = DefaultObscureConfig()
	}

	return &Sanitizer{
		config: config,
		logger: logger,
	}
}

// SanitizeDocument applies masking rules to a map (map[string]any)
// Returns a new map with masked values - does not modify the original
func (s *Sanitizer) SanitizeDocument(doc map[string]any) map[string]any {
	if doc == nil {
		return nil
	}

	sanitized := make(map[string]any, len(doc))

	for fieldName, value := range doc {
		policy := s.getPolicyForField(fieldName)
		sanitized[fieldName] = s.applyPolicy(fieldName, value, policy)
	}

	return sanitized
}

// SanitizeValue applies masking to a single value based on field name
func (s *Sanitizer) SanitizeValue(fieldName string, value any) any {
	policy := s.getPolicyForField(fieldName)
	return s.applyPolicy(fieldName, value, policy)
}

// getPolicyForField determines which policy applies to a given field
func (s *Sanitizer) getPolicyForField(fieldName string) MaskedFieldPolicy {
	// 1. Check explicit field mapping (highest priority)
	if policy, exists := s.config.Fields[fieldName]; exists {
		return policy
	}

	// 2. Check pattern-based rules
	for _, pattern := range s.config.Patterns {
		if pattern.Regex != nil && pattern.Regex.MatchString(fieldName) {
			return pattern.Policy
		}
	}

	// 3. Fall back to default policy
	return s.config.DefaultPolicy
}

// applyPolicy applies the specified masking policy to a value
func (s *Sanitizer) applyPolicy(fieldName string, value any, policy MaskedFieldPolicy) any {
	// Don't mask nil values
	if value == nil {
		return nil
	}

	switch policy {
	case MaskRedact:
		return "***"

	case MaskHash:
		return s.hashValue(value)

	case MaskObscure:
		return s.obscureValue(value)

	case MaskPreserve:
		return value

	default:
		s.logger.Warn("Unknown masking policy, preserving value",
			zap.String("field", fieldName),
			zap.String("policy", string(policy)))
		return value
	}
}

// hashValue creates a short hash of the value for auditing purposes
func (s *Sanitizer) hashValue(value any) string {
	// Convert value to string representation
	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		str = fmt.Sprintf("%v", v)
	}

	// Create SHA-256 hash
	hash := sha256.Sum256([]byte(str))

	// Return first 8 characters of hex encoding (32 bits)
	// This is enough for collision detection in logs without exposing data
	return fmt.Sprintf("[HASH:%s]", hex.EncodeToString(hash[:])[:8])
}

// obscureValue shows first/last characters with middle obscured
func (s *Sanitizer) obscureValue(value any) string {
	// Convert to string
	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		str = fmt.Sprintf("%v", v)
	}

	length := len(str)

	// If value is too short, show it fully or redact completely
	if length < s.config.ObscureConfig.MinLength {
		return "[OBSCURED]"
	}

	prefix := s.config.ObscureConfig.PrefixLength
	suffix := s.config.ObscureConfig.SuffixLength

	// Ensure we don't try to show more than available
	if prefix+suffix >= length {
		return strings.Repeat(s.config.ObscureConfig.Replacement, length)
	}

	// Build obscured string
	prefixPart := str[:prefix]
	suffixPart := str[length-suffix:]
	obscuredLength := length - prefix - suffix

	return fmt.Sprintf("%s%s%s",
		prefixPart,
		strings.Repeat(s.config.ObscureConfig.Replacement, obscuredLength),
		suffixPart)
}

// MustCompilePattern is a helper to compile regex patterns with panic on error
// Useful for initialization-time pattern compilation
func MustCompilePattern(pattern string, policy MaskedFieldPolicy) FieldMaskPattern {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		panic(fmt.Sprintf("failed to compile field mask pattern %q: %v", pattern, err))
	}
	return FieldMaskPattern{
		Regex:  regex,
		Policy: policy,
	}
}

// CommonSecurityPatterns returns commonly used patterns for credential fields
func CommonSecurityPatterns() []FieldMaskPattern {
	return []FieldMaskPattern{
		MustCompilePattern(`(?i)password`, MaskRedact),
		MustCompilePattern(`(?i)secret`, MaskRedact),
		MustCompilePattern(`(?i)token`, MaskRedact),
		MustCompilePattern(`(?i)api[_-]?key`, MaskRedact),
		MustCompilePattern(`(?i)private[_-]?key`, MaskRedact),
		MustCompilePattern(`(?i)credential`, MaskRedact),
		MustCompilePattern(`(?i)auth`, MaskHash),
		MustCompilePattern(`(?i)ssn|social[_-]?security`, MaskRedact),
		MustCompilePattern(`(?i)credit[_-]?card|cvv`, MaskRedact),
		MustCompilePattern(`(?i)email`, MaskObscure),
		MustCompilePattern(`(?i)phone|mobile`, MaskObscure),
	}
}

// NewSecureDefaultConfig creates a sanitizer config with common security patterns
func NewSecureDefaultConfig() *FieldMaskConfig {
	return &FieldMaskConfig{
		Fields:        make(map[string]MaskedFieldPolicy),
		Patterns:      CommonSecurityPatterns(),
		DefaultPolicy: MaskPreserve,
		ObscureConfig: DefaultObscureConfig(),
	}
}

// DocumentSanitizer extends Sanitizer with Document-aware operations
type DocumentSanitizer struct {
	*Sanitizer
}

// NewDocumentSanitizer creates a sanitizer that understands Document structure
func NewDocumentSanitizer(config FieldMaskConfig, logger *zap.Logger) *DocumentSanitizer {
	return &DocumentSanitizer{
		Sanitizer: NewSanitizer(config, logger),
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
		if fieldName == "_metadata_" {
			if metaMap, ok := value.(map[string]any); ok {
				sanitized[fieldName] = ds.sanitizeMetadata(metaMap)
				continue
			}
		}

		// Apply policy and recurse for nested structures
		sanitized[fieldName] = ds.sanitizeValueDeep(fieldName, value)
	}

	return sanitized
}

// sanitizeMetadata handles metadata specially - preserve system fields, sanitize user fields
func (ds *DocumentSanitizer) sanitizeMetadata(metadata map[string]any) map[string]any {
	sanitized := make(map[string]any, len(metadata))

	// System metadata fields that should always be preserved
	systemFields := map[string]bool{
		"version": true,
		"created": true,
		"updated": true,
	}

	for key, value := range metadata {
		if systemFields[key] {
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

// SanitizeForLogging creates a sanitized copy suitable for logging
// This is a convenience method for common use case
func (ds *DocumentSanitizer) SanitizeForLogging(doc map[string]any) map[string]any {
	return ds.SanitizeDocumentDeep(doc)
}

// SanitizeID determines if an ID field should be sanitized
// IDs are typically safe to log, but you might want to hash them in some contexts
func (ds *DocumentSanitizer) SanitizeID(id string) string {
	policy := ds.getPolicyForField("id")
	if policy == MaskPreserve {
		return id
	}

	sanitized := ds.applyPolicy("id", id, policy)
	if str, ok := sanitized.(string); ok {
		return str
	}
	return "[SANITIZED_ID]"
}

// Sanitize returns a sanitized copy of the document using context-aware sanitization.
func (d *Document) Sanitize(ctx ...context.Context) *Document {
	sctx := d.ctx
	if len(ctx) > 0 && ctx[0] != nil {
		sctx = ctx[0]
	}
	sanitizer := getFactory().getSanitizerForContext(sctx)
	if sanitizer == nil {
		// No sanitization configured - return clone to maintain immutability
		return d.Clone()
	}

	sanitized := sanitizer.SanitizeDocumentDeep(d.data)
	doc := MustNewDocument(sanitized, sctx)
	doc.Hash()
	return doc
}

// SafeString returns a sanitized string representation suitable for logging.
// Uses context to determine appropriate sanitization rules.
func (d *Document) SafeString(ctx ...context.Context) string {
	sanitized := d.Sanitize(ctx...)
	return sanitized.String()
}

// SanitizeArray sanitizes an array of documents using context-aware rules
func SanitizeDocumentArray(ctx context.Context, docs []*Document) []*Document {
	if len(docs) == 0 {
		return docs
	}

	sanitized := make([]*Document, len(docs))
	for i, doc := range docs {
		sanitized[i] = doc.Sanitize(ctx)
	}
	return sanitized
}

// SanitizeValue is a helper that sanitizes any value that might contain documents.
// It handles Document, []Document, map[string]any, and []map[string]any types.
// This is useful when you have generic data structures that might contain documents.
func SanitizeValue(ctx context.Context, value any) any {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case *Document:
		return v.Sanitize(ctx)
	case Document:
		return v.Sanitize(ctx)
	case []*Document:
		return SanitizeDocumentArray(ctx, v)
	case []Document:
		// Convert to slice of pointers before sanitizing
		docs := make([]*Document, len(v))
		for i := range v {
			docs[i] = &v[i]
		}
		return SanitizeDocumentArray(ctx, docs)
	case map[string]any:
		// Treat as document
		return (&Document{ctx: ctx, data: v}).Sanitize(ctx)

	case []map[string]any:
		sanitized := make([]map[string]any, len(v))
		for i, m := range v {
			sanitized[i] = (&Document{ctx: ctx, data: m}).Sanitize(ctx).data
		}
		return sanitized

	case []any:
		// Recurse on array elements
		sanitized := make([]any, len(v))
		for i, item := range v {
			sanitized[i] = SanitizeValue(ctx, item)
		}
		return sanitized

	default:
		// Scalar or unknown type - preserve as-is
		return value
	}
}

// mergeConfigs merges collection-specific config with global config.
// Collection config takes precedence for conflicts.
func mergeConfigs(globalConfig *FieldMaskConfig, collectionConfig FieldMaskConfig) FieldMaskConfig {
	// If no global config, return collection config as-is
	if globalConfig == nil {
		return collectionConfig
	}

	merged := FieldMaskConfig{
		Fields:   make(map[string]MaskedFieldPolicy),
		Patterns: []FieldMaskPattern{},
	}

	// Start with global field mappings
	maps.Copy(merged.Fields, globalConfig.Fields)

	// Override with collection-specific field mappings
	maps.Copy(merged.Fields, collectionConfig.Fields)

	// Append global patterns first (lower priority)
	merged.Patterns = append(merged.Patterns, globalConfig.Patterns...)

	// Append collection patterns (higher priority - checked first due to order)
	merged.Patterns = append(merged.Patterns, collectionConfig.Patterns...)

	// Collection default policy overrides global, or use global if not set
	if collectionConfig.DefaultPolicy != "" {
		merged.DefaultPolicy = collectionConfig.DefaultPolicy
	} else {
		merged.DefaultPolicy = globalConfig.DefaultPolicy
	}

	// Use collection obscure config if provided, otherwise use global
	if collectionConfig.ObscureConfig.Replacement != "" {
		merged.ObscureConfig = collectionConfig.ObscureConfig
	} else {
		merged.ObscureConfig = globalConfig.ObscureConfig
	}

	return merged
}

// GetSanitizerForCollection is a helper for testing or manual sanitization.
// In normal operation, use Document.Sanitize(ctx) instead.
func GetSanitizerForCollection(collectionName string) *DocumentSanitizer {
	f := getFactory()
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.sanitization == nil {
		return nil
	}

	if sanitizer, exists := f.sanitization.collection[collectionName]; exists {
		return sanitizer
	}

	return f.sanitization.global
}
