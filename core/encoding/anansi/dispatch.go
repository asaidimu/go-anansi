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

// readFieldPayload reads wf's value bytes from r (spec 2.5) and writes them
// into doc via the matching typed setter. The caller is responsible for
// having already established that a value follows (state == HasValue).
func readFieldPayload(r *byteReader, cs *definition.CompiledSchema, doc *container.DataContainer, wf wireField, pool *container.Pool) error {
	switch wf.fd.DataType() {
	case container.TypeInt:
		v, err := readInt(r)
		if err != nil {
			return err
		}
		return doc.SetInt(wf.key, v)
	case container.TypeFloat:
		v, err := readFloat(r)
		if err != nil {
			return err
		}
		return doc.SetFloat(wf.key, v)
	case container.TypeString:
		v, err := readString(r)
		if err != nil {
			return err
		}
		return doc.SetString(wf.key, v)
	case container.TypeBool:
		v, err := readBoolSparse(r)
		if err != nil {
			return err
		}
		return doc.SetBool(wf.key, v)
	case container.TypeBytes:
		v, err := readBytes(r)
		if err != nil {
			return err
		}
		return doc.SetBytes(wf.key, v)
	case container.TypeGeometry:
		v, err := readGeometry(r)
		if err != nil {
			return err
		}
		return doc.SetGeometry(wf.key, v)
	case container.TypeRecord:
		v, err := readRecord(r)
		if err != nil {
			return err
		}
		return doc.SetRecord(wf.key, v)
	case container.TypeUnknown:
		v, err := readAny(r)
		if err != nil {
			return err
		}
		return doc.SetUnknown(wf.key, v)
	case container.TypeArrayInt:
		v, err := readArrayInt(r)
		if err != nil {
			return err
		}
		return doc.SetArrayInt(wf.key, v)
	case container.TypeArrayFloat:
		v, err := readArrayFloat(r)
		if err != nil {
			return err
		}
		return doc.SetArrayFloat(wf.key, v)
	case container.TypeArrayString:
		v, err := readArrayString(r)
		if err != nil {
			return err
		}
		return doc.SetArrayString(wf.key, v)
	case container.TypeArrayBool:
		v, err := readArrayBool(r)
		if err != nil {
			return err
		}
		return doc.SetArrayBool(wf.key, v)
	case container.TypeArrayBytes:
		v, err := readArrayBytes(r)
		if err != nil {
			return err
		}
		return doc.SetArrayBytes(wf.key, v)
	case container.TypeArrayGeometry:
		v, err := readArrayGeometry(r)
		if err != nil {
			return err
		}
		return doc.SetArrayGeometry(wf.key, v)
	case container.TypeArrayUnknown:
		n, err := r.readUvarint()
		if err != nil {
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
		return doc.SetArrayUnknown(wf.key, v)
	case container.TypeArrayObject:
		v, err := readArrayObjectField(r, wf.child, pool)
		if err != nil {
			return err
		}
		return doc.SetArrayObject(wf.key, v)
	default:
		return fmt.Errorf("anansi: unsupported data type %d for field %q", wf.fd.DataType(), wf.name)
	}
}
