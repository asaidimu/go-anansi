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

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/google/uuid"
)

// MetadataProvider is a function that returns metadata to be merged into a document.
type MetadataProvider func(ctx context.Context, doc Document) (map[string]any, error)

// MetadataProviderConfig holds a  nested schema, its dependencies and its corresponding provider.
type MetadataProviderConfig struct {
	Name         string
	Schema       *schema.NestedSchemaDefinition
	Dependencies []*schema.NestedSchemaDefinition
	Provider     MetadataProvider
}

// DocumentFactoryConfig holds the complete configuration for the document factory.
type DocumentFactoryConfig struct {
	Providers []MetadataProviderConfig
}

// documentFactory is a singleton responsible for creating and managing documents.
type documentFactory struct {
	mu         sync.RWMutex
	config     DocumentFactoryConfig
	configured bool
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
func ConfigureDocumentFactory(config DocumentFactoryConfig) error {
	var err error

	configureOnce.Do(func() {
		f := getFactory()
		if f.configured {
			err = ErrFactoryAlreadyConfigured
			return
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

// newDocument creates a new document with injected metadata.
func (f *documentFactory) newDocument(ctx context.Context, data map[string]any) (Document, error) {
	if !f.configured {
		return nil, ErrFactoryNotConfigured
	}
	doc := Document(data)
	doc[DocumentID] = strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")

	// Ensure metadata field exists
	meta, ok := doc.Metadata()
	if !ok {
		meta = make(map[string]any)
	}

	// Inject system metadata
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, ok := meta[MetadataCreated]; !ok {
		meta[MetadataCreated] = now
	}
	meta[MetadataUpdated] = now
	meta[MetadataVersion] = 1

	// Apply user-defined metadata providers
	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, providerConfig := range f.config.Providers {
		providerMeta, err := providerConfig.Provider(ctx, doc)
		if err != nil {
			return nil, &DocumentError{
				Operation: "newDocument",
				Message:   fmt.Sprintf("%s: %s", ErrMetadataProviderFailed.Error(), providerConfig.Name),
				Cause:     errors.Join(ErrMetadataProviderFailed, err),
			}
		}
		maps.Copy(meta, providerMeta)
	}

	// Set the metadata back to the document
	doc.SetMetadata(meta)

	// Calculate and set the hash
	hash, err := f.calculateHash(doc)
	if err != nil {
		return nil, err
	}
	meta[MetadataChecksum] = hash
	doc.SetMetadata(meta)

	return doc.Normalize(), nil
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
	if mergedSchema.StructuredFieldsMap == nil {
		mergedSchema.StructuredFieldsMap = make(map[string]*schema.FieldDefinition)
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
		if providerConfig.Schema != nil && providerConfig.Schema.StructuredFieldsMap != nil {
			maps.Copy(mergedSchema.StructuredFieldsMap, providerConfig.Schema.StructuredFieldsMap)
		}
	}

	return mergedSchema, dependencies
}

// calculateHash computes the SHA256 hash of the document,
// excluding the 'checksum' field itself, to ensure a consistent and verifiable identifier.
func (f *documentFactory) calculateHash(doc Document) (string, error) {
	dataToSign := doc.Clone()
	meta, ok := dataToSign.Metadata()

	if !ok {
		return "", &DocumentError{
			Operation: "CalculateHash",
			Message:   ErrNoMetadata.Error(),
			Cause:     ErrNoMetadata,
		}
	}

	// Create a copy of the metadata and remove the hash field itself
	delete(meta, MetadataChecksum)
	dataToSign.SetMetadata(meta)
	// Use canonicalMarshal to ensure consistent key ordering for hashing.
	toSign, err := canonicalMarshal(dataToSign)
	if err != nil {
		return "", &DocumentError{
			Operation: "CalculateHash",
			Message:   ErrFailedToMarshalMetadata.Error(),
			Cause:     errors.Join(ErrFailedToMarshalMetadata, err),
		}
	}

	h := sha256.New()
	h.Write(toSign)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// signDocument signs the entire document (excluding the signature itself) using a private key.
func (f *documentFactory) signDocument(doc Document, privateKey *rsa.PrivateKey) (string, error) {
	// Create a copy of the document and remove the signature field
	docToSign := doc.Clone()
	meta, ok := docToSign.Metadata()
	if ok {
		delete(meta, MetadataSignature)
		docToSign.SetMetadata(meta)
	}

	// Marshal the document to a canonical byte slice
	canonicalBytes, err := canonicalMarshal(docToSign)
	if err != nil {
		return "", &DocumentError{
			Operation: "signDocument",
			Message:   "failed to marshal document for signing",
			Cause:     err,
		}
	}

	// Hash the canonical bytes
	hasher := sha256.New()
	hasher.Write(canonicalBytes)
	hashed := hasher.Sum(nil)

	// Sign the hash
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed)
	if err != nil {
		return "", &DocumentError{
			Operation: "signDocument",
			Message:   "failed to sign document hash",
			Cause:     err,
		}
	}

	return base64.StdEncoding.EncodeToString(signature), nil
}

// verifySignature verifies the signature of a document against a public key.
func (f *documentFactory) verifySignature(doc Document, publicKey *rsa.PublicKey, signature string) error {
	// Decode the signature
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return &DocumentError{
			Operation: "verifySignature",
			Message:   "failed to decode base64 signature",
			Cause:     err,
		}
	}

	// Create a copy of the document and remove the signature field
	docToVerify := doc.Clone()
	meta, ok := docToVerify.Metadata()
	if ok {
		delete(meta, MetadataSignature)
		docToVerify.SetMetadata(meta)
	}

	// Marshal the document to a canonical byte slice
	canonicalBytes, err := canonicalMarshal(docToVerify)
	if err != nil {
		return &DocumentError{
			Operation: "verifySignature",
			Message:   "failed to marshal document for verification",
			Cause:     err,
		}
	}

	// Hash the canonical bytes
	hasher := sha256.New()
	hasher.Write(canonicalBytes)
	hashed := hasher.Sum(nil)

	// Verify the signature
	err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed, sigBytes)
	if err != nil {
		return &DocumentError{
			Operation: "verifySignature",
			Message:   "signature verification failed",
			Cause:     errors.Join(ErrSignatureInvalid, err),
		}
	}

	return nil
}

// canonicalize recursively sorts slices and maps to ensure a consistent hash.
func canonicalize(v any) any {
	switch val := v.(type) {
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
	return json.Marshal(canonicalize(v))
}

// --- Key Loading Helpers ---

// LoadPrivateKey loads a PEM-encoded RSA private key.
func LoadPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("failed to decode PEM block containing private key")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try parsing as PKCS8
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.New("failed to parse private key: not in PKCS1 or PKCS8 format")
		}
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("key is not an RSA private key")
	}
	return key, nil
}

// LoadPublicKey loads a PEM-encoded RSA public key.
func LoadPublicKey(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("failed to decode PEM block containing public key")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.New("failed to parse public key")
	}

	if rsaKey, ok := key.(*rsa.PublicKey); ok {
		return rsaKey, nil
	}

	return nil, errors.New("key is not an RSA public key")
}
