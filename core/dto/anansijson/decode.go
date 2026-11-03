package anansijson

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"reflect"
	"unsafe"
)

// ============================================================================
// PLAN-DRIVEN DECODER
// ============================================================================
//
// Parses schema-driven JSON straight into the struct's fields through the
// compiled plan. Decisions mirror core/encoding/json's decoder: unknown keys
// are skipped, null leaves a field untouched, integers reject fractional or
// exponent forms, type mismatches error, and missing schema-required fields
// error. Decoded strings are fresh copies — they never alias the input.

const maxDecodeDepth = 512

type parser struct {
	data []byte
	pos  int
}

func decodeWithPlan(root *objPlan, data []byte, v any) error {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return fmt.Errorf("anansijson: unmarshal target must be a non-nil pointer to struct, got %T", v)
		}
		rv = rv.Elem()
	}
	p := &parser{data: data}
	if err := decodeObject(root, p, unsafe.Pointer(rv.UnsafeAddr()), 0); err != nil {
		return err
	}
	p.skipWS()
	if !p.eof() {
		return fmt.Errorf("anansijson: trailing data after JSON document")
	}
	return nil
}

func decodeObject(p *objPlan, ps *parser, base unsafe.Pointer, depth int) error {
	if depth > maxDecodeDepth {
		return fmt.Errorf("anansijson: JSON nesting too deep")
	}
	ps.skipWS()
	if err := ps.expect('{'); err != nil {
		return err
	}

	var seenMask uint64
	var seenHeap []bool
	useHeap := p.nRequired > 64
	if useHeap {
		seenHeap = make([]bool, p.nRequired)
	}

	if !ps.take('}') {
		for {
			key, keySpan, err := ps.parseKeySpan()
			if err != nil {
				return err
			}
			if err := ps.expect(':'); err != nil {
				return err
			}

			n := lookupKey(p, key, keySpan)
			if n == nil {
				if err := ps.skipValue(); err != nil {
					return err
				}
			} else {
				fp := unsafe.Pointer(uintptr(base) + n.offset)
				if err := decodeField(n, ps, fp, depth); err != nil {
					return err
				}
				if n.required {
					idx := requiredPos(p, n)
					if useHeap {
						seenHeap[idx] = true
					} else {
						seenMask |= uint64(1) << uint(idx)
					}
				}
			}

			if ps.take('}') {
				break
			}
			if !ps.take(',') {
				return ps.errf("expected ',' or '}' in object, got %q", ps.peek())
			}
		}
	}

	// Missing required fields error, like the schema-driven decoder.
	for i := 0; i < p.nRequired; i++ {
		seen := seenHeap[i]
		if !useHeap {
			seen = seenMask&(uint64(1)<<uint(i)) != 0
		}
		if !seen {
			return fmt.Errorf("anansijson: missing required field %q", p.fields[p.required[i]].name)
		}
	}
	return nil
}

// requiredPos returns the compact position of n among the plan's required
// fields.
func requiredPos(p *objPlan, n *fieldNode) int {
	for i, r := range p.required {
		if &p.fields[r] == n {
			return i
		}
	}
	return 0
}

// parseKeySpan consumes a JSON string and returns it either as a parsed
// string (escaped keys) or as a raw span into the input (the common case),
// which lets key lookup run allocation-free.
func (ps *parser) parseKeySpan() (string, []byte, error) {
	ps.skipWS()
	if ps.eof() || ps.data[ps.pos] != '"' {
		return "", nil, ps.errf("expected string")
	}
	ps.pos++
	start := ps.pos
	for ps.pos < len(ps.data) {
		c := ps.data[ps.pos]
		if c == '"' {
			span := ps.data[start:ps.pos]
			ps.pos++
			return "", span, nil
		}
		if c == '\\' || c < 0x20 {
			break
		}
		ps.pos++
	}
	ps.pos = start
	s, err := ps.parseString()
	return s, nil, err
}

// lookupKey resolves a raw key span against the plan's field names without
// allocating; escaped keys fall back to the parsed string. Linear scan keeps
// small objects (the overwhelmingly common DTO shape) free of map hashing;
// wide objects use the map.
func lookupKey(p *objPlan, key string, span []byte) *fieldNode {
	if len(p.fields) <= 24 {
		for i := range p.fields {
			n := &p.fields[i]
			if len(n.name) == len(span) && equalStringBytes(n.name, span) {
				return n
			}
		}
		if key == "" {
			return nil
		}
		return p.byName[key]
	}
	if key != "" {
		return p.byName[key]
	}
	if s := byteString(span); s != "" {
		return p.byName[s]
	}
	return p.byName[string(span)]
}

