package data

// DocumentCache provides simple in-memory caching for documents.
type DocumentCache struct {
	cache   map[string]Document
	maxSize int
}

// NewDocumentCache creates a new document cache with specified maximum size.
func NewDocumentCache(maxSize int) *DocumentCache {
	return &DocumentCache{
		cache:   make(map[string]Document),
		maxSize: maxSize,
	}
}

// Get retrieves a document from cache.
func (dc *DocumentCache) Get(key string) (Document, bool) {
	doc, ok := dc.cache[key]
	return doc, ok
}

// Set stores a document in cache.
func (dc *DocumentCache) Set(key string, doc Document) {
	if len(dc.cache) >= dc.maxSize {
		// Simple LRU: remove first key (not truly LRU but simple)
		for k := range dc.cache {
			delete(dc.cache, k)
			break
		}
	}
	dc.cache[key] = doc.Clone()
}

// Clear removes all cached documents.
func (dc *DocumentCache) Clear() {
	dc.cache = make(map[string]Document)
}
