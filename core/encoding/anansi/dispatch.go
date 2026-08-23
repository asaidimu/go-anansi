package anansi

import (
	"bytes"
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

// fieldStateOf reports key's state in doc (spec 2.7).
func fieldStateOf(doc *container.DataContainer, key container.DataContainerKey) fieldState {
	if !doc.IsSet(key) {
		return stateNotSet
	}
	if doc.IsNull(key) {
		return stateNull
	}
	return stateHasValue
}

// writeFieldPayload writes wf's value bytes (spec 2.5, per DataType) from
// doc into buf. The caller is responsible for having already established
// (via fieldStateOf) that the field carries a value; this function does not
// re-check state.
func writeFieldPayload(buf *bytes.Buffer, cs *definition.CompiledSchema, version header, doc *container.DataContainer, wf wireField) error {
	switch wf.fd.DataType() {
	case container.TypeInt:
		v, _, err := doc.GetInt(wf.key)
		if err != nil {
			return err
		}
		writeInt(buf, v)
	case container.TypeFloat:
		v, _, err := doc.GetFloat(wf.key)
		if err != nil {
			return err
		}
		writeFloat(buf, v)
	case container.TypeString:
		v, _, err := doc.GetString(wf.key)
		if err != nil {
			return err
		}
		writeString(buf, v)
	case container.TypeBool:
		v, _, err := doc.GetBool(wf.key)
		if err != nil {
			return err
		}
		writeBoolSparse(buf, v)
	case container.TypeBytes:
		v, _, err := doc.GetBytes(wf.key)
		if err != nil {
			return err
		}
		writeBytes(buf, v)
	case container.TypeGeometry:
		v, _, err := doc.GetGeometry(wf.key)
		if err != nil {
			return err
		}
		writeGeometry(buf, v)
	case container.TypeRecord:
		v, _, err := doc.GetRecord(wf.key)
		if err != nil {
			return err
		}
		return writeRecord(buf, v)
	case container.TypeUnknown:
		v, _, err := doc.GetUnknown(wf.key)
		if err != nil {
			return err
		}
		return writeAny(buf, v)
	case container.TypeArrayInt:
		v, _, err := doc.GetArrayInt(wf.key)
		if err != nil {
			return err
		}
		writeArrayInt(buf, v)
	case container.TypeArrayFloat:
		v, _, err := doc.GetArrayFloat(wf.key)
		if err != nil {
			return err
		}
		writeArrayFloat(buf, v)
	case container.TypeArrayString:
		v, _, err := doc.GetArrayString(wf.key)
		if err != nil {
			return err
		}
		writeArrayString(buf, v)
	case container.TypeArrayBool:
		v, _, err := doc.GetArrayBool(wf.key)
		if err != nil {
			return err
		}
		writeArrayBool(buf, v)
	case container.TypeArrayBytes:
		v, _, err := doc.GetArrayBytes(wf.key)
		if err != nil {
			return err
		}
		writeArrayBytes(buf, v)
	case container.TypeArrayGeometry:
		v, _, err := doc.GetArrayGeometry(wf.key)
		if err != nil {
			return err
		}
		writeArrayGeometry(buf, v)
	case container.TypeArrayUnknown:
		v, _, err := doc.GetArrayUnknown(wf.key)
		if err != nil {
			return err
		}
		writeUvarintTo(buf, uint64(len(v)))
		for _, e := range v {
			if err := writeAny(buf, e); err != nil {
				return err
			}
		}
	case container.TypeArrayObject:
		v, _, err := doc.GetArrayObject(wf.key)
		if err != nil {
			return err
		}
		return writeArrayObjectField(buf, cs, version, v, wf.childIdx, wf.childPath)
	default:
		return fmt.Errorf("anansi: unsupported data type %d for field %q", wf.fd.DataType(), wf.name)
	}
	return nil
}

// readFieldPayload reads wf's value bytes from r (spec 2.5) and writes them
// into doc via the matching typed setter. The caller is responsible for
// having already established that a value follows (state == HasValue).
func readFieldPayload(r *byteReader, cs *definition.CompiledSchema, version header, doc *container.DataContainer, wf wireField, pool *container.Pool) error {
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
		v, err := readArrayObjectField(r, cs, wf.childIdx, wf.childPath, pool)
		if err != nil {
			return err
		}
		return doc.SetArrayObject(wf.key, v)
	default:
		return fmt.Errorf("anansi: unsupported data type %d for field %q", wf.fd.DataType(), wf.name)
	}
}