func equalStringBytes(s string, b []byte) bool {
	for i := range b {
		if s[i] != b[i] {
			return false
		}
	}
	return true
}

func byteString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func decodeField(n *fieldNode, ps *parser, fp unsafe.Pointer, depth int) error {
	ps.skipWS()
	if ps.peek() == 'n' {
		// Null is absence: consume it and leave the field untouched.
		return ps.parseNull()
	}

	switch n.kind {
	case kString:
		s, err := ps.parseString()
		if err != nil {
			return err
		}
		*(*string)(fp) = s
	case kInt:
		v, err := ps.parseInteger()
		if err != nil {
			return err
		}
		setInt(fp, n.size, v)
	case kUint:
		v, err := ps.parseInteger()
		if err != nil {
			return err
		}
		if v < 0 {
			return ps.errf("expected unsigned integer, got %d", v)
		}
		setUint(fp, n.size, uint64(v))
	case kFloat:
		f, err := ps.parseFloat()
		if err != nil {
			return err
		}
		if n.size == 4 {
			*(*float32)(fp) = float32(f)
		} else {
			*(*float64)(fp) = f
		}
	case kBool:
		b, err := ps.parseBool()
		if err != nil {
			return err
		}
		*(*bool)(fp) = b
	case kBytes:
		s, err := ps.parseString()
		if err != nil {
			return err
		}
		*(*[]byte)(fp) = decodeBytesValue(s)
	case kObject:
		return decodeObject(n.child, ps, fp, depth+1)
	case kArrayObject:
		return decodeArrayObject(n, ps, fp, depth)
	case kArrayString:
		return decodeStringArray(n, ps, fp)
	case kArrayInt:
		return decodeIntArray(n, ps, fp)
	case kArrayUint:
		return decodeUintArray(n, ps, fp)
	case kArrayFloat:
		return decodeFloatArray(n, ps, fp)
	case kArrayBool:
		return decodeBoolArray(n, ps, fp)
	default:
		return errRefused
	}
	return nil
}

func setInt(fp unsafe.Pointer, size uint8, v int64) {
	switch size {
	case 1:
		*(*int8)(fp) = int8(v)
	case 2:
		*(*int16)(fp) = int16(v)
	case 4:
		*(*int32)(fp) = int32(v)
	default:
		*(*int64)(fp) = v
	}
}

func setUint(fp unsafe.Pointer, size uint8, v uint64) {
	switch size {
	case 1:
		*(*uint8)(fp) = uint8(v)
	case 2:
		*(*uint16)(fp) = uint16(v)
	case 4:
		*(*uint32)(fp) = uint32(v)
	default:
		*(*uint64)(fp) = v
	}
}

// decodeBytesValue mirrors the codec's bytes read-back: invalid base64 is
// tolerated as raw text so hand-written JSON still loads.
func decodeBytesValue(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return []byte(s)
	}
	return b
}

func decodeArrayObject(n *fieldNode, ps *parser, fp unsafe.Pointer, depth int) error {
	ps.skipWS()
	if ps.peek() != '[' {
		return ps.errf("expected array")
	}
	ps.pos++
	h := (*sliceHeader)(fp)
	if ps.take(']') {
		// An empty JSON array decodes to an empty (non-nil) slice, matching
		// the container round-trip.
		setEmptySlice(fp, n.elemType)
		return nil
	}
	for {
		// Grow the slice by hand: the plan knows the element size and type.
		if h.len == h.cap {
			growSlice(h, n.elemSize, n.elemType)
		}
		ep := unsafe.Pointer(uintptr(h.data) + uintptr(h.len)*n.elemSize)
		h.len++
		if err := decodeObject(n.child, ps, ep, depth+1); err != nil {
			return err
		}
		if ps.take(']') {
			return nil
		}
		if !ps.take(',') {
			return ps.errf("expected ',' or ']' in array, got %q", ps.peek())
		}
	}
}

// setEmptySlice stores a zero-length, non-nil slice at fp, matching what the
// container round-trip produces for an empty JSON array.
func setEmptySlice(fp unsafe.Pointer, elemType reflect.Type) {
	st := reflect.SliceOf(elemType)
	reflect.NewAt(st, fp).Elem().Set(reflect.MakeSlice(st, 0, 0))
}

