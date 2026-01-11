package utils

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	putils "github.com/asaidimu/go-anansi/v6/core/utils"
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
	collection     base.ModelCollection[data.FieldMaskConfig,*data.FieldMaskConfig]
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
func (p *sanitizationStore) ensureCollection(ctx context.Context) (base.ModelCollection[data.FieldMaskConfig,*data.FieldMaskConfig], error) {
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
func (p *sanitizationStore) createPolicyCollectionSchema() *schema.SchemaDefinition {
	sc := schema.SchemaDefinition{
		Name:        p.collectionName,
		Description: putils.StringPtr("System collection for storing sanitization policies"),
		Version:     "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"f997d3ff-4db1-4941-a4c5-ebe77c4fc54f": {
				Name:        "version",
				Type:        "string",
				Required:    putils.BoolPtr(false),
				Description: putils.StringPtr("Version of the policy"),
			},
			"3f247047-f653-4c99-b53a-636deed01a82": {
				Name:        "scope",
				Type:        "string",
				Required:    putils.BoolPtr(true),
				Description: putils.StringPtr("Scope identifier (must be non-empty)"),
			},
			"431ba4d5-93e7-4f00-8816-fbd494f7bca5": {
				Name:        "policy",
				Type:        "enum",
				Required:    putils.BoolPtr(false),
				Default:     "preserve",
				Values:      []any{"obscure", "preserve", "redact", "hash"},
				Description: putils.StringPtr("Default policy for this config"),
			},
			"11d3cfbd-77ee-451e-a92e-585afbf3d606": {
				Name:        "fields",
				Type:        "record",
				Required:    putils.BoolPtr(false),
				Description: putils.StringPtr("Field-specific masking policies"),
			},
			"2f6418cf-4e44-4396-8a5d-83f7e9baa281": {
				Name:        "patterns",
				Type:        "record",
				Required:    putils.BoolPtr(false),
				Description: putils.StringPtr("Regex-based field matching patterns"),
			},
			"abee6a37-dc45-4c31-89be-1ac9b091ee48": {
				Name:        "obscure",
				Type:        "record",
				Required:    putils.BoolPtr(false),
				Description: putils.StringPtr("Obscure policy configuration"),
			},
			"c8f3a9e2-1d7b-4f5c-9a3e-8b2d4c6f1a9e": {
				Name:        "salt",
				Type:        "string",
				Required:    putils.BoolPtr(false),
				Description: putils.StringPtr("Secret key for HMAC hashing"),
			},
			"d9e4b0f3-2e8c-5g6d-0b4f-9c3e5d7g2b0f": {
				Name:        "description",
				Type:        "string",
				Required:    putils.BoolPtr(false),
				Description: putils.StringPtr("Human-readable description of the policy"),
			},
		},
		Indexes: []schema.IndexOrReference{
			{
				Index: &schema.IndexDefinition{
					Fields: []string{"scope"},
					Unique: putils.BoolPtr(true),
					Name:   "idx_scope_unique",
				},
			},
		},
	}
	return (&sc).MustClone()
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
