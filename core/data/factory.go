package data

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
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
	HmacSecret []byte
	Providers  []MetadataProviderConfig
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
		if len(config.HmacSecret) == 0 {
			err = ErrHmacSecretNotConfigured
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

	// Ensure metadata field exists
	meta, ok := doc.Metadata()
	if !ok {
		meta = make(map[string]any)
	}

	// Inject system metadata
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, ok := meta["created"]; !ok {
		meta["created"] = now
	}
	meta["updated"] = now
	meta["version"] = 1

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
	meta["hash"] = hash
	doc.SetMetadata(meta)

	return doc.Normalize(), nil
}

// TouchMetadata updates the 'updated' timestamp and recalculates the hash.
func (d Document) TouchMetadata() error {
	f := getFactory()
	if !f.configured {
		return ErrFactoryNotConfigured
	}

	meta, ok := d.Metadata()
	if !ok {
		return &DocumentError{
			Operation: "TouchMetadata",
			Message:   "no metadata found",
			Cause:     ErrKeyNotFound,
		}
	}

	// Update timestamp and version

	// Inject system metadata
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	meta["updated"] = now
	if version, ok := utils.CoerceToPrimitiveValue[int](meta["version"]); ok {
		meta["version"] = version + 1
	} else {
		meta["version"] = 1
	}

	// Recalculate hash
	hash, err := f.calculateHash(d)
	if err != nil {
		return err
	}
	meta["hash"] = hash
	d.SetMetadata(meta)

	return nil
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

// calculateHash computes the SHA256 hash of the document's _metadata_ block,
// excluding the 'hash' field itself, to ensure a consistent and verifiable identifier.
func (f *documentFactory) calculateHash(doc Document) (string, error) {
	if len(f.config.HmacSecret) == 0 {
		return "", ErrHmacSecretNotConfigured
	}

	meta, ok := doc.Metadata()
	if !ok {
		return "", &DocumentError{
			Operation: "CalculateHash",
			Message:   ErrNoMetadata.Error(),
			Cause:     ErrNoMetadata,
		}
	}

	// Create a copy of the metadata and remove the hash field itself
	dataToSign := make(map[string]any, len(meta))
	for k, v := range meta {
		if k != "hash" {
			dataToSign[k] = v
		}
	}

	// Use canonicalMarshal to ensure consistent key ordering for hashing.
	toSign, err := canonicalMarshal(dataToSign)
	if err != nil {
		return "", &DocumentError{
			Operation: "CalculateHash",
			Message:   ErrFailedToMarshalMetadata.Error(),
			Cause:     errors.Join(ErrFailedToMarshalMetadata, err),
		}
	}

	h := hmac.New(sha256.New, f.config.HmacSecret)
	h.Write(toSign)
	return hex.EncodeToString(h.Sum(nil)), nil
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

func (d Document) HashMetadata() error {
	meta, ok := d.Metadata()

	if !ok {
		return &DocumentError{
			Operation: "HashMetadata",
			Message:   ErrNoMetadata.Error(),
			Cause:     ErrNoMetadata,
		}
	}

	hash, err := getFactory().calculateHash(d)
	if err != nil {
		return err
	}

	meta["hash"] = hash
	d.SetMetadata(meta)
	return nil
}

// VerifyHash checks the intergrity of the metadata field.
func (d Document) VerifyHash() bool {
	meta, ok := d.Metadata()
	if !ok {
		return false
	}

	providedHash, ok := meta["hash"].(string)

	if !ok {
		return false
	}

	calculatedHash, err := getFactory().calculateHash(d)
	if err != nil {
		return false
	}

	return hmac.Equal([]byte(providedHash), []byte(calculatedHash))
}

func (d Document) MustVerifyHash() {
	meta, ok := d.Metadata()
	if !ok {
		panic(ErrNoMetadata.Error())
	}

	providedHash, ok := meta["hash"].(string)
	if !ok {
		panic(ErrMetadataKeyNotFound.Error())
	}

	calculatedHash, err := getFactory().calculateHash(d)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", ErrFailedToCalculateHash.Error(), err))
	}

	if !hmac.Equal([]byte(providedHash), []byte(calculatedHash)) {
		panic(fmt.Sprintf("%s: expected %s, got %s", ErrHashMismatch.Error(), providedHash, calculatedHash))
	}
}

