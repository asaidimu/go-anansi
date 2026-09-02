package container

import (
	"fmt"
	"unsafe"
)

// DataContainerKey is a 64-bit key encoding both a DataPoint and a field descriptor.
//
// Layout:
//
//	bits 63–32: field descriptor (uint32) — type, owner_schema, field_index, flags
//	bits 31–0:  DataPoint (int32)          — null flag, DataType, 27-bit ordinal
//
// This makes a DataContainer self-describing: from a single key the holder can both
// look up the field value (via the DataPoint half) and evaluate all field-level
// rules (via the descriptor half) without any secondary lookup.
type DataContainerKey int64

// NewDataContainerKey constructs a DataContainerKey from a DataPoint and a field descriptor.
func NewDataContainerKey(dp DataPoint, descriptor uint32) DataContainerKey {
	return DataContainerKey(uint64(descriptor)<<32 | uint64(uint32(dp)))
}

// DataPoint extracts the DataPoint (low 32 bits) from a DataContainerKey.
func (k DataContainerKey) DataPoint() DataPoint {
	return DataPoint(int32(k))
}

// Descriptor extracts the field descriptor (high 32 bits) from a DataContainerKey.
func (k DataContainerKey) Descriptor() uint32 {
	return uint32(uint64(k) >> 32)
}

// Type extracts the DataType from the embedded DataPoint.
func (k DataContainerKey) Type() DataType {
	return k.DataPoint().Type()
}

// IsNull returns true if the null bit is set on the embedded DataPoint.
func (k DataContainerKey) IsNull() bool {
	return k.DataPoint().IsNull()
}

// DataContainer is a self-describing, type-indexed, poolable, sparse data container
// keyed by DataContainerKey (64-bit) rather than DataPoint (32-bit).
//
// Each key carries the field descriptor alongside the DataPoint, making it
// suitable for validation without any secondary schema lookups.

// data[i] holds a pointer to the slice header for DataType(i), lazily initialised.
// The pointer is to the header (*[]T), not the backing array, so it survives appends.
//
// positions maps int64(DataContainerKey) → slice index within the typed slice.
// The key is the full 64-bit DataContainerKey (field descriptor + DataPoint), so fields
// with the same DataPoint but different descriptors are distinct entries.
// A value of -1 means the field is explicitly null (present but valueless).
// Absence from the map means the field has never been set.
//
// holes tracks freed slice positions available for reuse, encoded as DataContainerKeys
// whose embedded DataPoint ID field holds the freed slice index.
type DataContainer struct {
	data      [16]unsafe.Pointer
	positions map[int64]int32
	holes     []DataContainerKey

	// backing is a private, immutable-after-fill byte buffer that decoded
	// string values alias (zero-copy decode). Buffers are pooled implicitly:
	// Clear intentionally RETAINS backing capacity so a pooled container's
	// next aliased decode reuses it — one bulk copy per fill instead of one
	// allocation per string. Oversized buffers are released to bound memory.
	// Strings are immutable in Go, so aliasing is safe for reads and field
	// writes alike; only buffer lifetime matters, and the container holds it.
	backing []byte
}

// maxRetainedBacking caps the backing capacity a pooled container retains
// across Clear. Larger buffers are dropped rather than pinned.
const maxRetainedBacking = 1 << 20 // 1 MiB

// AcquireBacking returns a len-n buffer for this fill cycle, reusing
// retained capacity when it fits (amortized steady state: no allocation).
// Codec-internal API: top-level decode calls this once and attaches the
// result to nested child containers verbatim via OwnBacking.
func (d *DataContainer) AcquireBacking(n int) []byte {
	if cap(d.backing) >= n {
		return d.backing[:n]
	}
	d.backing = make([]byte, n)
	return d.backing
}

// OwnBacking attaches an immutable buffer that this container's string
// values alias into. Used to propagate a parent's backing to nested child
// containers so extracted children remain valid independently. The buffer
// must never be mutated after attachment.
func (d *DataContainer) OwnBacking(b []byte) {
	d.backing = b
}

// Backing returns the buffer previously attached via acquireBacking or
// OwnBacking, if any.
func (d *DataContainer) Backing() []byte {
	return d.backing
}

