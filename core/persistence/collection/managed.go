package collection

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// managedCollection is a decorator that wraps a base.PersistenceCollectionInterface to provide
// transparent metadata management, versioning, and optimistic locking.
type managedCollection struct {
	physicalName  string
	logicalName   string
	wrapped       base.Collection
	options       *base.MetadataOptions
	schema        *schema.SchemaDefinition
	resolveSchema func(ctx context.Context, name string) (string, *schema.SchemaDefinition, error)
}

// newManagedCollection creates a new ManagedCollection decorator.
func newManagedCollection(
	schema *schema.SchemaDefinition,
	logicalName string,
	physicalName string,
	wrapped base.Collection,
	resolveSchema func(ctx context.Context, name string) (string, *schema.SchemaDefinition, error),
	opts *base.MetadataOptions,
) (*managedCollection, error) {
	if opts == nil || opts.HmacSecretKey == nil || len(opts.HmacSecretKey) == 0 {
		return nil, fmt.Errorf("HMAC secret key must be provided for managed collection")
	}
	if opts.MetadataSchema == nil {
		opts.MetadataSchema = DefaultMetadataSchema()
	}

	return &managedCollection{
		schema:        schema,
		physicalName:  physicalName,
		logicalName:   logicalName,
		wrapped:       wrapped,
		options:       opts,
		resolveSchema: resolveSchema,
	}, nil
}

// --- Core Method Overrides ---

// CreateOne handles the creation of a single document.
func (c *managedCollection) CreateOne(ctx context.Context, doc data.Document) (*base.CreateResult, error) {
	results, err := c.CreateMany(ctx, []data.Document{doc})
	if err != nil {
		return nil, err
	}
	return &results[0], nil
}

// CreateMany handles the creation of multiple documents, providing a rich result for each.
func (c *managedCollection) CreateMany(ctx context.Context, docs []data.Document) ([]base.CreateResult, error) {
	results := make([]base.CreateResult, len(docs))
	valid := 0
	metadata, err := c.createEntryMetadata()

	if err != nil {
		return nil, fmt.Errorf("failed to get custom metadata from provider: %w", err)
	}

	for i, doc := range docs {
		result, err := c.Validate(ctx, doc, false)

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
		return results, fmt.Errorf("validation failed for %d documents", len(docs)-valid)
	}

	return c.wrapped.CreateMany(ctx, docs)
}

func (c *managedCollection) createEntryMetadata(existing ...map[string]any) (map[string]any, error) {
	var meta map[string]any

	if existing != nil {
		meta = existing[0]
	} else {
		now := time.Now().Unix()
		meta = map[string]any{
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
	}

	hash, err := c.calculateHash(meta)
	if err != nil {
		return nil, err
	}

	meta["hash"] = hash
	return meta, nil
}

// Read fetches documents and enriches them with the metadata block for transport.
func (c *managedCollection) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	var fq *query.Query = q

	if q.Joins != nil {
		modifiedQuery, err := q.Clone()
		if err != nil {
			return nil, base.NewPersistenceError("failed to clone query for join resolution", err)
		}
		// Translate logical join targets to physical names
		for i, join := range modifiedQuery.Joins {
			name := join.Target
			if c.resolveSchema == nil {
				return nil, base.NewPersistenceError("physical name resolver function is not set", nil)
			}
			physicalName, schema, err := c.resolveSchema(ctx, name.Name)

			if err != nil {
				return nil, base.NewPersistenceError(fmt.Sprintf("failed to resolve physical name for join target '%s': %v", join.Target.Name, err), err)
			}

			modifiedQuery.Joins[i].Target.Name = physicalName
			modifiedQuery.Joins[i].Target.Schema = schema

			if join.Target.Alias == nil {
				modifiedQuery.Joins[i].Target.Alias = &name.Name
			}
		}

		fq = modifiedQuery
	}

	fq.Target = &query.QueryTarget{
		Name:  c.physicalName,
		Alias: &c.logicalName,
	}

	if fq.Target == nil {
		fmt.Printf("dsl.Target is NIL !!!\n")
	}

	// Pass the call to the wrapped collection first
	result, err := c.wrapped.Read(ctx, fq)

	if err != nil {
		return nil, err
	}

	return result, nil
}

// Update verifies the integrity of the metadata block, performs an optimistic lock check,
// and updates the document and its metadata.
func (c *managedCollection) Update(ctx context.Context, params *base.CollectionUpdate) (int, error) {
	meta, ok := params.Data.Metadata()
	if !ok {
		return 0, fmt.Errorf("update operation requires a valid metadata block, found")
	}

	var err error

	if params.Recover {
		meta, err = c.createEntryMetadata(meta) // recalculates the hash
		if err != nil {
			return 0, fmt.Errorf("failed to create new metadata for recovery: %w", err)
		}
	}

	if !c.verifyHash(meta) {
		return 0, fmt.Errorf("metadata hash verification failed: data may be tampered")
	}

	d := params.Data.StripMetadata()
	result, err := c.Validate(ctx, d, true)

	if err != nil {
		return 0, fmt.Errorf("Failed to update document in collection: %w", err)
	}

	if !result.Valid {
		return 0, fmt.Errorf("Update failed validation: %v", result.Issues)
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
			Field:    fmt.Sprintf("%s.version", data.MetadataFieldName),
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

	return c.wrapped.Update(ctx, params)
}

// --- Passthrough Methods ---
func (c *managedCollection) Delete(ctx context.Context, q *query.QueryFilter, unsafe bool) (int, error) {
	return c.wrapped.Delete(ctx, q, unsafe)
}

func (c *managedCollection) Validate(ctx context.Context, data data.Document, loose bool) (*schema.ValidationResult, error) {
	return c.wrapped.Validate(ctx, data, loose)
}

func (c *managedCollection) Metadata(ctx context.Context, filter *base.MetadataFilter, forceRefresh bool) (*base.CollectionMetadata, error) {
	return c.wrapped.Metadata(ctx, filter, forceRefresh)
}

func (c *managedCollection) RegisterSubscription(ctx context.Context, options base.RegisterSubscriptionOptions) string {
	return c.wrapped.RegisterSubscription(ctx, options)
}

func (c *managedCollection) UnregisterSubscription(ctx context.Context, id string) {
	c.wrapped.UnregisterSubscription(ctx, id)
}

func (c *managedCollection) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) {
	return c.wrapped.Subscriptions(ctx)
}

func (c *managedCollection) Capabilities(ctx context.Context) *query.Capabilities {
	return c.wrapped.Capabilities(ctx)
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
