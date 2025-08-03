package collection

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// managedCollection is a decorator that wraps a base.PersistenceCollectionInterface to provide
// transparent metadata management, versioning, and optimistic locking.
type managedCollection struct {
	wrapped base.Collection
	options *base.MetadataOptions
}

// newManagedCollection creates a new ManagedCollection decorator.
func newManagedCollection(
	wrapped base.Collection,
	opts *base.MetadataOptions,
) (*managedCollection, error) {
	if opts == nil || opts.HmacSecretKey == nil || len(opts.HmacSecretKey) == 0 {
		return nil, fmt.Errorf("HMAC secret key must be provided for managed collection")
	}
	if opts.MetadataSchema == nil {
		opts.MetadataSchema = DefaultMetadataSchema()
	}

	return &managedCollection{
		wrapped: wrapped,
		options: opts,
	}, nil
}

// --- Core Method Overrides ---

// CreateOne handles the creation of a single document.
func (c *managedCollection) CreateOne(doc common.Document) (*base.CreateResult, error) {
	results, err := c.CreateMany([]common.Document{doc})
	if err != nil {
		return nil, err
	}
	return &results[0], nil
}

// CreateMany handles the creation of multiple documents, providing a rich result for each.
func (c *managedCollection) CreateMany(docs []common.Document) ([]base.CreateResult, error) {
	results := make([]base.CreateResult, len(docs))
	valid := 0

	metadata, err := c.createEntryMetadata()

	if err != nil {
		return nil, fmt.Errorf("failed to get custom metadata from provider: %w", err)
	}

	for i, doc := range docs {
		result, err := c.Validate(doc, false)

		if err != nil {
			return nil, fmt.Errorf("Failed to create document in collection: %w", err)
		}

		if !result.Valid {
			results[i] = base.CreateResult{Status: base.StatusFailedValidation, Data: doc, Issues: result.Issues}
			continue
		}
		doc.SetMetadata(metadata)
		valid++
	}

	if valid != len(docs) {
		return results, nil
	}

	return c.wrapped.CreateMany(docs)
}

func (c *managedCollection) createEntryMetadata() (map[string]any, error) {
	now := time.Now().Unix()
	meta := map[string]any{
		"version": 1,
		"created": now,
		"updated": now,
	}

	customMeta := make(map[string]any)
	var err error
	if c.options.DefaultProvider != nil {
		customMeta, err = c.options.DefaultProvider()
		if err != nil {
			return nil, fmt.Errorf("failed to get custom metadata from provider: %w", err)
		}
	}

	maps.Copy(meta, customMeta)

	hash, err := c.calculateHash(meta)
	if err != nil {
		return nil, err
	}

	meta["hash"] = hash
	return meta, nil
}

// Read fetches documents and enriches them with the metadata block for transport.
func (c *managedCollection) Read(q *query.Query) (*query.QueryResult, error) {
	// Pass the call to the wrapped collection first
	result, err := c.wrapped.Read(q)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Update verifies the integrity of the metadata block, performs an optimistic lock check,
// and updates the document and its metadata.
func (c *managedCollection) Update(params *base.CollectionUpdate) (int, error) {
	meta, ok := params.Data.Metadata()
	if !ok {
		return 0, fmt.Errorf("update operation requires a valid metadata block, found")
	}

	if !c.verifyHash(meta) {
		return 0, fmt.Errorf("metadata hash verification failed: data may be tampered")
	}

	// 2. Prepare for optimistic locking
	version, ok := utils.CoercePrimitiveValue[float64](meta["version"])

	if !ok {
		return 0, fmt.Errorf("invalid or missing version in metadata, %v", meta)
	}

	// Modify the filter to include the version check
	if params.Filter == nil {
		params.Filter = &query.QueryFilter{}
	}

	versionFilter := query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    fmt.Sprintf("%s.version", common.MetadataFieldName),
			Operator: query.ComparisonOperatorEq,
			Value: query.FilterValue{
				NumberVal: &version,
			},
		},
	}

	qb := query.NewQueryBuilder()
	qb = qb.AndFilter(*params.Filter).AndFilter(versionFilter)
	combined := qb.Build().Filters
	params.Filter = combined

	// 3. Prepare the new metadata for the update
	newVersion := int(version) + 1
	newMeta := make(map[string]any)
	for k, v := range meta {
		if k != "hash" {
			newMeta[k] = v
		}
	}

	newMeta["version"] = newVersion
	newMeta["updated"] = time.Now().Unix()

	newHash, err := c.calculateHash(newMeta)
	if err != nil {
		return 0, err
	}
	newMeta["hash"] = newHash

	params.Data.SetMetadata(newMeta)

	return c.wrapped.Update(params)
}

// --- Passthrough Methods ---

func (c *managedCollection) Delete(q *query.QueryFilter, unsafe bool) (int, error) {
	return c.wrapped.Delete(q, unsafe)
}

func (c *managedCollection) Validate(data common.Document, loose bool) (*schema.ValidationResult, error) {
	return c.wrapped.Validate(data, loose)
}

func (c *managedCollection) Metadata(filter *base.MetadataFilter, forceRefresh bool) (*base.CollectionMetadata, error) {
	return c.wrapped.Metadata(filter, forceRefresh)
}

func (c *managedCollection) RegisterSubscription(options base.RegisterSubscriptionOptions) string {
	return c.wrapped.RegisterSubscription(options)
}

func (c *managedCollection) UnregisterSubscription(id string) {
	c.wrapped.UnregisterSubscription(id)
}

func (c *managedCollection) Subscriptions() ([]base.SubscriptionInfo, error) {
	return c.wrapped.Subscriptions()
}

func (c *managedCollection) Capabilities() *query.Capabilities {
	return c.wrapped.Capabilities()
}

// --- Helper Functions ---

// calculateHash generates a stable HMAC-SHA256 hash for a given metadata map.
func (c *managedCollection) calculateHash(meta map[string]any) (string, error) {
	keys := make([]string, 0, len(meta))
	for k := range meta {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var toSign []byte
	for _, k := range keys {
		jsonVal, err := json.Marshal(meta[k])
		if err != nil {
			return "", fmt.Errorf("failed to marshal metadata value for key %s: %w", k, err)
		}
		toSign = append(toSign, []byte(k)...)
		toSign = append(toSign, jsonVal...)
	}

	h := hmac.New(sha256.New, c.options.HmacSecretKey)
	h.Write(toSign)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifyHash recalculates the hash of the metadata and compares it to the provided hash.
func (c *managedCollection) verifyHash(meta map[string]any) bool {
	providedHash, ok := meta["hash"].(string)
	if !ok {
		return false
	}

	calculatedHash, err := c.calculateHash(meta)
	if err != nil {
		return false
	}

	return hmac.Equal([]byte(providedHash), []byte(calculatedHash))
}
