package data


// AsDocument attempts to convert any value to a Document.
func AsDocument(v any) (Document, bool) {
	switch val := v.(type) {
	case Document:
		return val, true
	case map[string]any:
		return Document(val), true
	case nil:
		return make(Document), true
	default:
		return nil, false
	}
}

// AsDocumentArray attempts to convert any value to []Document.
func AsDocumentArray(v any) ([]Document, bool) {
	switch val := v.(type) {
	case []Document:
		return val, true
	case []any:
		docs := make([]Document, 0, len(val))
		for _, item := range val {
			if doc, ok := AsDocument(item); ok {
				docs = append(docs, doc)
			} else {
				return nil, false
			}
		}
		return docs, true
	case []map[string]any:
		docs := make([]Document, len(val))
		for i, m := range val {
			docs[i] = Document(m)
		}
		return docs, true
	default:
		return nil, false
	}
}
