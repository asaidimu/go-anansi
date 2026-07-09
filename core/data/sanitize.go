package data

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"go.uber.org/zap"
)

// ============================================================================
// Policy Types and Constants
// ============================================================================

// MaskedFieldPolicy defines how a field should be treated during sanitization
type MaskedFieldPolicy string

const (
	MaskRedact   MaskedFieldPolicy = "redact"   // Replace with "***"
	MaskHash     MaskedFieldPolicy = "hash"     // Replace with short hash of value
	MaskPreserve MaskedFieldPolicy = "preserve" // Keep original (for safe fields)
	MaskObscure  MaskedFieldPolicy = "obscure"  // e.g., show first/last few chars
)

// String returns the string representation of the policy
func (p MaskedFieldPolicy) String() string {
	return string(p)
}

// ParseMaskedFieldPolicy parses a string into a MaskedFieldPolicy
func ParseMaskedFieldPolicy(s string) (MaskedFieldPolicy, error) {
	switch s {
	case "redact", "REDACT":
		return MaskRedact, nil
	case "hash", "HASH":
		return MaskHash, nil
	case "obscure", "OBSCURE":
		return MaskObscure, nil
	case "preserve", "PRESERVE":
		return MaskPreserve, nil
	default:
		return "", common.NewSystemError("ERR_SANITIZATION_CONFIG_INVALID").
			WithMessagef("unknown policy: %s (valid: redact, hash, obscure, preserve)", s)
	}
}

// policyWeight defines restrictiveness ordering for multi-scope composition
var policyWeight = map[MaskedFieldPolicy]int{
	MaskRedact:   4, // Most restrictive
	MaskHash:     3,
	MaskObscure:  2,
	MaskPreserve: 1, // Least restrictive
}

// ============================================================================
// Configuration Structures
// ============================================================================

// PatternRule allows regex matching on field names
type PatternRule struct {
	Pattern string            `json:"pattern"`           // regex string
	Policy  MaskedFieldPolicy `json:"policy"`            // Masking policy
	Comment string            `json:"comment,omitempty"` // Human-readable description

	// Private: compiled regex (populated after validation)
	regex *regexp.Regexp
}

// ObscureConfig defines how obscuring should work
type ObscureConfig struct {
	// PrefixLength: how many characters to show at the start
	PrefixLength int `json:"prefix_length"`

	// SuffixLength: how many characters to show at the end
	SuffixLength int `json:"suffix_length"`

	// Replacement: character to use for obscured portion (default: "*")
	Replacement string `json:"replacement"`

	// MaxLength: maximum total length of obscured output (0 = no limit)
	// When set, the output will be EXACTLY this length (truncating or padding as needed).
	// This normalizes all obscured values to the same length, hiding the original length.
	//
	// Example with max_length=12:
	//   Short:  "abc123"       → "ab******23" (padded to 12)
	//   Medium: "1ea82440-9c3e" → "1ea8****9c3e" (exact fit)
	//   Long:   "1ea82440-9c3e-460b-8fc2-d19a23ab2651" → "1ea8****2651" (truncated to 12)
	//
	// Values shorter than PrefixLength + SuffixLength + 1 are shown as "[OBSCURED]"
	// to avoid revealing the actual value.
	MaxLength int `json:"max_length,omitempty"`
}



// FieldMaskConfigDoc wraps FieldMaskConfig with proper document binding tags
type FieldMaskConfig struct {
	DocumentModel

	// Version for forward compatibility
	Version string `json:"version,omitempty" doc:"version,omitempty"`

	// Scope identifier (must be non-empty)
	Scope string `json:"scope" doc:"scope"`

	// DefaultPolicy is applied when no explicit rule matches
	DefaultPolicy MaskedFieldPolicy `json:"default,omitempty" doc:"policy,omitempty"`

	// Fields maps field name to masking policy
	Fields map[string]MaskedFieldPolicy `json:"fields,omitempty" doc:"fields,omitempty"`

	// Patterns allows regex-based field matching
	Patterns []PatternRule `json:"patterns,omitempty" doc:"patterns,omitempty"`

	// ObscureConfig controls behavior of MaskObscure policy
	ObscureConfig ObscureConfig `json:"obscure" doc:"obscure,omitempty"`

	// HashSecret for HMAC hashing
	HashSecret string `json:"salt,omitempty" doc:"salt,omitempty"`

	// Description provides human-readable context
	Description string `json:"description,omitempty" doc:"description,omitempty"`
}

