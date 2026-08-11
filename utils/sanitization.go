package utils

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
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
// loaded through the __sanitization__ collection. The anansi tags are the
// single source of truth for the persisted collection schema (see
// createPolicyCollectionSchema): field IDs are derived deterministically by the
// DTO schema extractor, which always emits IDs that sort after the injected
// system fields (_id_, _metadata_) — hand-authoring field UUIDs here would
// re-introduce the "field sorts before system field" compile failure. Structured
// values (fields, obscure) are kept as schema-free record maps. Regex Patterns
// are intentionally not persisted — they are runtime configuration supplied at
// startup via sanitize.Configure.
type sanitizationPolicyModel struct {
	document.DocumentModel
	Version       string                     `json:"version,omitempty" anansi:"version,omitempty"`
	Scope         string                     `json:"scope" anansi:"scope,required=true"`
	DefaultPolicy sanitize.MaskedFieldPolicy `json:"default,omitempty" anansi:"policy,omitempty,type=enum,values=obscure|preserve|redact|hash,default=preserve"`
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

// createPolicyCollectionSchema derives the schema for the sanitization policies
// collection from the sanitizationPolicyModel struct. Field and enum-schema IDs
// are produced by the DTO schema extractor (core/data), whose deterministic
// UUIDv7 generation starts from an epoch that sorts strictly after the injected
// system fields (_id_, _metadata_). This guarantees the enriched schema's system
// fields always lead, so the schema compiles regardless of future field
// additions. Hand-authored UUIDs would silently regress this invariant.
func (p *sanitizationStore) createPolicyCollectionSchema() *definition.Schema {
	jsonSchema, err := data.SchemaFrom[*sanitizationPolicyModel](true)
	if err != nil {
		panic(fmt.Sprintf("failed to generate sanitization schema: %v", err))
	}

	sc, err := definition.FromJSON(jsonSchema)
	if err != nil {
		panic(fmt.Sprintf("failed to parse sanitization schema: %v", err))
	}
	sc.Name = p.collectionName
	sc.Description = "System collection for storing sanitization policies"

	// The DTO extractor does not emit indexes; add the unique scope index here.
	// Its ID is a valid UUIDv7 that also sorts after the system fields.
	sc.Indexes = map[definition.IndexID]definition.Index{
		"019fba9e-d800-727f-ac81-b8d2562ecae8": {
			Name:   "idx_scope_unique",
			Fields: []definition.FieldName{"scope"},
			Type:   definition.IndexTypeUnique,
			Unique: true,
		},
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
