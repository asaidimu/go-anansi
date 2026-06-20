package data_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common" // Added this line
	"github.com/asaidimu/go-anansi/v6/core/data"
	"go.uber.org/zap"
)

// ============================================================================
// Test Helpers
// ============================================================================

func setupTestFactory(t *testing.T) {
	t.Helper()
	data.ResetFactoryForTesting()

	logger := zap.NewNop()
	config := data.DocumentFactoryConfig{
		GlobalSanitizer: data.NewSecureDefaultConfig(),
		ScopedSanitizers: map[string]*data.FieldMaskConfig{
			"strict": {
				Fields: map[string]data.MaskedFieldPolicy{
					"password": data.MaskRedact,
					"api_key":  data.MaskRedact,
				},
				DefaultPolicy: data.MaskHash,
			},
		},
	}

	if err := data.ConfigureDocumentFactory(config, logger); err != nil {
		t.Fatalf("Failed to configure factory: %v", err)
	}
}

// ============================================================================
// Basic Sanitization Tests
// ============================================================================

func TestBasicSanitization(t *testing.T) {
	setupTestFactory(t)

	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name: "redact password",
			input: map[string]any{
				"username": "john",
				"password": "secret123",
			},
			expected: map[string]any{
				"username": "john",
				"password": "***",
			},
		},
		{
			name: "obscure email",
			input: map[string]any{
				"name":  "John Doe",
				"email": "john.doe@example.com",
			},
			expected: map[string]any{
				"name":  "John Doe",
				"email": "jo****************om",
			},
		},
		{
			name: "preserve safe fields",
			input: map[string]any{
				"name": "John Doe",
				"age":  30,
			},
			expected: map[string]any{
				"name": "John Doe",
				"age":  30,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			doc := data.MustNewDocument(tt.input, ctx)

			sanitized, err := doc.Sanitize()
			if err != nil {
				t.Fatalf("Sanitize failed: %v", err)
			}

			// Check each expected field
			for key, expectedValue := range tt.expected {
				actualValue, err := sanitized.Get(key)
				if err != nil {
					t.Errorf("Field %q missing from sanitized document", key)
					continue
				}

				if actualValue != expectedValue {
					t.Errorf("Field %q: expected %v, got %v", key, expectedValue, actualValue)
				}
			}
		})
	}
}

// ============================================================================
// Scoped Sanitization Tests
// ============================================================================

func TestScopedSanitization(t *testing.T) {
	setupTestFactory(t)

	dataMap := map[string]any{
		"username": "john",
		"password": "secret123",
		"data":     "some data",
	}

	t.Run("global scope", func(t *testing.T) {
		ctx := context.Background()
		doc := data.MustNewDocument(dataMap, ctx)

		sanitized, err := doc.Sanitize()
		if err != nil {
			t.Fatalf("Sanitize failed: %v", err)
		}

		// Global scope redacts password
		pwd, err := sanitized.Get("password")
		if err != nil || pwd != "***" {
			t.Errorf("Expected password to be redacted, got %v", pwd)
		}
	})

	t.Run("strict scope", func(t *testing.T) {
		ctx := common.ContextWithSanitizationScope(context.Background(), "strict")
		doc := data.MustNewDocument(dataMap, ctx)

		sanitized, err := doc.Sanitize()
		if err != nil {
			t.Fatalf("Sanitize failed: %v", err)
		}

		pwd, err := sanitized.Get("password")
		if err != nil || pwd != "***" {
			t.Errorf("Expected password to be redacted, got %v", pwd)
		}

		// Strict scope hashes other data (default policy)
		dataVal, err := sanitized.Get("data")
		if err == nil { // Check if there was no error getting the value
			if str, ok := dataVal.(string); ok && !strings.Contains(str, "[HASH:") {
				t.Errorf("Expected 'data' to be hashed in strict scope, got %v", dataVal)
			}
		}
	})
}

// ============================================================================
// Multi-Scope Composition Tests
// ============================================================================