// DefaultObscureConfig provides sensible defaults for obscuring
func DefaultObscureConfig() ObscureConfig {
	return ObscureConfig{
		PrefixLength: 2,
		SuffixLength: 2,
		Replacement:  "*",
		MaxLength:    0, // No limit by default
	}
}

// Validate checks if the FieldMaskConfig is valid
func (c *FieldMaskConfig) Validate() error {
	var issues []common.Issue

	// Validate field policies
	for field, policy := range c.Fields {
		if _, err := ParseMaskedFieldPolicy(string(policy)); err != nil {
			issues = append(issues, common.Issue{
				Code:     "ERR_INVALID_POLICY",
				Message:  fmt.Sprintf("invalid policy %q for field %q", policy, field),
				Path:     fmt.Sprintf("fields.%s", field),
				Severity: common.SeverityError,
			})
		}
	}

	// Validate and compile patterns
	for i := range c.Patterns {
		pr := &c.Patterns[i]

		if pr.Pattern == "" {
			issues = append(issues, common.Issue{
				Code:     "ERR_EMPTY_PATTERN",
				Message:  "pattern string is empty",
				Path:     "patterns",
				Index:    &i,
				Severity: common.SeverityError,
			})
			continue
		}

		// Compile regex
		regex, err := regexp.Compile(pr.Pattern)
		if err != nil {
			issues = append(issues, common.Issue{
				Code:     "ERR_INVALID_REGEX",
				Message:  fmt.Sprintf("invalid regex: %v", err),
				Path:     "patterns",
				Index:    &i,
				Severity: common.SeverityError,
			})
			continue
		}
		pr.regex = regex // Store compiled regex

		// Validate policy
		if _, err := ParseMaskedFieldPolicy(string(pr.Policy)); err != nil {
			issues = append(issues, common.Issue{
				Code:     "ERR_INVALID_POLICY",
				Message:  fmt.Sprintf("invalid policy %q", pr.Policy),
				Path:     "patterns",
				Index:    &i,
				Severity: common.SeverityError,
			})
		}
	}

	// Validate default policy
	if c.DefaultPolicy != "" {
		if _, err := ParseMaskedFieldPolicy(string(c.DefaultPolicy)); err != nil {
			issues = append(issues, common.Issue{
				Code:     "ERR_INVALID_POLICY",
				Message:  fmt.Sprintf("invalid default policy %q", c.DefaultPolicy),
				Path:     "default_policy",
				Severity: common.SeverityError,
			})
		}
	} else {
		c.DefaultPolicy = MaskPreserve // Set default
	}

	// Validate obscure config
	if c.ObscureConfig.Replacement == "" {
		c.ObscureConfig = DefaultObscureConfig()
	} else {
		if c.ObscureConfig.PrefixLength < 0 {
			issues = append(issues, common.Issue{
				Code:     "ERR_INVALID_CONFIG",
				Message:  "prefix_length must be >= 0",
				Path:     "obscure.prefix_length",
				Severity: common.SeverityError,
			})
		}
		if c.ObscureConfig.SuffixLength < 0 {
			issues = append(issues, common.Issue{
				Code:     "ERR_INVALID_CONFIG",
				Message:  "suffix_length must be >= 0",
				Path:     "obscure.suffix_length",
				Severity: common.SeverityError,
			})
		}
		if c.ObscureConfig.MaxLength < 0 {
			issues = append(issues, common.Issue{
				Code:     "ERR_INVALID_CONFIG",
				Message:  "max_length must be >= 0",
				Path:     "obscure.max_length",
				Severity: common.SeverityError,
			})
		}
		// Warn if max_length is set but too small to be useful
		if c.ObscureConfig.MaxLength > 0 {
			minViable := c.ObscureConfig.PrefixLength + c.ObscureConfig.SuffixLength + 1
			if c.ObscureConfig.MaxLength < minViable {
				issues = append(issues, common.Issue{
					Code:     "ERR_INVALID_CONFIG",
					Message:  fmt.Sprintf("max_length (%d) is too small for prefix (%d) + suffix (%d) + 1", c.ObscureConfig.MaxLength, c.ObscureConfig.PrefixLength, c.ObscureConfig.SuffixLength),
					Path:     "obscure.max_length",
					Severity: common.SeverityWarning,
				})
			}
		}
	}

	// Validate hash secret if provided
	if c.HashSecret != "" {
		secret, err := hex.DecodeString(c.HashSecret)
		if err != nil {
			issues = append(issues, common.Issue{
				Code:     "ERR_INVALID_CONFIG",
				Message:  "hash_secret must be hex-encoded",
				Path:     "hash_secret",
				Severity: common.SeverityError,
			})
		} else if len(secret) < 16 {
			issues = append(issues, common.Issue{
				Code:     "ERR_INVALID_CONFIG",
				Message:  "hash_secret too short: must be at least 16 bytes (32 hex chars)",
				Path:     "hash_secret",
				Severity: common.SeverityError,
			})
		}
	}

	var errorsOnly []common.Issue
	for _, issue := range issues {
		if issue.Severity == common.SeverityError {
			errorsOnly = append(errorsOnly, issue)
		}
	}

	if len(errorsOnly) > 0 {
		return common.NewSystemError("ERR_SANITIZATION_CONFIG_INVALID").
			WithMessage("sanitization config validation failed").
			WithIssues(errorsOnly)
	}

	return nil
}

