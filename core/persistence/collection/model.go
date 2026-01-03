package collection

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
)

type modelCollection[T any] struct {
	raw base.Collection
}

// NewModelCollection creates a type-safe wrapper around a raw collection.
func NewModelCollection[T any](raw base.Collection) base.ModelCollection[T] {
	return &modelCollection[T]{raw: raw}
}

func (mc *modelCollection[T]) Create(ctx context.Context, doc T) (T, error) {
	var out T
	d, err := data.FromStructWithTags(doc)
	if err != nil {
		return out, err
	}
	res, err := mc.raw.CreateOne(ctx, d)
	if err != nil {
		return out, err
	}
	err = res.Data.Bind().To(&out)
	return out, err
}

func (mc *modelCollection[T]) CreateMany(ctx context.Context, docs []T) ([]T, error) {
	input := make([]*data.Document, len(docs))
	for i, d := range docs {
		converted, err := data.FromStructWithTags(d)
		if err != nil {
			return nil, err
		}
		input[i] = converted
	}
	results, err := mc.raw.CreateMany(ctx, input)
	if err != nil {
		return nil, err
	}
	out := make([]T, len(results))
	for i, r := range results {
		if err := r.Data.Bind().To(&out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (mc *modelCollection[T]) FindByID(ctx context.Context, id string) (T, error) {
	q := query.NewQueryBuilder().Where("id").Eq(id).Limit(1).Build()
	res, err := mc.Read(ctx, &q)
	if err != nil {
		return *new(T), err
	}
	if len(res) == 0 {
		return *new(T), fmt.Errorf("record %s not found", id)
	}
	return res[0], nil
}

func (mc *modelCollection[T]) Read(ctx context.Context, q *query.Query) ([]T, error) {
	res, err := mc.raw.Read(ctx, q)
	if err != nil {
		return nil, err
	}

	docs := res.Data

	out := make([]T, len(docs))
	for i, d := range docs {
		if err := d.Bind().To(&out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (mc *modelCollection[T]) Update(ctx context.Context, id string, update T) (T, error) {
	var out T
	upd, err := data.FromStructWithTags(update, true) // partial update
	if err != nil {
		return out, err
	}
	filter := query.NewQueryBuilder().Where("id").Eq(id).Build().Filters
	_, err = mc.raw.Update(ctx, &base.CollectionUpdate{Filter: filter, Set: upd})
	if err != nil {
		return out, err
	}
	return mc.FindByID(ctx, id)
}

func (mc *modelCollection[T]) UpdateMany(ctx context.Context, f *query.QueryFilter, update T) (int, error) {
	upd, err := data.FromStructWithTags(update, true)
	if err != nil {
		return 0, err
	}
	return mc.raw.Update(ctx, &base.CollectionUpdate{Filter: f, Set: upd})
}

func (mc *modelCollection[T]) DeleteByID(ctx context.Context, id string) error {
	filter := query.NewQueryBuilder().Where("id").Eq(id).Build().Filters
	count, err := mc.raw.Delete(ctx, filter, false)
	if err != nil {
		return err
	}
	if count == 0 {
		return common.SystemErrorFrom(fmt.Errorf("record %s not found", id))
	}
	return nil
}

func (mc *modelCollection[T]) DeleteMany(ctx context.Context, f *query.QueryFilter, unsafe bool) (int, error) {
	return mc.raw.Delete(ctx, f, unsafe)
}

func (mc *modelCollection[T]) Validate(ctx context.Context, doc T, loose bool) error {
	d, err := data.FromStructWithTags(doc)
	if err != nil {
		return err
	}
	_, err = mc.raw.Validate(ctx, d, loose)
	return err
}


func (mc *modelCollection[T]) Subscribe(ctx context.Context, opt base.SubscriptionOptions) string {
	return mc.raw.Subscribe(ctx, opt)
}

func (mc *modelCollection[T]) Unsubscribe(ctx context.Context, id string) {
	mc.raw.Unsubscribe(ctx, id)
}
