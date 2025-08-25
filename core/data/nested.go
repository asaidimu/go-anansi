package data

import (
	"errors"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/utils"
)

// GetNested with enhanced path parsing and error handling.
func (d Document) GetNested(path string) (any, error) {
	if path == "" {
		return nil, &DocumentError{
			Operation: "GetNested",
			Key:       path,
			Message:   ErrKeyEmpty.Error(),
			Cause:     ErrKeyEmpty,
		}
	}

	val, ok := utils.GetValueByPath(d, path)
	if !ok {
		return nil, &DocumentError{
			Operation: "GetNested",
			Key:       path,
			Message:   ErrPathSegmentNotFound.Error(),
			Cause:     ErrPathSegmentNotFound,
		}
	}
	return val, nil
}

// SetNested with path validation and intermediate map creation.
func (d Document) SetNested(path string, value any) error {
	if path == "" {
		return &DocumentError{
			Operation: "SetNested",
			Key:       path,
			Message:   ErrKeyEmpty.Error(),
			Cause:     ErrKeyEmpty,
		}
	}

	parts := strings.Split(path, ".")
	current := d

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
			current = Document(nextMap)
		} else if nextDoc, ok := next.(Document); ok {
			current = nextDoc
		} else {
			return &DocumentError{
				Operation: "SetNested",
				Key:       strings.Join(parts[:i+1], "."),
				Message:   fmt.Sprintf("%s: cannot traverse into %T", ErrCannotTraverse.Error(), next),
				Cause:     errors.Join(ErrCannotTraverse, ErrInvalidPath),
			}
		}
	}

	return nil
}

// DeleteNested removes a value at a nested path.
func (d Document) DeleteNested(path string) error {
	if path == "" {
		return &DocumentError{
			Operation: "DeleteNested",
			Key:       path,
			Message:   ErrKeyEmpty.Error(),
			Cause:     ErrKeyEmpty,
		}
	}

	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		delete(d, parts[0])
		return nil
	}

	parentPath := strings.Join(parts[:len(parts)-1], ".")
	keyToDelete := parts[len(parts)-1]

	parent, ok := utils.GetValueByPath(d, parentPath)
	if !ok {
		return &DocumentError{
			Operation: "DeleteNested",
			Key:       parentPath,
			Message:   "parent path not found",
			Cause:     ErrPathSegmentNotFound,
		}
	}

	switch p := parent.(type) {
	case Document:
		delete(p, keyToDelete)
	case map[string]any:
		delete(p, keyToDelete)
	default:
		return &DocumentError{
			Operation: "DeleteNested",
			Key:       path,
			Message:   fmt.Sprintf("%s: parent is not a map: %T", ErrParentNotMap.Error(), p),
			Cause:     errors.Join(ErrParentNotMap, ErrInvalidPath),
		}
	}

	return nil
}