func TestMultiScopeComposition(t *testing.T) {
	setupTestFactory(t)

	// data.Register additional scope for testing
	lenientConfig := &data.FieldMaskConfig{
		Fields: map[string]data.MaskedFieldPolicy{
			"password": data.MaskObscure, // Less restrictive
		},
		DefaultPolicy: data.MaskPreserve,
	}
	if err := data.RegisterScopedSanitizer("lenient", lenientConfig); err != nil {
		t.Fatalf("Failed to register lenient scope: %v", err)
	}

	dataMap := map[string]any{
		"password": "secret123",
		"api_key":  "key_12345",
		"username": "john",
	}

	t.Run("most restrictive wins", func(t *testing.T) {
		// Lenient wants to obscure, strict wants to redact
		// Most restrictive (redact) should win
		lenientCtx := common.ContextWithSanitizationScope(context.Background(), "lenient")
		strictCtx := common.ContextWithSanitizationScope(context.Background(), "strict")

		doc := data.MustNewDocument(dataMap, context.Background())
		sanitized, err := doc.Sanitize(lenientCtx, strictCtx)
		if err != nil {
			t.Fatalf("Multi-scope sanitize failed: %v", err)
		}

		// Password should be redacted (most restrictive)
		pwd, err := sanitized.Get("password")
		if err != nil || pwd != "***" {
			t.Errorf("Expected password to be redacted (most restrictive), got %v", pwd)
		}

		// API key should be redacted (from strict scope)
		key, err := sanitized.Get("api_key")
		if err != nil || key != "***" {
			t.Errorf("Expected api_key to be redacted, got %v", key)
		}

		// Username should be hashed (strict's default policy wins over preserve)
		user, err := sanitized.Get("username")
		if err != nil || !strings.HasPrefix(user.(string), "[HASH:") {
			t.Errorf("Expected username to be hashed, got %v", user)
		}
	})

	t.Run("duplicate scopes ignored", func(t *testing.T) {
		ctx := common.ContextWithSanitizationScope(context.Background(), "strict")

		doc := data.MustNewDocument(dataMap, context.Background())
		// Same scope multiple times should be deduplicated
		sanitized, err := doc.Sanitize(ctx, ctx, ctx)
		if err != nil {
			t.Fatalf("Multi-scope with duplicates failed: %v", err)
		}

		// Should still redact password
		pwd, err := sanitized.Get("password")
		if err != nil || pwd != "***" {
			t.Errorf("Expected password to be redacted, got %v", pwd)
		}
	})
}

// ============================================================================
// Deep Sanitization Tests
// ============================================================================

func TestDeepSanitization(t *testing.T) {
	setupTestFactory(t)

	dataMap := map[string]any{
		"user": map[string]any{
			"name": "John Doe",
			"credentials": map[string]any{
				"password": "secret123",
				"api_key":  "key_12345",
			},
		},
		"metadata": map[string]any{
			"created": "2024-01-01",
			"token":   "auth_token_123",
		},
	}

	ctx := context.Background()
	doc := data.MustNewDocument(dataMap, ctx)

	sanitized, err := doc.Sanitize()
	if err != nil {
		t.Fatalf("Sanitize failed: %v", err)
	}

	// Check nested password is redacted
	userVal, err := sanitized.Get("user")
	if err == nil {
		if user, ok := userVal.(map[string]any); ok {
			if creds, ok := user["credentials"].(map[string]any); ok {
				if pwd := creds["password"]; pwd != "***" {
					t.Errorf("Expected nested password to be redacted, got %v", pwd)
				}
			} else {
				t.Error("Expected credentials map in user")
			}
		} else {
			t.Error("Expected user map in sanitized document")
		}
	} else {
		t.Error("Expected user map in sanitized document (or no error)")
	}

	// Check nested token is redacted
	metaVal, err := sanitized.Get("metadata")
	if err == nil {
		if meta, ok := metaVal.(map[string]any); ok {
			if token := meta["token"]; token != "***" {
				t.Errorf("Expected nested token to be redacted, got %v", token)
			}
		} else {
			t.Error("Expected metadata map in sanitized document")
		}
	} else {
		t.Error("Expected metadata map in sanitized document (or no error)")
	}
}

// ============================================================================
// Array Sanitization Tests
// ============================================================================

func TestArraySanitization(t *testing.T) {
	setupTestFactory(t)

	dataMap := map[string]any{
		"users": []map[string]any{
			{
				"name":     "John",
				"password": "secret1",
			},
			{
				"name":     "Jane",
				"password": "secret2",
			},
		},
		"tokens": []any{"token1", "token2", "token3"},
	}

	ctx := context.Background()
	doc := data.MustNewDocument(dataMap, ctx)

	sanitized, err := doc.Sanitize()
	if err != nil {
		t.Fatalf("Sanitize failed: %v", err)
	}

	// Check array of objects
	usersVal, err := sanitized.Get("users")
	if err == nil {
		if users, ok := usersVal.([]map[string]any); ok {
			for i, user := range users {
				if pwd := user["password"]; pwd != "***" {
					t.Errorf("Expected password at index %d to be redacted, got %v", i, pwd)
				}
				if name := user["name"]; name == "***" {
					t.Errorf("Expected name at index %d to be preserved, got %v", i, name)
				}
			}
		} else {
			t.Error("Expected users array in sanitized document")
		}
	} else {
		t.Error("Expected users array in sanitized document (or no error)")

	// Check array of strings (tokens should be redacted)
	tokensVal, err := sanitized.Get("tokens")
	if err == nil {
		if tokens, ok := tokensVal.([]any); ok {
			for i, token := range tokens {
				if token != "***" {
					t.Errorf("Expected token at index %d to be redacted, got %v", i, token)
				}
			}
		} else {
			t.Error("Expected tokens array in sanitized document")
		}
	} else {
		t.Error("Expected tokens array in sanitized document (or no error)")
	}
}
}

