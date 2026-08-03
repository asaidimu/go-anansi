package document

import (
	"github.com/asaidimu/go-anansi/v8/core/data"
)

// ============================================================================
// NORMALIZATION
// ============================================================================

// Normalize returns a copy of d with the system metadata fields (_id_ and
// _metadata_) stripped from nested maps, mirroring data.Document.Normalize.
func (d *Document) Normalize() data.Documenter {
	if d == nil {
		return nil
	}
	if d.isRecord() {
		return newRecordView(stripNestedMetadata(d.record).(map[string]any), d.ctx)
	}
	out := &Document{
		cs:   d.cs,
		c:    newContainerFor(d),
		pool: d.pool,
		ctx:  d.ctx,
	}
	for k, v := range stripNestedMetadata(d.Data()).(map[string]any) {
		_ = out.Set(k, v)
	}
	out.setID(d.ID())
	out.SetMetadata(d.Metadata())
	return out
}

// stripNestedMetadata recursively removes reserved system fields from nested
// maps. The root map is left untouched so the outer _id_/_metadata_ survive.
func stripNestedMetadata(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, v2 := range val {
			if data.ReservedSystemField(k) {
				continue
			}
			out[k] = stripNestedMetadata(v2)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = stripNestedMetadata(item)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(val))
		for i, m := range val {
			out[i] = stripNestedMetadata(m).(map[string]any)
		}
		return out
	default:
		return v
	}
}
