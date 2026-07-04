package document

import (
	"fmt"
)

// DataPoint is a 32-bit descriptor encoding:
//
//	bit 0      : null flag (1 = null)
//	bits 1-4   : DataType (4 bits, 16 types)
//	bits 5-31  : unique field identifier (27 bits, schema-derived)
type DataPoint int32

type DataType uint8

const (
	TypeUnknown       DataType = iota // any                        — open records, unresolvable unions
	TypeInt                           // int64                      — integer, enum (ordinal)
	TypeFloat                         // float64                    — number
	TypeString                        // string                     — string
	TypeBool                          // bool                       — boolean
	TypeBytes                         // []byte                     — binary blobs, hashes, UUIDs, encoded payloads
	TypeGeometry                      // [][]float64                — geometry (coordinate rings)
	TypeRecord                        // *Document  — typed sub-object with known schema
	TypeArrayUnknown                  // []any                      — array of unknown/incompatible-union elements
	TypeArrayInt                      // []int64                    — array of integer/enum
	TypeArrayFloat                    // []float64                  — array of number
	TypeArrayString                   // []string                   — array of string
	TypeArrayBool                     // []bool                     — array of boolean
	TypeArrayBytes                    // [][]byte                   — array of bytes
	TypeArrayObject                   // []*DataContainer           — array of object (known schema)
	TypeArrayGeometry                 // [][][]float64              — array of geometry
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
