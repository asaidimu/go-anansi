package data

import (
	"maps"
	"reflect"
)

// Diff computes differences between two documents.
func (d Document) Diff(other Document) DocumentDiff {
	diff := DocumentDiff{
		Added:    make(map[string]any),
		Removed:  make(map[string]any),
		Modified: make(map[string]DiffValue),
	}

	// Find added and modified
	for k, v := range other {
		if existing, ok := d[k]; ok {
			if !reflect.DeepEqual(existing, v) {
				diff.Modified[k] = DiffValue{Old: existing, New: v}
			}
		} else {
			diff.Added[k] = v
		}
	}

	// Find removed
	for k, v := range d {
		if _, ok := other[k]; !ok {
			diff.Removed[k] = v
		}
	}

	return diff
}

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

// HasChanges returns true if there are any differences.
func (dd DocumentDiff) HasChanges() bool {
	return len(dd.Added) > 0 || len(dd.Removed) > 0 || len(dd.Modified) > 0
}

// Apply applies the diff to create a new document.
func (d Document) Apply(diff DocumentDiff) Document {
	result := d.Clone()

	// Remove deleted keys
	for k := range diff.Removed {
		delete(result, k)
	}

	// Add new keys
	maps.Copy(result, diff.Added)

	// Modify changed keys
	for k, v := range diff.Modified {
		result[k] = v.New
	}

	return result
}
