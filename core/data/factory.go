package data

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"maps"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ============================================================================
// Metadata Provider Types
// ============================================================================

// MetadataProvider is a function that returns metadata to be merged into a document.
type MetadataProvider func(ctx context.Context, doc *Document) (map[string]any, error)

// MetadataProviderConfig holds a nested schema, its dependencies and its corresponding provider.
type MetadataProviderConfig struct {
	Name         string
	Schema       *definition.NestedSchema
	Dependencies []*definition.NestedSchema
	Provider     MetadataProvider
}

// ============================================================================
// Factory Configuration
// ============================================================================

// DocumentFactoryConfig holds the complete configuration for the document factory.
type DocumentFactoryConfig struct {
	Providers []MetadataProviderConfig

	// GlobalSanitizer configures field-level data sanitization for all documents.
	// When set, all events emitted by the persistence layer will have their
	// Input and Output fields sanitized according to the specified policies.
	//
	// If nil, no sanitization is applied (NOT recommended for production).
	// Use data.NewSecureDefaultConfig() for sensible defaults.
	GlobalSanitizer *FieldMaskConfig

	// ScopedSanitizers allows scope-specific sanitization overrides.
	// Keys are scope identifiers (e.g., collection names, API paths, tenant IDs).
	// Values are the sanitization config to use for that specific scope.
	// If a scope is not found in this map, the GlobalSanitizer is used.
	//
	// Example:
	//   ScopedSanitizers: map[string]*FieldMaskConfig{
	//     "credentials":     strictConfig,  // Stricter rules for credentials
	//     "public_profiles": lenientConfig, // More lenient for public data
	//     "api/v1/auth":     authConfig,    // Scope by API path
	//   }
	ScopedSanitizers map[string]*FieldMaskConfig
}

// ============================================================================
// Factory Internal State
// ============================================================================

// documentFactory is a singleton responsible for creating and managing documents.
type documentFactory struct {
	mu                   sync.RWMutex
	config               DocumentFactoryConfig
	sanitizationRegistry *SanitizationRegistry
	logger               *zap.Logger
	configured           bool
}

var (
	factoryOnce   sync.Once
	configureOnce sync.Once
	factory       *documentFactory
)

func getFactory() *documentFactory {
	factoryOnce.Do(func() {
		factory = &documentFactory{}
	})
	return factory
}

// ============================================================================
// Factory Configuration
// ============================================================================

// ConfigureDocumentFactory sets up the document factory singleton.
// It must be called once at application startup.
func ConfigureDocumentFactory(config DocumentFactoryConfig, logger *zap.Logger) error {
	var err error
	configureOnce.Do(func() {
		f := getFactory()
		f.mu.Lock()
		defer f.mu.Unlock()

		// Store logger
		if logger == nil {
			logger = zap.NewNop()
		}
		f.logger = logger

		// Initialize sanitization registry
		f.sanitizationRegistry = NewSanitizationRegistry(logger)

		// Set global sanitizer
		if config.GlobalSanitizer != nil {
			if regErr := f.sanitizationRegistry.SetGlobal(config.GlobalSanitizer); regErr != nil {
				err = common.SystemErrorFrom(regErr).
					WithOperation("ConfigureDocumentFactory").
					WithMessage("failed to set global sanitizer")
				return
			}
		}

		// Register scoped sanitizers
		for scopeID, scopedConfig := range config.ScopedSanitizers {
			if scopedConfig == nil {
				logger.Warn("Skipping nil scoped sanitizer config",
					zap.String("scope", scopeID))
				continue
			}

			if regErr := f.sanitizationRegistry.Register(scopeID, scopedConfig); regErr != nil {
				err = common.SystemErrorFrom(regErr).
					WithOperation("ConfigureDocumentFactory").
					WithMessagef("failed to register scope %q", scopeID)
				return
			}
		}

		f.config = config
		f.configured = true
	})

	return err
}

// ============================================================================
// Dynamic Scope Management
// ============================================================================

// RegisterScopedSanitizer registers sanitization rules for a specific scope.
// The scope can be a collection name, API path, tenant ID, or any identifier
// that makes sense in your application context.
//
// This method is thread-safe and can be called after factory initialization.
//
// Returns error if:
//   - Factory not configured
//   - Scope ID is empty
//   - Config is nil
//   - Pattern compilation fails
func RegisterScopedSanitizer(scopeID string, config *FieldMaskConfig) error {
	f := getFactory()
	if !f.configured {
		return common.SystemErrorFrom(ErrFactoryNotConfigured).
			WithOperation("data.RegisterScopedSanitizer")
	}

	return f.sanitizationRegistry.Register(scopeID, config)
}

