package sanitize

import (
	"encoding/hex"
	"fmt"
	"regexp"

	"github.com/asaidimu/go-anansi/v8/core/common"
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

// DefaultObscureConfig provides sensible defaults for obscuring
func DefaultObscureConfig() ObscureConfig {
	return ObscureConfig{
		PrefixLength: 2,
		SuffixLength: 2,
		Replacement:  "*",
		MaxLength:    0, // No limit by default
	}
}

// FieldMaskConfig defines how a set of fields should be masked. It is a plain
// data structure (no document model embed) so the sanitizer package stays free
// of document/container dependencies. Persistence backends adapt it to their
// own record models.
type FieldMaskConfig struct {
	// Version for forward compatibility
	Version string `json:"version,omitempty" anansi:"version,omitempty" doc:"version,omitempty"`

	// Scope identifier (must be non-empty)
	Scope string `json:"scope" anansi:"scope" doc:"scope"`

	// DefaultPolicy is applied when no explicit rule matches
	DefaultPolicy MaskedFieldPolicy `json:"default,omitempty" anansi:"policy,omitempty" doc:"policy,omitempty"`

	// Fields maps field name to masking policy
	Fields map[string]MaskedFieldPolicy `json:"fields,omitempty" anansi:"fields,omitempty" doc:"fields,omitempty"`

	// Patterns allows regex-based field matching
	Patterns []PatternRule `json:"patterns,omitempty" anansi:"patterns,omitempty" doc:"patterns,omitempty"`

	// ObscureConfig controls behavior of MaskObscure policy
	ObscureConfig ObscureConfig `json:"obscure" anansi:"obscure,omitempty" doc:"obscure,omitempty"`

	// HashSecret for HMAC hashing
	HashSecret string `json:"salt,omitempty" anansi:"salt,omitempty" doc:"salt,omitempty"`

	// Description provides human-readable context
	Description string `json:"description,omitempty" anansi:"description,omitempty" doc:"description,omitempty"`
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