// ============================================================================
// Sanitizer - Core Logic
// ============================================================================

// Sanitizer handles field masking based on configuration
type Sanitizer struct {
	config     FieldMaskConfig
	logger     *zap.Logger
	hashSecret []byte // Secret key for HMAC hashing (prevents rainbow table attacks)
}

// NewSanitizer creates a new field sanitizer with the given configuration.
// Generates a unique HMAC secret for this sanitizer instance to prevent
// rainbow table attacks on hashed values, unless config.HashSecret is provided.
func NewSanitizer(config FieldMaskConfig, logger *zap.Logger) *Sanitizer {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Validate and set defaults (mutates config)
	if err := config.Validate(); err != nil {
		logger.Error("Invalid sanitizer config, using defaults", zap.Error(err))
		config = FieldMaskConfig{
			DefaultPolicy: MaskPreserve,
			ObscureConfig: DefaultObscureConfig(),
		}
	}

	// Use provided secret or generate a random one
	var hashSecret []byte
	if config.HashSecret != "" {
		// Decode hex string to bytes
		var err error
		hashSecret, err = hex.DecodeString(config.HashSecret)
		if err != nil || len(hashSecret) < 16 {
			logger.Warn("Invalid hash secret, generating random",
				zap.Error(err))
			hashSecret = nil
		} else {
			logger.Debug("Using provided hash secret for sanitizer")
		}
	}

	if hashSecret == nil {
		// Generate a random secret for HMAC hashing
		hashSecret = make([]byte, 32) // 256 bits
		if _, err := rand.Read(hashSecret); err != nil {
			// Fallback to deterministic but still secret value
			logger.Warn("Failed to generate random hash secret, using fallback",
				zap.Error(err))
			hashSecret = []byte("fallback-secret-key-change-in-production-" + time.Now().String())
		}
	}

	return &Sanitizer{
		config:     config,
		logger:     logger,
		hashSecret: hashSecret,
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

	// 2. Check pattern-based rules (scoped patterns checked first due to merge order)
	for _, pattern := range s.config.Patterns {
		if pattern.regex != nil && pattern.regex.MatchString(fieldName) {
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

// hashValue creates a salted hash of the value for auditing purposes.
// Uses HMAC-SHA256 with a per-sanitizer secret to prevent rainbow table attacks.
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

	// Create HMAC-SHA256 hash with secret key
	// This prevents rainbow table attacks since attacker doesn't have the key
	h := hmac.New(sha256.New, s.hashSecret)
	h.Write([]byte(str))
	hash := h.Sum(nil)

	// Return first 8 characters of hex encoding (32 bits)
	// This is enough for collision detection in logs without exposing data
	return fmt.Sprintf("[HASH:%s]", hex.EncodeToString(hash)[:8])
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
	prefixLen := s.config.ObscureConfig.PrefixLength
	suffixLen := s.config.ObscureConfig.SuffixLength
	replacementChar := s.config.ObscureConfig.Replacement
	maxLength := s.config.ObscureConfig.MaxLength

	// Handle cases where the value is too short to effectively obscure
	if length <= prefixLen+suffixLen+1 {
		return "[OBSCURED]"
	}

	// Determine the parts of the original string to keep
	prefixPart := str[:prefixLen]
	suffixPart := str[length-suffixLen:]

	// Calculate the default obscured length (if no maxLength is applied)
	defaultObscuredLen := length - prefixLen - suffixLen

	// Determine the actual obscured length based on maxLength
	var finalObscuredLen int
	if maxLength > 0 {
		// Calculate how many replacement chars are needed to achieve maxLength
		// total length = prefixLen + finalObscuredLen + suffixLen
		// finalObscuredLen = maxLength - prefixLen - suffixLen
		calculatedObscuredLenForMaxLength := maxLength - prefixLen - suffixLen

		// If the calculated length for replacement chars is negative or too small,
		// it means maxLength is too small to fit prefix, suffix, and at least one replacement.
		if calculatedObscuredLenForMaxLength < 1 {
			return "[OBSCURED]" // Unviable maxLength, return "[OBSCURED]"
		}
		finalObscuredLen = calculatedObscuredLenForMaxLength
	} else {
		// If maxLength is 0 (no limit), use the default obscured length
		finalObscuredLen = defaultObscuredLen
	}

	// Construct the final obscured string
	return fmt.Sprintf("%s%s%s",
		prefixPart,
		strings.Repeat(replacementChar, finalObscuredLen),
		suffixPart)
}

// ============================================================================
// Pattern Compilation Helpers
// ============================================================================

// CompilePattern compiles a regex pattern and creates a PatternRule.
// Use this for programmatic pattern creation.
func CompilePattern(pattern string, policy MaskedFieldPolicy) (PatternRule, error) {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return PatternRule{}, common.NewSystemError("ERR_SANITIZATION_PATTERN_INVALID").
			WithMessagef("failed to compile pattern %q", pattern).
			WithCause(err)
	}
	return PatternRule{
		Pattern: pattern,
		Policy:  policy,
		regex:   regex,
	}, nil
}

// MustCompilePattern compiles a pattern and panics on error.
// Use this only for static initialization at startup.
func MustCompilePattern(pattern string, policy MaskedFieldPolicy) PatternRule {
	pr, err := CompilePattern(pattern, policy)
	if err != nil {
		panic(err)
	}
	return pr
}

// CommonSecurityPatterns returns commonly used patterns for credential fields
func CommonSecurityPatterns() []PatternRule {
	return []PatternRule{
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
		Version:       "v1",
		Fields:        make(map[string]MaskedFieldPolicy),
		Patterns:      CommonSecurityPatterns(),
		DefaultPolicy: MaskPreserve,
		ObscureConfig: DefaultObscureConfig(),
	}
}

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

// Reserved metadata field names that should never be sanitized
var reservedMetadataFields = []string{
	MetadataVersion,
	MetadataCreated,
	MetadataUpdated,
	MetadataChecksum,
	MetadataSignature,
}

// isSystemMetadataField checks if a field is a reserved system metadata field
func isSystemMetadataField(key string) bool {
	return slices.Contains(reservedMetadataFields, key)
}

// sanitizeMetadata handles metadata specially - preserve system fields, sanitize user fields
func (ds *DocumentSanitizer) sanitizeMetadata(metadata map[string]any) map[string]any {
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

// ============================================================================
// Configuration Merging
// ============================================================================

// mergeConfigs merges scoped config with global config.
// Scoped config takes precedence for conflicts (more restrictive wins).
func mergeConfigs(globalConfig *FieldMaskConfig, scopedConfig FieldMaskConfig) FieldMaskConfig {
	merged := FieldMaskConfig{
		Fields:   make(map[string]MaskedFieldPolicy),
		Patterns: []PatternRule{},
	}

	// Start with global field mappings (lower priority)
	if globalConfig != nil {
		maps.Copy(merged.Fields, globalConfig.Fields)
	}

	// Override with scoped field mappings (higher priority)
	maps.Copy(merged.Fields, scopedConfig.Fields)

	// Patterns: Build map to handle duplicates, scoped overrides global
	patternMap := make(map[string]PatternRule)

	// Add global patterns first (lower priority)
	if globalConfig != nil {
		for _, pattern := range globalConfig.Patterns {
			patternMap[pattern.regex.String()] = pattern
		}
	}

	// Scoped patterns override global if same regex
	for _, pattern := range scopedConfig.Patterns {
		patternMap[pattern.regex.String()] = pattern
	}

	// Convert back to slice, preserving insertion order for deterministic behavior
	// Scoped patterns checked first, then global
	for _, pattern := range scopedConfig.Patterns {
		merged.Patterns = append(merged.Patterns, patternMap[pattern.regex.String()])
		delete(patternMap, pattern.regex.String()) // Remove to avoid duplicates
	}
	if globalConfig != nil {
		for _, pattern := range globalConfig.Patterns {
			if p, exists := patternMap[pattern.regex.String()]; exists {
				merged.Patterns = append(merged.Patterns, p)
			}
		}
	}

	// Scoped default policy overrides global, or use global if not set
	if scopedConfig.DefaultPolicy != "" {
		merged.DefaultPolicy = scopedConfig.DefaultPolicy
	} else if globalConfig != nil {
		merged.DefaultPolicy = globalConfig.DefaultPolicy
	}

	// Use scoped obscure config if provided, otherwise use global
	if scopedConfig.ObscureConfig.Replacement != "" {
		merged.ObscureConfig = scopedConfig.ObscureConfig
	} else if globalConfig != nil {
		merged.ObscureConfig = globalConfig.ObscureConfig
	}

	// Merge HashSecret: Scoped takes precedence
	if scopedConfig.HashSecret != "" {
		merged.HashSecret = scopedConfig.HashSecret
	} else if globalConfig != nil {
		merged.HashSecret = globalConfig.HashSecret
	}

	return merged
}

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
func (d *Document) Sanitize(ctx ...context.Context) (*Document, error) {
	sanitizer, err := getFactory().getSanitizersForContexts(d.ctx, ctx...)
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
	sanitizedMetadata := sanitizer.sanitizeMetadata(d.metadata)

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
func SanitizeDocumentArray(docs []*Document) ([]*Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	sanitized := make([]*Document, len(docs))
	for i, doc := range docs {
		var err error
		sanitized[i], err = doc.Sanitize()
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
	}
	return sanitized, nil
}

// SanitizeDocumentArrayWithContexts sanitizes documents with per-document contexts.
func SanitizeDocumentArrayWithContexts(docs []*Document, contexts []context.Context) ([]*Document, error) {
	if len(docs) != len(contexts) {
		return nil, common.NewSystemError("ERR_SANITIZATION_CONFIG_INVALID").
			WithMessage("docs and contexts length mismatch").
			WithMessagef("expected %d contexts for %d documents", len(docs), len(contexts))
	}

	sanitized := make([]*Document, len(docs))
	for i, doc := range docs {
		var err error
		sanitized[i], err = doc.Sanitize(contexts[i])
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
	}
	return sanitized, nil
}

// SanitizeValue sanitizes any value that might contain documents.
// For Documents, uses their embedded context. For raw maps, uses provided context.
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
		return sanitized.data, nil

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
			sanitized[i] = sanitizedDoc.data
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

// ============================================================================
// Scope Management (Public API)
// ============================================================================

// GetScopedSanitizer retrieves the sanitizer for a specific scope.
// Returns nil if no scope-specific sanitizer exists (global will be used).
//
// This is useful for testing or manual sanitization.
// In normal operation, use Document.Sanitize(ctx) instead.
func GetScopedSanitizer(scopeID string) *DocumentSanitizer {
	registry := GetSanitizationRegistry()
	if registry == nil {
		return nil
	}

	return registry.Get(scopeID)
}

