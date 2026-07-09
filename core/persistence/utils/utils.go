// Package utils provides utility functions for the persistence layer.
package utils

import (
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
)


// DecoratorFunc is a generic type for a function that decorates an object of type T.
type DecoratorFunc[T any] func(T) T

type CollectionDecorator DecoratorFunc[base.Collection]
type PersistenceDecorator DecoratorFunc[base.Persistence]

type Decorators struct {
	PersistenceDecorators []DecoratorFunc[base.Persistence]
	CollectionDecorators  []DecoratorFunc[base.Collection]
}

// applyDecorators takes an object and a slice of decorators,
// and applies each decorator to the object sequentially.
func ApplyDecorators[T any](baseObject T, decorators []DecoratorFunc[T]) T {
	if decorators == nil {
		return baseObject
	}
	decoratedObject := baseObject
	for _, decorator := range decorators {
		decoratedObject = decorator(decoratedObject)
	}
	return decoratedObject
}
