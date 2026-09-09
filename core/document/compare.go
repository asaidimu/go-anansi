package document

import (
	"reflect"

	"github.com/asaidimu/go-anansi/v8/core/data"
)

// ============================================================================
// COMPARISON / DIFFING
// ============================================================================

// Is reports whether other is identical to d: same id, data and metadata.
func (d *Document) Is(other data.Documenter) bool {
	if other == nil {
		return false
	}
	return d.ID() == other.ID() &&
		reflect.DeepEqual(d.Data(), other.Data()) &&
		reflect.DeepEqual(d.Metadata(), other.Metadata())
}

// Equals reports whether other has the same data as d, ignoring id and metadata.
func (d *Document) Equals(other data.Documenter) bool {
	if other == nil {
		return false
	}
	return reflect.DeepEqual(d.Data(), other.Data())
}

// Diff computes the difference between d and other. The returned diff
// expresses how other differs from d (additions/modifications/removals).
func (d *Document) Diff(other data.Documenter) data.DocumentDiff {
	diff := data.DocumentDiff{
		Added:    map[string]any{},
		Removed:  map[string]any{},
		Modified: map[string]data.DiffValue{},
	}
	if d == nil || other == nil {
		return diff
	}
	otherData := other.Data()
	myData := d.Data()
	for k, v := range otherData {
		if existing, ok := myData[k]; ok {
			if !reflect.DeepEqual(existing, v) {
				diff.Modified[k] = data.DiffValue{Old: existing, New: v}
			}
		} else {
			diff.Added[k] = v
		}
	}
	for k, v := range myData {
		if _, ok := otherData[k]; !ok {
			diff.Removed[k] = v
		}
	}
	return diff
}

// Apply returns a copy of d with the diff applied. It never mutates d.
func (d *Document) Apply(diff data.DocumentDiff) data.Documenter {
	if d == nil {
		return nil
	}
	result := d.Clone().(*Document)
	for k := range diff.Removed {
		result.Unset(k)
	}
	for k, v := range diff.Added {
		_ = result.Set(k, v)
	}
	for k, v := range diff.Modified {
		_ = result.Set(k, v.New)
	}
	return result
}
