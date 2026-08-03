package json

import (
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// Dump materialises a DataContainer as a JSON-friendly map keyed by each
// field's fully-qualified schema path. Records appear as their map value;
// arrays of objects appear as a list of per-element dumps.
func Dump(cs *definition.CompiledSchema, doc *container.DataContainer) (map[string]any, error) {
	return dump(cs, doc)
}

func dump(cs *definition.CompiledSchema, doc *container.DataContainer) (map[string]any, error) {
	out := map[string]any{}
	_, err := doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		for k, idx := range positions {
			key := container.DataContainerKey(k)
			name := nameFor(cs, key)
			if idx < 0 {
				out[name] = nil
				continue
			}
			switch key.Type() {
			case container.TypeInt:
				out[name] = (*(*[]int64)(slot(container.TypeInt)))[idx]
			case container.TypeFloat:
				out[name] = (*(*[]float64)(slot(container.TypeFloat)))[idx]
			case container.TypeString:
				out[name] = (*(*[]string)(slot(container.TypeString)))[idx]
			case container.TypeBool:
				out[name] = (*(*[]bool)(slot(container.TypeBool)))[idx]
			case container.TypeBytes:
				out[name] = (*(*[][]byte)(slot(container.TypeBytes)))[idx]
			case container.TypeGeometry:
				out[name] = (*(*[][][]float64)(slot(container.TypeGeometry)))[idx]
			case container.TypeRecord:
				out[name] = (*(*[]map[string]any)(slot(container.TypeRecord)))[idx]
			case container.TypeArrayInt:
				out[name] = (*(*[][]int64)(slot(container.TypeArrayInt)))[idx]
			case container.TypeArrayFloat:
				out[name] = (*(*[][]float64)(slot(container.TypeArrayFloat)))[idx]
			case container.TypeArrayString:
				out[name] = (*(*[][]string)(slot(container.TypeArrayString)))[idx]
			case container.TypeArrayBool:
				out[name] = (*(*[][]bool)(slot(container.TypeArrayBool)))[idx]
			case container.TypeArrayBytes:
				out[name] = (*(*[][][]byte)(slot(container.TypeArrayBytes)))[idx]
			case container.TypeArrayGeometry:
				out[name] = (*(*[][][][]float64)(slot(container.TypeArrayGeometry)))[idx]
			case container.TypeArrayUnknown:
				out[name] = (*(*[][]any)(slot(container.TypeArrayUnknown)))[idx]
			case container.TypeArrayObject:
				group := (*(*[][]*container.DataContainer)(slot(container.TypeArrayObject)))[idx]
				els := make([]any, len(group))
				for i, child := range group {
					m, err := dump(cs, child)
					if err != nil {
						return nil, err
					}
					els[i] = m
				}
				out[name] = els
			case container.TypeUnknown:
				out[name] = (*(*[]any)(slot(container.TypeUnknown)))[idx]
			}
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// nameFor recovers a stored key's fully-qualified schema path. Addressable
// leaves are named via the schema's cached address→path name (a single map
// lookup, populated when the path was resolved). Non-terminal containers
// (records, array-of-object fields, unions) carry no flat address, so they fall
// back to their descriptor's local name.
func nameFor(cs *definition.CompiledSchema, key container.DataContainerKey) string {
	if name, ok := cs.PathNameForAddress(uint32(key.DataPoint().ID())); ok {
		return name
	}
	return cs.FieldPath(definition.FieldDescriptor(key.Descriptor()))
}
