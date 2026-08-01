package main

import (
	"encoding/json"
	"fmt"
	"log"
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// Experimental JSON document decoder demo.
//
// Compiles an inline schema, decodes an inline JSON document into a
// DataContainer, then dumps the container's contents (via Walk) as a JSON
// object keyed by each field's schema path.
func main() {
	cs, err := compileSchema([]byte(schemaJSON))
	if err != nil {
		log.Fatalf("compile schema: %v", err)
	}

	doc, err := DecodeJSON(cs, []byte(documentJSON))
	if err != nil {
		log.Fatalf("decode document: %v", err)
	}

	dump, err := Dump(cs, doc)
	if err != nil {
		log.Fatalf("dump document: %v", err)
	}
	enc, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		log.Fatalf("marshal dump: %v", err)
	}
	fmt.Println(string(enc))
	fmt.Println()

	// ── usage patterns: reading the decoded document ────────────────────────
	usage(cs, doc)
}

// usage demonstrates realistic read patterns against a decoded document.
func usage(cs *definition.CompiledSchema, doc *container.DataContainer) {
	fmt.Println("── usage patterns ─────────────────────────────────────────")

	// 1. Random access by dotted path: ResolvePath → flat address → O(1) read.
	id, _ := Lookup(cs, doc, "id")
	street, _ := Lookup(cs, doc, "address.street")
	zipVal, _ := Lookup(cs, doc, "address.zip")
	fmt.Printf("1. random access:\n   id=%v address.street=%q address.zip=%v\n",
		id, street, zipVal)

	// 2. Read the record and unknown fields by path.
	profileRaw, _ := Lookup(cs, doc, "profile")
	metaRaw, _ := Lookup(cs, doc, "meta")
	profile := profileRaw.(map[string]any)
	meta := metaRaw.(map[string]any)
	fmt.Printf("2. raw containers:\n   profile=%v\n   meta=%v\n", profile, meta)

	// 2b. A composite collapses its parts into one object schema, so its
	//     children flatten into the key space just like an object's.
	lat, _ := Lookup(cs, doc, "location.lat")
	label, _ := Lookup(cs, doc, "location.label")
	fmt.Printf("2b. composite flattening:\n   location.lat=%v location.label=%q\n", lat, label)

	// 3. Iterate an array of objects, aggregating per-element fields. Each
	//    element is its own DataContainer; the full path ("items.zip") is
	//    resolved against it.
	itemsRaw, _ := Lookup(cs, doc, "items")
	items := itemsRaw.([]*container.DataContainer)
	var totalZip int64
	for i, child := range items {
		s, _ := Lookup(cs, child, "items.street")
		z, _ := Lookup(cs, child, "items.zip")
		zv := z.(int64)
		totalZip += zv
		fmt.Printf("   items[%d] -> %q zip=%d\n", i, s, zv)
	}
	fmt.Printf("   sum(items.zip) = %d\n", totalZip)

	// 4. Filter: keep elements matching a predicate.
	var near []int64
	for _, child := range items {
		z, _ := Lookup(cs, child, "items.zip")
		if z.(int64) < 35000 {
			near = append(near, z.(int64))
		}
	}
	fmt.Printf("4. filter items.zip < 35000: %v\n", near)

	// 5. Mutate a resolved value and observe the change.
	key, _, err := keyForPath(cs, "address.zip")
	if err != nil {
		log.Fatalf("resolve address.zip: %v", err)
	}
	if err := doc.SetInt(key, 99999); err != nil {
		log.Fatalf("update address.zip: %v", err)
	}
	updated, _ := Lookup(cs, doc, "address.zip")
	fmt.Printf("5. mutation:\n   address.zip now = %v (was %v)\n", updated, zipVal)

	// 6. Update a single array element and re-read it.
	childKey, _, _ := keyForPath(cs, "items.street")
	if err := items[0].SetString(childKey, "9 Maple Dr"); err != nil {
		log.Fatalf("update items[0].street: %v", err)
	}
	s0, _ := Lookup(cs, items[0], "items.street")
	fmt.Printf("6. element update:\n   items[0].street now = %q\n", s0)

	// 7. Storage-level scan with Walk: count values per data type.
	counts := map[container.DataType]int{}
	_, err = doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		for k, idx := range positions {
			if idx < 0 {
				continue
			}
			counts[container.DataContainerKey(k).Type()]++
		}
		return nil, nil
	})
	if err != nil {
		log.Fatalf("walk: %v", err)
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	fmt.Printf("7. walk scan:\n   %d values across %d data types\n", total, len(counts))
}

func compileSchema(data []byte) (*definition.CompiledSchema, error) {
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
