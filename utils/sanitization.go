package utils

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"go.uber.org/zap"
)

const (
	// SanitizationPoliciesCollection is the system collection name for storing sanitization policies
	SanitizationPoliciesCollection = "__sanitization__"
)

// ============================================================================
// Anansi Persistence Implementation
// ============================================================================

// sanitizationStore implements SanitizationPersistence using ModelCollection.
type sanitizationStore struct {
	persistence    base.Persistence
	collection     base.ModelCollection[data.FieldMaskConfig, *data.FieldMaskConfig]
	collectionName string
	logger         *zap.Logger
}

var _ data.SanitizationPersistence = (*sanitizationStore)(nil)

// NewSanitizationPolicyStore creates a new Anansi-backed persistence layer.
func NewSanitizationPolicyStore(persistence base.Persistence, logger *zap.Logger) (data.SanitizationPersistence, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	store := &sanitizationStore{
		persistence:    persistence,
		collectionName: SanitizationPoliciesCollection,
		logger:         logger,
	}

	_, err := store.ensureCollection(context.Background())
	if err != nil {
		return nil, err
	}

	return store, nil
}

// ensureCollection ensures the sanitization policies collection exists and returns model collection
func (p *sanitizationStore) ensureCollection(ctx context.Context) (base.ModelCollection[data.FieldMaskConfig, *data.FieldMaskConfig], error) {
	if p.collection != nil {
		return p.collection, nil
	}

	var col base.Collection
	ok, err := p.persistence.HasCollection(ctx, p.collectionName)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("sanitizationStore.ensureCollection").
			WithMessage("failed to check for sanitization policies collection")
	}

	if !ok {
		sc := p.createPolicyCollectionSchema()
		col, err = p.persistence.CreateCollection(ctx, sc)
	} else {
		col, err = p.persistence.Collection(ctx, p.collectionName)
	}

	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("sanitizationStore.ensureCollection").
			WithMessage("failed to instantiate sanitization policies collection")
	}
	// Wrap in model collection
	p.collection = collection.NewModelCollection[data.FieldMaskConfig](col, p.logger)
	return p.collection, nil
}

// createPolicyCollectionSchema defines the schema for the sanitization policies collection
func (p *sanitizationStore) createPolicyCollectionSchema() *definition.Schema {
	jsonSchema := fmt.Sprintf(`
{
  "name": "%s",
  "version": "1.0.0",
  "description": "System collection for storing sanitization policies",
  "fields": {
    "f1": {
      "name": "version",
      "type": "string",
      "required": false,
      "description": "Version of the policy"
    },
    "f2": {
      "name": "scope",
      "type": "string",
      "required": true,
      "description": "Scope identifier (must be non-empty)"
    },
    "f3": {
      "name": "policy",
      "type": "enum",
      "schema": { "id": "s1" },
      "required": false,
      "default": "preserve",
      "description": "Default policy for this config"
    },
    "f4": {
      "name": "fields",
      "type": "record",
      "required": false,
      "description": "Field-specific masking policies"
    },
    "f5": {
      "name": "patterns",
      "type": "record",
      "required": false,
      "description": "Regex-based field matching patterns"
    },
    "f6": {
      "name": "obscure",
      "type": "record",
      "required": false,
      "description": "Obscure policy configuration"
    },
    "f7": {
      "name": "salt",
      "type": "string",
      "required": false,
      "description": "Secret key for HMAC hashing"
    },
    "f8": {
      "name": "description",
      "type": "string",
      "required": false,
      "description": "Human-readable description of the policy"
    }
  },
  "schemas": {
    "s1": {
      "name": "SanitizationPolicy",
      "type": "string",
      "values": ["obscure", "preserve", "redact", "hash"]
    }
  },
  "indexes": {
    "idx1": {
      "name": "idx_scope_unique",
      "fields": ["f2"],
      "unique": true
    }
  }
}
`, p.collectionName)

	sc, err := definition.FromJSON([]byte(jsonSchema))
	if err != nil {
		panic(fmt.Sprintf("failed to parse sanitization schema: %v", err))
	}
	return sc
}

