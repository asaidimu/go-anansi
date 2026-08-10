package utils

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/document"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/sanitize"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"go.uber.org/zap"
)

const (
	// SanitizationPoliciesCollection is the system collection name for storing sanitization policies
	SanitizationPoliciesCollection = "__sanitization__"
)

// ============================================================================
// Anansi Persistence Implementation
// ============================================================================

// sanitizationPolicyModel is the document model persisted by sanitizationStore.
// It adapts the plain sanitize.FieldMaskConfig (which intentionally carries no
// document model embed) to the document pipeline so policies can be stored and
// loaded through the __sanitization__ collection. The schema-declared fields
// (version, scope, policy, fields, patterns, obscure, salt, description) map to
// the tagged fields below; structured values (fields, obscure) are kept as
// schema-free record maps.
type sanitizationPolicyModel struct {
	document.DocumentModel
	Version       string                     `json:"version,omitempty" anansi:"version,omitempty"`
	Scope         string                     `json:"scope" anansi:"scope"`
	DefaultPolicy sanitize.MaskedFieldPolicy `json:"default,omitempty" anansi:"policy,omitempty"`
	Fields        map[string]any             `json:"fields,omitempty" anansi:"fields,omitempty"`
	Obscure       map[string]any             `json:"obscure,omitempty" anansi:"obscure,omitempty"`
	HashSecret    string                     `json:"salt,omitempty" anansi:"salt,omitempty"`
	Description   string                     `json:"description,omitempty" anansi:"description,omitempty"`
}

// modelFromConfig converts a sanitize policy into the persisted model.
// Note: regex Patterns are intentionally not persisted — they are runtime
// configuration supplied at startup via sanitize.Configure.
func modelFromConfig(config *sanitize.FieldMaskConfig) *sanitizationPolicyModel {
	m := &sanitizationPolicyModel{
		Version:       config.Version,
		Scope:         config.Scope,
		DefaultPolicy: config.DefaultPolicy,
		HashSecret:    config.HashSecret,
		Description:   config.Description,
	}
	if len(config.Fields) > 0 {
		m.Fields = make(map[string]any, len(config.Fields))
		for field, policy := range config.Fields {
			m.Fields[field] = string(policy)
		}
	}
	if config.ObscureConfig != (sanitize.ObscureConfig{}) {
		b, err := json.Marshal(config.ObscureConfig)
		if err == nil {
			_ = json.Unmarshal(b, &m.Obscure)
		}
	}
	return m
}

// configFromModel converts a persisted model back into a sanitize policy.
func configFromModel(m *sanitizationPolicyModel) *sanitize.FieldMaskConfig {
	config := &sanitize.FieldMaskConfig{
		Version:       m.Version,
		Scope:         m.Scope,
		DefaultPolicy: m.DefaultPolicy,
		HashSecret:    m.HashSecret,
		Description:   m.Description,
	}
	if len(m.Fields) > 0 {
		config.Fields = make(map[string]sanitize.MaskedFieldPolicy, len(m.Fields))
		for field, v := range m.Fields {
			if s, ok := v.(string); ok {
				config.Fields[field] = sanitize.MaskedFieldPolicy(s)
			}
		}
	}
	if len(m.Obscure) > 0 {
		b, err := json.Marshal(m.Obscure)
		if err == nil {
			_ = json.Unmarshal(b, &config.ObscureConfig)
		}
	}
	return config
}

// sanitizationStore implements sanitize.SanitizationPersistence using ModelCollection.
type sanitizationStore struct {
	persistence    base.Persistence
	collection     *collection.ModelCollection[*sanitizationPolicyModel]
	collectionName string
	logger         *zap.Logger
}

var _ sanitize.SanitizationPersistence = (*sanitizationStore)(nil)

