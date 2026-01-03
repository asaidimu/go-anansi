package data

import "time"

// MustHelper provides panic-based operations
type MustHelper struct {
	doc *Document
}

// Must returns a helper for panic-based operations
func (d *Document) Must() *MustHelper {
	return &MustHelper{doc: d}
}

// Get retrieves a value (direct key only), panics if not found
func (m *MustHelper) Get(key string) any {
	val, err := m.doc.Get(key)
	if err != nil {
		panic(err)
	}
	return val
}

// GetString retrieves a string value with path support, panics if not found or not convertible
func (m *MustHelper) GetString(keyOrPath string) string {
	val, err := m.doc.GetString(keyOrPath)
	if err != nil {
		panic(err)
	}
	return val
}

// GetInt retrieves an int value with path support, panics if not found or not convertible
func (m *MustHelper) GetInt(keyOrPath string) int {
	val, err := m.doc.GetInt(keyOrPath)
	if err != nil {
		panic(err)
	}
	return val
}

// GetFloat64 retrieves a float64 value with path support, panics if not found or not convertible
func (m *MustHelper) GetFloat64(keyOrPath string) float64 {
	val, err := m.doc.GetFloat64(keyOrPath)
	if err != nil {
		panic(err)
	}
	return val
}

// GetBool retrieves a bool value with path support, panics if not found or not convertible
func (m *MustHelper) GetBool(keyOrPath string) bool {
	val, err := m.doc.GetBool(keyOrPath)
	if err != nil {
		panic(err)
	}
	return val
}

// GetTime retrieves a time.Time value with path support, panics if not found or not convertible
func (m *MustHelper) GetTime(keyOrPath string) time.Time {
	val, err := m.doc.GetTime(keyOrPath)
	if err != nil {
		panic(err)
	}
	return val
}

// GetDocument retrieves a *Document with path support, panics if not found or not convertible
func (m *MustHelper) GetDocument(keyOrPath string) *Document {
	val, err := m.doc.GetDocument(keyOrPath)
	if err != nil {
		panic(err)
	}
	return val
}

// GetDocumentArray retrieves a []*Document with path support, panics if not found or not convertible
func (m *MustHelper) GetDocumentArray(keyOrPath string) []*Document {
	val, err := m.doc.GetDocumentArray(keyOrPath)
	if err != nil {
		panic(err)
	}
	return val
}

// --- New Slice Helpers ---

// GetStringArray retrieves a []string with path support, panics if not found or not convertible
func (m *MustHelper) GetStringArray(keyOrPath string) []string {
	val, err := m.doc.GetStringArray(keyOrPath)
	if err != nil {
		panic(err)
	}
	return val
}

// GetIntArray retrieves a []int with path support, panics if not found or not convertible
func (m *MustHelper) GetIntArray(keyOrPath string) []int {
	val, err := m.doc.GetIntArray(keyOrPath)
	if err != nil {
		panic(err)
	}
	return val
}

// GetArray retrieves a []any with path support, panics if not found
func (m *MustHelper) GetArray(keyOrPath string) []any {
	val, err := m.doc.GetArray(keyOrPath)
	if err != nil {
		panic(err)
	}
	return val
}

// Generic Must getter with type parameter
// Note: This assumes a top-level Get[T] function exists in your package
func MustGet[T any](doc *Document, key string) T {
	val, err := Get[T](doc, key)
	if err != nil {
		panic(err)
	}
	return val
}
