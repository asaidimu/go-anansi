package data

// DocumentCache provides simple in-memory caching for documents.
//
// Deprecated: Use document.DocumentPool pooling instead.
type DocumentCache struct {
	cache   map[string]Documenter
	maxSize int
}

// NewDocumentCache creates a new document cache with specified maximum size.
//
// Deprecated: Use document.DocumentPool pooling instead.
func NewDocumentCache(maxSize int) *DocumentCache {
	return &DocumentCache{
		cache:   make(map[string]Documenter),
		maxSize: maxSize,
	}
}

// Get retrieves a document from cache.
//
// Deprecated: Use document.DocumentPool pooling instead.
func (dc *DocumentCache) Get(key string) (Documenter, bool) {
	doc, ok := dc.cache[key]
	return doc, ok
}

// Set stores a document in cache.
//
// Deprecated: Use document.DocumentPool pooling instead.
func (dc *DocumentCache) Set(key string, doc *Document) {
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
//
// Deprecated: Use document.DocumentPool pooling instead.
func (dc *DocumentCache) Clear() {
	dc.cache = make(map[string]Documenter)
}
