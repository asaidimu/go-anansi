package data

import (
	"context"
	"sort"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"go.uber.org/zap"
)

var (
	// ErrSanitizationScopeNotFound indicates that a sanitization scope was not registered
	ErrSanitizationScopeNotFound = common.NewSystemError("ERR_SANITIZATION_SCOPE_NOT_FOUND").
					WithMessage("sanitization scope not registered")

	// ErrSanitizationPatternInvalid indicates that a regex pattern in sanitization config is invalid
	ErrSanitizationPatternInvalid = common.NewSystemError("ERR_SANITIZATION_PATTERN_INVALID").
					WithMessage("invalid regex pattern in sanitization config")

	// ErrSanitizationConfigInvalid indicates that sanitization configuration is invalid
	ErrSanitizationConfigInvalid = common.NewSystemError("ERR_SANITIZATION_CONFIG_INVALID").
					WithMessage("sanitization configuration is invalid")

	// ErrSanitizationFailed indicates that document sanitization operation failed
	ErrSanitizationFailed = common.NewSystemError("ERR_SANITIZATION_FAILED").
				WithMessage("document sanitization failed")
)

// ============================================================================
// Policy Registry
// ============================================================================

type SanitizationPersistence interface {
	Save(ctx context.Context, scope string, config *FieldMaskConfig) error
	Load(ctx context.Context, scope string) (*FieldMaskConfig, error)
	LoadAll(ctx context.Context) ([]*FieldMaskConfig, error)
	Delete(ctx context.Context, scope string) error
}

// SanitizationRegistry manages sanitization policies in a centralized,
// thread-safe manner. It stores lightweight configs and creates sanitizers
// on-demand to handle dynamic scope combinations efficiently.
type SanitizationRegistry struct {
	mu          sync.RWMutex
	global      *FieldMaskConfig            // Global config (can be nil)
	scoped      map[string]*FieldMaskConfig // Scope ID -> config
	logger      *zap.Logger
	onUpdate    func(scopeID string) // Optional callback for policy updates
	persistence SanitizationPersistence
}

func (r *SanitizationRegistry) SetPersistence(p SanitizationPersistence) *SanitizationRegistry {
	r.persistence = p
	return r
}

// NewSanitizationRegistry creates a new policy registry.
func NewSanitizationRegistry(logger *zap.Logger) *SanitizationRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &SanitizationRegistry{
		scoped: make(map[string]*FieldMaskConfig),
		logger: logger,
	}
}

// ============================================================================
// Registration
// ============================================================================

// SetGlobal sets the global sanitization policy.
// This policy applies to all documents unless overridden by a scoped policy.
func (r *SanitizationRegistry) SetGlobal(config *FieldMaskConfig) error {
	if config == nil {
		return common.SystemErrorFrom(ErrSanitizationConfigInvalid).
			WithOperation("SanitizationRegistry.SetGlobal").
			WithMessage("config cannot be nil")
	}

	// Validate config
	if err := config.Validate(); err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("SanitizationRegistry.SetGlobal")
	}

	r.mu.Lock()
	persistence := r.persistence
	r.global = config
	r.mu.Unlock()

	r.logger.Info("Set global sanitization policy",
		zap.Int("fields", len(config.Fields)),
		zap.Int("patterns", len(config.Patterns)))

	// Persist if enabled
	if persistence != nil {
		if err := persistence.Save(context.Background(), "__global__", config); err != nil {
			r.logger.Error("Failed to persist global sanitization policy",
				zap.Error(err))
			// Don't fail the operation - it's still in memory
		}
	}

	if r.onUpdate != nil {
		r.onUpdate("")
	}

	return nil
}