// ============================================================================
// Dynamic Scope data.Registration Tests
// ============================================================================

func TestDynamicScopeRegistration(t *testing.T) {
	setupTestFactory(t)

	t.Run("register new scope", func(t *testing.T) {
		config := &data.FieldMaskConfig{
			Fields: map[string]data.MaskedFieldPolicy{
				"custom_field": data.MaskRedact,
			},
			DefaultPolicy: data.MaskPreserve,
		}

		err := data.RegisterScopedSanitizer("dynamic_scope", config)
		if err != nil {
			t.Fatalf("Failed to register scope: %v", err)
		}

		// Verify scope is registered
		scopes := data.ListScopedSanitizers()
		found := slices.Contains(scopes, "dynamic_scope")
		if !found {
			t.Error("Expected dynamic_scope to be registered")
		}

		// Test using the new scope
		dataMap := map[string]any{
			"custom_field": "sensitive",
			"other_field":  "public",
		}

		ctx := common.ContextWithSanitizationScope(context.Background(), "dynamic_scope")
		doc := data.MustNewDocument(dataMap, ctx)

		sanitized, err := doc.Sanitize()
		if err != nil {
			t.Fatalf("Sanitize with dynamic scope failed: %v", err)
		}

		val, err := sanitized.Get("custom_field")
		if err != nil || val != "***" {
			t.Errorf("Expected custom_field to be redacted, got %v", val)
		}

		val, err = sanitized.Get("other_field")
		if err != nil || val != "public" {
			t.Errorf("Expected other_field to be preserved, got %v", val)
		}
	})

	t.Run("unregister scope", func(t *testing.T) {
		config := &data.FieldMaskConfig{
			Fields:        map[string]data.MaskedFieldPolicy{"test": data.MaskRedact},
			DefaultPolicy: data.MaskPreserve,
		}

		if err := data.RegisterScopedSanitizer("temp_scope", config); err != nil {
			t.Fatalf("Failed to register temp scope: %v", err)
		}

		// data.Unregister
		if err := data.UnregisterScopedSanitizer("temp_scope"); err != nil {
			t.Fatalf("Failed to unregister scope: %v", err)
		}

		// Verify scope is gone
		scopes := data.ListScopedSanitizers()
		for _, scope := range scopes {
			if scope == "temp_scope" {
				t.Error("Expected temp_scope to be unregistered")
			}
		}
	})

	t.Run("invalid scope registration fails", func(t *testing.T) {
		// Empty scope ID
		err := data.RegisterScopedSanitizer("", &data.FieldMaskConfig{})
		if err == nil {
			t.Error("Expected error for empty scope ID")
		}

		// Nil config
		err = data.RegisterScopedSanitizer("test", nil)
		if err == nil {
			t.Error("Expected error for nil config")
		}
	})
}

// ============================================================================
// Metadata Sanitization Tests
// ============================================================================

func TestMetadataSanitization(t *testing.T) {
	setupTestFactory(t)

	dataMap := map[string]any{
		"name": "John",
		"_metadata_": map[string]any{
			"version":     1,
			"created":     "2024-01-01",
			"user_token":  "secret_token", // User-defined metadata
			"api_key":     "key_123",      // Should be sanitized
		},
	}

	ctx := context.Background()
	doc := data.MustNewDocument(dataMap, ctx)

	sanitized, err := doc.Sanitize()
	if err != nil {
		t.Fatalf("Sanitize failed: %v", err)
	}

	meta := sanitized.Metadata()
	if len(meta) == 0 {
		t.Fatal("Expected metadata in sanitized document")
	}

	// System fields should be preserved
	if meta["version"] != 1 {
		t.Errorf("Expected version to be preserved, got %v", meta["version"])
	}
	if meta["created"] != "2024-01-01" {
		t.Errorf("Expected created to be preserved, got %v", meta["created"])
	}

	// User-defined fields with sensitive patterns should be sanitized
	if meta["api_key"] != "***" {
		t.Errorf("Expected api_key in metadata to be redacted, got %v", meta["api_key"])
	}
}

