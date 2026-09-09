package sanitize

import (
	"regexp"

	"github.com/asaidimu/go-anansi/v8/core/common"
)

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