// Register registers a scoped sanitization policy.
// The config is stored as-is without merging with global.
// Merging happens on-demand when sanitizers are created.
func (r *SanitizationRegistry) Register(scopeID string, config *FieldMaskConfig) error {
	if scopeID == "" {
		return common.SystemErrorFrom(ErrSanitizationConfigInvalid).
			WithOperation("SanitizationRegistry.Register").
			WithMessage("scope identifier cannot be empty")
	}

	if config == nil {
		return common.SystemErrorFrom(ErrSanitizationConfigInvalid).
			WithOperation("SanitizationRegistry.Register").
			WithMessagef("config for scope %q cannot be nil", scopeID)
	}

	// Validate config
	if err := config.Validate(); err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("SanitizationRegistry.Register").
			WithMessagef("invalid config for scope %q", scopeID)
	}

	r.mu.Lock()
	persistence := r.persistence
	r.scoped[scopeID] = config
	r.mu.Unlock()

	if persistence != nil {
		if err := r.persistence.Save(context.Background(), scopeID, config); err != nil {
			r.logger.Info("Failed to save scope\n", zap.Any("Scope", config))
			r.logger.Error("Failed to persist sanitization policy",
				zap.String("scope", scopeID), zap.Error(err))
			// Don't fail the registration - it's still in memory
		}
	}

	r.logger.Info("Registered scoped sanitization policy",
		zap.String("scope", scopeID),
		zap.Int("fields", len(config.Fields)),
		zap.Int("patterns", len(config.Patterns)))

	if r.onUpdate != nil {
		r.onUpdate(scopeID)
	}

	return nil
}

// Unregister removes a scoped sanitization policy.
// Returns nil if scope doesn't exist (idempotent).
func (r *SanitizationRegistry) Unregister(scopeID string) error {
	r.mu.Lock()
	persistence := r.persistence
	exists := false
	if _, found := r.scoped[scopeID]; found {
		delete(r.scoped, scopeID)
		exists = true
	}
	r.mu.Unlock()

	if exists {
		r.logger.Info("Unregistered scoped sanitization policy",
			zap.String("scope", scopeID))

		if r.onUpdate != nil {
			r.onUpdate(scopeID)
		}

		// Delete from persistence if enabled
		if persistence != nil {
			if err := persistence.Delete(context.Background(), scopeID); err != nil {
				r.logger.Error("Failed to delete persisted sanitization policy",
					zap.String("scope", scopeID),
					zap.Error(err))
			}
		}
	}

	return nil
}

// ============================================================================
// Retrieval
// ============================================================================

// Get retrieves the sanitizer for a specific scope.
// Returns nil if scope doesn't exist.
// The sanitizer is created on-demand by merging with global config if it exists.
func (r *SanitizationRegistry) Get(scopeID string) *DocumentSanitizer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if scopeID == "" {
		return r.getGlobalSanitizerLocked()
	}

	config, exists := r.scoped[scopeID]
	if !exists {
		return nil
	}

	// Merge with global if exists
	var finalConfig FieldMaskConfig
	if r.global != nil {
		finalConfig = mergeConfigs(r.global, *config)
	} else {
		finalConfig = *config
	}

	return NewDocumentSanitizer(finalConfig, r.logger, scopeID)
}

// GetGlobal returns the global sanitizer.
func (r *SanitizationRegistry) GetGlobal() *DocumentSanitizer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getGlobalSanitizerLocked()
}

// getGlobalSanitizerLocked returns global sanitizer (caller must hold lock)
func (r *SanitizationRegistry) getGlobalSanitizerLocked() *DocumentSanitizer {
	if r.global == nil {
		return nil
	}
	return NewDocumentSanitizer(*r.global, r.logger, "global")
}

// GetScopesFromContext extracts all scope identifiers from a context.
// Returns empty slice if no scopes are present in the context.
func GetScopesFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}

	if val := ctx.Value(common.SanitizationScopeContextKey); val != nil {
		if scopes, ok := val.([]string); ok {
			return scopes
		}
	}

	return nil
}

