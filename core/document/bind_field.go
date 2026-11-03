package document

import (
	"reflect"

	"github.com/asaidimu/go-anansi/v8/core/schema/binding"
)

// BindField implements the binding.TypedBinder interface for container-backed
// documents. It copies a schema field's value straight from the container into
// rv without boxing the value through an interface (any). The slot machinery
// lives in core/schema/binding (ContainerSource); this method only adds the
// document-specific guards: nil/schema checks, record views, and reserved
// system fields, which route through the generic Get path instead.
func (d *Document) BindField(name string, rv reflect.Value, tag string) (bool, error) {
	if d == nil || d.cs == nil || d.c == nil || d.isRecord() || reservedBindField(name) {
		return false, nil
	}
	if !rv.IsValid() || !rv.CanSet() {
		return false, nil
	}
	src := &binding.ContainerSource{
		CS:      d.cs,
		C:       d.c,
		Prefix:  d.prefix,
		SlotIdx: d.slotIdx,
		Ctx:     d.ctx,
	}
	return src.BindField(name, rv, tag)
}
