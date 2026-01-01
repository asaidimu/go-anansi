package persistence_test

import (
	"context"
	"errors"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/ephemeral"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newCartTestSchema(name string) *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Version: "1.0.0",
		Name:    name,
		Fields: map[string]*schema.FieldDefinition{
			"name":     {Name: "name", Type: "string"},
			"quantity": {Name: "quantity", Type: "integer"},
			"amount":   {Name: "amount", Type: "integer"},
			"itemId":   {Name: "itemId", Type: "string"},
			"saleId":   {Name: "saleId", Type: "string"},
		},
	}
}

func setupCartTest(t *testing.T) (base.Persistence, data.Document, func()) {
	interactor := ephemeral.NewEphemeral()

	logger := zap.NewNop()
	// require.NoError(t, err)

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Create collections
	schemas := []*schema.SchemaDefinition{
		newCartTestSchema("inventory"),
		newCartTestSchema("sales"),
		newCartTestSchema("payments"),
	}

	for _, s := range schemas {
		_, err := p.CreateCollection(context.Background(), *s)
		require.NoError(t, err)
	}

	// Seed inventory
	inventory, err := p.Collection(context.Background(), "inventory")
	require.NoError(t, err)
	d, err := inventory.CreateOne(context.Background(), data.Document{"name": "test item", "quantity": 10})
	require.NoError(t, err)

	return p, d.Data, func() {}
}

func TestCartSimulation_Success(t *testing.T) {
	p, d, cleanup := setupCartTest(t)
	defer cleanup()

	var sale data.Document
	var payment data.Document
	// Simulate a successful cart checkout
	_, err := p.Transact(context.Background(), func(ctx context.Context, tx base.BasePersistence) (any, error) {
		inventory, err := tx.Collection(ctx, "inventory")
		require.NoError(t, err)

		sales, err := tx.Collection(ctx, "sales")
		require.NoError(t, err)

		payments, err := tx.Collection(ctx, "payments")
		require.NoError(t, err)

		// 1. Check inventory
		q := query.NewQueryBuilder().Where("id").Eq(d.Must().GetString("id")).Build()
		readResult, err := inventory.Read(ctx, &q)
		require.NoError(t, err)
		        require.Equal(t, 1, readResult.Count)

				item := readResult.Data[0]
		quantity := item["quantity"].(int)
		require.GreaterOrEqual(t, quantity, 1)

		// 2. Decrement inventory
		item["quantity"] = quantity - 1
		update := base.CollectionUpdate{
			Filter: q.Filters,
			Set:   item,
		}
		updatedCount, err := inventory.Update(ctx, &update)
		require.NoError(t, err)
		require.Equal(t, 1, updatedCount)

		// 3. Create sales record
		s, err := sales.CreateOne(ctx, data.Document{"itemId": "item1", "quantity": 1})
		require.NoError(t, err)
		sale = s.Data

		// 4. Create payment record
		p, err := payments.CreateOne(ctx, data.Document{"saleId": "sale1", "amount": 100})
		require.NoError(t, err)
		payment = p.Data

		return nil, nil
	})

	require.NoError(t, err)

	// Verify the changes
	inventory, err := p.Collection(context.Background(), "inventory")
	require.NoError(t, err)
	sales, err := p.Collection(context.Background(), "sales")
	require.NoError(t, err)
	payments, err := p.Collection(context.Background(), "payments")
	require.NoError(t, err)

	// Check inventory
	q := query.NewQueryBuilder().Where("id").Eq(d.Must().GetString("id")).Build()
	readResult, err := inventory.Read(context.Background(), &q)
	require.NoError(t, err)
	assert.Equal(t, int(9), readResult.Data[0]["quantity"])

	// Check sales record
	q = query.NewQueryBuilder().Where("id").Eq(sale.Must().GetString("id")).Build()
	readResult, err = sales.Read(context.Background(), &q)
	require.NoError(t, err)
	assert.Equal(t, 1, readResult.Count)

	// Check payment record
	q = query.NewQueryBuilder().Where("id").Eq(payment.Must().GetString("id")).Build()
	readResult, err = payments.Read(context.Background(), &q)
	require.NoError(t, err)
	assert.Equal(t, 1, readResult.Count)
}

