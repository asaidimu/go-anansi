package anansi

import (
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// fieldState is one of the three field states the Document Specification
// defines (spec 2.7): absent from the container, explicitly null, or
// carrying a value.
type fieldState uint8

const (
	stateNotSet fieldState = iota
	stateNull
	stateHasValue
)

// fieldStateOf reports key's state in doc (spec 2.7) in a single positions
// lookup (DataContainer.Position) instead of the two the IsSet/IsNull pair
// costs. Capture-based paths use stateAt on a positionsOf snapshot instead.
func fieldStateOf(doc *container.DataContainer, key container.DataContainerKey) fieldState {
	idx, ok := doc.Position(key)
	if !ok {
		return stateNotSet
	}
	if idx < 0 {
		return stateNull
	}
	return stateHasValue
}

// fillValue reads wf's value bytes from r (spec 2.5) and appends them
// directly into doc via the matching typed Append — the decode-path write
// primitive shared by all four decoders (#dcd004/#dcd005). AppendXxx skips
// the two SetXxx steps that are guaranteed no-ops on a decode target: the
// positions-map probe (the field cannot already be set in a fresh, Clear()ed
// or pool-fresh container) and the hole-claim scan (Clear resets holes; a
// decode target never frees positions mid-fill). The type check AppendXxx
// itself performs is one well-predicted branch on a value the caller just
// dispatched on. The caller is responsible for having already established
// that a value follows (state == HasValue).
func fillValue(r *byteReader, cs *definition.CompiledSchema, doc *container.DataContainer, wf wireField, pool *container.Pool) error {
	switch wf.fd.DataType() {
	case container.TypeInt:
		v, err := readInt(r)
		if err != nil {
			return err
		}
		return doc.AppendInt(wf.key, v)
	case container.TypeFloat:
		v, err := readFloat(r)
		if err != nil {
			return err
		}
		return doc.AppendFloat(wf.key, v)
	case container.TypeString:
		v, err := readString(r)
		if err != nil {
			return err
		}
		return doc.AppendString(wf.key, v)
	case container.TypeBool:
		v, err := readBoolSparse(r)
		if err != nil {
			return err
		}
		return doc.AppendBool(wf.key, v)
	case container.TypeBytes:
		v, err := readBytes(r)
		if err != nil {
			return err
		}
		return doc.AppendBytes(wf.key, v)
	case container.TypeGeometry:
		v, err := readGeometry(r)
		if err != nil {
			return err
		}
		return doc.AppendGeometry(wf.key, v)
	case container.TypeRecord:
		v, err := readRecord(r)
		if err != nil {
			return err
		}
		return doc.AppendRecord(wf.key, v)
	case container.TypeUnknown:
		v, err := readAny(r)
		if err != nil {
			return err
		}
		return doc.AppendUnknown(wf.key, v)
	case container.TypeArrayInt:
		v, err := readArrayInt(r)
		if err != nil {
			return err
		}
		return doc.AppendArrayInt(wf.key, v)
	case container.TypeArrayFloat:
		v, err := readArrayFloat(r)
		if err != nil {
			return err
		}
		return doc.AppendArrayFloat(wf.key, v)
	case container.TypeArrayString:
		v, err := readArrayString(r)
		if err != nil {
			return err
		}
		return doc.AppendArrayString(wf.key, v)
	case container.TypeArrayBool:
		v, err := readArrayBool(r)
		if err != nil {
			return err
		}
		return doc.AppendArrayBool(wf.key, v)
	case container.TypeArrayBytes:
		v, err := readArrayBytes(r)
		if err != nil {
			return err
		}
		return doc.AppendArrayBytes(wf.key, v)
	case container.TypeArrayGeometry:
		v, err := readArrayGeometry(r)
		if err != nil {
			return err
		}
		return doc.AppendArrayGeometry(wf.key, v)
	case container.TypeArrayUnknown:
		n, err := r.readUvarint()
		if err != nil {
			return err
		}
		// Each element leads with its type tag (spec 9.2.1: validate
		// length fields before allocation).
		if err := r.checkElems(n, 1); err != nil {
			return err
		}
		v := make([]any, 0, n)
		for i := uint64(0); i < n; i++ {
			e, err := readAny(r)
			if err != nil {
				return err
			}
			v = append(v, e)
		}
		return doc.AppendArrayUnknown(wf.key, v)
	case container.TypeArrayObject:
		v, err := readArrayObjectField(r, wf.child, pool)
		if err != nil {
			return err
		}
		return doc.AppendArrayObject(wf.key, v)
	default:
		return fmt.Errorf("anansi: unsupported data type %d for field %q", wf.fd.DataType(), wf.name)
	}
}
