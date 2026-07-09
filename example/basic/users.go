package main

import (
	"context"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/query"
)

const UsersCollectionName = "Users"

type User struct {
	data.DocumentModel
	Username string `doc:"username"`
	Email    string `doc:"email"`
}

type Users struct {
	base.ModelCollection[User, *User]
}

func (us *Users) CreateUser(ctx context.Context, user User) (User, error) {
	return us.Create(ctx, user)
}

func (us *Users) CreateUsers(ctx context.Context, users []User) ([]User, error) {
	results, err := us.ModelCollection.CreateMany(ctx, users)
	return results, err
}

func (us *Users) GetUser(ctx context.Context, id string) (User, error) {
	return us.FindByID(ctx, id)
}

func (us *Users) FindUsers(ctx context.Context, q *query.Query) ([]User, error) {
	return us.Read(ctx, q)
}

func (us *Users) ListAllUsers(ctx context.Context) ([]User, error) {
	q := query.NewQueryBuilder().Build()
	return us.FindUsers(ctx, &q)
}

func (us *Users) UpdateUser(ctx context.Context, id string, updates User) (User, error) {
	return us.Update(ctx, id, updates)
}

func (us *Users) UpdateUsers(ctx context.Context, filter *query.QueryFilter, updates User) (int, error) {
	return us.UpdateMany(ctx, filter, updates)
}

func (us *Users) DeleteUser(ctx context.Context, id string) error {
	return us.DeleteByID(ctx, id)
}

func (us *Users) DeleteUsers(ctx context.Context, filter *query.QueryFilter, unsafe bool) (int, error) {
	return us.DeleteMany(ctx, filter, unsafe)
}

func (us *Users) ValidateUser(ctx context.Context, user User, loose bool) error {
	return us.Validate(ctx, user, loose)
}
