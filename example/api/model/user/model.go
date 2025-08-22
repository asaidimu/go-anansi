package user

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/example/api/model"
)

var (
	modelSchema *schema.SchemaDefinition
)


type Model struct {
	collection base.Collection
	name string
}

func NewUserModel(ctx context.Context, p base.Persistence) (*Model, error) {
	model := &Model{}
	schema, err := model.Schema()
	if err != nil {
		return nil, err
	}

	model.name = schema.Name
	collection, err := p.Collection(ctx, schema.Name)

	if err != nil {
		// ideally we should check if error is `not exist` in which case we create
		// the collection
		return nil, err
	}

	model.collection = collection
	return  model, nil
}

func (m *Model) Schema() (*schema.SchemaDefinition, error) {
	if modelSchema != nil {
		return modelSchema, nil
	}

	modelSchema, err := model.GetSchema("user")
	if err != nil {
		return nil, err
	}

	return modelSchema, nil
}

func (m *Model) CreateAdminUser() (error) {
	ctx := context.Background()

	adminUser := data.MustNewDocument(map[string]any{
		"id":           "admin123",
		"username":     "admin",
		"passwordHash": "hashed_admin_password", // In real app, hash this
		"role":         "admin",
	})

	_, err := m.collection.CreateOne(ctx, adminUser)
	if err != nil {
		return err
	}
	return nil
}
