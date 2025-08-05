package common

import (
	"unsafe"
)

// Document represents a single document or row of data.
type Document map[string]any

type DocumentLike interface {
	~map[string]any
}

// NewDocument converts any DocumentLike type to Document without copying
func NewDocument[T DocumentLike](data T) (Document, bool) {
	if data == nil {
		return nil, false
	}

	// Use unsafe pointer conversion since all DocumentLike types
	// have the same underlying representation as Document
	result := *(*Document)(unsafe.Pointer(&data))
	return result, true
}

// Alternative constructor that panics on nil input
func MustNewDocument[T DocumentLike](data T) Document {
	doc, ok := NewDocument(data)
	if !ok {
		panic("cannot create Document from nil data")
	}
	return doc
}

const MetadataFieldName = "_metadata_"

func (doc Document) Metadata() (map[string]any, bool) {
	data, ok := doc[MetadataFieldName]
	if !ok {
		return nil, ok
	}

	if result, ok := data.(map[string]any); ok {
		return result, ok
	}

	return nil, false
}

func (doc Document) SetMetadata(metadata map[string]any) {
	doc[MetadataFieldName] = metadata
}

func (doc Document) StripMetadata() Document {
	if doc == nil {
		return nil
	}

	// Create new map without metadata
	cleaned := make(Document)
	for key, value := range doc {
		if key == MetadataFieldName {
			continue
		}
		cleaned[key] = value
	}
	return cleaned
}

