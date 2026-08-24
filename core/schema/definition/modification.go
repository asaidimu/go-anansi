package definition

import (
	"fmt"
	"reflect"

	"github.com/google/uuid"
)

// GetFieldByName returns the field with the given name, if it exists.
func (s BaseSchema) GetFieldByName(name FieldName) (FieldId, *Field, bool) {
	for id, field := range s.Fields {
		if field.Name == name {
			return id, &field, true
		}
	}
	return "", nil, false
}

// Equals checks if two fields are identical.
func (f *Field) Equals(other *Field) bool {
	if f.Name != other.Name ||
		f.Description != other.Description ||
		f.Required != other.Required ||
		f.Deprecated != other.Deprecated ||
		f.Unique != other.Unique ||
		!reflect.DeepEqual(f.Nullable, other.Nullable) ||
		f.Type != other.Type {
		return false
	}
	if !reflect.DeepEqual(f.Metadata, other.Metadata) {
		return false
	}
	// For simplicity in this migration, we'll skip deep comparison of Default and Schema for now,
	// or we can implement it if needed.
	return true
}

// WithField returns a new schema with the field added or replaced (by ID)
func (s *Schema) WithField(id FieldId, field Field) *Schema {
	clone := s.DeepCopy()
	if clone.Fields == nil {
		clone.Fields = make(map[FieldId]Field)
	}
	clone.Fields[id] = field
	return clone
}

// WithFieldEnsured returns a new schema ensuring the field exists with exact properties.
// If a field with the same name exists but different properties, it's replaced.
// If it doesn't exist, it's added with a new ID.
func (s *Schema) WithFieldEnsured(field *Field) (*Schema, FieldId, bool, error) {
	existingID, existingField, exists := s.GetFieldByName(field.Name)

	if exists {
		if existingField.Equals(field) {
			return s, existingID, false, nil
		}
		// Replace
		return s.WithField(existingID, *field), existingID, true, nil
	}

	// Add new
	newID := FieldId(uuid.Must(uuid.NewV7()).String())
	return s.WithField(newID, *field), newID, true, nil
}

// WithoutIndexesReferencingField returns a new schema without any indexes that reference the given field.
func (s *Schema) WithoutIndexesReferencingField(fieldName FieldName) (*Schema, bool, error) {
	clone := s.DeepCopy()
	modified := false
	for id, index := range clone.Indexes {
		for _, fn := range index.Fields {
			if fn == fieldName {
				delete(clone.Indexes, id)
				modified = true
				break
			}
		}
	}

	return clone, modified, nil
}

// Equals checks if two indexes are identical.
func (idx *Index) Equals(other *Index) bool {
	if idx.Name != other.Name ||
		idx.Type != other.Type ||
		idx.Order != other.Order ||
		idx.Unique != other.Unique ||
		len(idx.Fields) != len(other.Fields) {
		return false
	}

	for i, f := range idx.Fields {
		if f != other.Fields[i] {
			return false
		}
	}
	return true
}

// GetIndexByName returns the index with the given name, if it exists.
func (s *Schema) GetIndexByName(name string) (IndexID, *Index, bool) {
	for id, index := range s.Indexes {
		if index.Name == name {
			return id, &index, true
		}
	}
	return "", nil, false
}

// WithIndex returns a new schema with the index added or replaced (by ID)
func (s *Schema) WithIndex(id IndexID, index Index) *Schema {
	clone := s.DeepCopy()
	if clone.Indexes == nil {
		clone.Indexes = make(map[IndexID]Index)
	}
	clone.Indexes[id] = index
	return clone
}

// WithIndexEnsured returns a new schema ensuring the index exists with exact properties.
func (s *Schema) WithIndexEnsured(index *Index) (*Schema, bool, error) {
	existingID, existingIndex, exists := s.GetIndexByName(index.Name)

	if exists {
		if existingIndex.Equals(index) {
			return s, false, nil
		}
		// Replace
		return s.WithIndex(existingID, *index), true, nil
	}

	// Add new
	newID := IndexID(uuid.Must(uuid.NewV7()).String())
	return s.WithIndex(newID, *index), true, nil
}

