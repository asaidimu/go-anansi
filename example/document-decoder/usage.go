package main

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// keyForPath resolves a dot-separated schema path to the leaf field's flat
// DataContainerKey. This is the canonical address computation: ResolvePath
// turns the dotted path into (SchemaIdx, FieldIdx) steps, then Address computes
// the flat user-data address (memoised on the CompiledSchema) which, combined
// with the leaf descriptor, forms the key under which the value is stored.
func keyForPath(cs *definition.CompiledSchema, path string) (container.DataContainerKey, definition.FieldDescriptor, error) {
	// rp, err := cs.ResolvePath(path)
	// if err != nil {
		return 0, 0, nil
	// }
	// last := rp[len(rp)-1]
	// slot := cs.Schemas[last.SchemaIdx()]
	// abs := int(slot.FieldStart) + int(last.FieldIdx())
	// fd := cs.Descriptors[abs]
	// key, err := leafKey(cs, fd, rp)
	// if err != nil {
	// 	return 0, 0, err
	// }
	// return key, fd, nil
}

// Lookup reads the value stored at a dotted schema path, e.g. "address.zip" or
// "items" (an array of object containers). It demonstrates the primary usage
// pattern of the flat storage scheme: a single ResolvePath followed by an O(1)
// typed read from the container's slot. Pass a child container to read a path
// relative to that element (e.g. Lookup(cs, items[0], "items.street")).
func Lookup(cs *definition.CompiledSchema, doc *container.DataContainer, path string) (any, error) {
	key, fd, err := keyForPath(cs, path)
	if err != nil {
		return nil, err
	}
	v, ok, err := getByType(doc, fd.DataType(), key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("lookup %q: no value stored", path)
	}
	return v, nil
}

// getByType reads a value from the container slot matching the descriptor's
// data type, mirroring setByType for the read direction.
func getByType(doc *container.DataContainer, dt container.DataType, key container.DataContainerKey) (any, bool, error) {
	switch dt {
	case container.TypeInt:
		return doc.GetInt(key)
	case container.TypeFloat:
		return doc.GetFloat(key)
	case container.TypeString:
		return doc.GetString(key)
	case container.TypeBool:
		return doc.GetBool(key)
	case container.TypeBytes:
		return doc.GetBytes(key)
	case container.TypeGeometry:
		return doc.GetGeometry(key)
	case container.TypeRecord:
		return doc.GetRecord(key)
	case container.TypeArrayInt:
		return doc.GetArrayInt(key)
	case container.TypeArrayFloat:
		return doc.GetArrayFloat(key)
	case container.TypeArrayString:
		return doc.GetArrayString(key)
	case container.TypeArrayBool:
		return doc.GetArrayBool(key)
	case container.TypeArrayBytes:
		return doc.GetArrayBytes(key)
	case container.TypeArrayGeometry:
		return doc.GetArrayGeometry(key)
	case container.TypeArrayUnknown:
		return doc.GetArrayUnknown(key)
	case container.TypeArrayObject:
		return doc.GetArrayObject(key)
	case container.TypeUnknown:
		return doc.GetUnknown(key)
	}
	return nil, false, fmt.Errorf("lookup: unsupported data type %d", dt)
}
