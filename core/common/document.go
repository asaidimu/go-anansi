package common

// Document represents a single document or row of data.
type Document map[string]any

type DocumentLike interface {
	~map[string]any
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


func (doc Document) SetMetadata(metadata map[string]any)  {
	doc[MetadataFieldName] = metadata
}

func (doc Document) StripMetadata()  {
	delete(doc,MetadataFieldName)
}

