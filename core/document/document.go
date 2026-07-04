package document

import (
	"fmt"
	"unsafe"
)

// DocumentKey is a 64-bit key encoding both a DataPoint and a field descriptor.
//
// Layout:
//   bits 63–32: field descriptor (uint32) — type, owner_schema, field_index, flags
//   bits 31–0:  DataPoint (int32)          — null flag, DataType, 27-bit ordinal
//
// This makes a Document self-describing: from a single key the holder can both
// look up the field value (via the DataPoint half) and evaluate all field-level
// rules (via the descriptor half) without any secondary lookup.
type DocumentKey int64

// NewDocumentKey constructs a DocumentKey from a DataPoint and a field descriptor.
func NewDocumentKey(dp DataPoint, descriptor uint32) DocumentKey {
	return DocumentKey(uint64(descriptor)<<32 | uint64(uint32(dp)))
}

// DataPoint extracts the DataPoint (low 32 bits) from a DocumentKey.
func (k DocumentKey) DataPoint() DataPoint {
	return DataPoint(int32(k))
}

// Descriptor extracts the field descriptor (high 32 bits) from a DocumentKey.
func (k DocumentKey) Descriptor() uint32 {
	return uint32(uint64(k) >> 32)
}

// Type extracts the DataType from the embedded DataPoint.
func (k DocumentKey) Type() DataType {
	return k.DataPoint().Type()
}

// IsNull returns true if the null bit is set on the embedded DataPoint.
func (k DocumentKey) IsNull() bool {
	return k.DataPoint().IsNull()
}

// Document is a self-describing, type-indexed, poolable, sparse data container
// keyed by DocumentKey (64-bit) rather than DataPoint (32-bit).
//
// Each key carries the field descriptor alongside the DataPoint, making it
// suitable for validation without any secondary schema lookups.

// data[i] holds a pointer to the slice header for DataType(i), lazily initialised.
// The pointer is to the header (*[]T), not the backing array, so it survives appends.
//
// positions maps int32(DataPoint) → slice index within the typed slice.
// A value of -1 means the field is explicitly null (present but valueless).
// Absence from the map means the field has never been set.
//
// holes tracks freed slice positions available for reuse, encoded as DataPoints
// where the ID field holds the freed slice index.
type Document struct {
	data      [16]unsafe.Pointer
	positions map[int64]int32
	holes     []DocumentKey
}

func NewDocument() *Document {
	return &Document{
		positions: make(map[int64]int32),
	}
}

