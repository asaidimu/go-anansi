package main

import (
	"context"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	coreutils "github.com/asaidimu/go-anansi/v6/core/utils"
	"go.uber.org/zap"
)

type App struct {
	p       base.Persistence
	Logger  *zap.Logger
	cleanup func()
}

func getProductSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:    "Product",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Name: "name", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
			"price": {Name: "price", Type: "number", Required: coreutils.BoolPtr(true)},
			"stock": {Name: "stock", Type: "integer", Required: coreutils.BoolPtr(true)},
		},
	}
}

func NewApp() *App {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	return &App{
		Logger: logger,
	}
}

func (app *App) Init() (func(), error) {
	productSchema := getProductSchema() // we could load all schemas from maybe a file since schemas can be written in json

	p, cleanup, err := anansi.Playground(anansi.PlaygroundConfig{
		DBPath:        "anansi.db",
		EnableLogging: false,
		EnableEvents:  true,
		Schemas:       []schema.SchemaDefinition{*productSchema},
	})

	if err != nil {
		return nil, err
	}

	app.cleanup = cleanup
	app.p = p
	return app.cleanup, nil
}

func (app *App) ProductsModel() (*Products, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	productSchema := getProductSchema() // we could look up the name in the registry
	productsCollection, err := app.p.Collection(ctx, productSchema.Name)
	if err != nil {
		return nil, err
	}
	products := NewProductsCollection(productsCollection)
	return products, nil
}
