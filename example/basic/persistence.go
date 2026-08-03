package main

import (
	"context"
	"log"
	"sync"

	"github.com/asaidimu/go-anansi/v8"
	"github.com/asaidimu/go-anansi/v8/core/cache"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/collection"
	appschema "github.com/asaidimu/go-anansi/v8/example/basic/schema"
	"go.uber.org/zap"
)

type App struct {
	p       base.Persistence
	Logger  *zap.Logger
	cleanup func()

	// Models
	models map[string]any
	mu     sync.Mutex
}

func NewApp() *App {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	return &App{
		Logger: logger,
		models: make(map[string]any),
	}
}

func (app *App) Init() (func(), error) {
	schemas, err := appschema.GetSchemas()
	if err != nil {
		return nil, err
	}

	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		EnableLogging: false,
		EnableEvents:  true,
		Schemas:       schemas,
	})

	if err != nil {
		return nil, err
	}

	app.cleanup = cleanup
	app.p = p
	return app.cleanup, nil
}

// UseModel is a generic function to get or create a model singleton.
// It uses a factory function to construct the model if it doesn't exist.
func UseModel[T any](app *App, name string, factory func(base.Collection) (*T, error)) (*T, error) {
	app.mu.Lock()
	defer app.mu.Unlock()

	// If the model already exists in our cache, return it.
	if model, ok := app.models[name]; ok {
		return model.(*T), nil
	}

	// Get the underlying collection from the persistence layer.
	collection, err := app.p.Collection(context.Background(), name)
	if err != nil {
		return nil, err
	}

	// Use the factory to create a new instance of the model.
	model, err := factory(collection)
	if err != nil {
		return nil, err
	}
	app.models[name] = model
	return model, nil
}

// ProductsModel returns a singleton instance of the Products model.
func (app *App) ProductsModel() (*Products, error) {
	return UseModel(app, ProductsCollectionName, func(raw base.Collection) (*Products, error) {
		mc, err := collection.NewModelCollection[*Product](raw, app.Logger)
		if err != nil {
			return nil, err
		}
		return &Products{ModelCollection: mc}, nil
	})
}

// UsersModel returns a singleton instance of the Users model.
func (app *App) UsersModel() (*Users, error) {
	return UseModel(app, UsersCollectionName, func(raw base.Collection) (*Users, error) {
		// we can wrap this in custom functionality here
		mc, err := collection.NewModelCollection[*User](raw, app.Logger,
			collection.ModelCollectionOptions[*User]{
				CacheConfig: &cache.CacheConfig{MaxEntries: 100},
				AutoLoad:    true,
			},
		)
		if err != nil {
			return nil, err
		}
		return &Users{ModelCollection: mc}, nil
	})
}

// CartsModel returns a singleton instance of the Carts model.
func (app *App) CartsModel() (*Carts, error) {
	return UseModel(app, CartsCollectionName, func(raw base.Collection) (*Carts, error) {
		mc, err := collection.NewModelCollection[*Cart](raw, app.Logger)
		if err != nil {
			return nil, err
		}
		return &Carts{ModelCollection: mc}, nil
	})
}
