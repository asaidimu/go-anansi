package user

import (
	"context"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/example/api/model"
	"go.uber.org/zap"
)

var (
	modelSchema *schema.SchemaDefinition

	// userModelInstance is the single instance of our UserModel.
	userModelInstance *UserModel

	// once ensures that the initialization logic runs only once.
	once sync.Once

	// initError stores any error that occurred during the initialization.
	initError error
)

const ModelName = "user"

// UserModel represents the user collection and its operations.
type UserModel struct {
	base.Collection
	Name   string
	logger *zap.Logger
}

var _ base.Collection = (*UserModel)(nil)

// GetUserModel is the function to retrieve the singleton instance of UserModel.
// It will initialize the instance the first time it's called and return
// the same instance on subsequent calls. It is thread-safe.
func GetUserModel(ctx context.Context, p base.Persistence, logger *zap.Logger) (*UserModel, error) {
	// once.Do ensures the function literal is executed exactly once.
	once.Do(func() {
		// This is the initialization logic, which is run only once.
		var schemaDef *schema.SchemaDefinition
		schemaDef, initError = model.GetSchema(ModelName)
		if initError != nil {
			return // Stop here if we can't get the schema.
		}

		var collection base.Collection
		collection, initError = p.Collection(ctx, schemaDef.Name)
		if initError != nil {
			return // Stop here if we can't get the collection.
		}

		// If initialization was successful, create the instance.
		userModelInstance = &UserModel{
			Collection: collection,
			Name:       schemaDef.Name,
			logger:     logger,
		}
	})

	// Return the singleton instance and any error that occurred during initialization.
	return userModelInstance, initError
}

func (m *UserModel) CreateOne(ctx context.Context, doc data.Document) (base.CreateResult, error) {
	m.logger.Debug("Creating a user")
	return m.Collection.CreateOne(ctx, doc)
}

func (m *UserModel) CreateAdminUser() error {
	ctx := context.Background()

	adminUser := data.MustNewDocument(map[string]any{
		"id":       "admin123",
		"username": "admin",
		"password": "hashed_admin_password", // In a real app, hash this
		"role":     "admin",
	})

	_, err := m.CreateOne(ctx, adminUser)
	if err != nil {
		return err
	}
	return nil
}
