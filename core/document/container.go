package document

import (
	"fmt"
	"unsafe"
)

// DataPoint is a 32-bit descriptor encoding:
//
//	bit 0      : null flag (1 = null)
//	bits 1-4   : DataType (4 bits, 16 types)
//	bits 5-31  : unique field identifier (27 bits, schema-derived)
type DataPoint int32

type DataType uint8

const (
	TypeUnknown      DataType = iota // any                        — open records, unresolvable unions
	TypeInt                          // int64                      — integer, enum (ordinal)
	TypeFloat                        // float64                    — number
	TypeString                       // string                     — string
	TypeBool                         // bool                       — boolean
	TypeBytes                        // []byte                     — binary blobs, hashes, UUIDs, encoded payloads
	TypeGeometry                     // [][]float64                — geometry (coordinate rings)
	TypeRecord                       // map[string]*DataContainer  — record<T> with known schema
	TypeArrayUnknown                 // []any                      — array of unknown/incompatible-union elements
	TypeArrayInt                     // []int64                    — array of integer/enum
	TypeArrayFloat                   // []float64                  — array of number
	TypeArrayString                  // []string                   — array of string
	TypeArrayBool                    // []bool                     — array of boolean
	TypeArrayBytes                   // [][]byte                   — array of bytes
	TypeArrayObject                  // []*DataContainer           — array of object (known schema)
	TypeArrayGeometry                // [][][]float64              — array of geometry
)

const (
	nullBits = 1
	typeBits = 4
	dataBits = nullBits + typeBits // 5

	typeMask       DataPoint = 0xF       // 4 bits
	identifierMask int32     = 0x7FFFFFF // 27 bits
)

var (
	ErrTypeMismatch  = fmt.Errorf("type mismatch")
	ErrBucketFull    = fmt.Errorf("container full")
	ErrIDOutOfBounds = fmt.Errorf("id out of bounds")
)

// NewDataPoint constructs a DataPoint encoding the given type and optional id.
// If no id is provided, the DataPoint has a zero id.
func NewDataPoint(typ DataType, id ...int32) (DataPoint, error) {
	if len(id) == 0 {
		return DataPoint(typ) << nullBits, nil
	}
	if id[0] < 0 || id[0] > identifierMask {
		return 0, ErrIDOutOfBounds
	}
	return (DataPoint(id[0]) << dataBits) | (DataPoint(typ) << nullBits), nil
}

// Type extracts the DataType from a DataPoint.
func (p DataPoint) Type() DataType {
	return DataType((p >> nullBits) & typeMask)
}

// ID extracts the 27-bit unique identifier from a DataPoint.
func (p DataPoint) ID() int32 {
	return int32(p>>dataBits) & identifierMask
}

// WithID returns a new DataPoint with the same type and null bits but a different id.
func (p DataPoint) WithID(id int32) (DataPoint, error) {
	if id < 0 || id > identifierMask {
		return 0, ErrIDOutOfBounds
	}
	base := p & DataPoint((1<<dataBits)-1) // preserve bits 0..4
	return base | (DataPoint(id) << dataBits), nil
}

// IsNull returns true if the null bit is set.
func (p DataPoint) IsNull() bool {
	return p&1 == 1
}

// DataContainer is a type-indexed, poolable, sparse data container.
//
// data[i] holds a pointer to the slice header for DataType(i), lazily initialised.
// The pointer is to the header (*[]T), not the backing array, so it survives appends.
//
// positions maps int32(DataPoint) → slice index within the typed slice.
// A value of -1 means the field is explicitly null (present but valueless).
// Absence from the map means the field has never been set.
//
// holes tracks freed slice positions available for reuse, encoded as DataPoints
// where the ID field holds the freed slice index.
type DataContainer struct {
	data      [16]unsafe.Pointer
	positions map[int32]int32
	holes     []DataPoint
}

func NewDataContainer() *DataContainer {
	return &DataContainer{
		positions: make(map[int32]int32),
	}
}

// initSlice allocates a new typed slice for the given DataType and stores
// a pointer to its header in data[typ]. Called lazily on first write.
func (d *DataContainer) initSlice(typ DataType, size int) {
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
		s := make([]map[string]*DataContainer, 0, size)
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
		s := make([][]*DataContainer, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	case TypeArrayGeometry:
		s := make([][][][]float64, 0, size)
		d.data[typ] = unsafe.Pointer(&s)
	}
}

