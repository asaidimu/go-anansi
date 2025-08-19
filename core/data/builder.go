package data

// DocumentBuilder provides a fluent interface for building documents.
type DocumentBuilder struct {
	doc Document
}

// NewDocumentBuilder creates a new document builder.
func NewDocumentBuilder() *DocumentBuilder {
	return &DocumentBuilder{
		doc: make(Document),
	}
}

// Set adds a key-value pair.
func (db *DocumentBuilder) Set(key string, value any) *DocumentBuilder {
	db.doc[key] = value
	return db
}

// SetIf conditionally adds a key-value pair.
func (db *DocumentBuilder) SetIf(condition bool, key string, value any) *DocumentBuilder {
	if condition {
		db.doc[key] = value
	}
	return db
}

// SetNested adds a nested value.
func (db *DocumentBuilder) SetNested(path string, value any) *DocumentBuilder {
	db.doc.SetNested(path, value)
	return db
}

// WithMetadata adds metadata.
func (db *DocumentBuilder) WithMetadata(metadata map[string]any) *DocumentBuilder {
	db.doc.SetMetadata(metadata)
	return db
}

// Build returns the constructed document.
func (db *DocumentBuilder) Build() Document {
	return db.doc.Clone()
}
