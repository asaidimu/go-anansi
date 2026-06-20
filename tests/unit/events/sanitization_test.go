package events_test

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestMaskRedact(t *testing.T) {

	config :=&data.FieldMaskConfig{
		Fields: map[string]data.MaskedFieldPolicy{
			"password": data.MaskRedact,
		},
		DefaultPolicy: data.MaskPreserve,
	}

	sanitizer :=data.NewSanitizer(*config, zap.NewNop())
	doc := map[string]any{
		"username": "admin",
		"password": "SuperSecret123!",
		"email":    "admin@example.com",
	}

	sanitized := sanitizer.SanitizeDocument(doc)

	assert.Equal(t, "admin", sanitized["username"])
	assert.Equal(t, "***", sanitized["password"])
	assert.Equal(t, "admin@example.com", sanitized["email"])
}

func TestMaskHash(t *testing.T) {

	config :=&data.FieldMaskConfig{

		Fields: map[string]data.MaskedFieldPolicy{

			"api_key": data.MaskHash,

		},

		DefaultPolicy: data.MaskPreserve,

	}



	sanitizer :=data.NewSanitizer(*config, zap.NewNop())

	doc := map[string]any{
		"api_key": "sk-prod-abc123xyz789",
		"service": "production-db",
	}

	sanitized := sanitizer.SanitizeDocument(doc)

	// Hash should be deterministic and start with [HASH:
	hash1 := sanitized["api_key"].(string)
	assert.Contains(t, hash1, "[HASH:")
	assert.Equal(t, 15, len(hash1)) // [HASH: + 8 hex chars + ]

	// Same value should produce same hash
	sanitized2 := sanitizer.SanitizeDocument(doc)
	hash2 := sanitized2["api_key"].(string)
	assert.Equal(t, hash1, hash2)

	// Different value should produce different hash
	doc["api_key"] = "sk-prod-different789"
	sanitized3 := sanitizer.SanitizeDocument(doc)
	hash3 := sanitized3["api_key"].(string)
	assert.NotEqual(t, hash1, hash3)
}

func TestMaskObscure(t *testing.T) {
	config :=&data.FieldMaskConfig{
		Fields: map[string]data.MaskedFieldPolicy{
			"email":data.MaskObscure,
		},
		DefaultPolicy:data.MaskPreserve,
		ObscureConfig: data.ObscureConfig{
			PrefixLength: 2,
			SuffixLength: 2,
			Replacement:  "*",
		},
	}

	sanitizer :=data.NewSanitizer(*config, zap.NewNop())

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "email address",
			input:    "user@example.com",
			expected: "us************om",
		},
		{
			name:     "short value - fully obscured",
			input:    "short",
			expected: "[OBSCURED]",
		},
		{
			name:     "minimum length",
			input:    "abcdef",
			expected: "ab**ef",
		},
		{
			name:     "phone number",
			input:    "+254712345678",
			expected: "+2*********78",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := map[string]any{"email": tt.input}
			sanitized := sanitizer.SanitizeDocument(doc)
			assert.Equal(t, tt.expected, sanitized["email"])
		})
	}
}

func TestPatternMatching(t *testing.T) {
	config :=&data.FieldMaskConfig{
		Fields:   make(map[string]data.MaskedFieldPolicy),
		Patterns:data.CommonSecurityPatterns(),
		DefaultPolicy:data.MaskPreserve,
		ObscureConfig: data.DefaultObscureConfig(),
	}

	sanitizer :=data.NewSanitizer(*config, zap.NewNop())

	doc := map[string]any{
		// Should be redacted
		"user_password":    "secret123",
		"api_key":          "sk-abc123",
		"secret_token":     "token_xyz",
		"privateKey":       "-----BEGIN PRIVATE KEY-----",
		"ssn":              "123-45-6789",
		"credit_card":      "4532-1234-5678-9010",

		// Should be hashed
		"auth_header":      "Bearer xyz",

		// Should be obscured
		"email":            "user@example.com",
		"phone":            "+254712345678",

		// Should be preserved
		"id":               "USER001",
		"name":             "John Doe",
		"created_at":       "2024-01-01T00:00:00Z",
	}

	sanitized := sanitizer.SanitizeDocument(doc)

	// Check redacted fields
	assert.Equal(t, "***", sanitized["user_password"])
	assert.Equal(t, "***", sanitized["api_key"])
	assert.Equal(t, "***", sanitized["secret_token"])
	assert.Equal(t, "***", sanitized["privateKey"])
	assert.Equal(t, "***", sanitized["ssn"])
	assert.Equal(t, "***", sanitized["credit_card"])

	// Check hashed fields
	assert.Contains(t, sanitized["auth_header"].(string), "[HASH:")

	// Check obscured fields
	emailObscured := sanitized["email"].(string)
	assert.Contains(t, emailObscured, "*")
	assert.Equal(t, "us", emailObscured[:2])
	assert.Equal(t, "om", emailObscured[len(emailObscured)-2:])

	// Check preserved fields
	assert.Equal(t, "USER001", sanitized["id"])
	assert.Equal(t, "John Doe", sanitized["name"])
	assert.Equal(t, "2024-01-01T00:00:00Z", sanitized["created_at"])
}

