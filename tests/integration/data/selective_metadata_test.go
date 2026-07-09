package data_test

import (
	"context"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupSelectiveTest(t *testing.T) (base.Persistence, func()) {
	sensitiveProvider := data.MetadataProviderConfig{
		Name: "sensitive_provider",
		Schema: &definition.NestedSchema{
			BaseSchema: definition.BaseSchema{
				Name: "sensitive_meta",
				Fields: map[definition.FieldId]definition.Field{
					"sensitivity_level": {Name: "sensitivity_level", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				},
			},
		},
		Provider: func(ctx context.Context, _ *data.Document) (map[string]any, error) {
			if name, ok := ctx.Value(common.CollectionNameContextKey).(string); ok && name == "sensitive_documents" {
				return map[string]any{"sensitivity_level": "high"}, nil
			}
			return nil, nil
		},
	}

	auditedProvider := data.MetadataProviderConfig{
		Name: "audited_provider",
		Schema: &definition.NestedSchema{
			BaseSchema: definition.BaseSchema{
				Name: "audited_meta",
				Fields: map[definition.FieldId]definition.Field{
					"audit_required": {Name: "audit_required", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeBoolean}},
				},
			},
		},
		Provider: func(ctx context.Context, _ *data.Document) (map[string]any, error) {
			if name, ok := ctx.Value(common.CollectionNameContextKey).(string); ok && name == "audited_records" {
				return map[string]any{"audit_required": true}, nil
			}
			return nil, nil
		},
	}

	data.ResetFactoryForTesting()
	testutils.ConfigureDocumentFactory(sensitiveProvider, auditedProvider)

	// We need to create a persistence layer, not just a collection, to create multiple collections.
	interactor, cleanup := testutils.CreateNativeInteractor(t)
	p, err := persistence.NewPersistence(interactor, nil, zap.NewNop(), nil)
	require.NoError(t, err)

	return p, cleanup
}

func TestSelectiveMetadataInjection(t *testing.T) {
	// 1. Set up persistence with selective providers
	p, cleanup := setupSelectiveTest(t)
	defer cleanup()

	// 2. Create collections
	sensitiveSchema := testutils.NewTestSchema("sensitive_documents")
	sensitiveCollection, err := p.CreateCollection(context.Background(), sensitiveSchema)
	require.NoError(t, err)

	auditedSchema := testutils.NewTestSchema("audited_records")
	auditedCollection, err := p.CreateCollection(context.Background(), auditedSchema)
	require.NoError(t, err)

	normalSchema := testutils.NewTestSchema("normal_documents")
	normalCollection, err := p.CreateCollection(context.Background(), normalSchema)
	require.NoError(t, err)

	// 3. Create a document in the "sensitive" collection
	sensitiveContext := context.WithValue(context.Background(), common.CollectionNameContextKey, sensitiveSchema.Name)
	sensitiveDoc, err := data.NewDocument(map[string]any{"name": "secret", "age": 42}, sensitiveContext)
	require.NoError(t, err)
	sensitiveResult, err := sensitiveCollection.CreateOne(sensitiveContext, sensitiveDoc)
	require.NoError(t, err)

	// 4. Assert sensitive document metadata
	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(sensitiveResult.Data.ID()).Build()
	retrievedSensitiveDoc, err := sensitiveCollection.Read(context.Background(), &q)
	require.NoError(t, err)
	require.Equal(t, 1, len(retrievedSensitiveDoc.Data))

	sensitivity, err := retrievedSensitiveDoc.Data[0].GetMetadataString("sensitivity_level")
	assert.NoError(t, err)
	assert.Equal(t, "high", sensitivity)
	_, err = retrievedSensitiveDoc.Data[0].GetMetadataValue("audit_required")
	assert.Error(t, err, "audit_required should not be present in sensitive doc")

	// 5. Create a document in the "audited" collection
	auditedContext := context.WithValue(context.Background(), common.CollectionNameContextKey, auditedSchema.Name)
	auditedDoc, err := data.NewDocument(map[string]any{"name": "financial_record", "age": 35}, auditedContext)
	require.NoError(t, err)
	auditedResult, err := auditedCollection.CreateOne(auditedContext, auditedDoc)
	require.NoError(t, err)

	// 6. Assert audited document metadata
	q = query.NewQueryBuilder().Where(data.DocumentIDField).Eq(auditedResult.Data.ID()).Build()
	retrievedAuditedDoc, err := auditedCollection.Read(context.Background(), &q)
	require.NoError(t, err)
	require.Equal(t, 1, len(retrievedAuditedDoc.Data))

	auditRequired, err := retrievedAuditedDoc.Data[0].GetMetadataBool("audit_required")
	assert.NoError(t, err)
	assert.True(t, auditRequired)
	_, err = retrievedAuditedDoc.Data[0].GetMetadataValue("sensitivity_level")
	assert.Error(t, err, "sensitivity_level should not be present in audited doc")

	// 7. Create a document in the "normal" collection
	normalContext := context.WithValue(context.Background(), common.CollectionNameContextKey, normalSchema.Name)
	normalDoc, err := data.NewDocument(map[string]any{"name": "regular_doc", "age": 28}, normalContext)
	require.NoError(t, err)
	normalResult, err := normalCollection.CreateOne(normalContext, normalDoc)
	require.NoError(t, err)

	// 8. Assert normal document metadata
	q = query.NewQueryBuilder().Where(data.DocumentIDField).Eq(normalResult.Data.ID()).Build()
	retrievedNormalDoc, err := normalCollection.Read(context.Background(), &q)
	require.NoError(t, err)
	require.Equal(t, 1, len(retrievedNormalDoc.Data))

	_, err = retrievedNormalDoc.Data[0].GetMetadataValue("sensitivity_level")
	assert.Error(t, err, "sensitivity_level should not be present in normal doc")
	_, err = retrievedNormalDoc.Data[0].GetMetadataValue("audit_required")
	assert.Error(t, err, "audit_required should not be present in normal doc")
}