// initSlice allocates a new typed slice for the given DataType and stores
// a pointer to its header in data[typ]. Called lazily on first write.
func (d *Document) initSlice(typ DataType, size int) {
	switch typ {
	case TypeUnknown:
		s := make([]any, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeInt:
		s := make([]int64, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeFloat:
		s := make([]float64, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeString:
		s := make([]string, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeBool:
		s := make([]bool, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeBytes:
		s := make([][]byte, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeGeometry:
		s := make([][][]float64, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeRecord:
		s := make([]*Document, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeArrayUnknown:
		s := make([][]any, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeArrayInt:
		s := make([][]int64, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeArrayFloat:
		s := make([][]float64, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeArrayString:
		s := make([][]string, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeArrayBool:
		s := make([][]bool, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeArrayBytes:
		s := make([][][]byte, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeArrayObject:
		s := make([][]*Document, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeArrayGeometry:
		s := make([][][][]float64, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	}
}

// slot returns the unsafe.Pointer for the given type, initialising it if needed.
func (d *Document) slot(typ DataType, initialSize ...int) unsafe.Pointer {
	if d.data[typ] == nil {
		size := 8
		if len(initialSize) > 0 {
			size = initialSize[0]
		}
		d.initSlice(typ, size)
	}
	return d.data[typ]
}

// claimHole searches holes (LIFO) for a free position of the given type.
// Returns the slice index, or -1 if none found. Removes via swap-and-pop.
func (d *Document) claimHole(typ DataType) int32 {
	for i := len(d.holes) - 1; i >= 0; i-- {
		if d.holes[i].Type() == typ {
			idx := d.holes[i].DataPoint().ID()
			d.holes[i] = d.holes[len(d.holes)-1]
			d.holes = d.holes[:len(d.holes)-1]
			return idx
		}
	}
	return -1
}

// freePosition records a freed slice index as a hole for future reuse.
// idx is always a valid slice index bounded by identifierMask, so NewDataPoint
// cannot return ErrIDOutOfBounds here; the panic guards against future regressions.
func (d *Document) freePosition(key DocumentKey, idx int32) {
	hole, err := NewDataPoint(key.Type(), idx)
	if err != nil {
		panic(fmt.Sprintf("document: Document.freePosition: unexpected error encoding hole: %v", err))
	}
	d.holes = append(d.holes, NewDocumentKey(hole, key.Descriptor()))
}

// --- Int64 ---

func (d *Document) SetInt(key DocumentKey, value int64) error {
	if key.Type() != TypeInt {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[]int64)(d.slot(TypeInt)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeInt); idx >= 0 {
		(*(*[]int64)(d.slot(TypeInt)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendInt(key, value)
}

func (d *Document) AppendInt(key DocumentKey, value int64) error {
	if key.Type() != TypeInt {
		return ErrTypeMismatch
	}
	ptr := (*[]int64)(d.slot(TypeInt))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetInt(key DocumentKey) (int64, bool, error) {
	if key.Type() != TypeInt {
		return 0, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return 0, false, nil
	}
	if idx < 0 {
		return 0, true, nil
	}
	return (*(*[]int64)(d.slot(TypeInt)))[idx], true, nil
}

// --- Float64 ---

func (d *Document) SetFloat(key DocumentKey, value float64) error {
	if key.Type() != TypeFloat {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[]float64)(d.slot(TypeFloat)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeFloat); idx >= 0 {
		(*(*[]float64)(d.slot(TypeFloat)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendFloat(key, value)
}

func (d *Document) AppendFloat(key DocumentKey, value float64) error {
	if key.Type() != TypeFloat {
		return ErrTypeMismatch
	}
	ptr := (*[]float64)(d.slot(TypeFloat))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetFloat(key DocumentKey) (float64, bool, error) {
	if key.Type() != TypeFloat {
		return 0, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return 0, false, nil
	}
	if idx < 0 {
		return 0, true, nil
	}
	return (*(*[]float64)(d.slot(TypeFloat)))[idx], true, nil
}

// --- String ---

func (d *Document) SetString(key DocumentKey, value string) error {
	if key.Type() != TypeString {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[]string)(d.slot(TypeString)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeString); idx >= 0 {
		(*(*[]string)(d.slot(TypeString)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendString(key, value)
}

func (d *Document) AppendString(key DocumentKey, value string) error {
	if key.Type() != TypeString {
		return ErrTypeMismatch
	}
	ptr := (*[]string)(d.slot(TypeString))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetString(key DocumentKey) (string, bool, error) {
	if key.Type() != TypeString {
		return "", false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return "", false, nil
	}
	if idx < 0 {
		return "", true, nil
	}
	return (*(*[]string)(d.slot(TypeString)))[idx], true, nil
}

// --- Bool ---

func (d *Document) SetBool(key DocumentKey, value bool) error {
	if key.Type() != TypeBool {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[]bool)(d.slot(TypeBool)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeBool); idx >= 0 {
		(*(*[]bool)(d.slot(TypeBool)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendBool(key, value)
}

func (d *Document) AppendBool(key DocumentKey, value bool) error {
	if key.Type() != TypeBool {
		return ErrTypeMismatch
	}
	ptr := (*[]bool)(d.slot(TypeBool))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetBool(key DocumentKey) (bool, bool, error) {
	if key.Type() != TypeBool {
		return false, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return false, false, nil
	}
	if idx < 0 {
		return false, true, nil
	}
	return (*(*[]bool)(d.slot(TypeBool)))[idx], true, nil
}

// --- Bytes ---

func (d *Document) SetBytes(key DocumentKey, value []byte) error {
	if key.Type() != TypeBytes {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][]byte)(d.slot(TypeBytes)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeBytes); idx >= 0 {
		(*(*[][]byte)(d.slot(TypeBytes)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendBytes(key, value)
}

func (d *Document) AppendBytes(key DocumentKey, value []byte) error {
	if key.Type() != TypeBytes {
		return ErrTypeMismatch
	}
	ptr := (*[][]byte)(d.slot(TypeBytes))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetBytes(key DocumentKey) ([]byte, bool, error) {
	if key.Type() != TypeBytes {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]byte)(d.slot(TypeBytes)))[idx], true, nil
}

// --- Geometry ---

func (d *Document) SetGeometry(key DocumentKey, value [][]float64) error {
	if key.Type() != TypeGeometry {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][][]float64)(d.slot(TypeGeometry)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeGeometry); idx >= 0 {
		(*(*[][][]float64)(d.slot(TypeGeometry)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendGeometry(key, value)
}

func (d *Document) AppendGeometry(key DocumentKey, value [][]float64) error {
	if key.Type() != TypeGeometry {
		return ErrTypeMismatch
	}
	ptr := (*[][][]float64)(d.slot(TypeGeometry))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetGeometry(key DocumentKey) ([][]float64, bool, error) {
	if key.Type() != TypeGeometry {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][][]float64)(d.slot(TypeGeometry)))[idx], true, nil
}

// --- Record ---

func (d *Document) SetRecord(key DocumentKey, value *Document) error {
	if key.Type() != TypeRecord {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[]*Document)(d.slot(TypeRecord)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeRecord); idx >= 0 {
		(*(*[]*Document)(d.slot(TypeRecord)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendRecord(key, value)
}

func (d *Document) AppendRecord(key DocumentKey, value *Document) error {
	if key.Type() != TypeRecord {
		return ErrTypeMismatch
	}
	ptr := (*[]*Document)(d.slot(TypeRecord))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetRecord(key DocumentKey) (*Document, bool, error) {
	if key.Type() != TypeRecord {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[]*Document)(d.slot(TypeRecord)))[idx], true, nil
}

// --- Unknown ---

func (d *Document) SetUnknown(key DocumentKey, value any) error {
	if key.Type() != TypeUnknown {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[]any)(d.slot(TypeUnknown)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeUnknown); idx >= 0 {
		(*(*[]any)(d.slot(TypeUnknown)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendUnknown(key, value)
}

func (d *Document) AppendUnknown(key DocumentKey, value any) error {
	if key.Type() != TypeUnknown {
		return ErrTypeMismatch
	}
	ptr := (*[]any)(d.slot(TypeUnknown))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetUnknown(key DocumentKey) (any, bool, error) {
	if key.Type() != TypeUnknown {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[]any)(d.slot(TypeUnknown)))[idx], true, nil
}

// --- ArrayInt ---

func (d *Document) SetArrayInt(key DocumentKey, value []int64) error {
	if key.Type() != TypeArrayInt {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][]int64)(d.slot(TypeArrayInt)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayInt); idx >= 0 {
		(*(*[][]int64)(d.slot(TypeArrayInt)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendArrayInt(key, value)
}

func (d *Document) AppendArrayInt(key DocumentKey, value []int64) error {
	if key.Type() != TypeArrayInt {
		return ErrTypeMismatch
	}
	ptr := (*[][]int64)(d.slot(TypeArrayInt))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetArrayInt(key DocumentKey) ([]int64, bool, error) {
	if key.Type() != TypeArrayInt {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]int64)(d.slot(TypeArrayInt)))[idx], true, nil
}

// --- ArrayFloat ---

func (d *Document) SetArrayFloat(key DocumentKey, value []float64) error {
	if key.Type() != TypeArrayFloat {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][]float64)(d.slot(TypeArrayFloat)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayFloat); idx >= 0 {
		(*(*[][]float64)(d.slot(TypeArrayFloat)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendArrayFloat(key, value)
}

func (d *Document) AppendArrayFloat(key DocumentKey, value []float64) error {
	if key.Type() != TypeArrayFloat {
		return ErrTypeMismatch
	}
	ptr := (*[][]float64)(d.slot(TypeArrayFloat))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetArrayFloat(key DocumentKey) ([]float64, bool, error) {
	if key.Type() != TypeArrayFloat {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]float64)(d.slot(TypeArrayFloat)))[idx], true, nil
}

// --- ArrayString ---

func (d *Document) SetArrayString(key DocumentKey, value []string) error {
	if key.Type() != TypeArrayString {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][]string)(d.slot(TypeArrayString)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayString); idx >= 0 {
		(*(*[][]string)(d.slot(TypeArrayString)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendArrayString(key, value)
}

func (d *Document) AppendArrayString(key DocumentKey, value []string) error {
	if key.Type() != TypeArrayString {
		return ErrTypeMismatch
	}
	ptr := (*[][]string)(d.slot(TypeArrayString))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetArrayString(key DocumentKey) ([]string, bool, error) {
	if key.Type() != TypeArrayString {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]string)(d.slot(TypeArrayString)))[idx], true, nil
}

// --- ArrayBool ---

func (d *Document) SetArrayBool(key DocumentKey, value []bool) error {
	if key.Type() != TypeArrayBool {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][]bool)(d.slot(TypeArrayBool)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayBool); idx >= 0 {
		(*(*[][]bool)(d.slot(TypeArrayBool)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendArrayBool(key, value)
}

func (d *Document) AppendArrayBool(key DocumentKey, value []bool) error {
	if key.Type() != TypeArrayBool {
		return ErrTypeMismatch
	}
	ptr := (*[][]bool)(d.slot(TypeArrayBool))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetArrayBool(key DocumentKey) ([]bool, bool, error) {
	if key.Type() != TypeArrayBool {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]bool)(d.slot(TypeArrayBool)))[idx], true, nil
}

// --- ArrayBytes ---

func (d *Document) SetArrayBytes(key DocumentKey, value [][]byte) error {
	if key.Type() != TypeArrayBytes {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][][]byte)(d.slot(TypeArrayBytes)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayBytes); idx >= 0 {
		(*(*[][][]byte)(d.slot(TypeArrayBytes)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendArrayBytes(key, value)
}

func (d *Document) AppendArrayBytes(key DocumentKey, value [][]byte) error {
	if key.Type() != TypeArrayBytes {
		return ErrTypeMismatch
	}
	ptr := (*[][][]byte)(d.slot(TypeArrayBytes))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetArrayBytes(key DocumentKey) ([][]byte, bool, error) {
	if key.Type() != TypeArrayBytes {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][][]byte)(d.slot(TypeArrayBytes)))[idx], true, nil
}

// --- ArrayObject ---

func (d *Document) SetArrayObject(key DocumentKey, value []*Document) error {
	if key.Type() != TypeArrayObject {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][]*Document)(d.slot(TypeArrayObject)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayObject); idx >= 0 {
		(*(*[][]*Document)(d.slot(TypeArrayObject)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendArrayObject(key, value)
}

func (d *Document) AppendArrayObject(key DocumentKey, value []*Document) error {
	if key.Type() != TypeArrayObject {
		return ErrTypeMismatch
	}
	ptr := (*[][]*Document)(d.slot(TypeArrayObject))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetArrayObject(key DocumentKey) ([]*Document, bool, error) {
	if key.Type() != TypeArrayObject {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]*Document)(d.slot(TypeArrayObject)))[idx], true, nil
}

// --- ArrayUnknown ---

func (d *Document) SetArrayUnknown(key DocumentKey, value []any) error {
	if key.Type() != TypeArrayUnknown {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][]any)(d.slot(TypeArrayUnknown)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayUnknown); idx >= 0 {
		(*(*[][]any)(d.slot(TypeArrayUnknown)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendArrayUnknown(key, value)
}

func (d *Document) AppendArrayUnknown(key DocumentKey, value []any) error {
	if key.Type() != TypeArrayUnknown {
		return ErrTypeMismatch
	}
	ptr := (*[][]any)(d.slot(TypeArrayUnknown))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetArrayUnknown(key DocumentKey) ([]any, bool, error) {
	if key.Type() != TypeArrayUnknown {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]any)(d.slot(TypeArrayUnknown)))[idx], true, nil
}

// --- ArrayGeometry ---

func (d *Document) SetArrayGeometry(key DocumentKey, value [][][]float64) error {
	if key.Type() != TypeArrayGeometry {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][][][]float64)(d.slot(TypeArrayGeometry)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayGeometry); idx >= 0 {
		(*(*[][][][]float64)(d.slot(TypeArrayGeometry)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendArrayGeometry(key, value)
}

func (d *Document) AppendArrayGeometry(key DocumentKey, value [][][]float64) error {
	if key.Type() != TypeArrayGeometry {
		return ErrTypeMismatch
	}
	ptr := (*[][][][]float64)(d.slot(TypeArrayGeometry))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *Document) GetArrayGeometry(key DocumentKey) ([][][]float64, bool, error) {
	if key.Type() != TypeArrayGeometry {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int64(key)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][][][]float64)(d.slot(TypeArrayGeometry)))[idx], true, nil
}

// --- Null / Unset / State ---

// SetNull marks key as explicitly null, freeing any previously held slice position.
// The field becomes IsSet=true, IsNull=true, HasValue=false.
func (d *Document) SetNull(key DocumentKey) {
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		d.freePosition(key, idx)
	}
	d.positions[k] = -1
}

// Unset removes key entirely, freeing any previously held slice position.
// The field becomes IsSet=false.
func (d *Document) Unset(key DocumentKey) {
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		d.freePosition(key, idx)
	}
	delete(d.positions, k)
}

// IsSet returns true if the key has been set (including if null).
func (d *Document) IsSet(key DocumentKey) bool {
	_, exists := d.positions[int64(key)]
	return exists
}

// IsNull returns true if the key is explicitly null.
func (d *Document) IsNull(key DocumentKey) bool {
	idx, exists := d.positions[int64(key)]
	return exists && idx < 0
}

// HasValue returns true if the key is set and holds a concrete value.
func (d *Document) HasValue(key DocumentKey) bool {
	idx, exists := d.positions[int64(key)]
	return exists && idx >= 0
}

// Length returns the number of set positions (values + nulls).
func (d *Document) Length() int {
	return len(d.positions)
}

// --- Clear ---

// Clear resets all typed slice lengths to zero (preserving capacity), clears
// the positions map, and empties the holes slice.
// After Clear the container is ready to be returned to a pool and reused.
// Only DataContainer state is reset — Document.id and Document.schema are untouched.
func (d *Document) Clear() {
	clear(d.positions)
	d.holes = d.holes[:0]

	for i, ptr := range d.data {
		if ptr == nil {
			continue
		}
		switch DataType(i) {
		case TypeUnknown:
			*(*[]any)(ptr) = (*(*[]any)(ptr))[:0]
		case TypeInt:
			*(*[]int64)(ptr) = (*(*[]int64)(ptr))[:0]
		case TypeFloat:
			*(*[]float64)(ptr) = (*(*[]float64)(ptr))[:0]
		case TypeString:
			*(*[]string)(ptr) = (*(*[]string)(ptr))[:0]
		case TypeBool:
			*(*[]bool)(ptr) = (*(*[]bool)(ptr))[:0]
		case TypeBytes:
			*(*[][]byte)(ptr) = (*(*[][]byte)(ptr))[:0]
		case TypeGeometry:
			*(*[][][]float64)(ptr) = (*(*[][][]float64)(ptr))[:0]
		case TypeRecord:
			*(*[]*Document)(ptr) = (*(*[]*Document)(ptr))[:0]
		case TypeArrayUnknown:
			*(*[][]any)(ptr) = (*(*[][]any)(ptr))[:0]
		case TypeArrayInt:
			*(*[][]int64)(ptr) = (*(*[][]int64)(ptr))[:0]
		case TypeArrayFloat:
			*(*[][]float64)(ptr) = (*(*[][]float64)(ptr))[:0]
		case TypeArrayString:
			*(*[][]string)(ptr) = (*(*[][]string)(ptr))[:0]
		case TypeArrayBool:
			*(*[][]bool)(ptr) = (*(*[][]bool)(ptr))[:0]
		case TypeArrayBytes:
			*(*[][][]byte)(ptr) = (*(*[][][]byte)(ptr))[:0]
		case TypeArrayObject:
			*(*[][]*Document)(ptr) = (*(*[][]*Document)(ptr))[:0]
		case TypeArrayGeometry:
			*(*[][][][]float64)(ptr) = (*(*[][][][]float64)(ptr))[:0]
		}
	}
}

// --- Walk ---

// Walk exposes the internal positions map and slot accessor directly to the caller.
// This enables zero-copy serialization and in-place deserialization without boxing.
//
// The walker has mutable access to DataContainer internals. It is responsible for
// maintaining the container invariants:
//   - All positive indices in positions must be valid indices into their typed slice.
//   - Holes must reflect any positions freed outside of SetNull/Unset.
//
// Serialization example:
//
//	result, err := dc.Walk(func(positions map[int32]int32, slot func(DataType, ...int) unsafe.Pointer) (any, error) {
//	    ints := *(*[]int64)(slot(TypeInt))
//	    for point, idx := range positions {
//	        p := DataPoint(point)
//	        if idx < 0 { encoder.WriteNull(p); continue }
//	        if p.Type() == TypeInt { encoder.WriteInt(p, ints[idx]) }
//	    }
//	    return encoder.Bytes(), nil
//	})
//
// Deserialization example:
//
//	dc.Clear()
//	dc.Walk(func(positions map[int32]int32, slot func(DataType, ...int) unsafe.Pointer) (any, error) {
//	    ints := (*[]int64)(slot(TypeInt, schema.MinIntCount()))
//	    for decoder.HasInt() {
//	        point, value, index := decoder.NextInt()
//	        if index < int32(len(*ints)) {
//	            (*ints)[index] = value
//	            positions[int32(point)] = index
//	        } else {
//	            dc.AppendInt(point, value)
//	        }
//	    }
//	    return nil, nil
//	})
func (d *Document) Walk(
	walker func(
		positions map[int64]int32,
		slot func(t DataType, initialSize ...int) unsafe.Pointer,
	) (any, error),
) (any, error) {
	return walker(d.positions, d.slot)
}
