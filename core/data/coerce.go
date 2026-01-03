package data

import "context"

// AsDocument attempts to convert any value to a *Document.
func AsDocument(v any) (*Document, bool) {
	switch val := v.(type) {
	case *Document:
		return val, true
	case Document:
		return &val, true
	case map[string]any:
		return &Document{ctx: context.Background(), data: val}, true
	case nil:
		return &Document{ctx: context.Background(), data: make(map[string]any)}, true
	default:
		return nil, false
	}
}

// DocumentSlice attempts to convert any value to []*Document.
func DocumentSlice(v any) ([]*Document, bool) {
	switch val := v.(type) {
	case []*Document:
		return val, true
	case []Document:
		docs := make([]*Document, len(val))
		for i := range val {
			docs[i] = &val[i]
		}
		return docs, true
	case []any:
		docs := make([]*Document, 0, len(val))
		for _, item := range val {
			if doc, ok := AsDocument(item); ok {
				docs = append(docs, doc)
			} else {
				return nil, false
			}
		}
		return docs, true
	case []map[string]any:
		docs := make([]*Document, len(val))
		for i, m := range val {
			docs[i] = &Document{ctx: context.Background(), data: m}
		}
		return docs, true
	default:
		return nil, false
	}
}