// NewSanitizationPolicyStore creates a new Anansi-backed persistence layer.
func NewSanitizationPolicyStore(persistence base.Persistence, logger *zap.Logger) (sanitize.SanitizationPersistence, error) {
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
func (p *sanitizationStore) ensureCollection(ctx context.Context) (*collection.ModelCollection[*sanitizationPolicyModel], error) {
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
	mc, err := collection.NewModelCollection[*sanitizationPolicyModel](col, p.logger)
	if err != nil {
		return nil, err
	}
	p.collection = mc
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
    "019f32a8-e475-7e82-9f45-4aee484e8359": {
      "name": "version",
      "type": "string",
      "required": false,
      "description": "Version of the policy"
    },
    "019f32a8-e475-7e91-85dd-16594685bc76": {
      "name": "scope",
      "type": "string",
      "required": true,
      "description": "Scope identifier (must be non-empty)"
    },
    "019f32a8-e475-7be9-9cbd-c5af42aab072": {
      "name": "policy",
      "type": "enum",
      "schema": {
        "id": "019f32a8-e475-728a-9346-241344178f91"
      },
      "required": false,
      "default": "preserve",
      "description": "Default policy for this config"
    },
    "019f32a8-e475-7b3a-b5a0-717f48bfb5ff": {
      "name": "fields",
      "type": "record",
      "required": false,
      "description": "Field-specific masking policies"
    },
    "019f32a8-e475-702b-8bae-84b24213ac4d": {
      "name": "patterns",
      "type": "record",
      "required": false,
      "description": "Regex-based field matching patterns"
    },
    "019f32a8-e475-7472-8a91-62ce450fa515": {
      "name": "obscure",
      "type": "record",
      "required": false,
      "description": "Obscure policy configuration"
    },
    "019f32a8-e475-7e3d-a83e-a94641e3b5a0": {
      "name": "salt",
      "type": "string",
      "required": false,
      "description": "Secret key for HMAC hashing"
    },
    "019f32a8-e475-787f-85fb-9ffa4a0bb370": {
      "name": "description",
      "type": "string",
      "required": false,
      "description": "Human-readable description of the policy"
    }
  },
  "schemas": {
    "019f32a8-e475-728a-9346-241344178f91": {
      "name": "SanitizationPolicy",
      "type": "string",
      "values": [
        "obscure",
        "preserve",
        "redact",
        "hash"
      ]
    }
  },
  "indexes": {
    "019f32a8-e475-756d-8621-687745af9a4b": {
      "name": "idx_scope_unique",
      "fields": [
        "scope"
      ],
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
func (p *sanitizationStore) Save(ctx context.Context, config *sanitize.FieldMaskConfig) error {
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
	model := modelFromConfig(config)

	if isUpdate {
		_, err = col.Update(ctx, existing.ID, model)
		if err != nil {
			return common.SystemErrorFrom(err).
				WithOperation("sanitizationStore.Save").
				WithMessagef("failed to update policy for scope %q", config.Scope)
		}
	} else {
		// Create new document
		_, err := col.Create(ctx, model)
		if err != nil {
			return common.SystemErrorFrom(err).
				WithOperation("sanitizationStore.Save").
				WithMessagef("failed to create policy for scope %q", config.Scope)
		}
	}

	return nil
}

// Load retrieves a sanitization policy for a given scope
func (p *sanitizationStore) Load(ctx context.Context, scope string) (*sanitize.FieldMaskConfig, error) {
	if scope == "" {
		return nil, common.NewSystemError("INVALID_SCOPE").
			WithOperation("sanitizationStore.Load").
			WithMessage("scope must be non-empty")
	}

	docModel, err := p.findByScope(ctx, scope)
	if err != nil {
		return nil, err
	}

	return configFromModel(docModel), nil
}

// findByScope is an internal helper to find a document by scope
func (p *sanitizationStore) findByScope(ctx context.Context, scope string) (*sanitizationPolicyModel, error) {
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
		return nil, common.SystemErrorFrom(sanitize.ErrSanitizationScopeNotFound).
			WithOperation("sanitizationStore.findByScope").
			WithMessagef("policy not found for scope %q", scope)
	}

	return results[0], nil
}

// LoadAll retrieves all persisted sanitization policies
func (p *sanitizationStore) LoadAll(ctx context.Context) ([]*sanitize.FieldMaskConfig, error) {
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
		return []*sanitize.FieldMaskConfig{}, nil
	}

	// Convert all document models to configs
	configs := make([]*sanitize.FieldMaskConfig, 0, len(results))
	for i := range results {
		configs = append(configs, configFromModel(results[i]))
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
