package common

type BitState struct {
	// data holds 2 bits per node:
	// bit 0: hasValue (presence)
	// bit 1: value (success/failure)
	data []uint64
}

func NewBitState(nodeCount int) *BitState {
	// 32 nodes per uint64 (2 bits each)
	// +1 word intentionally keeps legacy behavior safe
	return &BitState{
		data: make([]uint64, ((nodeCount+31)/32)+1),
	}
}

func (s *BitState) word(idx int) (int, uint, bool) {
	if idx < 0 {
		return 0, 0, false
	}
	word := idx / 32
	if word >= len(s.data) {
		return 0, 0, false
	}
	shift := uint((idx % 32) * 2)
	return word, shift, true
}

// HasValue returns true if the node has been processed (visited)
func (s *BitState) HasValue(idx int) bool {
	word, shift, ok := s.word(idx)
	if !ok {
		return false
	}
	return (s.data[word]>>shift)&1 != 0
}

// Value returns the success status of the node.
// Only meaningful if HasValue() is true.
func (s *BitState) Value(idx int) bool {
	word, shift, ok := s.word(idx)
	if !ok {
		return false
	}
	return (s.data[word]>>(shift+1))&1 != 0
}

// Set updates both the presence and the success value
func (s *BitState) Set(idx int, success bool) {
	word, shift, ok := s.word(idx)
	if !ok {
		return
	}

	// Clear both bits
	mask := uint64(3) << shift
	s.data[word] &= ^mask

	// Set presence + value
	val := uint64(1)
	if success {
		val |= 2
	}
	s.data[word] |= val << shift
}

func (s *BitState) Clear() {
	for i := range s.data {
		s.data[i] = 0
	}
}

