package document

import (
	"fmt"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/utils"
)

// isReservedMetadataKey mirrors data's reserved system metadata fields.
func isReservedMetadataKey(key string) bool {
	switch key {
	case data.MetadataCreated, data.MetadataUpdated, data.MetadataVersion, data.MetadataChecksum, data.MetadataSignature:
		return true
	default:
		return false
	}
}

// clearMetadata removes every leaf of the _metadata_ object from the container.
func (d *Document) clearMetadata() error {
	if d == nil || d.cs == nil || d.c == nil {
		return nil
	}
	rp, fd, err := d.resolvePath(data.MetadataField)
	if err != nil {
		return err
	}
	if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
		return unsetSubtree(d.cs, d.c, fd.ChildSchemaIdx(), rp)
	}
	return nil
}

// populateMetadata replaces the metadata object's contents from a map. Keys
// not declared in the metadata schema are dropped (there is no slot to store
// them); declared keys are validated against their slot types.
func (d *Document) populateMetadata(m map[string]any) error {
	if d == nil || d.cs == nil || d.c == nil {
		return nil
	}
	if err := d.clearMetadata(); err != nil {
		return err
	}
	for k, v := range m {
		if _, _, err := d.resolvePath(data.MetadataFieldPath(k)); err != nil {
			continue // undeclared metadata key — no slot to store it
		}
		if err := d.setMetadataValue(k, v, true); err != nil {
			return err
		}
	}
	return nil
}

// setMetadataValue writes a single metadata field into the _metadata_ object.
// When allowReserved is false, system-managed fields are read-only.
func (d *Document) setMetadataValue(key string, value any, allowReserved bool) error {
	if d == nil {
		return common.SystemErrorFrom(data.ErrNoMetadata).WithOperation("document.metadata")
	}
	if d.isRecord() {
		if !allowReserved && isReservedMetadataKey(key) {
			return d.readonlyErr(key)
		}
		meta, ok := d.record[data.MetadataField].(map[string]any)
		if !ok {
			meta = map[string]any{}
			d.record[data.MetadataField] = meta
		}
		meta[key] = value
		return nil
	}
	if d.cs == nil || d.c == nil {
		return common.SystemErrorFrom(data.ErrNoMetadata).WithOperation("document.metadata")
	}
	if !allowReserved && isReservedMetadataKey(key) {
		return d.readonlyErr(key)
	}
	rp, _, err := d.resolvePath(data.MetadataFieldPath(key))
	if err != nil {
		return d.keyErr(key)
	}
	last := rp[len(rp)-1]
	if err := setInto(d.cs, d.c, d.pool, last.SchemaIdx(), uint16(last.FieldIdx()), rp[:len(rp)-1], value); err != nil {
		return common.SystemErrorFrom(data.ErrInvalidMetadata).
			WithOperation("document.metadata").WithPath(key).WithCause(err)
	}
	return nil
}

// getMetadataValue reads a single metadata field from the _metadata_ object.
func (d *Document) getMetadataValue(key string) (any, error) {
	if d == nil {
		return nil, common.SystemErrorFrom(data.ErrNoMetadata).WithOperation("document.metadata")
	}
	if d.isRecord() {
		val, ok := utils.GetValueByPath(d.record, data.MetadataFieldPath(key))
		if !ok {
			return nil, d.keyErr(data.MetadataFieldPath(key))
		}
		return val, nil
	}
	if d.cs == nil || d.c == nil {
		return nil, common.SystemErrorFrom(data.ErrNoMetadata).WithOperation("document.metadata")
	}
	return d.Get(data.MetadataFieldPath(key))
}

// metadataKeySet reports whether a metadata field has been set.
func (d *Document) metadataKeySet(key string) bool {
	if d == nil {
		return false
	}
	if d.isRecord() {
		_, ok := utils.GetValueByPath(d.record, data.MetadataFieldPath(key))
		return ok
	}
	if d.cs == nil || d.c == nil {
		return false
	}
	return d.HasKey(data.MetadataFieldPath(key))
}

// materializeMetadata renders the _metadata_ object as a map.
func (d *Document) materializeMetadata() map[string]any {
	if d == nil {
		return nil
	}
	if d.isRecord() {
		if m, ok := d.record[data.MetadataField]; ok {
			if mm, ok := m.(map[string]any); ok {
				return mm
			}
		}
		return nil
	}
	if d.cs == nil || d.c == nil {
		return nil
	}
	m, err := d.Get(data.MetadataField)
	if err != nil {
		return nil
	}
	mm, ok := m.(map[string]any)
	if !ok {
		return nil
	}
	return mm
}

// ============================================================================
// Public Metadata API
// ============================================================================

// Metadata returns a copy of the document's metadata map.
func (d *Document) Metadata() map[string]any {
	return d.materializeMetadata()
}

// SetMetadata replaces the entire metadata map. Keys not declared by the
// metadata schema are dropped.
func (d *Document) SetMetadata(metadata map[string]any) {
	if d == nil || d.cs == nil || d.c == nil {
		return
	}
	if metadata == nil {
		_ = d.clearMetadata()
		return
	}
	_ = d.populateMetadata(metadata)
}

// SetMetadataValue sets a custom metadata field. Reserved system fields
// (version, created, updated, checksum, signature) are read-only, and the key
// must be declared by a metadata provider schema.
func (d *Document) SetMetadataValue(key string, value any) error {
	return d.setMetadataValue(key, value, false)
}