// growSlice doubles a slice in place through its header. The fresh backing
// array is zeroed by reflect.MakeSlice, so a byte-wise copy of the
// initialized prefix keeps the new memory GC-consistent; h.data (an
// unsafe.Pointer field) keeps it reachable afterwards.
func growSlice(h *sliceHeader, elemSize uintptr, elemType reflect.Type) {
	newCap := h.cap * 2
	if newCap == 0 {
		newCap = 4
	}
	sv := reflect.MakeSlice(reflect.SliceOf(elemType), newCap, newCap)
	nd := unsafe.Pointer(sv.Index(0).Addr().UnsafePointer())
	if h.len > 0 {
		src := unsafe.Slice((*byte)(h.data), uintptr(h.len)*elemSize)
		dst := unsafe.Slice((*byte)(nd), uintptr(h.len)*elemSize)
		copy(dst, src)
	}
	h.data = nd
	h.cap = newCap
}

func decodeStringArray(n *fieldNode, ps *parser, fp unsafe.Pointer) error {
	return decodeArray(n, ps, fp, func(ep unsafe.Pointer) error {
		s, err := ps.parseString()
		if err != nil {
			return err
		}
		*(*string)(ep) = s
		return nil
	})
}

func decodeIntArray(n *fieldNode, ps *parser, fp unsafe.Pointer) error {
	return decodeArray(n, ps, fp, func(ep unsafe.Pointer) error {
		v, err := ps.parseInteger()
		if err != nil {
			return err
		}
		setInt(ep, n.size, v)
		return nil
	})
}

func decodeUintArray(n *fieldNode, ps *parser, fp unsafe.Pointer) error {
	return decodeArray(n, ps, fp, func(ep unsafe.Pointer) error {
		v, err := ps.parseInteger()
		if err != nil {
			return err
		}
		if v < 0 {
			return ps.errf("expected unsigned integer, got %d", v)
		}
		setUint(ep, n.size, uint64(v))
		return nil
	})
}

func decodeFloatArray(n *fieldNode, ps *parser, fp unsafe.Pointer) error {
	return decodeArray(n, ps, fp, func(ep unsafe.Pointer) error {
		f, err := ps.parseFloat()
		if err != nil {
			return err
		}
		if n.size == 4 {
			*(*float32)(ep) = float32(f)
		} else {
			*(*float64)(ep) = f
		}
		return nil
	})
}

func decodeBoolArray(n *fieldNode, ps *parser, fp unsafe.Pointer) error {
	return decodeArray(n, ps, fp, func(ep unsafe.Pointer) error {
		b, err := ps.parseBool()
		if err != nil {
			return err
		}
		*(*bool)(ep) = b
		return nil
	})
}

// decodeArray walks a JSON array, invoking set for each element at its slot
// in the (grown) backing array.
func decodeArray(n *fieldNode, ps *parser, fp unsafe.Pointer, set func(ep unsafe.Pointer) error) error {
	ps.skipWS()
	if ps.peek() != '[' {
		return ps.errf("expected array")
	}
	ps.pos++
	h := (*sliceHeader)(fp)
	if ps.take(']') {
		setEmptySlice(fp, n.elemType)
		return nil
	}
	for {
		if h.len == h.cap {
			growSlice(h, n.elemSize, n.elemType)
		}
		ep := unsafe.Pointer(uintptr(h.data) + uintptr(h.len)*n.elemSize)
		h.len++
		if err := set(ep); err != nil {
			return err
		}
		if ps.take(']') {
			return nil
		}
		if !ps.take(',') {
			return ps.errf("expected ',' or ']' in array, got %q", ps.peek())
		}
	}
}

// ----------------------------------------------------------------------------
// JSON primitives
// ----------------------------------------------------------------------------

func (ps *parser) eof() bool { return ps.pos >= len(ps.data) }

func (ps *parser) peek() byte {
	if ps.eof() {
		return 0
	}
	return ps.data[ps.pos]
}

func (ps *parser) errf(format string, args ...any) error {
	return fmt.Errorf("anansijson: "+format, args...)
}

func (ps *parser) skipWS() {
	for ps.pos < len(ps.data) {
		switch ps.data[ps.pos] {
		case ' ', '\t', '\n', '\r':
			ps.pos++
		default:
			return
		}
	}
}

