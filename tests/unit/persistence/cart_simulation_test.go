package persistence_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/ephemeral"
	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/persistence/persistence"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)


func newCartTestSchema(name string) *definition.Schema {
	return &definition.Schema{
		Version: common.MustNewVersion("1.0.0"),
		BaseSchema: definition.BaseSchema{
			Name: name,
			Fields: map[definition.FieldId]definition.Field{
				"name":     {Name: "name", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"quantity": {Name: "quantity", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"amount":   {Name: "amount", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeInteger}},
				"itemId":   {Name: "itemId", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
				"saleId":   {Name: "saleId", FieldProperties: definition.FieldProperties{Type: definition.FieldTypeString}},
			},
		},
	}
}

func setupCartTest(t *testing.T) (base.Persistence, *data.Document, func()) {
	interactor := ephemeral.NewEphemeral()

	logger := zap.NewNop()
	// require.NoError(t, err)

	p, err := persistence.NewPersistence(interactor, nil, logger, nil)
	require.NoError(t, err)

	// Create collections
	schemas := []*definition.Schema{
		newCartTestSchema("inventory"),
		newCartTestSchema("sales"),
		newCartTestSchema("payments"),
	}

	for _, s := range schemas {
		_, err := schema.ValidateSchema(s)
		require.NoError(t, err)

		_, err = p.CreateCollection(context.Background(), s)

		if err != nil {
			var sysErr *common.SystemError
			if errors.As(err, &sysErr) {
				t.Logf("Error creating inventory document: %s\n cause:%v", sysErr.Message, sysErr.Cause)
			} else {
				t.Logf("Error creating inventory document: %v", err)
			}

		}
		require.NoError(t, err)
	}

	// Seed inventory
	inventory, err := p.Collection(context.Background(), "inventory")
	require.NoError(t, err)
	doc := data.MustNewDocument(map[string]any{"name": "test item", "quantity": 10})
	sc, err := p.Schema(context.Background(), "inventory")
	require.NoError(t, err)
	if err != nil {
		t.Logf("Error creating inventory document: %v", err)
	}
	d, err := inventory.CreateOne(context.Background(), doc)
	if err != nil {
		if d.Issues != nil {
			scJSON, _ := json.Marshal(sc)
			t.Logf("Error creating inventory document: %v \n with schema %s", d.Issues, string(scJSON))
		}
		t.Logf("Error creating inventory document: %v", err)
	}
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
		q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(d.ID()).Build()
		readResult, err := inventory.Read(ctx, &q)
		require.NoError(t, err)
		require.Equal(t, 1, readResult.Count)

		item := readResult.Data[0]
		quantity := item.Must().GetInt("quantity")
		require.GreaterOrEqual(t, quantity, 1)

		// 2. Decrement inventory
		item.Set("quantity", quantity-1)
		update := base.CollectionUpdate{
			Filter: q.Filters,
			Set:    item,
		}
		updateResult, err := inventory.Update(ctx, &update)
		require.NoError(t, err)
		require.NotNil(t, updateResult)
		require.Equal(t, 1, updateResult.Count)

		// 3. Create sales record
		s, err := sales.CreateOne(ctx, data.MustNewDocument(map[string]any{"itemId": "item1", "quantity": 1}))
		if err != nil {
			t.Logf("Error creating sales record: %v", err)
		}
		require.NoError(t, err)
		sale = *s.Data

		// 4. Create payment record
		p, err := payments.CreateOne(ctx, data.MustNewDocument(map[string]any{"saleId": "sale1", "amount": 100}))
		if err != nil {
			t.Logf("Error creating payment record: %v", err)
		}
		require.NoError(t, err)
		payment = *p.Data

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
	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(d.ID()).Build()
	readResult, err := inventory.Read(context.Background(), &q)
	require.NoError(t, err)
	assert.Equal(t, int(9), readResult.Data[0].Must().GetInt("quantity"))

	// Check sales record
	q = query.NewQueryBuilder().Where(data.DocumentIDField).Eq(sale.ID()).Build()
	readResult, err = sales.Read(context.Background(), &q)
	require.NoError(t, err)
	assert.Equal(t, 1, readResult.Count)

	// Check payment record
	q = query.NewQueryBuilder().Where(data.DocumentIDField).Eq(payment.ID()).Build()
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
		q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(d.ID()).Build()
		readResult, err := inventory.Read(ctx, &q)
		require.NoError(t, err)
		item := readResult.Data[0]
		quantity := item.Must().GetInt("quantity")

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
	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(d.ID()).Build()
	readResult, err := inventory.Read(context.Background(), &q)
	require.NoError(t, err)

	item := readResult.Data[0]
	assert.Equal(t, int(10), item.Must().GetInt("quantity"))

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
		q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(d.ID()).Build()
		readResult, err := inventory.Read(ctx, &q)
		require.NoError(t, err)
		item := readResult.Data[0]
		quantity := item.Must().GetInt("quantity")
		require.GreaterOrEqual(t, quantity, 1)

		// 2. Decrement inventory
		item.Set("quantity", quantity-1)
		update := base.CollectionUpdate{
			Filter: q.Filters,
			Set:    item,
		}
		_, err = inventory.Update(ctx, &update)
		require.NoError(t, err)

		// 2. Create sales record
		_, err = sales.CreateOne(ctx, data.MustNewDocument(map[string]any{data.DocumentIDField: "sale1", "itemId": "item1", "quantity": 1}))
		if err != nil {
			t.Logf("Error creating sales record in payment failure test: %v", err)
		}
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
	q := query.NewQueryBuilder().Where(data.DocumentIDField).Eq(d.ID()).Build()

	readResult, err := inventory.Read(context.Background(), &q)
	require.NoError(t, err)
	item := readResult.Data[0]
	assert.Equal(t, int(10), item.Must().GetInt("quantity"))

	// Check no sales record was created
	readResult, err = sales.Read(context.Background(), &query.Query{})
	require.NoError(t, err)
	assert.Equal(t, 0, readResult.Count)

	// Check no payment record was created
	readResult, err = payments.Read(context.Background(), &query.Query{})
	require.NoError(t, err)
	assert.Equal(t, 0, readResult.Count)
}
