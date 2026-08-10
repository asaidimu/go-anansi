package sanitize

import (
	"sync"

	"go.uber.org/zap"
)

// ============================================================================
// Global Default Registry
// ============================================================================

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *SanitizationRegistry
)

// Config holds the process-wide sanitization configuration applied to the
// default registry by Configure.
type Config struct {
	// Global applies to all documents unless overridden by a scoped policy.
	Global *FieldMaskConfig

	// Scoped keys are scope identifiers (collection names, API paths, tenant
	// IDs); values are the policies for that scope.
	Scoped map[string]*FieldMaskConfig
}

// Registry returns the process-wide default sanitization registry. It is
// created lazily with a nop logger and is safe to call before Configure.
func Registry() *SanitizationRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewSanitizationRegistry(zap.NewNop())
	})
	return defaultRegistry
}

// Configure applies a global and scoped sanitization policies to the default
// registry, replacing any previously configured policies. It returns an error
// if any policy fails validation; on error the previous configuration is kept.
func Configure(config Config, logger *zap.Logger) error {
	reg := Registry()
	if logger != nil {
		reg.logger = logger
	}

	if config.Global != nil {
		if err := reg.SetGlobal(config.Global); err != nil {
			return err
		}
	}

	for scopeID, scopedConfig := range config.Scoped {
		if scopedConfig == nil {
			reg.logger.Warn("Skipping nil scoped sanitizer config",
				zap.String("scope", scopeID))
			continue
		}
		if err := reg.Register(scopeID, scopedConfig); err != nil {
			return err
		}
	}

	return nil
}

// GetScopedSanitizer retrieves the sanitizer for a specific scope.
// Returns nil if no scope-specific sanitizer exists (global will be used).
//
// This is useful for testing or manual sanitization.
func GetScopedSanitizer(scopeID string) *DocumentSanitizer {
	return Registry().Get(scopeID)
}

// ResetForTesting clears the default registry so tests start from a clean
// state. It must not be used in production code.
func ResetForTesting() {
	defaultRegistryOnce = sync.Once{}
	defaultRegistry = nil
}
