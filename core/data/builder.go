package data

// DocumentBuilder provides a fluent interface for building documents.
type DocumentBuilder struct {
	doc Document
}

// NewDocumentBuilder creates a new document builder.
func NewDocumentBuilder() *DocumentBuilder {
	return &DocumentBuilder{
		doc: MustNewDocument(nil),
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
func (db *DocumentBuilder) SetNested(path string, value any) (*DocumentBuilder, error) {
	if err := db.doc.SetNested(path, value); err != nil {
		return nil, err
	}
	return db, nil
}

// WithMetadata adds metadata.
func (db *DocumentBuilder) WithMetadata(metadata map[string]any) (*DocumentBuilder, error) {
	meta, ok := db.doc.Metadata()
	if !ok {
		return nil, ErrNoMetadata
	}
	set := make(map[string]any, 0)
	for key, value := range(metadata) {
		if err := db.doc.SetMetadataValue(key, value); err != nil {
			db.doc.SetMetadata(meta)
			return nil, err
		}
		set[key] = value
	}
	return db, nil
}

// Build returns the constructed document.
func (db *DocumentBuilder) Build() Document {
	return db.doc.Clone()
}
