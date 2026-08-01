package main

import (
	"fmt"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"
)

// jsonParser is a minimal hand-written JSON parser. It never materialises a
// generic map[string]any tree: the schema-driven call sites parse values
// straight into the DataContainer's typed slots, or skip them entirely when
// the key does not exist in the schema.
type jsonParser struct {
	data    []byte
	pos     int
	scratch []byte // Reusable byte slice for unescaping string literals

	// zeroCopy, when true, makes the unescaped fast-path of parseString
	// alias data directly instead of copying it into a new string. See
	// newJSONParserUnsafe for the lifetime contract this requires of the
	// caller. Escaped strings (the slow path) always copy regardless of
	// this flag, since unescaping necessarily produces new bytes.
	zeroCopy bool
}

func newJSONParser(data []byte) *jsonParser {
	return &jsonParser{
		data:    data,
		scratch: make([]byte, 0, 64),
	}
}

// newJSONParserUnsafe is identical to newJSONParser, except that unescaped
// strings parsed off the fast path will alias data instead of being copied.
// The caller must guarantee data is not mutated or reused for as long as
// anything decoded from it remains in use.
func newJSONParserUnsafe(data []byte) *jsonParser {
	return &jsonParser{
		data:     data,
		scratch:  make([]byte, 0, 64),
		zeroCopy: true,
	}
}

func (p *jsonParser) errf(format string, args ...any) error {
	return fmt.Errorf("document-decoder: json at byte %d: %s", p.pos, fmt.Sprintf(format, args...))
}

func (p *jsonParser) eof() bool { return p.pos >= len(p.data) }

func (p *jsonParser) skipWS() {
	for !p.eof() {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *jsonParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.data[p.pos]
}

// isNext reports whether the next non-whitespace byte is b.
func (p *jsonParser) isNext(b byte) bool {
	p.skipWS()
	return !p.eof() && p.data[p.pos] == b
}

// take consumes b if it is the next non-whitespace byte.
func (p *jsonParser) take(b byte) bool {
	if p.isNext(b) {
		p.pos++
		return true
	}
	return false
}

func (p *jsonParser) expect(b byte) error {
	if !p.take(b) {
		return p.errf("expected %q, got %q", b, p.peek())
	}
	return nil
}

// literal consumes an exact keyword.
func (p *jsonParser) literal(word string) error {
	p.skipWS()
	if p.pos+len(word) > len(p.data) || string(p.data[p.pos:p.pos+len(word)]) != word {
		return p.errf("expected %q", word)
	}
	p.pos += len(word)
	return nil
}

func (p *jsonParser) parseString() (string, error) {
	p.skipWS()
	if p.eof() || p.data[p.pos] != '"' {
		return "", p.errf("expected string")
	}
	p.pos++
	start := p.pos

	// FAST-PATH: Scan for closing quote without escape characters or control bytes.
	// Avoids buffer/strings.Builder allocation entirely for plain JSON strings & keys.
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == '"' {
			s := p.sliceString(start, p.pos)
			p.pos++
			return s, nil
		}
		if c == '\\' || c < 0x20 {
			break
		}
		p.pos++
	}

	// SLOW-PATH: Handle escape sequences using parser's scratch buffer.
	p.pos = start
	return p.parseStringSlow()
}

// sliceString materializes the string for data[start:end]. In zero-copy mode
// it aliases the underlying bytes directly (see newJSONParserUnsafe for the
// lifetime contract this requires); otherwise it copies, as string(...)
// conversion normally would.
func (p *jsonParser) sliceString(start, end int) string {
	if start == end {
		return ""
	}
	if p.zeroCopy {
		return unsafe.String(&p.data[start], end-start)
	}
	return string(p.data[start:end])
}

func (p *jsonParser) parseStringSlow() (string, error) {
	p.scratch = p.scratch[:0]
	for !p.eof() {
		c := p.data[p.pos]
		switch {
		case c == '"':
			p.pos++
			return string(p.scratch), nil
		case c == '\\':
			p.pos++
			if p.eof() {
				return "", p.errf("unterminated escape sequence")
			}
			switch e := p.data[p.pos]; e {
			case '"', '\\', '/':
				p.scratch = append(p.scratch, e)
				p.pos++
			case 'b':
				p.scratch = append(p.scratch, '\b')
				p.pos++
			case 'f':
				p.scratch = append(p.scratch, '\f')
				p.pos++
			case 'n':
				p.scratch = append(p.scratch, '\n')
				p.pos++
			case 'r':
				p.scratch = append(p.scratch, '\r')
				p.pos++
			case 't':
				p.scratch = append(p.scratch, '\t')
				p.pos++
			case 'u':
				r, err := p.parseUnicodeEscape()
				if err != nil {
					return "", err
				}
				p.scratch = utf8.AppendRune(p.scratch, r)
			default:
				return "", p.errf("invalid escape sequence \\%c", e)
			}
		case c < 0x20:
			return "", p.errf("control character in string")
		default:
			p.scratch = append(p.scratch, c)
			p.pos++
		}
	}
	return "", p.errf("unterminated string")
}

