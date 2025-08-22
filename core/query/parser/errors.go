package parser

import (
	"errors"
)

// Pre-defined errors for the parser package.
var (
	ErrUnexpectedCharacter     = errors.New("unexpected character")
	ErrUnterminatedStringLiteral = errors.New("unterminated string literal")
)
