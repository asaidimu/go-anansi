package data

import (
	"maps"
	"reflect"
)

// DocumentDiff represents differences between two documents.
type DocumentDiff struct {
	Added    map[string]any       `json:"added"`
	Removed  map[string]any       `json:"removed"`
	Modified map[string]DiffValue `json:"modified"`
}

// DiffValue represents a changed value.
type DiffValue struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// isSystemField checks if a key is a system-managed field that should be ignored during content comparison.
func isSystemField(key string) bool {
	return key == DocumentIDField || key == MetadataField
}

// Diff computes differences between two documents.
func (d *Document) Diff(other *Document) DocumentDiff {
	diff := DocumentDiff{
		Added:    make(map[string]any),
		Removed:  make(map[string]any),
		Modified: make(map[string]DiffValue),
	}

	if d == nil || other == nil {
		// Handle nil documents appropriately, maybe return an empty diff or an error
		return diff
	}

	// Find added and modified
	for k, v := range other.data {
		if isSystemField(k) {
			continue
		}
		if existing, ok := d.data[k]; ok {
			if !reflect.DeepEqual(existing, v) {
				diff.Modified[k] = DiffValue{Old: existing, New: v}
			}
		} else {
			diff.Added[k] = v
		}
	}

	// Find removed
	for k, v := range d.data {
		if isSystemField(k) {
			continue
		}
		if _, ok := other.data[k]; !ok {
			diff.Removed[k] = v
		}
	}

	return diff
}

// HasChanges returns true if there are any differences.
func (dd DocumentDiff) HasChanges() bool {
	return len(dd.Added) > 0 || len(dd.Removed) > 0 || len(dd.Modified) > 0
}

// Apply applies the diff to create a new document.
func (d *Document) Apply(diff DocumentDiff) *Document {
	result := d.Clone()

	// Remove deleted keys
	for k := range diff.Removed {
		delete(result.data, k)
	}

	// Add new keys
	maps.Copy(result.data, diff.Added)

	// Modify changed keys
	for k, v := range diff.Modified {
		result.data[k] = v.New
	}

	return result
}
