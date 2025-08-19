package data

import "time"

// MustHelper provides panic-based operations
type MustHelper struct {
	doc Document
}

// Must returns a helper for panic-based operations
func (d Document) Must() *MustHelper {
	return &MustHelper{doc: d}
}

func (m *MustHelper) Get(key string) any {
	val, err := m.doc.Get(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetString(key string) string {
	val, err := m.doc.GetString(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetInt(key string) int {
	val, err := m.doc.GetInt(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetFloat64(key string) float64 {
	val, err := m.doc.GetFloat64(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetBool(key string) bool {
	val, err := m.doc.GetBool(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetTime(key string) time.Time {
	val, err := m.doc.GetTime(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetDocument(key string) Document {
	val, err := m.doc.GetDocument(key)
	if err != nil {
		panic(err)
	}
	return val
}

func (m *MustHelper) GetNested(path string) any {
	val, err := m.doc.GetNested(path)
	if err != nil {
		panic(err)
	}
	return val
}

// Generic Must getter
func MustGet[T any](doc Document, key string) T {
	val, err := Get[T](doc, key)
	if err != nil {
		panic(err)
	}
	return val
}