// UnregisterScopedSanitizer removes sanitization rules for a specific scope.
// After removal, documents in that scope will fall back to global sanitizer.
//
// Returns error if factory not configured.
// Returns nil if scope doesn't exist (idempotent).
func UnregisterScopedSanitizer(scopeID string) error {
	f := getFactory()
	if !f.configured {
		return common.SystemErrorFrom(ErrFactoryNotConfigured).
			WithOperation("data.UnregisterScopedSanitizer")
	}

	return f.sanitizationRegistry.Unregister(scopeID)
}

// ListScopedSanitizers returns all registered scope identifiers.
// Useful for introspection and debugging.
func ListScopedSanitizers() []string {
	f := getFactory()
	if !f.configured || f.sanitizationRegistry == nil {
		return nil
	}

	return f.sanitizationRegistry.List()
}

// GetSanitizationRegistry returns the registry for advanced operations.
// Returns nil if factory not configured.
func GetSanitizationRegistry() *SanitizationRegistry {
	f := getFactory()
	if !f.configured {
		return nil
	}

	return f.sanitizationRegistry
}

// ============================================================================
// Sanitizer Retrieval
// ============================================================================

// getSanitizersForContexts returns sanitizers for all contexts in order.
// Returns error if any explicitly specified scope is not found (fail-fast).
func (f *documentFactory) getSanitizersForContexts(ctx context.Context, contexts ...context.Context) (*DocumentSanitizer, error) {
	if f.sanitizationRegistry == nil {
		return nil, nil
	}

	return f.sanitizationRegistry.GetForContext(ctx, contexts...)
}

// ============================================================================
// Document Creation
// ============================================================================

// newDocument creates a new document with injected metadata.
//
// This method:
//  1. Extracts _id from input data (or generates new one)
//  2. Extracts _metadata_ from input data (or creates new)
//  3. Separates user data from system fields
//  4. Applies metadata providers
//  5. Normalizes and hashes the document
func (f *documentFactory) newDocument(ctx context.Context, inputData map[string]any) (*Document, error) {
	if !f.configured {
		return nil, common.SystemErrorFrom(ErrFactoryNotConfigured).
			WithOperation("data.documentFactory.newDocument")
	}

	if inputData == nil {
		inputData = make(map[string]any)
	}

	// Step 1: Extract or generate ID
	id := f.extractOrGenerateID(inputData)

	// Step 2: Extract or create metadata
	metadata := f.extractOrCreateMetadata(inputData)

	// Step 3: Separate user data from system fields
	userData := make(map[string]any)
	for k, v := range inputData {
		if !ReservedSystemField(k) {
			userData[k] = v
		}
	}

	// Step 4: Create document with clean separation
	doc := &Document{
		id:       id,
		ctx:      ctx,
		data:     userData,
		metadata: metadata,
	}

	// Step 5: Apply metadata providers
	if err := f.applyMetadataProviders(ctx, doc); err != nil {
		return nil, err
	}

	// Step 6: Normalize and preserve context
	normalizedDoc := doc.Normalize()

	// Step 7: Calculate hash based on document with complete metadata
	if err := normalizedDoc.Hash(); err != nil {
		return nil, common.SystemErrorFrom(err).WithOperation("data.documentFactory.newDocument")
	}

	return normalizedDoc, nil
}

// extractOrGenerateID extracts the ID from input data or generates a new one.
func (f *documentFactory) extractOrGenerateID(data map[string]any) string {
	if idVal, ok := data[DocumentIDField]; ok {
		if id, ok := idVal.(string); ok && f.isValidID(id) {
			return id
		}
	}
	// Generate new UUIDv7 without dashes
	return strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
}

