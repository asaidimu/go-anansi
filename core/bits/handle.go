package bits

import (
	"errors"
	"fmt"
)

var (
	ErrLengthExceedsBitSpace = errors.New("length exceeds maximum allowed bit space")
	ErrLengthNegative        = errors.New("length cannot be negative")
	ErrOffsetOutOfBounds     = errors.New("offset must be between 0 and 65536 (2^16)")
	ErrIDExceedsBitSpace     = errors.New("id exceeds maximum allowed bit space")
)

const MaxOffset = 1 << 16 // 65,536 (2^16)

// Handle is a packed 64-bit identifier storing offset, length, and ID.
// Layout: Offset (bits 0-15), Length (bits 16..16+N-1), ID (bits 16+N..63).
type Handle uint64

// HandleSpec is a packed 8-bit configuration storing the length bit width (1-32).
type HandleSpec uint8

// NewHandleSpec constructs an 8-bit HandleSpec.
// - lengthBits: max 32 bits. If 0, defaults to 32 bits.
func NewHandleSpec(lengthBits uint8) (HandleSpec, error) {
	if lengthBits == 0 {
		lengthBits = 32
	}
	if lengthBits > 32 {
		return 0, fmt.Errorf("lengthBits %d exceeds max allowed 32 bits", lengthBits)
	}
	return HandleSpec(lengthBits), nil
}

// ConfiguredLengthBits extracts the configured length bit width (1-32).
func (s HandleSpec) ConfiguredLengthBits() uint8 {
	return uint8(s)
}

// BitSpace returns the number of bits available for ID (64 - 16 - lengthBits = 48 - lengthBits).
func (s HandleSpec) BitSpace() uint8 {
	return 48 - s.ConfiguredLengthBits()
}

// MaxLength returns the maximum length representable based on configured lengthBits.
func (s HandleSpec) MaxLength() uint64 {
	bits := s.ConfiguredLengthBits()
	if bits >= 64 {
		return ^uint64(0)
	}
	return (uint64(1) << bits) - 1
}

// MaxID returns the maximum ID value that can fit in BitSpace().
func (s HandleSpec) MaxID() uint64 {
	bits := s.BitSpace()
	if bits >= 64 {
		return ^uint64(0)
	}
	return (uint64(1) << bits) - 1
}

// Handle constructs a 64-bit Handle packing offset and length.
func (s HandleSpec) Handle(offset, length int) (Handle, error) {
	if offset < 0 || offset > MaxOffset {
		return 0, fmt.Errorf("%w: got %d", ErrOffsetOutOfBounds, offset)
	}
	if length < 0 {
		return 0, ErrLengthNegative
	}

	maxLen := s.MaxLength()
	if uint64(length) > maxLen {
		return 0, fmt.Errorf("%w: requested %d, max allowed for %d bits is %d",
			ErrLengthExceedsBitSpace, length, s.ConfiguredLengthBits(), maxLen)
	}

	offsetPart := uint64(offset) & 0xFFFF

	lengthBits := s.ConfiguredLengthBits()
	lenMask := (uint64(1) << lengthBits) - 1
	lenPart := (uint64(length) & lenMask) << 16

	return Handle(offsetPart | lenPart), nil
}

// Offset extracts the offset (bits 0-15).
func (h Handle) Offset() int32 {
	return int32(h & 0xFFFF)
}

// Length extracts the stored length using the provided spec configuration.
func (s HandleSpec) Length(h Handle) int32 {
	bits := s.ConfiguredLengthBits()
	mask := (uint64(1) << bits) - 1
	return int32((uint64(h) >> 16) & mask)
}

// Shift adjusts the Handle's offset by delta (positive or negative) and returns the updated Handle.
func (s HandleSpec) Shift(h Handle, delta int) (Handle, error) {
	newOffset := int(h.Offset()) + delta
	if newOffset < 0 || newOffset > MaxOffset {
		return h, fmt.Errorf("%w: got %d", ErrOffsetOutOfBounds, newOffset)
	}

	// Clear existing 16-bit offset (bits 0-15)
	cleared := uint64(h) &^ uint64(0xFFFF)
	offsetPart := uint64(newOffset) & 0xFFFF

	return Handle(cleared | offsetPart), nil
}

// SetID embeds a custom ID into the remaining dynamic bit space (bits 16+N up to 63).
func (s HandleSpec) SetID(h Handle, id uint64) (Handle, error) {
	maxID := s.MaxID()
	if id > maxID {
		return h, fmt.Errorf("%w: requested %d, max allowed for %d bits space is %d",
			ErrIDExceedsBitSpace, id, s.BitSpace(), maxID)
	}

	shift := 16 + s.ConfiguredLengthBits()
	bitSpace := s.BitSpace()

	// Mask out any existing ID bits
	idMask := ((uint64(1) << bitSpace) - 1) << shift
	cleared := uint64(h) &^ idMask

	return Handle(cleared | (id << shift)), nil
}

// ID extracts the custom ID starting at bit 16+N.
func (s HandleSpec) ID(h Handle) uint64 {
	shift := 16 + s.ConfiguredLengthBits()
	bitSpace := s.BitSpace()
	mask := (uint64(1) << bitSpace) - 1
	return (uint64(h) >> shift) & mask
}