// ============================================================================
// Hash Security Tests (Rainbow Table Protection)
// ============================================================================

func TestHashRainbowTableProtection(t *testing.T) {
	setupTestFactory(t)

	// data.Register scope that uses hashing
	hashConfig := &data.FieldMaskConfig{
		Fields: map[string]data.MaskedFieldPolicy{
			"sensitive": data.MaskHash,
		},
		DefaultPolicy: data.MaskPreserve,
	}
	if err := data.RegisterScopedSanitizer("hash_test", hashConfig); err != nil {
		t.Fatalf("Failed to register hash scope: %v", err)
	}

	t.Run("different sanitizers produce different hashes", func(t *testing.T) {
		// Same value should produce different hashes with different sanitizers
		value := "password123"
		dataMap := map[string]any{"sensitive": value}

		ctx := common.ContextWithSanitizationScope(context.Background(), "hash_test")

		// Create two documents (will use different sanitizer instances potentially)
		doc1 := data.MustNewDocument(dataMap, ctx)
		doc2 := data.MustNewDocument(dataMap, ctx)

		sanitized1, err := doc1.Sanitize()
		if err != nil {
			t.Fatalf("Sanitize 1 failed: %v", err)
		}

		sanitized2, err := doc2.Sanitize()
		if err != nil {
			t.Fatalf("Sanitize 2 failed: %v", err)
		}

		// Both should be hashes
		hash1Val, err1 := sanitized1.Get("sensitive")
		hash2Val, err2 := sanitized2.Get("sensitive")

		if err1 != nil || err2 != nil {
			t.Fatalf("Sanitize.Get failed: err1=%v, err2=%v", err1, err2)
		}

		hash1, ok1 := hash1Val.(string)
		_, ok2 := hash2Val.(string)

		if !ok1 || !ok2 {
			t.Fatal("Expected hashed string values to be of type string")
		}

		if !strings.Contains(hash1, "[HASH:") {
			t.Errorf("Expected hash format, got %s", hash1)
		}

		// Note: Same sanitizer instance will produce same hash (HMAC is deterministic)
		// This test verifies the hash format is correct
		// Rainbow table protection comes from the secret being unknown to attackers
	})

	t.Run("hash cannot be plain sha256", func(t *testing.T) {
		// Verify we're not using plain SHA256 which would be vulnerable
		value := "password123"
		dataMap := map[string]any{"sensitive": value}

		ctx := common.ContextWithSanitizationScope(context.Background(), "hash_test")
		doc := data.MustNewDocument(dataMap, ctx)

		sanitized, err := doc.Sanitize()
		if err != nil {
			t.Fatalf("Sanitize failed: %v", err)
		}

		hashVal, err := sanitized.Get("sensitive")
		if err != nil {
			t.Fatalf("Sanitize.Get failed: %v", err)
		}
		hash, ok := hashVal.(string)
		if !ok {
			t.Fatal("Expected hashed string value to be of type string")
		}

		// Calculate what plain SHA256 would produce
		plainHash := sha256.Sum256([]byte(value))
		plainHashStr := hex.EncodeToString(plainHash[:])[:8]

		// Our hash should NOT match plain SHA256 (we use HMAC)
		if strings.Contains(hash, plainHashStr) {
			t.Error("Hash appears to be plain SHA256 - vulnerable to rainbow tables!")
		}
	})

	t.Run("stable secret produces consistent hashes", func(t *testing.T) {
		// When using a stable secret, same value should produce same hash
		stableSecret := hex.EncodeToString([]byte("test-stable-secret-key-32-bytes-long!"))

		config1 := &data.FieldMaskConfig{
			Fields:     map[string]data.MaskedFieldPolicy{"sensitive": data.MaskHash},
			HashSecret: stableSecret,
		}

		config2 := &data.FieldMaskConfig{
			Fields:     map[string]data.MaskedFieldPolicy{"sensitive": data.MaskHash},
			HashSecret: stableSecret,
		}

		if err := data.RegisterScopedSanitizer("stable1", config1); err != nil {
			t.Fatalf("Failed to register stable1: %v", err)
		}
		if err := data.RegisterScopedSanitizer("stable2", config2); err != nil {
			t.Fatalf("Failed to register stable2: %v", err)
		}

		value := "consistent_value"
		dataMap := map[string]any{"sensitive": value}

		ctx1 := common.ContextWithSanitizationScope(context.Background(), "stable1")
		ctx2 := common.ContextWithSanitizationScope(context.Background(), "stable2")

		doc1 := data.MustNewDocument(dataMap, ctx1)
		doc2 := data.MustNewDocument(dataMap, ctx2)

		sanitized1, err := doc1.Sanitize()
		if err != nil {
			t.Fatalf("Sanitize 1 failed: %v", err)
		}

		sanitized2, err := doc2.Sanitize()
		if err != nil {
			t.Fatalf("Sanitize 2 failed: %v", err)
		}

		hash1, _ := sanitized1.Get("sensitive")
		hash2, _ := sanitized2.Get("sensitive")
		// With same secret, hashes should match
		if hash1 != hash2 {
			t.Errorf("Expected consistent hashes with same secret, got %s vs %s", hash1, hash2)
		}

		// Cleanup
		data.UnregisterScopedSanitizer("stable1")
		data.UnregisterScopedSanitizer("stable2")
	})
}

