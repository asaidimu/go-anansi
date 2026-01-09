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
	"github.com/asaidimu/go-anansi/v6/core/schema"
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
	Schema       *schema.NestedSchemaDefinition
	Dependencies []*schema.NestedSchemaDefinition
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
	mu                     sync.RWMutex
	config                 DocumentFactoryConfig
	sanitizationRegistry   *SanitizationRegistry
	logger                 *zap.Logger
	configured             bool
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
func (f *documentFactory) getSanitizersForContexts(ctx context.Context,contexts ...context.Context) (*DocumentSanitizer, error) {
	if f.sanitizationRegistry == nil {
		return nil, nil
	}

	return f.sanitizationRegistry.GetForContext(ctx, contexts...)
}

// ============================================================================
// Document Creation
// ============================================================================

// newDocument creates a new document with injected metadata.
func (f *documentFactory) newDocument(ctx context.Context, data map[string]any) (*Document, error) {
	if !f.configured {
		return nil, common.SystemErrorFrom(ErrFactoryNotConfigured).
			WithOperation("data.documentFactory.newDocument")
	}

	if data == nil {
		data = make(map[string]any)
	}

	docData := make(map[string]any, len(data))
	maps.Copy(docData, data)

	doc := &Document{ctx: ctx, data: docData}

	// Ensure document has a valid ID
	if !f.hasValidID(doc) {
		doc.data[DocumentIDField] = strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	}

	meta := make(map[string]any)

	// Copy existing metadata if present
	if existingMeta, ok := doc.Metadata(); ok {
		maps.Copy(meta, existingMeta)
	}

	// Inject system metadata
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, ok := meta[MetadataCreated]; !ok {
		meta[MetadataCreated] = now
	}

	if _, ok := meta[MetadataUpdated]; !ok {
		meta[MetadataUpdated] = now
	}

	if version, ok := meta[MetadataVersion]; !ok || !utils.IsInteger(version) {
		meta[MetadataVersion] = 1
	}

	// Copy provider configs while holding lock, then release immediately
	f.mu.RLock()
	providers := make([]MetadataProviderConfig, len(f.config.Providers))
	copy(providers, f.config.Providers)
	f.mu.RUnlock()

	// Apply user-defined metadata providers without holding the lock
	for _, providerConfig := range providers {
		providerMeta, err := providerConfig.Provider(ctx, doc)
		if err != nil {
			return nil, common.SystemErrorFrom(ErrMetadataProviderFailed).
				WithOperation("data.documentFactory.newDocument").
				WithMessagef("metadata provider %q failed", providerConfig.Name).
				WithCause(err)
		}
		if providerMeta == nil {
			continue // Skip nil results
		}

		// Validate provider doesn't overwrite reserved fields
		for key := range providerMeta {
			if isReservedMetadataField(key) {
				return nil, common.SystemErrorFrom(ErrInvalidMetadata).
					WithOperation("data.documentFactory.newDocument").
					WithMessagef("provider %q attempted to set reserved field %q", providerConfig.Name, key)
			}
		}
		maps.Copy(meta, providerMeta)
	}

	// Set metadata once before hash calculation
	doc.SetMetadata(meta)

	// Normalize and preserve context
	normalizedDoc := doc.Normalize()

	// Calculate hash based on document with complete metadata
	err := normalizedDoc.Hash()
	if err != nil {
		return nil, common.SystemErrorFrom(err).WithOperation("data.documentFactory.newDocument")
	}

	return normalizedDoc, nil
}

// ============================================================================
// Metadata Schema
// ============================================================================