// extractOrCreateMetadata extracts metadata from input data or creates new metadata.
func (f *documentFactory) extractOrCreateMetadata(data map[string]any) map[string]any {
	metadata := make(map[string]any)

	// Extract existing metadata if present
	if metaVal, ok := data[MetadataField]; ok {
		if existingMeta, ok := metaVal.(map[string]any); ok {
			maps.Copy(metadata, existingMeta)
		}
	}

	// Inject system metadata defaults
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, ok := metadata[MetadataCreated]; !ok {
		metadata[MetadataCreated] = now
	}

	if _, ok := metadata[MetadataUpdated]; !ok {
		metadata[MetadataUpdated] = now
	}

	if version, ok := metadata[MetadataVersion]; !ok || !utils.IsInteger(version) {
		metadata[MetadataVersion] = 1
	}

	return metadata
}

// applyMetadataProviders applies all configured metadata providers to the document.
func (f *documentFactory) applyMetadataProviders(ctx context.Context, doc *Document) error {
	// Copy provider configs while holding lock, then release immediately
	f.mu.RLock()
	providers := make([]MetadataProviderConfig, len(f.config.Providers))
	copy(providers, f.config.Providers)
	f.mu.RUnlock()

	// Apply user-defined metadata providers without holding the lock
	for _, providerConfig := range providers {
		providerMeta, err := providerConfig.Provider(ctx, doc)
		if err != nil {
			return common.SystemErrorFrom(ErrMetadataProviderFailed).
				WithOperation("data.documentFactory.applyMetadataProviders").
				WithMessagef("metadata provider %q failed", providerConfig.Name).
				WithCause(err)
		}
		if providerMeta == nil {
			continue // Skip nil results
		}

		// Validate provider doesn't overwrite reserved fields
		for key := range providerMeta {
			if isReservedMetadataField(key) {
				return common.SystemErrorFrom(ErrInvalidMetadata).
					WithOperation("data.documentFactory.applyMetadataProviders").
					WithMessagef("provider %q attempted to set reserved field %q", providerConfig.Name, key)
			}
		}

		// Merge provider metadata
		if doc.metadata == nil {
			doc.metadata = make(map[string]any)
		}
		maps.Copy(doc.metadata, providerMeta)
	}

	return nil
}

// isValidID checks if a string is a valid UUIDv7 without dashes.
func (f *documentFactory) isValidID(id string) bool {
	// UUID without dashes must be exactly 32 hex characters
	if len(id) != 32 {
		return false
	}

	// Reconstruct dashed UUID
	var b strings.Builder
	b.Grow(36)
	b.WriteString(id[0:8])
	b.WriteByte('-')
	b.WriteString(id[8:12])
	b.WriteByte('-')
	b.WriteString(id[12:16])
	b.WriteByte('-')
	b.WriteString(id[16:20])
	b.WriteByte('-')
	b.WriteString(id[20:])

	u, err := uuid.Parse(b.String())
	if err != nil {
		return false
	}

	return u.Version() == 7
}

// ============================================================================
// Metadata Schema
// ============================================================================

// GetMetadataSchema merges all provider schemas into a single metadata schema
func GetMetadataSchema() (*definition.NestedSchema, []*definition.NestedSchema) {
	f := getFactory()
	// Copy provider configs while holding lock
	f.mu.RLock()
	providers := make([]MetadataProviderConfig, len(f.config.Providers))
	copy(providers, f.config.Providers)
	f.mu.RUnlock()

	mergedSchema := DefaultMetadataSchema()
	dependencies := make([]*definition.NestedSchema, 0)
	dependencyNames := make(map[string]bool)

	// Initialize the Fields map if it's nil
	if mergedSchema.Fields == nil {
		mergedSchema.Fields = make(map[definition.FieldId]definition.Field)
	}

	for _, providerConfig := range providers {
		// Add dependencies to the list of unique dependencies
		for _, dep := range providerConfig.Dependencies {
			if _, ok := dependencyNames[dep.Name]; !ok {
				dependencyNames[dep.Name] = true
				dependencies = append(dependencies, dep)
			}
		}

		// Merge the provider's schema fields into the top-level metadata schema
		if providerConfig.Schema != nil && providerConfig.Schema.Fields != nil {
			maps.Copy(mergedSchema.Fields, providerConfig.Schema.Fields)
		}
	}

	return mergedSchema, dependencies
}

// ============================================================================
// Hash and Signature Operations
// ============================================================================

