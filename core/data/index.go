package data

// Index provides fast lookups for document collections.
type DocumentIndex struct {
	keyIndex     map[string]map[any][]int // key -> value -> document indices
	documents    []Document
	keyExtractor func(Document) map[string]any
}

// NewDocumentIndex creates a new index for documents.
func NewDocumentIndex(docs []Document, keyExtractor func(Document) map[string]any) *DocumentIndex {
	index := &DocumentIndex{
		keyIndex:     make(map[string]map[any][]int),
		documents:    docs,
		keyExtractor: keyExtractor,
	}

	index.rebuild()
	return index
}

// rebuild recreates the index.
func (di *DocumentIndex) rebuild() {
	di.keyIndex = make(map[string]map[any][]int)

	for i, doc := range di.documents {
		keys := di.keyExtractor(doc)
		for key, value := range keys {
			if di.keyIndex[key] == nil {
				di.keyIndex[key] = make(map[any][]int)
			}
			di.keyIndex[key][value] = append(di.keyIndex[key][value], i)
		}
	}
}

// Find returns documents matching the key-value pair.
func (di *DocumentIndex) Find(key string, value any) []Document {
	if valueMap, ok := di.keyIndex[key]; ok {
		if indices, ok := valueMap[value]; ok {
			result := make([]Document, len(indices))
			for i, idx := range indices {
				result[i] = di.documents[idx]
			}
			return result
		}
	}
	return []Document{}
}