// GetMetadataSchema merges all provider schemas into a single metadata schema
func GetMetadataSchema() (*schema.NestedSchemaDefinition, []*schema.NestedSchemaDefinition) {
	f := getFactory()
	// Copy provider configs while holding lock
	f.mu.RLock()
	providers := make([]MetadataProviderConfig, len(f.config.Providers))
	copy(providers, f.config.Providers)
	f.mu.RUnlock()

	mergedSchema := DefaultMetadataSchema()
	dependencies := make([]*schema.NestedSchemaDefinition, 0)
	dependencyNames := make(map[string]bool)

	// Initialize the StructuredFieldsMap if it's nil
	if mergedSchema.Fields == nil {
		mergedSchema.Fields = &schema.NestedSchemaFields{
			FieldsMap: make(map[string]*schema.FieldDefinition),
		}
	}

	if mergedSchema.Fields.FieldsMap == nil {
		mergedSchema.Fields.FieldsMap = make(map[string]*schema.FieldDefinition)
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
		if providerConfig.Schema != nil && providerConfig.Schema.Fields != nil && providerConfig.Schema.Fields.FieldsMap != nil {
			maps.Copy(mergedSchema.Fields.FieldsMap, providerConfig.Schema.Fields.FieldsMap)
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
	dataToSign := doc.Clone()
	meta, ok := dataToSign.Metadata()

	if !ok {
		return "", common.SystemErrorFrom(ErrNoMetadata).WithOperation("data.documentFactory.calculateHash")
	}

	// Create a copy of the metadata and remove the hash field itself
	delete(meta, MetadataSignature)
	delete(meta, MetadataChecksum)
	dataToSign.SetMetadata(meta)
	// Use canonicalMarshal to ensure consistent key ordering for hashing.
	toSign, err := canonicalMarshal(dataToSign)
	if err != nil {
		return "", common.SystemErrorFrom(ErrFailedToMarshalMetadata).WithOperation("data.documentFactory.calculateHash").WithCause(err)
	}

	h := sha256.New()
	h.Write(toSign)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// signDocument signs the entire document (excluding the signature itself) using a private key.
func (f *documentFactory) signDocument(doc *Document, privateKey *rsa.PrivateKey) (string, error) {
	// Create a copy of the document and remove the signature field
	docToSign := doc.Clone()
	meta, ok := docToSign.Metadata()
	if ok {
		delete(meta, MetadataSignature)
		delete(meta, MetadataChecksum)
		docToSign.SetMetadata(meta)
	}

	// Marshal the document to a canonical byte slice
	canonicalBytes, err := canonicalMarshal(docToSign)
	if err != nil {
		return "", common.SystemErrorFrom(ErrSignDocumentMarshalFailed).WithOperation("data.documentFactory.signDocument").WithCause(err)
	}

	// Hash the canonical bytes
	hasher := sha256.New()
	hasher.Write(canonicalBytes)
	hashed := hasher.Sum(nil)

	// Sign the hash
	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hashed, nil)
	if err != nil {
		return "", common.SystemErrorFrom(ErrSignDocumentFailed).WithOperation("data.documentFactory.signDocument").WithCause(err)
	}

	return base64.StdEncoding.EncodeToString(signature), nil
}

func (f *documentFactory) hasValidID(doc *Document) bool {
	i, ok := doc.data[DocumentIDField]
	if !ok {
		return false
	}

	id, ok := i.(string)
	if !ok {
		return false
	}

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

// verifySignature verifies the signature of a document against a public key.
func (f *documentFactory) verifySignature(doc *Document, publicKey *rsa.PublicKey, signature string) error {
	// Decode the signature
	signedBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return common.SystemErrorFrom(ErrVerifySignatureDecodeFailed).WithOperation("data.documentFactory.verifySignature").WithCause(err)
	}

	// Create a copy of the document and remove the signature field
	docToVerify := doc.Clone()
	meta, ok := docToVerify.Metadata()
	if ok {
		delete(meta, MetadataSignature)
		delete(meta, MetadataChecksum)
		docToVerify.SetMetadata(meta)
	}

	// Marshal the document to a canonical byte slice
	canonicalBytes, err := canonicalMarshal(docToVerify)
	if err != nil {
		return common.SystemErrorFrom(ErrVerifyDocumentMarshalFailed).WithOperation("data.documentFactory.verifySignature").WithCause(err)
	}

	// Hash the canonical bytes
	hasher := sha256.New()
	hasher.Write(canonicalBytes)
	hashed := hasher.Sum(nil)

	// Verify the signature
	err = rsa.VerifyPSS(publicKey, crypto.SHA256, hashed, signedBytes, nil)
	if err != nil {
		return common.SystemErrorFrom(ErrSignatureVerificationFailed).WithOperation("data.documentFactory.verifySignature").WithCause(errors.Join(ErrSignatureInvalid, err))
	}

	return nil
}

// ============================================================================
// Canonicalization
// ============================================================================

func canonicalize(v any) any {
	switch val := v.(type) {
	case *Document:
		return canonicalize(val.data)
	case Document:
		return canonicalize(val.data)
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
