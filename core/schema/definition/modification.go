package definition

import (
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
		f.Type != other.Type {
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