// calculateHash computes the SHA256 hash of the document,
// excluding the 'checksum' field itself, to ensure a consistent and verifiable identifier.
func (f *documentFactory) calculateHash(doc *Document) (string, error) {
	// Clone document to avoid modifying original
	dataToHash := doc.Clone()

	// Remove checksum and signature from metadata for hash calculation
	if dataToHash.metadata != nil {
		delete(dataToHash.metadata, MetadataSignature)
		delete(dataToHash.metadata, MetadataChecksum)
	}

	// Use canonicalMarshal to ensure consistent key ordering for hashing
	toHash, err := canonicalMarshal(dataToHash)
	if err != nil {
		return "", common.SystemErrorFrom(ErrFailedToMarshalMetadata).
			WithOperation("data.documentFactory.calculateHash").
			WithCause(err)
	}

	h := sha256.New()
	h.Write(toHash)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// signDocument signs the entire document (excluding the signature itself) using a private key.
func (f *documentFactory) signDocument(doc *Document, privateKey *rsa.PrivateKey) (string, error) {
	// Clone document and remove signature/checksum
	docToSign := doc.Clone()
	if docToSign.metadata != nil {
		delete(docToSign.metadata, MetadataSignature)
		delete(docToSign.metadata, MetadataChecksum)
	}

	// Marshal the document to a canonical byte slice
	canonicalBytes, err := canonicalMarshal(docToSign)
	if err != nil {
		return "", common.SystemErrorFrom(ErrSignDocumentMarshalFailed).
			WithOperation("data.documentFactory.signDocument").
			WithCause(err)
	}

	// Hash the canonical bytes
	hasher := sha256.New()
	hasher.Write(canonicalBytes)
	hashed := hasher.Sum(nil)

	// Sign the hash
	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hashed, nil)
	if err != nil {
		return "", common.SystemErrorFrom(ErrSignDocumentFailed).
			WithOperation("data.documentFactory.signDocument").
			WithCause(err)
	}

	return base64.StdEncoding.EncodeToString(signature), nil
}

// verifySignature verifies the signature of a document against a public key.
func (f *documentFactory) verifySignature(doc *Document, publicKey *rsa.PublicKey, signature string) error {
	// Decode the signature
	signedBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return common.SystemErrorFrom(ErrVerifySignatureDecodeFailed).
			WithOperation("data.documentFactory.verifySignature").
			WithCause(err)
	}

	// Clone document and remove signature/checksum
	docToVerify := doc.Clone()
	if docToVerify.metadata != nil {
		delete(docToVerify.metadata, MetadataSignature)
		delete(docToVerify.metadata, MetadataChecksum)
	}

	// Marshal the document to a canonical byte slice
	canonicalBytes, err := canonicalMarshal(docToVerify)
	if err != nil {
		return common.SystemErrorFrom(ErrVerifyDocumentMarshalFailed).
			WithOperation("data.documentFactory.verifySignature").
			WithCause(err)
	}

	// Hash the canonical bytes
	hasher := sha256.New()
	hasher.Write(canonicalBytes)
	hashed := hasher.Sum(nil)

	// Verify the signature
	err = rsa.VerifyPSS(publicKey, crypto.SHA256, hashed, signedBytes, nil)
	if err != nil {
		return common.SystemErrorFrom(ErrSignatureVerificationFailed).
			WithOperation("data.documentFactory.verifySignature").
			WithCause(errors.Join(ErrSignatureInvalid, err))
	}

	return nil
}

// ============================================================================
// Canonicalization
// ============================================================================

// canonicalize recursively normalizes a value for consistent serialization.
// For Documents, it creates a canonical representation with id, metadata, and data.
func canonicalize(v any) any {
	switch val := v.(type) {
	case *Document:
		if val == nil {
			return nil
		}
		// Create canonical document representation
		canonical := make(map[string]any, 3)
		canonical[DocumentIDField] = val.id
		if val.metadata != nil {
			canonical[MetadataField] = canonicalize(val.metadata)
		}
		// Add all user data fields
		for k, v := range val.data {
			canonical[k] = canonicalize(v)
		}
		// Sort keys for consistent ordering
		keys := make([]string, 0, len(canonical))
		for k := range canonical {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sorted := make(map[string]any, len(canonical))
		for _, k := range keys {
			sorted[k] = canonical[k]
		}
		return sorted
	case Document:
		// Handle value type by converting to pointer
		return canonicalize(&val)
	case map[string]any:
		newMap := make(map[string]any, len(val))
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			newMap[k] = canonicalize(val[k])
		}
		return newMap
	case []any:
		newSlice := make([]any, len(val))
		for i, item := range val {
			newSlice[i] = canonicalize(item)
		}
		return newSlice
	case float64:
		if val == float64(int64(val)) {
			return int64(val)
		}
		return val
	case int:
		return int64(val)
	case int8:
		return int64(val)
	case int16:
		return int64(val)
	case int32:
		return int64(val)
	case int64:
		return val
	case uint:
		return uint64(val)
	case uint8:
		return uint64(val)
	case uint16:
		return uint64(val)
	case uint32:
		return uint64(val)
	case uint64:
		return val
	default:
		return v
	}
}

