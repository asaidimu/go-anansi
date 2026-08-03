package document

import (
	"github.com/asaidimu/go-anansi/v8/core/data"
)

// ============================================================================
// MERGING
// ============================================================================

// Merge copies the top-level data of each of the given documents into d.
// Fields already present in d are overwritten.
func (d *Document) Merge(others ...data.Documenter) {
	if d == nil {
		return
	}
	for _, other := range others {
		if other == nil {
			continue
		}
		for k, v := range other.Data() {
			_ = d.Set(k, v)
		}
	}
}

// DeepMerge recursively merges the data of the given documents into d. Maps at
// any nesting level are merged recursively; all other values are overwritten.
func (d *Document) DeepMerge(others ...data.Documenter) {
	if d == nil {
		return
	}
	if d.isRecord() {
		merged := d.record
		for _, other := range others {
			if other != nil {
				merged = deepMergeMaps(merged, other.Data())
			}
		}
		d.record = merged
		return
	}
	merged := d.Data()
	for _, other := range others {
		if other != nil {
			merged = deepMergeMaps(merged, other.Data())
		}
	}
	d.c.Clear()
	for k, v := range merged {
		_ = d.Set(k, v)
	}
}

func deepMergeMaps(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		if existing, ok := result[k]; ok {
			if existingMap, ok2 := existing.(map[string]any); ok2 {
				if overlayMap, ok3 := v.(map[string]any); ok3 {
					result[k] = deepMergeMaps(existingMap, overlayMap)
					continue
				}
			}
		}
		result[k] = v
	}
	return result
}
