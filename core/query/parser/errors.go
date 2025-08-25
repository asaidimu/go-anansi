package parser

import (
	"errors"
	"strings"
)

// LexerError represents errors specific to lexer operations.
type LexerError struct {
	Operation string
	Key       string
	Message   string
	Cause     error
}

func (e *LexerError) Error() string {
	var b strings.Builder
	b.WriteString(e.Operation)
	b.WriteString(" operation failed")

	if e.Key != "" {
		b.WriteString(" for key '")
		b.WriteString(e.Key)
		b.WriteString("': ")
	} else {
		b.WriteString(": ")
	}
	b.WriteString(e.Message)

	if e.Cause != nil {
		b.WriteString(" (caused by: ")
		b.WriteString(e.Cause.Error())
		b.WriteString(")")
	}
	return b.String()
}

func (e *LexerError) Unwrap() error {
	return e.Cause
}

// Pre-defined errors for the lexer package.
var (
	ErrUnexpectedCharacter = errors.New("unexpected character")
	ErrUnterminatedString  = errors.New("unterminated string literal")
)