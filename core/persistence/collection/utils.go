package collection

import "github.com/asaidimu/go-anansi/v6/core/common"

func WithMetadata(data map[string]any, metadata map[string]any) map[string]any {
	data[common.MetadataFieldName] = metadata
	return data
}