// parseUnicodeEscape consumes a \uXXXX escape (with optional surrogate pair).
// The backslash has already been consumed; p.pos points at 'u'.
func (p *jsonParser) parseUnicodeEscape() (rune, error) {
	if p.eof() || p.data[p.pos] != 'u' {
		return 0, p.errf("expected \\u escape")
	}
	p.pos++
	r, err := p.parseHex4()
	if err != nil {
		return 0, err
	}
	if utf16.IsSurrogate(r) {
		if p.pos+1 < len(p.data) && p.data[p.pos] == '\\' && p.data[p.pos+1] == 'u' {
			p.pos += 2
			r2, err := p.parseHex4()
			if err != nil {
				return 0, err
			}
			if dec := utf16.DecodeRune(r, r2); dec != utf8.RuneError {
				r = dec
			}
		}
	}
	return r, nil
}

func (p *jsonParser) parseHex4() (rune, error) {
	if p.pos+4 > len(p.data) {
		return 0, p.errf("truncated \\u escape")
	}
	var v rune
	for i := 0; i < 4; i++ {
		h, err := hexVal(p.data[p.pos+i])
		if err != nil {
			return 0, p.errf("%s", err)
		}
		v = v<<4 | rune(h)
	}
	p.pos += 4
	return v, nil
}

func hexVal(b byte) (byte, error) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', nil
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, nil
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, nil
	}
	return 0, fmt.Errorf("invalid hex digit %q", b)
}

// numberLit scans a JSON number and returns the raw literal bytes.
func (p *jsonParser) numberLit() ([]byte, error) {
	p.skipWS()
	start := p.pos
	if !p.eof() && p.data[p.pos] == '-' {
		p.pos++
	}
	if p.eof() || !isDigit(p.data[p.pos]) {
		return nil, p.errf("expected number")
	}
	intStart := p.pos
	for !p.eof() && isDigit(p.data[p.pos]) {
		p.pos++
	}
	// Strict JSON forbids leading zeros ("01", "-01").
	if p.data[intStart] == '0' && p.pos > intStart+1 {
		return nil, p.errf("leading zero in number")
	}
	if !p.eof() && p.data[p.pos] == '.' {
		p.pos++
		if p.eof() || !isDigit(p.data[p.pos]) {
			return nil, p.errf("expected digit after '.'")
		}
		for !p.eof() && isDigit(p.data[p.pos]) {
			p.pos++
		}
	}
	if !p.eof() && (p.data[p.pos] == 'e' || p.data[p.pos] == 'E') {
		p.pos++
		if !p.eof() && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
			p.pos++
		}
		if p.eof() || !isDigit(p.data[p.pos]) {
			return nil, p.errf("expected digit in exponent")
		}
		for !p.eof() && isDigit(p.data[p.pos]) {
			p.pos++
		}
	}
	return p.data[start:p.pos], nil
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func (p *jsonParser) parseInteger() (int64, error) {
	lit, err := p.numberLit()
	if err != nil {
		return 0, err
	}
	for _, c := range lit {
		if c == '.' || c == 'e' || c == 'E' {
			return 0, p.errf("expected integer, got %q", lit)
		}
	}
	n, err := strconv.ParseInt(string(lit), 10, 64)
	if err != nil {
		return 0, p.errf("invalid integer %q", lit)
	}
	return n, nil
}

func (p *jsonParser) parseFloat() (float64, error) {
	lit, err := p.numberLit()
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(string(lit), 64)
	if err != nil {
		return 0, p.errf("invalid number %q", lit)
	}
	return f, nil
}

func (p *jsonParser) parseBool() (bool, error) {
	p.skipWS()
	if p.pos+4 <= len(p.data) && string(p.data[p.pos:p.pos+4]) == "true" {
		p.pos += 4
		return true, nil
	}
	if p.pos+5 <= len(p.data) && string(p.data[p.pos:p.pos+5]) == "false" {
		p.pos += 5
		return false, nil
	}
	return false, p.errf("expected boolean")
}

func (p *jsonParser) parseNull() error {
	return p.literal("null")
}

// parseArray drives a generic JSON array: it consumes '[' and calls elem for
// each element, handling separators.
func (p *jsonParser) parseArray(elem func() error) error {
	if err := p.expect('['); err != nil {
		return err
	}
	if p.take(']') {
		return nil
	}
	for {
		if err := elem(); err != nil {
			return err
		}
		if p.take(']') {
			return nil
		}
		if !p.take(',') {
			return p.errf("expected ',' or ']' in array, got %q", p.peek())
		}
	}
}