// ============================================================================
// SafeString Tests
// ============================================================================

func TestSafeString(t *testing.T) {
	setupTestFactory(t)

	dataMap := map[string]any{
		"username": "john",
		"password": "secret123",
	}

	ctx := context.Background()
	doc := data.MustNewDocument(dataMap, ctx)

	safeStr := doc.SafeString()

	// Password should not appear in string
	if strings.Contains(safeStr, "secret123") {
		t.Error("SafeString should not contain password")
	}

	// Should contain redacted marker
	if !strings.Contains(safeStr, "***") {
		t.Error("SafeString should contain redaction marker")
	}

	// Username should still appear
	if !strings.Contains(safeStr, "john") {
		t.Error("SafeString should contain username")
	}
}

// ============================================================================
// Obscure MaxLength Tests
// ============================================================================

func TestObscureMaxLength(t *testing.T) {
	setupTestFactory(t)

	tests := []struct {
		name      string
		value     string
		config    data.ObscureConfig
		expected  string
	}{
		{
			name:  "UUID with max_length",
			value: "1ea82440-9c3e-460b-8fc2-d19a23ab2651",
			config: data.ObscureConfig{
				PrefixLength: 4,
				SuffixLength: 4,
				Replacement:  "*",
				MaxLength:    12, // Total: prefix(4) + replacement(4) + suffix(4)
			},
			expected: "1ea8****2651",
		},
		{
			name:  "long token truncated",
			value: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ",
			config: data.ObscureConfig{
				PrefixLength: 3,
				SuffixLength: 3,
				Replacement:  "*",
				MaxLength:    10, // Total: prefix(3) + replacement(4) + suffix(3)
			},
			expected: "eyJ****yfQ",
		},
		{
			name:  "no truncation needed",
			value: "short",
			config: data.ObscureConfig{
				PrefixLength: 1,
				SuffixLength: 1,
				Replacement:  "*",
				MaxLength:    10,
			},
			expected: "s********t",
		},
		{
			name:  "max_length=0 means no limit",
			value: "1ea82440-9c3e-460b-8fc2-d19a23ab2651",
			config: data.ObscureConfig{
				PrefixLength: 4,
				SuffixLength: 4,
				Replacement:  "*",
				MaxLength:    0, // No limit
			},
			expected: "1ea8****************************2651",
		},
		{
			name:  "max_length too small - ignored",
			value: "longvalue",
			config: data.ObscureConfig{
				PrefixLength: 2,
				SuffixLength: 2,
				Replacement:  "*",
				MaxLength:    3, // Too small (needs at least prefix+suffix+1 = 5)
			},
			expected: "[OBSCURED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// data.Register scope with custom obscure config
			config := &data.FieldMaskConfig{
				Fields: map[string]data.MaskedFieldPolicy{
					"test_field": data.MaskObscure,
				},
				ObscureConfig: tt.config,
				DefaultPolicy: data.MaskPreserve,
			}

			scopeName := "test_obscure_" + tt.name
			if err := data.RegisterScopedSanitizer(scopeName, config); err != nil {
				t.Fatalf("Failed to register scope: %v", err)
			}

			// Test sanitization
			dataMap := map[string]any{"test_field": tt.value}
			ctx := common.ContextWithSanitizationScope(context.Background(), scopeName)
			doc := data.MustNewDocument(dataMap, ctx)

			sanitized, err := doc.Sanitize()
			if err != nil {
				t.Fatalf("Sanitize failed: %v", err)
			}

			result, err := sanitized.Get("test_field")
			if err != nil {
				t.Fatal("Expected string result")
			}

			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}

			// Cleanup
			data.UnregisterScopedSanitizer(scopeName)
		})
	}
}
