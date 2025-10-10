package data

func (d Document) Normalize() Document {
    // Copy the document
    clean := make(Document, len(d))
    for k, v := range d {
        if k == MetadataField {
            clean[k] = v // keep top-level metadata
            continue
        }
        clean[k] = stripNestedMetadata(v)
    }
    return clean
}

// stripNestedMetadata recursively removes the _metadata_ field from nested documents,
// maps, and slices of documents or interfaces. It ensures that only the top-level
// document retains its metadata, providing a clean data structure for operations
// that do not require metadata on nested elements.
func stripNestedMetadata(value any) any {
    switch v := value.(type) {
    case Document:
        // strip metadata completely for nested docs
        clean := make(Document, len(v))
        for k2, v2 := range v {
            if k2 != MetadataField {
                clean[k2] = stripNestedMetadata(v2)
            }
        }
        return clean
    case []Document:
        out := make([]Document, len(v))
        for i, doc := range v {
            out[i] = stripNestedMetadata(doc).(Document)
        }
        return out
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