func TestCustomPatternPriority(t *testing.T) {
	// Test that explicit field config takes priority over patterns
	config :=&data.FieldMaskConfig{
		Fields: map[string]data.MaskedFieldPolicy{
			"password":data.MaskObscure, // Explicit: obscure
		},
		Patterns: []data.PatternRule{
			data.MustCompilePattern(`(?i)password`,data.MaskRedact), // Pattern: redact
		},
		DefaultPolicy:data.MaskPreserve,
		ObscureConfig: data.DefaultObscureConfig(),
	}

	sanitizer :=data.NewSanitizer(*config, zap.NewNop())

	doc := map[string]any{
		"password":      "secret123",      // Should use explicit policy (obscure)
		"old_password":  "oldsecret456",   // Should use pattern policy (redact)
	}

	sanitized := sanitizer.SanitizeDocument(doc)

	// Explicit field should be obscured (takes priority)
	assert.Contains(t, sanitized["password"].(string), "*")
	assert.NotEqual(t, "***", sanitized["password"])

	// Pattern match should be redacted
	assert.Equal(t, "***", sanitized["old_password"])
}

func TestNilAndEmptyValues(t *testing.T) {
	config :=&data.FieldMaskConfig{
		Fields: map[string]data.MaskedFieldPolicy{
			"password":data.MaskRedact,
		},
		DefaultPolicy:data.MaskPreserve,
	}

	sanitizer :=data.NewSanitizer(*config, zap.NewNop())

	// Test nil document
	assert.Nil(t, sanitizer.SanitizeDocument(nil))

	// Test nil field value
	doc := map[string]any{
		"password": nil,
		"username": "admin",
	}

	sanitized := sanitizer.SanitizeDocument(doc)
	assert.Nil(t, sanitized["password"])
	assert.Equal(t, "admin", sanitized["username"])

	// Test empty string
	doc["password"] = ""
	sanitized = sanitizer.SanitizeDocument(doc)
	assert.Equal(t, "***", sanitized["password"])
}

func TestComplexDataStructures(t *testing.T) {
	config :=&data.FieldMaskConfig{
		Fields: map[string]data.MaskedFieldPolicy{
			"password":data.MaskRedact,
			"token":   data.MaskHash,
		},
		DefaultPolicy:data.MaskPreserve,
	}

	sanitizer :=data.NewSanitizer(*config, zap.NewNop())

	// Test nested maps
	doc := map[string]any{
		"user": map[string]any{
			"username": "admin",
			"password": "secret123",
		},
		"session": map[string]any{
			"token":      "session_xyz",
			"expires_at": "2024-12-31",
		},
	}

	sanitized := sanitizer.SanitizeDocument(doc)

	// Note: Current implementation sanitizes top-level fields only
	// For nested sanitization, you would need recursive handling
	// This test documents current behavior
	userMap := sanitized["user"].(map[string]any)
	assert.Equal(t, "admin", userMap["username"])
	// Password is nested, so not sanitized by current implementation
	assert.Equal(t, "secret123", userMap["password"])
}