// Save persists a sanitization policy (upsert based on scope)
func (p *sanitizationStore) Save(ctx context.Context, config *data.FieldMaskConfig) error {
	if config.Scope == "" {
		return common.NewSystemError("INVALID_SCOPE").
			WithOperation("sanitizationStore.Save").
			WithMessage("scope must be non-empty")
	}

	col, err := p.ensureCollection(ctx)
	if err != nil {
		return err
	}

	// Check if policy with this scope already exists
	existing, err := p.findByScope(ctx, config.Scope)
	isUpdate := err == nil && existing != nil

	if isUpdate {
		_, err = col.Update(ctx, existing.ID, *config)
		if err != nil {
			return common.SystemErrorFrom(err).
				WithOperation("sanitizationStore.Save").
				WithMessagef("failed to update policy for scope %q", config.Scope)
		}
	} else {
		// Create new document
		_, err := col.Create(ctx, *config)
		if err != nil {
			return common.SystemErrorFrom(err).
				WithOperation("sanitizationStore.Save").
				WithMessagef("failed to create policy for scope %q", config.Scope)
		}
	}

	return nil
}

// Load retrieves a sanitization policy for a given scope
func (p *sanitizationStore) Load(ctx context.Context, scope string) (*data.FieldMaskConfig, error) {
	if scope == "" {
		return nil, common.NewSystemError("INVALID_SCOPE").
			WithOperation("sanitizationStore.Load").
			WithMessage("scope must be non-empty")
	}

	docModel, err := p.findByScope(ctx, scope)
	if err != nil {
		return nil, err
	}

	return docModel, nil
}

// findByScope is an internal helper to find a document by scope
func (p *sanitizationStore) findByScope(ctx context.Context, scope string) (*data.FieldMaskConfig, error) {
	col, err := p.ensureCollection(ctx)
	if err != nil {
		return nil, err
	}

	q := query.NewQueryBuilder().
		Where("scope").Eq(scope).
		Limit(1).
		Build()

	results, err := col.Read(ctx, &q)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("sanitizationStore.findByScope").
			WithMessagef("failed to query policy for scope %q", scope)
	}

	if len(results) == 0 {
		return nil, common.SystemErrorFrom(data.ErrSanitizationScopeNotFound).
			WithOperation("sanitizationStore.findByScope").
			WithMessagef("policy not found for scope %q", scope)
	}

	return &results[0], nil
}

// LoadAll retrieves all persisted sanitization policies
func (p *sanitizationStore) LoadAll(ctx context.Context) ([]*data.FieldMaskConfig, error) {
	col, err := p.ensureCollection(ctx)
	if err != nil {
		return nil, err
	}

	q := query.NewQueryBuilder().Build()
	results, err := col.Read(ctx, &q)
	if err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("sanitizationStore.LoadAll").
			WithMessage("failed to load all policies")
	}

	if len(results) == 0 {
		return []*data.FieldMaskConfig{}, nil
	}

	// Convert all document models to domain models
	configs := make([]*data.FieldMaskConfig, 0, len(results))
	for i := range results {
		configs = append(configs, &results[i])
	}

	return configs, nil
}

// Delete removes a persisted sanitization policy
func (p *sanitizationStore) Delete(ctx context.Context, scope string) error {
	if scope == "" {
		return common.NewSystemError("INVALID_SCOPE").
			WithOperation("sanitizationStore.Delete").
			WithMessage("scope must be non-empty")
	}

	col, err := p.ensureCollection(ctx)
	if err != nil {
		// If collection doesn't exist, nothing to delete
		return nil
	}

	// Find the document first to get its ID
	docModel, err := p.findByScope(ctx, scope)
	if err != nil {
		return err
	}

	// Delete by ID
	err = col.DeleteByID(ctx, docModel.ID)
	if err != nil {
		return common.SystemErrorFrom(err).
			WithOperation("sanitizationStore.Delete").
			WithMessagef("failed to delete policy for scope %q", scope)
	}

	return nil
}

// Exists checks if a policy exists for the given scope
func (p *sanitizationStore) Exists(ctx context.Context, scope string) (bool, error) {
	if scope == "" {
		return false, common.NewSystemError("INVALID_SCOPE").
			WithOperation("sanitizationStore.Exists").
			WithMessage("scope must be non-empty")
	}

	col, err := p.ensureCollection(ctx)
	if err != nil {
		// If collection doesn't exist, no policies exist
		return false, nil
	}

	q := query.NewQueryBuilder().
		Where("scope").Eq(scope).
		Limit(1).
		Build()

	results, err := col.Read(ctx, &q)
	if err != nil {
		return false, common.SystemErrorFrom(err).
			WithOperation("sanitizationStore.Exists").
			WithMessagef("failed to check existence for scope %q", scope)
	}

	return len(results) > 0, nil
}
