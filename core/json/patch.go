// Package patch provides JSON Patch operations according to RFC 6902
// with an additional non-standard 'removeValue' operation.
package json

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/asaidimu/go-anansi/v7/core/common"
)

// PatchOperation represents a single JSON Patch operation
type PatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
	From  string `json:"from,omitempty"`
}

type Patcher struct {
	mu    sync.RWMutex
	cache map[string][]string
}

func NewPatcher() *Patcher {
	return &Patcher{cache: make(map[string][]string)}
}

// CreatePatch creates a sequence of JSON Patch operations that transform one object into another
func (p *Patcher) CreatePatch(oldObj, newObj any) ([]PatchOperation, error) {
	patches := make([]PatchOperation, 0)
	if err := p.generatePatches(oldObj, newObj, "", &patches); err != nil {
		return nil, err
	}
	return patches, nil
}

// generatePatches recursively generates patch operations for nested objects
func (p *Patcher) generatePatches(oldObj, newObj any, path string, patches *[]PatchOperation) error {
	if deepEqual(oldObj, newObj) {
		return nil
	}

	oldType := fmt.Sprintf("%T", oldObj)
	newType := fmt.Sprintf("%T", newObj)

	if oldType != newType {
		*patches = append(*patches, PatchOperation{
			Op:    "replace",
			Path:  path,
			Value: newObj,
		})
		return nil
	}

	switch oldVal := oldObj.(type) {
	case map[string]any:
		newVal, ok := newObj.(map[string]any)
		if !ok {
			*patches = append(*patches, PatchOperation{
				Op:    "replace",
				Path:  path,
				Value: newObj,
			})
			return nil
		}
		return p.handleObjects(oldVal, newVal, path, patches)

	case []any:
		newVal, ok := newObj.([]any)
		if !ok {
			*patches = append(*patches, PatchOperation{
				Op:    "replace",
				Path:  path,
				Value: newObj,
			})
			return nil
		}
		return p.handleArrays(oldVal, newVal, path, patches)

	default:
		if !deepEqual(oldObj, newObj) {
			*patches = append(*patches, PatchOperation{
				Op:    "replace",
				Path:  path,
				Value: newObj,
			})
		}
	}

	return nil
}


// handleArrays generates patch operations for array differences
func (p *Patcher) handleArrays(oldArr, newArr []any, path string, patches *[]PatchOperation) error {
	maxLen := max(len(newArr), len(oldArr))

	for i := range maxLen {
		currentPath := fmt.Sprintf("%s/%d", path, i)

		if i >= len(oldArr) {
			*patches = append(*patches, PatchOperation{
				Op:    "add",
				Path:  fmt.Sprintf("%s/-", path),
				Value: newArr[i],
			})
		} else if i >= len(newArr) {
			*patches = append(*patches, PatchOperation{
				Op:   "remove",
				Path: currentPath,
			})
		} else {
			if err := p.generatePatches(oldArr[i], newArr[i], currentPath, patches); err != nil {
				return err
			}
		}
	}

	return nil
}

// handleObjects generates patch operations for object differences
func (p *Patcher) handleObjects(oldObj, newObj map[string]any, path string, patches *[]PatchOperation) error {
	seen := make(map[string]bool)

	// Check for removed and modified keys
	for key := range oldObj {
		escapedKey := escapeJsonPointer(key)
		currentPath := path + "/" + escapedKey
		if path == "" {
			currentPath = "/" + escapedKey
		}

		if _, exists := newObj[key]; !exists {
			*patches = append(*patches, PatchOperation{
				Op:   "remove",
				Path: currentPath,
			})
		} else {
			if err := p.generatePatches(oldObj[key], newObj[key], currentPath, patches); err != nil {
				return err
			}
			seen[key] = true
		}
	}

	// Check for added keys
	for key := range newObj {
		if !seen[key] {
			escapedKey := escapeJsonPointer(key)
			currentPath := path + "/" + escapedKey
			if path == "" {
				currentPath = "/" + escapedKey
			}
			*patches = append(*patches, PatchOperation{
				Op:    "add",
				Path:  currentPath,
				Value: newObj[key],
			})
		}
	}

	return nil
}