func TestDifferentValueTypes(t *testing.T) {
	config :=&data.FieldMaskConfig{
		Fields: map[string]data.MaskedFieldPolicy{
			"count":  data.MaskHash,
			"active": data.MaskRedact,
			"balance":data.MaskObscure,
		},
		DefaultPolicy:data.MaskPreserve,
		ObscureConfig: data.DefaultObscureConfig(),
	}

	sanitizer :=data.NewSanitizer(*config, zap.NewNop())

	doc := map[string]any{
		"count":   42,
		"active":  true,
		"balance": 1234.56,
		"name":    "John Doe",
	}

	sanitized := sanitizer.SanitizeDocument(doc)

	// Non-string values are converted to string for sanitization
	assert.Contains(t, sanitized["count"].(string), "[HASH:")
	assert.Equal(t, "***", sanitized["active"])
	assert.Contains(t, sanitized["balance"].(string), "*")
	assert.Equal(t, "John Doe", sanitized["name"])
}

func TestDefaultPolicy(t *testing.T) {
	tests := []struct {
		name          string
		defaultPolicy data.MaskedFieldPolicy
		fieldValue    string
		expectRedact  bool
	}{
		{
			name:          "default preserve",
			defaultPolicy:data.MaskPreserve,
			fieldValue:    "sensitive_data",
			expectRedact:  false,
		},
		{
			name:          "default redact",
			defaultPolicy:data.MaskRedact,
			fieldValue:    "sensitive_data",
			expectRedact:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config :=&data.FieldMaskConfig{
				Fields:        make(map[string]data.MaskedFieldPolicy),
				Patterns:      []data.PatternRule{},
				DefaultPolicy: tt.defaultPolicy,
			}

			sanitizer :=data.NewSanitizer(*config, zap.NewNop())

			doc := map[string]any{
				"unknown_field": tt.fieldValue,
			}

			sanitized := sanitizer.SanitizeDocument(doc)

			if tt.expectRedact {
				assert.Equal(t, "***", sanitized["unknown_field"])
			} else {
				assert.Equal(t, tt.fieldValue, sanitized["unknown_field"])
			}
		})
	}
}

func TestMustCompilePatternPanic(t *testing.T) {
	assert.Panics(t, func() {
		data.MustCompilePattern("[invalid(regex",data.MaskRedact)
	})
}

func TestCommonSecurityPatterns(t *testing.T) {
	patterns :=data.CommonSecurityPatterns()
	assert.Greater(t, len(patterns), 0)

	// Verify all patterns compile
	for _, pattern := range patterns {
		assert.NotNil(t, pattern.Pattern)
		assert.NotEmpty(t, pattern.Policy)
	}
}

func TestNewSecureDefaultConfig(t *testing.T) {
	config :=data.NewSecureDefaultConfig()

	assert.NotNil(t, config.Fields)
	assert.NotEmpty(t, config.Patterns)
	assert.Equal(t,data.MaskPreserve, config.DefaultPolicy)
	assert.NotEmpty(t, config.ObscureConfig.Replacement)
}

func TestSanitizeValue(t *testing.T) {
	config := &data.FieldMaskConfig{
		Fields: map[string]data.MaskedFieldPolicy{
			"password":data.MaskRedact,
		},
		DefaultPolicy:data.MaskPreserve,
	}

	sanitizer :=data.NewSanitizer(*config, zap.NewNop())

	// Test single value sanitization
	result := sanitizer.SanitizeValue("password", "secret123")
	assert.Equal(t, "***", result)

	result = sanitizer.SanitizeValue("username", "admin")
	assert.Equal(t, "admin", result)
}

func BenchmarkSanitizeDocument(b *testing.B) {
	config :=data.NewSecureDefaultConfig()
	sanitizer :=data.NewSanitizer(*config, zap.NewNop())

	doc := map[string]any{
		"id":               "USER001",
		"username":         "john_doe",
		"password":         "SuperSecret123!",
		"email":            "john@example.com",
		"api_key":          "sk-prod-abc123xyz789",
		"phone":            "+254712345678",
		"created_at":       "2024-01-01T00:00:00Z",
		"last_login":       "2024-12-29T10:30:00Z",
		"account_balance":  1234.56,
		"is_active":        true,
	}


	for b.Loop() {
		sanitizer.SanitizeDocument(doc)
	}
}