// GetForContext retrieves a sanitizer for the given context(s).
//
// Behavior:
// - Extracts all scopes from the primary context and any additional contexts
// - Scopes that are not registered are silently skipped (not all scopes need sanitization)
// - If multiple scopes are found, composes them using most-restrictive-wins strategy
// - If no registered scopes are found, returns the global sanitizer if available
// - Returns (nil, nil) if no sanitizers are configured
//
// Usage:
//
//	sanitizer, err := registry.GetForContext(ctx)
//	sanitizer, err := registry.GetForContext(ctx, otherCtx1, otherCtx2)
func (r *SanitizationRegistry) GetForContext(ctx context.Context, others ...context.Context) (*DocumentSanitizer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect all contexts
	contexts := append([]context.Context{ctx}, others...)

	// Gather all unique scopes across all contexts
	seenScopes := make(map[string]bool)
	var scopeIDs []string

	for _, c := range contexts {
		if c == nil {
			continue
		}

		ctxScopes := GetScopesFromContext(c)
		for _, scopeID := range ctxScopes {
			if !seenScopes[scopeID] {
				seenScopes[scopeID] = true
				scopeIDs = append(scopeIDs, scopeID)
			}
		}
	}

	// No scopes in any context, return global if available
	if len(scopeIDs) == 0 {
		return r.getGlobalSanitizerLocked(), nil
	}

	// Collect configs for all registered scopes
	var configs []*FieldMaskConfig
	var foundScopeIDs []string

	for _, scopeID := range scopeIDs {
		if config, exists := r.scoped[scopeID]; exists {
			// Merge with global if exists
			if r.global != nil {
				merged := mergeConfigs(r.global, *config)
				configs = append(configs, &merged)
			} else {
				configs = append(configs, config)
			}
			foundScopeIDs = append(foundScopeIDs, scopeID)
		} else {
			r.logger.Debug("Scope not registered, skipping",
				zap.String("scope", scopeID))
		}
	}

	// If we found at least one registered scope, compose and return
	if len(configs) > 0 {
		if len(configs) == 1 {
			// Single scope, create sanitizer directly
			return NewDocumentSanitizer(*configs[0], r.logger, foundScopeIDs[0]), nil
		}

		// Multiple scopes, compose them
		composed := r.composeConfigsLocked(configs, foundScopeIDs)
		return NewDocumentSanitizer(composed, r.logger, "multi-scope"), nil
	}

	// No registered scopes found, fall back to global
	if r.global != nil {
		r.logger.Debug("No registered scopes found, using global sanitizer",
			zap.Strings("requested_scopes", scopeIDs))
		return r.getGlobalSanitizerLocked(), nil
	}

	// No registered scopes and no global
	r.logger.Debug("No registered scopes or global sanitizer found",
		zap.Strings("requested_scopes", scopeIDs))
	return nil, nil
}

