package data

import (
	"errors"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// GetNested with enhanced path parsing and error handling.
func (d Document) GetNested(path string) (any, error) {
	if path == "" {
		return nil, common.SystemErrorFrom(ErrKeyEmpty).WithOperation("data.Document.GetNested").WithPath(path)
	}

	val, ok := utils.GetValueByPath(d.data, path)
	if !ok {
		return nil, common.SystemErrorFrom(ErrPathSegmentNotFound).WithOperation("data.Document.GetNested").WithPath(path)
	}
	return val, nil
}

// SetNested with path validation and intermediate map creation.
func (d *Document) SetNested(path string, value any) error {
	if path == DocumentIDField {
		return common.SystemErrorFrom(ErrReadOnlyField).WithOperation("data.Document.SetNested").WithPath(path).WithMessage(fmt.Sprintf("field '%s' is managed by the library and cannot be set manually", DocumentIDField))
	}
	if path == "" {
		return common.SystemErrorFrom(ErrKeyEmpty).WithOperation("data.Document.SetNested").WithPath(path)
	}

	parts := strings.Split(path, ".")
	current := d.data

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return nil
		}

		next, ok := current[part]
		if !ok {
			next = make(map[string]any)
			current[part] = next
		}

		if nextMap, ok := next.(map[string]any); ok {
			current = nextMap
		} else if nextDoc, ok := next.(Document); ok {
			current = nextDoc.data
		} else {
			return common.SystemErrorFrom(ErrCannotTraverse).WithOperation("data.Document.SetNested").WithPath(strings.Join(parts[:i+1], ".")).WithMessage(fmt.Sprintf("cannot traverse into %T", next)).WithCause(errors.Join(ErrCannotTraverse, ErrInvalidPath))
		}
	}

	return nil
}

// Delete removes a value at a nested path.
func (d *Document) Delete(path string) error {
	if path == "" {
		return common.SystemErrorFrom(ErrKeyEmpty).WithOperation("data.Document.DeleteNested").WithPath(path)
	}

	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		delete(d.data, parts[0])
		return nil
	}

	parentPath := strings.Join(parts[:len(parts)-1], ".")
	keyToDelete := parts[len(parts)-1]

	parent, ok := utils.GetValueByPath(d.data, parentPath)
	if !ok {
		return common.SystemErrorFrom(ErrPathSegmentNotFound).WithOperation("data.Document.DeleteNested").WithPath(parentPath).WithMessage("parent path not found")
	}

	switch p := parent.(type) {
	case Document:
		delete(p.data, keyToDelete)
	case map[string]any:
		delete(p, keyToDelete)
	default:
		return common.SystemErrorFrom(ErrParentNotMap).WithOperation("data.Document.DeleteNested").WithPath(path).WithMessage(fmt.Sprintf("parent is not a map: %T", p)).WithCause(errors.Join(ErrParentNotMap, ErrInvalidPath))
	}

	return nil
}
