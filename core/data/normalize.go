package data

import "context"

// Normalize returns a new Document with nested metadata fields removed,
// while preserving the top-level metadata.
func (d *Document) Normalize() *Document {
	if d == nil {
		return nil
	}

	cleanData := make(map[string]any, len(d.data))
	for k, v := range d.data {
		if k == MetadataField {
			// Keep top-level metadata as is
			cleanData[k] = deepCloneValue(v) // Deep clone to avoid modification side-effects
			continue
		}
		cleanData[k] = stripNestedMetadata(v)
	}
	return &Document{ctx: d.ctx, data: cleanData}
}

// stripNestedMetadata recursively removes the _metadata_ field from nested documents,
// maps, and slices of documents or interfaces. It ensures that only the top-level
// document retains its metadata, providing a clean data structure for operations
// that do not require metadata on nested elements.
func stripNestedMetadata(value any) any {
	switch v := value.(type) {
	case *Document:
		if v == nil {
			return nil
		}
		cleanData := make(map[string]any, len(v.data))
		for k, v2 := range v.data {
			if k != MetadataField { // Skip metadata field
				cleanData[k] = stripNestedMetadata(v2)
			}
		}
		// Return a new Document *without* the old context (as it's nested)
		// and with the cleaned data.
		return &Document{ctx: context.Background(), data: cleanData}
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
	case map[string]any: // Handle maps that are not Documents
		cleanMap := make(map[string]any, len(v))
		for k, v2 := range v {
			if k != MetadataField {
				cleanMap[k] = stripNestedMetadata(v2)
			}
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