// composeConfigsLocked creates a single config by composing multiple configs
// using most-restrictive-wins strategy. Caller must hold read lock.
//
// This is the heart of multi-scope composition: we analyze all configs and
// produce a single config where each field gets the most restrictive policy
// across all scopes. The resulting sanitizer applies these policies directly,
// so no runtime composition is needed during sanitization.
func (r *SanitizationRegistry) composeConfigsLocked(configs []*FieldMaskConfig, scopeIDs []string) FieldMaskConfig {
	composed := FieldMaskConfig{
		Version:       "v1",
		Fields:        make(map[string]MaskedFieldPolicy),
		Patterns:      []PatternRule{},
		DefaultPolicy: MaskPreserve,
		ObscureConfig: DefaultObscureConfig(),
	}

	// Create temporary sanitizers to leverage getPolicyForField logic
	// which handles field mappings, pattern matching, and defaults
	tempSanitizers := make([]*Sanitizer, len(configs))
	for i, config := range configs {
		tempSanitizers[i] = NewSanitizer(*config, r.logger)
	}

	// Collect all field names across all configs
	allFields := make(map[string]bool)
	for _, config := range configs {
		for field := range config.Fields {
			allFields[field] = true
		}
		// Also consider fields that might match patterns
		// (We'll handle this through pattern collection below)
	}

	// For each explicitly named field, find most restrictive policy
	for field := range allFields {
		mostRestrictive := MaskPreserve
		maxWeight := 0

		for _, s := range tempSanitizers {
			policy := s.getPolicyForField(field)
			if weight := policyWeight[policy]; weight > maxWeight {
				maxWeight = weight
				mostRestrictive = policy
			}
		}

		composed.Fields[field] = mostRestrictive
	}

	// Collect all patterns (deduplicated by pattern string)
	// Patterns will be evaluated in order during sanitization
	seenPatterns := make(map[string]bool)
	for _, config := range configs {
		for _, pattern := range config.Patterns {
			if !seenPatterns[pattern.Pattern] {
				composed.Patterns = append(composed.Patterns, pattern)
				seenPatterns[pattern.Pattern] = true
			}
		}
	}

	// Use most restrictive default policy
	for _, config := range configs {
		if weight := policyWeight[config.DefaultPolicy]; weight > policyWeight[composed.DefaultPolicy] {
			composed.DefaultPolicy = config.DefaultPolicy
		}
	}

	// Use first non-default obscure config (they should be consistent across scopes)
	for _, config := range configs {
		if config.ObscureConfig.Replacement != "" {
			composed.ObscureConfig = config.ObscureConfig
			break
		}
	}

	// Add description noting composition
	if len(scopeIDs) > 0 {
		composed.Description = "Composed from scopes: " + joinStrings(scopeIDs, ", ")
	}

	return composed
}

// joinStrings is a simple helper to join strings (avoids strings import)
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// ============================================================================
// Introspection
// ============================================================================

// List returns all registered scope identifiers (sorted).
func (r *SanitizationRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	scopes := make([]string, 0, len(r.scoped))
	for scopeID := range r.scoped {
		scopes = append(scopes, scopeID)
	}
	sort.Strings(scopes)
	return scopes
}

// Has checks if a scope is registered.
func (r *SanitizationRegistry) Has(scopeID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.scoped[scopeID]
	return exists
}

// HasGlobal checks if a global policy is registered.
func (r *SanitizationRegistry) HasGlobal() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.global != nil
}

// Count returns the number of registered scoped policies.
func (r *SanitizationRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.scoped)
}

// ============================================================================
// Lifecycle Management
// ============================================================================

// OnUpdate sets a callback that is invoked when policies are updated.
// The callback receives the scope ID (empty string for global).
func (r *SanitizationRegistry) OnUpdate(callback func(scopeID string)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.onUpdate = callback
}

// Clear removes all policies (global and scoped).
// Useful for testing or reconfiguration.
func (r *SanitizationRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.global = nil
	r.scoped = make(map[string]*FieldMaskConfig)

	r.logger.Info("Cleared all sanitization policies")
}

// ============================================================================
// Batch Operations
// ============================================================================

// RegisterBatch registers multiple scoped policies atomically.
// If any registration fails, none are registered (all-or-nothing).
func (r *SanitizationRegistry) RegisterBatch(policies map[string]*FieldMaskConfig) error {
	// Validate all configs first
	for scopeID, config := range policies {
		if scopeID == "" {
			return common.SystemErrorFrom(ErrSanitizationConfigInvalid).
				WithOperation("SanitizationRegistry.RegisterBatch").
				WithMessage("scope identifier cannot be empty")
		}

		if config == nil {
			return common.SystemErrorFrom(ErrSanitizationConfigInvalid).
				WithOperation("SanitizationRegistry.RegisterBatch").
				WithMessagef("config for scope %q cannot be nil", scopeID)
		}

		if err := config.Validate(); err != nil {
			return common.SystemErrorFrom(err).
				WithOperation("SanitizationRegistry.RegisterBatch").
				WithMessagef("invalid config for scope %q", scopeID)
		}
	}

	// All valid, register atomically
	r.mu.Lock()
	defer r.mu.Unlock()

	for scopeID, config := range policies {
		r.scoped[scopeID] = config

		if r.onUpdate != nil {
			r.onUpdate(scopeID)
		}
	}

	r.logger.Info("Registered batch of sanitization policies",
		zap.Int("count", len(policies)))

	return nil
}

