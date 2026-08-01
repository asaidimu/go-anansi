package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStringEscapes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`"plain"`, "plain"},
		{`"a\"b"`, `a"b`},
		{`"a\\b"`, `a\b`},
		{`"a\/b"`, "a/b"},
		{`"a\nb\tc\rd"`, "a\nb\tc\rd"},
		{`"\b\f"`, "\b\f"},
		{`"\u0041"`, "A"},
		{`"\ud83d\ude00"`, "\U0001F600"},
		{`"\u00e9"`, "é"},
		{`"mixed \u0042 and \n"`, "mixed B and \n"},
	}
	for _, c := range cases {
		p := newJSONParser([]byte(c.in))
		got, err := p.parseString()
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, got, c.in)
		assert.True(t, p.eof(), c.in)
	}
}

func TestParseStringErrors(t *testing.T) {
	for _, in := range []string{
		`"unterminated`,
		`"bad \x escape"`,
		`"\u12"`,
		`"\uZZZZ"`,
		`"ctl \x01 char"`,
	} {
		p := newJSONParser([]byte(in))
		_, err := p.parseString()
		require.Error(t, err, in)
	}
}

func TestParseNumber(t *testing.T) {
	p := newJSONParser([]byte(`0`))
	n, err := p.parseInteger()
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	for _, c := range []struct {
		in   string
		want int64
	}{
		{`-17`, -17},
		{`42`, 42},
		{`9007199254740991`, 9007199254740991},
	} {
		p := newJSONParser([]byte(c.in))
		n, err := p.parseInteger()
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, n, c.in)
	}

	for _, c := range []struct {
		in   string
		want float64
	}{
		{`1.5`, 1.5},
		{`-2.25`, -2.25},
		{`1e3`, 1000},
		{`1.5E-2`, 0.015},
		{`0.1`, 0.1},
	} {
		p := newJSONParser([]byte(c.in))
		f, err := p.parseFloat()
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, f, c.in)
	}

	// Integer fields must reject non-integral literals.
	p = newJSONParser([]byte(`1.5`))
	_, err = p.parseInteger()
	require.Error(t, err)

	// Malformed numbers.
	for _, in := range []string{`-`, `01`, `-01`, `1.`, `1e`, `+1`} {
		p := newJSONParser([]byte(in))
		_, err := p.parseAny()
		require.Error(t, err, in)
	}
}

func TestParseAnyTypes(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{`true`, true},
		{`false`, false},
		{`null`, nil},
		{`"hi"`, "hi"},
		{`42`, int64(42)},
		{`4.5`, 4.5},
		{`[1, 2]`, []any{int64(1), int64(2)}},
		{`{"a": 1}`, map[string]any{"a": int64(1)}},
		{`{"a": [true, null], "b": {"c": 1.5}}`, map[string]any{"a": []any{true, nil}, "b": map[string]any{"c": 1.5}}},
	}
	for _, c := range cases {
		p := newJSONParser([]byte(c.in))
		got, err := p.parseAny()
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, got, c.in)
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{
		``,
		`{`,
		`[`,
		`[1`,
		`[1 2]`,
		`{"a" 1}`,
		`{"a":}`,
		`tru`,
		`nul`,
		`'single'`,
	} {
		p := newJSONParser([]byte(in))
		_, err := p.parseAny()
		require.Error(t, err, in)
	}
}
