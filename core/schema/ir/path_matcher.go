package ir

import (
	"regexp"
	"sort"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

// pathEntry stores the path and key together.
type pathEntry struct {
	path string
	key  document.DocumentKey
}

// PathRegistry is the single source of truth for field paths. It provides
// O(1) bi-directional lookups and O(log n) prefix matching while ensuring
// data is never duplicated in memory.
type PathRegistry struct {
	mu sync.RWMutex

	// data is an append-only slice of path/key pairs.
	// Because it is append-only, indices into this slice are stable.
	data []pathEntry

	// byPath is a sorted slice of indices into 'data'.
	// It is sorted lexicographically by data[idx].path.
	byPath []int

	// byKey and byPathStr map keys/paths to their index in 'data'.
	byKey     map[document.DocumentKey]int
	byPathStr map[string]int
}

func NewPathRegistry() *PathRegistry {
	return &PathRegistry{
		byKey:     make(map[document.DocumentKey]int),
		byPathStr: make(map[string]int),
	}
}

// Put adds a path/key pair to the registry if it doesn't exist.
// It maintains the sorted order of byPath for prefix searching.
func (pr *PathRegistry) Put(key document.DocumentKey, path string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, exists := pr.byKey[key]; exists {
		return
	}

	idx := len(pr.data)
	pr.data = append(pr.data, pathEntry{path: path, key: key})
	pr.byKey[key] = idx
	pr.byPathStr[path] = idx

	// Maintain byPath sort order (O(n) insertion of an int)
	insertAt := sort.Search(len(pr.byPath), func(i int) bool {
		return pr.data[pr.byPath[i]].path >= path
	})
	pr.byPath = append(pr.byPath, 0)
	copy(pr.byPath[insertAt+1:], pr.byPath[insertAt:])
	pr.byPath[insertAt] = idx
}

// GetKey returns the DocumentKey for a path. O(1)
func (pr *PathRegistry) GetKey(path string) (document.DocumentKey, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	if idx, ok := pr.byPathStr[path]; ok {
		return pr.data[idx].key, true
	}
	return 0, false
}

// GetPath returns the path string for a key. O(1)
func (pr *PathRegistry) GetPath(key document.DocumentKey) (string, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	if idx, ok := pr.byKey[key]; ok {
		return pr.data[idx].path, true
	}
	return "", false
}

func (pm *Schema) Match(re *regexp.Regexp) []document.DocumentKey {
	pm.PathCache.mu.RLock()
	defer pm.PathCache.mu.RUnlock()

	var out []document.DocumentKey
	for _, entry := range pm.PathCache.data {
		if re.MatchString(entry.path) {
			out = append(out, entry.key)
		}
	}
	return out
}

func (pm *Schema) MatchString(pattern string) ([]document.DocumentKey, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return pm.Match(re), nil
}

func (pm *Schema) MatchPrefix(prefix string) []document.DocumentKey {
	pm.PathCache.mu.RLock()
	defer pm.PathCache.mu.RUnlock()

	start := sort.Search(len(pm.PathCache.byPath), func(i int) bool {
		return pm.PathCache.data[pm.PathCache.byPath[i]].path >= prefix
	})

	var out []document.DocumentKey
	for i := start; i < len(pm.PathCache.byPath); i++ {
		entry := pm.PathCache.data[pm.PathCache.byPath[i]]
		if len(entry.path) < len(prefix) || entry.path[:len(prefix)] != prefix {
			break
		}
		out = append(out, entry.key)
	}
	return out
}