// Apply now performs a deep copy only once and uses improved navigation
func (p *Patcher) Apply(target any, operations []PatchOperation) (any, error) {
	// 1. Initial Deep Copy (Improved)
	result := deepCopy(target)

	for _, op := range operations {
		parts, err := p.getParts(op.Path)
		if err != nil {
			return nil, err
		}

		switch op.Op {
		case "add":
			result, err = p.add(result, parts, op.Value)
		case "remove":
			result, err = p.remove(result, parts)
		case "replace":
			// Replace is logically remove + add
			result, err = p.remove(result, parts)
			if err == nil {
				result, err = p.add(result, parts, op.Value)
			}
		case "move":
			fromParts, _ := p.getParts(op.From)
			val, err := p.getVal(result, fromParts)
			if err != nil { return nil, err }

			result, err = p.remove(result, fromParts)
			if err == nil {
				result, err = p.add(result, parts, val)
			}
		case "test":
			val, _ := p.getVal(result, parts)
			if !reflect.DeepEqual(val, op.Value) {
				return nil, common.NewSystemError("PATCH_TEST_FAILED").WithPath(op.Path)
			}
		case "removeValue":
			result, err = p.removeValue(result, parts, op.Value)
		}

		if err != nil {
			// If err is already a SystemError, augment it, otherwise wrap it.
			if sysErr, ok := err.(*common.SystemError); ok {
				return nil, sysErr.
					WithOperation(op.Op).
					WithPath(op.Path)
			}
			return nil, common.NewSystemError("PATCH_OPERATION_FAILED").
				WithMessagef("operation '%s' failed at path '%s'", op.Op, op.Path).
				WithOperation(op.Op).
				WithPath(op.Path).
				WithCause(err)
		}
	}
	return result, nil
}

// --- Internal Recursive Logic ---

func (p *Patcher) add(node any, parts []string, val any) (any, error) {
	if len(parts) == 0 {
		return val, nil
	}

	key := parts[0]
	if m, ok := node.(map[string]any); ok {
		res, err := p.add(m[key], parts[1:], val)
		if err != nil {
			return nil, err
		}
		m[key] = res
		return m, nil
	}

	if s, ok := node.([]any); ok {
		idx, isEnd, err := parseIndex(key, len(s))
		if err != nil {
			return nil, err
		}

		// Validation: index cannot be greater than current length
		if idx > len(s) {
			return nil, common.NewSystemError("ARRAY_INDEX_OUT_OF_BOUNDS").WithMessagef("index out of bounds: %d > %d", idx, len(s))
		}

		if len(parts) == 1 {
			newS := make([]any, 0, len(s)+1)
			newS = append(newS, s[:idx]...)
			newS = append(newS, val)
			return append(newS, s[idx:]...), nil
		}

		// For nested paths, we must be within existing bounds
		if isEnd || idx >= len(s) {
			return nil, common.NewSystemError("ARRAY_INDEX_OUT_OF_BOUNDS_NESTED").WithMessage("index out of bounds for nested path")
		}
		res, err := p.add(s[idx], parts[1:], val)
		if err != nil {
			return nil, err
		}
		s[idx] = res
		return s, nil
	}

	return nil, common.NewSystemError("CANNOT_ADD_TO_PRIMITIVE").WithMessage("cannot add to primitive type")
}

