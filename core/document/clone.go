package document

import (
	"sort"
	"strings"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
)

// cloneContainer deep-copies a DataContainer, recursively cloning array-of-
// object child containers.
func cloneContainer(c *container.DataContainer) *container.DataContainer {
	if c == nil {
		return nil
	}
	out := container.NewDataContainer()
	_, _ = c.Walk(func(positions map[int64]int32, slot func(t container.DataType, initialSize ...int) unsafe.Pointer) (any, error) {
		for k, idx := range positions {
			key := container.DataContainerKey(k)
			dt := key.Type()
			switch dt {
			case container.TypeInt:
				if idx < 0 {
					out.SetNull(key)
				} else {
					_ = out.SetInt(key, (*(*[]int64)(slot(container.TypeInt)))[idx])
				}
			case container.TypeFloat:
				if idx < 0 {
					out.SetNull(key)
				} else {
					_ = out.SetFloat(key, (*(*[]float64)(slot(container.TypeFloat)))[idx])
				}
			case container.TypeString:
				if idx < 0 {
					out.SetNull(key)
				} else {
					_ = out.SetString(key, (*(*[]string)(slot(container.TypeString)))[idx])
				}
			case container.TypeBool:
				if idx < 0 {
					out.SetNull(key)
				} else {
					_ = out.SetBool(key, (*(*[]bool)(slot(container.TypeBool)))[idx])
				}
			case container.TypeBytes:
				if idx < 0 {
					out.SetNull(key)
				} else {
					src := (*(*[][]byte)(slot(container.TypeBytes)))[idx]
					dst := make([]byte, len(src))
					copy(dst, src)
					_ = out.SetBytes(key, dst)
				}
			case container.TypeGeometry:
				if idx < 0 {
					out.SetNull(key)
				} else {
					_ = out.SetGeometry(key, cloneGeometry((*(*[][][]float64)(slot(container.TypeGeometry)))[idx]))
				}
			case container.TypeRecord:
				if idx < 0 {
					out.SetNull(key)
				} else {
					_ = out.SetRecord(key, deepCloneMap((*(*[]map[string]any)(slot(container.TypeRecord)))[idx]))
				}
			case container.TypeArrayUnknown:
				if idx < 0 {
					out.SetNull(key)
				} else {
					_ = out.SetArrayUnknown(key, cloneAnySlice((*(*[][]any)(slot(container.TypeArrayUnknown)))[idx]))
				}
			case container.TypeArrayInt:
				if idx < 0 {
					out.SetNull(key)
				} else {
					src := (*(*[][]int64)(slot(container.TypeArrayInt)))[idx]
					_ = out.SetArrayInt(key, append([]int64(nil), src...))
				}
			case container.TypeArrayFloat:
				if idx < 0 {
					out.SetNull(key)
				} else {
					src := (*(*[][]float64)(slot(container.TypeArrayFloat)))[idx]
					_ = out.SetArrayFloat(key, append([]float64(nil), src...))
				}
			case container.TypeArrayString:
				if idx < 0 {
					out.SetNull(key)
				} else {
					src := (*(*[][]string)(slot(container.TypeArrayString)))[idx]
					_ = out.SetArrayString(key, append([]string(nil), src...))
				}
			case container.TypeArrayBool:
				if idx < 0 {
					out.SetNull(key)
				} else {
					src := (*(*[][]bool)(slot(container.TypeArrayBool)))[idx]
					_ = out.SetArrayBool(key, append([]bool(nil), src...))
				}
			case container.TypeArrayBytes:
				if idx < 0 {
					out.SetNull(key)
				} else {
					src := (*(*[][][]byte)(slot(container.TypeArrayBytes)))[idx]
					dst := make([][]byte, len(src))
					for i, b := range src {
						dst[i] = append([]byte(nil), b...)
					}
					_ = out.SetArrayBytes(key, dst)
				}
			case container.TypeArrayObject:
				if idx < 0 {
					out.SetNull(key)
				} else {
					src := (*(*[][]*container.DataContainer)(slot(container.TypeArrayObject)))[idx]
					dst := make([]*container.DataContainer, len(src))
					for i, ch := range src {
						dst[i] = cloneContainer(ch)
					}
					_ = out.SetArrayObject(key, dst)
				}
			case container.TypeArrayGeometry:
				if idx < 0 {
					out.SetNull(key)
				} else {
					src := (*(*[][][][]float64)(slot(container.TypeArrayGeometry)))[idx]
					dst := make([][][]float64, len(src))
					for i, g := range src {
						rings := make([][]float64, len(g))
						for j, ring := range g {
							rings[j] = append([]float64(nil), ring...)
						}
						dst[i] = rings
					}
					_ = out.SetArrayGeometry(key, dst)
				}
			case container.TypeUnknown:
				if idx < 0 {
					out.SetNull(key)
				} else {
					_ = out.SetUnknown(key, (*(*[]any)(slot(container.TypeUnknown)))[idx])
				}
			}
		}
		return nil, nil
	})
	return out
}

func cloneGeometry(g [][]float64) [][]float64 {
	out := make([][]float64, len(g))
	for i, ring := range g {
		out[i] = append([]float64(nil), ring...)
	}
	return out
}

func cloneAnySlice(src []any) []any {
	out := make([]any, len(src))
	for i, v := range src {
		out[i] = deepCloneValue(v)
	}
	return out
}

// deepCloneMap deep-copies a map[string]any.
func deepCloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCloneValue(v)
	}
	return out
}

func deepCloneValue(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]any:
		return deepCloneMap(val)
	case []any:
		return cloneAnySlice(val)
	case []map[string]any:
		out := make([]map[string]any, len(val))
		for i, m := range val {
			out[i] = deepCloneMap(m)
		}
		return out
	default:
		return v
	}
}

// sortedMapKeys returns the map's keys in sorted order.
func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// setValueByPath writes a value into a nested map, creating intermediate maps.
func setValueByPath(m map[string]any, path string, value any) error {
	parts := strings.Split(path, ".")
	current := m
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
		nm, ok := next.(map[string]any)
		if !ok {
			return errCannotTraverse
		}
		current = nm
	}
	return nil
}

// deleteValueByPath removes a value from a nested map.
func deleteValueByPath(m map[string]any, path string) error {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		delete(m, parts[0])
		return nil
	}
	current := m
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part]
		if !ok {
			return errPathNotFound
		}
		nm, ok := next.(map[string]any)
		if !ok {
			return errCannotTraverse
		}
		current = nm
	}
	delete(current, parts[len(parts)-1])
	return nil
}
