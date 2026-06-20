package data

import "context"

// Normalize returns a new Document with nested metadata fields removed from nested documents,
// while preserving the top-level document's ID and metadata.
//
// This ensures that only the top-level document retains its system fields,
// providing a clean data structure for operations that do not require metadata
// on nested elements.
func (d *Document) Normalize() *Document {
	if d == nil {
		return nil
	}

	// Clean user data by stripping metadata from nested documents
	cleanData := make(map[string]any, len(d.data))
	for k, v := range d.data {
		cleanData[k] = stripNestedMetadata(v)
	}

	// Return new document with same ID and metadata, but cleaned data
	return &Document{
		id:       d.id,
		ctx:      d.ctx,
		data:     cleanData,
		metadata: deepCloneValue(d.metadata).(map[string]any),
	}
}

// stripNestedMetadata recursively removes the metadata field from nested documents,
// maps, and slices of documents or interfaces. It ensures that only the top-level
// document retains its metadata, providing a clean data structure for operations
// that do not require metadata on nested elements.
//
// This function works with the refactored Document structure where id, data, and
// metadata are separate fields.
func stripNestedMetadata(value any) any {
	switch v := value.(type) {
	case *Document:
		if v == nil {
			return nil
		}
		// Recursively strip metadata from nested document's data
		cleanData := make(map[string]any, len(v.data))
		for k, v2 := range v.data {
			cleanData[k] = stripNestedMetadata(v2)
		}
		// Return a new Document WITHOUT metadata, with background context
		// Only the ID and cleaned data are preserved
		return &Document{
			id:   v.id,
			ctx:  context.Background(),
			data: cleanData,
			// metadata intentionally omitted - stripped for nested documents
		}

	case Document:
		// Handle value type by converting to pointer
		return stripNestedMetadata(&v)

	case []*Document:
		out := make([]*Document, len(v))
		for i, doc := range v {
			if doc == nil {
				out[i] = nil
				continue
			}
			if strippedDoc, ok := stripNestedMetadata(doc).(*Document); ok {
				out[i] = strippedDoc
			} else {
				out[i] = doc // fallback to original if stripping fails
			}
		}
		return out

	case map[string]any:
		// Handle maps that are not Documents
		cleanMap := make(map[string]any, len(v))
		for k, v2 := range v {
			// Skip reserved system fields in plain maps
			// (though they shouldn't appear in user data)
			if ReservedSystemField(k) {
				continue
			}
			cleanMap[k] = stripNestedMetadata(v2)
		}
		return cleanMap

	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = stripNestedMetadata(item)
		}
		return out

	default:
		return v
	}
}
