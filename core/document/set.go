package document

import (
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// setLeaf writes a terminal leaf field directly to its container slot. Type
// safety is enforced by construction: the schema compiles every field to a
// fixed DataType and the storage key embeds that type, so writing a slot with a
// non-matching setter fails at the container. A mismatched setter is a
// call-site error and returns a type error immediately.
//
// Record views are map-backed; the typed value is stored directly.
func (d *Document) setLeaf(keyOrPath string, value any, want container.DataType, write func(container.DataContainerKey) error) error {
	if d == nil {
		return d.keyErr(keyOrPath)
	}
	if data.ReservedSystemField(keyOrPath) {
		return d.readonlyErr(keyOrPath)
	}
	if keyOrPath == "" {
		return d.keyEmptyErr()
	}
	if d.isRecord() {
		return setValueByPath(d.record, keyOrPath, value)
	}
	rp, fd, err := d.resolvePath(keyOrPath)
	if err != nil {
		return err
	}
	if fd.DataType() != want {
		return d.typeErr(keyOrPath, dtName(want), dtName(fd.DataType()))
	}
	if !fd.Terminal() || fd.ChildSchemaIdx() != definition.FdNoChild {
		return d.typeErr(keyOrPath, dtName(want), "non-terminal field")
	}
	k, err := computeLeafKey(d.cs, fd, rp)
	if err != nil {
		return err
	}
	return write(k)
}

// SetString writes a string value to a field schema-declared as a string.
func (d *Document) SetString(keyOrPath string, value string) error {
	return d.setLeaf(keyOrPath, value, container.TypeString, func(k container.DataContainerKey) error {
		return d.c.SetString(k, value)
	})
}

// SetInt writes an integer value to a field schema-declared as an integer.
func (d *Document) SetInt(keyOrPath string, value int) error {
	return d.setLeaf(keyOrPath, value, container.TypeInt, func(k container.DataContainerKey) error {
		return d.c.SetInt(k, int64(value))
	})
}

// SetFloat64 writes a number value to a field schema-declared as a number.
func (d *Document) SetFloat64(keyOrPath string, value float64) error {
	return d.setLeaf(keyOrPath, value, container.TypeFloat, func(k container.DataContainerKey) error {
		return d.c.SetFloat(k, value)
	})
}

// SetBool writes a boolean value to a field schema-declared as a boolean.
func (d *Document) SetBool(keyOrPath string, value bool) error {
	return d.setLeaf(keyOrPath, value, container.TypeBool, func(k container.DataContainerKey) error {
		return d.c.SetBool(k, value)
	})
}

// SetStringArray writes an array of strings to a field schema-declared as an
// array of strings.
func (d *Document) SetStringArray(keyOrPath string, value []string) error {
	return d.setLeaf(keyOrPath, value, container.TypeArrayString, func(k container.DataContainerKey) error {
		return d.c.SetArrayString(k, value)
	})
}

// SetIntArray writes an array of integers to a field schema-declared as an
// array of integers.
func (d *Document) SetIntArray(keyOrPath string, value []int) error {
	return d.setLeaf(keyOrPath, value, container.TypeArrayInt, func(k container.DataContainerKey) error {
		arr := make([]int64, len(value))
		for i, n := range value {
			arr[i] = int64(n)
		}
		return d.c.SetArrayInt(k, arr)
	})
}
