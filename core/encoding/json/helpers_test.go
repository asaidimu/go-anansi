package json

import (
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

func compileSchema(t testing.TB, data []byte) (*definition.CompiledSchema, error) {
	t.Helper()
	s, err := definition.FromJSON(data)
	if err != nil {
		return nil, err
	}
	rs, err := definition.Compile(s)
	if err != nil {
		return nil, err
	}
	return definition.Link(rs)
}

// keyForPath resolves a dot-separated schema path to the leaf field's flat
// DataContainerKey: ResolvePath turns the dotted path into (SchemaIdx,
// FieldIdx) steps, then Address computes the flat user-data address (memoised
// on the CompiledSchema) which, combined with the leaf descriptor, forms the
// key under which the value is stored.
func keyForPath(cs *definition.CompiledSchema, path string) (container.DataContainerKey, definition.FieldDescriptor, error) {
	rp, err := cs.ResolvePath(path)
	if err != nil {
		return 0, 0, err
	}
	last := rp[len(rp)-1]
	slot := cs.Schemas[last.SchemaIdx()]
	abs := int(slot.FieldStart) + int(last.FieldIdx())
	fd := cs.Descriptors[abs]
	key, err := computeLeafKey(cs, fd, rp)
	if err != nil {
		return 0, 0, err
	}
	return key, fd, nil
}

// Lookup reads the value stored at a dotted schema path, e.g. "address.zip" or
// "items" (an array of object containers).
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

const schemaJSON = `{
  "version": "1.0.0",
  "name": "demo",
  "fields": {
    "id":        { "name": "id",        "type": "string" },
    "active":    { "name": "active",    "type": "boolean" },
    "age":       { "name": "age",       "type": "integer" },
    "score":     { "name": "score",     "type": "number" },
    "tags":      { "name": "tags",      "type": "array",  "schema": { "type": "string" } },
    "address":   { "name": "address",   "type": "object", "schema": { "id": "addr" } },
    "profile":   { "name": "profile",   "type": "record", "schema": { "id": "addr" } },
    "items":     { "name": "items",     "type": "array",  "schema": { "id": "addr" } },
    "location": { "name": "location", "type": "composite", "schema": [ { "id": "coord" }, { "id": "zone" } ] },
    "meta":      { "name": "meta",      "type": "unknown" },
    "nickname":  { "name": "nickname",  "type": "string", "default": "anon" }
  },
  "schemas": {
    "addr": {
      "name": "addr",
      "fields": {
        "street": { "name": "street", "type": "string" },
        "zip":    { "name": "zip",    "type": "integer" }
      }
    },
    "coord": {
      "name": "coord",
      "fields": {
        "lat": { "name": "lat", "type": "number" },
        "lng": { "name": "lng", "type": "number" }
      }
    },
    "zone": {
      "name": "zone",
      "fields": {
        "label": { "name": "label", "type": "string" }
      }
    }
  }
}`

const documentJSON = `{
  "id": "user-1",
  "active": true,
  "age": 31,
  "score": 9.5,
  "tags": ["go", "schema"],
  "address": { "street": "1 Main St", "zip": 10001 },
  "location": { "lat": 40.7128, "lng": -74.006, "label": "downtown" },
  "profile": { "street": "2 Side Rd", "zip": 20002 },
  "items": [
    { "street": "3 Leaf Ct", "zip": 30003 },
    { "street": "4 Oak Ave", "zip": 40004 }
  ],
  "meta": { "source": "demo" }
}`

const requiredSchemaJSON = `{
  "version": "1.0.0",
  "name": "required_demo",
  "fields": {
    "id": { "name": "id", "type": "string", "required": true }
  }
}`

const compositeSchemaJSON = `{
  "version": "1.0.0",
  "name": "composite_demo",
  "fields": {
    "identity": { "name": "identity", "type": "composite", "schema": [ { "id": "contact" }, { "id": "geo" } ] }
  },
  "schemas": {
    "contact": { "name": "contact", "fields": {
      "email": { "name": "email", "type": "string" }
    } },
    "geo": { "name": "geo", "fields": {
      "lat": { "name": "lat", "type": "number" },
      "lng": { "name": "lng", "type": "number" }
    } }
  }
}`

const unionSchemaJSON = `{
  "version": "1.0.0",
  "name": "union_demo",
  "fields": {
    "payload": { "name": "payload", "type": "union", "schema": [ { "id": "obj_a" }, { "id": "obj_b" } ] },
    "extras":  { "name": "extras",  "type": "array",  "schema": { "id": "holder" } }
  },
  "schemas": {
    "obj_a": { "name": "obj_a", "fields": { "a_name": { "name": "a_name", "type": "string" } } },
    "obj_b": { "name": "obj_b", "fields": { "b_name": { "name": "b_name", "type": "string" } } },
    "holder": { "name": "holder", "fields": {
      "choice": { "name": "choice", "type": "union", "schema": [ { "id": "obj_a" }, { "id": "obj_b" } ] }
    }     }
  }
}`
