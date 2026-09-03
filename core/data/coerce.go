package data

import "context"

// DocumentFrom attempts to convert any value to a *Document.
// It also accepts an optional context for metadata injection.
//
// Deprecated: Use document.DocumentPool.FromMap instead.
func DocumentFrom(v any, ctx ...context.Context) (*Document, bool) {
	switch val := v.(type) {
	case *Document:
		return val, true
	case Document:
		return &val, true
	case map[string]any:
		newDoc, err := NewDocument(val, ctx...)
		if err != nil {
			return nil, false // Or handle error appropriately
		}
		return newDoc, true
	case nil:
		newDoc, err := NewDocument(make(map[string]any), ctx...)
		if err != nil {
			return nil, false // Or handle error appropriately
		}
		return newDoc, true
	default:
		return nil, false
	}
}

// Deprecated: Use NewDocumentSet instead.
func DocumentSlice(v any, ctx ...context.Context) ([]*Document, bool) {
	set, ok := NewDocumentSet(v, ctx...)
	if !ok {
		return nil, false
	}
	result := make([]*Document, len(set))
	for i, doc := range set {
		doc, ok := doc.(*Document)
		if !ok {
			return nil, false
		}
		result[i] = doc
	}
	return result, true
}
