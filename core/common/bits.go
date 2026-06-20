package common

type ResultSet struct {
	// data holds 2 bits per node:
	// bit 0: hasValue (presence)
	// bit 1: value (success/failure)
	data []uint64
}

func NewResultState(size int) *ResultSet {
	return &ResultSet{
		data: make([]uint64, ((size+31)/32)+1),
	}
}

func (s *ResultSet) word(idx int) (int, uint, bool) {
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
func (s *ResultSet) HasValue(idx int) bool {
	word, shift, ok := s.word(idx)
	if !ok {
		return false
	}
	return (s.data[word]>>shift)&1 != 0
}

// Value returns the success status of the node.
// Only meaningful if HasValue() is true.
func (s *ResultSet) Value(idx int) bool {
	word, shift, ok := s.word(idx)
	if !ok {
		return false
	}
	return (s.data[word]>>(shift+1))&1 != 0
}

// Set updates both the presence and the success value
func (s *ResultSet) Set(idx int, success bool) {
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

func (s *ResultSet) Clear() {
	for i := range s.data {
		s.data[i] = 0
	}
}