// slot returns the unsafe.Pointer for the given type, initialising it if needed.
func (d *DataContainer) slot(typ DataType, initialSize ...int) unsafe.Pointer {
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
func (d *DataContainer) claimHole(typ DataType) int32 {
	for i := len(d.holes) - 1; i >= 0; i-- {
		if d.holes[i].Type() == typ {
			idx := d.holes[i].ID()
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
func (d *DataContainer) freePosition(point DataPoint, idx int32) {
	hole, err := NewDataPoint(point.Type(), idx)
	if err != nil {
		panic(fmt.Sprintf("document: freePosition: unexpected error encoding hole: %v", err))
	}
	d.holes = append(d.holes, hole)
}

// --- Int64 ---

func (d *DataContainer) SetInt(point DataPoint, value int64) error {
	if point.Type() != TypeInt {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[]int64)(d.slot(TypeInt)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeInt); idx >= 0 {
		(*(*[]int64)(d.slot(TypeInt)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendInt(point, value)
}

func (d *DataContainer) AppendInt(point DataPoint, value int64) error {
	if point.Type() != TypeInt {
		return ErrTypeMismatch
	}
	ptr := (*[]int64)(d.slot(TypeInt))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetInt(point DataPoint) (int64, bool, error) {
	if point.Type() != TypeInt {
		return 0, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return 0, false, nil
	}
	if idx < 0 {
		return 0, true, nil // null
	}
	return (*(*[]int64)(d.slot(TypeInt)))[idx], true, nil
}

// --- Float64 ---

func (d *DataContainer) SetFloat(point DataPoint, value float64) error {
	if point.Type() != TypeFloat {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[]float64)(d.slot(TypeFloat)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeFloat); idx >= 0 {
		(*(*[]float64)(d.slot(TypeFloat)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendFloat(point, value)
}

func (d *DataContainer) AppendFloat(point DataPoint, value float64) error {
	if point.Type() != TypeFloat {
		return ErrTypeMismatch
	}
	ptr := (*[]float64)(d.slot(TypeFloat))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetFloat(point DataPoint) (float64, bool, error) {
	if point.Type() != TypeFloat {
		return 0, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return 0, false, nil
	}
	if idx < 0 {
		return 0, true, nil
	}
	return (*(*[]float64)(d.slot(TypeFloat)))[idx], true, nil
}

// --- String ---

func (d *DataContainer) SetString(point DataPoint, value string) error {
	if point.Type() != TypeString {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[]string)(d.slot(TypeString)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeString); idx >= 0 {
		(*(*[]string)(d.slot(TypeString)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendString(point, value)
}

func (d *DataContainer) AppendString(point DataPoint, value string) error {
	if point.Type() != TypeString {
		return ErrTypeMismatch
	}
	ptr := (*[]string)(d.slot(TypeString))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetString(point DataPoint) (string, bool, error) {
	if point.Type() != TypeString {
		return "", false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return "", false, nil
	}
	if idx < 0 {
		return "", true, nil
	}
	return (*(*[]string)(d.slot(TypeString)))[idx], true, nil
}

// --- Bool ---

func (d *DataContainer) SetBool(point DataPoint, value bool) error {
	if point.Type() != TypeBool {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[]bool)(d.slot(TypeBool)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeBool); idx >= 0 {
		(*(*[]bool)(d.slot(TypeBool)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendBool(point, value)
}

func (d *DataContainer) AppendBool(point DataPoint, value bool) error {
	if point.Type() != TypeBool {
		return ErrTypeMismatch
	}
	ptr := (*[]bool)(d.slot(TypeBool))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetBool(point DataPoint) (bool, bool, error) {
	if point.Type() != TypeBool {
		return false, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return false, false, nil
	}
	if idx < 0 {
		return false, true, nil
	}
	return (*(*[]bool)(d.slot(TypeBool)))[idx], true, nil
}

// --- Bytes ([]byte — binary blobs, hashes, UUIDs, encoded payloads) ---

func (d *DataContainer) SetBytes(point DataPoint, value []byte) error {
	if point.Type() != TypeBytes {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[][]byte)(d.slot(TypeBytes)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeBytes); idx >= 0 {
		(*(*[][]byte)(d.slot(TypeBytes)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendBytes(point, value)
}

func (d *DataContainer) AppendBytes(point DataPoint, value []byte) error {
	if point.Type() != TypeBytes {
		return ErrTypeMismatch
	}
	ptr := (*[][]byte)(d.slot(TypeBytes))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetBytes(point DataPoint) ([]byte, bool, error) {
	if point.Type() != TypeBytes {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]byte)(d.slot(TypeBytes)))[idx], true, nil
}

// --- Geometry ([][]float64 — coordinate rings) ---

func (d *DataContainer) SetGeometry(point DataPoint, value [][]float64) error {
	if point.Type() != TypeGeometry {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[][][]float64)(d.slot(TypeGeometry)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeGeometry); idx >= 0 {
		(*(*[][][]float64)(d.slot(TypeGeometry)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendGeometry(point, value)
}

func (d *DataContainer) AppendGeometry(point DataPoint, value [][]float64) error {
	if point.Type() != TypeGeometry {
		return ErrTypeMismatch
	}
	ptr := (*[][][]float64)(d.slot(TypeGeometry))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetGeometry(point DataPoint) ([][]float64, bool, error) {
	if point.Type() != TypeGeometry {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][][]float64)(d.slot(TypeGeometry)))[idx], true, nil
}

// --- Record (map[string]*DataContainer — record<T> with known schema) ---
//
// TypeRecord stores schema-typed records with arbitrary runtime string keys.
// For open records (no schema, T = any), use SetUnknown with a map[string]any value.

func (d *DataContainer) SetRecord(point DataPoint, value map[string]*DataContainer) error {
	if point.Type() != TypeRecord {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[]map[string]*DataContainer)(d.slot(TypeRecord)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeRecord); idx >= 0 {
		(*(*[]map[string]*DataContainer)(d.slot(TypeRecord)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendRecord(point, value)
}

func (d *DataContainer) AppendRecord(point DataPoint, value map[string]*DataContainer) error {
	if point.Type() != TypeRecord {
		return ErrTypeMismatch
	}
	ptr := (*[]map[string]*DataContainer)(d.slot(TypeRecord))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetRecord(point DataPoint) (map[string]*DataContainer, bool, error) {
	if point.Type() != TypeRecord {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[]map[string]*DataContainer)(d.slot(TypeRecord)))[idx], true, nil
}

// --- Unknown (any — open records, unresolvable unions) ---

func (d *DataContainer) SetUnknown(point DataPoint, value any) error {
	if point.Type() != TypeUnknown {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[]any)(d.slot(TypeUnknown)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeUnknown); idx >= 0 {
		(*(*[]any)(d.slot(TypeUnknown)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendUnknown(point, value)
}

func (d *DataContainer) AppendUnknown(point DataPoint, value any) error {
	if point.Type() != TypeUnknown {
		return ErrTypeMismatch
	}
	ptr := (*[]any)(d.slot(TypeUnknown))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetUnknown(point DataPoint) (any, bool, error) {
	if point.Type() != TypeUnknown {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[]any)(d.slot(TypeUnknown)))[idx], true, nil
}

// --- ArrayInt ([]int64 — array of integer or enum ordinals) ---

func (d *DataContainer) SetArrayInt(point DataPoint, value []int64) error {
	if point.Type() != TypeArrayInt {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[][]int64)(d.slot(TypeArrayInt)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayInt); idx >= 0 {
		(*(*[][]int64)(d.slot(TypeArrayInt)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendArrayInt(point, value)
}

func (d *DataContainer) AppendArrayInt(point DataPoint, value []int64) error {
	if point.Type() != TypeArrayInt {
		return ErrTypeMismatch
	}
	ptr := (*[][]int64)(d.slot(TypeArrayInt))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetArrayInt(point DataPoint) ([]int64, bool, error) {
	if point.Type() != TypeArrayInt {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]int64)(d.slot(TypeArrayInt)))[idx], true, nil
}

// --- ArrayFloat ([]float64) ---

func (d *DataContainer) SetArrayFloat(point DataPoint, value []float64) error {
	if point.Type() != TypeArrayFloat {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[][]float64)(d.slot(TypeArrayFloat)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayFloat); idx >= 0 {
		(*(*[][]float64)(d.slot(TypeArrayFloat)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendArrayFloat(point, value)
}

func (d *DataContainer) AppendArrayFloat(point DataPoint, value []float64) error {
	if point.Type() != TypeArrayFloat {
		return ErrTypeMismatch
	}
	ptr := (*[][]float64)(d.slot(TypeArrayFloat))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetArrayFloat(point DataPoint) ([]float64, bool, error) {
	if point.Type() != TypeArrayFloat {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]float64)(d.slot(TypeArrayFloat)))[idx], true, nil
}

// --- ArrayString ([]string) ---

func (d *DataContainer) SetArrayString(point DataPoint, value []string) error {
	if point.Type() != TypeArrayString {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[][]string)(d.slot(TypeArrayString)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayString); idx >= 0 {
		(*(*[][]string)(d.slot(TypeArrayString)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendArrayString(point, value)
}

func (d *DataContainer) AppendArrayString(point DataPoint, value []string) error {
	if point.Type() != TypeArrayString {
		return ErrTypeMismatch
	}
	ptr := (*[][]string)(d.slot(TypeArrayString))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetArrayString(point DataPoint) ([]string, bool, error) {
	if point.Type() != TypeArrayString {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]string)(d.slot(TypeArrayString)))[idx], true, nil
}

// --- ArrayBool ([]bool) ---

func (d *DataContainer) SetArrayBool(point DataPoint, value []bool) error {
	if point.Type() != TypeArrayBool {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[][]bool)(d.slot(TypeArrayBool)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayBool); idx >= 0 {
		(*(*[][]bool)(d.slot(TypeArrayBool)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendArrayBool(point, value)
}

func (d *DataContainer) AppendArrayBool(point DataPoint, value []bool) error {
	if point.Type() != TypeArrayBool {
		return ErrTypeMismatch
	}
	ptr := (*[][]bool)(d.slot(TypeArrayBool))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetArrayBool(point DataPoint) ([]bool, bool, error) {
	if point.Type() != TypeArrayBool {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]bool)(d.slot(TypeArrayBool)))[idx], true, nil
}

// --- ArrayBytes ([][]byte — array of binary values) ---

func (d *DataContainer) SetArrayBytes(point DataPoint, value [][]byte) error {
	if point.Type() != TypeArrayBytes {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[][][]byte)(d.slot(TypeArrayBytes)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayBytes); idx >= 0 {
		(*(*[][][]byte)(d.slot(TypeArrayBytes)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendArrayBytes(point, value)
}

func (d *DataContainer) AppendArrayBytes(point DataPoint, value [][]byte) error {
	if point.Type() != TypeArrayBytes {
		return ErrTypeMismatch
	}
	ptr := (*[][][]byte)(d.slot(TypeArrayBytes))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetArrayBytes(point DataPoint) ([][]byte, bool, error) {
	if point.Type() != TypeArrayBytes {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][][]byte)(d.slot(TypeArrayBytes)))[idx], true, nil
}

// --- ArrayObject ([]*DataContainer — ordered array of typed objects) ---

func (d *DataContainer) SetArrayObject(point DataPoint, value []*DataContainer) error {
	if point.Type() != TypeArrayObject {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[][]*DataContainer)(d.slot(TypeArrayObject)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayObject); idx >= 0 {
		(*(*[][]*DataContainer)(d.slot(TypeArrayObject)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendArrayObject(point, value)
}

func (d *DataContainer) AppendArrayObject(point DataPoint, value []*DataContainer) error {
	if point.Type() != TypeArrayObject {
		return ErrTypeMismatch
	}
	ptr := (*[][]*DataContainer)(d.slot(TypeArrayObject))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetArrayObject(point DataPoint) ([]*DataContainer, bool, error) {
	if point.Type() != TypeArrayObject {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]*DataContainer)(d.slot(TypeArrayObject)))[idx], true, nil
}

// --- ArrayUnknown ([]any — array of unknown or incompatible-union elements) ---

func (d *DataContainer) SetArrayUnknown(point DataPoint, value []any) error {
	if point.Type() != TypeArrayUnknown {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[][]any)(d.slot(TypeArrayUnknown)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayUnknown); idx >= 0 {
		(*(*[][]any)(d.slot(TypeArrayUnknown)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendArrayUnknown(point, value)
}

func (d *DataContainer) AppendArrayUnknown(point DataPoint, value []any) error {
	if point.Type() != TypeArrayUnknown {
		return ErrTypeMismatch
	}
	ptr := (*[][]any)(d.slot(TypeArrayUnknown))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetArrayUnknown(point DataPoint) ([]any, bool, error) {
	if point.Type() != TypeArrayUnknown {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][]any)(d.slot(TypeArrayUnknown)))[idx], true, nil
}

// --- ArrayGeometry ([][][]float64 — array of geometry values) ---

func (d *DataContainer) SetArrayGeometry(point DataPoint, value [][][]float64) error {
	if point.Type() != TypeArrayGeometry {
		return ErrTypeMismatch
	}
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		(*(*[][][][]float64)(d.slot(TypeArrayGeometry)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayGeometry); idx >= 0 {
		(*(*[][][][]float64)(d.slot(TypeArrayGeometry)))[idx] = value
		d.positions[key] = idx
		return nil
	}
	return d.AppendArrayGeometry(point, value)
}

func (d *DataContainer) AppendArrayGeometry(point DataPoint, value [][][]float64) error {
	if point.Type() != TypeArrayGeometry {
		return ErrTypeMismatch
	}
	ptr := (*[][][][]float64)(d.slot(TypeArrayGeometry))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int32(point)] = idx
	return nil
}

func (d *DataContainer) GetArrayGeometry(point DataPoint) ([][][]float64, bool, error) {
	if point.Type() != TypeArrayGeometry {
		return nil, false, ErrTypeMismatch
	}
	idx, exists := d.positions[int32(point)]
	if !exists {
		return nil, false, nil
	}
	if idx < 0 {
		return nil, true, nil
	}
	return (*(*[][][][]float64)(d.slot(TypeArrayGeometry)))[idx], true, nil
}

// --- Null / Unset / State ---

// SetNull marks point as explicitly null, freeing any previously held slice position.
// The field becomes IsSet=true, IsNull=true, HasValue=false.
func (d *DataContainer) SetNull(point DataPoint) {
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		d.freePosition(point, idx)
	}
	d.positions[key] = -1
}

// Unset removes point entirely, freeing any previously held slice position.
// The field becomes IsSet=false.
func (d *DataContainer) Unset(point DataPoint) {
	key := int32(point)
	if idx, exists := d.positions[key]; exists && idx >= 0 {
		d.freePosition(point, idx)
	}
	delete(d.positions, key)
}

// IsSet returns true if the point has been set (including if null).
func (d *DataContainer) IsSet(point DataPoint) bool {
	_, exists := d.positions[int32(point)]
	return exists
}

// IsNull returns true if the point is explicitly null (set but valueless).
func (d *DataContainer) IsNull(point DataPoint) bool {
	idx, exists := d.positions[int32(point)]
	return exists && idx < 0
}

// HasValue returns true if the point is set and holds a concrete value.
func (d *DataContainer) HasValue(point DataPoint) bool {
	idx, exists := d.positions[int32(point)]
	return exists && idx >= 0
}

// Length returns the number of set positions (values + nulls).
func (d *DataContainer) Length() int {
	return len(d.positions)
}

// --- Clear (pool reuse) ---

// Clear resets all typed slice lengths to zero (preserving capacity), clears
// the positions map, and empties the holes slice.
// After Clear the container is ready to be returned to a pool and reused.
// Only DataContainer state is reset — Document.id and Document.schema are untouched.
func (d *DataContainer) Clear() {
	clear(d.positions)
	d.holes = d.holes[:0]

	for i, ptr := range d.data {
		if ptr == nil {
			continue
		}
		switch DataType(i) {
		case TypeUnknown:
			s := (*[]any)(ptr)
			*s = (*s)[:0]
		case TypeInt:
			s := (*[]int64)(ptr)
			*s = (*s)[:0]
		case TypeFloat:
			s := (*[]float64)(ptr)
			*s = (*s)[:0]
		case TypeString:
			s := (*[]string)(ptr)
			*s = (*s)[:0]
		case TypeBool:
			s := (*[]bool)(ptr)
			*s = (*s)[:0]
		case TypeBytes:
			s := (*[][]byte)(ptr)
			*s = (*s)[:0]
		case TypeGeometry:
			s := (*[][][]float64)(ptr)
			*s = (*s)[:0]
		case TypeRecord:
			s := (*[]map[string]*DataContainer)(ptr)
			*s = (*s)[:0]
		case TypeArrayUnknown:
			s := (*[][]any)(ptr)
			*s = (*s)[:0]
		case TypeArrayInt:
			s := (*[][]int64)(ptr)
			*s = (*s)[:0]
		case TypeArrayFloat:
			s := (*[][]float64)(ptr)
			*s = (*s)[:0]
		case TypeArrayString:
			s := (*[][]string)(ptr)
			*s = (*s)[:0]
		case TypeArrayBool:
			s := (*[][]bool)(ptr)
			*s = (*s)[:0]
		case TypeArrayBytes:
			s := (*[][][]byte)(ptr)
			*s = (*s)[:0]
		case TypeArrayObject:
			s := (*[][]*DataContainer)(ptr)
			*s = (*s)[:0]
		case TypeArrayGeometry:
			s := (*[][][][]float64)(ptr)
			*s = (*s)[:0]
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
func (d *DataContainer) Walk(
	walker func(
		positions map[int32]int32,
		slot func(t DataType, initialSize ...int) unsafe.Pointer,
	) (any, error),
) (any, error) {
	return walker(d.positions, d.slot)
}
