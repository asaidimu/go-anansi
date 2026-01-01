package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// DelayedEphemeralInteractor wraps a DatabaseInteractor to simulate a delay.
type DelayedEphemeralInteractor struct {
	query.DatabaseInteractor
	delay time.Duration
}

func (i *DelayedEphemeralInteractor) SelectDocuments(ctx context.Context, schemaDef *schema.SchemaDefinition, dsl *query.Query) ([]data.Document, error) {
	time.Sleep(i.delay)
	return i.DatabaseInteractor.SelectDocuments(ctx, schemaDef, dsl)
}

func newTestSchema(name ...string) *schema.SchemaDefinition {
	sname := "test_collection"
	if name != nil {
		sname = name[0]
	}
	return &schema.SchemaDefinition{
		Name:        sname,
		Version:     "1.0.0",
		Description: stringPtr("test collection"),
		Fields: map[string]*schema.FieldDefinition{
			"id":        {Name: "id", Type: "string", Required: func() *bool { b := true; return &b }(), Unique: func() *bool { b := true; return &b }()},
			"name":      {Name: "name", Type: "string", Required: func() *bool { b := true; return &b }()},
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
	_, err = delayedInteractor.InsertDocuments(context.Background(), schema, []data.Document{{"id": "1", "name": "test"}})
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
