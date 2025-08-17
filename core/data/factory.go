package data

import (
	"maps"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// MetadataProvider is a function that returns metadata to be merged into a document.
type MetadataProvider func(doc Document) (map[string]any, error)

// documentFactory is a singleton responsible for creating and managing documents.
type documentFactory struct {
	mu         sync.RWMutex
	providers  map[string]MetadataProvider
	schemas    map[string]*schema.NestedSchemaDefinition
}

var (
	factoryOnce sync.Once
	factory     *documentFactory
)

func getFactory() *documentFactory {
	factoryOnce.Do(func() {
		factory = &documentFactory{
			providers: make(map[string]MetadataProvider),
			schemas:   make(map[string]*schema.NestedSchemaDefinition),
		}
	})
	return factory
}

// newDocument creates a new document with injected metadata.
func (f *documentFactory) newDocument(data map[string]any) (Document, error) {
	doc := Document(data)

	// Ensure metadata field exists
	meta, ok := doc.Metadata()
	if !ok {
		meta = make(map[string]any)
	}

	// Inject system metadata
	now := time.Now().UnixNano()
	if _, ok := meta["created"]; !ok {
		meta["created"] = now
	}
	meta["updated"] = now
	meta["version"] = 1

	// Apply user-defined metadata providers
	f.mu.RLock()
	defer f.mu.RUnlock()

	for name, provider := range f.providers {
		providerMeta, err := provider(doc)
		if err != nil {
			return nil, &DocumentError{
				Operation: "newDocument",
				Message:   fmt.Sprintf("%s: %s", ErrMetadataProviderFailed.Error(), name),
				Cause:     fmt.Errorf("%w: %w", ErrMetadataProviderFailed, err),
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
	meta, ok := d.Metadata()
	if !ok {
		return &DocumentError{
			Operation: "TouchMetadata",
			Message:   "no metadata found",
			Cause:     ErrKeyNotFound,
		}
	}

	// Update timestamp and version
	meta["updated"] = time.Now().UnixNano()
	if version, ok := CoerceToInt(meta["version"]); ok {
		meta["version"] = version + 1
	} else {
		meta["version"] = 1
	}

	// Recalculate hash
	hash, err := getFactory().calculateHash(d)
	if err != nil {
		return err
	}
	meta["hash"] = hash
	d.SetMetadata(meta)

	return nil
}


// RegisterMetadata allows users to extend the document metadata.
func RegisterMetadata(name string, schema *schema.NestedSchemaDefinition, provider MetadataProvider) error {
	f := getFactory()
	f.mu.Lock()
	defer f.mu.Unlock()

	defaultSchema := DefaultMetadataSchema()
	for fieldName := range schema.StructuredFieldsMap {
		if _, exists := defaultSchema.StructuredFieldsMap[fieldName]; exists {
			return fmt.Errorf("%w: %s", ErrConflictingMetadataField, fieldName)
		}
	}

	f.providers[name] = provider
	f.schemas[name] = schema
	return nil
}

func GetMetadataSchema() *schema.NestedSchemaDefinition {
	f := getFactory()
	f.mu.RLock()
	defer f.mu.RUnlock()

	mergedSchema := DefaultMetadataSchema()

	for _, customSchema := range f.schemas {
		maps.Copy(mergedSchema.StructuredFieldsMap, customSchema.StructuredFieldsMap)
	}

	return mergedSchema
}

// calculateHash computes the SHA256 hash of the document's _metadata_ block,
// excluding the 'hash' field itself, to ensure a consistent and verifiable identifier.
func (f *documentFactory) calculateHash(doc Document) (string, error) {
	meta, ok := doc.Metadata()
	if !ok {
		return "", nil
	}

	// Create a copy of the metadata and remove the hash field itself
	hashableMeta := make(map[string]any)
	for k, v := range meta {
		if k != "hash" {
			hashableMeta[k] = v
		}
	}

	// Marshal to a canonical JSON format (sorted keys)
	jsonBytes, err := canonicalMarshal(hashableMeta)
	if err != nil {
		return "", &DocumentError{
			Operation: "calculateHash",
			Message:   ErrFailedToMarshalMetadata.Error(),
			Cause:     fmt.Errorf("%w: %w", ErrFailedToMarshalMetadata, err),
		}
	}

	hash := sha256.Sum256(jsonBytes)
	return hex.EncodeToString(hash[:]), nil
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
	default:
		return v
	}
}

// canonicalMarshal marshals a value into a canonical JSON byte slice.
// It uses the canonicalize helper to ensure consistent key ordering for hashing.
func canonicalMarshal(v any) ([]byte, error) {
	return json.Marshal(canonicalize(v))
}
