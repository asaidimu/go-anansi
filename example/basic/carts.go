package main

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
)

const CartsCollectionName = "Carts"

type Cart struct {
	ID      string   `doc:"id,omitempty"`
	UserID  string   `doc:"user_id"`
	ProductIDs []string `doc:"product_ids"`
	Quantity int      `doc:"quantity"`
}

type Carts struct {
	base.ModelCollection[Cart]
}

func (cs *Carts) CreateCart(ctx context.Context, cart Cart) (Cart, error) {
	return cs.Create(ctx, cart)
}

func (cs *Carts) CreateCarts(ctx context.Context, carts []Cart) ([]Cart, error) {
	results, err := cs.ModelCollection.CreateMany(ctx, carts)
	return results, err
}

func (cs *Carts) GetCart(ctx context.Context, id string) (Cart, error) {
	return cs.FindByID(ctx, id)
}

func (cs *Carts) FindCarts(ctx context.Context, q *query.Query) ([]Cart, error) {
	return cs.Read(ctx, q)
}

func (cs *Carts) ListAllCarts(ctx context.Context) ([]Cart, error) {
	q := query.NewQueryBuilder().Build()
	return cs.FindCarts(ctx, &q)
}

func (cs *Carts) UpdateCart(ctx context.Context, id string, updates Cart) (Cart, error) {
	return cs.Update(ctx, id, updates)
}

func (cs *Carts) UpdateCarts(ctx context.Context, filter *query.QueryFilter, updates Cart) (int, error) {
	return cs.UpdateMany(ctx, filter, updates)
}

func (cs *Carts) DeleteCart(ctx context.Context, id string) error {
	return cs.DeleteByID(ctx, id)
}

func (cs *Carts) DeleteCarts(ctx context.Context, filter *query.QueryFilter, unsafe bool) (int, error) {
	return cs.DeleteMany(ctx, filter, unsafe)
}

func (cs *Carts) ValidateCart(ctx context.Context, cart Cart, loose bool) error {
	return cs.Validate(ctx, cart, loose)
}
