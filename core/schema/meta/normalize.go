package meta

import (
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/gofrs/uuid/v5"
)

type idMap struct {
	fields      map[string]string
	indexes     map[string]string
	constraints map[string]string
	schemas     map[string]string
}

func newIDMap() *idMap {
	return &idMap{
		fields:      make(map[string]string),
		indexes:     make(map[string]string),
		constraints: make(map[string]string),
		schemas:     make(map[string]string),
	}
}

func newUUID() string {
	return uuid.Must(uuid.NewV7()).String()
}


func NormalizeSchema(s *definition.Schema) bool {
	m := newIDMap()
	changed := false

	changed = normalizeBaseSchema(&s.BaseSchema, m) || changed
	changed = normalizeSchemas(s, m) || changed
	changed = updateSchemaRefs(s, m) || changed

	return changed
}

func normalizeBaseSchema(bs *definition.BaseSchema, m *idMap) bool {
	changed := false

	if newFields, ok := normalizeFieldMap(bs.Fields, m); ok {
		bs.Fields = newFields
		changed = true
	}
	if newIndexes, ok := normalizeIndexMap(bs.Indexes, m); ok {
		bs.Indexes = newIndexes
		changed = true
	}
	if newConstraints, ok := normalizeConstraintMap(bs.Constraints, m); ok {
		bs.Constraints = newConstraints
		changed = true
	}

	return changed
}

func normalizeFieldMap(fields map[definition.FieldId]definition.Field, m *idMap) (map[definition.FieldId]definition.Field, bool) {
	result := make(map[definition.FieldId]definition.Field, len(fields))
	changed := false
	for id, f := range fields {
		sid := string(id)
		if isUUIDv7(sid) {
			result[id] = f
			continue
		}
		newID := definition.FieldId(newUUID())
		m.fields[sid] = string(newID)
		if f.Name == "" {
			f.Name = definition.FieldName(sid)
		}
		result[newID] = f
		changed = true
	}
	return result, changed
}

func normalizeIndexMap(indexes map[definition.IndexID]definition.Index, m *idMap) (map[definition.IndexID]definition.Index, bool) {
	result := make(map[definition.IndexID]definition.Index, len(indexes))
	changed := false
	for id, idx := range indexes {
		sid := string(id)
		if isUUIDv7(sid) {
			result[id] = idx
			continue
		}
		newID := definition.IndexID(newUUID())
		m.indexes[sid] = string(newID)
		if idx.Name == "" {
			idx.Name = sid
		}
		result[newID] = idx
		changed = true
	}
	return result, changed
}

func normalizeConstraintMap(constraints map[definition.ConstraintId]definition.Constraint, m *idMap) (map[definition.ConstraintId]definition.Constraint, bool) {
	result := make(map[definition.ConstraintId]definition.Constraint, len(constraints))
	changed := false
	for id, c := range constraints {
		sid := string(id)
		if isUUIDv7(sid) {
			result[id] = c
			continue
		}
		newID := definition.ConstraintId(newUUID())
		m.constraints[sid] = string(newID)
		if c.Name == "" {
			c.Name = sid
		}
		result[newID] = c
		changed = true
	}
	return result, changed
}

func normalizeNestedSchemaMap(schemas map[definition.SchemaId]definition.NestedSchema, m *idMap) (map[definition.SchemaId]definition.NestedSchema, bool) {
	result := make(map[definition.SchemaId]definition.NestedSchema, len(schemas))
	changed := false
	for id, ns := range schemas {
		sid := string(id)
		newKey := id
		if !isUUIDv7(sid) {
			newID := definition.SchemaId(newUUID())
			m.schemas[sid] = string(newID)
			if ns.Name == "" {
				ns.Name = sid
			}
			newKey = newID
			changed = true
		}
		// Recurse into nested schema's own fields/indexes/constraints
		changed = normalizeBaseSchema(&ns.BaseSchema, m) || changed
		result[newKey] = ns
	}
	return result, changed
}

func normalizeSchemas(s *definition.Schema, m *idMap) bool {
	if len(s.Schemas) == 0 {
		return false
	}
	newSchemas, changed := normalizeNestedSchemaMap(s.Schemas, m)
	s.Schemas = newSchemas
	return changed
}

func updateSchemaRefs(s *definition.Schema, m *idMap) bool {
	changed := false

	// Update schema refs in fields
	for id, f := range s.Fields {
		if f2, ok := rewriteFieldSchemaRef(f, m); ok {
			s.Fields[id] = f2
			changed = true
		}
	}

	// Update schema refs in nested schemas
	for sid, ns := range s.Schemas {
		nsChanged := false
		for id, f := range ns.Fields {
			if f2, ok := rewriteFieldSchemaRef(f, m); ok {
				ns.Fields[id] = f2
				nsChanged = true
			}
		}
		// Also handle NestedSchema's own FieldProperties.Schema (composite/union refs)
		if !ns.Schema.IsZero() {
			if ns.Schema.IsSingle() {
				sr, _ := definition.FieldSchemaAs[definition.SchemaReference](ns.Schema)
				sr2, ok := rewriteSchemaRef(sr, m)
				if ok {
					ns.Schema = definition.NewSchemaReference(sr2)
					nsChanged = true
				}
			} else if ns.Schema.IsMultiple() {
				refs, _ := definition.FieldSchemaAs[[]definition.SchemaReference](ns.Schema)
				for i, sr := range refs {
					sr2, ok := rewriteSchemaRef(sr, m)
					if ok {
						refs[i] = sr2
						nsChanged = true
					}
				}
				if nsChanged {
					ns.Schema = definition.NewSchemaReference(refs)
				}
			}
		}
		if nsChanged {
			s.Schemas[sid] = ns
			changed = true
		}
	}

	return changed
}

func rewriteFieldSchemaRef(f definition.Field, m *idMap) (definition.Field, bool) {
	if f.Schema.IsZero() {
		return f, false
	}
	changed := false
	if f.Schema.IsSingle() {
		sr, _ := definition.FieldSchemaAs[definition.SchemaReference](f.Schema)
		sr2, ok := rewriteSchemaRef(sr, m)
		if ok {
			f.Schema = definition.NewSchemaReference(sr2)
			changed = true
		}
	} else if f.Schema.IsMultiple() {
		refs, _ := definition.FieldSchemaAs[[]definition.SchemaReference](f.Schema)
		for i, sr := range refs {
			sr2, ok := rewriteSchemaRef(sr, m)
			if ok {
				refs[i] = sr2
				changed = true
			}
		}
		if changed {
			f.Schema = definition.NewSchemaReference(refs)
		}
	}
	return f, changed
}

func rewriteSchemaRef(sr definition.SchemaReference, m *idMap) (definition.SchemaReference, bool) {
	changed := false
	if newID, ok := m.schemas[string(sr.ID)]; ok {
		sr.ID = definition.SchemaId(newID)
		changed = true
	}
	// Also normalize indexes/constraints within the schema reference
	if newIdx, ok := normalizeIndexMap(sr.Indexes, m); ok {
		sr.Indexes = newIdx
		changed = true
	}
	if newCons, ok := normalizeConstraintMap(sr.Constraints, m); ok {
		sr.Constraints = newCons
		changed = true
	}
	return sr, changed
}
