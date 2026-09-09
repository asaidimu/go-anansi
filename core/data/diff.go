package data

import (
	"maps"
	"reflect"
)

// DocumentDiff represents differences between two documents.
//
// Deprecated: Use document.Document.Diff and Apply instead.
type DocumentDiff struct {
	Added    map[string]any       `json:"added"`
	Removed  map[string]any       `json:"removed"`
	Modified map[string]DiffValue `json:"modified"`
}

// DiffValue represents a changed value.
//
// Deprecated: Use document.Document.Diff and Apply instead.
type DiffValue struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// ReservedSystemField checks if a key is a system-managed field that should be ignored during content comparison.
func ReservedSystemField(key string) bool {
	return key == DocumentIDField || key == MetadataField
}

// Diff computes differences between two documents.
//
// Deprecated: Use document.Document.Diff and Apply instead.
func (d *Document) Diff(other Documenter) DocumentDiff {
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
	for k, v := range other.Data() {
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
		if _, ok := other.Data()[k]; !ok {
			diff.Removed[k] = v
		}
	}

	return diff
}

// HasChanges returns true if there are any differences.
//
// Deprecated: Use document.Document.Diff and Apply instead.
func (dd DocumentDiff) HasChanges() bool {
	return len(dd.Added) > 0 || len(dd.Removed) > 0 || len(dd.Modified) > 0
}

// Apply applies the diff to create a new document.
//
// Deprecated: Use document.Document.Diff and Apply instead.
func (d *Document) Apply(diff DocumentDiff) Documenter {
	result := d.Clone().(*Document)

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