func NewDataContainer() *DataContainer {
	return &DataContainer{
		positions: make(map[int64]int32),
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
		s := make([]map[string]any, 0, size)
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
func (d *DataContainer) freePosition(key DataContainerKey, idx int32) {
	hole, err := NewDataPoint(key.Type(), idx)
	if err != nil {
		panic(fmt.Sprintf("document: DataContainer.freePosition: unexpected error encoding hole: %v", err))
	}
	d.holes = append(d.holes, NewDataContainerKey(hole, key.Descriptor()))
}

// --- Int64 ---

func (d *DataContainer) SetInt(key DataContainerKey, value int64) error {
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

func (d *DataContainer) AppendInt(key DataContainerKey, value int64) error {
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

func (d *DataContainer) GetInt(key DataContainerKey) (int64, bool, error) {
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

func (d *DataContainer) SetFloat(key DataContainerKey, value float64) error {
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

func (d *DataContainer) AppendFloat(key DataContainerKey, value float64) error {
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

func (d *DataContainer) GetFloat(key DataContainerKey) (float64, bool, error) {
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

func (d *DataContainer) SetString(key DataContainerKey, value string) error {
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

func (d *DataContainer) AppendString(key DataContainerKey, value string) error {
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

func (d *DataContainer) GetString(key DataContainerKey) (string, bool, error) {
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

func (d *DataContainer) SetBool(key DataContainerKey, value bool) error {
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

func (d *DataContainer) AppendBool(key DataContainerKey, value bool) error {
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

func (d *DataContainer) GetBool(key DataContainerKey) (bool, bool, error) {
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

func (d *DataContainer) SetBytes(key DataContainerKey, value []byte) error {
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

func (d *DataContainer) AppendBytes(key DataContainerKey, value []byte) error {
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

func (d *DataContainer) GetBytes(key DataContainerKey) ([]byte, bool, error) {
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

func (d *DataContainer) SetGeometry(key DataContainerKey, value [][]float64) error {
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

func (d *DataContainer) AppendGeometry(key DataContainerKey, value [][]float64) error {
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

func (d *DataContainer) GetGeometry(key DataContainerKey) ([][]float64, bool, error) {
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

// SetRecord stores a record value as map[string]any in the dedicated
// TypeRecord slot. Records are schema-free sub-objects; the map's values are
// ordinary Go values (scalars, slices, nested map[string]any).
func (d *DataContainer) SetRecord(key DataContainerKey, value map[string]any) error {
	if key.Type() != TypeRecord {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[]map[string]any)(d.slot(TypeRecord)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeRecord); idx >= 0 {
		(*(*[]map[string]any)(d.slot(TypeRecord)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendRecord(key, value)
}

func (d *DataContainer) AppendRecord(key DataContainerKey, value map[string]any) error {
	if key.Type() != TypeRecord {
		return ErrTypeMismatch
	}
	ptr := (*[]map[string]any)(d.slot(TypeRecord))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *DataContainer) GetRecord(key DataContainerKey) (map[string]any, bool, error) {
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
	return (*(*[]map[string]any)(d.slot(TypeRecord)))[idx], true, nil
}

// --- Unknown ---

func (d *DataContainer) SetUnknown(key DataContainerKey, value any) error {
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

func (d *DataContainer) AppendUnknown(key DataContainerKey, value any) error {
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

func (d *DataContainer) GetUnknown(key DataContainerKey) (any, bool, error) {
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

func (d *DataContainer) SetArrayInt(key DataContainerKey, value []int64) error {
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

func (d *DataContainer) AppendArrayInt(key DataContainerKey, value []int64) error {
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

func (d *DataContainer) GetArrayInt(key DataContainerKey) ([]int64, bool, error) {
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

func (d *DataContainer) SetArrayFloat(key DataContainerKey, value []float64) error {
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

func (d *DataContainer) AppendArrayFloat(key DataContainerKey, value []float64) error {
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

func (d *DataContainer) GetArrayFloat(key DataContainerKey) ([]float64, bool, error) {
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

func (d *DataContainer) SetArrayString(key DataContainerKey, value []string) error {
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

func (d *DataContainer) AppendArrayString(key DataContainerKey, value []string) error {
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

func (d *DataContainer) GetArrayString(key DataContainerKey) ([]string, bool, error) {
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

func (d *DataContainer) SetArrayBool(key DataContainerKey, value []bool) error {
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

func (d *DataContainer) AppendArrayBool(key DataContainerKey, value []bool) error {
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

func (d *DataContainer) GetArrayBool(key DataContainerKey) ([]bool, bool, error) {
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

func (d *DataContainer) SetArrayBytes(key DataContainerKey, value [][]byte) error {
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

func (d *DataContainer) AppendArrayBytes(key DataContainerKey, value [][]byte) error {
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

func (d *DataContainer) GetArrayBytes(key DataContainerKey) ([][]byte, bool, error) {
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

func (d *DataContainer) SetArrayObject(key DataContainerKey, value []*DataContainer) error {
	if key.Type() != TypeArrayObject {
		return ErrTypeMismatch
	}
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		(*(*[][]*DataContainer)(d.slot(TypeArrayObject)))[idx] = value
		return nil
	}
	if idx := d.claimHole(TypeArrayObject); idx >= 0 {
		(*(*[][]*DataContainer)(d.slot(TypeArrayObject)))[idx] = value
		d.positions[k] = idx
		return nil
	}
	return d.AppendArrayObject(key, value)
}

func (d *DataContainer) AppendArrayObject(key DataContainerKey, value []*DataContainer) error {
	if key.Type() != TypeArrayObject {
		return ErrTypeMismatch
	}
	ptr := (*[][]*DataContainer)(d.slot(TypeArrayObject))
	idx := int32(len(*ptr))
	if idx >= identifierMask {
		return ErrBucketFull
	}
	*ptr = append(*ptr, value)
	d.positions[int64(key)] = idx
	return nil
}

func (d *DataContainer) GetArrayObject(key DataContainerKey) ([]*DataContainer, bool, error) {
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
	return (*(*[][]*DataContainer)(d.slot(TypeArrayObject)))[idx], true, nil
}

// --- ArrayUnknown ---

func (d *DataContainer) SetArrayUnknown(key DataContainerKey, value []any) error {
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

func (d *DataContainer) AppendArrayUnknown(key DataContainerKey, value []any) error {
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

func (d *DataContainer) GetArrayUnknown(key DataContainerKey) ([]any, bool, error) {
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

func (d *DataContainer) SetArrayGeometry(key DataContainerKey, value [][][]float64) error {
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

func (d *DataContainer) AppendArrayGeometry(key DataContainerKey, value [][][]float64) error {
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

func (d *DataContainer) GetArrayGeometry(key DataContainerKey) ([][][]float64, bool, error) {
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
func (d *DataContainer) SetNull(key DataContainerKey) {
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		d.freePosition(key, idx)
	}
	d.positions[k] = -1
}

// Unset removes key entirely, freeing any previously held slice position.
// The field becomes IsSet=false.
func (d *DataContainer) Unset(key DataContainerKey) {
	k := int64(key)
	if idx, exists := d.positions[k]; exists && idx >= 0 {
		d.freePosition(key, idx)
	}
	delete(d.positions, k)
}

// IsSet returns true if the key has been set (including if null).
func (d *DataContainer) IsSet(key DataContainerKey) bool {
	_, exists := d.positions[int64(key)]
	return exists
}

// IsNull returns true if the key is explicitly null.
func (d *DataContainer) IsNull(key DataContainerKey) bool {
	idx, exists := d.positions[int64(key)]
	return exists && idx < 0
}

// HasValue returns true if the key is set and holds a concrete value.
func (d *DataContainer) HasValue(key DataContainerKey) bool {
	idx, exists := d.positions[int64(key)]
	return exists && idx >= 0
}

// Position reports key's raw positions-map entry in a single lookup — the
// NotSet/Null/HasValue tri-state in one map access instead of the two
// (IsSet + IsNull) the individual predicates need.
//
//	ok == false            → NotSet (field never set)
//	ok == true, idx < 0    → Null   (explicitly null)
//	ok == true, idx >= 0   → HasValue (value stored at typed-slice index idx)
//
// This is the codec-facing form of the state tri-state; IsSet/IsNull/
// HasValue remain the friendlier per-question wrappers.
func (d *DataContainer) Position(key DataContainerKey) (idx int32, ok bool) {
	idx, ok = d.positions[int64(key)]
	return idx, ok
}

// Length returns the number of set positions (values + nulls).
func (d *DataContainer) Length() int {
	return len(d.positions)
}

// --- Clear ---

// Clear resets all typed slice lengths to zero (preserving capacity), clears
// the positions map, and empties the holes slice.
// After Clear the container is ready to be returned to a pool and reused.
// DataContainer has no identity or schema fields of its own — the container state is
// the whole of a DataContainer.
func (d *DataContainer) Clear() {
	clear(d.positions)
	d.holes = d.holes[:0]

	// Retain backing capacity for the next aliased fill (amortization),
	// unless it is large enough to be worth releasing.
	if cap(d.backing) > maxRetainedBacking {
		d.backing = nil
	}

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
			*(*[]map[string]any)(ptr) = (*(*[]map[string]any)(ptr))[:0]
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
			*(*[][]*DataContainer)(ptr) = (*(*[][]*DataContainer)(ptr))[:0]
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
//	result, err := doc.Walk(func(positions map[int64]int32, slot func(DataType, ...int) unsafe.Pointer) (any, error) {
//	    ints := *(*[]int64)(slot(TypeInt))
//	    for k, idx := range positions {
//	        key := DataContainerKey(k)
//	        if idx < 0 { encoder.WriteNull(key); continue }
//	        if key.Type() == TypeInt { encoder.WriteInt(key, ints[idx]) }
//	    }
//	    return encoder.Bytes(), nil
//	})
//
// Deserialization example:
//
//	doc.Clear()
//	doc.Walk(func(positions map[int64]int32, slot func(DataType, ...int) unsafe.Pointer) (any, error) {
//	    ints := (*[]int64)(slot(TypeInt, schema.MinIntCount()))
//	    for decoder.HasInt() {
//	        key, value, index := decoder.NextInt()
//	        if index < int32(len(*ints)) {
//	            (*ints)[index] = value
//	            positions[int64(key)] = index
//	        } else {
//	            doc.AppendInt(key, value)
//	        }
//	    }
//	    return nil, nil
//	})
func (d *DataContainer) Walk(
	walker func(
		positions map[int64]int32,
		slot func(t DataType, initialSize ...int) unsafe.Pointer,
	) (any, error),
) (any, error) {
	return walker(d.positions, d.slot)
}
