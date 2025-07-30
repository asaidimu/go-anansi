package nodes

import (
	"fmt"
	"reflect"

	"github.com/asaidimu/go-anansi/v6/core/schema"
	"github.com/asaidimu/go-anansi/v6/core/utils"
	"github.com/asaidimu/go-anansi/v6/core/validator/types"
)

// BaseNode provides common fields for all nodes.
type BaseNode[T any] struct {
	id               string
	path             string
	deps             []string
	fmap      schema.FunctionMap
	fieldType        *schema.FieldType
	validatorFactory types.ValidatorFactory
}

func (n *BaseNode[T]) GetID() string             { return n.id }
func (n *BaseNode[T]) GetPath() string           { return n.path }
func (n *BaseNode[T]) GetDependencies() []string { return n.deps }
func (n *BaseNode[T]) GetBaseNode() *BaseNode[T] { return n }

func NewBaseNode[T any](path, nodeType, suffix string, fieldType *schema.FieldType, deps []string, factory types.ValidatorFactory, fmap schema.FunctionMap) BaseNode[T] {
	id := fmt.Sprintf("%s:%s", path, nodeType)
	if path == "" {
		id = nodeType
	}
	return BaseNode[T]{
		id:               id,
		path:             path,
		fieldType:        fieldType,
		validatorFactory: factory,
		fmap: fmap,
	}
}

func (n *BaseNode[T]) execute(ctx *types.ValidationContext, useReflect bool, callback func(data T, dataType reflect.Type) *types.NodeResult) *types.NodeResult {
	doc, ok := ctx.RootData.(schema.Document)
	if !ok {
		return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "TYPE_MISMATCH", Message: "Expected root node to be object", Path: ""}}}
	}

	path := n.path

	if ctx.Path != nil {
		path = *(ctx.Path)
	}

	value, exists := doc.GetFieldValue(path)

	if !exists {
		return &types.NodeResult{Success: true} // Not present, handled by required check.
	}

	if useReflect {
		data := value.(T)
		dataType := reflect.TypeOf(value)
		return callback(data, dataType)
	} else {
		data, ok := value.(T)
		if !ok {
			return &types.NodeResult{Success: false, Issues: []types.Issue{{Code: "TYPE_MISMATCH", Message: fmt.Sprintf("Unexpected type for %s", *(n.fieldType)), Path: n.path}}}
		}
		return callback(data, nil)
	}
}

func (n *BaseNode[T]) buildPath(fieldName string) string {
	return utils.BuildPath(n.path, fieldName)
}

func (n *BaseNode[T]) getScopedPath() string {
	return utils.GetScopedPath(n.path)
}