// GetMetadataValue retrieves a metadata field.
func (d *Document) GetMetadataValue(key string) (any, error) {
	return d.getMetadataValue(key)
}

// GetMetadataString returns a metadata value coerced to string.
func (d *Document) GetMetadataString(key string) (string, error) {
	val, err := d.GetMetadataValue(key)
	if err != nil {
		return "", err
	}
	str, ok := utils.CoerceToPrimitiveValue[string](val)
	if !ok {
		return "", common.SystemErrorFrom(data.ErrTypeConversion).WithOperation("document.metadata").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to string", val))
	}
	return str, nil
}

// GetMetadataInt returns a metadata value coerced to int.
func (d *Document) GetMetadataInt(key string) (int, error) {
	val, err := d.GetMetadataValue(key)
	if err != nil {
		return 0, err
	}
	num, ok := utils.CoerceToPrimitiveValue[int](val)
	if !ok {
		return 0, common.SystemErrorFrom(data.ErrTypeConversion).WithOperation("document.metadata").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to int", val))
	}
	return num, nil
}

// GetMetadataFloat returns a metadata value coerced to float64.
func (d *Document) GetMetadataFloat(key string) (float64, error) {
	val, err := d.GetMetadataValue(key)
	if err != nil {
		return 0, err
	}
	num, ok := utils.CoerceToPrimitiveValue[float64](val)
	if !ok {
		return 0, common.SystemErrorFrom(data.ErrTypeConversion).WithOperation("document.metadata").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to float64", val))
	}
	return num, nil
}

// GetMetadataBool returns a metadata value coerced to bool.
func (d *Document) GetMetadataBool(key string) (bool, error) {
	val, err := d.GetMetadataValue(key)
	if err != nil {
		return false, err
	}
	boolean, ok := utils.CoerceToPrimitiveValue[bool](val)
	if !ok {
		return false, common.SystemErrorFrom(data.ErrTypeConversion).WithOperation("document.metadata").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to bool", val))
	}
	return boolean, nil
}

// GetMetadataTime returns a metadata value coerced to time.Time.
func (d *Document) GetMetadataTime(key string) (time.Time, error) {
	val, err := d.GetMetadataValue(key)
	if err != nil {
		return time.Time{}, err
	}
	t, ok := utils.CoerceTime(val)
	if !ok {
		return time.Time{}, common.SystemErrorFrom(data.ErrTypeConversion).WithOperation("document.metadata").WithPath(key).WithMessage(fmt.Sprintf("cannot convert %T to time.Time", val))
	}
	return t, nil
}

// Version returns the document version.
func (d *Document) Version() (int, error) {
	val, err := d.GetMetadataValue(data.MetadataVersion)
	if err != nil {
		return 0, err
	}
	version, ok := utils.CoerceToPrimitiveValue[int](val)
	if !ok {
		return 0, common.SystemErrorFrom(data.ErrTypeConversion).WithOperation("document.metadata").WithPath(data.MetadataVersion).WithMessage(fmt.Sprintf("cannot convert %T to int", val))
	}
	return version, nil
}

// Checksum returns the document's checksum.
func (d *Document) Checksum() (string, error) {
	val, err := d.GetMetadataValue(data.MetadataChecksum)
	if err != nil {
		return "", err
	}
	checksum, ok := val.(string)
	if !ok {
		return "", common.SystemErrorFrom(data.ErrTypeConversion).WithOperation("document.metadata").WithPath(data.MetadataChecksum).WithMessage(fmt.Sprintf("cannot convert %T to string", val))
	}
	return checksum, nil
}

// Signature returns the document's signature.
func (d *Document) Signature() (string, error) {
	val, err := d.GetMetadataValue(data.MetadataSignature)
	if err != nil {
		return "", err
	}
	signature, ok := val.(string)
	if !ok {
		return "", common.SystemErrorFrom(data.ErrTypeConversion).WithOperation("document.metadata").WithPath(data.MetadataSignature).WithMessage(fmt.Sprintf("cannot convert %T to string", val))
	}
	return signature, nil
}

// CreatedAt returns the document creation timestamp.
func (d *Document) CreatedAt() (time.Time, error) {
	val, err := d.GetMetadataValue(data.MetadataCreated)
	if err != nil {
		return time.Time{}, err
	}
	createdAt, ok := utils.CoerceTime(val)
	if !ok {
		return time.Time{}, common.SystemErrorFrom(data.ErrTypeConversion).WithOperation("document.metadata").WithPath(data.MetadataCreated).WithMessage(fmt.Sprintf("cannot convert %T to time.Time", val))
	}
	return createdAt, nil
}

// UpdatedAt returns the document last-update timestamp.
func (d *Document) UpdatedAt() (time.Time, error) {
	val, err := d.GetMetadataValue(data.MetadataUpdated)
	if err != nil {
		return time.Time{}, err
	}
	updatedAt, ok := utils.CoerceTime(val)
	if !ok {
		return time.Time{}, common.SystemErrorFrom(data.ErrTypeConversion).WithOperation("document.metadata").WithPath(data.MetadataUpdated).WithMessage(fmt.Sprintf("cannot convert %T to time.Time", val))
	}
	return updatedAt, nil
}
