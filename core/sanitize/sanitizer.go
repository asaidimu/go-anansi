package sanitize

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

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

// @note #sanitize-type-blindness issue status=open priority=P1 tags=#sanitizer,#types : applyPolicy is type-blind and cannot mask non-string fields
//
// applyPolicy/hashValue stringify every value (fmt.Sprintf %v), so a policy
// match on a non-string slot (array/object/number) produces a string. The
// container pipeline (core/document/sanitize.go sanitizeContainerLeaves)
// correctly refuses to write a masked string into a non-string slot and
// returns "cannot store sanitized value ... in <type> slot", which fails
// Sanitize() for the WHOLE document. Real-world fallout: hestia's global
// (?i)auth pattern matched the array field `authors` of its dynamic `notes`
// collection, so every document failed sanitization and the query API
// silently returned data:[] with correct pagination counts (the HTTP layer
// drops documents whose Sanitize errors).
//
// Decision (2026-08-22): fix belongs at the app config level for now (keep
// masking rules targeted/collection-scoped so patterns only ever hit string
// fields); no library change shipped. If this is ever fixed here, likely
// shapes are: element-wise masking for arrays of strings, skipping
// non-maskable slots with a warning, or schema-type-aware policies threaded
// through FieldMaskConfig.
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