// WithSchema composes a full schema into this schema as a fields-mode nested
// schema. The sub-schema's root fields become the nested schema's fields, and
// the sub-schema's own nested schemas (sub.Schemas) are merged into this
// schema's nested-schema registry.
//
// It returns the new schema and the SchemaId of the composed nested schema.
// Use that SchemaId to attach the composed body to a root field:
//
//	composed, dtoID, err := envelope.WithSchema(dtoSchema)
//	envelope = composed.WithField(fid, definition.Field{
//	    Name: "payload",
//	    FieldProperties: definition.FieldProperties{
//	        Type:   definition.FieldTypeObject,
//	        Schema: definition.NewSchemaReference(definition.SchemaReference{ID: dtoID}),
//	    },
//	})
//
// What is merged:
//   - sub.Fields become the nested schema's fields (schema-mode -> object
//     referenceable by SchemaId).
//   - sub.Schemas are merged into s.Schemas. A nested schema ID that already
//     exists in s is remapped to a fresh UUIDv7, and every field reference
//     pointing at the old ID (within the merged subtree) is rewritten to the
//     new ID so the composed body stays internally consistent.
//
// The receiver is not mutated; a deep copy is returned.
func (s *Schema) WithSchema(sub *Schema) (*Schema, SchemaId, error) {
	if sub == nil {
		return nil, "", fmt.Errorf("WithSchema: nil sub-schema")
	}
	clone := s.DeepCopy()
	if clone.Schemas == nil {
		clone.Schemas = make(map[SchemaId]NestedSchema)
	}

	// Root nested schema bearing the sub-schema's root fields.
	composedID := SchemaId(uuid.Must(uuid.NewV7()).String())

	// Build a remap that rewrites every reference destined for a sub nested
	// schema: keep the ID when free, else mint a fresh one.
	remap := make(map[SchemaId]SchemaId)

	// Integration order matters: mapping must be registered before any fields
	// are rewritten. First pass registers remaps (no rewrites that depend on
	// future entries), so do discovery in two passes.
	// Pass 1: decide new IDs for any collision.
	for id := range sub.Schemas {
		if _, taken := clone.Schemas[id]; taken {
			if _, ok := remap[id]; !ok {
				remap[id] = SchemaId(uuid.Must(uuid.NewV7()).String())
			}
		}
	}
	// Pass 2: copy schemas under their (possibly remapped) IDs, rewriting refs.
	for id, ns := range sub.Schemas {
		target := id
		if nid, ok := remap[id]; ok {
			target = nid
		}
		ns.Fields = rewriteFieldRefs(ns.Fields, remap)
		clone.Schemas[target] = ns
	}

	// The root composed body carries the sub-schema's root fields, with any
	// references to sub.Schemas remapped too.
	rootBody := NestedSchema{
		BaseSchema: BaseSchema{
			Name:        sub.Name,
			Fields:      rewriteFieldRefs(sub.Fields, remap),
			Indexes:     sub.Indexes,
			Constraints: sub.Constraints,
			Metadata:    sub.Metadata,
		},
	}
	clone.Schemas[composedID] = rootBody

	return clone, composedID, nil
}

// rewriteFieldRefs copies fields and rewrites every SchemaReference ID through
// remap. Unknown IDs are preserved. The input map is not mutated.
func rewriteFieldRefs(fields map[FieldId]Field, remap map[SchemaId]SchemaId) map[FieldId]Field {
	if len(remap) == 0 || len(fields) == 0 {
		return fields
	}
	out := make(map[FieldId]Field, len(fields))
	for id, f := range fields {
		f.Schema = remapRef(f.Schema, remap)
		out[id] = f
	}
	return out
}

// remapRef rewrites a single or multiple FieldSchemaReference through remap;
// the original reference is not mutated.
func remapRef(fsr FieldSchemaReference, remap map[SchemaId]SchemaId) FieldSchemaReference {
	if fsr.IsZero() {
		return fsr
	}
	if fsr.IsSingle() {
		sr, err := FieldSchemaAs[SchemaReference](fsr)
		if err != nil {
			return fsr
		}
		if nid, ok := remap[sr.ID]; ok {
			sr.ID = nid
			return NewSchemaReference(sr)
		}
		return fsr
	}
	if fsr.IsMultiple() {
		refs, err := FieldSchemaAs[[]SchemaReference](fsr)
		if err != nil {
			return fsr
		}
		copied := make([]SchemaReference, len(refs))
		changed := false
		for i, r := range refs {
			copied[i] = r
			if nid, ok := remap[r.ID]; ok {
				copied[i].ID = nid
				changed = true
			}
		}
		if changed {
			return NewSchemaReference(copied)
		}
		return fsr
	}
	return fsr
}