// take consumes b if it is the next non-whitespace byte, mirroring the
// codec parser's whitespace-tolerant scanning between tokens.
func (ps *parser) take(b byte) bool {
	ps.skipWS()
	if !ps.eof() && ps.data[ps.pos] == b {
		ps.pos++
		return true
	}
	return false
}

func (ps *parser) expect(b byte) error {
	if ps.take(b) {
		return nil
	}
	if ps.eof() {
		return ps.errf("unexpected end of input, expected %q", string(b))
	}
	return ps.errf("expected %q, got %q", string(b), string(ps.data[ps.pos]))
}

func (ps *parser) literal(word string) error {
	if ps.pos+len(word) <= len(ps.data) && string(ps.data[ps.pos:ps.pos+len(word)]) == word {
		ps.pos += len(word)
		return nil
	}
	return ps.errf("invalid literal")
}

func (ps *parser) parseNull() error {
	return ps.literal("null")
}

func (ps *parser) parseBool() (bool, error) {
	ps.skipWS()
	if ps.take('t') {
		return true, ps.literal("rue")
	}
	if ps.take('f') {
		return false, ps.literal("alse")
	}
	return false, ps.errf("expected boolean")
}

func (ps *parser) parseString() (string, error) {
	ps.skipWS()
	if ps.eof() || ps.data[ps.pos] != '"' {
		return "", ps.errf("expected string")
	}
	ps.pos++
	start := ps.pos

	for ps.pos < len(ps.data) {
		c := ps.data[ps.pos]
		if c == '"' {
			s := string(ps.data[start:ps.pos])
			ps.pos++
			return s, nil
		}
		if c == '\\' || c < 0x20 {
			break
		}
		ps.pos++
	}

	ps.pos = start
	return ps.parseStringSlow()
}

func (ps *parser) parseStringSlow() (string, error) {
	// Copy the raw literal and unescape into it. Escaped quotes are skipped
	// so the scan finds the real closing quote.
	start := ps.pos
	for ps.pos < len(ps.data) {
		c := ps.data[ps.pos]
		if c == '"' {
			raw := ps.data[start:ps.pos]
			ps.pos++
			return unescapeJSON(raw)
		}
		if c == '\\' {
			ps.pos += 2
			continue
		}
		if c < 0x20 {
			return "", ps.errf("control character in string")
		}
		ps.pos++
	}
	return "", ps.errf("unterminated string")
}

// unescapeJSON decodes a raw JSON string body (no surrounding quotes) into a
// fresh string.
func unescapeJSON(raw []byte) (string, error) {
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' {
			if c < 0x20 {
				return "", fmt.Errorf("anansijson: control character in string")
			}
			out = append(out, c)
			continue
		}
		i++
		if i >= len(raw) {
			return "", fmt.Errorf("anansijson: unterminated escape")
		}
		switch raw[i] {
		case '"':
			out = append(out, '"')
		case '\\':
			out = append(out, '\\')
		case '/':
			out = append(out, '/')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			r, n, err := decodeUnicodeEscape(raw[i+1:])
			if err != nil {
				return "", err
			}
			i += n
			out = appendRune(out, r)
		default:
			return "", fmt.Errorf("anansijson: invalid escape %q", string(raw[i]))
		}
	}
	return string(out), nil
}

// decodeUnicodeEscape decodes \uXXXX at the head of b, joining surrogate
// pairs. It returns the rune and the number of bytes consumed (excluding the
// leading 'u').
func decodeUnicodeEscape(b []byte) (rune, int, error) {
	if len(b) < 4 {
		return 0, 0, fmt.Errorf("anansijson: truncated unicode escape")
	}
	r, err := parseHex4(b[:4])
	if err != nil {
		return 0, 0, err
	}
	if r >= 0xD800 && r <= 0xDBFF && len(b) >= 10 && b[4] == '\\' && b[5] == 'u' {
		lo, err := parseHex4(b[6:10])
		if err != nil {
			return 0, 0, err
		}
		if lo >= 0xDC00 && lo <= 0xDFFF {
			return ((r-0xD800)<<10 | (lo - 0xDC00)) + 0x10000, 10, nil
		}
	}
	return r, 4, nil
}