// canonicalMarshal marshals a value into a canonical JSON byte slice.
func canonicalMarshal(v any) ([]byte, error) {
	data, err := json.Marshal(canonicalize(v))
	if err != nil {
		return nil, common.SystemErrorFrom(ErrFailedToMarshalJSON).
			WithMessage("failed to marshal canonicalized data to JSON").
			WithCause(err)
	}
	return data, nil
}

// ============================================================================
// Key Loading Helpers
// ============================================================================

// LoadPrivateKey loads a PEM-encoded RSA private key.
func LoadPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, common.SystemErrorFrom(ErrFailedToDecodePEMBlock).WithOperation("data.LoadPrivateKey")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try parsing as PKCS8
		pkcs8Key, pkcs8Err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if pkcs8Err != nil {
			return nil, common.SystemErrorFrom(ErrFailedToParsePrivateKey).WithOperation("data.LoadPrivateKey").WithCause(errors.Join(err, pkcs8Err))
		}
		if rsaKey, ok := pkcs8Key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, common.SystemErrorFrom(ErrNotRSAPrivateKey).WithOperation("data.LoadPrivateKey")
	}
	return key, nil
}

// LoadPublicKey loads a PEM-encoded RSA public key.
func LoadPublicKey(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, common.SystemErrorFrom(ErrFailedToDecodePEMPublicKey).WithOperation("data.LoadPublicKey")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, common.SystemErrorFrom(ErrFailedToParsePublicKey).WithOperation("data.LoadPublicKey").WithCause(err)
	}

	if rsaKey, ok := key.(*rsa.PublicKey); ok {
		return rsaKey, nil
	}

	return nil, common.SystemErrorFrom(ErrNotRSAPublicKey).WithOperation("data.LoadPublicKey")
}

// GetSanitizationPolicy retrieves the policy for a specific scope.
// If scopeID is empty, returns the global policy.
func GetSanitizationPolicy(scopeID string) (*FieldMaskConfig, error) {
	registry := GetSanitizationRegistry()
	if registry == nil {
		return nil, common.SystemErrorFrom(ErrFactoryNotConfigured).
			WithOperation("data.GetSanitizationPolicy")
	}

	var sanitizer *DocumentSanitizer
	if scopeID != "" {
		sanitizer = registry.Get(scopeID)
		if sanitizer == nil {
			return nil, common.SystemErrorFrom(ErrSanitizationScopeNotFound).
				WithOperation("data.GetSanitizationPolicy").
				WithMessagef("scope %q not found", scopeID)
		}
	} else {
		sanitizer = registry.GetGlobal()
		if sanitizer == nil {
			return nil, common.SystemErrorFrom(ErrSanitizationConfigInvalid).
				WithOperation("data.GetSanitizationPolicy").
				WithMessage("global sanitizer not configured")
		}
	}

	config := sanitizer.config
	config.Scope = scopeID
	return &config, nil
}

// ListSanitizationPolicies returns all registered policies (global + scoped).
func ListSanitizationPolicies() ([]*FieldMaskConfig, error) {
	registry := GetSanitizationRegistry()
	if registry == nil {
		return nil, common.SystemErrorFrom(ErrFactoryNotConfigured).
			WithOperation("data.ListSanitizationPolicies")
	}

	return registry.Export()
}

// ============================================================================
// Testing Support
// ============================================================================

// ResetFactoryForTesting resets the singleton document factory and its configuration.
// This is intended for use in tests only, to ensure a clean state between test runs.
func ResetFactoryForTesting() {
	factory = nil
	factoryOnce = sync.Once{}
	configureOnce = sync.Once{}
}