// parseAny parses an arbitrary JSON value (used for TypeUnknown fields and
// records). Numbers decode to int64 when integral, float64 otherwise.
func (p *jsonParser) parseAny() (any, error) {
	p.skipWS()
	switch c := p.peek(); {
	case c == '"':
		return p.parseString()
	case c == '{':
		return p.parseObjectAny()
	case c == '[':
		return p.parseArrayAny()
	case c == 't':
		if err := p.literal("true"); err != nil {
			return nil, err
		}
		return true, nil
	case c == 'f':
		if err := p.literal("false"); err != nil {
			return nil, err
		}
		return false, nil
	case c == 'n':
		if err := p.literal("null"); err != nil {
			return nil, err
		}
		return nil, nil
	case c == '-' || isDigit(c):
		lit, err := p.numberLit()
		if err != nil {
			return nil, err
		}
		hasDot := false
		for _, ch := range lit {
			if ch == '.' || ch == 'e' || ch == 'E' {
				hasDot = true
				break
			}
		}
		if hasDot {
			return strconv.ParseFloat(string(lit), 64)
		}
		return strconv.ParseInt(string(lit), 10, 64)
	default:
		return nil, p.errf("unexpected byte %q", c)
	}
}

func (p *jsonParser) parseObjectAny() (map[string]any, error) {
	out := map[string]any{}
	if err := p.expect('{'); err != nil {
		return nil, err
	}
	if p.take('}') {
		return out, nil
	}
	for {
		key, err := p.parseString()
		if err != nil {
			return nil, err
		}
		if err := p.expect(':'); err != nil {
			return nil, err
		}
		v, err := p.parseAny()
		if err != nil {
			return nil, err
		}
		out[key] = v
		if p.take('}') {
			return out, nil
		}
		if !p.take(',') {
			return nil, p.errf("expected ',' or '}' in object, got %q", p.peek())
		}
	}
}

func (p *jsonParser) parseArrayAny() ([]any, error) {
	out := make([]any, 0, 8)
	err := p.parseArray(func() error {
		v, err := p.parseAny()
		if err != nil {
			return err
		}
		out = append(out, v)
		return nil
	})
	return out, err
}

// skipValue discards an arbitrary JSON value.
func (p *jsonParser) skipValue() error {
	_, err := p.parseAny()
	return err
}

// ── typed helpers used by the leaf parser ────────────────────────────────────

func parseGeometry(p *jsonParser) ([][]float64, error) {
	out := make([][]float64, 0, 4)
	err := p.parseArray(func() error {
		ring := make([]float64, 0, 8)
		err := p.parseArray(func() error {
			f, err := p.parseFloat()
			if err != nil {
				return err
			}
			ring = append(ring, f)
			return nil
		})
		if err != nil {
			return err
		}
		out = append(out, ring)
		return nil
	})
	return out, err
}

func parseArrayInt(p *jsonParser) ([]int64, error) {
	out := make([]int64, 0, 8)
	err := p.parseArray(func() error {
		n, err := p.parseInteger()
		if err != nil {
			return err
		}
		out = append(out, n)
		return nil
	})
	return out, err
}

func parseArrayFloat(p *jsonParser) ([]float64, error) {
	out := make([]float64, 0, 8)
	err := p.parseArray(func() error {
		f, err := p.parseFloat()
		if err != nil {
			return err
		}
		out = append(out, f)
		return nil
	})
	return out, err
}

func parseArrayString(p *jsonParser) ([]string, error) {
	out := make([]string, 0, 8)
	err := p.parseArray(func() error {
		s, err := p.parseString()
		if err != nil {
			return err
		}
		out = append(out, s)
		return nil
	})
	return out, err
}

func parseArrayBool(p *jsonParser) ([]bool, error) {
	out := make([]bool, 0, 8)
	err := p.parseArray(func() error {
		b, err := p.parseBool()
		if err != nil {
			return err
		}
		out = append(out, b)
		return nil
	})
	return out, err
}

func parseArrayBytes(p *jsonParser) ([][]byte, error) {
	out := make([][]byte, 0, 8)
	err := p.parseArray(func() error {
		s, err := p.parseString()
		if err != nil {
			return err
		}
		out = append(out, []byte(s))
		return nil
	})
	return out, err
}

func parseArrayGeometry(p *jsonParser) ([][][]float64, error) {
	out := make([][][]float64, 0, 4)
	err := p.parseArray(func() error {
		g, err := parseGeometry(p)
		if err != nil {
			return err
		}
		out = append(out, g)
		return nil
	})
	return out, err
}
