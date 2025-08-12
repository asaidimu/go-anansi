package collection

import "github.com/asaidimu/go-anansi/v6/core/data"

func WithMetadata(d map[string]any, metadata map[string]any) map[string]any {
	d[data.MetadataFieldName] = metadata
	return d
}