// UnregisterBatch removes multiple scoped policies atomically.
func (r *SanitizationRegistry) UnregisterBatch(scopeIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, scopeID := range scopeIDs {
		if _, exists := r.scoped[scopeID]; exists {
			delete(r.scoped, scopeID)

			if r.onUpdate != nil {
				r.onUpdate(scopeID)
			}
		}
	}

	r.logger.Info("Unregistered batch of sanitization policies",
		zap.Int("count", len(scopeIDs)))

	return nil
}

// ============================================================================
// Export/Import
// ============================================================================

// Export exports all policies.
func (r *SanitizationRegistry) Export() ([]*FieldMaskConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	configs := make([]*FieldMaskConfig, 0, 1+len(r.scoped))

	// Export global
	if r.global != nil {
		config := *r.global
		config.Scope = ""
		config.Description = "Global sanitization policy"
		configs = append(configs, &config)
	}

	// Export scoped
	for scopeID, config := range r.scoped {
		exportConfig := *config
		exportConfig.Scope = scopeID
		configs = append(configs, &exportConfig)
	}

	return configs, nil
}

// Import imports policies.
// Replaces all existing policies (use RegisterBatch for additive).
func (r *SanitizationRegistry) Import(configs []*FieldMaskConfig) error {
	// Validate all first
	var globalConfig *FieldMaskConfig
	scopedConfigs := make(map[string]*FieldMaskConfig)

	for _, config := range configs {
		if err := config.Validate(); err != nil {
			return common.SystemErrorFrom(err).
				WithOperation("SanitizationRegistry.Import").
				WithMessagef("invalid config for scope %q", config.Scope)
		}

		if config.Scope == "" {
			globalConfig = config
		} else {
			scopedConfigs[config.Scope] = config
		}
	}

	// Clear and import atomically
	r.mu.Lock()
	defer r.mu.Unlock()

	r.global = globalConfig
	r.scoped = scopedConfigs

	r.logger.Info("Imported sanitization policies",
		zap.Bool("has_global", globalConfig != nil),
		zap.Int("scoped_count", len(scopedConfigs)))

	return nil
}

// LoadFromPersistence loads all policies from the persistence backend into the registry.
// This should typically be called once during application startup.
func (r *SanitizationRegistry) LoadFromPersistence(ctx context.Context) error {
	r.mu.Lock()
	persistence := r.persistence
	r.mu.Unlock()

	if persistence == nil {
		return common.NewSystemError("NO_PERSISTENCE").
			WithOperation("SanitizationRegistry.LoadFromPersistence").
			WithMessage("persistence not configured")
	}

	configs, err := persistence.LoadAll(ctx)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("SanitizationRegistry.LoadFromPersistence")
	}

	if len(configs) == 0 {
		r.logger.Info("No persisted sanitization policies found")
		return nil
	}

	// Import policies without triggering persistence callbacks
	// (since they're already persisted)
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, config := range configs {
		if config.Scope == "__global__" {
			// Global policy
			r.global = config
			r.logger.Info("Loaded global sanitization policy from persistence",
				zap.Int("fields", len(config.Fields)),
				zap.Int("patterns", len(config.Patterns)))
		} else {
			// Scoped policy
			r.scoped[config.Scope] = config
			r.logger.Info("Loaded scoped sanitization policy from persistence",
				zap.String("scope", config.Scope),
				zap.Int("fields", len(config.Fields)),
				zap.Int("patterns", len(config.Patterns)))
		}
	}

	r.logger.Info("Loaded all sanitization policies from persistence",
		zap.Int("total", len(configs)))

	return nil
}