func TestCartSimulation_InsufficientInventory(t *testing.T) {
	p, d, cleanup := setupCartTest(t)
	defer cleanup()

	// Simulate a failed cart checkout due to insufficient inventory
	_, err := p.Transact(context.Background(), func(ctx context.Context, tx base.BasePersistence) (any, error) {
		inventory, err := tx.Collection(ctx, "inventory")
		require.NoError(t, err)

		// Attempt to buy 20 items when only 10 are in stock
		q := query.NewQueryBuilder().Where("id").Eq(d.Must().GetString("id")).Build()
		readResult, err := inventory.Read(ctx, &q)
		require.NoError(t, err)
		item := readResult.Data[0]
		quantity := item["quantity"].(int)

		if quantity < 20 {
			return nil, errors.New("insufficient inventory")
		}

		// This part should not be reached
		return nil, nil
	})

	require.Error(t, err)
	assert.Equal(t, "insufficient inventory", err.Error())

	// Verify that no changes were made
	inventory, err := p.Collection(context.Background(), "inventory")
	require.NoError(t, err)
	sales, err := p.Collection(context.Background(), "sales")
	require.NoError(t, err)
	payments, err := p.Collection(context.Background(), "payments")
	require.NoError(t, err)

	// Check inventory is unchanged
	q := query.NewQueryBuilder().Where("id").Eq(d.Must().GetString("id")).Build()
	readResult, err := inventory.Read(context.Background(), &q)
	require.NoError(t, err)

	item := readResult.Data[0]
	assert.Equal(t, int(10), item["quantity"])

	// Check no sales record was created
	readResult, err = sales.Read(context.Background(), &query.Query{})
	require.NoError(t, err)
	assert.Equal(t, 0, readResult.Count)

	// Check no payment record was created
	readResult, err = payments.Read(context.Background(), &query.Query{})
	require.NoError(t, err)
	assert.Equal(t, 0, readResult.Count)
}

func TestCartSimulation_PaymentFailure(t *testing.T) {
	p, d, cleanup := setupCartTest(t)
	defer cleanup()

	// Simulate a failed cart checkout due to payment failure
	_, err := p.Transact(context.Background(), func(ctx context.Context, tx base.BasePersistence) (any, error) {
		inventory, err := tx.Collection(ctx, "inventory")
		require.NoError(t, err)

		sales, err := tx.Collection(ctx, "sales")
		require.NoError(t, err)

		// 1. Decrement inventory
		q := query.NewQueryBuilder().Where("id").Eq(d.Must().GetString("id")).Build()
		readResult, err := inventory.Read(ctx, &q)
		require.NoError(t, err)
		item := readResult.Data[0]
		quantity := item["quantity"].(int)
		item["quantity"] = quantity - 1
		update := base.CollectionUpdate{
			Filter: q.Filters,
			Set:   item,
		}
		_, err = inventory.Update(ctx, &update)
		require.NoError(t, err)

		// 2. Create sales record
		_, err = sales.CreateOne(ctx, data.Document{"id": "sale1", "itemId": "item1", "quantity": 1})
		require.NoError(t, err)

		// 3. Simulate payment failure
		return nil, errors.New("payment failed")
	})

	require.Error(t, err)
	assert.Equal(t, "payment failed", err.Error())

	// Verify that no changes were made
	inventory, err := p.Collection(context.Background(), "inventory")
	require.NoError(t, err)
	sales, err := p.Collection(context.Background(), "sales")
	require.NoError(t, err)
	payments, err := p.Collection(context.Background(), "payments")
	require.NoError(t, err)

	// Check inventory is unchanged
	q := query.NewQueryBuilder().Where("id").Eq(d.Must().GetString("id")).Build()

	readResult, err := inventory.Read(context.Background(), &q)
	require.NoError(t, err)
	item := readResult.Data[0]
	assert.Equal(t, int(10), item["quantity"])

	// Check no sales record was created
	readResult, err = sales.Read(context.Background(), &query.Query{})
	require.NoError(t, err)
	assert.Equal(t, 0, readResult.Count)

	// Check no payment record was created
	readResult, err = payments.Read(context.Background(), &query.Query{})
	require.NoError(t, err)
	assert.Equal(t, 0, readResult.Count)
}