func parseHex4(b []byte) (rune, error) {
	var v rune
	for _, c := range b {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			v |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= rune(c-'A') + 10
		default:
			return 0, fmt.Errorf("anansijson: invalid unicode escape")
		}
	}
	return v, nil
}

func appendRune(dst []byte, r rune) []byte {
	switch {
	case r < 0x80:
		return append(dst, byte(r))
	case r < 0x800:
		return append(dst, byte(0xC0|r>>6), byte(0x80|r&0x3F))
	case r < 0x10000:
		return append(dst, byte(0xE0|r>>12), byte(0x80|(r>>6)&0x3F), byte(0x80|r&0x3F))
	default:
		return append(dst, byte(0xF0|r>>18), byte(0x80|(r>>12)&0x3F), byte(0x80|(r>>6)&0x3F), byte(0x80|r&0x3F))
	}
}

// numberLit scans a JSON number token.
func (ps *parser) numberLit() ([]byte, error) {
	ps.skipWS()
	start := ps.pos
	if ps.take('-') {
	}
	if ps.eof() {
		return nil, ps.errf("expected number")
	}
	if ps.peek() == '0' {
		ps.pos++
	} else if c := ps.peek(); c >= '1' && c <= '9' {
		for !ps.eof() && ps.data[ps.pos] >= '0' && ps.data[ps.pos] <= '9' {
			ps.pos++
		}
	} else {
		return nil, ps.errf("expected number")
	}
	if !ps.eof() && ps.data[ps.pos] == '.' {
		ps.pos++
		for !ps.eof() && ps.data[ps.pos] >= '0' && ps.data[ps.pos] <= '9' {
			ps.pos++
		}
	}
	if !ps.eof() && (ps.data[ps.pos] == 'e' || ps.data[ps.pos] == 'E') {
		ps.pos++
		if !ps.eof() && (ps.data[ps.pos] == '+' || ps.data[ps.pos] == '-') {
			ps.pos++
		}
		for !ps.eof() && ps.data[ps.pos] >= '0' && ps.data[ps.pos] <= '9' {
			ps.pos++
		}
	}
	if ps.pos == start {
		return nil, ps.errf("expected number")
	}
	return ps.data[start:ps.pos], nil
}

func (ps *parser) parseInteger() (int64, error) {
	lit, err := ps.numberLit()
	if err != nil {
		return 0, err
	}
	for _, c := range lit {
		if c == '.' || c == 'e' || c == 'E' {
			return 0, ps.errf("expected integer, got %q", lit)
		}
	}
	n, err := strconv.ParseInt(string(lit), 10, 64)
	if err != nil {
		return 0, ps.errf("invalid integer %q", lit)
	}
	return n, nil
}

func (ps *parser) parseFloat() (float64, error) {
	lit, err := ps.numberLit()
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(string(lit), 64)
	if err != nil {
		return 0, ps.errf("invalid number %q", lit)
	}
	return f, nil
}

// skipValue consumes one JSON value of any shape without interpreting it.
func (ps *parser) skipValue() error {
	return ps.skipValueDepth(0)
}

func (ps *parser) skipValueDepth(depth int) error {
	if depth > maxDecodeDepth {
		return ps.errf("JSON nesting too deep")
	}
	ps.skipWS()
	if ps.eof() {
		return ps.errf("unexpected end of input")
	}
	switch c := ps.peek(); c {
	case '"':
		_, err := ps.parseString()
		return err
	case '{':
		ps.pos++
		if ps.take('}') {
			return nil
		}
		for {
			if _, _, err := ps.parseKeySpan(); err != nil {
				return err
			}
			if err := ps.expect(':'); err != nil {
				return err
			}
			if err := ps.skipValueDepth(depth + 1); err != nil {
				return err
			}
			if ps.take('}') {
				return nil
			}
			if !ps.take(',') {
				return ps.errf("expected ',' or '}' in object, got %q", ps.peek())
			}
		}
	case '[':
		ps.pos++
		if ps.take(']') {
			return nil
		}
		for {
			if err := ps.skipValueDepth(depth + 1); err != nil {
				return err
			}
			if ps.take(']') {
				return nil
			}
			if !ps.take(',') {
				return ps.errf("expected ',' or ']' in array, got %q", ps.peek())
			}
		}
	case 't':
		return ps.literal("true")
	case 'f':
		return ps.literal("false")
	case 'n':
		return ps.literal("null")
	default:
		_, err := ps.numberLit()
		return err
	}
}
