package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// DelayedEphemeralInteractor wraps a DatabaseInteractor to simulate a delay.
type DelayedEphemeralInteractor struct {
	query.DatabaseInteractor
	delay time.Duration
}

func (i *DelayedEphemeralInteractor) SelectDocuments(ctx context.Context, schemaDef *definition.Schema, dsl *query.Query) ([]map[string]any, int64, error) {
	time.Sleep(i.delay)
	docs, count, err := i.DatabaseInteractor.SelectDocuments(ctx, schemaDef, dsl)
	return docs, count, err
}

func newTestSchema(name ...string) *definition.Schema {
	sname := "test_collection"
	if name != nil {
		sname = name[0]
	}
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name:        sname,
			Description: "test collection",
			Fields: map[definition.FieldId]definition.Field{
				"id":   {Name: "id", Required: true, Unique: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"name": {Name: "name", Required: true, FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}
}

func TestQueryEngineCancellation(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	ephemeralInteractor := ephemeral.NewEphemeral()
	delayedInteractor := &DelayedEphemeralInteractor{
		DatabaseInteractor: ephemeralInteractor,
		delay:              200 * time.Millisecond,
	}

	queryEngine := query.NewQueryEngine(delayedInteractor.Capabilities(), logger)

	schema := newTestSchema("cancellation_test")
	manager := delayedInteractor.SchemaManager()
	err = manager.CreateCollection(context.Background(), *schema)
	require.NoError(t, err)

	// Create a document to query
	_, err = delayedInteractor.InsertDocuments(context.Background(), schema, []map[string]any{{"id": "1", "name": "test"}})
	require.NoError(t, err)

	// Create a context with a short timeout
	atx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ctx := query.WithInteractor(atx, delayedInteractor)

	// This query will hang because the interactor has a delay.
	// We expect the context to be cancelled and the query engine to return an error.
	_, err = queryEngine.Query(ctx, schema, &query.Query{})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
