package collection

import (
	"context"
	"sync"

	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

// registrySchemaProvider resolves the active schema and validator from a
// CollectionRegistry for a named collection.
type registrySchemaProvider struct {
	registry base.CollectionRegistry
	name     string
}

func (p *registrySchemaProvider) CurrentSchema(ctx context.Context) (*definition.Schema, error) {
	return p.registry.CurrentSchema(ctx, p.name)
}

func (p *registrySchemaProvider) CurrentValidator(ctx context.Context) (*definition.DocumentValidator, error) {
	return p.registry.CurrentValidator(ctx, p.name)
}

func (p *registrySchemaProvider) PhysicalName(ctx context.Context) (string, error) {
	return p.registry.ResolvePhysicalName(ctx, p.name)
}

// staticSchemaProvider returns a fixed schema and its validator. Used for the
// bootstrap registry collection that must exist before the registry is created.
type staticSchemaProvider struct {
	schema      *definition.Schema
	validator   *definition.DocumentValidator
	physicalOnce sync.Once
	physicalName string
}

func newStaticSchemaProvider(schema *definition.Schema) *staticSchemaProvider {
	validator, err := definition.NewDocumentValidator(schema, nil)
	if err != nil {
		panic("static schema provider: invalid bootstrap schema: " + err.Error())
	}
	return &staticSchemaProvider{
		schema:    schema,
		validator: validator,
	}
}

func (p *staticSchemaProvider) CurrentSchema(_ context.Context) (*definition.Schema, error) {
	return p.schema, nil
}

func (p *staticSchemaProvider) CurrentValidator(_ context.Context) (*definition.DocumentValidator, error) {
	return p.validator, nil
}

func (p *staticSchemaProvider) PhysicalName(_ context.Context) (string, error) {
	p.physicalOnce.Do(func() {
		p.physicalName = p.schema.Name
	})
	return p.physicalName, nil
}

// NewRegistrySchemaProvider creates a SchemaProvider backed by a CollectionRegistry.
// The provider resolves the active schema and validator for the named collection
// on every call, always reflecting the latest registered version.
func NewRegistrySchemaProvider(registry base.CollectionRegistry, name string) base.SchemaProvider {
	return &registrySchemaProvider{registry: registry, name: name}
}

// NewStaticSchemaProvider creates a SchemaProvider that always returns the same
// fixed schema and validator. Used for bootstrapping the registry collection
// before the registry itself exists.
func NewStaticSchemaProvider(schema *definition.Schema) base.SchemaProvider {
	return newStaticSchemaProvider(schema)
}
