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
	"fmt"
	"maps"
	"reflect"
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

// MetadataProvider is a function that returns metadata to be merged into a document.
type MetadataProvider func(ctx context.Context, doc *Document) (map[string]any, error)

// MetadataProviderConfig holds a  nested schema, its dependencies and its corresponding provider.
type MetadataProviderConfig struct {
	Name         string
	Schema       *schema.NestedSchemaDefinition
	Dependencies []*schema.NestedSchemaDefinition
	Provider     MetadataProvider
}

// DocumentFactoryConfig holds the complete configuration for the document factory.
// TODO, we need a way to configure the sanitizers at runtime to
// accomodate schema changes
type DocumentFactoryConfig struct {
	Providers []MetadataProviderConfig
	// GlobalSanitizer configures field-level data sanitization for events.
	// When set, all events emitted by the persistence layer will have their
	// Input and Output fields sanitized according to the specified policies.
	// This prevents sensitive data from appearing in logs and event subscribers.
	//
	// If nil, no sanitization is applied (NOT recommended for production).
	// Use data.NewSecureDefaultConfig() for sensible defaults.
	GlobalSanitizer *FieldMaskConfig
	// CollectionSanitizers allows per-collection sanitization overrides.
	// Keys are collection names, values are the sanitization config to use
	// for that specific collection. If a collection is not found in this map,
	// the global Sanitizer Config is used.
	//
	// Example:
	//   CollectionSanitizers: map[string]data.FieldMaskConfig{
	//     "credentials": strictConfig,  // Stricter rules for credentials
	//     "users":       moderateConfig, // Moderate rules for users
	//   }
	CollectionSanitizers map[string]*FieldMaskConfig
}

// Sanitization state in the factory
type sanitizationState struct {
	global     *DocumentSanitizer
	collection map[string]*DocumentSanitizer
}

// documentFactory is a singleton responsible for creating and managing documents.
type documentFactory struct {
	mu           sync.RWMutex
	config       DocumentFactoryConfig
	sanitization *sanitizationState
	configured   bool
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

// ConfigureDocumentFactory sets up the document factory singleton.
// It must be called once at application startup.
func ConfigureDocumentFactory(config DocumentFactoryConfig, logger *zap.Logger) error {
	var err error

	configureOnce.Do(func() {
		f := getFactory()
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.configured {
			err = ErrFactoryAlreadyConfigured
			return
		}

		// Initialize sanitization state
		f.sanitization = &sanitizationState{
			collection: make(map[string]*DocumentSanitizer),
		}

		if config.GlobalSanitizer != nil {
			f.sanitization.global = NewDocumentSanitizer(*config.GlobalSanitizer, logger)
		}

		for collectionName, collectionConfig := range config.CollectionSanitizers {
			mergedConfig := mergeConfigs(config.GlobalSanitizer, *collectionConfig)
			f.sanitization.collection[collectionName] = NewDocumentSanitizer(mergedConfig, logger)
		}

		f.config = config
		f.configured = true
	})

	if err != nil {
		return err
	}

	// If called multiple times, configureOnce ensures the block doesn't run again,
	// but we should still signal that the factory is already configured.
	if !getFactory().configured {
		return ErrConfigurationNotApplied
	}

	return nil
}

// getSanitizerForContext returns the appropriate sanitizer based on context.
// If the context contains a collection name, it returns the collection-specific
// sanitizer (which is already merged with global config). Otherwise, it returns
// the global sanitizer.
func (f *documentFactory) getSanitizerForContext(ctx context.Context) *DocumentSanitizer {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.sanitization == nil {
		return nil // No sanitization configured
	}

	// Try to get collection name from context
	if collectionName, ok := ctx.Value(common.CollectionNameContextKey).(string); ok && collectionName != "" {
		// Check for collection-specific sanitizer
		if sanitizer, exists := f.sanitization.collection[collectionName]; exists {
			return sanitizer
		}
	}

	// Fall back to global sanitizer
	return f.sanitization.global
}


// newDocument creates a new document with injected metadata.
func (f *documentFactory) newDocument(ctx context.Context, data map[string]any) (*Document, error) {
	if !f.configured {
		return nil, ErrFactoryNotConfigured
	}
	if data == nil {
		data = make(map[string]any)
	}

	doc := &Document{ctx: ctx, data: data}

	// Ensure document has a valid ID
	if !f.hasValidID(doc) {
		doc.data[DocumentIDField] = strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")
	}

	// Get or initialize metadata - we'll build it up completely before setting
	meta, ok := doc.Metadata()
	if !ok {
		meta = make(map[string]any)
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
				WithMessage(fmt.Sprintf("metadata provider '%s' failed", providerConfig.Name)).
				WithCause(err)
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

	// Preserve context
	normalizedDoc.ctx = ctx
	return normalizedDoc, nil
}


// GetMetadataSchema merges all provider schemas into a single metadata schema
// Conflicts are already handled during configuration, so simple merging is safe here
func GetMetadataSchema() (*schema.NestedSchemaDefinition, []*schema.NestedSchemaDefinition) {
	f := getFactory()
	f.mu.RLock()
	defer f.mu.RUnlock()

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

	for _, providerConfig := range f.config.Providers {
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
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed)
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
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
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
	err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed, sigBytes)
	if err != nil {
		return common.SystemErrorFrom(ErrSignatureVerificationFailed).WithOperation("data.documentFactory.verifySignature").WithCause(errors.Join(ErrSignatureInvalid, err))
	}

	return nil
}

// canonicalize recursively sorts slices and maps to ensure a consistent hash.
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
		// If a float64 represents a whole number, convert it to int64
		if val == float64(int64(val)) {
			return int64(val)
		}
		return val
	case float32:
		// If a float32 represents a whole number, convert it to int64
		if val == float32(int64(val)) {
			return int64(val)
		}
		return val
	case int, int8, int16, int32, int64:
		// Convert all integer types to int64 for consistency
		return reflect.ValueOf(val).Int()
	case uint, uint8, uint16, uint32, uint64:
		// Convert all unsigned integer types to uint64 for consistency
		return reflect.ValueOf(val).Uint()
	default:
		return v
	}
}

// canonicalMarshal marshals a value into a canonical JSON byte slice.
// It uses the canonicalize helper to ensure consistent key ordering for hashing.
func canonicalMarshal(v any) ([]byte, error) {
	data, err := json.Marshal(canonicalize(v))
	if err != nil {
		return nil, common.SystemErrorFrom(err).WithOperation("data.canonicalMarshal").WithCode(common.ErrFailedToMarshalInput.Code).WithMessage("failed to marshal canonicalized data to JSON")
	}
	return data, nil
}

// --- Key Loading Helpers ---

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