func (p *Patcher) remove(node any, parts []string) (any, error) {
	if len(parts) == 0 {
		return nil, nil
	}

	key := parts[0]
	if m, ok := node.(map[string]any); ok {
		if _, exists := m[key]; !exists {
			return nil, common.NewSystemError("OBJECT_KEY_NOT_FOUND").WithMessagef("key not found: '%s'", key)
		}
		if len(parts) == 1 {
			delete(m, key)
			return m, nil
		}
		res, err := p.remove(m[key], parts[1:])
		if err != nil {
			return nil, err
		}
		m[key] = res
		return m, nil
	}

	if s, ok := node.([]any); ok {
		idx, _, err := parseIndex(key, len(s))
		if err != nil || idx >= len(s) {
			return nil, common.NewSystemError("ARRAY_INDEX_OUT_OF_BOUNDS").WithMessagef("index out of bounds for '%s'", key)
		}
		if len(parts) == 1 {
			return append(s[:idx], s[idx+1:]...), nil
		}
		res, err := p.remove(s[idx], parts[1:])
		if err != nil {
			return nil, err
		}
		s[idx] = res
		return s, nil
	}
	return nil, common.NewSystemError("PATH_NOT_FOUND").WithMessage("path not found for removal")
}

func (p *Patcher) removeValue(node any, parts []string, targetValue any) (any, error) {
	// Navigate to the target container
	if len(parts) > 0 {
		key := parts[0]
		if m, ok := node.(map[string]any); ok {
			res, err := p.removeValue(m[key], parts[1:], targetValue)
			m[key] = res
			return m, err
		}
		if s, ok := node.([]any); ok {
			idx, _, err := parseIndex(key, len(s))
			if err != nil || idx >= len(s) { return nil, err }
			res, err := p.removeValue(s[idx], parts[1:], targetValue)
			s[idx] = res
			return s, err
		}
	}

	// We are at the container where we want to filter values
	switch v := node.(type) {
	case []any:
		newS := make([]any, 0)
		for _, item := range v {
			if !reflect.DeepEqual(item, targetValue) {
				newS = append(newS, item)
			}
		}
		return newS, nil
	case map[string]any:
		// If it's a map, removeValue acts like a conditional delete
		for k, val := range v {
			if reflect.DeepEqual(val, targetValue) {
				delete(v, k)
			}
		}
		return v, nil
	}
	return node, nil
}

// --- Helpers ---

func (p *Patcher) getVal(node any, parts []string) (any, error) {
	curr := node
	for _, part := range parts {
		if m, ok := curr.(map[string]any); ok {
			curr = m[part]
		} else if s, ok := curr.([]any); ok {
			idx, _, err := parseIndex(part, len(s))
			if err != nil || idx >= len(s) {
				return nil, common.NewSystemError("INVALID_ARRAY_INDEX").WithMessagef("invalid array index: '%s'", part)
			}
			curr = s[idx]
		} else {
			return nil, common.NewSystemError("INVALID_PATH_SEGMENT").WithMessagef("invalid path segment for non-object/array type: '%s'", part)
		}
	}
	return curr, nil
}

func parseIndex(key string, length int) (int, bool, error) {
	if key == "-" {
		return length, true, nil
	}
	i, err := strconv.Atoi(key)
	if err != nil || i < 0 {
		return 0, false, common.NewSystemError("INVALID_ARRAY_INDEX").WithMessagef("invalid index '%s'", key)
	}
	return i, false, nil
}

func (p *Patcher) getParts(path string) ([]string, error) {
	if path == "" { return []string{}, nil }
	if path == "/" { return []string{""}, nil }

	p.mu.RLock()
	cached, ok := p.cache[path]
	p.mu.RUnlock()
	if ok { return cached, nil }

	// RFC 6901: must start with /
	if !strings.HasPrefix(path, "/") {
		return nil, common.NewSystemError("INVALID_PATH").WithMessage("path must start with /")
	}

	segments := strings.Split(path[1:], "/")
	for i, s := range segments {
		s = strings.ReplaceAll(s, "~1", "/")
		segments[i] = strings.ReplaceAll(s, "~0", "~")
	}

	p.mu.Lock()
	p.cache[path] = segments
	p.mu.Unlock()
	return segments, nil
}

func deepCopy(v any) any {
	if v == nil { return nil }

	switch t := v.(type) {
	case map[string]any:
		cp := make(map[string]any, len(t))
		for k, val := range t { cp[k] = deepCopy(val) }
		return cp
	case []any:
		cp := make([]any, len(t))
		for i, val := range t { cp[i] = deepCopy(val) }
		return cp
	default:
		return v // Primitive types are immutable in Go
	}
}
